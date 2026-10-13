// Multica's stdio protocol carried by the existing, browser-owned DSH host.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { createHash, randomUUID } from 'node:crypto';
import { createInterface } from 'node:readline';
import { pathToFileURL } from 'node:url';
import { zstdDecompressSync } from 'node:zlib';

const safeId = /^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$/;
const fail = code => { throw new Error(code); };
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const textOf = message => (message?.content ?? []).filter(p => p.type === 'text').map(p => p.text ?? '').join('');
const reasonOf = error => /^[a-z0-9_/-]+$/i.test(error?.message ?? '') ? error.message : 'web_bridge_failed';

export function localOrigin(value) {
  let url;
  try { url = new URL(value); } catch { fail('invalid_web_origin'); }
  if (url.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)
    || url.username || url.password || url.pathname !== '/' || url.search || url.hash) fail('local_web_origin_required');
  return url.origin;
}

export function loadConfig(env = process.env) {
  const file = env.MULTICA_DSH_WEB_CONFIG ?? path.join(os.homedir(), '.multica', 'dsh-web.json');
  let config;
  try { config = JSON.parse(fs.readFileSync(file, 'utf8').replace(/^\uFEFF/, '')); }
  catch { fail('web_bridge_config_unavailable'); }
  config.origin = localOrigin(config.origin);
  for (const key of ['accessLog', 'dataHome']) {
    if (typeof config[key] !== 'string' || !path.isAbsolute(config[key])) fail('absolute_config_paths_required');
  }
  return config;
}

// Exchange the normal launch link. Neither credentials nor raw host errors leave this module.
export async function connectWeb(config, fetchImpl = fetch) {
  const origin = localOrigin(config.origin);
  if (fs.statSync(config.accessLog).size > 4 * 1024 * 1024) fail('access_log_too_large');
  let bytes = fs.readFileSync(config.accessLog);
  if (bytes.subarray(0, 3).equals(Buffer.from([0xef, 0xbb, 0xbf]))) bytes = bytes.subarray(3);
  const log = bytes[0] === 0xff && bytes[1] === 0xfe ? bytes.subarray(2).toString('utf16le')
    : bytes.includes(0) ? bytes.toString('utf16le') : bytes.toString('utf8');
  const links = log.match(/http:\/\/[^\s<>"']+/g) ?? [];
  const access = links.reverse().map(link => { try { return new URL(link); } catch { return null; } })
    .find(url => url?.origin === origin && url.pathname === '/' && url.searchParams.getAll('token').length === 1
      && url.searchParams.get('token'));
  if (!access) fail('web_access_link_missing');
  let response;
  try { response = await fetchImpl(access.href, { redirect: 'manual', signal: AbortSignal.timeout(10000) }); }
  catch { fail('web_auth_transport_failed'); }
  if (response.status !== 303) fail('web_auth_failed');
  const cookies = response.headers.getSetCookie().map(value => value.split(';')[0]);
  if (cookies.length !== 1) fail('web_auth_cookie_missing');
  return async (method, request = {}) => {
    const rpcId = randomUUID();
    let result;
    try {
      const r = await fetchImpl(`${origin}/api/${method}`, {
        method: 'POST', redirect: 'error', signal: AbortSignal.timeout(method === 'session/cancel' ? 2000 : 10000),
        headers: { 'Content-Type': 'application/json', Origin: origin, Cookie: cookies[0] },
        body: JSON.stringify({ type: 'client-request', rpcId, method, payload: { args: { request } } }),
      });
      if (!r.ok) fail(`web_rpc_http_${r.status}`);
      result = await r.json();
    } catch (error) {
      if (/^web_rpc_http_\d+$/.test(error.message)) throw error;
      fail('web_rpc_outcome_unknown');
    }
    if (result?.type !== 'server-response' || result.rpcId !== rpcId) fail('web_rpc_receipt_invalid');
    if (result.result?.ok !== true) {
      const code = result.result?.error?.code;
      fail(typeof code === 'string' && /^[a-z0-9_/-]+$/i.test(code) ? code : 'web_rpc_refused');
    }
    return result.result.value;
  };
}

function canonicalDirectory(value) {
  if (!value || !path.isAbsolute(value)) fail('absolute_project_directory_required');
  const directory = fs.realpathSync.native(value);
  if (!fs.statSync(directory).isDirectory()) fail('project_directory_required');
  return os.platform() === 'win32' ? directory.toLowerCase() : directory;
}

export function findLog(dataHome, sessionId) {
  if (!safeId.test(sessionId)) fail('invalid_session_id');
  const root = path.join(dataHome, 'sessions');
  for (const group of fs.readdirSync(root)) {
    const file = path.join(root, group, sessionId, 'session.v3.jsonl.zstd');
    if (fs.existsSync(file)) return file;
  }
  return null;
}

// A log is concatenated zstd frames. Leave an incomplete flush for the next poll.
export function readRows(file) {
  const bytes = fs.readFileSync(file);
  let offset = 0, text = '';
  while (offset + 5 <= bytes.length) {
    const start = offset;
    if (bytes.readUInt32LE(offset) !== 0xfd2fb528) fail('unsupported_session_log');
    offset += 4;
    const header = bytes[offset++], single = Boolean(header & 32), size = header >>> 6, dict = header & 3;
    offset += (single ? 0 : 1) + (dict === 3 ? 4 : dict) + (size === 0 ? (single ? 1 : 0) : (1 << size));
    let complete = false;
    while (offset + 3 <= bytes.length) {
      const block = bytes.readUIntLE(offset, 3);
      offset += 3;
      offset += ((block >>> 1) & 3) === 1 ? 1 : block >>> 3;
      if (block & 1) { complete = true; break; }
    }
    if (header & 4) offset += 4;
    if (!complete || offset > bytes.length) break;
    text += zstdDecompressSync(bytes.subarray(start, offset)).toString('utf8');
  }
  return text.split('\n').slice(0, -1).filter(line => line.trim()).map(line => JSON.parse(line));
}

function openTurn(rows) {
  let open = null;
  for (const row of rows) {
    if (row.type === 'turn/start') open = row.data.turn;
    if (row.type === 'turn/end' && row.data.turn === open) open = null;
  }
  return open;
}

export async function prepareSession(command, config, rpc, read = readRows) {
  const cwd = canonicalDirectory(command.cwd);
  const sessionId = command.resume_session_id || `multica-web-${createHash('sha256').update(command.request_id).digest('hex').slice(0, 32)}`;
  if (!safeId.test(sessionId) || !sessionId.startsWith('multica-web-')) fail('multica_session_required');
  if (command.resume_session_id) {
    const file = findLog(config.dataHome, sessionId);
    const header = file && read(file)[0];
    if (!header?.cwd || canonicalDirectory(header.cwd) !== cwd || header.parentSession || header.origin === 'subagent') fail('session_project_mismatch');
  }
  const created = await rpc('workspace/create', { path: command.cwd });
  const workspace = created?.workspace;
  if (!workspace?.workspaceId || canonicalDirectory(workspace.path) !== cwd) fail('workspace_receipt_mismatch');
  const adopted = await rpc('session/create', { sessionId, workspaceId: workspace.workspaceId });
  if (adopted?.sessionId !== sessionId) fail('session_receipt_mismatch');
  const checked = await rpc('workspace/create', { path: command.cwd });
  if (checked?.workspace?.workspaceId !== workspace.workspaceId || !checked.workspace.sessionIds?.includes(sessionId)) fail('workspace_membership_unconfirmed');
  // Session creation is acknowledged before its initial log flush completes.
  let file, rows;
  const deadline = Date.now() + 3000;
  do {
    file = findLog(config.dataHome, sessionId);
    rows = file && read(file);
    if (rows?.[0]) break;
    await sleep(50);
  } while (Date.now() < deadline);
  if (!rows?.[0]) fail('session_header_unavailable');
  if (!rows?.[0]?.cwd || canonicalDirectory(rows[0].cwd) !== cwd) fail('session_project_mismatch');
  if (openTurn(rows) !== null) fail('session_busy');
  if (!command.resume_session_id && command.thread_name) await rpc('session/rename', { sessionId, title: `Multica · ${command.thread_name}` });
  return { sessionId, file, rows };
}

// The browser host has its own environment. Give only Multica CLI calls the task-scoped environment.
export function taskCli(env = process.env) {
  if (!env.MULTICA_TASK_ID) return { instructions: '', cleanup() {} };
  if (!env.MULTICA_TOKEN?.startsWith('mat_') || !path.isAbsolute(env.MULTICA_TASK_CONFIG_ROOT ?? '')) fail('task_cli_environment_missing');
  const directory = fs.mkdtempSync(path.join(env.MULTICA_TASK_CONFIG_ROOT, 'dsh-cli-'));
  const environment = {};
  for (const key of ['PATH', 'MULTICA_TOKEN', 'MULTICA_TASK_CONFIG_ROOT', 'MULTICA_SERVER_URL', 'MULTICA_DAEMON_PORT',
    'MULTICA_WORKSPACE_ID', 'MULTICA_AGENT_NAME', 'MULTICA_AGENT_ID', 'MULTICA_TASK_ID', 'MULTICA_TASK_SLOT',
    'MULTICA_AUTOPILOT_ID', 'MULTICA_AUTOPILOT_RUN_ID', 'MULTICA_QUICK_CREATE_TASK_ID', 'MULTICA_QUICK_CREATE_ATTACHMENT_IDS']) {
    if (env[key] !== undefined) environment[key] = env[key];
  }
  const entry = path.join(directory, 'multica.mjs');
  fs.writeFileSync(path.join(directory, 'env.json'), JSON.stringify(environment), { mode: 0o600 });
  fs.writeFileSync(entry, `import fs from 'node:fs';\nimport {spawnSync} from 'node:child_process';\nconst env=JSON.parse(fs.readFileSync(new URL('./env.json',import.meta.url),'utf8'));\nconst result=spawnSync('multica',process.argv.slice(2),{env:{...process.env,...env},stdio:'inherit',windowsHide:true});\nprocess.exit(result.status??1);\n`, { mode: 0o600 });
  return {
    instructions: `For every Multica CLI command below, replace the executable multica with node ${JSON.stringify(entry)} and keep the arguments. This entry uses this run's agent credentials. Use it only for this run; never use the owner's signed-in Multica CLI.\n`,
    cleanup() { fs.rmSync(directory, { recursive: true, force: true }); },
  };
}

export class TurnReader {
  constructor(requestId, emit) { this.requestId = requestId; this.emit = emit; this.turn = null; this.activeTurn = null; this.output = ''; this.pending = []; }
  push(row) {
    if (row.type === 'turn/start') this.activeTurn = row.data.turn;
    if (this.turn === null) {
      this.pending.push(row);
      if (row.type !== 'user/message' || row.data?.source?.rpcId !== this.requestId || this.activeTurn === null) return;
      this.turn = this.activeTurn;
      const pending = this.pending;
      this.pending = [];
      for (const event of pending) this.push(event);
      return;
    }
    const data = row.data ?? {};
    if (data.turn !== this.turn) return;
    switch (row.type) {
      case 'assistant/message': {
        const text = textOf(data.message);
        if (text) { this.output = text; this.emit({ type: 'text', content: text }); }
        for (const part of data.message?.content ?? []) {
          if (part.type === 'reasoning' && part.text) this.emit({ type: 'thinking', content: part.text });
        }
        const usage = data.usage;
        if (usage) this.emit({ type: 'usage', provider: data.message.source?.provider, model: data.message.source?.model,
          input_tokens: usage.inputTokens ?? 0, output_tokens: usage.outputTokens ?? 0,
          cache_read_tokens: usage.cacheReadTokens ?? 0, cache_write_tokens: usage.cacheWriteTokens ?? 0 });
        break;
      }
      case 'tool/call': this.output = ''; this.emit({ type: 'tool_call', call_id: data.callId, name: data.name, arguments: data.arguments }); break;
      case 'tool/result': this.emit({ type: 'tool_result', call_id: data.callId, name: data.name,
        output: (data.message?.content ?? []).filter(p => p.type === 'tool-result').map(p => textOf(p)).join('\n'),
        is_error: data.message?.content?.some(p => p.isError) }); break;
      case 'turn/end': this.terminal = { status: data.reason?.kind === 'completed' ? 'completed' : data.reason?.kind === 'user' ? 'cancelled' : 'failed', output: this.output,
        ...(data.reason?.kind === 'completed' ? {} : { error: { code: 'dsh_turn_ended', message: String(data.reason?.kind ?? 'unknown') } }) }; break;
    }
  }
}

export async function execute(command, config, emit, control, dependencies = {}) {
  const read = dependencies.readRows ?? readRows;
  let cli, prepared, rpc, delivered = false;
  try {
    if (command.v !== 1 || command.type !== 'execute' || !safeId.test(command.request_id) || !command.prompt?.trim()) fail('invalid_execute_command');
    if (command.model && (command.model.provider !== 'web' || command.model.id !== 'default') || command.reasoning_effort) fail('use_dsh_web_model_settings');
    if (command.mcp_servers?.length) fail('use_dsh_web_mcp_settings');
    rpc = await (dependencies.connectWeb ?? connectWeb)(config);
    prepared = await prepareSession(command, config, rpc, read);
    emit({ type: 'session', session_id: prepared.sessionId, resumed: Boolean(command.resume_session_id) });
    if (control.cancelled) fail('cancelled_before_delivery');
    cli = taskCli(dependencies.env ?? process.env);
    const text = `${cli.instructions}${command.system_prompt ? command.system_prompt + '\n\n' : ''}${command.prompt}`;
    const reader = new TurnReader(command.request_id, emit);
    // Include historical rows only to recover an already-admitted request, never replay their output.
    for (const row of prepared.rows) reader.push(row);
    if (reader.turn !== null) {
      if (reader.terminal) { emit({ type: 'result', session_id: prepared.sessionId, ...reader.terminal }); return; }
      fail('existing_request_still_running');
    }
    const receipt = await rpc('session/prompt', { sessionId: prepared.sessionId, requestId: command.request_id, mode: 'queue', content: [{ type: 'text', text }] });
    if (receipt?.accepted !== true) fail('prompt_receipt_invalid');
    delivered = true;
    control.cancel = async () => {
      const receipt = await rpc('session/cancel', { sessionId: prepared.sessionId });
      if (receipt?.accepted !== true) fail('cancel_receipt_invalid');
    };
    let cursor = prepared.rows.length;
    for (;;) {
      const rows = read(prepared.file);
      if (rows.length < cursor) fail('session_log_replaced');
      for (const row of rows.slice(cursor)) reader.push(row);
      cursor = rows.length;
      if (reader.terminal) { emit({ type: 'result', session_id: prepared.sessionId, ...reader.terminal }); return; }
      if (control.cancelled) {
        if (openTurn(rows) !== null && openTurn(rows) !== reader.turn) fail('cancel_target_not_active');
        await control.cancel();
        emit({ type: 'result', session_id: prepared.sessionId, status: 'cancelled', output: reader.output });
        return;
      }
      await sleep(config.pollMs ?? 500);
    }
  } catch (error) {
    emit({ type: 'result', session_id: prepared?.sessionId ?? command.resume_session_id ?? '', status: control.cancelled ? 'cancelled' : 'failed', output: '',
      error: { code: reasonOf(error), message: delivered || reasonOf(error) === 'web_rpc_outcome_unknown' ? 'The DSH web run may still be active. Open DSH to inspect it before retrying.' : reasonOf(error) } });
  } finally { cli?.cleanup(); }
}

export async function main(args = process.argv.slice(2)) {
  const emit = frame => process.stdout.write(JSON.stringify({ v: 1, ...frame }) + '\n');
  if (args.includes('--version')) { process.stdout.write('multica-dsh-web 1.0.0\n'); return; }
  const config = loadConfig();
  if (args.includes('--probe')) {
    await connectWeb(config);
    emit({ type: 'probe', runtime: 'dsh', protocol_version: 1 });
    return;
  }
  if (args.includes('--list-models')) {
    await connectWeb(config);
    emit({ type: 'models', models: [{ id: 'web/default', label: 'Use DSH web settings', provider: 'web', default: true }] });
    return;
  }
  if (!args.includes('--stdio')) fail('stdio_required');
  emit({ type: 'ready', runtime: 'dsh', protocol_version: 1 });
  const input = createInterface({ input: process.stdin });
  let active = null;
  const finished = new Promise(resolve => {
    input.on('line', line => {
      let command;
      try { command = JSON.parse(line); } catch { emit({ type: 'protocol_error', code: 'invalid_json', message: 'Expected a JSON command.' }); return; }
      if (command.type === 'cancel' && active && command.request_id === active.id) { active.control.cancelled = true; return; }
      if (command.type !== 'execute' || active) { emit({ type: 'protocol_error', code: 'invalid_command', message: 'One execute command is allowed.' }); return; }
      const control = { cancelled: false };
      active = { id: command.request_id, control };
      void execute(command, config, frame => emit({ request_id: command.request_id, ...frame }), control).finally(() => { input.close(); resolve(); });
    });
    input.on('close', () => { if (active) active.control.cancelled = true; else resolve(); });
  });
  await finished;
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  main().catch(error => { process.stderr.write(reasonOf(error) + '\n'); process.exitCode = 1; });
}
