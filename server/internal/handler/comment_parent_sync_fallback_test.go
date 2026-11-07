package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestParentSyncSquadResolvesLeader covers B11 item 5: a parent assigned to a
// squad resolves to its leader, keeps the squad id on the run, and records the
// squad as the recipient.
func TestParentSyncSquadResolvesLeader(t *testing.T) {
	leader := createHandlerTestAgent(t, "Sync squad leader", nil)
	squad := dbfx.Squad(t, "Sync squad", leader)
	child, _, _ := parentSyncFixture(t, "squad", squad)

	resp := createSyncedComment(t, child, "please coordinate this", 201)
	assertParentSync(t, resp, parentSyncModeAgent, false, parentSyncDeliveryQueued)
	if len(resp.ParentSync.Recipients) != 1 || resp.ParentSync.Recipients[0].Type != "squad" || resp.ParentSync.Recipients[0].ID != squad {
		t.Fatalf("squad recipients = %+v", resp.ParentSync.Recipients)
	}

	var taskID string
	dbfx.QueryRow(t, `SELECT id::text FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2`, child, leader).Scan(&taskID)
	task, err := testHandler.Queries.GetAgentTask(context.Background(), parseUUID(taskID))
	if err != nil {
		t.Fatalf("load sync task: %v", err)
	}
	if !task.IsLeaderTask || uuidToString(task.SquadID) != squad {
		t.Fatalf("sync run leader/squad = %v/%s, want true/%s", task.IsLeaderTask, uuidToString(task.SquadID), squad)
	}
}

// TestPreviewSyncLocksWhenRoutingAlreadyReachesSquadLeader covers the locked
// toggle state (R4.0-6): when the child card's own assignee is the parent
// squad's leader, the ordinary comment routing already reaches the parent owner,
// so the empty-composer preview must report already_routed and the client locks
// the box instead of promising a second delivery.
func TestPreviewSyncLocksWhenRoutingAlreadyReachesSquadLeader(t *testing.T) {
	leader := createHandlerTestAgent(t, "Locked sync leader", nil)
	squad := dbfx.Squad(t, "Locked sync squad", leader)
	parent := dbfx.Issue(t, "Locked sync parent", testutil.Cols{
		"assignee_type": "squad", "assignee_id": squad, "status": "in_progress",
	})
	child := dbfx.Issue(t, "Locked sync child", testutil.Cols{
		"parent_issue_id": parent, "assignee_type": "agent", "assignee_id": leader,
	})

	var preview CommentTriggerPreviewResponse
	testutil.Call(t, testHandler.PreviewCommentTriggers,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments/trigger-preview", map[string]any{
			"content":              "",
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(200).JSON(&preview)
	if preview.ParentSync == nil || preview.ParentSync.Mode != parentSyncModeAgent {
		t.Fatalf("preview parent_sync = %+v", preview.ParentSync)
	}
	if !preview.ParentSync.AlreadyRouted {
		t.Fatal("routing already reaches the parent squad leader; the toggle must lock")
	}
}

// newPrivateParentAgent creates a parent-card agent owned by someone else and
// not invocable by the workspace.
func newPrivateParentAgent(t *testing.T, name, ownerID string) string {
	t.Helper()
	return dbfx.Agent(t, name, testRuntimeID, testutil.Cols{
		"owner_id":        ownerID,
		"visibility":      "private",
		"permission_mode": "private",
	})
}

// TestParentSyncPermissionDowngrade covers B11 item 6: without invoke permission
// on the parent's private agent the sync degrades to the dispatcher's inbox; once
// the permission is granted the same flow runs the agent.
func TestParentSyncPermissionDowngrade(t *testing.T) {
	owner := dbfx.User(t, "Sync private agent owner", "sync-private-owner@example.test")
	dbfx.Member(t, testWorkspaceID, owner, "member")
	author := dbfx.User(t, "Sync private agent author", "sync-private-author@example.test")
	dbfx.Member(t, testWorkspaceID, author, "member")

	parentAgent := newPrivateParentAgent(t, "Sync private parent agent", owner)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)

	create := func(t *testing.T, content string) CommentResponse {
		t.Helper()
		var resp CommentResponse
		testutil.Call(t, testHandler.CreateComment,
			withURLParam(newRequestAs(author, http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
				"content":              content,
				"sync_parent_assignee": true,
			}), "id", child)).
			Want(201).JSON(&resp)
		return resp
	}

	denied := create(t, "please take a look")
	// The parent card was created by the fixture owner: the dispatcher fallback.
	assertParentSync(t, denied, parentSyncModeDispatcher, false, parentSyncDeliveryNotified)
	if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 0 {
		t.Fatalf("private agent was woken %d time(s) despite no permission", tasks)
	}
	if inbox := parentSyncInboxCount(t, child, testUserID); inbox != 1 {
		t.Fatalf("dispatcher inbox notifications = %d, want 1", inbox)
	}

	// Open the agent to the workspace, then the same flow runs it.
	dbfx.Exec(t, `UPDATE agent SET permission_mode = 'public_to' WHERE id = $1`, parentAgent)
	dbfx.Exec(t, `INSERT INTO agent_invocation_target (agent_id, target_type, target_id) VALUES ($1, 'workspace', $2) ON CONFLICT DO NOTHING`, parentAgent, testWorkspaceID)

	allowed := create(t, "second look now that access is granted")
	assertParentSync(t, allowed, parentSyncModeAgent, false, parentSyncDeliveryQueued)
	if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 1 {
		t.Fatalf("granted sync runs for the parent agent = %d, want 1", tasks)
	}
}

// TestParentSyncUsesCommenterAuthorityNotDispatcher covers B11 item 7: an agent
// commenter's own originator decides whether the parent agent can be woken —
// the dispatcher's separate grant is never borrowed.
func TestParentSyncUsesCommenterAuthorityNotDispatcher(t *testing.T) {
	agentOwner := dbfx.User(t, "Sync chain agent owner", "sync-chain-owner@example.test")
	dbfx.Member(t, testWorkspaceID, agentOwner, "member")
	commenterOriginator := dbfx.User(t, "Sync chain commenter human", "sync-chain-human@example.test")
	dbfx.Member(t, testWorkspaceID, commenterOriginator, "member")
	commenterAgent := dbfx.Agent(t, "Sync chain commenter agent", testRuntimeID, testutil.Cols{
		"owner_id": agentOwner, "visibility": "workspace", "permission_mode": "public_to",
	})

	agentCommenterTask := func(t *testing.T, issueID, originator string) string {
		t.Helper()
		return dbfx.Task(t, commenterAgent, testutil.Cols{
			"issue_id":            issueID,
			"runtime_id":          testRuntimeID,
			"status":              "running",
			"started_at":          testutil.Raw("now()"),
			"originator_user_id":  originator,
			"accountable_user_id": originator,
		})
	}
	commentAsAgent := func(t *testing.T, issueID, taskID, content string) CommentResponse {
		t.Helper()
		req := withURLParam(newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", map[string]any{
			"content":              content,
			"sync_parent_assignee": true,
		}), "id", issueID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", commenterAgent)
		req.Header.Set("X-Task-ID", taskID)
		var resp CommentResponse
		testutil.Call(t, testHandler.CreateComment, req).Want(201).JSON(&resp)
		return resp
	}

	t.Run("commenter grant runs the parent agent", func(t *testing.T) {
		parentAgent := dbfx.Agent(t, "Sync chain allowed parent", testRuntimeID, testutil.Cols{
			"owner_id": agentOwner, "visibility": "workspace", "permission_mode": "public_to",
		})
		dbfx.Exec(t, `INSERT INTO agent_invocation_target (agent_id, target_type, target_id) VALUES ($1, 'member', $2)`, parentAgent, commenterOriginator)
		child, _, _ := parentSyncFixture(t, "agent", parentAgent)
		taskID := agentCommenterTask(t, child, commenterOriginator)

		resp := commentAsAgent(t, child, taskID, "the parent owner should read this")
		assertParentSync(t, resp, parentSyncModeAgent, false, parentSyncDeliveryQueued)
		if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 1 {
			t.Fatalf("parent agent runs = %d, want 1", tasks)
		}
	})

	t.Run("dispatcher-only grant is not borrowed", func(t *testing.T) {
		parentAgent := dbfx.Agent(t, "Sync chain dispatcher-only parent", testRuntimeID, testutil.Cols{
			"owner_id": agentOwner, "visibility": "workspace", "permission_mode": "public_to",
		})
		// Only the dispatcher (the fixture creator) may invoke it.
		dbfx.Exec(t, `INSERT INTO agent_invocation_target (agent_id, target_type, target_id) VALUES ($1, 'member', $2)`, parentAgent, testUserID)
		child, _, _ := parentSyncFixture(t, "agent", parentAgent)
		taskID := agentCommenterTask(t, child, commenterOriginator)

		resp := commentAsAgent(t, child, taskID, "this needs a human look")
		if resp.ParentSync == nil || resp.ParentSync.Mode != parentSyncModeDispatcher {
			t.Fatalf("parent_sync = %+v, want dispatcher fallback (dispatcher authority not borrowed)", resp.ParentSync)
		}
		if resp.ParentSync.DeliveryStatus != parentSyncDeliveryNotified {
			t.Fatalf("delivery = %q, want notified", resp.ParentSync.DeliveryStatus)
		}
		if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 0 {
			t.Fatalf("parent agent woke %d time(s) using the dispatcher's grant", tasks)
		}
		if inbox := parentSyncInboxCount(t, child, testUserID); inbox != 1 {
			t.Fatalf("dispatcher inbox notifications = %d, want 1", inbox)
		}
	})
}

// TestParentSyncFallbacks covers B11 item 8: with the dispatcher gone and the
// assignee deleted, owner/admin gets the notification; when no fallback exists
// at all, the preview is unavailable and a true submission fails before the
// comment is written.
func TestParentSyncFallbacks(t *testing.T) {
	t.Run("owner admin notified when dispatcher and assignee are gone", func(t *testing.T) {
		author := dbfx.User(t, "Sync fallback author", "sync-fallback-author@example.test")
		dbfx.Member(t, testWorkspaceID, author, "member")
		leaver := dbfx.User(t, "Sync fallback leaver", "sync-fallback-leaver@example.test")
		dbfx.Member(t, testWorkspaceID, leaver, "member")
		parentAgent := createHandlerTestAgent(t, "Sync fallback deleted agent", nil)
		parent := dbfx.Issue(t, "Sync fallback parent", testutil.Cols{
			"creator_id": leaver, "assignee_type": "agent", "assignee_id": parentAgent, "status": "in_progress",
		})
		childAgent := createHandlerTestAgent(t, "Sync fallback child owner", nil)
		child := dbfx.Issue(t, "Sync fallback child", testutil.Cols{
			"parent_issue_id": parent, "assignee_type": "agent", "assignee_id": childAgent,
		})
		// The assignee is deleted and the dispatcher has left the workspace.
		dbfx.Exec(t, `DELETE FROM agent WHERE id = $1`, parentAgent)
		dbfx.Exec(t, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, leaver)

		var resp CommentResponse
		testutil.Call(t, testHandler.CreateComment,
			withURLParam(newRequestAs(author, http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
				"content":              "nobody owns the parent now",
				"sync_parent_assignee": true,
			}), "id", child)).
			Want(201).JSON(&resp)
		if resp.ParentSync == nil || resp.ParentSync.Mode != parentSyncModeAdmin {
			t.Fatalf("parent_sync = %+v, want admin fallback", resp.ParentSync)
		}
		if resp.ParentSync.DeliveryStatus != parentSyncDeliveryNotified {
			t.Fatalf("delivery = %q, want notified", resp.ParentSync.DeliveryStatus)
		}
		if inbox := parentSyncInboxCount(t, child, testUserID); inbox != 1 {
			t.Fatalf("owner/admin inbox notifications = %d, want 1", inbox)
		}
	})

	t.Run("no fallback at all rejects before writing", func(t *testing.T) {
		workspace := dbfx.Workspace(t, "Sync no fallback ws", "sync-no-fallback-"+timeNowSuffix())
		dbfx.Member(t, workspace, testUserID, "member")
		leaver := dbfx.User(t, "Sync no fallback leaver", "sync-no-fallback-leaver@example.test")
		dbfx.Member(t, workspace, leaver, "member")
		parent := dbfx.Issue(t, "Sync no fallback parent", testutil.Cols{
			"workspace_id": workspace, "creator_id": leaver,
			"assignee_type": "agent", "assignee_id": "00000000-0000-0000-0000-0000000000aa",
			"status": "in_progress",
		})
		childAgent := createHandlerTestAgent(t, "Sync no fallback child owner", nil)
		child := dbfx.Issue(t, "Sync no fallback child", testutil.Cols{
			"workspace_id": workspace, "parent_issue_id": parent,
			"assignee_type": "agent", "assignee_id": childAgent,
		})
		dbfx.Exec(t, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, workspace, leaver)

		var preview CommentTriggerPreviewResponse
		testutil.Call(t, testHandler.PreviewCommentTriggers,
			withURLParam(r4Request(http.MethodPost, "/api/issues/"+child+"/comments/trigger-preview", workspace, map[string]any{
				"content":              "needs a decision",
				"sync_parent_assignee": true,
			}), "id", child)).
			Want(200).JSON(&preview)
		if preview.ParentSync == nil || preview.ParentSync.Mode != parentSyncModeUnavailable {
			t.Fatalf("preview parent_sync = %+v, want unavailable", preview.ParentSync)
		}
		if preview.ParentSync.Reason == "" {
			t.Fatal("unavailable preview carries no reason")
		}

		testutil.Call(t, testHandler.CreateComment,
			withURLParam(r4Request(http.MethodPost, "/api/issues/"+child+"/comments", workspace, map[string]any{
				"content":              "needs a decision",
				"sync_parent_assignee": true,
			}), "id", child)).
			Want(400)
		if comments := parentSyncCommentCount(t, child); comments != 0 {
			t.Fatalf("failed sync wrote %d comment(s), want none", comments)
		}
	})
}

// timeNowSuffix keeps the temp workspace slug unique.
func timeNowSuffix() string {
	return uuid.NewString()
}
