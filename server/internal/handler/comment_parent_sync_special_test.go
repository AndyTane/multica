package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// parentSyncTaskCount counts runs for one agent on the child card.
func parentSyncTaskCount(t *testing.T, issueID, agentID string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2`, issueID, agentID)
}

// parentSyncCommentCount counts comments on the child card.
func parentSyncCommentCount(t *testing.T, issueID string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, issueID)
}

// parentSyncInboxCount counts sync notifications delivered to a member.
func parentSyncInboxCount(t *testing.T, issueID, recipient string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM inbox_item WHERE issue_id = $1 AND recipient_id = $2 AND type = $3`, issueID, recipient, parentSyncInboxType)
}

// assertParentSync checks the server-decided result recorded on the comment.
func assertParentSync(t *testing.T, resp CommentResponse, mode string, alreadyRouted bool, delivery string) {
	t.Helper()
	if resp.ParentSync == nil {
		t.Fatal("comment response carries no parent_sync")
	}
	if resp.ParentSync.Mode != mode {
		t.Fatalf("parent_sync.mode = %q, want %q (%+v)", resp.ParentSync.Mode, mode, resp.ParentSync)
	}
	if resp.ParentSync.AlreadyRouted != alreadyRouted {
		t.Fatalf("parent_sync.already_routed = %v, want %v (%+v)", resp.ParentSync.AlreadyRouted, alreadyRouted, resp.ParentSync)
	}
	if resp.ParentSync.DeliveryStatus != delivery {
		t.Fatalf("parent_sync.delivery_status = %q, want %q (%+v)", resp.ParentSync.DeliveryStatus, delivery, resp.ParentSync)
	}
}

// TestParentSyncExplicitMentionDedupes covers B11 item 1: an explicit @ of the
// parent card's assignee already routes there, so the sync must not enqueue a
// second run for the same agent.
func TestParentSyncExplicitMentionDedupes(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "Sync mention dedupe agent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)

	resp := createSyncedComment(t, child, "[@P](mention://agent/"+parentAgent+") please look at this", 201)
	assertParentSync(t, resp, parentSyncModeAgent, true, parentSyncDeliveryAlready)
	if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 1 {
		t.Fatalf("runs for the parent agent = %d, want exactly the normal @mention run", tasks)
	}
}

// TestParentSyncReplyToParentAssigneeDedupes covers B11 item 2: replying to a
// comment written by the parent assignee already wakes it, so the sync must
// report already_routed and add no delivery.
func TestParentSyncReplyToParentAssigneeDedupes(t *testing.T) {
	parentAgent := createHandlerTestAgent(t, "Sync reply parent agent", nil)
	child, _, _ := parentSyncFixture(t, "agent", parentAgent)
	rootID := dbfx.Comment(t, child, "earlier from the parent owner", testutil.Cols{
		"author_type": "agent", "author_id": parentAgent,
	})

	var resp CommentResponse
	testutil.Call(t, testHandler.CreateComment,
		withURLParam(newRequest(http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
			"content":              "answering your point",
			"parent_id":            rootID,
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(201).JSON(&resp)
	assertParentSync(t, resp, parentSyncModeAgent, true, parentSyncDeliveryAlready)
	if tasks := parentSyncTaskCount(t, child, parentAgent); tasks != 1 {
		t.Fatalf("runs for the parent agent = %d, want exactly the thread-parent run", tasks)
	}
}

// TestParentSyncMemberThreadReplyAddsNoFallback covers B11 item 3: a plain
// member-to-member thread reply starts no run, and the sync only notifies the
// parent member's inbox.
func TestParentSyncMemberThreadReplyAddsNoFallback(t *testing.T) {
	parentMember := dbfx.User(t, "Sync member parent", "sync-member-parent@example.test")
	dbfx.Member(t, testWorkspaceID, parentMember, "member")
	author := dbfx.User(t, "Sync thread author", "sync-thread-author@example.test")
	dbfx.Member(t, testWorkspaceID, author, "member")
	replier := dbfx.User(t, "Sync thread replier", "sync-thread-replier@example.test")
	dbfx.Member(t, testWorkspaceID, replier, "member")

	child, _, _ := parentSyncFixture(t, "member", parentMember)
	childAgent := dbfx.QueryRow(t, `SELECT assignee_id::text FROM issue WHERE id = $1`, child)
	var childAgentID string
	childAgent.Scan(&childAgentID)

	rootID := dbfx.Comment(t, child, "question from one member", testutil.Cols{
		"author_type": "member", "author_id": author,
	})

	var resp CommentResponse
	testutil.Call(t, testHandler.CreateComment,
		withURLParam(newRequestAs(replier, http.MethodPost, "/api/issues/"+child+"/comments", map[string]any{
			"content":              "reply from another member",
			"parent_id":            rootID,
			"sync_parent_assignee": true,
		}), "id", child)).
		Want(201).JSON(&resp)
	assertParentSync(t, resp, parentSyncModeMember, false, parentSyncDeliveryNotified)
	if tasks := parentSyncTaskCount(t, child, childAgentID); tasks != 0 {
		t.Fatalf("member-to-member reply started %d agent run(s), want none", tasks)
	}
	if inbox := parentSyncInboxCount(t, child, parentMember); inbox != 1 {
		t.Fatalf("parent member inbox notifications = %d, want 1", inbox)
	}
}

// TestParentSyncSamePersonAddsNothing covers B11 item 4: when the parent
// assignee is already the child's own assignee (normal routing reached it) or
// is the comment author herself, the sync adds no delivery and no self
// notification.
func TestParentSyncSamePersonAddsNothing(t *testing.T) {
	t.Run("same agent as the child assignee", func(t *testing.T) {
		agent := createHandlerTestAgent(t, "Sync same agent", nil)
		parent := dbfx.Issue(t, "Sync same-agent parent", testutil.Cols{
			"assignee_type": "agent", "assignee_id": agent, "status": "in_progress",
		})
		child := dbfx.Issue(t, "Sync same-agent child", testutil.Cols{
			"parent_issue_id": parent, "assignee_type": "agent", "assignee_id": agent,
		})
		resp := createSyncedComment(t, child, "please check this", 201)
		assertParentSync(t, resp, parentSyncModeAgent, true, parentSyncDeliveryAlready)
		if tasks := parentSyncTaskCount(t, child, agent); tasks != 1 {
			t.Fatalf("runs for the shared agent = %d, want the single normal run", tasks)
		}
	})

	t.Run("parent assignee is the commenter", func(t *testing.T) {
		child, _, _ := parentSyncFixture(t, "member", testUserID)
		resp := createSyncedComment(t, child, "note for myself", 201)
		assertParentSync(t, resp, parentSyncModeMember, true, parentSyncDeliveryAlready)
		if inbox := parentSyncInboxCount(t, child, testUserID); inbox != 0 {
			t.Fatalf("self notifications = %d, want 0", inbox)
		}
	})
}
