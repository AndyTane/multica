package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// TestCompleteTaskSuppressesFallbackAfterNoAction covers CLAWSTA-151 B12: once
// the run recorded the silent "read it, nothing to change" exit, CompleteTask
// must not synthesize the usual completion comment. The control task proves the
// fallback still fires without the record.
func TestCompleteTaskSuppressesFallbackAfterNoAction(t *testing.T) {
	pool := newResolveOriginatorPool(t)
	ctx := context.Background()
	q := db.New(pool)
	workspaceID, userID, agentID, issueID := seedAttributionFixture(t, pool)
	svc := &TaskService{Queries: q, TxStarter: pool, Bus: events.New()}

	seedRunningTask := func(t *testing.T) string {
		t.Helper()
		var taskID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO agent_task_queue (
				agent_id, runtime_id, issue_id, status, priority, started_at,
				originator_user_id, accountable_user_id
			)
			SELECT $1, a.runtime_id, $2, 'running', 0, now(), $3, $3
			FROM agent a WHERE a.id = $1
			RETURNING id`, agentID, issueID, userID).Scan(&taskID); err != nil {
			t.Fatalf("seed running task: %v", err)
		}
		return taskID
	}
	agentCommentCount := func(t *testing.T) int {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM comment
			WHERE issue_id = $1 AND author_type = 'agent' AND author_id = $2`, issueID, agentID).Scan(&count); err != nil {
			t.Fatalf("count agent comments: %v", err)
		}
		return count
	}
	complete := func(t *testing.T, taskID string) {
		t.Helper()
		if _, err := svc.CompleteTask(ctx, util.MustParseUUID(taskID), []byte(`{"output":"delivered work"}`), "", "", "", false, "", ""); err != nil {
			t.Fatalf("CompleteTask: %v", err)
		}
	}

	// Control: without a no_action record the fallback comment is written.
	controlTask := seedRunningTask(t)
	complete(t, controlTask)
	if count := agentCommentCount(t); count != 1 {
		t.Fatalf("control task fallback comments = %d, want 1", count)
	}

	// Silent: the same final output must not become a comment.
	silentTask := seedRunningTask(t)
	details, err := json.Marshal(map[string]any{
		"task_id":            silentTask,
		"outcome":            "no_action",
		"reason":             "the synced comment needs no change",
		"parent_issue_id":    issueID,
		"source_comment_ids": []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateActivity(ctx, db.CreateActivityParams{
		ID:          dbid.NewV7(),
		WorkspaceID: util.MustParseUUID(workspaceID),
		IssueID:     util.MustParseUUID(issueID),
		ActorType:   pgtype.Text{String: "agent", Valid: true},
		ActorID:     util.MustParseUUID(agentID),
		Action:      "issue_assignee_evaluated",
		Details:     details,
	}); err != nil {
		t.Fatalf("seed no_action activity: %v", err)
	}
	complete(t, silentTask)
	if count := agentCommentCount(t); count != 1 {
		t.Fatalf("silent task added a fallback comment (total = %d, want 1)", count)
	}
}
