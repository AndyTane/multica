import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { Issue, IssueMetadata } from "../types";
import { issueKeys } from "./queries";

/**
 * The five review metadata keys the server validates, replaces and clears as
 * one group (B7). The snapshot the 5-second undo restores must carry all five
 * — review_kind included — or an undone approval comes back without its type.
 */
export const REVIEW_KEYS = ["review_by", "review_action", "review_user", "review_agent", "review_kind"] as const;

/**
 * What a card waiting on its dispatcher asks for (R4.0-3):
 *
 * - approve — in_review, the deliverer wants a decision.
 * - reply   — blocked, the deliverer asked a question.
 * - act     — blocked, the deliverer needs something done outside the app.
 */
export type AwaitingKind = "approve" | "reply" | "act";

/** Group order of the list: answer a question, do the thing, then approve. */
export const AWAITING_KINDS: readonly AwaitingKind[] = ["reply", "act", "approve"];

/** The two statuses that can hold a review mark (B8/B10). */
const AWAITING_STATUSES = ["in_review", "blocked"] as const;

/**
 * The review mark lives in issue.metadata, a flat KV map. Read it through an
 * optional field so a partial snapshot (legacy payload, cached row from an
 * older backend, or a partial fixture) reads as "no mark" instead of throwing
 * inside a board row.
 */
type ReviewMarkedIssue = Pick<Issue, "status"> & { metadata?: IssueMetadata };

const PAGE_LIMIT = 100;

/**
 * Which of the three review kinds this card is, or null when it is not a
 * well-formed "waiting on the dispatcher" mark.
 *
 * B7 makes status + review_kind decide everything: in_review omits the kind or
 * says approve, blocked must say reply or act. A legacy blocked card without a
 * kind is NOT guessed into a group (B10) — guessing would hand the dispatcher an
 * approval button that writes the wrong status.
 */
export function awaitingKind(issue: ReviewMarkedIssue): AwaitingKind | null {
  const metadata = issue.metadata ?? {};
  if (metadata.review_by !== "user") return null;
  const kind = metadata.review_kind;
  if (issue.status === "in_review") return kind === undefined || kind === "approve" ? "approve" : null;
  if (issue.status === "blocked") return kind === "reply" || kind === "act" ? kind : null;
  return null;
}

/**
 * Whether this card is waiting on meUserId right now (R4.0-1/2, B9, B10).
 *
 * The recipient is the explicit review_user, or the dispatcher when a legacy
 * card never recorded one. A card with no resolvable dispatcher is nobody's:
 * B10 forbids reading a missing/unknown dispatcher as "any member may handle
 * it", so it stays out of every member's list and surfaces through the unmarked
 * window instead.
 */
export function isAwaitingMe(
  issue: ReviewMarkedIssue &
    Pick<Issue, "dispatcher_user_id" | "dispatcher_active">,
  meUserId: string,
): boolean {
  if (!meUserId || !awaitingKind(issue)) return false;
  const dispatcher = typeof issue.dispatcher_user_id === "string" ? issue.dispatcher_user_id : "";
  if (!dispatcher || issue.dispatcher_active === false) return false;
  const reviewUser = (issue.metadata ?? {}).review_user;
  const recipient = typeof reviewUser === "string" ? reviewUser : "";
  return (recipient || dispatcher) === meUserId;
}

/**
 * A card the current member dispatched but nobody can be handed (B10): it has
 * no usable review mark, or the server already says why it cannot be routed.
 * Valid specialist cards and cards with a working recipient never appear here.
 */
export function isAwaitingUnmarked(issue: Issue, meUserId: string): boolean {
  if (!meUserId) return false;
  if (issue.status !== "in_review" && issue.status !== "blocked") return false;
  // The server's reason is authoritative — it also covers orphaned cards an
  // owner/admin alone may see, whose dispatcher no longer resolves.
  if (issue.awaiting_problem) return true;
  if (issue.dispatcher_user_id !== meUserId) return false;
  if ((issue.metadata ?? {}).review_by === "specialist") return false;
  return awaitingKind(issue) === null;
}

export function reviewSnapshot(issue: Pick<Issue, "metadata">): IssueMetadata {
  return Object.fromEntries(REVIEW_KEYS.flatMap((key) =>
    issue.metadata[key] === undefined ? [] : [[key, issue.metadata[key]]],
  ));
}

export function reviewSince(issue: Pick<Issue, "status_changed_at" | "updated_at">): string {
  return issue.status_changed_at ?? issue.updated_at;
}

export function sortAwaiting(issues: Issue[]): Issue[] {
  return [...issues].sort((a, b) => Date.parse(reviewSince(a)) - Date.parse(reviewSince(b)) || a.id.localeCompare(b.id));
}

export interface AwaitingGroup {
  kind: AwaitingKind;
  issues: Issue[];
}

/**
 * Split the complete "waiting on me" result into the visible groups, oldest
 * wait first inside each. Empty groups are dropped so the page never prints a
 * heading with nothing under it.
 */
export function groupAwaiting(issues: Issue[]): AwaitingGroup[] {
  const groups: AwaitingGroup[] = [];
  for (const kind of AWAITING_KINDS) {
    const rows = sortAwaiting(issues.filter((issue) => awaitingKind(issue) === kind));
    if (rows.length > 0) groups.push({ kind, issues: rows });
  }
  return groups;
}

async function loadAwaiting(wsId: string, awaiting: "mine" | "unmarked") {
  const issues = new Map<string, Issue>();
  let offset = 0;
  while (true) {
    const page = await api.listIssues({
      workspace_id: wsId,
      // Bounded even when an older server ignores awaiting: only the two
      // review statuses can hold a mark.
      statuses: [...AWAITING_STATUSES],
      awaiting,
      limit: PAGE_LIMIT,
      offset,
    });
    for (const issue of page.issues) issues.set(issue.id, issue);
    offset += page.issues.length;
    if (offset >= page.total) break;
    if (page.issues.length === 0) throw new Error("Incomplete awaiting list");
  }
  return [...issues.values()];
}

// A single complete result drives both the list and its sidebar count.
export function awaitingIssuesOptions(wsId: string, userId: string) {
  return queryOptions({
    queryKey: [...issueKeys.all(wsId), "awaiting", userId] as const,
    enabled: !!wsId && !!userId,
    queryFn: async () => {
      const issues = await loadAwaiting(wsId, "mine");
      return sortAwaiting(issues.filter((issue) => isAwaitingMe(issue, userId)));
    },
  });
}

// The footer count and its expanded list share this one complete result.
export function awaitingUnmarkedOptions(wsId: string, userId: string) {
  return queryOptions({
    queryKey: [...issueKeys.all(wsId), "awaiting", "unmarked", userId] as const,
    enabled: !!wsId && !!userId,
    queryFn: async () => {
      const issues = await loadAwaiting(wsId, "unmarked");
      return sortAwaiting(issues.filter((issue) => isAwaitingUnmarked(issue, userId)));
    },
  });
}
