import { describe, expect, it } from "vitest";
import { ChatPendingTaskSchema, SendChatMessageResponseSchema, TaskMessageListSchema } from "./schemas";

it("preserves compaction lifecycle messages and tolerates unknown message types", () => {
  const types = ["context_compaction_started", "context_compaction_completed", "context_compaction_failed"];
  expect(TaskMessageListSchema.parse([...types, "future_status"].map((type, seq) => ({ task_id: "run", type, seq })))
    .map((message) => message.type)).toEqual([...types, "text"]);
});

describe("ChatPendingTaskSchema", () => {
  it("keeps the pending head and valid queue rows when one row is malformed", () => {
    const parsed = ChatPendingTaskSchema.parse({
      task_id: "task-active",
      status: "running",
      queued_tasks: [
        {
          task_id: "task-next",
          status: "queued",
          created_at: "2026-08-03T00:00:00Z",
        },
        { status: "queued" },
      ],
    });

    expect(parsed.task_id).toBe("task-active");
    expect(parsed.queued_tasks).toEqual([
      {
        task_id: "task-next",
        status: "queued",
        created_at: "2026-08-03T00:00:00Z",
      },
    ]);
  });
});

describe("SendChatMessageResponseSchema", () => {
  const base = {
    message_id: "message-1",
    task_id: "task-1",
    created_at: "2026-08-05T00:00:00Z",
  };

  it("keeps queue position parity with web", () => {
    expect(SendChatMessageResponseSchema.parse({ ...base, queued: false }).queued).toBe(false);
  });

  it("ignores a malformed additive queue position", () => {
    expect(SendChatMessageResponseSchema.parse({ ...base, queued: "no" }).queued).toBeUndefined();
  });
});
