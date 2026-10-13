import assert from 'node:assert/strict';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import { zstdCompressSync } from 'node:zlib';
import { execute, localOrigin, readRows, taskCli, connectWeb, TurnReader } from './bridge.mjs';

async function fixture(t, behavior = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'multica-dsh-test-'));
  const cwd = path.join(root, 'project'), dataHome = path.join(root, 'data');
  fs.mkdirSync(cwd);
  fs.mkdirSync(path.join(dataHome, 'sessions'), { recursive: true });
  const sessions = new Map(), calls = [], timers = [];
  const append = (session, rows) => fs.appendFileSync(session.file, zstdCompressSync(Buffer.from(rows.map(row => JSON.stringify(row) + '\n').join(''))));
  const server = http.createServer(async (request, response) => {
    if (request.url === '/?token=test-secret') { response.writeHead(303, { 'Set-Cookie': 'dsh=test-cookie; HttpOnly', Location: '/' }); response.end(); return; }
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
      let session = sessions.get(args.sessionId);
      if (!session) {
        const directory = path.join(dataHome, 'sessions', 'group-test', args.sessionId);
        fs.mkdirSync(directory, { recursive: true });
        session = { file: path.join(directory, 'session.v3.jsonl.zstd'), turn: 0 };
        sessions.set(args.sessionId, session);
        if (behavior.headerDelay) timers.push(setTimeout(() => append(session, [{ cwd }]), behavior.headerDelay));
        else append(session, [{ cwd }]);
      }
      value = { sessionId: args.sessionId };
    } else if (method === 'session/rename') value = { title: args.title };
    else if (method === 'session/cancel') value = { accepted: true };
    else if (method === 'session/prompt') {
      if (behavior.disconnect) { request.socket.destroy(); return; }
      const session = sessions.get(args.sessionId), turn = ++session.turn;
      assert.equal(args.mode, 'queue');
      timers.push(setTimeout(() => {
        append(session, [
          { type: 'turn/start', data: { turn } },
          { type: 'user/message', data: { role: 'user', content: [{ type: 'text', text: 'Do the requested work' }], source: { kind: 'user', rpcId: args.requestId } } },
          { type: 'tool/call', data: { turn, callId: 'call-test', name: 'bash', arguments: '{"command":"pwd"}' } },
          { type: 'tool/result', data: { turn, callId: 'call-test', name: 'bash', message: { content: [{ type: 'tool-result', toolCallId: 'call-test', content: [{ type: 'text', text: cwd }] }] } } },
          ...behavior.hold ? [] : [
            { type: 'assistant/message', data: { turn, message: { content: [{ type: 'text', text: `reply-${turn}` }], source: { provider: 'test', model: 'fake' } }, usage: { inputTokens: 20, outputTokens: 2 } } },
            { type: 'turn/end', data: { turn, reason: { kind: 'completed' } } },
          ],
        ]);
      }, 20));
      value = { accepted: true };
    } else throw new Error(`Unexpected method ${method}`);
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({ type: 'server-response', rpcId, result: { ok: true, value } }));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const accessLog = path.join(root, 'launch.log');
  fs.writeFileSync(accessLog, `${origin}/?token=test-secret\n`);
  t.after(async () => {
    timers.forEach(clearTimeout);
    await new Promise(resolve => server.close(resolve));
    fs.rmSync(root, { recursive: true, force: true });
  });
  return { cwd, root, sessions, calls, append, config: { origin, accessLog, dataHome, pollMs: 5 } };
}

const command = (f, id, extra = {}) => ({ v: 1, type: 'execute', request_id: id, cwd: f.cwd, prompt: 'Do the requested work', mcp_servers: [], ...extra });
async function run(f, request, control = { cancelled: false }) {
  const frames = [];
  await execute(request, f.config, frame => frames.push(frame), control, { env: {} });
  return frames;
}

test('browser steering in the active turn keeps the revised reply attached to the Multica request', () => {
  const frames = [], reader = new TurnReader('multica-request', frame => frames.push(frame));
  for (const row of [
    { type: 'turn/start', data: { turn: 1 } },
    { type: 'user/message', data: { source: { kind: 'user', rpcId: 'multica-request' } } },
    { type: 'assistant/message', data: { turn: 1, message: { content: [{ type: 'text', text: 'Initial answer' }] } } },
    { type: 'user/message', data: { source: { kind: 'user', rpcId: 'browser-steering' }, content: [{ type: 'text', text: 'Revise the answer' }] } },
    { type: 'assistant/message', data: { turn: 1, message: { content: [{ type: 'text', text: 'Revised answer' }] } } },
    { type: 'turn/end', data: { turn: 1, reason: { kind: 'completed' } } },
  ]) reader.push(row);
  assert.equal(reader.terminal.output, 'Revised answer');
  assert.equal(reader.terminal.status, 'completed');
  assert.deepEqual(frames.map(frame => frame.content), ['Initial answer', 'Revised answer']);
});

test('a queued browser follow-up is not reported as the completed Multica turn', () => {
  const frames = [], reader = new TurnReader('multica-request', frame => frames.push(frame));
  for (const row of [
    { type: 'turn/start', data: { turn: 1 } },
    { type: 'user/message', data: { source: { kind: 'user', rpcId: 'multica-request' } } },
    { type: 'assistant/message', data: { turn: 1, message: { content: [{ type: 'text', text: 'Multica result' }] } } },
    { type: 'turn/end', data: { turn: 1, reason: { kind: 'completed' } } },
    { type: 'turn/start', data: { turn: 2 } },
    { type: 'user/message', data: { source: { kind: 'user', rpcId: 'browser-followup' } } },
    { type: 'assistant/message', data: { turn: 2, message: { content: [{ type: 'text', text: 'Browser-only result' }] } } },
    { type: 'turn/end', data: { turn: 2, reason: { kind: 'completed' } } },
  ]) reader.push(row);
  assert.equal(reader.terminal.output, 'Multica result');
  assert.deepEqual(frames.map(frame => frame.content), ['Multica result']);
});

test('new session waits for the browser host to flush its project header', async t => {
  const f = await fixture(t, { headerDelay: 80 });
  const frames = await run(f, command(f, 'task-header-flush'));
  assert.equal(frames.at(-1).status, 'completed');
  assert.equal(frames.at(-1).output, 'reply-1');
});

test('message runs in the web host; follow-up retains the same session and ignores the previous reply', async t => {
  const f = await fixture(t);
  const first = await run(f, command(f, 'task-first', { thread_name: 'TEST-1' }));
  const session = first.find(frame => frame.type === 'session').session_id;
  assert.equal(first.at(-1).status, 'completed');
  assert.equal(first.at(-1).output, 'reply-1');
  assert.ok(first.some(frame => frame.type === 'tool_call' && frame.name === 'bash'));
  assert.equal(first.find(frame => frame.type === 'usage').input_tokens, 20);
  assert.equal(f.calls.find(call => call.method === 'session/rename').args.title, 'Multica · TEST-1');
  const second = await run(f, command(f, 'task-second', { resume_session_id: session }));
  assert.equal(second.find(frame => frame.type === 'session').session_id, session);
  assert.equal(second.at(-1).output, 'reply-2');
  assert.deepEqual(second.filter(frame => frame.type === 'text').map(frame => frame.content), ['reply-2']);
  assert.equal(f.sessions.size, 1);
});

test('a retry recovers the admitted request without posting a second prompt', async t => {
  const f = await fixture(t);
  await run(f, command(f, 'task-repeat'));
  const retry = await run(f, command(f, 'task-repeat'));
  assert.equal(retry.at(-1).output, 'reply-1');
  assert.equal(f.calls.filter(call => call.method === 'session/prompt').length, 1);
});

test('cancel reaches the web-owned agent', async t => {
  const f = await fixture(t, { hold: true }), control = { cancelled: false };
  const pending = run(f, command(f, 'task-cancel'), control);
  const timer = setInterval(() => { if (f.calls.some(call => call.method === 'session/prompt')) { control.cancelled = true; clearInterval(timer); } }, 5);
  t.after(() => clearInterval(timer));
  const frames = await pending;
  assert.equal(frames.at(-1).status, 'cancelled');
  assert.equal(f.calls.filter(call => call.method === 'session/cancel').length, 1);
});

test('uncertain delivery fails visibly and never resends', async t => {
  const f = await fixture(t, { disconnect: true });
  const frames = await run(f, command(f, 'task-unknown'));
  assert.equal(frames.at(-1).error.code, 'web_rpc_outcome_unknown');
  assert.match(frames.at(-1).error.message, /Open DSH to inspect/);
  assert.equal(f.calls.filter(call => call.method === 'session/prompt').length, 1);
  assert.ok(!JSON.stringify(frames).includes('test-secret'));
});

test('cancel after completion does not stop the next browser turn', async t => {
  const f = await fixture(t), frames = [], control = { cancelled: false };
  await execute(command(f, 'task-cancel-race'), f.config, frame => frames.push(frame), control, {
    env: {},
    readRows(file) {
      const rows = readRows(file);
      if (rows.some(row => row.type === 'turn/end')) {
        control.cancelled = true;
        rows.push({ type: 'turn/start', data: { turn: 99 } });
      }
      return rows;
    },
  });
  assert.equal(frames.at(-1).status, 'completed');
  assert.equal(f.calls.filter(call => call.method === 'session/cancel').length, 0);
});

test('refuses a session from another project and an already running turn', async t => {
  const f = await fixture(t);
  const first = await run(f, command(f, 'task-owner'));
  const sessionId = first.at(-1).session_id, session = f.sessions.get(sessionId);
  const other = path.join(f.root, 'other'); fs.mkdirSync(other);
  const wrong = await run(f, command(f, 'task-wrong', { cwd: other, resume_session_id: sessionId }));
  assert.equal(wrong.at(-1).error.code, 'session_project_mismatch');
  f.append(session, [{ type: 'turn/start', data: { turn: 99 } }]);
  const busy = await run(f, command(f, 'task-busy', { resume_session_id: sessionId }));
  assert.equal(busy.at(-1).error.code, 'session_busy');
  assert.equal(f.calls.filter(call => call.method === 'session/prompt').length, 1);
});

test('explicit model and MCP overrides fail instead of being silently ignored', async t => {
  const f = await fixture(t);
  assert.equal((await run(f, command(f, 'task-model', { model: { provider: 'deepseek', id: 'x' } }))).at(-1).error.code, 'use_dsh_web_model_settings');
  assert.equal((await run(f, command(f, 'task-mcp', { mcp_servers: [{ name: 'tools' }] }))).at(-1).error.code, 'use_dsh_web_mcp_settings');
  assert.equal(f.calls.length, 0);
});

test('authentication only exchanges a local, matching launch link and hides raw host errors', async t => {
  const f = await fixture(t);
  for (const url of ['https://127.0.0.1/', 'http://example.com/', 'http://127.0.0.1/?token=secret']) assert.throws(() => localOrigin(url));
  const rpc = await connectWeb(f.config, async (_url, options) => {
    if (options.method !== 'POST') return { status: 303, headers: { getSetCookie: () => ['dsh=private; HttpOnly'] } };
    const rpcId = JSON.parse(options.body).rpcId;
    return { ok: true, json: async () => ({ type: 'server-response', rpcId, result: { ok: false, error: { message: 'secret-token', code: 'bad code secret-token' } } }) };
  });
  await assert.rejects(rpc('workspace/create', {}), { message: 'web_rpc_refused' });
});

test('accepts the UTF-16 launch log produced by Windows redirection', async t => {
  const f = await fixture(t);
  fs.writeFileSync(f.config.accessLog, Buffer.concat([Buffer.from([0xef, 0xbb, 0xbf]), Buffer.from(`${f.config.origin}/?token=test-secret\n`, 'utf16le')]));
  const rpc = await connectWeb(f.config);
  assert.equal((await rpc('workspace/create', { path: f.cwd })).workspace.workspaceId, 'ws-test');
});

test('task CLI keeps credentials out of the prompt and removes its private files', t => {
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

test('log reader ignores a partially written final zstd frame', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'multica-log-test-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const file = path.join(root, 'log.zstd');
  const first = zstdCompressSync(Buffer.from('{"cwd":"project"}\n'));
  const second = zstdCompressSync(Buffer.from('{"type":"turn/start"}\n'));
  fs.writeFileSync(file, Buffer.concat([first, second.subarray(0, second.length - 2)]));
  assert.deepEqual(readRows(file), [{ cwd: 'project' }]);
});
