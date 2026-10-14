import assert from 'node:assert/strict';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { zstdCompressSync } from 'node:zlib';
import { execute, localOrigin, readRows, createLogReader, prepareSession, taskCli, connectWeb, TurnReader } from './bridge.mjs';

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
        if (behavior.noLog) return;
        append(session, [
          { type: 'turn/start', data: { turn } },
          { type: 'user/message', data: { role: 'user', content: args.content, source: { kind: 'user', rpcId: args.requestId } } },
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
  await execute(request, f.config, frame => frames.push(frame), control, { env: {}, briefCacheDir: path.join(f.root, 'briefs') });
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

function logFixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'multica-incremental-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const file = path.join(root, 'log.zstd');
  fs.writeFileSync(file, Buffer.alloc(0));
  return { file, append: bytes => fs.appendFileSync(file, zstdCompressSync(Buffer.from(bytes))) };
}

test('incremental reader parses only appended frames and rows, with exact bigint identity', t => {
  const { file, append } = logFixture(t);
  append('{"cwd":"project"}\n');
  const reader = createLogReader(file);
  assert.deepEqual(reader.readNew(), [{ cwd: 'project' }]);
  const first = { ...reader.stats };
  assert.deepEqual(reader.readNew(), []);
  assert.deepEqual(reader.stats, first);
  assert.equal(reader.identity.ino, fs.statSync(file, { bigint: true }).ino);
  assert.equal(typeof reader.identity.dev, 'bigint');
  append('{"type":"turn/start","data":{"turn":7}}\n');
  assert.equal(reader.readNew().length, 1);
  assert.equal(reader.stats.frames, 2);
  assert.equal(reader.stats.rows, 2);
  assert.equal(reader.stats.bytesRead, fs.statSync(file).size);
  assert.equal(reader.activeTurn, 7);
  assert.deepEqual(reader.readNew(), []);
  assert.equal(reader.activeTurn, 7);
  append('{"type":"turn/end","data":{"turn":7}}\n');
  reader.readNew();
  assert.equal(reader.activeTurn, null);
  assert.equal(readRows(file).length, 3);
  assert.equal(readRows(file).length, 3);
});

test('incomplete frame stays at its starting offset until completed', t => {
  const { file, append } = logFixture(t);
  append('{"first":1}\n');
  const reader = createLogReader(file);
  reader.readNew();
  const offset = reader.frameOffset;
  const next = zstdCompressSync(Buffer.from('{"second":2}\n'));
  for (const part of [next.subarray(0, 4), next.subarray(4, next.length - 1)]) {
    fs.appendFileSync(file, part);
    assert.deepEqual(reader.readNew(), []);
    assert.equal(reader.frameOffset, offset);
  }
  const before = { ...reader.stats };
  assert.deepEqual(reader.readNew(), []);
  assert.deepEqual(reader.stats, before);
  fs.appendFileSync(file, next.subarray(-1));
  assert.deepEqual(reader.readNew(), [{ second: 2 }]);
  assert.equal(reader.frameOffset, fs.statSync(file).size);
});

test('JSON and Chinese UTF-8 bytes split across frames and polls are preserved literally', t => {
  const { file, append } = logFixture(t);
  const bytes = Buffer.from('{"text":"中文测试内容"}\n');
  const cut = bytes.indexOf(Buffer.from('中')) + 1;
  append(bytes.subarray(0, cut));
  const reader = createLogReader(file);
  assert.deepEqual(reader.readNew(), []);
  assert.ok(Buffer.isBuffer(reader.pendingBytes));
  append(bytes.subarray(cut, -1));
  assert.deepEqual(reader.readNew(), []);
  append(bytes.subarray(-1));
  assert.equal(reader.readNew()[0].text, '中文测试内容');
  assert.equal(readRows(file)[0].text, '中文测试内容');
});

test('truncation, including only an incomplete frame, and same-size replacement fail', t => {
  const { file, append } = logFixture(t);
  append('{"first":1}\n');
  const original = fs.readFileSync(file);
  const reader = createLogReader(file);
  reader.readNew();
  fs.appendFileSync(file, Buffer.from([0x28, 0xb5]));
  reader.readNew();
  fs.truncateSync(file, original.length);
  assert.throws(() => reader.readNew(), /session_log_replaced/);
  const replacementReader = createLogReader(file);
  replacementReader.readNew();
  fs.renameSync(file, file + '.old');
  fs.writeFileSync(file, original);
  assert.throws(() => replacementReader.readNew(), /session_log_replaced/);
});

test('resume preparation hands its first scan to polling without decoding history twice', async t => {
  const f = await fixture(t);
  const first = await run(f, command(f, 'scan-first'));
  const sessionId = first.at(-1).session_id;
  const rpc = await connectWeb(f.config);
  const prepared = await prepareSession(command(f, 'scan-resume', { resume_session_id: sessionId }), f.config, rpc);
  assert.equal(prepared.logReader.stats.bytesRead, fs.statSync(prepared.file).size);
  assert.equal(prepared.logReader.stats.rows, prepared.rows.length);
  const stats = { ...prepared.logReader.stats };
  assert.deepEqual(prepared.logReader.readNew(), []);
  assert.deepEqual(prepared.logReader.stats, stats);
});

test('default incremental polling remembers another active turn across empty polls before cancellation', async t => {
  const f = await fixture(t, { hold: true }), control = { cancelled: false };
  const pending = run(f, command(f, 'incremental-cancel'), control);
  let timer;
  const watch = setInterval(() => {
    const session = [...f.sessions.values()][0];
    if (!session || !readRows(session.file).some(row => row.type === 'user/message')) return;
    clearInterval(watch);
    f.append(session, [{ type: 'turn/start', data: { turn: 99 } }]);
    timer = setTimeout(() => { control.cancelled = true; }, 60);
  }, 5);
  t.after(() => { clearInterval(watch); clearTimeout(timer); });
  const frames = await pending;
  assert.equal(frames.at(-1).error.code, 'cancel_target_not_active');
  assert.equal(f.calls.filter(call => call.method === 'session/cancel').length, 0);
});

const promptTexts = f => f.calls.filter(call => call.method === 'session/prompt').map(call => call.args.content[0].text);
const cacheFile = f => path.join(f.root, 'briefs', fs.readdirSync(path.join(f.root, 'briefs')).find(name => name.endsWith('.json')));

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

test('brief digest survives bridge processes and ignores changing CLI paths and user prompts', async t => {
  const f = await fixture(t), system_prompt = 'Stable runtime instructions';
  const first = await childRun(f, command(f, 'brief-first', { system_prompt }));
  const sessionId = first.at(-1).session_id;
  assert.equal(first.at(-1).status, 'completed');
  const initial = JSON.parse(fs.readFileSync(cacheFile(f), 'utf8'));
  assert.equal(initial.hash, createHash('sha256').update(system_prompt).digest('hex'));
  assert.equal(initial.requestId, 'brief-first');
  assert.equal(initial.ino, String(fs.statSync(f.sessions.get(sessionId).file, { bigint: true }).ino));
  const second = await childRun(f, command(f, 'brief-second', { system_prompt, resume_session_id: sessionId, prompt: 'Second request' }));
  assert.equal(second.at(-1).status, 'completed');
  const texts = promptTexts(f);
  assert.ok(texts[0].includes(system_prompt));
  assert.ok(!texts[1].includes(system_prompt));
  assert.ok(texts[1].endsWith('Second request'));
  assert.notEqual(texts[0].split('\n')[0], texts[1].split('\n')[0]);
  assert.match(texts[1], /replace the executable multica with node/);
  assert.ok(!JSON.stringify(initial).includes(system_prompt));
  assert.ok(!JSON.stringify(initial).includes('mat_test-secret'));
});

test('changed brief supersedes earlier instructions; declaration never changes the digest', async t => {
  const f = await fixture(t);
  const first = await run(f, command(f, 'brief-old', { system_prompt: 'OLD INSTRUCTIONS' }));
  const resume_session_id = first.at(-1).session_id;
  await run(f, command(f, 'brief-new', { resume_session_id, system_prompt: 'NEW INSTRUCTIONS' }));
  assert.match(promptTexts(f)[1], /^以下运行说明取代本会话中此前的版本，按本版执行。\nNEW INSTRUCTIONS\n\n/);
  assert.equal(JSON.parse(fs.readFileSync(cacheFile(f))).hash, createHash('sha256').update('NEW INSTRUCTIONS').digest('hex'));
  await run(f, command(f, 'brief-same', { resume_session_id, system_prompt: 'NEW INSTRUCTIONS' }));
  assert.equal(promptTexts(f)[2], 'Do the requested work');
  await run(f, command(f, 'brief-fresh', { system_prompt: 'NEW INSTRUCTIONS' }));
  assert.ok(promptTexts(f)[3].startsWith('NEW INSTRUCTIONS\n\n'));
});

test('missing, corrupt, wrong-identity and unwitnessed brief caches conservatively resend', async t => {
  const f = await fixture(t), system_prompt = 'Recovery instructions';
  const first = await run(f, command(f, 'recover-first', { system_prompt }));
  const resume_session_id = first.at(-1).session_id;
  const mutations = [
    file => fs.unlinkSync(file),
    file => fs.writeFileSync(file, '{'),
    file => { const record = JSON.parse(fs.readFileSync(file)); record.ino = String(BigInt(record.ino) + 1n); fs.writeFileSync(file, JSON.stringify(record)); },
    file => { const record = JSON.parse(fs.readFileSync(file)); record.requestId = 'missing-request'; fs.writeFileSync(file, JSON.stringify(record)); },
  ];
  for (const [i, mutate] of mutations.entries()) {
    mutate(cacheFile(f));
    const result = await run(f, command(f, `recover-${i}`, { resume_session_id, system_prompt }));
    assert.equal(result.at(-1).status, 'completed');
    assert.ok(promptTexts(f).at(-1).includes(system_prompt));
  }
});

test('uncertain updates invalidate old hashes before delivery', async t => {
  const behavior = {}, f = await fixture(t, behavior);
  const first = await run(f, command(f, 'cache-old', { system_prompt: 'OLD' }));
  const resume_session_id = first.at(-1).session_id, file = cacheFile(f);
  behavior.disconnect = true;
  const result = await run(f, command(f, 'cache-uncertain', { resume_session_id, system_prompt: 'NEW' }));
  assert.equal(result.at(-1).error.code, 'web_rpc_outcome_unknown');
  assert.equal(fs.existsSync(file), false);
  behavior.disconnect = false;
  await run(f, command(f, 'cache-back', { resume_session_id, system_prompt: 'OLD' }));
  assert.ok(promptTexts(f).at(-1).includes('OLD'));
});

test('an accepted receipt without a log witness never records the brief', async t => {
  const f = await fixture(t, { noLog: true }), control = { cancelled: false }, frames = [];
  const rpc = await connectWeb(f.config);
  await execute(command(f, 'cache-no-witness', { system_prompt: 'MUST BE WITNESSED' }), f.config,
    frame => frames.push(frame), control, {
      env: {}, briefCacheDir: path.join(f.root, 'briefs'),
      connectWeb: async () => async (method, request) => {
        const receipt = await rpc(method, request);
        if (method === 'session/prompt') control.cancelled = true;
        return receipt;
      },
    });
  assert.equal(frames.at(-1).status, 'cancelled');
  assert.equal(promptTexts(f).length, 1);
  assert.equal(fs.existsSync(path.join(f.root, 'briefs')), false);
});
