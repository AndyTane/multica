package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestParentSyncPreviewRechecksAtCreate covers B11 item 9: a preview taken while
// the parent agent was invocable is not an authorization — the create path
// re-resolves and the create-time result wins.
func TestParentSyncPreviewRechecksAtCreate(t *testing.T) {
	author := dbfx.User(t, "Sync revoke author", "sync-revoke-author@example.test")
	dbfx.Member(t, testWorkspaceID, author, "member")
	parentAgent := createHandlerTestAgent(t, "Sync revoke parent agent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)

	var preview CommentTriggerPreviewResponse
	testutil.Call(t, testHandler.PreviewCommentTriggers,
		withURLParam(newRequestAs(author, http.MethodPost, "/api/issues/"+child+"/comments/trigger-preview", map[string]any{
			"content":              "should the parent owner read this",
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(200).JSON(&preview)
	if preview.ParentSync == nil || preview.ParentSync.Mode != parentSyncModeAgent {
		t.Fatalf("preview parent_sync = %+v, want agent", preview.ParentSync)
	}

	// Revoke the invocation permission between preview and create.
	dbfx.Exec(t, `DELETE FROM agent_invocation_target WHERE agent_id = $1`, parentAgent)
	dbfx.Exec(t, `UPDATE agent SET permission_mode = 'private' WHERE id = $1`, parentAgent)

	var resp CommentResponse
	testutil.Call(t, testHandler.CreateComment,
		withURLParam(newRequestAs(author, http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
			"content":              "should the parent owner read this",
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(201).JSON(&resp)
	assertParentSync(t, resp, parentSyncModeDispatcher, false, parentSyncDeliveryNotified)
	if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 0 {
		t.Fatalf("revoked agent woke %d time(s); create must recheck", tasks)
	}
	if inbox := parentSyncInboxCount(t, child, testUserID); inbox != 1 {
		t.Fatalf("dispatcher inbox notifications = %d, want 1", inbox)
	}
}

// TestParentSyncQueuedRunMerges covers B11 item 10 (first half): when a QUEUED
// run for the same agent already owns the thread but the comment's normal
// routing goes elsewhere, the sync folds the comment into that run and reports a
// deferred delivery instead of enqueueing a duplicate.
func TestParentSyncQueuedRunMerges(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "Sync merge parent agent", nil)
	threadAgent := createHandlerTestAgent(t, "Sync merge thread agent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)

	rootID := dbfx.Comment(t, child, "thread root", testutil.Cols{"author_type": "agent", "author_id": threadAgent})
	replyTo := dbfx.Comment(t, child, "another agent comment", testutil.Cols{
		"author_type": "agent", "author_id": threadAgent, "parent_id": rootID,
	})
	// The parent agent already has a queued run covering this thread.
	existing := dbfx.Task(t, parentAgent, testutil.Cols{
		"issue_id": child, "runtime_id": testRuntimeID, "status": "queued",
		"trigger_comment_id": rootID,
	})

	var resp CommentResponse
	testutil.Call(t, testHandler.CreateComment,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
			"content":              "please fold this into your run",
			"parent_id":            replyTo,
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(201).JSON(&resp)
	assertParentSync(t, resp, parentSyncModeAgent, false, parentSyncDeliveryDeferred)
	if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 1 {
		t.Fatalf("parent agent tasks = %d, want the single pre-existing run", tasks)
	}
	task, err := testHandler.Queries.GetAgentTask(context.Background(), parseUUID(existing))
	if err != nil {
		t.Fatal(err)
	}
	if uuidToString(task.TriggerCommentID) != resp.ID {
		t.Fatalf("merge trigger = %s, want the synced comment %s", uuidToString(task.TriggerCommentID), resp.ID)
	}
}

// TestParentSyncUndeliverablePendingRun covers B11 item 10 (second half): when
// the competing run cannot accept the comment (here the merge is refused
// fail-closed), the delivery is reported blocked with a concrete reason, while
// the comment itself is saved exactly once and its id is returned.
func TestParentSyncUndeliverablePendingRun(t *testing.T) {
	authorAgent := dbfx.Agent(t, "Sync blocked author agent", testRuntimeID, testutil.Cols{
		"owner_id": testUserID, "visibility": "workspace", "permission_mode": "public_to",
	})
	authorTask := dbfx.Task(t, authorAgent, testutil.Cols{"issue_id": nil, "runtime_id": testRuntimeID})
	parentAgent := createHandlerTestAgent(t, "Sync blocked parent agent", nil)
	threadAgent := createHandlerTestAgent(t, "Sync blocked thread agent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)

	rootID := dbfx.Comment(t, child, "thread root by another agent", testutil.Cols{"author_type": "agent", "author_id": threadAgent})
	replyTo := dbfx.Comment(t, child, "direct parent by another agent", testutil.Cols{
		"author_type": "agent", "author_id": threadAgent, "parent_id": rootID,
	})
	existing := dbfx.Task(t, parentAgent, testutil.Cols{
		"issue_id": child, "runtime_id": testRuntimeID, "status": "queued",
		"trigger_comment_id": rootID,
	})
	var beforeTrigger string
	dbfx.QueryRow(t, `SELECT COALESCE(trigger_comment_id::text, '') FROM agent_task_queue WHERE id = $1`, existing).Scan(&beforeTrigger)

	// Fail-closed workspace + an agent comment with no human lineage: the merge
	// into the pending run must be refused rather than degrade the run.
	dbfx.Exec(t, `UPDATE workspace SET attribution_fail_closed = true WHERE id = $1`, testWorkspaceID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE workspace SET attribution_fail_closed = false WHERE id = $1`, testWorkspaceID)
	})
	req := withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
		"content":              "the pending run must take this",
		"parent_id":            replyTo,
		"sync_parent_assignee": true,
	}), "id", child)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", authorAgent)
	req.Header.Set("X-Task-ID", authorTask)
	var resp CommentResponse
	testutil.Call(t, testHandler.CreateComment, req).Want(201).JSON(&resp)

	if resp.ID == "" {
		t.Fatal("comment id was not returned on a blocked delivery")
	}
	assertParentSync(t, resp, parentSyncModeAgent, false, parentSyncDeliveryBlocked)
	if resp.ParentSync.Reason == "" {
		t.Fatal("blocked delivery carries no reason")
	}
	if comments := parentSyncCommentCount(t, child); comments != 3 { // root + agent reply + this one
		t.Fatalf("comments on the child = %d, want exactly 3 (no duplicate body)", comments)
	}
	task, err := testHandler.Queries.GetAgentTask(context.Background(), parseUUID(existing))
	if err != nil {
		t.Fatal(err)
	}
	if uuidToString(task.TriggerCommentID) != beforeTrigger {
		t.Fatal("blocked delivery still mutated the existing run")
	}
}

// TestParentSyncEnqueueFailureIsBlocked covers the enqueue-error half of B11
// item 10 directly: the delivery-time re-check catches an agent archived after
// the route was resolved, and reports blocked with a concrete reason instead of
// claiming a run.
func TestParentSyncEnqueueFailureIsBlocked(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "Sync stale parent agent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)
	childIssue, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(child))
	if err != nil {
		t.Fatal(err)
	}
	commentID := dbfx.Comment(t, child, "please read this", testutil.Cols{})
	comment, err := testHandler.Queries.GetComment(context.Background(), parseUUID(commentID))
	if err != nil {
		t.Fatal(err)
	}

	plan, err := testHandler.resolveParentSyncPlan(context.Background(), childIssue, "member", testUserID, testUserID, "please read this", nil)
	if err != nil {
		t.Fatalf("resolve plan: %v", err)
	}
	if plan.response.Mode != parentSyncModeAgent {
		t.Fatalf("plan mode = %q, want agent", plan.response.Mode)
	}
	// Archive the agent after resolution but before delivery.
	dbfx.Exec(t, `UPDATE agent SET archived_at = now() WHERE id = $1`, parentAgent)

	status, reason := testHandler.deliverParentSync(context.Background(), childIssue, comment, plan)
	if status != parentSyncDeliveryBlocked || reason == "" {
		t.Fatalf("delivery = %q/%q, want blocked with a reason", status, reason)
	}
	if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 0 {
		t.Fatalf("archived agent got %d task(s)", tasks)
	}
}

// TestParentSyncSuppressConflictAndEmptyPreview covers B11 item 11: suppressing
// the sync target is an explicit 400, and an empty-body preview still reports the
// parent-sync route.
func TestParentSyncSuppressConflictAndEmptyPreview(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "Sync suppress parent agent", nil)
	child, parent, _ := parentSyncFixture(t, "agent", parentAgent)

	testutil.Call(t, testHandler.CreateComment,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
			"content":              "look here",
			"sync_parent_assignee": true,
			"suppress_agent_ids":   []string{parentAgent},
		}), "id", child)).
		Want(400)
	if comments := parentSyncCommentCount(t, child); comments != 0 {
		t.Fatalf("conflicting request wrote %d comment(s)", comments)
	}

	var preview CommentTriggerPreviewResponse
	testutil.Call(t, testHandler.PreviewCommentTriggers,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments/trigger-preview", map[string]any{
			"content":              "",
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(200).JSON(&preview)
	if preview.ParentSync == nil || preview.ParentSync.Mode != parentSyncModeAgent {
		t.Fatalf("empty-body preview parent_sync = %+v", preview.ParentSync)
	}
	if preview.ParentSync.ParentIssueID != parent {
		t.Fatalf("empty-body preview parent = %q, want %q", preview.ParentSync.ParentIssueID, parent)
	}
	if preview.ParentSync.AlreadyRouted {
		t.Fatal("empty-body preview claims already routed")
	}
	if len(preview.Agents) != 0 {
		t.Fatalf("empty-body preview agents = %+v", preview.Agents)
	}
	_ = time.Now()
}
