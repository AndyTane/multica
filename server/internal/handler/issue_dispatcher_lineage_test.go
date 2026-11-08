package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func dispatcherTask(t *testing.T, agent string, over ...testutil.Cols) string {
	t.Helper()
	cols := testutil.Cols{
		"runtime_id": testRuntimeID, "originator_user_id": testUserID,
		"accountable_user_id": testUserID, "originator_source": "direct_human",
	}
	for _, extra := range over {
		for k, v := range extra {
			cols[k] = v
		}
	}
	return dbfx.Task(t, agent, cols)
}

func assertDispatcher(t *testing.T, id, user string, active bool) {
	t.Helper()
	rows, err := testHandler.Queries.ListIssueDispatchers(context.Background(), db.ListIssueDispatchersParams{
		WorkspaceID: parseUUID(testWorkspaceID), Ids: []pgtype.UUID{parseUUID(id)},
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("dispatcher rows: %v, %v", rows, err)
	}
	if uuidToString(rows[0].DispatcherUserID) != user || rows[0].DispatcherActive != active {
		t.Fatalf("dispatcher = %s/%v, want %s/%v", uuidToString(rows[0].DispatcherUserID), rows[0].DispatcherActive, user, active)
	}
}

func TestDispatcherParentLineage(t *testing.T) {
	agent := dbfx.Agent(t, "Lineage creator", "")
	rootTask := dispatcherTask(t, agent)
	other := dbfx.User(t, "Other dispatcher", "lineage-other@example.test")
	dbfx.Member(t, testWorkspaceID, other, "member")
	agentCard := func(t *testing.T, parent, task string) string {
		cols := testutil.Cols{"creator_type": "agent", "creator_id": agent}
		if parent != "" {
			cols["parent_issue_id"] = parent
		}
		if task != "" {
			cols["origin_type"] = "agent_create"
			cols["origin_id"] = task
		}
		return dbfx.Issue(t, "Lineage card", cols)
	}
	t.Run("member child owns its dispatcher", func(t *testing.T) {
		parent := dbfx.Issue(t, "Parent member")
		child := dbfx.Issue(t, "Child member", testutil.Cols{"parent_issue_id": parent, "creator_id": other})
		assertDispatcher(t, child, other, true)
	})
	t.Run("multilevel inheritance is the resolved parent's result", func(t *testing.T) {
		root := dbfx.Issue(t, "Member root")
		otherTask := dispatcherTask(t, agent, testutil.Cols{"originator_user_id": other, "accountable_user_id": other})
		parent := agentCard(t, root, otherTask)
		child := agentCard(t, parent, otherTask)
		assertDispatcher(t, parent, testUserID, true)
		assertDispatcher(t, child, testUserID, true)
	})
	t.Run("nearest member stops inheritance", func(t *testing.T) {
		root := dbfx.Issue(t, "Distant member")
		parent := dbfx.Issue(t, "Near member", testutil.Cols{"parent_issue_id": root, "creator_id": other})
		assertDispatcher(t, agentCard(t, parent, rootTask), other, true)
	})
	t.Run("departed parent is retained", func(t *testing.T) {
		left := dbfx.User(t, "Left dispatcher", "lineage-left@example.test")
		root := dbfx.Issue(t, "Active distant member")
		parent := dbfx.Issue(t, "Departed parent", testutil.Cols{"creator_id": left, "parent_issue_id": root})
		assertDispatcher(t, agentCard(t, parent, rootTask), left, false)
	})
	t.Run("unresolved ancestor falls back to own run", func(t *testing.T) {
		assertDispatcher(t, agentCard(t, agentCard(t, "", ""), rootTask), testUserID, true)
	})
	t.Run("missing parent terminates", func(t *testing.T) {
		id := agentCard(t, "", rootTask)
		tx, err := testPool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		// Simulate a damaged legacy reference without changing the shared schema
		// or disabling integrity checks on any other connection.
		if _, err = tx.Exec(context.Background(), "SET LOCAL session_replication_role=replica"); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(context.Background(), "UPDATE issue SET parent_issue_id='00000000-0000-0000-0000-000000000001' WHERE id=$1", id); err != nil {
			t.Fatal(err)
		}
		rows, err := testHandler.Queries.WithTx(tx).ListIssueDispatchers(context.Background(), db.ListIssueDispatchersParams{WorkspaceID: parseUUID(testWorkspaceID), Ids: []pgtype.UUID{parseUUID(id)}})
		if err != nil || len(rows) != 1 || uuidToString(rows[0].DispatcherUserID) != testUserID || !rows[0].DispatcherActive {
			t.Fatalf("missing parent fallback: %v %v", rows, err)
		}
	})
	t.Run("foreign parent cannot supply a dispatcher", func(t *testing.T) {
		ws := dbfx.Workspace(t, "Foreign parent", "dispatcher-foreign-parent")
		parent := dbfx.Issue(t, "Foreign parent", testutil.Cols{"workspace_id": ws, "creator_id": other})
		assertDispatcher(t, agentCard(t, parent, ""), "", false)
	})
	t.Run("cycle terminates without selecting an arbitrary member", func(t *testing.T) {
		a := agentCard(t, "", rootTask)
		b := agentCard(t, a, rootTask)
		dbfx.Exec(t, "UPDATE issue SET parent_issue_id=$2 WHERE id=$1", a, b)
		assertDispatcher(t, a, "", false)
		assertDispatcher(t, b, "", false)
	})
}

func TestDispatcherTrustedTaskLineage(t *testing.T) {
	agent := dbfx.Agent(t, "Task lineage creator", "")
	otherAgent := dbfx.Agent(t, "Task lineage other", "")
	ws := dbfx.Workspace(t, "Foreign tasks", "dispatcher-foreign-tasks")
	foreignAgent := dbfx.Agent(t, "Foreign agent", "", testutil.Cols{"workspace_id": ws})
	root := dispatcherTask(t, agent)
	delegated := dispatcherTask(t, otherAgent, testutil.Cols{"originator_source": "delegation", "delegated_from_task_id": root})
	comment := dispatcherTask(t, agent, testutil.Cols{"originator_source": "comment_source", "delegated_from_task_id": delegated})
	for _, tc := range []struct {
		name string
		task string
		user string
	}{
		{"direct human", root, testUserID},
		{"delegated human", comment, testUserID},
		{"wrong creator", dispatcherTask(t, otherAgent), ""},
		{"foreign origin", dispatcherTask(t, foreignAgent), ""},
		{"foreign ancestor", dispatcherTask(t, agent, testutil.Cols{"originator_source": "delegation", "delegated_from_task_id": dispatcherTask(t, foreignAgent)}), ""},
		{"missing ancestor", dispatcherTask(t, agent, testutil.Cols{"originator_source": "delegation", "delegated_from_task_id": "00000000-0000-0000-0000-000000000001"}), ""},
		{"unlabelled origin", dispatcherTask(t, agent, testutil.Cols{"originator_source": nil}), ""},
		{"null originator", dispatcherTask(t, agent, testutil.Cols{"originator_user_id": nil, "originator_source": "owner_fallback"}), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := dbfx.Issue(t, tc.name, testutil.Cols{"creator_type": "agent", "creator_id": agent, "origin_type": "agent_create", "origin_id": tc.task})
			assertDispatcher(t, id, tc.user, tc.user != "")
		})
	}
	for _, source := range []string{"trigger_owner", "rule_owner", "owner_fallback", "backfill", "unattributed"} {
		t.Run(source+" and its delegation stay unattributed", func(t *testing.T) {
			cols := testutil.Cols{"originator_source": source}
			if source != "trigger_owner" {
				cols["originator_user_id"] = nil
			}
			task := dispatcherTask(t, agent, cols)
			for depth := 0; depth < 3; depth++ {
				id := dbfx.Issue(t, "Automation card", testutil.Cols{"creator_type": "agent", "creator_id": agent, "origin_type": "agent_create", "origin_id": task})
				assertDispatcher(t, id, "", false)
				cols["originator_source"] = "delegation"
				cols["delegated_from_task_id"] = task
				task = dispatcherTask(t, agent, cols)
			}
		})
	}
	t.Run("mismatched human along the chain fails closed", func(t *testing.T) {
		other := dbfx.User(t, "Different root", "lineage-different-root@example.test")
		task := dispatcherTask(t, agent, testutil.Cols{"originator_source": "delegation", "originator_user_id": other, "accountable_user_id": other, "delegated_from_task_id": root})
		id := dbfx.Issue(t, "Mismatched chain", testutil.Cols{"creator_type": "agent", "creator_id": agent, "origin_type": "agent_create", "origin_id": task})
		assertDispatcher(t, id, "", false)
	})
	t.Run("task cycle fails closed", func(t *testing.T) {
		a := dispatcherTask(t, agent, testutil.Cols{"originator_source": "delegation"})
		b := dispatcherTask(t, agent, testutil.Cols{"originator_source": "comment_source", "delegated_from_task_id": a})
		dbfx.Exec(t, "UPDATE agent_task_queue SET delegated_from_task_id=$2 WHERE id=$1", a, b)
		id := dbfx.Issue(t, "Task cycle", testutil.Cols{"creator_type": "agent", "creator_id": agent, "origin_type": "agent_create", "origin_id": a})
		assertDispatcher(t, id, "", false)
	})
	t.Run("long verified chain has no silent depth cutoff", func(t *testing.T) {
		task := root
		for depth := 0; depth < 40; depth++ {
			task = dispatcherTask(t, agent, testutil.Cols{"originator_source": "delegation", "delegated_from_task_id": task})
		}
		id := dbfx.Issue(t, "Deep task chain", testutil.Cols{"creator_type": "agent", "creator_id": agent, "origin_type": "agent_create", "origin_id": task})
		assertDispatcher(t, id, testUserID, true)
	})
}

func TestDispatcherCreateAndAtomicReview(t *testing.T) {
	listAwaiting := func(t *testing.T, query string) (map[string]int, []IssueResponse, int64) {
		t.Helper()
		var out struct {
			Issues []IssueResponse `json:"issues"`
			Total  int64           `json:"total"`
		}
		testutil.Call(t, testHandler.ListIssues, r4Request(http.MethodGet, "/api/issues?workspace_id="+testWorkspaceID+query, testWorkspaceID, nil)).Want(200).JSON(&out)
		ids := map[string]int{}
		for _, row := range out.Issues {
			ids[row.ID] = 1
		}
		return ids, out.Issues, out.Total
	}
	agent := dbfx.Agent(t, "Real creator", "")
	advancer := dbfx.Agent(t, "Real advancer", "")
	root := dispatcherTask(t, agent)
	for _, parent := range []string{"", dbfx.Issue(t, "Real parent")} {
		dbfx.Exec(t, "UPDATE workspace SET issue_counter=GREATEST(issue_counter,(SELECT COALESCE(MAX(number),0) FROM issue WHERE workspace_id=$1)) WHERE id=$1", testWorkspaceID)
		body := map[string]any{"title": "Real agent card", "assignee_type": "agent", "assignee_id": advancer}
		if parent != "" {
			body["parent_issue_id"] = parent
		}
		var created IssueResponse
		req := testutil.WithHeaders(newRequest(http.MethodPost, "/api/issues?workspace_id="+testWorkspaceID, body), "X-Agent-ID", agent, "X-Task-ID", root, "X-Actor-Source", "task_token")
		testutil.Call(t, testHandler.CreateIssue, req).Want(201).JSON(&created)
		dbfx.Cleanup(t, "DELETE FROM issue WHERE id=$1", created.ID)
		assertDispatcher(t, created.ID, testUserID, true)
		var out IssueResponse
		testutil.Call(t, testHandler.SubmitIssueReview, withURLParam(newRequest(http.MethodPost, "/api/issues/"+created.ID+"/review", map[string]any{
			"status": "in_review", "expected_revision": created.Revision,
			"review_metadata": map[string]any{"review_by": "user", "review_kind": "approve", "review_action": "Check delivery"},
		}), "id", created.ID)).Want(200).JSON(&out)
		if out.Metadata["review_user"] != testUserID || out.Status != "in_review" || out.Revision != created.Revision+1 || out.Metadata["review_action"] != "Check delivery" {
			t.Fatalf("atomic review: %+v", out)
		}
		var detail IssueResponse
		testutil.Call(t, testHandler.GetIssue, withURLParam(newRequest(http.MethodGet, "/api/issues/"+created.ID, nil), "id", created.ID)).Want(200).JSON(&detail)
		if detail.DispatcherUserID == nil || *detail.DispatcherUserID != testUserID || detail.DispatcherActive == nil || !*detail.DispatcherActive {
			t.Fatalf("detail dispatcher: %+v", detail)
		}
		mine, _, _ := listAwaiting(t, "&awaiting=mine&limit=100")
		if _, ok := mine[created.ID]; !ok {
			t.Fatal("published card missing from awaiting=mine")
		}
		filtered, _, _ := listAwaiting(t, "&dispatcher_user_id="+testUserID+"&limit=100")
		if _, ok := filtered[created.ID]; !ok {
			t.Fatal("card missing from dispatcher filter")
		}
	}
	for _, departed := range []bool{false, true} {
		cols := testutil.Cols{"status": "in_progress", "assignee_type": "agent", "assignee_id": advancer, "creator_type": "agent", "creator_id": agent, "metadata": `{"keep":true}`}
		want := "no trusted member source"
		if departed {
			left := dbfx.User(t, "Departed reviewer", "dispatcher-departed-review@example.test")
			cols["parent_issue_id"] = dbfx.Issue(t, "Departed parent", testutil.Cols{"creator_id": left})
			want = "dispatcher has left the workspace"
		}
		id, before := reviewFixture(t, fmt.Sprint("Rejected ", departed), cols)
		rec := testutil.Call(t, testHandler.SubmitIssueReview, withURLParam(newRequest(http.MethodPost, "/api/issues/"+id+"/review", map[string]any{
			"status": "in_review", "expected_revision": before.Revision,
			"review_metadata": map[string]any{"review_by": "user", "review_user": testUserID, "review_action": "Check"},
		}), "id", id)).Want(400)
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("missing reason %q: %s", want, rec.Body.String())
		}
		after, _ := reviewMetadataOf(t, id)
		if after.Status != before.Status || after.Revision != before.Revision || string(after.Metadata) != string(before.Metadata) {
			t.Fatal("rejected review mutated issue")
		}
		// Simulate an old published mark: list anomaly reason must agree with rejection.
		dbfx.Exec(t, `UPDATE issue SET status='in_review', metadata='{"review_by":"user","review_action":"Check"}' WHERE id=$1`, id)
		row, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
		reason := testHandler.loadAwaitingProblems(context.Background(), parseUUID(testWorkspaceID), []db.Issue{row})[id]
		wantCode := awaitingProblemNoDispatcher
		if departed {
			wantCode = awaitingProblemDispatcherInactive
		}
		if reason != wantCode {
			t.Fatalf("problem=%s, want %s", reason, wantCode)
		}
	}
}

func TestDispatcherCreateRejectsInvalidTaskWorkspace(t *testing.T) {
	agent := dbfx.Agent(t, "Trusted creator", "")
	other := dbfx.Agent(t, "Wrong creator", "")
	ws := dbfx.Workspace(t, "Other task workspace", "dispatcher-create-foreign")
	foreign := dbfx.Agent(t, "Foreign creator", "", testutil.Cols{"workspace_id": ws})
	for _, tc := range []struct{ name, actor, task string }{
		{"wrong agent", agent, dispatcherTask(t, other)},
		{"foreign workspace", foreign, dispatcherTask(t, foreign)},
		{"missing task", agent, "00000000-0000-0000-0000-000000000001"},
		{"invalid task", agent, "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := dbfx.Count(t, "SELECT count(*) FROM issue WHERE workspace_id=$1", testWorkspaceID)
			for _, quick := range []bool{false, true} {
				body := map[string]any{"title": "Untrusted origin"}
				if quick && tc.task != "invalid" {
					body["origin_type"] = "quick_create"
					body["origin_id"] = tc.task
				}
				req := testutil.WithHeaders(newRequest(http.MethodPost, "/api/issues", body), "X-Actor-Source", "task_token", "X-Agent-ID", tc.actor, "X-Task-ID", tc.task)
				testutil.Call(t, testHandler.CreateIssue, req).Want(403)
			}
			if got := dbfx.Count(t, "SELECT count(*) FROM issue WHERE workspace_id=$1", testWorkspaceID); got != before {
				t.Fatal("rejected create wrote an issue")
			}
		})
	}
}
