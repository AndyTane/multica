// Multica's stdio protocol carried by the existing, browser-owned DSH host.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { createHash, randomUUID } from 'node:crypto';
import { createInterface } from 'node:readline';

const safeId = /^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$/;
const fail = code => { throw new Error(code); };
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const textOf = message => (message?.content ?? []).filter(p => p.type === 'text').map(p => p.text ?? '').join('');
const reasonOf = error => /^[a-z0-9_/-]+$/i.test(error?.message ?? '') ? error.message : 'web_bridge_failed';
const transportFailure = error => ['web_stream_disconnected', 'web_stream_timeout', 'web_stream_overflow',
  'web_rpc_outcome_unknown'].includes(reasonOf(error)) || /^web_rpc_http_(408|429|5\d\d)$/.test(reasonOf(error));

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
  for (const key of ['accessLog']) {
    if (typeof config[key] !== 'string' || !path.isAbsolute(config[key])) fail('absolute_config_paths_required');
  }
  return config;
}

// Exchange the normal launch link. Neither credentials nor raw host errors leave this module.
export async function connectWeb(config, fetchImpl = fetch, WebSocketImpl = WebSocket) {
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
  const rpc = async (method, request = {}) => {
    const rpcId = randomUUID();
    let result;
    try {
      const r = await fetchImpl(`${origin}/api/${method}`, {
        method: 'POST', redirect: 'error', signal: AbortSignal.timeout(method === 'session/cancel' ? 2000 : config.rpcTimeoutMs ?? 10000),
        headers: { 'Content-Type': 'application/json', Origin: origin, Cookie: cookies[0] },
        body: JSON.stringify({ type: 'client-request', rpcId, method, payload: { args: { request } } }),
      });
      if (!r.ok) fail(`web_rpc_http_${r.status}`);
      result = await r.json();
    } catch (error) {
      if (/^web_rpc_http_\d+$/.test(error.message)) throw error;
      const unknown = new Error('web_rpc_outcome_unknown');
      unknown.rpcMethod = method;
      throw unknown;
    }
    if (result?.type !== 'server-response' || result.rpcId !== rpcId) fail('web_rpc_receipt_invalid');
    if (result.result?.ok !== true) {
      const code = result.result?.error?.code;
      fail(typeof code === 'string' && /^[a-z0-9_/-]+$/i.test(code) ? code : 'web_rpc_refused');
    }
    return result.result.value;
  };
  rpc.follow = request => openRemoteStream(config, cookies[0], request, WebSocketImpl);
  return rpc;
}

function canonicalDirectory(value) {
  if (!value || !path.isAbsolute(value)) fail('absolute_project_directory_required');
  const directory = fs.realpathSync.native(value);
  if (!fs.statSync(directory).isDirectory()) fail('project_directory_required');
  return os.platform() === 'win32' ? directory.toLowerCase() : directory;
}

// One authenticated logical stream on the host's Remote WebSocket carrier.
// Node's WebSocket owns framing, masking, heartbeat replies, and the close handshake.
function openRemoteStream(config, cookie, request, WebSocketImpl) {
  const origin = localOrigin(config.origin), streamId = randomUUID();
  const socket = new WebSocketImpl(`${origin.replace('http:', 'ws:')}/api/remote.mux`, {
    headers: { Origin: origin, Cookie: cookie },
  });
  const queue = [];
  let queuedBytes = 0, failure, wake, closed = false;
  const notify = () => { const pending = wake; wake = undefined; pending?.(); };
  const stop = code => {
    if (!failure) failure = new Error(code);
    notify();
    socket.close();
  };
  socket.addEventListener('open', () => {
    socket.send(JSON.stringify({ type: 'open', streamId, endpoint: 'session/follow', payload: { args: { request } } }));
  });
  socket.addEventListener('error', () => stop('web_stream_disconnected'));
  socket.addEventListener('close', () => { if (!closed) stop('web_stream_disconnected'); });
  socket.addEventListener('message', event => {
    if (closed || failure) return;
    try {
      if (typeof event.data !== 'string' || Buffer.byteLength(event.data) > 32 * 1024 * 1024) fail('web_stream_frame_invalid');
      const frame = JSON.parse(event.data);
      if (frame.streamId !== streamId) fail('web_stream_frame_invalid');
      if (frame.type === 'error') {
        const code = frame.error?.code;
        stop(['gateway/not-found', 'gateway/method-not-found'].includes(code) ? 'dsh_event_api_unsupported'
          : typeof code === 'string' && /^[a-z0-9_/-]+$/i.test(code) ? code : 'web_stream_refused');
        return;
      }
      if (frame.type === 'end') { stop('web_stream_disconnected'); return; }
      if (frame.type !== 'item' || !frame.value || typeof frame.value !== 'object') fail('web_stream_frame_invalid');
      const size = Buffer.byteLength(event.data);
      if (queuedBytes + size > 32 * 1024 * 1024 || queue.length >= 16384) fail('web_stream_overflow');
      queue.push({ value: frame.value, size });
      queuedBytes += size;
      notify();
    } catch (error) { stop(reasonOf(error) === 'web_bridge_failed' ? 'web_stream_frame_invalid' : reasonOf(error)); }
  });
  return {
    async next(timeoutMs) {
      if (!queue.length && !failure && !closed && timeoutMs > 0) {
        let timer;
        try { await new Promise(resolve => { wake = resolve; timer = setTimeout(resolve, timeoutMs); }); }
        finally { clearTimeout(timer); wake = undefined; }
      }
      if (queue.length) { const item = queue.shift(); queuedBytes -= item.size; return item.value; }
      if (failure) throw failure;
      return null;
    },
    close() {
      closed = true;
      queue.length = 0;
      queuedBytes = 0;
      notify();
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'cancel', streamId }));
      socket.close();
    },
  };
}

const passiveEvents = new Set([
  'agent-preset/selected', 'agent/inbox/spliced', 'approval/asked', 'approval/decided', 'approval/policy',
  'assistant/attempt', 'command/done', 'command/run', 'compaction/end', 'compaction/prune', 'compaction/start',
  'compaction/summary', 'deliverables/presented', 'developer/message', 'feedback/message-delete',
  'feedback/message-put', 'feedback/record', 'goal/change', 'hook/invoked', 'hook/result', 'image/offload',
  'llm/retry', 'llm/retry-started', 'model/selection', 'permission/preset', 'plan/mode', 'request/context',
  'request/header', 'sandbox/mode', 'schedule/change', 'session-log-deepseek/delivery-accepted',
  'session/end-seed', 'session/title', 'session/title-llm-request', 'step/end', 'step/start', 'subagent/catalog',
  'subagent/descriptor', 'subagent/model-selection-policy', 'system/message', 'team/member',
  'team/message/delivered', 'team/message/queued', 'team/task', 'todo/write', 'tool-workflow/agent-end',
  'tool-workflow/agent-start', 'tool-workflow/run-end', 'tool-workflow/run-start', 'tool/ptc-dispatch',
  'tool/ptc-dispatch-start', 'web/deepseek-search-llm-request', 'workspace/changes',
]);
const handledEvents = new Set(['turn/start', 'turn/end', 'user/message', 'assistant/message', 'tool/call', 'tool/result']);

function wireEvent(record) {
  const event = record?.type === 'event' && record.event;
  if (!event || typeof event.type !== 'string' || !Number.isSafeInteger(event.seq) || event.seq < 0
    || !Number.isSafeInteger(event.time) || !Object.hasOwn(event, 'data')) fail('session_event_invalid');
  if (!handledEvents.has(event.type) && !passiveEvents.has(event.type) && event.ignorable !== true) fail('session_event_unsupported');
  if (handledEvents.has(event.type)) {
    const data = event.data;
    if (!data || typeof data !== 'object') fail('session_event_invalid');
    if (event.type !== 'user/message' && (!Number.isSafeInteger(data.turn) || data.turn < 0)) fail('session_event_invalid');
    if (event.type === 'turn/end' && typeof data.reason?.kind !== 'string') fail('session_event_invalid');
    if (event.type === 'user/message' && !Array.isArray(data.content)) fail('session_event_invalid');
    if (event.type === 'assistant/message' && !Array.isArray(data.message?.content)) fail('session_event_invalid');
    if (event.type === 'tool/call' && (typeof data.callId !== 'string' || typeof data.name !== 'string'
      || typeof data.arguments !== 'string')) fail('session_event_invalid');
    if (event.type === 'tool/result' && (data.message?.role !== 'tool' || typeof data.message.toolCallId !== 'string'
      || !Array.isArray(data.message.content))) fail('session_event_invalid');
  }
  return event;
}

function checkSessionHeader(header, cwd, sessionId) {
  if (!header || header.id !== sessionId || !Number.isSafeInteger(header.createdAt) || !header.cwd
    || canonicalDirectory(header.cwd) !== cwd || header.parentSession || header.origin === 'subagent'
    || (header.delegationDepth ?? 0) !== 0) fail('session_project_mismatch');
}

function openTurn(rows, active = null) {
  for (const row of rows) {
    if (row.type === 'turn/start') active = row.data.turn;
    if (row.type === 'turn/end' && row.data.turn === active) active = null;
  }
  return active;
}

// A journal is a service-owned event sequence. No DSH storage paths or formats enter this boundary.
export class SessionJournal {
  constructor(rpc, config, sessionId, cwd) {
    this.rpc = rpc; this.config = config; this.sessionId = sessionId; this.cwd = canonicalDirectory(cwd);
    this.rows = []; this.cursor = -1; this.activeTurn = null;
  }
  async refresh() {
    this.stream?.close();
    const stream = this.rpc.follow({ address: { kind: 'session', sessionId: this.sessionId }, maxMessages: 100 });
    this.stream = stream;
    try {
      const snapshot = await stream.next(this.config.streamTimeoutMs ?? 10000);
      if (!snapshot) fail('web_stream_timeout');
      if (snapshot.type !== 'snapshot' || !Number.isSafeInteger(snapshot.cursor) || snapshot.cursor < -1
        || !Array.isArray(snapshot.records) || typeof snapshot.hasMore !== 'boolean') fail('session_snapshot_invalid');
      checkSessionHeader(snapshot.header, this.cwd, this.sessionId);
      if (this.header && this.header.createdAt !== snapshot.header.createdAt) fail('session_history_changed');
      const pages = [snapshot.records.map(wireEvent)];
      let hasMore = snapshot.hasMore, before = pages[0][0]?.seq;
      while (hasMore) {
        if (!Number.isSafeInteger(before) || before <= 0) fail('session_page_invalid');
        const page = await this.rpc('session/page', { address: { kind: 'session', sessionId: this.sessionId },
          throughSeq: snapshot.cursor, beforeSeq: before, maxMessages: 100 });
        if (!Array.isArray(page?.records) || !page.records.length || typeof page.hasMore !== 'boolean') fail('session_page_invalid');
        const rows = page.records.map(wireEvent);
        if (rows.at(-1).seq !== before - 1) fail('session_event_gap');
        pages.push(rows);
        before = rows[0].seq;
        hasMore = page.hasMore;
      }
      const rows = pages.reverse().flat();
      if (rows.length !== snapshot.cursor + 1 || rows.some((row, i) => row.seq !== i)) fail('session_event_gap');
      if (this.cursor > snapshot.cursor || this.rows.some((row, i) => JSON.stringify(row) !== JSON.stringify(rows[i]))) fail('session_history_changed');
      const added = rows.slice(this.cursor + 1);
      this.header = snapshot.header;
      this.rows = rows;
      this.cursor = snapshot.cursor;
      this.activeTurn = openTurn(rows);
      return added;
    } catch (error) { stream.close(); throw error; }
  }
  async recover(error, control = {}) {
    let attempt = 0;
    for (;;) {
      if (control.cancelled) fail('cancelled_during_recovery');
      // Log only controlled codes and endpoint names, never host errors or credentials.
      if (attempt === 0 || (attempt & (attempt - 1)) === 0) {
        process.stderr.write(`DSH transport recovery: attempt=${attempt + 1} ${reasonOf(error)}${error.rpcMethod ? ` (${error.rpcMethod})` : ''}\n`);
      }
      const delay = Math.min((this.config.reconnectMs ?? 250) * 2 ** Math.min(attempt++, 5), 5000);
      const until = Date.now() + delay;
      while (Date.now() < until) {
        if (control.cancelled) fail('cancelled_during_recovery');
        await sleep(Math.min(50, until - Date.now()));
      }
      try {
        // The session-scoped snapshot includes running turns and durable outcomes.
        // A host-wide session list can be slow and cannot identify this request's result.
        await this.refresh();
        return;
      } catch (next) {
        if (!transportFailure(next)) throw next;
        error = next;
      }
    }
  }
  async refreshRecover(control = {}) {
    try { return await this.refresh(); }
    catch (error) {
      if (!transportFailure(error)) throw error;
      const start = this.cursor;
      await this.recover(error, control);
      return this.rows.slice(start + 1);
    }
  }
  async readNew(timeoutMs = 0, control = {}) {
    const start = this.cursor;
    try {
      let frame = await this.stream.next(timeoutMs);
      while (frame) {
        const event = wireEvent(frame);
        if (event.seq <= this.cursor) {
          if (JSON.stringify(this.rows[event.seq]) !== JSON.stringify(event)) fail('session_history_changed');
        } else {
          if (event.seq !== this.cursor + 1) fail('session_event_gap');
          this.rows.push(event);
          this.cursor = event.seq;
          this.activeTurn = openTurn([event], this.activeTurn);
        }
        frame = await this.stream.next(0);
      }
    } catch (error) {
      if (!transportFailure(error) && reasonOf(error) !== 'session_event_gap') throw error;
      // Preserve a terminal event already received even if its carrier closes immediately.
      if (!this.rows.slice(start + 1).some(row => row.type === 'turn/end')) await this.recover(error, control);
    }
    return this.rows.slice(start + 1);
  }
  close() { this.stream?.close(); }
}

export async function prepareSession(command, config, rpc, control = {}) {
  const cwd = canonicalDirectory(command.cwd);
  const sessionId = command.resume_session_id || `multica-web-${createHash('sha256').update(command.request_id).digest('hex').slice(0, 32)}`;
  if (!safeId.test(sessionId) || !sessionId.startsWith('multica-web-')) fail('multica_session_required');
  const journal = new SessionJournal(rpc, config, sessionId, cwd);
  // These startup operations are idempotent by canonical path, explicit session
  // identity, or exact title. Retrying a prompt is deliberately outside this path.
  const setup = async (method, request) => {
    let attempt = 0;
    for (;;) {
      if (control.cancelled) fail('cancelled_before_delivery');
      try { return await rpc(method, request); }
      catch (error) {
        if (!transportFailure(error)) throw error;
        if (attempt === 0 || (attempt & (attempt - 1)) === 0) {
          process.stderr.write(`DSH startup recovery: attempt=${attempt + 1} ${reasonOf(error)} (${method})\n`);
        }
        const until = Date.now() + Math.min((config.reconnectMs ?? 250) * 2 ** Math.min(attempt++, 5), 5000);
        while (Date.now() < until) {
          if (control.cancelled) fail('cancelled_before_delivery');
          await sleep(Math.min(50, until - Date.now()));
        }
      }
    }
  };
  try {
    // Validate existing identities before adoption, including deterministic first-send retries.
    try { await journal.refreshRecover(control); }
    catch (error) {
      if (reasonOf(error) !== 'session/not-found' || command.resume_session_id) throw error;
    }
    const created = await setup('workspace/create', { path: command.cwd });
    const workspace = created?.workspace;
    if (!workspace?.workspaceId || canonicalDirectory(workspace.path) !== cwd) fail('workspace_receipt_mismatch');
    const adopted = await setup('session/create', { sessionId, workspaceId: workspace.workspaceId });
    if (adopted?.sessionId !== sessionId) fail('session_receipt_mismatch');
    const checked = await setup('workspace/create', { path: command.cwd });
    if (checked?.workspace?.workspaceId !== workspace.workspaceId || !checked.workspace.sessionIds?.includes(sessionId)) fail('workspace_membership_unconfirmed');
    await journal.refreshRecover(control);
    if (!command.resume_session_id && command.thread_name) await setup('session/rename', { sessionId, title: `Multica · ${command.thread_name}` });
    await journal.readNew(0, control);
    return { sessionId, journal, rows: journal.rows, header: journal.header, origin: config.origin };
  } catch (error) { journal.close(); throw error; }
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

// Persist only the brief digest and an API event witness, never prompt text or credentials.
function prepareBrief(command, prepared, directory) {
  const brief = command.system_prompt;
  if (!brief) return { prefix: '', confirm() {} };
  const hash = createHash('sha256').update(brief).digest('hex');
  const identity = `${prepared.origin}\n${prepared.sessionId}\n${prepared.header.createdAt}`;
  const key = createHash('sha256').update(identity).digest('hex');
  const file = path.join(directory, `${key}.json`);
  let cached;
  try { cached = JSON.parse(fs.readFileSync(file, 'utf8')); } catch { /* Missing or corrupt means resend. */ }
  const witness = cached && prepared.rows.find(row => row.seq === cached.seq && row.type === 'user/message'
    && row.data?.source?.rpcId === cached.requestId && textOf(row.data).includes(brief + '\n\n'));
  if (command.resume_session_id && cached?.v === 2 && cached.hash === hash && witness) return { prefix: '', confirm() {} };
  try { fs.unlinkSync(file); }
  catch (error) { if (error.code !== 'ENOENT') fail('runtime_brief_cache_unavailable'); }
  let confirmed = false;
  return {
    prefix: `${command.resume_session_id ? '以下运行说明取代本会话中此前的版本，按本版执行。\n' : ''}${brief}\n\n`,
    confirm(rows, sentText) {
      const witness = rows.find(row => row.type === 'user/message'
        && row.data?.source?.rpcId === command.request_id && textOf(row.data) === sentText);
      if (confirmed || !witness) return;
      confirmed = true;
      const record = { v: 2, hash, requestId: command.request_id, seq: witness.seq };
      const temp = file + `.${randomUUID()}.tmp`;
      try {
        fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
        fs.writeFileSync(temp, JSON.stringify(record), { flag: 'wx', mode: 0o600 });
        fs.renameSync(temp, file);
      } catch { /* Cache failure is safe: the next run sends the full brief again. */ }
      finally { try { fs.unlinkSync(temp); } catch { /* The rename normally consumed it. */ } }
    },
  };
}

export class TurnReader {
  constructor(requestId, emit) { this.requestId = requestId; this.emit = emit; this.turn = null; this.activeTurn = null; this.output = ''; this.pending = []; this.toolNames = new Map(); }
  push(row) {
    if (this.terminal) return;
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
      case 'tool/call':
        this.output = '';
        this.toolNames.set(data.callId, data.name);
        this.emit({ type: 'tool_call', call_id: data.callId, name: data.name, arguments: data.arguments });
        break;
      case 'tool/result': {
        const message = data.message, callId = message.toolCallId;
        this.emit({ type: 'tool_result', call_id: callId, name: this.toolNames.get(callId),
          output: textOf(message), is_error: message.isError === true });
        break;
      }
      case 'turn/end': this.terminal = { status: data.reason?.kind === 'completed' ? 'completed' : data.reason?.kind === 'user' ? 'cancelled' : 'failed', output: this.output,
        ...(data.reason?.kind === 'completed' ? {} : { error: { code: 'dsh_turn_ended', message: String(data.reason?.kind ?? 'unknown') } }) }; break;
    }
  }
}

export async function execute(command, config, emit, control, dependencies = {}) {
  let cli, prepared, delivered = false;
  try {
    if (command.v !== 1 || command.type !== 'execute' || !safeId.test(command.request_id) || !command.prompt?.trim()) fail('invalid_execute_command');
    if (command.model && (command.model.provider !== 'web' || command.model.id !== 'default') || command.reasoning_effort) fail('use_dsh_web_model_settings');
    if (command.mcp_servers?.length) fail('use_dsh_web_mcp_settings');
    const rpc = await (dependencies.connectWeb ?? connectWeb)(config);
    prepared = await prepareSession(command, config, rpc, control);
    emit({ type: 'session', session_id: prepared.sessionId, resumed: Boolean(command.resume_session_id) });
    if (control.cancelled) fail('cancelled_before_delivery');
    const reader = new TurnReader(command.request_id, emit);
    for (const row of prepared.rows) reader.push(row);
    if (reader.terminal) { emit({ type: 'result', session_id: prepared.sessionId, ...reader.terminal }); return; }
    // A retry of an admitted request observes that run instead of re-posting the prompt.
    delivered = reader.turn !== null || prepared.rows.some(row => row.type === 'agent/inbox/spliced'
      && Array.isArray(row.data?.inserted) && row.data.inserted.some(message => message?.source?.rpcId === command.request_id));
    if (!delivered && prepared.journal.activeTurn !== null) fail('session_busy');
    let brief = { confirm() {} }, text;
    if (!delivered) {
      cli = taskCli(dependencies.env ?? process.env);
      brief = prepareBrief(command, prepared, dependencies.briefCacheDir ?? path.join(os.homedir(), '.multica', 'dsh-web-briefs'));
      text = `${cli.instructions}${brief.prefix}${command.prompt}`;
      try {
        const receipt = await rpc('session/prompt', { sessionId: prepared.sessionId, requestId: command.request_id, mode: 'queue', content: [{ type: 'text', text }] });
        if (receipt?.accepted !== true) fail('prompt_receipt_invalid');
      } catch (error) {
        if (!transportFailure(error)) throw error;
        // Never re-post a mutation whose receipt was lost. Observe its request witness instead.
        const deadline = Date.now() + (config.receiptTimeoutMs ?? 30000);
        for (;;) {
          const rows = await prepared.journal.readNew(config.pollMs ?? 250, control);
          brief.confirm(rows, text);
          for (const row of rows) reader.push(row);
          if (reader.turn !== null || prepared.journal.rows.some(row => row.type === 'agent/inbox/spliced'
            && row.data?.inserted?.some(message => message?.source?.rpcId === command.request_id))) break;
          if (control.cancelled) fail('cancelled_during_recovery');
          if (Date.now() >= deadline) fail('web_rpc_outcome_unknown');
        }
      }
      delivered = true;
    }
    let cancelDeadline;
    const consume = rows => { brief.confirm(rows, text); for (const row of rows) reader.push(row); };
    for (;;) {
      if (reader.terminal) { emit({ type: 'result', session_id: prepared.sessionId, ...reader.terminal }); return; }
      consume(await prepared.journal.readNew(config.pollMs ?? 250, control));
      if (reader.terminal) { emit({ type: 'result', session_id: prepared.sessionId, ...reader.terminal }); return; }
      if (control.cancelled && !cancelDeadline) {
        // Obtain a fresh event cut before cancelling a session shared with browser users.
        consume(await prepared.journal.refresh());
        if (reader.terminal) { emit({ type: 'result', session_id: prepared.sessionId, ...reader.terminal }); return; }
        if (reader.turn === null || prepared.journal.activeTurn !== reader.turn) fail('cancel_target_not_active');
        const receipt = await rpc('session/cancel', { sessionId: prepared.sessionId });
        if (receipt?.accepted !== true) fail('cancel_receipt_invalid');
        cancelDeadline = Date.now() + 5000;
      }
      if (cancelDeadline && Date.now() >= cancelDeadline) fail('cancel_not_confirmed');
    }
  } catch (error) {
    const code = reasonOf(error);
    emit({ type: 'result', session_id: prepared?.sessionId ?? command.resume_session_id ?? '', status: control.cancelled ? 'cancelled' : 'failed', output: '',
      error: { code, message: delivered || code === 'web_rpc_outcome_unknown'
        ? 'The DSH web run may still be active. Open DSH to inspect it before retrying.'
        : code === 'dsh_event_api_unsupported' ? 'This DSH host does not support the required session event API. Update the bridge or use a supported DSH host.' : code } });
  } finally { prepared?.journal.close(); cli?.cleanup(); }
}

// Inspect a historical request without workspace adoption, prompts, or cancellation.
export async function recoverRun(command, config, rpc) {
  if (typeof command.request_id !== 'string' || typeof command.session_id !== 'string'
    || !safeId.test(command.request_id) || !safeId.test(command.session_id)
    || !command.session_id.startsWith('multica-web-')) fail('invalid_recovery_identity');
  const journal = new SessionJournal(rpc, config, command.session_id, command.cwd);
  try {
    await journal.refresh();
    const reader = new TurnReader(command.request_id, () => {});
    for (const row of journal.rows) reader.push(row);
    if (!reader.terminal) fail('request_not_completed');
    return { v: 1, type: 'result', request_id: command.request_id, session_id: command.session_id, ...reader.terminal };
  } finally { journal.close(); }
}

export async function probeWeb(config, rpc) {
  const stream = rpc.follow({ address: { kind: 'session', sessionId: `multica-web-probe-${randomUUID()}` } });
  try {
    await stream.next(config.streamTimeoutMs ?? 10000);
    fail('session_probe_invalid');
  } catch (error) {
    // A recognized missing session proves authentication, the carrier and the endpoint, without creating a session.
    if (reasonOf(error) !== 'session/not-found') throw error;
  } finally { stream.close(); }
}

export async function main(args = process.argv.slice(2)) {
  const emit = frame => process.stdout.write(JSON.stringify({ v: 1, ...frame }) + '\n');
  if (args.includes('--version')) { process.stdout.write('multica-dsh-web 1.0.3\n'); return; }
  const config = loadConfig();
  if (args.includes('--recover')) {
    const option = flag => args.includes(flag) ? args[args.indexOf(flag) + 1] : undefined;
    const result = await recoverRun({ request_id: option('--request-id'), session_id: option('--session-id'), cwd: option('--cwd') },
      config, await connectWeb(config));
    emit(result);
    return;
  }
  if (args.includes('--probe')) {
    await probeWeb(config, await connectWeb(config));
    emit({ type: 'probe', runtime: 'dsh', protocol_version: 1 });
    return;
  }
  if (args.includes('--list-models')) {
    await probeWeb(config, await connectWeb(config));
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

// import.meta.main requires Node.js 24.2 or newer; older 24.x would otherwise
// load this file, skip main() and exit 0 without doing anything.
if (typeof import.meta.main !== 'boolean') {
  process.stderr.write('DSH web bridge requires Node.js 24 or newer\n');
  process.exitCode = 1;
} else if (import.meta.main) {
  main().catch(error => { process.stderr.write(reasonOf(error) + '\n'); process.exitCode = 1; });
}
