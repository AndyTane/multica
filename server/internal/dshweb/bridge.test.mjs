import assert from 'node:assert/strict';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import WS, { WebSocketServer } from 'ws';
import { execute, localOrigin, loadConfig, taskCli, connectWeb, TurnReader, SessionJournal, probeWeb } from './bridge.mjs';

// This host has no DSH data directory or session files. Only its public wire contract exists.
async function fixture(t, behavior = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'multica-dsh-api-test-'));
  const cwd = path.join(root, 'project');
  fs.mkdirSync(cwd);
  const sessions = new Map(), calls = [], timers = [], followers = new Set();
  const entry = event => ({ type: 'event', event });
  function seed(id, options = {}) {
    const session = { header: { id, version: 99, createdAt: Date.now(), cwd, isSeeded: false, ...options }, rows: [], turn: 0 };
    sessions.set(id, session);
    return session;
  }
  function append(session, rows) {
    for (const row of rows) {
      const event = { seq: session.rows.length, time: Date.now(), ...row };
      session.rows.push(event);
      for (const follower of followers) {
        if (follower.session !== session || follower.socket.readyState !== WS.OPEN) continue;
        if (behavior.dropAtType === event.type) {
          behavior.dropAtType = undefined;
          follower.socket.terminate();
        } else if (behavior.skipAtType === event.type) {
          behavior.skipAtType = undefined;
        } else follower.send(entry(event));
      }
    }
  }
  const page = (session, before = session.rows.length) => {
    const end = Math.min(before, session.rows.length);
    const start = Math.max(0, end - (behavior.pageSize ?? 50));
    return { records: session.rows.slice(start, end).map(entry), hasMore: start > 0 };
  };
  const server = http.createServer(async (request, response) => {
    response.setHeader('Connection', 'close');
    if (request.url === '/?token=test-secret') {
      response.writeHead(303, { 'Set-Cookie': 'dsh=test-cookie; HttpOnly', Location: '/' }); response.end(); return;
    }
    let body = '';
    for await (const bytes of request) body += bytes;
    const { rpcId, method, payload } = JSON.parse(body);
    assert.equal(request.headers.cookie, 'dsh=test-cookie');
    const args = payload.args.request;
    calls.push({ method, args });
    let value;
    if (method === 'workspace/create') value = { workspace: { workspaceId: 'ws-test', path: cwd, sessionIds: [...sessions.keys()] } };
    else if (method === 'session/create') {
      assert.equal(args.workspaceId, 'ws-test');
      if (!sessions.has(args.sessionId)) seed(args.sessionId);
      value = { sessionId: args.sessionId };
    } else if (method === 'session/rename') value = { title: args.title };
    else if (method === 'session/page') {
      const session = sessions.get(args.address.sessionId);
      assert.ok(Number.isInteger(args.throughSeq));
      assert.ok(args.throughSeq <= session.rows.length - 1);
      value = page(session, Math.min(args.beforeSeq ?? args.throughSeq + 1, args.throughSeq + 1));
    } else if (method === 'session/cancel') {
      const session = sessions.get(args.sessionId);
      if (behavior.cancelAdvance) behavior.cancelAdvance(session, append);
      else append(session, [{ type: 'turn/end', data: { turn: session.turn, reason: { kind: 'user' } } }]);
      value = { accepted: true };
    } else if (method === 'session/prompt') {
      if (behavior.disconnectPrompt) { request.socket.destroy(); return; }
      const session = sessions.get(args.sessionId), turn = ++session.turn;
      assert.equal(args.mode, 'queue');
      timers.push(setTimeout(() => {
        if (behavior.noWitness) return;
        append(session, [
          { type: 'turn/start', data: { turn } },
          { type: 'user/message', data: { role: 'user', content: args.content, source: { kind: 'user', rpcId: args.requestId } } },
          { type: 'tool/call', data: { turn, callId: 'call-test', name: 'bash', arguments: '{"command":"pwd"}' } },
          { type: 'tool/result', data: { turn, message: { role: 'tool', toolCallId: 'call-test', content: [{ type: 'text', text: cwd }], isError: behavior.toolError === true } } },
          ...(behavior.extraEvents ?? []),
          ...behavior.hold ? [] : [
            { type: 'assistant/message', data: { turn, message: { content: [{ type: 'text', text: `reply-${turn}` }], source: { provider: 'test', model: 'fake' } }, usage: { inputTokens: 20, outputTokens: 2 } } },
            { type: 'turn/end', data: { turn, reason: { kind: 'completed' } } },
          ],
        ]);
        behavior.afterAppend?.(session, append);
      }, 15));
      value = { accepted: true };
    } else throw new Error(`Unexpected method ${method}`);
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({ type: 'server-response', rpcId, result: { ok: true, value } }));
  });
  const sockets = new WebSocketServer({ server, path: '/api/remote.mux' });
  sockets.on('connection', (socket, request) => {
    assert.equal(request.headers.cookie, 'dsh=test-cookie');
    assert.equal(request.headers.origin, origin);
    socket.on('message', bytes => {
      const frame = JSON.parse(bytes.toString());
      if (frame.type === 'cancel') return;
      assert.equal(frame.type, 'open');
      assert.equal(frame.endpoint, 'session/follow');
      const args = frame.payload.args.request;
      calls.push({ method: frame.endpoint, args });
      const send = value => socket.send(JSON.stringify({ type: 'item', streamId: frame.streamId, value }));
      if (behavior.unsupported || !sessions.has(args.address.sessionId)) {
        socket.send(JSON.stringify({ type: 'error', streamId: frame.streamId,
          error: { code: behavior.unsupported ? 'gateway/not-found' : 'session/not-found', message: 'private-host-error test-secret', details: {} } }));
        return;
      }
      const session = sessions.get(args.address.sessionId);
      const follower = { socket, session, send };
      followers.add(follower);
      socket.on('close', () => followers.delete(follower));
      const snapshot = { type: 'snapshot', header: session.header, cursor: session.rows.length - 1,
        ...page(session), projections: { asOfSeq: session.rows.length - 1, values: {} } };
      behavior.snapshot?.(snapshot, session);
      send(snapshot);
    });
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const accessLog = path.join(root, 'launch.log');
  fs.writeFileSync(accessLog, `${origin}/?token=test-secret\n`);
  t.after(async () => {
    timers.forEach(clearTimeout);
    sockets.clients.forEach(socket => socket.terminate());
    await new Promise(resolve => sockets.close(resolve));
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
    fs.rmSync(root, { recursive: true, force: true });
  });
  return { cwd, root, sessions, calls, append, seed, followers, behavior,
    config: { origin, accessLog, pollMs: 5, reconnectMs: 5, streamTimeoutMs: 1000 } };
}

const command = (f, id, extra = {}) => ({ v: 1, type: 'execute', request_id: id, cwd: f.cwd, prompt: 'Do the requested work', mcp_servers: [], ...extra });
async function run(f, request, control = { cancelled: false }, dependencies = {}) {
  const frames = [];
  await execute(request, f.config, frame => frames.push(frame), control,
    { env: {}, briefCacheDir: path.join(f.root, 'briefs'), ...dependencies });
  return frames;
}
const prompts = f => f.calls.filter(call => call.method === 'session/prompt');

test('new messages and resume work through the event API without any DSH session files', async t => {
  const f = await fixture(t);
  const first = await run(f, command(f, 'api-first', { thread_name: 'TEST-1' }));
  assert.equal(first.at(-1).status, 'completed');
  assert.equal(first.at(-1).output, 'reply-1');
  const sessionId = first.at(-1).session_id;
  const second = await run(f, command(f, 'api-second', { resume_session_id: sessionId }));
  assert.equal(second.at(-1).status, 'completed');
  assert.equal(second.at(-1).session_id, sessionId);
  assert.deepEqual(second.filter(frame => frame.type === 'text').map(frame => frame.content), ['reply-2']);
  assert.equal(f.sessions.size, 1);
  assert.equal(fs.existsSync(path.join(f.root, 'data')), false);
  assert.equal(f.calls.find(call => call.method === 'session/rename').args.title, 'Multica · TEST-1');
});

test('the standalone stdio entry returns correlated protocol frames through the event API', async t => {
  const f = await fixture(t), file = path.join(f.root, 'bridge-config.json');
  fs.writeFileSync(file, JSON.stringify(f.config));
  const child = spawn(process.execPath, [fileURLToPath(new URL('./bridge.mjs', import.meta.url)), '--stdio'], {
    env: { ...process.env, MULTICA_DSH_WEB_CONFIG: file, MULTICA_TASK_ID: '' },
    stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true,
  });
  t.after(() => child.kill());
  let output = '', error = '';
  child.stdout.on('data', bytes => {
    output += bytes;
    if (output.includes('"type":"result"')) child.stdin.end();
  });
  child.stderr.on('data', bytes => { error += bytes; });
  const completed = new Promise((resolve, reject) => {
    child.on('error', reject);
    child.on('close', code => code === 0 ? resolve() : reject(new Error(`child exit ${code}: ${error}`)));
  });
  child.stdin.write(JSON.stringify(command(f, 'stdio-api')) + '\n');
  await completed;
  const frames = output.trim().split('\n').map(line => JSON.parse(line));
  assert.equal(frames[0].type, 'ready');
  assert.ok(frames.every(frame => frame.v === 1));
  assert.equal(frames.at(-1).status, 'completed');
  assert.equal(frames.at(-1).request_id, 'stdio-api');
  assert.equal(prompts(f).length, 1);
});

async function waitUntil(condition) {
  const deadline = Date.now() + 2000;
  while (!condition()) {
    if (Date.now() > deadline) throw new Error('fixture condition timed out');
    await new Promise(resolve => setTimeout(resolve, 5));
  }
}

test('API events retain tool identity, errors and usage', async t => {
  const f = await fixture(t, { toolError: true });
  const frames = await run(f, command(f, 'api-tools'));
  assert.equal(frames.at(-1).status, 'completed');
  const tool = frames.find(frame => frame.type === 'tool_result');
  assert.deepEqual({ call_id: tool.call_id, name: tool.name, output: tool.output, is_error: tool.is_error },
    { call_id: 'call-test', name: 'bash', output: f.cwd, is_error: true });
  assert.equal(frames.find(frame => frame.type === 'usage').input_tokens, 20);
});

test('completed request recovery is idempotent even while the browser runs another turn', async t => {
  const f = await fixture(t);
  const first = await run(f, command(f, 'api-repeat'));
  const session = f.sessions.get(first.at(-1).session_id);
  f.append(session, [{ type: 'turn/start', data: { turn: 9 } }]);
  const repeated = await run(f, command(f, 'api-repeat'));
  assert.equal(repeated.at(-1).status, 'completed');
  assert.equal(repeated.at(-1).output, 'reply-1');
  assert.equal(prompts(f).length, 1);
});

test('history pages recover a request whose user message is outside the opening window', async t => {
  const f = await fixture(t, { pageSize: 2 });
  const first = await run(f, command(f, 'api-paged'));
  const session = f.sessions.get(first.at(-1).session_id);
  f.append(session, Array.from({ length: 15 }, () => ({ type: 'session/title', data: { title: 'Later metadata' } })));
  const recovered = await run(f, command(f, 'api-paged'));
  assert.equal(recovered.at(-1).status, 'completed');
  assert.equal(recovered.at(-1).output, 'reply-1');
  assert.ok(f.calls.some(call => call.method === 'session/page'));
  assert.equal(prompts(f).length, 1);
});

for (const field of ['cwd', 'origin', 'parentSession', 'delegationDepth']) {
  test(`rejects an existing session with invalid ${field} before workspace adoption`, async t => {
    const f = await fixture(t);
    const other = path.join(f.root, 'other'); fs.mkdirSync(other);
    const invalid = { cwd: other, origin: 'subagent', parentSession: 'parent', delegationDepth: 1 };
    const sessionId = 'multica-web-invalid';
    f.seed(sessionId, { [field]: invalid[field] });
    const result = await run(f, command(f, `reject-${field}`, { resume_session_id: sessionId }));
    assert.equal(result.at(-1).error.code, 'session_project_mismatch');
    assert.equal(f.calls.filter(call => call.method === 'workspace/create' || call.method === 'session/create').length, 0);
    assert.equal(prompts(f).length, 0);
  });
}

test('missing resume identities are not silently recreated', async t => {
  const f = await fixture(t);
  const result = await run(f, command(f, 'missing-resume', { resume_session_id: 'multica-web-missing' }));
  assert.equal(result.at(-1).error.code, 'session/not-found');
  assert.equal(f.sessions.size, 0);
  assert.equal(prompts(f).length, 0);
});

test('another running turn rejects a new prompt without queueing it', async t => {
  const f = await fixture(t);
  const sessionId = 'multica-web-busy', session = f.seed(sessionId);
  f.append(session, [{ type: 'turn/start', data: { turn: 1 } }]);
  const result = await run(f, command(f, 'api-busy', { resume_session_id: sessionId }));
  assert.equal(result.at(-1).error.code, 'session_busy');
  assert.equal(prompts(f).length, 0);
});

for (const dropAtType of ['tool/call', 'assistant/message', 'turn/end']) {
  test(`a disconnect at ${dropAtType} repairs from API history without repeated output or prompts`, async t => {
    const f = await fixture(t, { dropAtType, pageSize: 2 });
    const frames = await run(f, command(f, 'api-reconnect'));
    assert.equal(frames.at(-1).status, 'completed');
    assert.equal(frames.at(-1).output, 'reply-1');
    assert.equal(frames.filter(frame => frame.type === 'tool_call').length, 1);
    assert.equal(frames.filter(frame => frame.type === 'tool_result').length, 1);
    assert.deepEqual(frames.filter(frame => frame.type === 'text').map(frame => frame.content), ['reply-1']);
    assert.equal(prompts(f).length, 1);
  });
}

test('a live sequence gap is repaired through the service', async t => {
  const f = await fixture(t, { skipAtType: 'tool/call' });
  const frames = await run(f, command(f, 'api-gap'));
  assert.equal(frames.at(-1).status, 'completed');
  assert.equal(frames.filter(frame => frame.type === 'tool_call').length, 1);
  assert.equal(prompts(f).length, 1);
});

test('unknown required events fail visibly, while explicitly ignorable additions remain compatible', async t => {
  const behavior = { extraEvents: [{ type: 'future/event', data: { future: true }, ignorable: true }] };
  const f = await fixture(t, behavior);
  assert.equal((await run(f, command(f, 'future-optional'))).at(-1).status, 'completed');
  behavior.extraEvents = [{ type: 'future/required', data: { future: true } }];
  const result = await run(f, command(f, 'future-required'));
  assert.equal(result.at(-1).status, 'failed');
  assert.equal(result.at(-1).error.code, 'session_event_unsupported');
});

test('unsupported event services are detected before a prompt or workspace is created', async t => {
  const f = await fixture(t, { unsupported: true });
  const frames = await run(f, command(f, 'unsupported-api'));
  assert.equal(frames.at(-1).error.code, 'dsh_event_api_unsupported');
  assert.equal(prompts(f).length, 0);
  assert.equal(f.sessions.size, 0);
  assert.ok(!JSON.stringify(frames).includes('test-secret'));
});

test('delivery uncertainty never automatically re-posts the prompt', async t => {
  const f = await fixture(t, { disconnectPrompt: true });
  const frames = await run(f, command(f, 'uncertain-api'));
  assert.equal(frames.at(-1).error.code, 'web_rpc_outcome_unknown');
  assert.match(frames.at(-1).error.message, /Open DSH to inspect/);
  assert.equal(prompts(f).length, 1);
  assert.ok(!JSON.stringify(frames).includes('test-secret'));
});

test('cancel waits for the web-owned turn end', async t => {
  const f = await fixture(t, { hold: true }), control = { cancelled: false };
  const pending = run(f, command(f, 'cancel-api'), control);
  await waitUntil(() => [...f.sessions.values()][0]?.rows.some(row => row.type === 'tool/result'));
  control.cancelled = true;
  const frames = await pending;
  assert.equal(frames.at(-1).status, 'cancelled');
  assert.equal(f.calls.filter(call => call.method === 'session/cancel').length, 1);
});

test('cancel observes a completed turn before attempting to stop a subsequent browser turn', async t => {
  const control = { cancelled: false };
  const f = await fixture(t, { afterAppend(session, append) {
    control.cancelled = true;
    append(session, [{ type: 'turn/start', data: { turn: 99 } }]);
  } });
  const frames = await run(f, command(f, 'cancel-finished'), control);
  assert.equal(frames.at(-1).status, 'completed');
  assert.equal(f.calls.filter(call => call.method === 'session/cancel').length, 0);
});

test('cancel never targets a different active browser turn', async t => {
  const f = await fixture(t, { hold: true }), control = { cancelled: false };
  const pending = run(f, command(f, 'cancel-other'), control);
  await waitUntil(() => [...f.sessions.values()][0]?.rows.some(row => row.type === 'tool/result'));
  const session = [...f.sessions.values()][0];
  f.append(session, [{ type: 'turn/start', data: { turn: 99 } }]);
  control.cancelled = true;
  const frames = await pending;
  assert.equal(frames.at(-1).error.code, 'cancel_target_not_active');
  assert.equal(f.calls.filter(call => call.method === 'session/cancel').length, 0);
});

test('a running admitted request is watched without posting a second prompt', async t => {
  const f = await fixture(t), sessionId = 'multica-web-already-running';
  const session = f.seed(sessionId);
  f.append(session, [
    { type: 'turn/start', data: { turn: 1 } },
    { type: 'user/message', data: { content: [{ type: 'text', text: 'Original' }], source: { kind: 'user', rpcId: 'running-request' } } },
  ]);
  const pending = run(f, command(f, 'running-request', { resume_session_id: sessionId }));
  await waitUntil(() => f.calls.some(call => call.method === 'session/create'));
  f.append(session, [
    { type: 'assistant/message', data: { turn: 1, message: { content: [{ type: 'text', text: 'Recovered' }] } } },
    { type: 'turn/end', data: { turn: 1, reason: { kind: 'completed' } } },
  ]);
  const frames = await pending;
  assert.equal(frames.at(-1).output, 'Recovered');
  assert.equal(prompts(f).length, 0);
});

test('an admitted queue witness prevents another prompt before the turn starts', async t => {
  const f = await fixture(t), sessionId = 'multica-web-already-queued';
  const session = f.seed(sessionId);
  f.append(session, [{ type: 'agent/inbox/spliced', data: {
    inserted: [{ content: [{ type: 'text', text: 'Original' }], source: { kind: 'user', rpcId: 'queued-request' } }],
  } }]);
  const pending = run(f, command(f, 'queued-request', { resume_session_id: sessionId }));
  await waitUntil(() => f.calls.some(call => call.method === 'session/create'));
  await new Promise(resolve => setTimeout(resolve, 40));
  f.append(session, [
    { type: 'turn/start', data: { turn: 1 } },
    { type: 'user/message', data: { content: [{ type: 'text', text: 'Original' }], source: { kind: 'user', rpcId: 'queued-request' } } },
    { type: 'assistant/message', data: { turn: 1, message: { content: [{ type: 'text', text: 'Recovered queue' }] } } },
    { type: 'turn/end', data: { turn: 1, reason: { kind: 'completed' } } },
  ]);
  const frames = await pending;
  assert.equal(frames.at(-1).output, 'Recovered queue');
  assert.equal(prompts(f).length, 0);
});

test('browser steering retains the revised reply and excludes a queued browser follow-up', () => {
  const frames = [], reader = new TurnReader('multica-request', frame => frames.push(frame));
  for (const event of [
    { type: 'turn/start', data: { turn: 1 } },
    { type: 'user/message', data: { source: { kind: 'user', rpcId: 'multica-request' } } },
    { type: 'assistant/message', data: { turn: 1, message: { content: [{ type: 'text', text: 'Initial answer' }] } } },
    { type: 'user/message', data: { source: { kind: 'user', rpcId: 'browser-steering' } } },
    { type: 'assistant/message', data: { turn: 1, message: { content: [{ type: 'text', text: 'Revised answer' }] } } },
    { type: 'turn/end', data: { turn: 1, reason: { kind: 'completed' } } },
    { type: 'turn/start', data: { turn: 2 } },
    { type: 'assistant/message', data: { turn: 2, message: { content: [{ type: 'text', text: 'Browser only' }] } } },
    { type: 'turn/end', data: { turn: 2, reason: { kind: 'completed' } } },
  ]) reader.push(event);
  assert.equal(reader.terminal.output, 'Revised answer');
  assert.deepEqual(frames.map(frame => frame.content), ['Initial answer', 'Revised answer']);
});

test('malformed snapshots, required events and incomplete history fail before delivery', async t => {
  const cases = [
    snapshot => { snapshot.cursor = '0'; },
    snapshot => { snapshot.records = null; },
    snapshot => { snapshot.hasMore = true; },
    snapshot => { snapshot.cursor = 1; },
    snapshot => { snapshot.records = [{ type: 'event', event: { type: 'turn/start', seq: 0, time: 0, data: { turn: 'wrong' } } }]; snapshot.cursor = 0; },
    snapshot => { snapshot.records = [{ type: 'event', event: { type: 'future/event', seq: 0, time: 0, data: {} } }]; snapshot.cursor = 0; },
  ];
  for (const snapshot of cases) {
    const f = await fixture(t, { snapshot });
    const sessionId = 'multica-web-malformed'; f.seed(sessionId);
    const result = await run(f, command(f, 'malformed', { resume_session_id: sessionId }));
    assert.equal(result.at(-1).status, 'failed');
    assert.equal(prompts(f).length, 0);
  }
});

test('reconnection refuses changed history instead of associating another result', async t => {
  const f = await fixture(t), sessionId = 'multica-web-changed';
  const session = f.seed(sessionId);
  f.append(session, [{ type: 'session/title', data: { title: 'Before' } }]);
  const journal = new SessionJournal(await connectWeb(f.config), f.config, sessionId, f.cwd);
  t.after(() => journal.close());
  await journal.refresh();
  session.rows[0].data.title = 'After';
  await assert.rejects(journal.refresh(), { message: 'session_history_changed' });
});

test('repeated broken reconnections stop after a bounded number of attempts', async t => {
  const f = await fixture(t), sessionId = 'multica-web-reconnect-limit';
  f.seed(sessionId);
  const rpc = await connectWeb(f.config), journal = new SessionJournal(rpc, f.config, sessionId, f.cwd);
  t.after(() => journal.close());
  await journal.refresh();
  f.behavior.snapshot = snapshot => { snapshot.cursor = 1; };
  [...f.followers][0].socket.terminate();
  await assert.rejects(journal.readNew(100), { message: 'web_stream_reconnect_failed' });
});

test('probe checks the event API without creating a workspace, session or prompt', async t => {
  const f = await fixture(t), rpc = await connectWeb(f.config);
  await probeWeb(f.config, rpc);
  assert.deepEqual(f.calls.map(call => call.method), ['session/follow']);
  assert.equal(f.sessions.size, 0);
  f.behavior.unsupported = true;
  await assert.rejects(probeWeb(f.config, rpc), { message: 'dsh_event_api_unsupported' });
});

test('configuration needs no DSH data path and never rewrites an existing file', async t => {
  const f = await fixture(t), file = path.join(f.root, 'config.json');
  const original = JSON.stringify({ origin: f.config.origin, accessLog: f.config.accessLog, dataHome: 'old-unused-value' });
  fs.writeFileSync(file, original);
  assert.equal(loadConfig({ MULTICA_DSH_WEB_CONFIG: file }).origin, f.config.origin);
  assert.equal(fs.readFileSync(file, 'utf8'), original);
  fs.writeFileSync(file, JSON.stringify({ origin: f.config.origin, accessLog: f.config.accessLog }));
  assert.equal(loadConfig({ MULTICA_DSH_WEB_CONFIG: file }).accessLog, f.config.accessLog);
});

test('model and MCP overrides remain explicit errors', async t => {
  const f = await fixture(t);
  assert.equal((await run(f, command(f, 'override-model', { model: { provider: 'deepseek', id: 'x' } }))).at(-1).error.code, 'use_dsh_web_model_settings');
  assert.equal((await run(f, command(f, 'override-mcp', { mcp_servers: [{ name: 'tools' }] }))).at(-1).error.code, 'use_dsh_web_mcp_settings');
  assert.equal(f.calls.length, 0);
});

test('authentication is loopback-only and never returns raw host errors', async t => {
  const f = await fixture(t);
  for (const url of ['https://127.0.0.1/', 'http://example.com/', 'http://127.0.0.1/?token=secret']) assert.throws(() => localOrigin(url));
  const rpc = await connectWeb(f.config, async (_url, options) => {
    if (options.method !== 'POST') return { status: 303, headers: { getSetCookie: () => ['dsh=private; HttpOnly'] } };
    const rpcId = JSON.parse(options.body).rpcId;
    return { ok: true, json: async () => ({ type: 'server-response', rpcId, result: { ok: false, error: { message: 'secret-token', code: 'bad code secret-token' } } }) };
  });
  await assert.rejects(rpc('workspace/create', {}), { message: 'web_rpc_refused' });
});

test('Windows UTF-16 launch logs still authenticate the WebSocket carrier', async t => {
  const f = await fixture(t);
  fs.writeFileSync(f.config.accessLog, Buffer.concat([Buffer.from([0xef, 0xbb, 0xbf]), Buffer.from(`${f.config.origin}/?token=test-secret\n`, 'utf16le')]));
  await probeWeb(f.config, await connectWeb(f.config));
});

test('task CLI keeps credentials out of prompts and removes private files', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'multica-task-cli-test-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const cli = taskCli({ MULTICA_TASK_ID: 'task-test', MULTICA_TASK_CONFIG_ROOT: root, MULTICA_TOKEN: 'mat_test-secret', OWNER_TOKEN: 'owner-secret' });
  assert.ok(!cli.instructions.includes('mat_test-secret'));
  const folder = path.join(root, fs.readdirSync(root)[0]);
  const env = JSON.parse(fs.readFileSync(path.join(folder, 'env.json'), 'utf8'));
  assert.equal(env.MULTICA_TOKEN, 'mat_test-secret');
  assert.equal(env.OWNER_TOKEN, undefined);
  cli.cleanup();
  assert.deepEqual(fs.readdirSync(root), []);
});

const promptTexts = f => prompts(f).map(call => call.args.content[0].text);
const cacheFiles = f => fs.readdirSync(path.join(f.root, 'briefs')).filter(name => name.endsWith('.json')).map(name => path.join(f.root, 'briefs', name));

async function childRun(f, request) {
  const script = `import fs from 'node:fs'; import {execute} from ${JSON.stringify(new URL('./bridge.mjs', import.meta.url).href)};
    const input=JSON.parse(fs.readFileSync(0,'utf8'));
    await execute(input.request,input.config,frame=>process.stdout.write(JSON.stringify(frame)+'\\n'),{cancelled:false},input.dependencies);`;
  const child = spawn(process.execPath, ['--input-type=module', '--eval', script], { stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
  let out = '', error = '';
  child.stdout.on('data', bytes => { out += bytes; });
  child.stderr.on('data', bytes => { error += bytes; });
  const completed = new Promise((resolve, reject) => {
    child.on('error', reject);
    child.on('close', code => code === 0 ? resolve() : reject(new Error(`child exit ${code}: ${error}`)));
  });
  child.stdin.end(JSON.stringify({ request, config: f.config, dependencies: {
    briefCacheDir: path.join(f.root, 'briefs'), env: { MULTICA_TASK_ID: request.request_id,
      MULTICA_TOKEN: 'mat_test-secret', MULTICA_TASK_CONFIG_ROOT: f.root },
  } }));
  await completed;
  return out.trim().split('\n').map(line => JSON.parse(line));
}

test('brief digests survive processes and use API witnesses without storing instructions or credentials', async t => {
  const f = await fixture(t), system_prompt = 'Stable runtime instructions';
  const first = await childRun(f, command(f, 'brief-first', { system_prompt }));
  assert.equal(first.at(-1).status, 'completed');
  const sessionId = first.at(-1).session_id;
  const initial = JSON.parse(fs.readFileSync(cacheFiles(f)[0], 'utf8'));
  assert.equal(initial.hash, createHash('sha256').update(system_prompt).digest('hex'));
  assert.equal(initial.v, 2);
  assert.equal(initial.requestId, 'brief-first');
  assert.equal(initial.seq, f.sessions.get(sessionId).rows.find(row => row.type === 'user/message').seq);
  const second = await childRun(f, command(f, 'brief-second', { system_prompt, resume_session_id: sessionId, prompt: 'Second request' }));
  assert.equal(second.at(-1).status, 'completed');
  const texts = promptTexts(f);
  assert.ok(texts[0].includes(system_prompt));
  assert.ok(!texts[1].includes(system_prompt));
  assert.ok(texts[1].endsWith('Second request'));
  assert.notEqual(texts[0].split('\n')[0], texts[1].split('\n')[0]);
  assert.ok(!JSON.stringify(initial).includes(system_prompt));
  assert.ok(!JSON.stringify(initial).includes('mat_test-secret'));
});

test('changed briefs supersede previous instructions and fresh sessions always receive them', async t => {
  const f = await fixture(t);
  const first = await run(f, command(f, 'brief-old', { system_prompt: 'OLD INSTRUCTIONS' }));
  const resume_session_id = first.at(-1).session_id;
  await run(f, command(f, 'brief-new', { resume_session_id, system_prompt: 'NEW INSTRUCTIONS' }));
  assert.match(promptTexts(f)[1], /^以下运行说明取代本会话中此前的版本，按本版执行。\nNEW INSTRUCTIONS\n\n/);
  assert.equal(JSON.parse(fs.readFileSync(cacheFiles(f)[0])).hash, createHash('sha256').update('NEW INSTRUCTIONS').digest('hex'));
  await run(f, command(f, 'brief-same', { resume_session_id, system_prompt: 'NEW INSTRUCTIONS' }));
  assert.equal(promptTexts(f)[2], 'Do the requested work');
  await run(f, command(f, 'brief-fresh', { system_prompt: 'NEW INSTRUCTIONS' }));
  assert.ok(promptTexts(f)[3].startsWith('NEW INSTRUCTIONS\n\n'));
});

test('missing, corrupt and unwitnessed brief records resend conservatively', async t => {
  const f = await fixture(t), system_prompt = 'Recovery instructions';
  const first = await run(f, command(f, 'recover-first', { system_prompt }));
  const resume_session_id = first.at(-1).session_id;
  const mutations = [
    file => fs.unlinkSync(file),
    file => fs.writeFileSync(file, '{'),
    file => { const record = JSON.parse(fs.readFileSync(file)); record.seq = 9999; fs.writeFileSync(file, JSON.stringify(record)); },
    file => { const record = JSON.parse(fs.readFileSync(file)); record.requestId = 'missing-request'; fs.writeFileSync(file, JSON.stringify(record)); },
  ];
  for (const [i, mutate] of mutations.entries()) {
    mutate(cacheFiles(f)[0]);
    const result = await run(f, command(f, `recover-${i}`, { resume_session_id, system_prompt }));
    assert.equal(result.at(-1).status, 'completed');
    assert.ok(promptTexts(f).at(-1).includes(system_prompt));
  }
});

test('an uncertain update invalidates the previous brief before delivery', async t => {
  const f = await fixture(t);
  const first = await run(f, command(f, 'cache-old', { system_prompt: 'OLD' }));
  const resume_session_id = first.at(-1).session_id, file = cacheFiles(f)[0];
  f.behavior.disconnectPrompt = true;
  const result = await run(f, command(f, 'cache-uncertain', { resume_session_id, system_prompt: 'NEW' }));
  assert.equal(result.at(-1).error.code, 'web_rpc_outcome_unknown');
  assert.equal(fs.existsSync(file), false);
});

test('a receipt without a durable request witness never caches the brief or cancels an unrelated turn', async t => {
  const f = await fixture(t, { noWitness: true }), control = { cancelled: false };
  const pending = run(f, command(f, 'cache-no-witness', { system_prompt: 'MUST BE WITNESSED' }), control);
  await waitUntil(() => prompts(f).length > 0);
  control.cancelled = true;
  const frames = await pending;
  assert.equal(frames.at(-1).error.code, 'cancel_target_not_active');
  assert.equal(fs.existsSync(path.join(f.root, 'briefs')), false);
  assert.equal(f.calls.filter(call => call.method === 'session/cancel').length, 0);
});
