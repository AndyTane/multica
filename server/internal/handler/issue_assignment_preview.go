package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// IssueAssignmentPreviewRequest is the body of POST
// /api/issues/{id}/assignment-preview (CLAWSTA-151 B13). The caller never
// passes the dispatcher: it is always resolved server-side from the issue.
type IssueAssignmentPreviewRequest struct {
	AssigneeType string `json:"assignee_type"`
	AssigneeID   string `json:"assignee_id"`
}

// IssueAssignmentPreviewResponse is a read-only prediction, never an
// authorization: the real assign/transfer path re-checks the caller.
//
//   - actor_can_assign: today's permission decision for the CALLER.
//   - dispatcher_can_invoke: whether the issue's dispatcher would still be able
//     to wake that agent for the follow-up run; null when the issue has no
//     trusted dispatcher (an internal execution card is not a permission
//     failure, so it must not disable reassignment).
//   - reason: a stable code for the UI copy when either flag is false.
type IssueAssignmentPreviewResponse struct {
	ActorCanAssign      bool    `json:"actor_can_assign"`
	DispatcherUserID    *string `json:"dispatcher_user_id"`
	DispatcherCanInvoke *bool   `json:"dispatcher_can_invoke"`
	Reason              string  `json:"reason,omitempty"`
}

const (
	assignmentPreviewReasonActorDenied       = "actor_cannot_assign"
	assignmentPreviewReasonDispatcherDenied  = "dispatcher_cannot_invoke"
	assignmentPreviewReasonDispatcherMissing = "dispatcher_inactive"
)

func (h *Handler) PreviewIssueAssignment(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req IssueAssignmentPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AssigneeType != "agent" && req.AssigneeType != "squad" {
		writeError(w, http.StatusBadRequest, "assignee_type must be agent or squad")
		return
	}
	assigneeID, err := util.ParseUUID(req.AssigneeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "assignee_id must be a UUID")
		return
	}

	// Resolve the candidate inside the issue's workspace. A squad's preview
	// judges its leader, exactly like the real assign path.
	var targetAgent db.Agent
	switch req.AssigneeType {
	case "agent":
		agent, agentErr := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
			ID:          assigneeID,
			WorkspaceID: issue.WorkspaceID,
		})
		if agentErr != nil {
			writeError(w, http.StatusBadRequest, "assignee_id does not refer to an agent of this workspace")
			return
		}
		if agent.ArchivedAt.Valid {
			writeError(w, http.StatusBadRequest, "cannot assign to archived agent")
			return
		}
		targetAgent = agent
	case "squad":
		squad, squadErr := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{
			ID:          assigneeID,
			WorkspaceID: issue.WorkspaceID,
		})
		if squadErr != nil {
			writeError(w, http.StatusBadRequest, "assignee_id does not refer to a squad in this workspace")
			return
		}
		if squad.ArchivedAt.Valid {
			writeError(w, http.StatusBadRequest, "cannot assign to an archived squad")
			return
		}
		leader, leaderErr := h.Queries.GetAgent(r.Context(), squad.LeaderID)
		if leaderErr != nil || leader.ArchivedAt.Valid {
			writeError(w, http.StatusBadRequest, "squad leader is archived; cannot assign to this squad")
			return
		}
		targetAgent = leader
	}

	workspaceID := uuidToString(issue.WorkspaceID)
	actorType, actorID := h.resolveActor(r, requestUserID(r), workspaceID)
	invoker := h.invokeOriginatorFromRequest(r, actorType, actorID)
	response := IssueAssignmentPreviewResponse{
		ActorCanAssign: h.canInvokeAgent(r.Context(), targetAgent, actorType, actorID, invoker, workspaceID),
	}
	if !response.ActorCanAssign {
		response.Reason = assignmentPreviewReasonActorDenied
	}

	rows, dispatcherErr := h.Queries.ListIssueDispatchers(r.Context(), db.ListIssueDispatchersParams{
		WorkspaceID: issue.WorkspaceID,
		Ids:         []pgtype.UUID{issue.ID},
	})
	if dispatcherErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve issue dispatcher")
		return
	}
	if len(rows) == 1 && rows[0].DispatcherUserID.Valid {
		dispatcherID := uuidToString(rows[0].DispatcherUserID)
		response.DispatcherUserID = &dispatcherID
		canInvoke := false
		if rows[0].DispatcherActive {
			canInvoke = h.canInvokeAgent(r.Context(), targetAgent, "member", dispatcherID, "", workspaceID)
		} else {
			response.Reason = assignmentPreviewReasonDispatcherMissing
		}
		response.DispatcherCanInvoke = &canInvoke
		if rows[0].DispatcherActive && !canInvoke && response.Reason == "" {
			response.Reason = assignmentPreviewReasonDispatcherDenied
		}
	}

	writeJSON(w, http.StatusOK, response)
}
