// CLAWSTA-151 R4 (B9–B14) schema compatibility: every field these batches add
// is additive, and a client talking to an older server must still parse the
// response with the field absent. The zod contracts are also the place a
// malformed additive value must degrade to "absent" instead of failing the
// whole payload.
import { describe, expect, it } from "vitest";
import {
  CommentSchema,
  CommentTriggerPreviewSchema,
  InboxItemListSchema,
  IssueSchema,
} from "./schemas";

const baseIssue = {
  id: "issue-1",
  workspace_id: "ws-1",
  number: 1,
  identifier: "MUL-1",
  title: "Card",
  description: null,
  status: "in_review",
  priority: "none",
  assignee_type: "agent",
  assignee_id: "agent-1",
  creator_type: "member",
  creator_id: "user-1",
  parent_issue_id: null,
  project_id: null,
  position: 0,
  start_date: null,
  due_date: null,
  metadata: {},
  properties: {},
  created_at: "2026-11-07T00:00:00Z",
  updated_at: "2026-11-07T00:00:00Z",
};

describe("R4 issue dispatcher fields (B9/B10)", () => {
  it("parses an issue from an older server without the fields", () => {
    const parsed = IssueSchema.parse(baseIssue);
    expect(parsed.dispatcher_user_id).toBeUndefined();
    expect(parsed.dispatcher_active).toBeUndefined();
    expect(parsed.awaiting_problem).toBeUndefined();
  });

  it("keeps the resolved dispatcher and awaiting reason", () => {
    const parsed = IssueSchema.parse({
      ...baseIssue,
      dispatcher_user_id: "user-2",
      dispatcher_active: false,
      awaiting_problem: "dispatcher_inactive",
    });
    expect(parsed.dispatcher_user_id).toBe("user-2");
    expect(parsed.dispatcher_active).toBe(false);
    expect(parsed.awaiting_problem).toBe("dispatcher_inactive");
  });

  it("degrades a malformed additive field to absent, not a failed payload", () => {
    const parsed = IssueSchema.parse({
      ...baseIssue,
      dispatcher_user_id: 42,
      dispatcher_active: "yes",
    });
    expect(parsed.id).toBe("issue-1");
    expect(parsed.dispatcher_user_id).toBeUndefined();
    expect(parsed.dispatcher_active).toBeUndefined();
  });
});

const baseComment = {
  id: "comment-1",
  issue_id: "issue-1",
  author_type: "member",
  author_id: "user-1",
  content: "hello",
  type: "comment",
  parent_id: null,
  created_at: "2026-11-07T00:00:00Z",
  updated_at: "2026-11-07T00:00:00Z",
};

describe("R4 parent-sync comment fields (B11)", () => {
  it("parses an ordinary comment with no parent_sync", () => {
    expect(CommentSchema.parse(baseComment).parent_sync).toBeUndefined();
  });

  it("keeps the server-decided delivery result", () => {
    const parsed = CommentSchema.parse({
      ...baseComment,
      parent_sync: {
        parent_issue_id: "parent-1",
        mode: "agent",
        recipients: [{ type: "agent", id: "agent-1", name: "Reviewer" }],
        already_routed: false,
        delivery_status: "queued",
      },
    });
    expect(parsed.parent_sync?.mode).toBe("agent");
    expect(parsed.parent_sync?.delivery_status).toBe("queued");
    expect(parsed.parent_sync?.recipients).toHaveLength(1);
  });

  it("carries the preview mode and falls back when the server omits it", () => {
    expect(CommentTriggerPreviewSchema.parse({ agents: [] }).parent_sync).toBeUndefined();
    const parsed = CommentTriggerPreviewSchema.parse({
      agents: [],
      parent_sync: {
        parent_issue_id: "parent-1",
        mode: "unavailable",
        recipients: [],
        already_routed: false,
        reason: "no_fallback_recipient",
      },
    });
    expect(parsed.parent_sync?.mode).toBe("unavailable");
    expect(parsed.parent_sync?.reason).toBe("no_fallback_recipient");
  });
});

describe("R4 inbox parent projection (B14)", () => {
  const baseItem = {
    id: "item-1",
    workspace_id: "ws-1",
    recipient_type: "member",
    recipient_id: "user-1",
    type: "new_comment",
    severity: "info",
    issue_id: "issue-2",
    title: "Activity",
    body: "…",
    read: false,
    archived: false,
    created_at: "2026-11-07T00:00:00Z",
  };

  it("parses an older server's row with no parent fields", () => {
    const [item] = InboxItemListSchema.parse([baseItem]);
    expect(item?.parent_issue_identifier).toBeUndefined();
  });

  it("keeps the parent identifier and title", () => {
    const [item] = InboxItemListSchema.parse([
      {
        ...baseItem,
        parent_issue_id: "issue-1",
        parent_issue_identifier: "CLAWSTA-151",
        parent_issue_title: "Parent card",
      },
    ]);
    expect(item?.parent_issue_identifier).toBe("CLAWSTA-151");
    expect(item?.parent_issue_title).toBe("Parent card");
  });
});
