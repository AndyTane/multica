import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { Issue, IssueMetadata } from "../types";
import { issueKeys } from "./queries";

export const REVIEW_KEYS = ["review_by", "review_action", "review_user", "review_agent"] as const;

export function isAwaitingMe(issue: Pick<Issue, "status" | "metadata">, meUserId: string): boolean {
  return !!meUserId && issue.status === "in_review" && issue.metadata.review_by === "user"
    && (issue.metadata.review_user === undefined || issue.metadata.review_user === meUserId);
}

export function reviewSnapshot(issue: Pick<Issue, "metadata">): IssueMetadata {
  return Object.fromEntries(REVIEW_KEYS.flatMap((key) =>
    issue.metadata[key] === undefined ? [] : [[key, issue.metadata[key]]],
  ));
}

export function reviewSince(issue: Pick<Issue, "metadata" | "updated_at">): string {
  const since = issue.metadata.review_since;
  return typeof since === "string" && Number.isFinite(Date.parse(since)) ? since : issue.updated_at;
}

export function sortAwaiting(issues: Issue[]): Issue[] {
  return [...issues].sort((a, b) => Date.parse(reviewSince(a)) - Date.parse(reviewSince(b)) || a.id.localeCompare(b.id));
}

// A single complete result drives both the list and its sidebar count.
export function awaitingIssuesOptions(wsId: string, userId: string) {
  return queryOptions({
    queryKey: [...issueKeys.all(wsId), "awaiting", userId] as const,
    enabled: !!wsId && !!userId,
    queryFn: async () => {
      const issues = new Map<string, Issue>();
      let offset = 0;
      while (true) {
        const page = await api.listIssues({ workspace_id: wsId, status: "in_review", metadata: { review_by: "user" }, limit: 100, offset });
        for (const issue of page.issues) issues.set(issue.id, issue);
        offset += page.issues.length;
        if (offset >= page.total) break;
        if (page.issues.length === 0) throw new Error("Incomplete review list");
      }
      return sortAwaiting([...issues.values()].filter((issue) => isAwaitingMe(issue, userId)));
    },
  });
}
