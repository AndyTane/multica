package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// "Read it, nothing to change" for a parent-synced sub-issue comment
// (CLAWSTA-151 B12). It generalizes the squad leader no_action exit to an
// ordinary parent-card assignee: the run read the synced comment, decided the
// child needs no adjustment, and may end WITHOUT posting a comment.
const (
	issueAssigneeEvaluatedAction   = "issue_assignee_evaluated"
	issueEvaluationOutcomeNoAction = "no_action"
	issueEvaluationReasonMaxRunes  = 120
)

// IssueEvaluatedRequest is the body of POST /api/issues/{id}/evaluated.
type IssueEvaluatedRequest struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

// RecordIssueAssigneeEvaluated records the silent exit for the parent card's
// assignee after a synced comment. Authorization is task provenance, not the
// current assignee alone: the trusted X-Task-ID token must name a PURE
// parent-sync run whose recorded parent still matches and whose receiver is
// still this agent. Everything else must reply normally.
func (h *Handler) RecordIssueAssigneeEvaluated(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req IssueEvaluatedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Outcome != issueEvaluationOutcomeNoAction {
		writeError(w, http.StatusBadRequest, "outcome must be no_action")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if utf8.RuneCountInString(reason) > issueEvaluationReasonMaxRunes {
		writeError(w, http.StatusBadRequest, "reason must contain at most 120 Unicode code points")
		return
	}

	workspaceID := uuidToString(issue.WorkspaceID)
	userID := requestUserID(r)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	if actorType != "agent" {
		writeError(w, http.StatusForbidden, "only the agent running the synced task can record this evaluation")
		return
	}
	// The CLI stamps X-Task-ID from the authenticated mat_ token, so this names
	// the caller's own run, not a client-supplied id.
	task, ok := h.taskFromRequestHeader(r)
	if !ok {
		writeError(w, http.StatusForbidden, "only the agent running the synced task can record this evaluation")
		return
	}
	synced, isSync := h.TaskService.ParseParentSyncTaskContext(task)
	if !isSync {
		writeError(w, http.StatusBadRequest, "task was not created by a parent-sync comment; record the evaluation with a reply instead")
		return
	}
	// Ownership gate before quoting task-derived ids (same order as the squad
	// leader endpoint): the task must belong to this workspace, this issue, and
	// this agent.
	taskInWorkspace, taskErr := h.Queries.GetAgentTaskInWorkspace(r.Context(), db.GetAgentTaskInWorkspaceParams{
		ID:          task.ID,
		WorkspaceID: issue.WorkspaceID,
	})
	if taskErr != nil || !taskInWorkspace.IssueID.Valid {
		writeError(w, http.StatusBadRequest, "task does not belong to issue")
		return
	}
	if uuidToString(taskInWorkspace.IssueID) != uuidToString(issue.ID) {
		writeError(w, http.StatusBadRequest, "task does not belong to issue; record the evaluation on issue "+uuidToString(taskInWorkspace.IssueID))
		return
	}
	if !taskInWorkspace.AgentID.Valid || uuidToString(taskInWorkspace.AgentID) != actorID {
		writeError(w, http.StatusForbidden, "only the agent running the synced task can record this evaluation")
		return
	}
	task = taskInWorkspace

	// The parent relation and the receiver are re-checked, not trusted from the
	// task context: a reassignment since the sync means the person who read the
	// comment is no longer the one responsible, so the run must reply instead.
	if !issue.ParentIssueID.Valid || uuidToString(issue.ParentIssueID) != synced.ParentIssueID {
		writeError(w, http.StatusConflict, "the comment's parent card changed; record the evaluation again on the current parent")
		return
	}
	parent, parentErr := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{
		ID:          issue.ParentIssueID,
		WorkspaceID: issue.WorkspaceID,
	})
	if parentErr != nil {
		writeError(w, http.StatusConflict, "the parent card is no longer available; a reply is required")
		return
	}
	expectedAgentID, expectedSquadID := h.parentSyncReceiverKey(r.Context(), parent)
	if !expectedAgentID.Valid || uuidToString(expectedAgentID) != uuidToString(task.AgentID) {
		writeError(w, http.StatusConflict, "the parent card's assignee changed; the silent exit no longer applies")
		return
	}
	if synced.ReceiverType == "squad" && uuidToString(expectedSquadID) != synced.ReceiverID {
		writeError(w, http.StatusConflict, "the parent card's squad changed; the silent exit no longer applies")
		return
	}

	// Serialize with comment merging / steering: once this record exists, new
	// comments must start a fresh round (the merge queries exclude such tasks),
	// and a comment that already landed must not be silently superseded.
	ctx := r.Context()
	tx, beginErr := h.beginWakeupWrite(ctx)
	if beginErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to record evaluation")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	locked, lockErr := qtx.LockAgentTask(ctx, task.ID)
	if lockErr != nil {
		writeError(w, http.StatusConflict, "task is no longer available")
		return
	}

	existing, evalErr := qtx.GetTaskEvaluationActivity(ctx, db.GetTaskEvaluationActivityParams{
		IssueID: issue.ID,
		AgentID: task.AgentID,
		TaskID:  uuidToString(task.ID),
	})
	if evalErr == nil {
		if existing.Action == issueAssigneeEvaluatedAction {
			// Idempotent retry: same decision, same row.
			var details map[string]any
			_ = json.Unmarshal(existing.Details, &details)
			writeJSON(w, http.StatusOK, map[string]any{
				"id":         uuidToString(existing.ID),
				"action":     existing.Action,
				"details":    json.RawMessage(existing.Details),
				"created_at": timestampToString(existing.CreatedAt),
			})
			return
		}
		writeError(w, http.StatusConflict, "this task already recorded a different evaluation")
		return
	}
	if !errors.Is(evalErr, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to record evaluation")
		return
	}

	// A pure-sync round only. Every comment the task covers (trigger plus any
	// merged sync comments) must be a parent-sync record for this parent; an
	// ordinary @/reply obligation mixed in would make silence wrong.
	commentIDs := make([]pgtype.UUID, 0, 1+len(locked.CoalescedCommentIds))
	if locked.TriggerCommentID.Valid {
		commentIDs = append(commentIDs, locked.TriggerCommentID)
	}
	for _, id := range locked.CoalescedCommentIds {
		if id.Valid {
			commentIDs = append(commentIDs, id)
		}
	}
	if len(commentIDs) == 0 {
		writeError(w, http.StatusBadRequest, "task has no synced comment input")
		return
	}
	sourceCommentIDs := make([]string, 0, len(commentIDs))
	for _, commentID := range commentIDs {
		comment, commentErr := qtx.GetCommentInWorkspace(ctx, db.GetCommentInWorkspaceParams{
			ID:          commentID,
			WorkspaceID: issue.WorkspaceID,
		})
		if commentErr != nil {
			writeError(w, http.StatusConflict, "a comment covered by this task changed; a reply is required")
			return
		}
		var stored parentSyncPersisted
		if len(comment.ParentSync) == 0 || json.Unmarshal(comment.ParentSync, &stored) != nil ||
			stored.ParentIssueID != synced.ParentIssueID || stored.Mode != parentSyncModeAgent ||
			!stringInSlice(stored.DeliveryStatus, []string{parentSyncDeliveryQueued, parentSyncDeliveryDeferred, "pending"}) {
			writeError(w, http.StatusConflict, "this task also carries obligations other than the parent sync; a reply is required")
			return
		}
		sourceCommentIDs = append(sourceCommentIDs, uuidToString(commentID))
	}

	// A run that already posted a comment cannot also claim it stayed silent.
	commented, commentedErr := qtx.HasAgentCommentedSince(ctx, db.HasAgentCommentedSinceParams{
		IssueID:  issue.ID,
		AuthorID: task.AgentID,
		Since:    task.StartedAt,
	})
	if commentedErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to record evaluation")
		return
	}
	if commented {
		writeError(w, http.StatusConflict, "this run already posted a comment; the silent exit is not available")
		return
	}

	details, marshalErr := json.Marshal(map[string]any{
		"task_id":            uuidToString(task.ID),
		"outcome":            issueEvaluationOutcomeNoAction,
		"reason":             reason,
		"parent_issue_id":    synced.ParentIssueID,
		"source_comment_ids": sourceCommentIDs,
	})
	if marshalErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to record evaluation")
		return
	}
	activity, createErr := qtx.CreateActivity(ctx, db.CreateActivityParams{
		ID:          dbid.NewV7(),
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
		ActorType:   pgtype.Text{String: "agent", Valid: true},
		ActorID:     task.AgentID,
		Action:      issueAssigneeEvaluatedAction,
		Details:     details,
	})
	if createErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to record evaluation")
		return
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to record evaluation")
		return
	}

	h.publish(protocol.EventActivityCreated, uuidToString(issue.WorkspaceID), "agent", actorID, map[string]any{
		"issue_id": uuidToString(issue.ID),
		"entry": map[string]any{
			"type":       "activity",
			"id":         uuidToString(activity.ID),
			"actor_type": "agent",
			"actor_id":   actorID,
			"action":     activity.Action,
			"details":    json.RawMessage(details),
			"created_at": timestampToString(activity.CreatedAt),
		},
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         uuidToString(activity.ID),
		"action":     activity.Action,
		"details":    json.RawMessage(details),
		"created_at": timestampToString(activity.CreatedAt),
	})
}

// parentSyncReceiverKey resolves the executing agent (and squad, when the parent
// is squad-assigned) the sync should currently reach.
func (h *Handler) parentSyncReceiverKey(ctx context.Context, parent db.Issue) (pgtype.UUID, pgtype.UUID) {
	agent, squad, err := h.parentSyncTargetAgent(ctx, parent)
	if err != nil || agent == nil {
		return pgtype.UUID{}, pgtype.UUID{}
	}
	squadID := pgtype.UUID{}
	if squad != nil {
		squadID = squad.ID
	}
	return agent.ID, squadID
}

func stringInSlice(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}
