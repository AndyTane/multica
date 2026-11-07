package service

import (
	"context"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// HasSquadLeaderNoActionEvaluationForTask reports whether this exact task
// already recorded a squad leader no_action evaluation.
func HasSquadLeaderNoActionEvaluationForTask(ctx context.Context, q *db.Queries, task db.AgentTaskQueue) (bool, error) {
	if q == nil || !task.ID.Valid || !task.IssueID.Valid || !task.AgentID.Valid {
		return false, nil
	}
	return q.HasSquadLeaderNoActionEvaluationForTask(ctx, db.HasSquadLeaderNoActionEvaluationForTaskParams{
		IssueID: task.IssueID,
		AgentID: task.AgentID,
		TaskID:  util.UUIDToString(task.ID),
	})
}

// HasNoActionEvaluationForTask reports whether this exact task recorded a
// "read it, nothing to change" decision through EITHER exit: the existing squad
// leader evaluation, or the R4 parent-assignee sync exit (CLAWSTA-151 B12).
// Both suppress the completion fallback comment and both mean "no reply", so
// every server-side gate must ask the combined question.
func HasNoActionEvaluationForTask(ctx context.Context, q *db.Queries, task db.AgentTaskQueue) (bool, error) {
	if q == nil || !task.ID.Valid || !task.IssueID.Valid || !task.AgentID.Valid {
		return false, nil
	}
	return q.HasTaskNoActionEvaluation(ctx, db.HasTaskNoActionEvaluationParams{
		IssueID: task.IssueID,
		AgentID: task.AgentID,
		TaskID:  util.UUIDToString(task.ID),
	})
}
