package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestSubmitIssueReviewAtomicity(t *testing.T) {
	advancer := dbfx.Agent(t, "Review command advancer", "")
	for _, kind := range []string{"approve", "reply", "act"} {
		t.Run(kind, func(t *testing.T) {
			id, before := reviewFixture(t, "Submit review", testutil.Cols{"status": "in_progress", "assignee_type": "agent", "assignee_id": advancer})
			status := "blocked"
			if kind == "approve" {
				status = "in_review"
			}
			body := map[string]any{"status": status, "expected_revision": before.Revision, "review_metadata": map[string]any{"review_by": "user", "review_kind": kind, "review_action": "请确认结果"}}
			req := withURLParams(newRequest("POST", "/api/issues/"+id+"/review", body), "id", id)
			var out IssueResponse
			testutil.Call(t, testHandler.SubmitIssueReview, req).Want(http.StatusOK).JSON(&out)
			if out.Status != status || out.Revision != before.Revision+1 || out.Metadata["review_action"] != "请确认结果" || out.Metadata["review_user"] != testUserID {
				t.Fatalf("incomplete submit: %+v", out)
			}
			// The same stale snapshot cannot overwrite the published round.
			testutil.Call(t, testHandler.SubmitIssueReview, withURLParams(newRequest("POST", "/api/issues/"+id+"/review", body), "id", id)).Want(409)
			after, _ := reviewMetadataOf(t, id)
			if after.Revision != out.Revision {
				t.Fatal("stale submit changed revision")
			}
		})
	}
	for _, mark := range []map[string]any{
		{"review_by": "user"},
		{"review_by": "user", "review_action": " "},
		{"review_by": "user", "review_action": "Check", "review_kind": "reply"},
		{"review_by": "user", "review_action": "Check", "review_user": "00000000-0000-0000-0000-000000000001"},
	} {
		id, before := reviewFixture(t, "Rejected submission", testutil.Cols{"status": "in_progress", "assignee_type": "agent", "assignee_id": advancer, "metadata": `{"keep":true}`})
		body := map[string]any{"status": "in_review", "expected_revision": before.Revision, "review_metadata": mark}
		testutil.Call(t, testHandler.SubmitIssueReview, withURLParams(newRequest("POST", "/api/issues/"+id+"/review", body), "id", id)).Want(400)
		after, _ := reviewMetadataOf(t, id)
		if after.Status != before.Status || after.Revision != before.Revision || string(after.Metadata) != string(before.Metadata) {
			t.Fatal("rejected review partially changed issue")
		}
	}
}
