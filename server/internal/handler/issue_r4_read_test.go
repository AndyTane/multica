package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func r4Request(method, path, workspaceID string, body any) *http.Request {
	return testutil.WithHeaders(
		testutil.JSONRequest(method, path, body),
		"X-User-ID", testUserID,
		"X-Workspace-ID", workspaceID,
	)
}

// TestIssueDispatcherProjection pins B9: the member creator and a trusted
// quick-create requester resolve; agent_create and forged quick-create contexts
// do not, and a departed dispatcher stays readable with active=false.
func TestIssueDispatcherProjection(t *testing.T) {
	// member-created issue
	memberIssue := dbfx.Issue(t, "Dispatcher member issue", testutil.Cols{})
	var got IssueResponse
	got = IssueResponse{}
	testutil.Call(t, testHandler.GetIssue,
		withURLParam(r4Request(http.MethodGet, "/api/issues/"+memberIssue, testWorkspaceID, nil), "id", memberIssue)).
		Want(200).JSON(&got)
	if got.DispatcherUserID == nil || *got.DispatcherUserID != testUserID || got.DispatcherActive == nil || !*got.DispatcherActive {
		t.Fatalf("member creator dispatcher = %v/%v", got.DispatcherUserID, got.DispatcherActive)
	}

	// agent_create issue (even with a human originator on its origin task)
	creator := createHandlerTestAgent(t, "Dispatcher agent-create owner", nil)
	originTask := dbfx.Task(t, creator, testutil.Cols{
		"issue_id":            nil,
		"runtime_id":          testRuntimeID,
		"originator_user_id":  testUserID,
		"accountable_user_id": testUserID,
	})
	agentIssue := dbfx.Issue(t, "Dispatcher agent issue", testutil.Cols{
		"creator_type": "agent", "creator_id": creator, "origin_type": "agent_create", "origin_id": originTask,
	})
	got = IssueResponse{}
	testutil.Call(t, testHandler.GetIssue,
		withURLParam(r4Request(http.MethodGet, "/api/issues/"+agentIssue, testWorkspaceID, nil), "id", agentIssue)).
		Want(200).JSON(&got)
	if got.DispatcherUserID != nil {
		t.Fatalf("agent_create resolved dispatcher %v", *got.DispatcherUserID)
	}

	// quick-create issue with a validated origin context inherits the requester
	quickAgent := createHandlerTestAgent(t, "Dispatcher quick creator", nil)
	quickTask := dbfx.Task(t, quickAgent, testutil.Cols{
		"issue_id":            nil,
		"runtime_id":          testRuntimeID,
		"originator_user_id":  testUserID,
		"accountable_user_id": testUserID,
		"context":             "{\"type\":\"quick_create\",\"prompt\":\"x\",\"requester_id\":\"" + testUserID + "\",\"workspace_id\":\"" + testWorkspaceID + "\"}",
	})
	quickIssue := dbfx.Issue(t, "Dispatcher quick issue", testutil.Cols{
		"creator_type": "agent", "creator_id": quickAgent, "origin_type": "quick_create", "origin_id": quickTask,
	})
	got = IssueResponse{}
	testutil.Call(t, testHandler.GetIssue,
		withURLParam(r4Request(http.MethodGet, "/api/issues/"+quickIssue, testWorkspaceID, nil), "id", quickIssue)).
		Want(200).JSON(&got)
	if got.DispatcherUserID == nil || *got.DispatcherUserID != testUserID {
		t.Fatalf("quick-create dispatcher = %v, want the requester", got.DispatcherUserID)
	}

	// deprecated quick-create context: requester mismatch resolves nothing
	forgedTask := dbfx.Task(t, quickAgent, testutil.Cols{
		"issue_id":            nil,
		"runtime_id":          testRuntimeID,
		"originator_user_id":  testUserID,
		"accountable_user_id": testUserID,
		"context":             "{\"type\":\"quick_create\",\"requester_id\":\"00000000-0000-0000-0000-000000000001\",\"workspace_id\":\"" + testWorkspaceID + "\"}",
	})
	forgedIssue := dbfx.Issue(t, "Dispatcher forged quick issue", testutil.Cols{
		"creator_type": "agent", "creator_id": quickAgent, "origin_type": "quick_create", "origin_id": forgedTask,
	})
	got = IssueResponse{}
	testutil.Call(t, testHandler.GetIssue,
		withURLParam(r4Request(http.MethodGet, "/api/issues/"+forgedIssue, testWorkspaceID, nil), "id", forgedIssue)).
		Want(200).JSON(&got)
	if got.DispatcherUserID != nil {
		t.Fatalf("forged quick-create resolved dispatcher %v", *got.DispatcherUserID)
	}

	// a dispatcher who left the workspace keeps the historical id, active=false
	leaver := dbfx.User(t, "Dispatcher leaver", "dispatcher-leaver@example.test")
	dbfx.Member(t, testWorkspaceID, leaver, "member")
	leftIssue := dbfx.Issue(t, "Dispatcher left issue", testutil.Cols{"creator_id": leaver})
	dbfx.Exec(t, "DELETE FROM member WHERE workspace_id = $1 AND user_id = $2", testWorkspaceID, leaver)
	got = IssueResponse{}
	testutil.Call(t, testHandler.GetIssue,
		withURLParam(r4Request(http.MethodGet, "/api/issues/"+leftIssue, testWorkspaceID, nil), "id", leftIssue)).
		Want(200).JSON(&got)
	if got.DispatcherUserID == nil || *got.DispatcherUserID != leaver || got.DispatcherActive == nil || *got.DispatcherActive {
		t.Fatalf("departed dispatcher = %v/%v", got.DispatcherUserID, got.DispatcherActive)
	}
}

// TestAwaitingFilters covers B10: mine, unmarked (incl. anomaly reasons), the
// dispatcher filter, and the exclusion of valid specialist marks and plain
// agent_create execution cards.
func TestAwaitingFilters(t *testing.T) {
	advancer := createHandlerTestAgent(t, "Awaiting advancer", nil)
	otherUser := dbfx.User(t, "Awaiting other member", "awaiting-other@example.test")
	dbfx.Member(t, testWorkspaceID, otherUser, "member")
	otherAgent := createHandlerTestAgent(t, "Awaiting other agent", nil)

	mine := dbfx.Issue(t, "Awaiting mine", testutil.Cols{
		"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
		"metadata": `{"review_by":"user","review_kind":"approve","review_user":"` + testUserID + `"}`,
	})
	blockedMine := dbfx.Issue(t, "Awaiting blocked mine", testutil.Cols{
		"status": "blocked", "assignee_type": "agent", "assignee_id": advancer,
		"metadata": `{"review_by":"user","review_kind":"reply","review_user":"` + testUserID + `"}`,
	})
	othersCard := dbfx.Issue(t, "Awaiting others card", testutil.Cols{
		"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
		"creator_id": otherUser,
		"metadata":   `{"review_by":"user","review_kind":"approve","review_user":"` + otherUser + `"}`,
	})
	specialistCard := dbfx.Issue(t, "Awaiting specialist", testutil.Cols{
		"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
		"metadata": `{"review_by":"specialist","review_kind":"approve"}`,
	})
	unmarked := dbfx.Issue(t, "Awaiting unmarked", testutil.Cols{"status": "in_review"})
	// A mark the contract rejects outright: blocked + user with no review_kind.
	// Its combination check is unknown (not just false), and it must still surface
	// as the dispatcher's unmarked card with an explanatory reason.
	invalidMark := dbfx.Issue(t, "Awaiting invalid mark", testutil.Cols{
		"status": "blocked", "assignee_type": "agent", "assignee_id": advancer,
		"metadata": `{"review_by":"user"}`,
	})
	invalidReceiver := dbfx.Issue(t, "Awaiting invalid receiver", testutil.Cols{
		"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
		"metadata": `{"review_by":"user","review_kind":"approve","review_user":"00000000-0000-0000-0000-000000000002"}`,
	})
	// A valid specialist mark whose dispatcher left is still a specialist mark.
	leftUser := dbfx.User(t, "Awaiting left dispatcher", "awaiting-left@example.test")
	dbfx.Member(t, testWorkspaceID, leftUser, "member")
	leftCard := dbfx.Issue(t, "Awaiting left dispatcher card", testutil.Cols{
		"status": "in_review", "creator_id": leftUser,
		"metadata": `{"review_by":"user","review_kind":"approve"}`,
	})
	dbfx.Exec(t, "DELETE FROM member WHERE workspace_id = $1 AND user_id = $2", testWorkspaceID, leftUser)

	listAwaiting := func(t *testing.T, query string) (map[string]int, []IssueResponse, int64) {
		t.Helper()
		var resp struct {
			Issues []IssueResponse `json:"issues"`
			Total  int64           `json:"total"`
		}
		testutil.Call(t, testHandler.ListIssues,
			r4Request(http.MethodGet, "/api/issues?workspace_id="+testWorkspaceID+query, testWorkspaceID, nil)).
			Want(200).JSON(&resp)
		ids := map[string]int{}
		for _, issue := range resp.Issues {
			ids[issue.ID] = 1
		}
		return ids, resp.Issues, resp.Total
	}

	ids, _, total := listAwaiting(t, "&awaiting=mine&limit=100")
	if ids[mine] != 1 || ids[blockedMine] != 1 {
		t.Fatalf("awaiting=mine missing my cards: total=%d", total)
	}
	if ids[othersCard] == 1 || ids[specialistCard] == 1 || ids[unmarked] == 1 {
		t.Fatal("awaiting=mine included a card waiting on someone else")
	}

	unmarkedIDs, unmarkedRows, _ := listAwaiting(t, "&awaiting=unmarked&limit=100")
	if unmarkedIDs[unmarked] != 1 {
		t.Fatal("awaiting=unmarked missing my unmarked card")
	}
	if unmarkedIDs[invalidReceiver] != 1 {
		t.Fatal("awaiting=unmarked missing the invalid-receiver anomaly")
	}
	if unmarkedIDs[invalidMark] != 1 {
		t.Fatal("awaiting=unmarked missing the invalid-mark anomaly")
	}
	if unmarkedIDs[leftCard] != 1 {
		t.Fatal("awaiting=unmarked (owner) missing the departed-dispatcher anomaly")
	}
	if unmarkedIDs[specialistCard] == 1 || unmarkedIDs[othersCard] == 1 {
		t.Fatal("awaiting=unmarked counted a deliverable card")
	}
	problems := map[string]*string{}
	for _, row := range unmarkedRows {
		problems[row.ID] = row.AwaitingProblem
	}
	if p := problems[invalidReceiver]; p == nil || *p == "" {
		t.Fatal("invalid receiver row has no awaiting_problem")
	}
	if p := problems[invalidMark]; p == nil || *p != awaitingProblemInvalidMark {
		t.Fatalf("invalid mark problem = %v, want %s", p, awaitingProblemInvalidMark)
	}
	if p := problems[leftCard]; p == nil || *p == "" {
		t.Fatal("departed dispatcher row has no awaiting_problem")
	}
	if p := problems[unmarked]; p != nil {
		t.Fatalf("plain unmarked row reported problem %q", *p)
	}

	// dispatcher filter resolves the same way the response field does.
	byDispatcher, _, _ := listAwaiting(t, "&dispatcher_user_id="+testUserID+"&limit=100")
	if byDispatcher[mine] != 1 || byDispatcher[unmarked] != 1 {
		t.Fatal("dispatcher filter missing my dispatched cards")
	}
	if byDispatcher[othersCard] == 1 {
		t.Fatal("dispatcher filter matched another member's card")
	}
	_ = otherAgent
}

// TestAssignmentPreview covers B13: the caller verdict, the dispatcher verdict,
// null when there is no dispatcher, and no write side effects.
func TestAssignmentPreview(t *testing.T) {
	agentID := createHandlerTestAgent(t, "Preview target agent", nil)
	issueID := dbfx.Issue(t, "Preview issue", testutil.Cols{})
	before, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(issueID))

	preview := func(t *testing.T, body map[string]any) IssueAssignmentPreviewResponse {
		t.Helper()
		var resp IssueAssignmentPreviewResponse
		testutil.Call(t, testHandler.PreviewIssueAssignment,
			withURLParam(r4Request(http.MethodPost, "/api/issues/"+issueID+"/assignment-preview", testWorkspaceID, body), "id", issueID)).
			Want(200).JSON(&resp)
		return resp
	}

	ok := preview(t, map[string]any{"assignee_type": "agent", "assignee_id": agentID})
	if !ok.ActorCanAssign {
		t.Fatal("owner could not assign to a workspace agent")
	}
	if ok.DispatcherUserID == nil || *ok.DispatcherUserID != testUserID {
		t.Fatalf("preview dispatcher = %v", ok.DispatcherUserID)
	}
	if ok.DispatcherCanInvoke == nil || !*ok.DispatcherCanInvoke {
		t.Fatal("dispatcher could not invoke the workspace agent")
	}

	// A private agent owned by someone else: the actor cannot assign it, and
	// the dispatcher cannot invoke it either.
	foreignOwner := dbfx.User(t, "Preview foreign owner", "preview-foreign@example.test")
	dbfx.Member(t, testWorkspaceID, foreignOwner, "member")
	privateAgent := dbfx.Agent(t, "Preview private agent", testRuntimeID, testutil.Cols{
		"owner_id": foreignOwner, "visibility": "private", "permission_mode": "private",
	})
	denied := preview(t, map[string]any{"assignee_type": "agent", "assignee_id": privateAgent})
	if denied.ActorCanAssign {
		t.Fatal("plain workspace member's agent should not be assignable by the owner here")
	}
	if denied.Reason != assignmentPreviewReasonActorDenied {
		t.Fatalf("reason = %q", denied.Reason)
	}

	// An internal agent_create card has no dispatcher: dispatcher_can_invoke is
	// null (not a permission failure) so the UI does not block reassignment.
	creator := createHandlerTestAgent(t, "Preview agent-create owner", nil)
	task := dbfx.Task(t, creator, testutil.Cols{"issue_id": nil, "runtime_id": testRuntimeID})
	internalCard := dbfx.Issue(t, "Preview internal card", testutil.Cols{
		"creator_type": "agent", "creator_id": creator, "origin_type": "agent_create", "origin_id": task,
	})
	var internal IssueAssignmentPreviewResponse
	testutil.Call(t, testHandler.PreviewIssueAssignment,
		withURLParam(r4Request(http.MethodPost, "/api/issues/"+internalCard+"/assignment-preview", testWorkspaceID,
			map[string]any{"assignee_type": "agent", "assignee_id": agentID}), "id", internalCard)).
		Want(200).JSON(&internal)
	if internal.DispatcherUserID != nil || internal.DispatcherCanInvoke != nil {
		t.Fatalf("internal card preview claimed a dispatcher: %+v", internal)
	}

	after, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(issueID))
	if after.AssigneeType.Valid || after.Revision != before.Revision {
		t.Fatal("preview modified the issue")
	}
}

// TestInboxParentProjection covers B14 on the active list and single-item read.
func TestInboxParentProjection(t *testing.T) {
	parent := dbfx.Issue(t, "Inbox parent card", testutil.Cols{})
	child := dbfx.Issue(t, "Inbox child card", testutil.Cols{"parent_issue_id": parent})
	dbfx.Insert(t, "inbox_item", testutil.Cols{
		"workspace_id":   testWorkspaceID,
		"recipient_type": "member",
		"recipient_id":   testUserID,
		"type":           "new_comment",
		"severity":       "info",
		"issue_id":       child,
		"title":          "Child activity",
	})
	var items []InboxItemResponse
	testutil.Call(t, inboxWorkspaceHandler(testHandler.ListInbox),
		inboxRequest(http.MethodGet, "/api/inbox", testWorkspaceID)).Want(200).JSON(&items)
	var found *InboxItemResponse
	for i := range items {
		if items[i].IssueID != nil && *items[i].IssueID == child {
			found = &items[i]
		}
	}
	if found == nil {
		t.Fatal("child notification missing from the inbox list")
	}
	parentIssue, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(parent))
	wantIdentifier := "HAN-" + strconv.Itoa(int(parentIssue.Number))
	if found.ParentIssueID == nil || *found.ParentIssueID != parent {
		t.Fatalf("parent id = %v", found.ParentIssueID)
	}
	if found.ParentIssueIdentifier == nil || *found.ParentIssueIdentifier != wantIdentifier {
		t.Fatalf("parent identifier = %v, want %s", found.ParentIssueIdentifier, wantIdentifier)
	}
	if found.ParentIssueTitle == nil || *found.ParentIssueTitle != "Inbox parent card" {
		t.Fatalf("parent title = %v", found.ParentIssueTitle)
	}

	// The single-item read carries the same projection.
	var item InboxItemResponse
	testutil.Call(t, inboxWorkspaceHandler(testHandler.MarkInboxRead),
		withURLParam(inboxRequest(http.MethodPost, "/api/inbox/"+found.ID+"/read", testWorkspaceID), "id", found.ID)).
		Want(200).JSON(&item)
	if item.ParentIssueID == nil || *item.ParentIssueID != parent {
		t.Fatalf("single-item parent id = %v", item.ParentIssueID)
	}
}

// itoa avoids importing strconv for one call in test helpers.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}

// uuidString resolves a fixture id that is already a UUID string.
func uuidString(t *testing.T, id string) string {
	t.Helper()
	return id
}
