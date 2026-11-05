// @vitest-environment node
import { describe, expect, it } from "vitest";
import { DEFAULT_EXECUTION_TIMEOUTS, executionTimeoutsPatch, readExecutionTimeouts, supportsExecutionTimeouts, supportsServerExecutionTimeouts } from "./execution-timeouts";

describe("execution timeout policy boundary", () => {
  it("does not mistake legacy JSON storage for server execution support", () => {
    for (const response of [null, [], {}, { execution_timeouts_supported: "true" }, { settings: executionTimeoutsPatch(DEFAULT_EXECUTION_TIMEOUTS) }]) {
      expect(supportsServerExecutionTimeouts(response)).toBe(false);
    }
    expect(supportsServerExecutionTimeouts({ execution_timeouts_supported: true, future: 1 })).toBe(true);
  });
  it("distinguishes absent, zero, malformed and future fields", () => {
    expect(readExecutionTimeouts({})).toBeNull();
    const p = { ...DEFAULT_EXECUTION_TIMEOUTS, idleSeconds: 0, opencodeIdleSeconds: 0, opencodeShellSeconds: 5400 };
    expect(readExecutionTimeouts(executionTimeoutsPatch(p))).toEqual(p);
    expect(readExecutionTimeouts({ execution_timeouts: { idle_seconds: 0 } })).toBeNull();
    for (const n of [-1, 1.5, "7200", null, 9223372037]) {
      const wire = executionTimeoutsPatch(p).execution_timeouts;
      expect(readExecutionTimeouts({ execution_timeouts: { ...wire, idle_seconds: n } })).toBeNull();
    }
    expect(readExecutionTimeouts({ execution_timeouts: { ...executionTimeoutsPatch(p).execution_timeouts, future: true } })).toEqual(p);
    expect(executionTimeoutsPatch(null)).toEqual({ execution_timeouts: null });
  });
  it("requires an explicit capability", () => {
    expect(supportsExecutionTimeouts({ cli_version: "v99.0.0" })).toBe(false);
    expect(supportsExecutionTimeouts({ capabilities: "execution-timeouts-v1" })).toBe(false);
    expect(supportsExecutionTimeouts({ capabilities: ["execution-timeouts-v1"] })).toBe(true);
  });
});
