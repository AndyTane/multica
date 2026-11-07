package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// parentSyncFixture builds a parent card assigned to one agent and a child card
// assigned to another, so the parent synced read is additive to the child's
// normal routing.
func parentSyncFixture(t *testing.T, parentAssigneeType, parentAssigneeID string) (childID, parentID, parentAgentID string) {
	t.Helper()
	childAgent := createHandlerTestAgent(t, "Parent sync child owner", nil)
	parent := dbfx.Issue(t, "Parent sync parent card", testutil.Cols{
		"assignee_type": parentAssigneeType, "assignee_id": parentAssigneeID, "status": "in_progress",
	})
	child := dbfx.Issue(t, "Parent sync child card", testutil.Cols{
		"parent_issue_id": parent, "assignee_type": "agent", "assignee_id": childAgent,
	})
	return child, parent, parentAssigneeID
}

func createSyncedComment(t *testing.T, childID, content string, wantStatus int) CommentResponse {
	t.Helper()
	var resp CommentResponse
	testutil.Call(t, testHandler.CreateComment,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+childID+"/comments", map[string]any{
			"content":              content,
			"sync_parent_assignee": true,
		}), "id", childID)).
		Want(wantStatus).JSON(&resp)
	return resp
}

// TestCreateCommentSyncsToParentAgent covers B11's happy path: preview, create,
// persisted parent_sync, and the context-stamped task on the child card.
func TestCreateCommentSyncsToParentAgent(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "Parent sync assignee", nil)
	child, parent, _ := parentSyncFixture(t, "agent", parentAgent)

	// Preview names the resolved receiver and does not pretend the sync runs now.
	var preview CommentTriggerPreviewResponse
	testutil.Call(t, testHandler.PreviewCommentTriggers,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments/trigger-preview", map[string]any{
			"content":              "please review this",
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(200).JSON(&preview)
	if preview.ParentSync == nil || preview.ParentSync.Mode != parentSyncModeAgent {
		t.Fatalf("preview parent_sync = %+v", preview.ParentSync)
	}
	if len(preview.ParentSync.Recipients) != 1 || preview.ParentSync.Recipients[0].ID != parentAgent {
		t.Fatalf("preview recipients = %+v", preview.ParentSync.Recipients)
	}
	if preview.ParentSync.AlreadyRouted {
		t.Fatal("fresh card should not report the parent assignee as already routed")
	}

	resp := createSyncedComment(t, child, "please review this", 201)
	if resp.ParentSync == nil || resp.ParentSync.Mode != parentSyncModeAgent {
		t.Fatalf("created comment parent_sync = %+v", resp.ParentSync)
	}
	if resp.ParentSync.DeliveryStatus != parentSyncDeliveryQueued {
		t.Fatalf("delivery status = %q, want queued", resp.ParentSync.DeliveryStatus)
	}
	if resp.ParentSync.ParentIssueID != parent {
		t.Fatalf("parent issue id = %q", resp.ParentSync.ParentIssueID)
	}

	// The persisted comment record survives a re-read.
	stored, err := testHandler.Queries.GetComment(context.Background(), parseUUID(resp.ID))
	if err != nil {
		t.Fatal(err)
	}
	var persisted parentSyncPersisted
	if err := json.Unmarshal(stored.ParentSync, &persisted); err != nil {
		t.Fatalf("decode parent_sync: %v (%s)", err, stored.ParentSync)
	}
	if persisted.ParentIssueID != parent || persisted.Mode != parentSyncModeAgent || persisted.DeliveryStatus != parentSyncDeliveryQueued {
		t.Fatalf("persisted parent_sync = %+v", persisted)
	}

	// The sync run is a task on the CHILD issue for the parent's assignee, with
	// the parent provenance recorded in its context (B11).
	var taskID string
	dbfx.QueryRow(t, `SELECT id::text FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2`, child, parentAgent).Scan(&taskID)
	task, err := testHandler.Queries.GetAgentTask(context.Background(), parseUUID(taskID))
	if err != nil {
		t.Fatalf("load sync task: %v", err)
	}
	sync, ok := testHandler.TaskService.ParseParentSyncTaskContext(task)
	if !ok {
		t.Fatalf("task context missing parent_sync: %s", task.Context)
	}
	if sync.ParentIssueID != parent || sync.SourceCommentID != resp.ID || sync.Mode != parentSyncModeAgent {
		t.Fatalf("task parent_sync = %+v", sync)
	}
}

// TestCreateCommentSyncToMemberNotifiesInbox covers the member fallback: a
// member-assigned parent card is notified through the inbox, and no run starts.
func TestCreateCommentSyncToMemberNotifiesInbox(t *testing.T) {
	recipient := dbfx.User(t, "Parent sync member", "parent-sync-member@example.test")
	dbfx.Member(t, testWorkspaceID, recipient, "member")
	child, parent, _ := parentSyncFixture(t, "member", recipient)

	resp := createSyncedComment(t, child, "please take a look", 201)
	if resp.ParentSync == nil || resp.ParentSync.Mode != parentSyncModeMember {
		t.Fatalf("parent_sync = %+v", resp.ParentSync)
	}
	if resp.ParentSync.DeliveryStatus != parentSyncDeliveryNotified {
		t.Fatalf("delivery status = %q, want notified", resp.ParentSync.DeliveryStatus)
	}
	count := dbfx.Count(t, `
		SELECT count(*) FROM inbox_item
		WHERE workspace_id = $1 AND recipient_id = $2 AND type = $3 AND issue_id = $4
	`, testWorkspaceID, recipient, parentSyncInboxType, child)
	if count != 1 {
		t.Fatalf("sync inbox items = %d, want 1", count)
	}
	var title string
	var details []byte
	dbfx.QueryRow(t, `SELECT title, details FROM inbox_item WHERE workspace_id = $1 AND recipient_id = $2 AND type = $3 AND issue_id = $4`,
		testWorkspaceID, recipient, parentSyncInboxType, child).Scan(&title, &details)
	if title != "Parent sync child card" {
		t.Fatalf("inbox title = %q, want the child issue title", title)
	}
	var parsed map[string]any
	if err := json.Unmarshal(details, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["comment_id"] != resp.ID || parsed["parent_issue_id"] != parent {
		t.Fatalf("inbox details = %v", parsed)
	}
	// No run for the member fallback.
	tasks := dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1`, child)
	if tasks != 1 { // only the child's own assignee task from normal routing
		t.Fatalf("tasks on child = %d, want only the normal assignee task", tasks)
	}
}

// TestParentSyncRejects covers the two hard refusals: /note and edit.
func TestParentSyncRejects(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "Parent sync reject assignee", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)

	createSyncedComment(t, child, "/note this is a note", 400)
	createSyncedComment(t, child, "please look", 201)

	var comment CommentResponse
	testutil.Call(t, testHandler.CreateComment,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
			"content": "plain comment",
		}), "id", child)).Want(201).JSON(&comment)

	testutil.Call(t, testHandler.UpdateComment,
		withURLParam(newRequest(http.MethodPut, "/api/comments/"+comment.ID, map[string]any{
			"content":              "edited",
			"sync_parent_assignee": true,
		}), "commentId", comment.ID)).Want(400)
}

// TestRecordNoActionForSyncedRead covers B12: a pure parent-sync run records the
// silent exit, repeats idempotently, and a non-sync task is refused.
func TestRecordNoActionForSyncedRead(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "No action parent assignee", nil)
	child, parent, _ := parentSyncFixture(t, "agent", parentAgent)
	resp := createSyncedComment(t, child, "read and decide", 201)

	var taskID string
	dbfx.QueryRow(t, `SELECT id FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2`, child, parentAgent).Scan(&taskID)

	evaluate := func(t *testing.T, want int) map[string]any {
		t.Helper()
		req := withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/evaluated", map[string]any{
			"outcome": "no_action",
			"reason":  "the sub-issue needs no adjustment",
		}), "id", child)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", parentAgent)
		req.Header.Set("X-Task-ID", taskID)
		var out map[string]any
		testutil.Call(t, testHandler.RecordIssueAssigneeEvaluated, req).Want(want).JSON(&out)
		return out
	}

	first := evaluate(t, 201)
	second := evaluate(t, 200)
	if first["id"] != second["id"] {
		t.Fatalf("idempotent repeat returned a different activity: %v vs %v", first["id"], second["id"])
	}

	activity, err := testHandler.Queries.GetTaskEvaluationActivity(context.Background(), db.GetTaskEvaluationActivityParams{
		IssueID: parseUUID(child),
		AgentID: parseUUID(parentAgent),
		TaskID:  taskID,
	})
	if err != nil {
		t.Fatalf("evaluation activity: %v", err)
	}
	if activity.Action != issueAssigneeEvaluatedAction {
		t.Fatalf("activity action = %q", activity.Action)
	}
	var details map[string]any
	if err := json.Unmarshal(activity.Details, &details); err != nil {
		t.Fatal(err)
	}
	if details["parent_issue_id"] != parent || details["outcome"] != "no_action" {
		t.Fatalf("activity details = %v", details)
	}
	sources, ok := details["source_comment_ids"].([]any)
	if !ok || len(sources) != 1 || sources[0] != resp.ID {
		t.Fatalf("source comments = %v", details["source_comment_ids"])
	}

	// An ordinary task (no parent-sync provenance) cannot use the silent exit.
	plainAgent := createHandlerTestAgent(t, "No action plain agent", nil)
	plainIssue := dbfx.Issue(t, "No action plain issue", testutil.Cols{"assignee_type": "agent", "assignee_id": plainAgent})
	plainTask := dbfx.Task(t, plainAgent, testutil.Cols{
		"issue_id": plainIssue, "runtime_id": testRuntimeID, "status": "running",
		"started_at": testutil.Raw("now()"),
	})
	req := withURLParam(newRequest(http.MethodPost, "/api/issues/"+plainIssue+"/evaluated", map[string]any{
		"outcome": "no_action", "reason": "nothing to do",
	}), "id", plainIssue)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", plainAgent)
	req.Header.Set("X-Task-ID", plainTask)
	testutil.Call(t, testHandler.RecordIssueAssigneeEvaluated, req).Want(400)

}
