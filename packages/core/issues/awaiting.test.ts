// @vitest-environment node
import { describe, it, expect, vi, afterEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import type { Issue } from "../types";
import { api } from "../api";
import {
  AWAITING_KINDS,
  awaitingIssuesOptions,
  awaitingKind,
  awaitingUnmarkedOptions,
  groupAwaiting,
  isAwaitingMe,
  isAwaitingUnmarked,
  reviewSince,
  reviewSnapshot,
  sortAwaiting,
} from "./awaiting";
import { onIssueCreated, onIssueUpdated, onIssueDeleted, onIssueMetadataChanged } from "./ws-updaters";

vi.mock("../api", async (original) => ({ ...await original<typeof import("../api")>(), api: { listIssues: vi.fn(), previewCommentTriggers: vi.fn(), createComment: vi.fn() } }));

function issue(patch: Partial<Issue> = {}): Issue {
  return { id: "i1", workspace_id: "ws", number: 1, identifier: "WS-1", title: "Review", description: null, status: "in_review", priority: "none", assignee_id: null, assignee_type: null, creator_id: "u1", creator_type: "member", parent_issue_id: null, project_id: null, stage: null, start_date: null, due_date: null, position: 0, metadata: { review_by: "user" }, properties: {}, updated_at: "2026-11-01T00:00:00Z", created_at: "2026-10-01T00:00:00Z", revision: 1, dispatcher_user_id: "u1", dispatcher_active: true, ...patch };
}

afterEach(() => vi.clearAllMocks());

describe("awaiting kind", () => {
  it.each([
    ["in_review", { review_by: "user" }, "approve"],
    ["in_review", { review_by: "user", review_kind: "approve" }, "approve"],
    ["in_review", { review_by: "user", review_kind: "reply" }, null],
    ["in_review", { review_by: "user", review_kind: "act" }, null],
    ["in_review", { review_by: "user", review_kind: "nonsense" }, null],
    ["blocked", { review_by: "user", review_kind: "reply" }, "reply"],
    ["blocked", { review_by: "user", review_kind: "act" }, "act"],
    ["blocked", { review_by: "user" }, null],
    ["blocked", { review_by: "user", review_kind: "approve" }, null],
    ["in_review", { review_by: "specialist" }, null],
    ["blocked", { review_by: "specialist", review_kind: "reply" }, null],
    ["in_progress", { review_by: "user" }, null],
  ])("maps %s / %j", (status, metadata, expected) => {
    expect(awaitingKind({ status: String(status), metadata })).toBe(expected);
  });

  // The list order is the product decision, not an implementation detail:
  // answer a question first, then do the off-system action, then approve.
  it("orders the groups reply, act, approve", () => {
    expect(AWAITING_KINDS).toEqual(["reply", "act", "approve"]);
  });
});

describe("isAwaitingMe", () => {
  it.each([
    ["in_review", { review_by: "user" }, "u1", true, "u1", true],
    ["in_review", { review_by: "user", review_kind: "approve", review_user: "u1" }, "u1", true, "u1", true],
    ["blocked", { review_by: "user", review_kind: "reply" }, "u1", true, "u1", true],
    ["blocked", { review_by: "user", review_kind: "act" }, "u1", true, "u1", true],
    ["in_review", { review_by: "user", review_user: "u2" }, "u1", true, "u1", false],
    ["in_review", { review_by: "user", review_user: "u2" }, "u1", true, "u2", true],
    ["blocked", { review_by: "user" }, "u1", true, "u1", false],
    ["in_review", { review_by: "user" }, null, undefined, "u1", false],
    ["in_review", { review_by: "user" }, "u1", false, "u1", false],
    ["in_review", { review_by: "specialist" }, "u1", true, "u1", false],
    ["in_progress", { review_by: "user" }, "u1", true, "u1", false],
    ["custom_review", { review_by: "user" }, "u1", true, "u1", false],
    ["in_review", { review_by: "user" }, "u1", true, "", false],
  ])("status %s / %j / dispatcher %s (%s) for %s", (status, metadata, dispatcher, active, me, expected) => {
    expect(isAwaitingMe({ status: String(status), metadata, dispatcher_user_id: dispatcher as string | null, dispatcher_active: active as boolean | undefined }, String(me))).toBe(expected);
  });
});

describe("isAwaitingUnmarked", () => {
  it("keeps cards this member dispatched that cannot be routed, including orphans the server flagged", () => {
    expect(isAwaitingUnmarked(issue({ status: "blocked", metadata: { review_by: "user" } }), "u1")).toBe(true);
    expect(isAwaitingUnmarked(issue({ metadata: {} }), "u1")).toBe(true);
    expect(isAwaitingUnmarked(issue({ metadata: { review_by: "specialist" } }), "u1")).toBe(false);
    expect(isAwaitingUnmarked(issue({ metadata: { review_by: "user" } }), "u1")).toBe(false);
    expect(isAwaitingUnmarked(issue({ metadata: {}, dispatcher_user_id: "u2" }), "u1")).toBe(false);
    expect(isAwaitingUnmarked(issue({ metadata: {}, dispatcher_user_id: null, awaiting_problem: "dispatcher_left" }), "u1")).toBe(true);
    expect(isAwaitingUnmarked(issue({ status: "done", metadata: {} }), "u1")).toBe(false);
    expect(isAwaitingUnmarked(issue({ metadata: {} }), "")).toBe(false);
  });
});

describe("grouping and ordering", () => {
  it("groups reply, act, approve, keeps the oldest wait first and hides empty groups", () => {
    const rows = [
      issue({ id: "approve-1", metadata: { review_by: "user", review_kind: "approve", review_since: "2026-10-01T00:00:00Z" } }),
      issue({ id: "reply-late", status: "blocked", metadata: { review_by: "user", review_kind: "reply", review_since: "2026-10-04T00:00:00Z" } }),
      issue({ id: "reply-early", status: "blocked", metadata: { review_by: "user", review_kind: "reply", review_since: "2026-10-02T00:00:00Z" } }),
      issue({ id: "act-1", status: "blocked", metadata: { review_by: "user", review_kind: "act" }, updated_at: "2026-10-03T00:00:00Z" }),
    ];
    expect(groupAwaiting(rows).map((group) => [group.kind, group.issues.map((row) => row.id)])).toEqual([
      ["reply", ["reply-early", "reply-late"]],
      ["act", ["act-1"]],
      ["approve", ["approve-1"]],
    ]);
    expect(groupAwaiting([rows[0]!]).map((group) => group.kind)).toEqual(["approve"]);
    expect(groupAwaiting([])).toEqual([]);
  });

  it("breaks equal review times by issue id so the order is stable", () => {
    const same = "2026-10-02T00:00:00Z";
    const rows = [issue({ id: "b", metadata: { review_by: "user", review_since: same } }), issue({ id: "a", metadata: { review_by: "user", review_since: same } })];
    expect(sortAwaiting(rows).map((row) => row.id)).toEqual(["a", "b"]);
    expect(groupAwaiting(rows)[0]!.issues.map((row) => row.id)).toEqual(["a", "b"]);
  });
});

describe("review snapshot and time", () => {
  it("uses review_since, falls back for older servers, and snapshots all five keys", () => {
    const fallback = issue({ id: "b" });
    expect(reviewSince(fallback)).toBe(fallback.updated_at);
    expect(reviewSince(issue({ metadata: { review_since: "2026-10-02T00:00:00Z" } }))).toBe("2026-10-02T00:00:00Z");
    for (const review_since of ["invalid", 42, true]) {
      expect(reviewSince(issue({ metadata: { review_since } }))).toBe(fallback.updated_at);
    }
    expect(reviewSnapshot(issue({ metadata: { review_by: "user", review_kind: "act", review_action: "Install", review_user: "u1", other: true } })))
      .toEqual({ review_by: "user", review_kind: "act", review_action: "Install", review_user: "u1" });
    expect(reviewSnapshot(issue({ metadata: {} }))).toEqual({});
  });
});

describe("awaiting queries", () => {
  it("loads every page of the mine window before filtering, with tenant and user keys", async () => {
    const mine = issue({ id: "mine", status: "blocked", metadata: { review_by: "user", review_kind: "reply" } });
    vi.mocked(api.listIssues)
      .mockResolvedValueOnce({ issues: Array.from({ length: 100 }, (_, n) => issue({ id: `other-${n}`, metadata: { review_by: "user", review_user: "other" } })), total: 101 })
      .mockResolvedValueOnce({ issues: [mine, issue({ id: "theirs", metadata: { review_by: "user", review_user: "other" } })], total: 101 });
    const qc = new QueryClient();
    expect((await qc.fetchQuery(awaitingIssuesOptions("ws", "u1"))).map((row) => row.id)).toEqual(["mine"]);
    expect(api.listIssues).toHaveBeenNthCalledWith(2, { workspace_id: "ws", statuses: ["in_review", "blocked"], awaiting: "mine", limit: 100, offset: 100 });
    expect(awaitingIssuesOptions("ws2", "u1").queryKey).not.toEqual(awaitingIssuesOptions("ws", "u1").queryKey);
    expect(awaitingIssuesOptions("ws", "u2").queryKey).not.toEqual(awaitingIssuesOptions("ws", "u1").queryKey);
    qc.clear();
  });

  it("loads the unmarked window and keeps only the cards nobody can be handed", async () => {
    vi.mocked(api.listIssues).mockResolvedValueOnce({
      issues: [
        issue({ id: "legacy-blocked", status: "blocked", metadata: { review_by: "user" } }),
        issue({ id: "healthy", metadata: { review_by: "user" } }),
        issue({ id: "someone-else", metadata: {}, dispatcher_user_id: "u2" }),
        issue({ id: "orphan", metadata: { review_by: "user" }, dispatcher_user_id: null, awaiting_problem: "dispatcher_left" }),
      ],
      total: 4,
    });
    const qc = new QueryClient();
    expect((await qc.fetchQuery(awaitingUnmarkedOptions("ws", "u1"))).map((row) => row.id)).toEqual(["legacy-blocked", "orphan"]);
    expect(api.listIssues).toHaveBeenCalledWith({ workspace_id: "ws", statuses: ["in_review", "blocked"], awaiting: "unmarked", limit: 100, offset: 0 });
    qc.clear();
  });

  it("refreshes the shared result for updates, metadata, deletes and creates", () => {
    const qc = new QueryClient();
    const keys = [awaitingIssuesOptions("ws", "u1").queryKey, awaitingUnmarkedOptions("ws", "u1").queryKey];
    for (const key of keys) {
      for (const event of [() => onIssueCreated(qc, "ws", issue()), () => onIssueUpdated(qc, "ws", issue()), () => onIssueDeleted(qc, "ws", "i1"), () => onIssueMetadataChanged(qc, "ws", "i1", {}, 10)]) {
        qc.setQueryData(key, []);
        event();
        expect(qc.getQueryState(key)?.isInvalidated).toBe(true);
      }
    }
    qc.clear();
  });
});
