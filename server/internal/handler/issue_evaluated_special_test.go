package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// syncTaskForChild returns the parent-sync run created for a synced comment.
func syncTaskForChild(t *testing.T, childID, agentID string) string {
	t.Helper()
	var taskID string
	dbfx.QueryRow(t, `SELECT id::text FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2`, childID, agentID).Scan(&taskID)
	if taskID == "" {
		t.Fatal("no parent-sync task found")
	}
	return taskID
}

// postIssueEvaluated calls POST /api/issues/{id}/evaluated as a task-token agent.
func postIssueEvaluated(t *testing.T, issueID, agentID, taskID, outcome, reason string, want int) {
	t.Helper()
	req := withURLParam(newRequest(http.MethodPost, "/api/issues/"+issueID+"/evaluated", map[string]any{
		"outcome": outcome,
		"reason":  reason,
	}), "id", issueID)
	if taskID != "" {
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
	}
	testutil.Call(t, testHandler.RecordIssueAssigneeEvaluated, req).Want(want)
}

// TestNoActionForgedSourcesRejected covers B12 item 12: a member call, a
// hand-filled task id with no parent-sync provenance, and another agent's task
// all fail — only the run actually woken by the sync may record the silent exit.
func TestNoActionForgedSourcesRejected(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "No action forge parent agent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)
	syncComment := createSyncedComment(t, child, "read this and decide", 201)
	syncTask := syncTaskForChild(t, child, parentAgent)
	_ = syncComment

	// A member session (no task token) may never use the silent exit.
	postIssueEvaluated(t, child, "", "", "no_action", "member shortcut", http.StatusForbidden)

	// A hand-filled id naming an ordinary task of the same agent has no sync
	// provenance: refused before any write.
	ordinaryTask := dbfx.Task(t, parentAgent, testutil.Cols{
		"issue_id": child, "runtime_id": testRuntimeID, "status": "running",
		"started_at": testutil.Raw("now()"),
	})
	postIssueEvaluated(t, child, parentAgent, ordinaryTask, "no_action", "forged task", http.StatusBadRequest)

	// A hand-filled id naming ANOTHER agent's ordinary task fails provenance
	// before the ownership gate can even be reached.
	otherAgent := createHandlerTestAgent(t, "No action other agent", nil)
	otherTask := dbfx.Task(t, otherAgent, testutil.Cols{
		"issue_id": child, "runtime_id": testRuntimeID, "status": "running",
		"started_at": testutil.Raw("now()"),
	})
	postIssueEvaluated(t, child, parentAgent, otherTask, "no_action", "foreign task", http.StatusBadRequest)

	// The real sync task id with another agent as the caller is also refused.
	postIssueEvaluated(t, child, otherAgent, syncTask, "no_action", "wrong caller", http.StatusForbidden)
}

// TestNoActionRejectsMixedObligations covers B12 item 13: a run that also owns a
// normal comment obligation cannot claim it stayed silent for everything.
func TestNoActionRejectsMixedObligations(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "No action mixed parent agent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)
	createSyncedComment(t, child, "pure sync input", 201)
	taskID := syncTaskForChild(t, child, parentAgent)

	ordinary := dbfx.Comment(t, child, "an ordinary obligation folded in", testutil.Cols{})
	dbfx.Exec(t, `UPDATE agent_task_queue SET coalesced_comment_ids = array_append(coalesced_comment_ids, $2::uuid) WHERE id = $1`, taskID, ordinary)

	postIssueEvaluated(t, child, parentAgent, taskID, "no_action", "covers everything", http.StatusConflict)
}

// TestNoActionRecordStartsNewRound covers B12 item 14: after the silent record,
// a later comment is never folded into the finished round — its delivery is
// already routed normally (deferred), and completing the silent run starts it as
// its own run.
func TestNoActionRecordStartsNewRound(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "No action new round parent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)
	first := createSyncedComment(t, child, "first synced read", 201)
	taskID := syncTaskForChild(t, child, parentAgent)
	// A real silent round is a claimed run: mark it running before it reads and
	// records the decision.
	dbfx.Exec(t, `UPDATE agent_task_queue SET status = 'running', dispatched_at = now(), started_at = now() WHERE id = $1`, taskID)

	postIssueEvaluated(t, child, parentAgent, taskID, "no_action", "nothing to adjust", http.StatusCreated)
	before, err := testHandler.Queries.GetAgentTask(context.Background(), parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}

	// A reply in the same thread arrives while the silent round is pending. The
	// normal conversation routing already owns it, so the sync reports
	// already_routed; either way it must not be merged into the silent round.
	var second CommentResponse
	testutil.Call(t, testHandler.CreateComment,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
			"content":              "one more thing after the silence",
			"parent_id":            first.ID,
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(201).JSON(&second)
	if second.ParentSync == nil {
		t.Fatal("post-record comment has no parent_sync")
	}
	if second.ParentSync.DeliveryStatus != parentSyncDeliveryAlready && second.ParentSync.DeliveryStatus != parentSyncDeliveryBlocked {
		t.Fatalf("post-record delivery = %q, want already_routed or blocked (never merged)", second.ParentSync.DeliveryStatus)
	}
	after, err := testHandler.Queries.GetAgentTask(context.Background(), parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if uuidToString(after.TriggerCommentID) != uuidToString(before.TriggerCommentID) || len(after.CoalescedCommentIds) != len(before.CoalescedCommentIds) {
		t.Fatal("new comment was merged into the round that recorded no_action")
	}

	// Finishing the silent run releases the deferred comment as a NEW run.
	if _, err := testHandler.TaskService.CompleteTask(context.Background(), parseUUID(taskID), []byte(`{"output":"finished"}`), "", "", "", false, "", ""); err != nil {
		t.Fatalf("complete silent run: %v", err)
	}
	var newTaskID, newTrigger string
	dbfx.QueryRow(t, `SELECT id::text, COALESCE(trigger_comment_id::text, '') FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND id <> $3 AND status IN ('queued','dispatched') LIMIT 1`, child, parentAgent, taskID).Scan(&newTaskID, &newTrigger)
	if newTaskID == "" {
		t.Fatal("completion did not start a new run for the deferred comment")
	}
	if newTrigger != second.ID {
		t.Fatalf("new run trigger = %s, want the deferred comment %s", newTrigger, second.ID)
	}
}

type dbTaskState struct {
	trigger   string
	coalesced int
}
