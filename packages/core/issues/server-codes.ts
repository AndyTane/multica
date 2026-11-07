/**
 * Stable machine codes the R4 review surfaces receive from the server (B10 /
 * B11 / B13).
 *
 * The UI never prints one: every known code names a localized sentence, and a
 * code added by a newer server degrades to that category's generic sentence
 * instead of leaking the identifier. Keep the lists here so the copy sets in
 * the five locales and the tests share one source of truth.
 */

/** Why the server cannot hand a dispatched card to any member (B10). */
export const AWAITING_PROBLEM_CODES = [
  "no_dispatcher",
  "dispatcher_inactive",
  "receiver_inactive",
  "invalid_review_mark",
] as const;

export type AwaitingProblemCode = (typeof AWAITING_PROBLEM_CODES)[number];
export type AwaitingProblemMessageKey = AwaitingProblemCode | "unknown";

const AWAITING_PROBLEM_SET: ReadonlySet<string> = new Set(AWAITING_PROBLEM_CODES);

export function awaitingProblemMessageKey(
  code: string | null | undefined,
): AwaitingProblemMessageKey {
  const normalized = (code ?? "").trim();
  return AWAITING_PROBLEM_SET.has(normalized)
    ? (normalized as AwaitingProblemCode)
    : "unknown";
}

/**
 * Why a synced sub-issue comment did not reach the parent card's owner (B11):
 * the delivery outcomes, plus the reasons a sync preview reports as
 * `unavailable`.
 */
export const PARENT_SYNC_REASON_CODES = [
  "duplicate_pending_task",
  "attribution_blocked",
  "internal_error",
  "enqueue_failed",
  "inbox_failed",
  "issue_has_no_parent",
  "parent_issue_missing",
  "no_fallback_recipient",
] as const;

export type ParentSyncReasonCode = (typeof PARENT_SYNC_REASON_CODES)[number];
export type ParentSyncReasonMessageKey = ParentSyncReasonCode | "unknown";

const PARENT_SYNC_REASON_SET: ReadonlySet<string> = new Set(
  PARENT_SYNC_REASON_CODES,
);

export function parentSyncReasonMessageKey(
  code: string | null | undefined,
): ParentSyncReasonMessageKey {
  const normalized = (code ?? "").trim();
  return PARENT_SYNC_REASON_SET.has(normalized)
    ? (normalized as ParentSyncReasonCode)
    : "unknown";
}

/** Why the assignment-preview preflight refuses or warns (B13). */
export const ASSIGN_PREVIEW_REASON_CODES = [
  "actor_cannot_assign",
  "dispatcher_cannot_invoke",
  "dispatcher_inactive",
] as const;

export type AssignPreviewReasonCode = (typeof ASSIGN_PREVIEW_REASON_CODES)[number];
export type AssignPreviewReasonMessageKey = AssignPreviewReasonCode | "unknown";

const ASSIGN_PREVIEW_REASON_SET: ReadonlySet<string> = new Set(
  ASSIGN_PREVIEW_REASON_CODES,
);

export function assignPreviewReasonMessageKey(
  code: string | null | undefined,
): AssignPreviewReasonMessageKey {
  const normalized = (code ?? "").trim();
  return ASSIGN_PREVIEW_REASON_SET.has(normalized)
    ? (normalized as AssignPreviewReasonCode)
    : "unknown";
}
