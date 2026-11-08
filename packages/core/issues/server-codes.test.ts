// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  ASSIGN_PREVIEW_REASON_CODES,
  AWAITING_PROBLEM_CODES,
  PARENT_SYNC_REASON_CODES,
  assignPreviewReasonMessageKey,
  awaitingProblemMessageKey,
  parentSyncReasonMessageKey,
} from "./server-codes";

describe("server machine codes name a locale key, never themselves", () => {
  it("keeps every awaiting problem code addressable", () => {
    for (const code of AWAITING_PROBLEM_CODES) {
      expect(awaitingProblemMessageKey(code)).toBe(code);
    }
  });

  it("degrades an unknown or missing awaiting problem to the generic key", () => {
    expect(awaitingProblemMessageKey("no_dispatcher_v2")).toBe("unknown");
    expect(awaitingProblemMessageKey("")).toBe("unknown");
    expect(awaitingProblemMessageKey(null)).toBe("unknown");
    expect(awaitingProblemMessageKey(undefined)).toBe("unknown");
  });

  it("keeps every parent-sync reason addressable", () => {
    for (const code of PARENT_SYNC_REASON_CODES) {
      expect(parentSyncReasonMessageKey(code)).toBe(code);
    }
  });

  it("degrades an unknown parent-sync reason instead of printing it", () => {
    expect(parentSyncReasonMessageKey("queue_melted")).toBe("unknown");
    expect(parentSyncReasonMessageKey("  ")).toBe("unknown");
  });

  it("keeps every assignment-preview reason addressable", () => {
    for (const code of ASSIGN_PREVIEW_REASON_CODES) {
      expect(assignPreviewReasonMessageKey(code)).toBe(code);
    }
  });

  it("degrades an unknown assignment-preview reason", () => {
    expect(assignPreviewReasonMessageKey("actor_cannot_assign_v2")).toBe("unknown");
    expect(assignPreviewReasonMessageKey(null)).toBe("unknown");
  });
});
