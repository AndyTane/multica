// @vitest-environment node
import { describe, expect, it } from "vitest";
import { contextCompactionState, isContextCompactionMessage } from "./context-compaction";
import { TaskMessageListSchema } from "../api/schemas";

describe("context compaction activity", () => {
  it("uses sequence order for backfilled and streamed maintenance events", () => {
    expect(contextCompactionState([
      { seq: 5, type: "context_compaction_started" },
      { seq: 1, type: "text" },
    ])).toBe("running");
    expect(contextCompactionState([
      { seq: 6, type: "context_compaction_completed" },
      { seq: 5, type: "context_compaction_started" },
    ])).toBeNull();
  });

  it("clears a stale start when actual task output follows and preserves failures separately", () => {
    expect(contextCompactionState([
      { seq: 1, type: "context_compaction_started" },
      { seq: 2, type: "thinking" },
    ])).toBeNull();
    expect(contextCompactionState([{ seq: 1, type: "context_compaction_failed" }])).toBe("failed");
    expect(contextCompactionState([])).toBeNull();
    expect(isContextCompactionMessage({ type: "context_compaction_future" })).toBe(false);
  });

  it("preserves maintenance lifecycle types while degrading unknown server enums", () => {
    const types = ["context_compaction_started", "context_compaction_completed", "context_compaction_failed"];
    const parsed = TaskMessageListSchema.parse([
      ...types.map((type, seq) => ({ task_id: "task", seq, type })),
      { task_id: "task", type: "future_status" },
    ]);
    expect(parsed.map((message) => message.type)).toEqual([...types, "text"]);
  });
});
