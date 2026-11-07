export type CommentType = "comment" | "status_change" | "progress_update" | "system";

// `system` is used by platform-generated rows (e.g. the parent-issue
// child-done notification, MUL-2538). System rows carry a zero UUID for
// author_id; render paths should branch on author_type rather than the UUID.
export type CommentAuthorType = "member" | "agent" | "system";

export interface Reaction {
  id: string;
  comment_id: string;
  actor_type: string;
  actor_id: string;
  emoji: string;
  created_at: string;
  comment_revision?: number;
}

export interface Comment {
  id: string;
  issue_id: string;
  author_type: CommentAuthorType;
  author_id: string;
  content: string;
  type: CommentType;
  parent_id: string | null;
  reactions: Reaction[];
  attachments: import("./attachment").Attachment[];
  created_at: string;
  updated_at: string;
  /** Monotonic server revision; absent when connected to an older backend. */
  revision?: number;
  /** Parent issue revision after this semantic comment mutation. */
  issue_revision?: number;
  resolved_at: string | null;
  resolved_by_type: CommentAuthorType | null;
  resolved_by_id: string | null;
  source_task_id?: string | null;
  // The quick action that produced this comment (MUL-5465). A quick action
  // posts an ORDINARY comment and marks it with this id; the collapsed card
  // keys off the id rather than a dedicated `type`, because `type` is
  // client-supplied on the generic comment endpoint and would be forgeable.
  quick_action_id?: string | null;
  // Set only on a comment deleted while it still had replies (#8296): the
  // server keeps an empty tombstone so the replies keep their parent. Older
  // servers omit it.
  deleted_at?: string | null;
  // Per-target result of every explicit @agent / @squad mention in this comment
  // (MUL-4525 §2). Present only on create/edit responses; older servers omit it.
  trigger_outcomes?: CommentTriggerOutcome[];
  /**
   * Where a `sync_parent_assignee` copy of this comment landed (CLAWSTA-151 R4,
   * B11). Present only on a sync-attempted comment; older servers omit it.
   */
  parent_sync?: ParentSync | null;
  /** Every running turn this comment steered, one receipt per run. */
  supplements?: CommentSupplementReceipt[];
  /** Mirrors the first receipt; servers that predate `supplements` send only these. */
  supplement_task_id?: string;
  supplement_status?: "pending" | "delivering" | "delivered" | "failed";
  supplement_failure_reason?: string;
  supplement_delivered_at?: string;
}

export type CommentSupplementStatus = "pending" | "delivering" | "delivered" | "failed";

/** Delivery receipt of a comment that steered one agent's running turn. */
export interface CommentSupplementReceipt {
  task_id: string;
  agent_id?: string;
  status: CommentSupplementStatus;
  failure_reason?: string;
  delivered_at?: string;
}

// The domain result of one explicitly-mentioned trigger target. Success-shaped
// statuses (queued/coalesced/deferred/steered) mean the mention was handled;
// `steered` means the comment went into the target's running turn. `blocked`
// means it was refused with an enumeration-safe reason_code.
export type CommentTriggerStatus =
  | "queued"
  | "coalesced"
  | "deferred"
  | "steered"
  | "blocked";

export interface CommentTriggerOutcome {
  target_type: string; // "agent" | "squad"
  target_id: string;
  status: CommentTriggerStatus | string;
  reason_code: string;
}

export type CommentTriggerSource =
  | "issue_assignee"
  | "mention_agent"
  | "mention_squad_leader";

export interface CommentTriggerPreviewAgent {
  id: string;
  name: string;
  avatar_url?: string;
  source: CommentTriggerSource | string;
  squad_id?: string;
  reason: string;
}

export interface CommentTriggerPreview {
  agents: CommentTriggerPreviewAgent[];
  // Explicit @agent / @squad mentions that will NOT trigger if posted as-is
  // (MUL-4525 §2). Additive: older servers omit it.
  blocked?: CommentTriggerOutcome[];
  // Where a comment posted with `sync_parent_assignee` would land (CLAWSTA-151
  // R4, B11). Present only when the request asked for the sync; older servers
  // omit it and the composer hides the toggle.
  parent_sync?: ParentSync;
}

// How a sub-issue comment's copy reaches its parent issue's owner (CLAWSTA-151
// R4, B11). `agent`/`member` = the parent owner itself; `dispatcher`/`admin`
// = the fallback recipient when the commenter cannot wake the owner;
// `unavailable` = no reachable recipient, so the toggle is inert.
export type ParentSyncMode = "agent" | "member" | "dispatcher" | "admin" | "unavailable";

/** A resolved recipient of a parent-sync delivery, as the server reports it. */
export interface ParentSyncRecipient {
  type: string;
  id: string;
  name: string;
  avatar_url?: string;
}

export interface ParentSync {
  parent_issue_id?: string;
  // Unknown future modes must not break parsing; consumers fall back to
  // hiding the toggle rather than guessing a destination.
  mode: ParentSyncMode | string;
  recipients: ParentSyncRecipient[];
  // True when the ordinary comment routing already reaches the parent owner,
  // so the toggle is locked checked rather than adding a second delivery.
  already_routed: boolean;
  reason?: string;
  // Create responses only: the actual outcome of the sync delivery.
  delivery_status?: string;
}
