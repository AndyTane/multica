// Pure observation helpers for the packaged desktop acceptance test
// (CLAWSTA-80 Task 8, batch D2).
//
// The desktop spec feeds raw recorder and TCP-sampler metadata through these
// helpers so the acceptance rules can be regression-tested without launching
// Electron. Nothing here writes evidence or talks to the network: the input is
// already-sanitized metadata (timestamp, kind, host, method, result, PID,
// remote address, state).

export const TRAFFIC_KINDS = new Set(["http", "connect", "upgrade"]);

export class ObserverError extends Error {
  constructor(message) {
    super(message);
    this.name = "ObserverError";
  }
}

function requireFiniteTimestamp(value, label) {
  const parsed = typeof value === "string" ? Date.parse(value) : Number.NaN;
  if (!Number.isFinite(parsed)) {
    throw new ObserverError(label + " has no valid timestamp: " + JSON.stringify(value));
  }
  return parsed;
}

/**
 * Parses JSON-lines evidence. A malformed line is fatal: a partial or corrupt
 * record must never be silently dropped and read back as "zero traffic".
 */
export function parseJsonLines(text, label) {
  if (typeof text !== "string") throw new ObserverError(label + " content must be a string");
  const entries = [];
  const lines = text.split(/\r?\n/);
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index].trim();
    if (!line) continue;
    let parsed;
    try {
      parsed = JSON.parse(line);
    } catch (error) {
      throw new ObserverError(
        "malformed " + label + " line " + (index + 1) + ": " + (error instanceof Error ? error.message : String(error)),
      );
    }
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new ObserverError(label + " line " + (index + 1) + " is not a JSON object");
    }
    entries.push(parsed);
  }
  return entries;
}

/** Recorder entries; traffic entries require a valid timestamp and exact host. */
export function parseRecorderLog(text) {
  const entries = parseJsonLines(text, "recorder");
  for (const entry of entries) {
    if (typeof entry.kind !== "string" || entry.kind.length === 0) {
      throw new ObserverError("recorder entry has no kind");
    }
    if (TRAFFIC_KINDS.has(entry.kind)) {
      requireFiniteTimestamp(entry.ts, "recorder " + entry.kind + " entry");
      if (typeof entry.host !== "string" || entry.host.length === 0) {
        throw new ObserverError("recorder " + entry.kind + " entry has no destination host");
      }
    }
  }
  return entries;
}

/**
 * TCP sampler samples. Each sample must carry a timestamp, the poll interval
 * and per-process-group PID lists; sampler errors stay in the sample so a
 * phase can fail instead of counting the gap as zero.
 */
export function parseSamplerLog(text) {
  const samples = parseJsonLines(text, "sampler");
  for (const sample of samples) {
    requireFiniteTimestamp(sample.ts, "sampler sample");
    if (typeof sample.intervalMs !== "number" || !(sample.intervalMs > 0)) {
      throw new ObserverError("sampler sample has no valid intervalMs");
    }
    if (!sample.pids || typeof sample.pids !== "object" || Array.isArray(sample.pids)) {
      throw new ObserverError("sampler sample has no pids object");
    }
    if (!Array.isArray(sample.nonLoopback)) {
      throw new ObserverError("sampler sample has no nonLoopback array");
    }
    if (sample.error !== undefined && typeof sample.error !== "string") {
      throw new ObserverError("sampler sample error must be a string when present");
    }
  }
  return samples;
}

/**
 * Control windows are the only exclusion mechanism. Each window names the
 * exact destination hosts the probe may create inside its own time range; the
 * same host outside the window is a violation, and there is no wildcard.
 */
function toControlWindow(window) {
  const startMs = requireFiniteTimestamp(window.start, "control window start");
  const endMs = requireFiniteTimestamp(window.end, "control window end");
  if (endMs < startMs) throw new ObserverError("control window ends before it starts: " + window.label);
  const hosts = window.hosts instanceof Set ? [...window.hosts] : window.hosts;
  if (!Array.isArray(hosts) || hosts.length === 0) {
    throw new ObserverError("control window has no exact hosts: " + window.label);
  }
  for (const host of hosts) {
    if (typeof host !== "string" || host.length === 0 || host.includes("*")) {
      throw new ObserverError("control window hosts must be exact strings: " + window.label);
    }
  }
  return { label: window.label, startMs, endMs, hosts: new Set(hosts) };
}

export function makeControlWindows(windows) {
  return windows.map((window) => toControlWindow(window));
}

function insideControlWindow(entry, windows) {
  const ts = Date.parse(entry.ts);
  for (const window of windows) {
    if (ts < window.startMs || ts > window.endMs) continue;
    if (window.hosts.has(entry.host)) return window.label;
  }
  return null;
}

/**
 * Splits recorder traffic inside one phase window into excluded control
 * entries and violations. Control entries are matched by exact host AND their
 * own timestamp window only.
 */
export function classifyRecorderEntries(entries, options) {
  const startMs = requireFiniteTimestamp(options.start, "phase start");
  const endMs = requireFiniteTimestamp(options.end, "phase end");
  const controlWindows = (options.controlWindows ?? []).map((window) =>
    window.hosts instanceof Set ? window : toControlWindow(window),
  );
  const violations = [];
  const excluded = [];
  for (const entry of entries) {
    if (!TRAFFIC_KINDS.has(entry.kind)) continue;
    const ts = Date.parse(entry.ts);
    if (!Number.isFinite(ts) || ts < startMs || ts > endMs) continue;
    const controlLabel = insideControlWindow(entry, controlWindows);
    if (controlLabel) {
      excluded.push({ kind: entry.kind, host: entry.host, method: entry.method ?? null, control: controlLabel });
    } else {
      violations.push({ kind: entry.kind, host: entry.host, method: entry.method ?? null });
    }
  }
  return { violations, excluded };
}

/** Browser-layer delta; any non-loopback request/websocket/navigation fails. */
export function browserPhaseViolations(delta) {
  const violations = [];
  if (delta.externalRequests > 0) violations.push({ layer: "browser", kind: "request", count: delta.externalRequests });
  if (delta.externalWebsockets > 0) {
    violations.push({ layer: "browser", kind: "websocket", count: delta.externalWebsockets });
  }
  if (delta.externalNavigations > 0) {
    violations.push({ layer: "browser", kind: "navigation", count: delta.externalNavigations });
  }
  if (delta.failedRequests > 0) violations.push({ layer: "browser", kind: "failedRequest", count: delta.failedRequests });
  return violations;
}

/**
 * Summarizes one observation phase. A phase without samples, with sampler
 * errors, with unexpected non-loopback connections or with a recorder/browser
 * violation fails; missing data is never treated as zero.
 */
export function summarizePhase(options) {
  const startMs = requireFiniteTimestamp(options.start, options.name + " start");
  const endMs = requireFiniteTimestamp(options.end, options.name + " end");
  const durationMs = endMs - startMs;
  const intervalMs = options.samplerIntervalMs;
  const samples = (options.samples ?? []).filter((sample) => {
    const ts = Date.parse(sample.ts);
    return ts >= startMs && ts <= endMs;
  });
  const errors = [];

  if (!(durationMs >= 0)) errors.push(options.name + ": phase window is inverted");
  if (!(typeof intervalMs === "number" && intervalMs > 0)) errors.push(options.name + ": sampler interval is unknown");

  if (samples.length === 0) {
    errors.push(options.name + ": no TCP samples inside the phase; missing data cannot be read as zero outbound");
  }
  const samplerErrors = samples.filter((sample) => typeof sample.error === "string" && sample.error.length > 0);
  for (const sample of samplerErrors) {
    errors.push(options.name + ": TCP sampler error at " + sample.ts + ": " + sample.error);
  }

  const timestamps = samples.map((sample) => Date.parse(sample.ts)).sort((a, b) => a - b);
  const gaps = [];
  for (let index = 1; index < timestamps.length; index += 1) {
    gaps.push(timestamps[index] - timestamps[index - 1]);
  }
  const maxGapMs = gaps.length > 0 ? Math.max(...gaps) : durationMs;
  const gapLimitMs = (typeof intervalMs === "number" && intervalMs > 0 ? intervalMs : 1000) * 4 + 1500;
  if (durationMs > gapLimitMs && maxGapMs > gapLimitMs) {
    errors.push(
      options.name + ": TCP sampling gap of " + maxGapMs + " ms exceeds the " + gapLimitMs + " ms limit",
    );
  }

  const allowedNonLoopback = new Set(options.allowedNonLoopback ?? []);
  const nonLoopback = [];
  const sampledAppPids = new Set();
  const sampledDaemonPids = new Set();
  for (const sample of samples) {
    for (const pid of sample.pids?.app ?? []) sampledAppPids.add(pid);
    for (const pid of sample.pids?.daemon ?? []) sampledDaemonPids.add(pid);
    for (const connection of sample.nonLoopback) {
      nonLoopback.push(connection);
      if (!allowedNonLoopback.has(connection.remote)) {
        errors.push(
          options.name + ": non-loopback connection " + connection.remote + " (pid " + connection.pid + ") at " + sample.ts,
        );
      }
    }
  }
  for (const expected of allowedNonLoopback) {
    if (!nonLoopback.some((connection) => connection.remote === expected)) {
      errors.push(options.name + ": expected control connection " + expected + " was not sampled");
    }
  }

  const recorder = classifyRecorderEntries(options.recorderEntries ?? [], {
    start: options.start,
    end: options.end,
    controlWindows: options.controlWindows ?? [],
  });
  for (const violation of recorder.violations) {
    errors.push(options.name + ": recorder saw " + violation.kind + " to " + violation.host);
  }

  // A null delta means the browser event layer was not attached yet (for
  // example before the first window). That is recorded as "not measured", it is
  // never reported as a measured zero; the recorder layer still covers Chromium
  // traffic for that phase.
  const browserLayerMeasured = options.browserDelta !== null && options.browserDelta !== undefined;
  const browserViolations = browserLayerMeasured ? browserPhaseViolations(options.browserDelta) : [];
  for (const violation of browserViolations) {
    errors.push(options.name + ": browser " + violation.kind + " count " + violation.count);
  }

  return {
    name: options.name,
    start: options.start,
    end: options.end,
    durationMs,
    samplerIntervalMs: intervalMs ?? null,
    ok: errors.length === 0,
    errors,
    recorder: {
      violations: recorder.violations,
      excludedControlEntries: recorder.excluded,
    },
    browserLayerMeasured,
    browser: browserLayerMeasured ? options.browserDelta : null,
    tcp: {
      samples: samples.length,
      errors: samplerErrors.length,
      minGapMs: gaps.length > 0 ? Math.min(...gaps) : null,
      maxGapMs: gaps.length > 0 ? maxGapMs : null,
      nonLoopbackCount: nonLoopback.length,
      nonLoopback,
      sampledAppProcesses: sampledAppPids.size,
      sampledDaemonProcesses: sampledDaemonPids.size,
    },
  };
}

/**
 * Cleanup attribution: only processes proven to belong to this run may be
 * stopped. The workspace daemon and anything from the pre-run snapshot are
 * protected even if their command line looks similar.
 */
export function planProcessCleanup(candidates, options) {
  const preExisting = new Set(options.preExistingPids ?? []);
  const protectedPids = new Set(options.protectedPids ?? []);
  const runStartedAtMs = requireFiniteTimestamp(options.runStartedAt, "run start");
  const expectedPath = String(options.expectedExecutablePath ?? "").replace(/\//g, "\\").toLowerCase();
  const approved = [];
  const rejected = [];
  for (const candidate of candidates) {
    const pid = candidate.pid;
    if (!Number.isInteger(pid) || pid <= 0) {
      rejected.push({ pid: pid ?? null, reason: "invalid pid" });
      continue;
    }
    if (protectedPids.has(pid)) {
      rejected.push({ pid, reason: "workspace daemon must never be stopped" });
      continue;
    }
    if (preExisting.has(pid)) {
      rejected.push({ pid, reason: "process existed before this run" });
      continue;
    }
    const executablePath = String(candidate.executablePath ?? "").replace(/\//g, "\\").toLowerCase();
    if (!executablePath || executablePath !== expectedPath) {
      rejected.push({
        pid,
        reason: "executable path does not match the run-owned binary: " + (candidate.executablePath ?? "unknown"),
      });
      continue;
    }
    const creationMs = Date.parse(candidate.creationDate);
    if (!Number.isFinite(creationMs)) {
      rejected.push({ pid, reason: "creation time is unknown" });
      continue;
    }
    if (creationMs < runStartedAtMs - 2000) {
      rejected.push({ pid, reason: "process started before this run" });
      continue;
    }
    approved.push(pid);
  }
  return { approved, rejected };
}

/** Recursive deletions must stay inside the run's isolated root. */
export function assertInsideRoot(targetPath, rootPath, label) {
  const resolvedTarget = String(targetPath).replace(/\//g, "\\").replace(/\\+$/, "").toLowerCase();
  const resolvedRoot = String(rootPath).replace(/\//g, "\\").replace(/\\+$/, "").toLowerCase();
  if (!resolvedTarget || !resolvedRoot) throw new ObserverError(label + ": path is empty");
  if (resolvedTarget === resolvedRoot) throw new ObserverError(label + ": refusing to remove the isolated root itself");
  if (!resolvedTarget.startsWith(resolvedRoot + "\\")) {
    throw new ObserverError(label + ": refusing to remove a path outside the isolated root: " + targetPath);
  }
  return true;
}
