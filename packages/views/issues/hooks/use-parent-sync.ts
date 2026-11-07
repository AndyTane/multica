"use client";

import { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  parentSyncToggleState,
  type ParentSyncOwnership,
  type ParentSyncToggleKind,
} from "@multica/core/issues/parent-sync";
import { issueKeys } from "@multica/core/issues/queries";

// The destination of a sub-issue comment's copy changes rarely (parent owner,
// the commenter's permission to wake it) but every composer in a thread asks
// for it, so keep the answer warm for a mount or two. The key names the
// ownership inputs, so an owner change refetches instead of relabelling from a
// stale snapshot.
const PARENT_SYNC_PREVIEW_KEY = "parent-sync";
const PARENT_SYNC_PREVIEW_STALE_MS = 60_000;

const EMPTY_OWNERSHIP: ParentSyncOwnership = {
  parentAssignee: null,
  subIssueAssignee: null,
  currentUserId: null,
};

export interface ParentSyncControl {
  /** Whether the composer renders the toggle at all. */
  visible: boolean;
  kind: ParentSyncToggleKind | null;
  /** Recipient display name for the label, when the server resolved one. */
  name: string | null;
  /** Why the sync is unavailable (`unavailable` only). */
  reason: string | null;
  /** The ordinary routing already reaches the recipient: the box is locked on. */
  locked: boolean;
  /** Cannot be changed: unavailable, or the draft is a `/note`. */
  disabled: boolean;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  /** Clear the user's choice after a successful send. */
  reset: () => void;
  /** Value to send as `sync_parent_assignee` on this submit. */
  syncParentAssignee: boolean;
}

/**
 * The "also hand this comment to the parent issue's owner" toggle
 * (CLAWSTA-151 R4.0-6 / F3).
 *
 * The destination comes from the trigger preview: asking it for
 * `sync_parent_assignee` makes the server resolve the parent's owner, the
 * fallback recipient, and whether the ordinary routing already includes them.
 * The preview is requested with an empty body on purpose — the destination does
 * not depend on the draft, so the label is settled before the user types and
 * does not flicker while they do.
 *
 * `ownership` is the sub-issue/parent ownership snapshot; `null` means this is
 * not a sub-issue (no toggle, no request).
 */
export function useParentSync({
  issueId,
  parentCommentId,
  ownership,
  note,
}: {
  issueId: string;
  parentCommentId?: string;
  ownership: ParentSyncOwnership | null;
  note: boolean;
}): ParentSyncControl {
  const enabled = ownership != null;
  const ownerKey = ownership?.parentAssignee
    ? `${ownership.parentAssignee.type}:${ownership.parentAssignee.id}`
    : "";
  const preview = useQuery({
    queryKey: [
      ...issueKeys.commentTriggerPreview(issueId),
      parentCommentId ?? "",
      PARENT_SYNC_PREVIEW_KEY,
      ownerKey,
      ownership?.currentUserId ?? "",
    ],
    queryFn: () => api.previewCommentTriggers(issueId, "", parentCommentId, undefined, true),
    enabled,
    retry: false,
    staleTime: PARENT_SYNC_PREVIEW_STALE_MS,
  });

  const [checked, setChecked] = useState(false);
  const state = useMemo(
    () =>
      parentSyncToggleState(
        enabled ? preview.data?.parent_sync : undefined,
        ownership ?? EMPTY_OWNERSHIP,
      ),
    [enabled, preview.data, ownership],
  );

  // `/note` never wakes anyone, so a note draft cannot carry the sync flag —
  // the box is disabled and shown unchecked.
  const disabled = state.kind === "unavailable" || note;
  const visible = enabled && state.visible;
  const effectiveChecked = visible && !note && (state.locked || checked);
  const reset = useCallback(() => setChecked(false), []);

  return {
    visible,
    kind: state.kind,
    name: state.name,
    reason: state.reason,
    locked: state.locked,
    disabled,
    checked: effectiveChecked,
    onCheckedChange: setChecked,
    reset,
    syncParentAssignee: effectiveChecked && !disabled,
  };
}
