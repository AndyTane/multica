package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// "Sync this sub-issue comment to the parent card's assignee" (CLAWSTA-151
// B11). The parent is the DIRECT parent_issue_id — never a grandparent — and the
// synced delivery is additive to the normal comment routing, not a replacement:
// the child's own owner still runs, and the parent's assignee merely reads.
const (
	parentSyncModeAgent       = "agent"
	parentSyncModeMember      = "member"
	parentSyncModeDispatcher  = "dispatcher"
	parentSyncModeAdmin       = "admin"
	parentSyncModeUnavailable = "unavailable"

	parentSyncDeliveryQueued   = "queued"
	parentSyncDeliveryDeferred = "deferred"
	parentSyncDeliveryNotified = "notified"
	parentSyncDeliveryAlready  = "already_routed"
	parentSyncDeliveryBlocked  = "blocked"

	parentSyncInboxType = "parent_sync_comment"
)

// CommentParentSyncRecipient identifies one resolved recipient of the sync.
type CommentParentSyncRecipient struct {
	Type string `json:"type"` // agent | squad | member
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CommentParentSyncResponse is the preview / result of the parent-sync switch.
// The client only ever sends the boolean; mode, recipients and reason are
// server-decided. delivery_status appears on the created comment only.
type CommentParentSyncResponse struct {
	ParentIssueID  string                       `json:"parent_issue_id"`
	Mode           string                       `json:"mode"`
	Recipients     []CommentParentSyncRecipient `json:"recipients"`
	AlreadyRouted  bool                         `json:"already_routed"`
	DeliveryStatus string                       `json:"delivery_status,omitempty"`
	Reason         string                       `json:"reason,omitempty"`
}

// parentSyncPlan is the resolved route, computed before any write so a preview
// and a create agree and a create can reject before saving the comment.
type parentSyncPlan struct {
	response        CommentParentSyncResponse
	targetAgent     db.Agent
	targetIsLeader  bool
	targetSquadID   pgtype.UUID
	inboxRecipients []pgtype.UUID
	needsDelivery   bool
}

// parentSyncPersisted is what lands in comment.parent_sync. Only the server
// writes it; it names the resolved target and the final outcome so a replay can
// dedupe and the UI can explain a downgrade.
type parentSyncPersisted struct {
	ParentIssueID  string                       `json:"parent_issue_id"`
	Mode           string                       `json:"mode"`
	Recipients     []CommentParentSyncRecipient `json:"recipients"`
	AlreadyRouted  bool                         `json:"already_routed"`
	DeliveryStatus string                       `json:"delivery_status,omitempty"`
	Reason         string                       `json:"reason,omitempty"`
}

func (p parentSyncPlan) persisted(status string) parentSyncPersisted {
	return parentSyncPersisted{
		ParentIssueID:  p.response.ParentIssueID,
		Mode:           p.response.Mode,
		Recipients:     p.response.Recipients,
		AlreadyRouted:  p.response.AlreadyRouted,
		DeliveryStatus: status,
		Reason:         p.response.Reason,
	}
}

func (p parentSyncPlan) responseWith(status string) CommentParentSyncResponse {
	out := p.response
	out.DeliveryStatus = status
	return out
}

// parentSyncUnavailable returns a plan that cannot deliver; callers reject a
// true submission BEFORE writing the comment (B11: the user can cancel the
// switch and post a normal comment).
func parentSyncUnavailable(parentIssueID, reason string) parentSyncPlan {
	return parentSyncPlan{response: CommentParentSyncResponse{
		ParentIssueID: parentIssueID,
		Mode:          parentSyncModeUnavailable,
		Recipients:    []CommentParentSyncRecipient{},
		Reason:        reason,
	}}
}

// resolveParentSyncPlan computes where a synced comment would go. It performs no
// writes and is not itself an authorization: the create path re-checks the queue
// admission and the parent relation at delivery time.
func (h *Handler) resolveParentSyncPlan(ctx context.Context, issue db.Issue, actorType, actorID, originatorUserID, content string, routedAgents map[string]struct{}) (parentSyncPlan, error) {
	if !issue.ParentIssueID.Valid {
		return parentSyncUnavailable("", "issue_has_no_parent"), nil
	}
	parent, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{
		ID:          issue.ParentIssueID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return parentSyncUnavailable(uuidToString(issue.ParentIssueID), "parent_issue_missing"), nil
		}
		return parentSyncPlan{}, fmt.Errorf("load parent issue: %w", err)
	}
	parentID := uuidToString(parent.ID)

	// The dispatcher is the last human fallback when the resolved target cannot
	// be woken; it is never authority to wake somebody else.
	dispatcher, dispatcherActive, dispatcherErr := reviewDispatcher(ctx, h.Queries, parent)
	if dispatcherErr != nil {
		return parentSyncPlan{}, dispatcherErr
	}

	fallback := func(reason string) (parentSyncPlan, error) {
		if dispatcher.Valid && dispatcherActive {
			name := h.userDisplayName(ctx, dispatcher)
			return parentSyncPlan{
				response: CommentParentSyncResponse{
					ParentIssueID: parentID,
					Mode:          parentSyncModeDispatcher,
					Recipients:    []CommentParentSyncRecipient{{Type: "member", ID: uuidToString(dispatcher), Name: name}},
					Reason:        reason,
				},
				inboxRecipients: []pgtype.UUID{dispatcher},
				needsDelivery:   true,
			}, nil
		}
		return h.parentSyncAdminPlan(ctx, issue, parentID, reason)
	}

	if parent.AssigneeType.Valid && parent.AssigneeID.Valid {
		switch parent.AssigneeType.String {
		case "member":
			if _, memberErr := h.Queries.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{
				UserID:      parent.AssigneeID,
				WorkspaceID: issue.WorkspaceID,
			}); memberErr == nil {
				recipient := CommentParentSyncRecipient{Type: "member", ID: uuidToString(parent.AssigneeID), Name: h.userDisplayName(ctx, parent.AssigneeID)}
				_, already := routedMemberIDs(content)[recipient.ID]
				if actorType == "member" && actorID == recipient.ID {
					// Never notify the author about their own comment (B11).
					already = true
				}
				return parentSyncPlan{
					response: CommentParentSyncResponse{
						ParentIssueID: parentID,
						Mode:          parentSyncModeMember,
						Recipients:    []CommentParentSyncRecipient{recipient},
						AlreadyRouted: already,
					},
					inboxRecipients: []pgtype.UUID{parent.AssigneeID},
					needsDelivery:   !already,
				}, nil
			}
			return fallback("parent_assignee_inactive")
		case "agent", "squad":
			agent, squad, resolveErr := h.parentSyncTargetAgent(ctx, parent)
			if resolveErr != nil {
				return parentSyncPlan{}, resolveErr
			}
			if agent == nil {
				return fallback("parent_assignee_missing")
			}
			if agent.ArchivedAt.Valid || !agent.RuntimeID.Valid {
				return fallback("parent_assignee_runtime_unavailable")
			}
			if !h.canInvokeAgent(ctx, *agent, actorType, actorID, originatorUserID, uuidToString(issue.WorkspaceID)) {
				return fallback("parent_assignee_not_invocable")
			}
			if !h.isRuntimeOnline(ctx, agent.RuntimeID) {
				return fallback("parent_assignee_runtime_offline")
			}
			recipient := CommentParentSyncRecipient{Type: "agent", ID: uuidToString(agent.ID), Name: agent.Name}
			squadID := pgtype.UUID{}
			if squad != nil {
				recipient = CommentParentSyncRecipient{Type: "squad", ID: uuidToString(squad.ID), Name: squad.Name}
				squadID = squad.ID
			}
			// Dedupe against the actual executing agent id as well as the display
			// target: a @squad mention resolves to its leader for routing.
			_, already := routedAgents[recipient.ID]
			if !already {
				_, already = routedAgents[uuidToString(agent.ID)]
			}
			if actorType == "agent" && actorID == uuidToString(agent.ID) {
				// The parent assignee is the run that wrote this comment; a
				// self-notification is not a hand-off (B11).
				already = true
			}
			return parentSyncPlan{
				response: CommentParentSyncResponse{
					ParentIssueID: parentID,
					Mode:          parentSyncModeAgent,
					Recipients:    []CommentParentSyncRecipient{recipient},
					AlreadyRouted: already,
				},
				targetAgent:    *agent,
				targetIsLeader: squad != nil,
				targetSquadID:  squadID,
				needsDelivery:  !already,
			}, nil
		}
	}
	return fallback("parent_has_no_assignee")
}

// parentSyncTargetAgent resolves the executing agent (and squad, when the parent
// is squad-assigned) the sync would wake.
func (h *Handler) parentSyncTargetAgent(ctx context.Context, parent db.Issue) (*db.Agent, *db.Squad, error) {
	switch parent.AssigneeType.String {
	case "agent":
		agent, err := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{
			ID:          parent.AssigneeID,
			WorkspaceID: parent.WorkspaceID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("load parent assignee agent: %w", err)
		}
		return &agent, nil, nil
	case "squad":
		squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{
			ID:          parent.AssigneeID,
			WorkspaceID: parent.WorkspaceID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("load parent assignee squad: %w", err)
		}
		leader, leaderErr := h.Queries.GetAgent(ctx, squad.LeaderID)
		if leaderErr != nil {
			return nil, nil, nil
		}
		return &leader, &squad, nil
	}
	return nil, nil, nil
}

// parentSyncAdminPlan is the last fallback: tell the workspace owner/admin(s)
// that a synced comment needs a human decision. Admins are only informed — the
// sync never starts a run for them (B11).
func (h *Handler) parentSyncAdminPlan(ctx context.Context, issue db.Issue, parentIssueID, reason string) (parentSyncPlan, error) {
	managerIDs, err := h.Queries.ListWorkspaceManagerUserIDs(ctx, issue.WorkspaceID)
	if err != nil {
		return parentSyncPlan{}, fmt.Errorf("list workspace managers: %w", err)
	}
	recipients := make([]CommentParentSyncRecipient, 0, len(managerIDs))
	inboxRecipients := make([]pgtype.UUID, 0, len(managerIDs))
	for _, managerID := range managerIDs {
		id := managerID
		recipients = append(recipients, CommentParentSyncRecipient{Type: "member", ID: uuidToString(id), Name: h.userDisplayName(ctx, id)})
		inboxRecipients = append(inboxRecipients, id)
	}
	if len(recipients) == 0 {
		return parentSyncUnavailable(parentIssueID, "no_fallback_recipient"), nil
	}
	return parentSyncPlan{
		response: CommentParentSyncResponse{
			ParentIssueID: parentIssueID,
			Mode:          parentSyncModeAdmin,
			Recipients:    recipients,
			Reason:        reason,
		},
		inboxRecipients: inboxRecipients,
		needsDelivery:   true,
	}, nil
}

// userDisplayName resolves a member's display name for the recipient preview.
func (h *Handler) userDisplayName(ctx context.Context, userID pgtype.UUID) string {
	user, err := h.Queries.GetUser(ctx, userID)
	if err != nil {
		return uuidToString(userID)
	}
	if name := strings.TrimSpace(user.Name); name != "" {
		return name
	}
	return user.Email
}

// routedMemberIDs returns the members explicitly mentioned in the comment: the
// only way normal routing already delivers to a member.
func routedMemberIDs(content string) map[string]struct{} {
	routed := map[string]struct{}{}
	for _, mention := range util.ParseMentions(content) {
		if mention.Type == "member" {
			routed[mention.ID] = struct{}{}
		}
	}
	return routed
}

// deliverParentSync performs the additive delivery after the comment was saved.
// A failure never removes the comment: the caller reports the partial result and
// the persisted parent_sync records what happened (B11).
func (h *Handler) deliverParentSync(ctx context.Context, issue db.Issue, comment db.Comment, plan parentSyncPlan) (string, string) {
	if !plan.needsDelivery {
		return parentSyncDeliveryAlready, ""
	}
	if plan.response.Mode == parentSyncModeAgent {
		sync := service.ParentSyncTaskContext{
			ParentIssueID:   plan.response.ParentIssueID,
			SourceCommentID: uuidToString(comment.ID),
			Mode:            plan.response.Mode,
			ReceiverType:    plan.response.Recipients[0].Type,
			ReceiverID:      plan.response.Recipients[0].ID,
		}
		_, err := h.TaskService.EnqueueTaskForParentSync(ctx, issue, plan.targetAgent.ID, plan.targetIsLeader, plan.targetSquadID, comment.ID, sync)
		if err == nil {
			return parentSyncDeliveryQueued, ""
		}
		if errors.Is(err, service.ErrDuplicatePendingTask) {
			// A run for this agent on the child already owns this thread. Fold the
			// synced comment into it instead of dropping it: a queued sibling is
			// merged atomically (deferred), a claimed sibling records it as a
			// planned input (deferred). Only an existing run that can take neither
			// — or whose merge would degrade its attribution — is reported blocked.
			mergeTrigger := commentAgentTrigger{Agent: plan.targetAgent, Source: commentTriggerSourceMentionAgent}
			switch h.mergeCommentIntoPendingTask(ctx, issue, mergeTrigger, comment.ID, h.TaskService.ResolveIssueReviewSHAParam(ctx, issue.ID)) {
			case commentMergeSucceeded:
				return parentSyncDeliveryDeferred, ""
			case commentMergeNoPendingTask:
				registered, registerErr := h.registerPlannedCommentForActiveTask(ctx, issue, plan.targetAgent.ID, comment.ID, h.TaskService.ResolveIssueReviewSHAParam(ctx, issue.ID))
				if registerErr == nil && registered {
					return parentSyncDeliveryDeferred, ""
				}
				return parentSyncDeliveryBlocked, "duplicate_pending_task"
			case commentMergeAttributionBlocked:
				return parentSyncDeliveryBlocked, "attribution_blocked"
			default:
				return parentSyncDeliveryBlocked, "internal_error"
			}
		}
		slog.Warn("parent sync enqueue failed",
			"issue_id", uuidToString(issue.ID),
			"comment_id", uuidToString(comment.ID),
			"parent_issue_id", plan.response.ParentIssueID,
			"agent_id", uuidToString(plan.targetAgent.ID),
			"error", err)
		return parentSyncDeliveryBlocked, "enqueue_failed"
	}

	failed := 0
	for _, recipient := range plan.inboxRecipients {
		if comment.AuthorType == "member" && recipient == comment.AuthorID {
			continue // never notify the author about their own comment
		}
		if err := h.createParentSyncInbox(ctx, issue, comment, recipient, plan.response); err != nil {
			failed++
			slog.Warn("parent sync inbox write failed",
				"issue_id", uuidToString(issue.ID),
				"comment_id", uuidToString(comment.ID),
				"recipient_id", uuidToString(recipient),
				"error", err)
		}
	}
	if failed == len(plan.inboxRecipients) {
		return parentSyncDeliveryBlocked, "inbox_failed"
	}
	return parentSyncDeliveryNotified, ""
}

// createParentSyncInbox writes one member notification pointing at the original
// child comment, so the inbox row can open the thread it came from (B11/B14).
func (h *Handler) createParentSyncInbox(ctx context.Context, issue db.Issue, comment db.Comment, recipient pgtype.UUID, plan CommentParentSyncResponse) error {
	details, err := json.Marshal(map[string]any{
		"comment_id":          uuidToString(comment.ID),
		"parent_sync":         true,
		"parent_issue_id":     plan.ParentIssueID,
		"synced_issue_id":     uuidToString(issue.ID),
		"synced_issue_number": issue.Number,
	})
	if err != nil {
		return err
	}
	// The item lives on the CHILD issue: that is the atomic card whose comment
	// is being read; the inbox row adds the parent line through the B14 fields.
	item, err := h.Queries.CreateInboxItem(ctx, db.CreateInboxItemParams{
		ID:            dbid.NewV7(),
		WorkspaceID:   issue.WorkspaceID,
		RecipientType: "member",
		RecipientID:   recipient,
		Type:          parentSyncInboxType,
		Severity:      "info",
		IssueID:       issue.ID,
		Title:         "Sub-issue comment synced",
		Body:          util.StrToText(inboxSyncBody(comment.Content)),
		ActorType:     util.StrToText(comment.AuthorType),
		ActorID:       comment.AuthorID,
		Details:       details,
	})
	if err != nil {
		return err
	}
	resp := h.enrichInboxResponse(ctx, inboxToResponse(item), item.IssueID)
	h.publish(protocol.EventInboxNew, uuidToString(issue.WorkspaceID), comment.AuthorType, uuidToString(comment.AuthorID), map[string]any{
		"item": resp,
	})
	return nil
}

// inboxSyncBody bounds the notification preview the same way list rows do.
func inboxSyncBody(content string) string {
	runes := []rune(content)
	if len(runes) <= inboxListBodyPreviewLimit {
		return content
	}
	return string(runes[:inboxListBodyPreviewLimit-1]) + "…"
}

// persistParentSync stores the server-decided outcome on the comment row.
func (h *Handler) persistParentSync(ctx context.Context, workspaceID pgtype.UUID, commentID pgtype.UUID, plan parentSyncPlan, status, reason string) (*CommentParentSyncResponse, error) {
	persisted := plan.persisted(status)
	if reason != "" && persisted.Reason == "" {
		persisted.Reason = reason
	}
	encoded, err := json.Marshal(persisted)
	if err != nil {
		return nil, err
	}
	if _, err := h.Queries.SetCommentParentSync(ctx, db.SetCommentParentSyncParams{
		ID:          commentID,
		WorkspaceID: workspaceID,
		ParentSync:  encoded,
	}); err != nil {
		return nil, err
	}
	response := plan.responseWith(status)
	if reason != "" && response.Reason == "" {
		response.Reason = reason
	}
	return &response, nil
}
