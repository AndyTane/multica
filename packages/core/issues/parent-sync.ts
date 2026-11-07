import type { CommentParentSync } from "../types";

/**
 * Who owns what, for the "nothing new to reach" hide rules of the sub-issue
 * comment sync toggle (CLAWSTA-151 R4.0-6 / F3). Built by the issue detail from
 * the sub-issue and its direct parent; `null` assignees mean unassigned.
 */
export interface ParentSyncOwnership {
  parentAssignee: { type: string; id: string } | null;
  subIssueAssignee: { type: string; id: string } | null;
  currentUserId: string | null;
}

/**
 * Which label the toggle wears:
 * - `owner`: the parent issue's owner receives the copy.
 * - `recipient`: the parent owner cannot be reached from this commenter, so a
 *   fallback recipient (the parent's dispatcher, or a workspace admin) does.
 * - `unavailable`: nothing can be reached; the toggle is inert and shows why.
 */
export type ParentSyncToggleKind = "owner" | "recipient" | "unavailable";

export interface ParentSyncToggleState {
  visible: boolean;
  kind: ParentSyncToggleKind | null;
  /** Recipient display name, when the server resolved one. */
  name: string | null;
  /** Why the sync cannot be delivered (`unavailable` only). */
  reason: string | null;
  /** The ordinary routing already reaches the recipient, so the box is locked on. */
  locked: boolean;
}

const HIDDEN: ParentSyncToggleState = {
  visible: false,
  kind: null,
  name: null,
  reason: null,
  locked: false,
};

function sameAssignee(
  a: ParentSyncOwnership["parentAssignee"],
  b: ParentSyncOwnership["parentAssignee"],
): boolean {
  return !!a && !!b && a.type === b.type && a.id === b.id;
}

/**
 * Decide whether the sync toggle shows and what it says (CLAWSTA-151 F3).
 *
 * Hidden when the sync would reach nobody new: the parent owner IS the
 * sub-issue owner, or the commenter IS the parent owner. Also hidden when the
 * server did not report a destination (older server) or reported a mode this
 * client cannot name — an unnamed destination is worse than no toggle.
 */
export function parentSyncToggleState(
  parentSync: CommentParentSync | null | undefined,
  ownership: ParentSyncOwnership,
): ParentSyncToggleState {
  if (!parentSync) return HIDDEN;
  if (sameAssignee(ownership.parentAssignee, ownership.subIssueAssignee)) {
    return HIDDEN;
  }
  if (
    ownership.parentAssignee?.type === "member" &&
    ownership.currentUserId != null &&
    ownership.parentAssignee.id === ownership.currentUserId
  ) {
    return HIDDEN;
  }

  const name = parentSync.recipients[0]?.name.trim() || null;
  const locked = parentSync.already_routed === true;

  switch (parentSync.mode) {
    case "agent":
    case "member":
      return { visible: true, kind: "owner", name, reason: null, locked };
    case "dispatcher":
    case "admin":
      // "Sync to {recipient}" needs a name; without one the user cannot tell
      // who is being handed the copy, so do not offer it.
      return name
        ? { visible: true, kind: "recipient", name, reason: null, locked }
        : HIDDEN;
    case "unavailable":
      return {
        visible: true,
        kind: "unavailable",
        name: null,
        reason: parentSync.reason?.trim() || null,
        locked: false,
      };
    default:
      return HIDDEN;
  }
}

// `delivery_status` values that mean the copy did not reach the recipient. The
// contract (B11) reuses the queue vocabulary for success, so this list stays
// failure-only: an unknown future value must not turn a delivered comment into
// a scary toast.
const FAILED_DELIVERY_STATUSES = new Set([
  "failed",
  "blocked",
  "undelivered",
  "not_delivered",
  "unavailable",
  "error",
]);

export interface ParentSyncDeliveryWarning {
  reason: string | null;
}

/**
 * Whether the create response says the comment saved but its parent-sync copy
 * did not land (B11: partial failure returns the comment id plus the actual
 * destination). The message names the reason when the server gave one; the
 * body is already posted and must never be resent.
 */
export function parentSyncDeliveryWarning(
  parentSync: CommentParentSync | null | undefined,
): ParentSyncDeliveryWarning | null {
  if (!parentSync?.delivery_status) return null;
  const status = parentSync.delivery_status.trim().toLowerCase();
  if (!FAILED_DELIVERY_STATUSES.has(status)) return null;
  return { reason: parentSync.reason?.trim() || null };
}
