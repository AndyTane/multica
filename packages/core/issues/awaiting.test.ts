// @vitest-environment node
import { describe, it, expect, vi, afterEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import type { Issue } from "../types";
import { api } from "../api";
import { awaitingIssuesOptions, isAwaitingMe, reviewSince, reviewSnapshot, sortAwaiting } from "./awaiting";
import { onIssueCreated, onIssueUpdated, onIssueDeleted, onIssueMetadataChanged } from "./ws-updaters";

vi.mock("../api", async (original) => ({ ...await original<typeof import("../api")>(), api: { listIssues: vi.fn(), previewCommentTriggers: vi.fn(), createComment: vi.fn() } }));

function issue(patch: Partial<Issue> = {}): Issue {
  return { id: "i1", workspace_id: "ws", number: 1, identifier: "WS-1", title: "Review", description: null, status: "in_review", priority: "none", assignee_id: null, assignee_type: null, creator_id: "u1", creator_type: "member", parent_issue_id: null, project_id: null, stage: null, start_date: null, due_date: null, position: 0, metadata: { review_by: "user" }, properties: {}, updated_at: "2026-11-01T00:00:00Z", created_at: "2026-10-01T00:00:00Z", revision: 1, ...patch };
}

afterEach(() => vi.clearAllMocks());

describe("awaiting review", () => {
  it.each([
    ["in_review", { review_by: "user" }, "u1", true],
    ["in_review", { review_by: "user", review_user: "u1" }, "u1", true],
    ["in_review", { review_by: "user", review_user: "u2" }, "u1", false],
    ["in_progress", { review_by: "user" }, "u1", false],
    ["custom_review", { review_by: "user" }, "u1", false],
    ["in_review", { review_by: "specialist" }, "u1", false],
    ["in_review", {}, "u1", false],
    ["in_review", { review_by: "user" }, "", false],
  ])("checks %s / %j", (status, metadata, user, expected) => {
    expect(isAwaitingMe({ status: String(status), metadata }, user)).toBe(expected);
  });

  it("uses status entry time, falls back for older servers, and breaks ties by id", () => {
    const old = issue({ id: "a", metadata: { review_since: "2026-10-02T00:00:00Z" } });
    const fallback = issue({ id: "b" });
    const equal = issue({ id: "c", metadata: { review_since: fallback.updated_at } });
    expect(reviewSince(fallback)).toBe(fallback.updated_at);
    for (const review_since of ["invalid", 42, true]) {
      expect(reviewSince(issue({ metadata: { review_since } }))).toBe(fallback.updated_at);
    }
    expect(sortAwaiting([equal, fallback, old]).map((i) => i.id)).toEqual(["a", "b", "c"]);
    expect(reviewSnapshot(issue({ metadata: { review_by: "user", other: true } }))).toEqual({ review_by: "user" });
  });

  it("loads every page before filtering and counting, with tenant and user keys", async () => {
    vi.mocked(api.listIssues).mockResolvedValueOnce({ issues: Array.from({ length: 100 }, (_, n) => issue({ id: `other-${n}`, metadata: { review_by: "user", review_user: "other" } })), total: 102 })
      .mockResolvedValueOnce({ issues: [issue(), issue({ id: "i2" })], total: 102 });
    const qc = new QueryClient();
    expect(await qc.fetchQuery(awaitingIssuesOptions("ws", "u1"))).toHaveLength(2);
    expect(api.listIssues).toHaveBeenNthCalledWith(2, { workspace_id: "ws", status: "in_review", metadata: { review_by: "user" }, limit: 100, offset: 100 });
    expect(awaitingIssuesOptions("ws2", "u1").queryKey).not.toEqual(awaitingIssuesOptions("ws", "u1").queryKey);
    expect(awaitingIssuesOptions("ws", "u2").queryKey).not.toEqual(awaitingIssuesOptions("ws", "u1").queryKey);
    qc.clear();
  });

  it("refreshes the shared result for updates, metadata, deletes and creates", () => {
    const qc = new QueryClient();
    const key = awaitingIssuesOptions("ws", "u1").queryKey;
    for (const event of [() => onIssueCreated(qc, "ws", issue()), () => onIssueUpdated(qc, "ws", issue()), () => onIssueDeleted(qc, "ws", "i1"), () => onIssueMetadataChanged(qc, "ws", "i1", {}, 10)]) {
      qc.setQueryData(key, []);
      event();
      expect(qc.getQueryState(key)?.isInvalidated).toBe(true);
    }
    qc.clear();
  });

});
