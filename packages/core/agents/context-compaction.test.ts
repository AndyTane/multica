// @vitest-environment node
import { describe, expect, it } from "vitest";
import { parseContextCompaction, contextCompactionSchema } from "./context-compaction";

describe("context compaction settings", () => {
  it("uses enabled and 80000 for absent old-server settings", () => {
    expect(parseContextCompaction({})).toEqual({ enabled: true, threshold_tokens: 80000 });
    expect(parseContextCompaction({ context_compaction: { enabled: false } })).toEqual({ enabled: false, threshold_tokens: 80000 });
  });
  it("tolerates malformed responses without trusting unsafe thresholds", () => {
    for (const context_compaction of [null, [], true, { enabled: "yes" }, { threshold_tokens: 0 }, { threshold_tokens: 2000001 }, { threshold_tokens: 1.5 }]) {
      expect(parseContextCompaction({ context_compaction })).toEqual({ enabled: true, threshold_tokens: 80000 });
      expect(contextCompactionSchema.safeParse(context_compaction).success).toBe(false);
    }
  });
  it("accepts a valid policy and ignores future settings", () => {
    expect(parseContextCompaction({ context_compaction: { enabled: true, threshold_tokens: 120000, future: 1 }, unrelated: "kept" })).toEqual({ enabled: true, threshold_tokens: 120000 });
  });
  it("preserves an explicit disabled setting if another response field is malformed", () => {
    expect(parseContextCompaction({ context_compaction: { enabled: false, threshold_tokens: "future" } })).toEqual({ enabled: false, threshold_tokens: 80000 });
  });
});
