package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Reproduce the top-level create that failed after a human-requested wait.
func TestDispatcherWakeupCreateAndAtomicReview(t *testing.T) {
	agent := dbfx.Agent(t, "Wakeup creator", "")
	advancer := dbfx.Agent(t, "Wakeup advancer", "")
	root := dispatcherTask(t, agent)
	wakeup := dispatcherTask(t, agent, testutil.Cols{
		"originator_source": "trigger_owner", "trigger_evidence_kind": "issue_wakeup",
		"trigger_evidence_ref_id": "00000000-0000-0000-0000-000000000172", "delegated_from_task_id": root,
	})
	task := dispatcherTask(t, agent, testutil.Cols{"originator_source": "delegation", "delegated_from_task_id": wakeup})
	dbfx.Exec(t, "UPDATE workspace SET issue_counter=GREATEST(issue_counter,(SELECT COALESCE(MAX(number),0) FROM issue WHERE workspace_id=$1)) WHERE id=$1", testWorkspaceID)
	var created IssueResponse
	req := testutil.WithHeaders(newRequest(http.MethodPost, "/api/issues", map[string]any{
		"title": "Top-level after human wait", "assignee_type": "agent", "assignee_id": advancer,
	}), "X-Agent-ID", agent, "X-Task-ID", task, "X-Actor-Source", "task_token")
	testutil.Call(t, testHandler.CreateIssue, req).Want(201).JSON(&created)
	dbfx.Cleanup(t, "DELETE FROM issue WHERE id=$1", created.ID)
	row, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(created.ID))
	if err != nil || row.ParentIssueID.Valid || row.OriginType.String != "agent_create" || uuidToString(row.OriginID) != task {
		t.Fatalf("create provenance: %+v, %v", row, err)
	}
	assertDispatcher(t, created.ID, testUserID, true)
	var out IssueResponse
	testutil.Call(t, testHandler.SubmitIssueReview, withURLParam(newRequest(http.MethodPost, "/api/issues/"+created.ID+"/review", map[string]any{
		"status": "in_review", "expected_revision": created.Revision,
		"review_metadata": map[string]any{"review_by": "user", "review_kind": "approve", "review_action": "Check wakeup delivery"},
	}), "id", created.ID)).Want(200).JSON(&out)
	if out.Status != "in_review" || out.Revision != created.Revision+1 || out.Metadata["review_user"] != testUserID || out.Metadata["review_action"] != "Check wakeup delivery" {
		t.Fatalf("atomic review: %+v", out)
	}
	var detail IssueResponse
	testutil.Call(t, testHandler.GetIssue, withURLParam(newRequest(http.MethodGet, "/api/issues/"+created.ID, nil), "id", created.ID)).Want(200).JSON(&detail)
	if detail.DispatcherUserID == nil || *detail.DispatcherUserID != testUserID || detail.DispatcherActive == nil || !*detail.DispatcherActive {
		t.Fatalf("detail dispatcher: %+v", detail)
	}
	for _, filter := range []string{"&dispatcher_user_id=" + testUserID, "&awaiting=mine"} {
		var list struct {
			Issues []IssueResponse `json:"issues"`
		}
		testutil.Call(t, testHandler.ListIssues, r4Request(http.MethodGet, "/api/issues?workspace_id="+testWorkspaceID+"&limit=100"+filter, testWorkspaceID, nil)).Want(200).JSON(&list)
		found := false
		for _, issue := range list.Issues {
			if issue.ID == created.ID {
				found = true
				if issue.DispatcherUserID == nil || *issue.DispatcherUserID != testUserID || issue.DispatcherActive == nil || !*issue.DispatcherActive {
					t.Fatalf("list dispatcher: %+v", issue)
				}
			}
		}
		if !found {
			t.Fatalf("card missing from %s", filter)
		}
	}
}

func TestDispatcherWakeupTrustBoundary(t *testing.T) {
	agent := dbfx.Agent(t, "Wakeup boundary creator", "")
	root := dispatcherTask(t, agent)
	other := dbfx.User(t, "Different wakeup human", "wakeup-other@example.test")
	ws := dbfx.Workspace(t, "Foreign wakeup source", "dispatcher-wakeup-foreign")
	foreign := dbfx.Agent(t, "Foreign wakeup agent", "", testutil.Cols{"workspace_id": ws})
	automation := dispatcherTask(t, agent, testutil.Cols{"originator_source": "trigger_owner", "trigger_evidence_kind": "autopilot"})
	cycle := dispatcherTask(t, agent, testutil.Cols{"originator_source": "delegation"})
	base := testutil.Cols{
		"originator_source": "trigger_owner", "trigger_evidence_kind": "issue_wakeup",
		"trigger_evidence_ref_id": "00000000-0000-0000-0000-000000000172", "delegated_from_task_id": root,
	}
	for _, tc := range []struct {
		name string
		over testutil.Cols
	}{
		{"no source run", testutil.Cols{"delegated_from_task_id": nil}},
		{"only owner", testutil.Cols{"delegated_from_task_id": nil, "trigger_evidence_ref_id": nil}},
		{"autopilot even with human parent", testutil.Cols{"trigger_evidence_kind": "autopilot"}},
		{"missing evidence kind", testutil.Cols{"trigger_evidence_kind": nil}},
		{"missing evidence ref", testutil.Cols{"trigger_evidence_ref_id": nil}},
		{"context cannot supply evidence", testutil.Cols{"trigger_evidence_kind": nil, "trigger_evidence_ref_id": nil, "context": `{"trigger_evidence_kind":"issue_wakeup","wakeup_id":"00000000-0000-0000-0000-000000000172"}`}},
		{"automation ancestor", testutil.Cols{"delegated_from_task_id": automation}},
		{"null human", testutil.Cols{"originator_user_id": nil}},
		{"mismatched human", testutil.Cols{"originator_user_id": other, "accountable_user_id": other}},
		{"missing task reference", testutil.Cols{"delegated_from_task_id": "00000000-0000-0000-0000-000000000001"}},
		{"foreign task reference", testutil.Cols{"delegated_from_task_id": dispatcherTask(t, foreign)}},
		{"cycle", testutil.Cols{"delegated_from_task_id": cycle}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cols := testutil.Cols{}
			for k, v := range base {
				cols[k] = v
			}
			for k, v := range tc.over {
				cols[k] = v
			}
			human := any(testUserID)
			if value, ok := cols["originator_user_id"]; ok {
				human = value
			}
			cols["accountable_user_id"] = human
			wakeup := dispatcherTask(t, agent, cols)
			if tc.name == "cycle" {
				dbfx.Exec(t, "UPDATE agent_task_queue SET delegated_from_task_id=$2 WHERE id=$1", cycle, wakeup)
			}
			// A later delegation must not launder an untrusted wakeup into a human request.
			task := dispatcherTask(t, agent, testutil.Cols{"originator_source": "delegation", "originator_user_id": human, "accountable_user_id": human, "delegated_from_task_id": wakeup})
			id, before := reviewFixture(t, "Rejected wakeup: "+tc.name, testutil.Cols{
				"creator_type": "agent", "creator_id": agent, "origin_type": "agent_create", "origin_id": task,
				"status": "backlog", "assignee_type": "agent", "assignee_id": agent, "metadata": `{"keep":true}`,
			})
			assertDispatcher(t, id, "", false)
			rec := testutil.Call(t, testHandler.SubmitIssueReview, withURLParam(newRequest(http.MethodPost, "/api/issues/"+id+"/review", map[string]any{
				"status": "in_review", "expected_revision": before.Revision,
				"review_metadata": map[string]any{"review_by": "user", "review_user": testUserID, "review_action": "Check"},
			}), "id", id)).Want(400)
			if !strings.Contains(rec.Body.String(), "no trusted member source") {
				t.Fatal(rec.Body.String())
			}
			after, _ := reviewMetadataOf(t, id)
			if after.Status != before.Status || after.Revision != before.Revision || string(after.Metadata) != string(before.Metadata) {
				t.Fatal("rejected review mutated issue")
			}
		})
	}
}
