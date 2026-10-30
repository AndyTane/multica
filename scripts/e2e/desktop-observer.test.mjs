// Minimal observer regression for the desktop acceptance rules (D2).
// Runs with the Node test runner: node --test scripts/e2e/desktop-observer.test.mjs
import test from "node:test";
import assert from "node:assert/strict";

import {
  ObserverError,
  assertInsideRoot,
  parseRecorderLog,
  parseSamplerLog,
  planProcessCleanup,
  summarizePhase,
} from "./desktop-observer.mjs";

const START = "2026-10-31T00:00:00.000Z";
const END = "2026-10-31T00:00:10.000Z";
const INTERVAL = 500;

function samplesBetween(startIso, endIso, extra = {}) {
  const samples = [];
  const start = Date.parse(startIso);
  const end = Date.parse(endIso);
  for (let ts = start; ts <= end; ts += INTERVAL) {
    samples.push({
      ts: new Date(ts).toISOString(),
      intervalMs: INTERVAL,
      pids: { app: [1000, 1001], daemon: [2000], extra: [] },
      nonLoopback: [],
      ...extra,
    });
  }
  return samples;
}

function phaseOptions(overrides = {}) {
  return {
    name: "login",
    start: START,
    end: END,
    samplerIntervalMs: INTERVAL,
    samples: samplesBetween(START, END),
    recorderEntries: [],
    controlWindows: [],
    browserDelta: { externalRequests: 0, externalWebsockets: 0, externalNavigations: 0, failedRequests: 0 },
    ...overrides,
  };
}

test("a non-control request during the login phase fails the phase", () => {
  const phase = summarizePhase(
    phaseOptions({
      recorderEntries: [
        { ts: "2026-10-31T00:00:03.000Z", kind: "connect", host: "updates.example.invalid:443", method: "CONNECT" },
      ],
    }),
  );
  assert.equal(phase.ok, false);
  assert.match(phase.errors.join("\n"), /recorder saw connect to updates\.example\.invalid:443/);
});

test("only the exact control host inside its own window is excluded", () => {
  const controlWindows = [
    {
      label: "chromium self test",
      start: "2026-10-31T00:00:01.000Z",
      end: "2026-10-31T00:00:02.000Z",
      hosts: ["uw80-desktop-selftest.invalid"],
    },
  ];
  const excludedPhase = summarizePhase(
    phaseOptions({
      controlWindows,
      recorderEntries: [
        { ts: "2026-10-31T00:00:01.500Z", kind: "http", host: "uw80-desktop-selftest.invalid", method: "GET" },
      ],
    }),
  );
  assert.equal(excludedPhase.ok, true, excludedPhase.errors.join("; "));
  assert.equal(excludedPhase.recorder.excludedControlEntries.length, 1);
  assert.equal(excludedPhase.recorder.violations.length, 0);
});

test("the same control host outside its window, or with a different host, is a violation", () => {
  const controlWindows = [
    {
      label: "chromium self test",
      start: "2026-10-31T00:00:05.000Z",
      end: "2026-10-31T00:00:06.000Z",
      hosts: ["uw80-desktop-selftest.invalid"],
    },
  ];
  const outsideWindow = summarizePhase(
    phaseOptions({
      controlWindows,
      recorderEntries: [
        { ts: "2026-10-31T00:00:01.500Z", kind: "http", host: "uw80-desktop-selftest.invalid", method: "GET" },
      ],
    }),
  );
  assert.equal(outsideWindow.ok, false);
  assert.match(outsideWindow.errors.join("\n"), /selftest\.invalid/);

  const differentHost = summarizePhase(
    phaseOptions({
      controlWindows,
      recorderEntries: [
        { ts: "2026-10-31T00:00:05.500Z", kind: "http", host: "uw80-desktop-selftest-evil.invalid", method: "GET" },
      ],
    }),
  );
  assert.equal(differentHost.ok, false);
});

test("a traffic entry without a valid timestamp is rejected, never read as zero", () => {
  assert.throws(
    () => parseRecorderLog(JSON.stringify({ kind: "connect", host: "example.invalid:443" }) + "\n"),
    ObserverError,
  );
  assert.throws(
    () => parseRecorderLog(JSON.stringify({ kind: "connect", host: "example.invalid:443", ts: "not-a-date" }) + "\n"),
    ObserverError,
  );
});

test("malformed sampler lines fail parsing instead of shrinking the sample set", () => {
  assert.throws(() => parseSamplerLog("{\n"), ObserverError);
  assert.throws(() => parseSamplerLog(JSON.stringify({ ts: "2026-10-31T00:00:00.000Z" }) + "\n"), ObserverError);
  assert.throws(
    () =>
      parseSamplerLog(
        JSON.stringify({
          ts: "2026-10-31T00:00:00.000Z",
          intervalMs: 500,
          pids: {},
          nonLoopback: "not-an-array",
        }) + "\n",
      ),
    ObserverError,
  );
});

test("a phase with missing samples or sampler errors fails instead of counting zero", () => {
  const empty = summarizePhase(phaseOptions({ samples: [] }));
  assert.equal(empty.ok, false);
  assert.match(empty.errors.join("\n"), /no TCP samples/);

  const withError = summarizePhase(
    phaseOptions({
      samples: samplesBetween(START, END).map((sample, index) =>
        index === 4 ? { ...sample, error: "Get-NetTCPConnection failed" } : sample,
      ),
    }),
  );
  assert.equal(withError.ok, false);
  assert.match(withError.errors.join("\n"), /TCP sampler error/);
});

test("unexpected non-loopback connections fail the phase; a declared control connection is reported missing until sampled", () => {
  const failure = summarizePhase(
    phaseOptions({
      samples: samplesBetween(START, END).map((sample, index) =>
        index === 2
          ? { ...sample, nonLoopback: [{ remote: "203.0.113.5:443", state: "Established", pid: 1000 }] }
          : sample,
      ),
    }),
  );
  assert.equal(failure.ok, false);
  assert.match(failure.errors.join("\n"), /non-loopback connection 203\.0\.113\.5:443/);

  const missingControl = summarizePhase(
    phaseOptions({
      allowedNonLoopback: ["192.0.2.1:80"],
    }),
  );
  assert.equal(missingControl.ok, false);
  assert.match(missingControl.errors.join("\n"), /expected control connection 192\.0\.2\.1:80/);
});

test("an unattached browser layer is reported as not measured, not as a measured zero", () => {
  const phase = summarizePhase(phaseOptions({ browserDelta: null }));
  assert.equal(phase.ok, true, phase.errors.join("; "));
  assert.equal(phase.browserLayerMeasured, false);
  assert.equal(phase.browser, null);
});

test("a browser request in any phase fails the phase", () => {
  const phase = summarizePhase(
    phaseOptions({
      browserDelta: { externalRequests: 1, externalWebsockets: 0, externalNavigations: 0, failedRequests: 0 },
    }),
  );
  assert.equal(phase.ok, false);
  assert.match(phase.errors.join("\n"), /browser request count 1/);
});

test("cleanup attribution protects the workspace daemon, pre-existing and mismatched processes", () => {
  const plan = planProcessCleanup(
    [
      { pid: 1000, executablePath: "C:\\App\\U Workers.exe", creationDate: "2026-10-31T00:00:01.000Z" },
      { pid: 83428, executablePath: "C:\\App\\U Workers.exe", creationDate: "2026-10-31T00:00:01.000Z" },
      { pid: 1200, executablePath: "C:\\App\\U Workers.exe", creationDate: "2026-10-31T00:00:01.000Z" },
      { pid: 1300, executablePath: "D:\\Workspace\\multica.exe", creationDate: "2026-10-31T00:00:01.000Z" },
      { pid: 1400, executablePath: "C:\\App\\U Workers.exe", creationDate: "2026-10-30T00:00:01.000Z" },
    ],
    {
      preExistingPids: [1200],
      protectedPids: [83428],
      runStartedAt: "2026-10-31T00:00:00.000Z",
      expectedExecutablePath: "C:\\App\\U Workers.exe",
    },
  );
  assert.deepEqual(plan.approved, [1000]);
  const reasons = plan.rejected.map((item) => item.pid + ":" + item.reason).join("\n");
  assert.match(reasons, /83428:workspace daemon/);
  assert.match(reasons, /1200:process existed before this run/);
  assert.match(reasons, /1300:executable path/);
  assert.match(reasons, /1400:process started before this run/);
});

test("recursive deletion boundaries reject anything outside the isolated root", () => {
  const root = "C:\\Users\\HuaWei\\uw80-d2\\native";
  assert.equal(assertInsideRoot(root + "\\home\\AppData\\Roaming\\U Workers", root, "userData"), true);
  assert.throws(() => assertInsideRoot("C:\\Users\\HuaWei\\AppData\\Roaming\\U Workers", root, "userData"), ObserverError);
  assert.throws(() => assertInsideRoot(root, root, "root"), ObserverError);
  assert.throws(() => assertInsideRoot(root + "-evil\\x", root, "sibling"), ObserverError);
});
