package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func reviewPut(t *testing.T, id string, body any, status int) IssueResponse {
	t.Helper()
	var result IssueResponse
	req := withURLParam(newRequest("PUT", "/api/issues/"+id, body), "id", id)
	response := testutil.Call(t, testHandler.UpdateIssue, req).Want(status)
	if status == http.StatusOK {
		response.JSON(&result)
	}
	return result
}

func TestReviewCreationAndStatusEntry(t *testing.T) {
	var created IssueResponse
	testutil.Call(t, testHandler.CreateIssue, newRequest("POST", "/api/issues", map[string]any{
		"workspace_id": testWorkspaceID, "title": "Created in review", "status": "in_review",
	})).Want(http.StatusCreated).JSON(&created)
	dbfx.Cleanup(t, "DELETE FROM issue WHERE id=$1", created.ID)
	if since, ok := created.Metadata["review_since"].(string); !ok {
		t.Fatal("create in_review must write review_since")
	} else if _, err := time.Parse(time.RFC3339Nano, since); err != nil {
		t.Fatal(err)
	}
	id := dbfx.Issue(t, "Status entry", testutil.Cols{"status": "in_progress"})
	entered, err := testHandler.Queries.UpdateIssueStatus(context.Background(), db.UpdateIssueStatusParams{
		ID: parseUUID(id), WorkspaceID: parseUUID(testWorkspaceID), Status: "in_review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if parseIssueMetadata(entered.Metadata)["review_since"] == nil {
		t.Fatal("status writer omitted entry time")
	}
}

func TestReviewLifecycleAndAtomicUndo(t *testing.T) {
	// R4/B7: publishing a user review requires a valid assignee to advance the
	// card after the person answers (the local port keeps the review round time
	// in metadata.review_since instead of the status_changed_at column).
	advancer := dbfx.Agent(t, "Review lifecycle advancer", "")
	id := dbfx.Issue(t, "Review lifecycle", testutil.Cols{"status": "in_progress", "assignee_type": "agent", "assignee_id": advancer, "metadata": `{"review_by":"user","review_action":"Check it","review_user":"` + testUserID + `","review_agent":"legacy","other":true,"review_custom":"keep"}`, "created_at": time.Now().Add(-time.Hour)})
	entered := reviewPut(t, id, map[string]any{"status": "in_review"}, 200)
	since, ok := entered.Metadata["review_since"].(string)
	if !ok {
		t.Fatal("entering review must write review_since")
	}
	if _, err := time.Parse(time.RFC3339Nano, since); err != nil {
		t.Fatal(err)
	}
	same := reviewPut(t, id, map[string]any{"status": "in_review"}, 200)
	edited := reviewPut(t, id, map[string]any{"title": "Review edited"}, 200)
	if same.Metadata["review_since"] != since || edited.Metadata["review_since"] != since {
		t.Fatal("non-transitions reset review time")
	}
	saved := map[string]any{"review_by": "user", "review_action": "Check it", "review_user": testUserID}
	approved := reviewPut(t, id, map[string]any{"status": "done", "expected_revision": edited.Revision}, 200)
	for _, k := range reviewKeys {
		if _, ok := approved.Metadata[k]; ok {
			t.Fatalf("leaked %s after approve", k)
		}
	}
	if approved.Metadata["other"] != true || approved.Metadata["review_custom"] != nil || approved.Metadata["review_since"] != nil {
		t.Fatal("unrelated metadata removed")
	}
	restored := reviewPut(t, id, map[string]any{"status": "in_review", "expected_revision": approved.Revision, "review_metadata": saved}, 200)
	if restored.Revision != approved.Revision+1 || restored.Metadata["review_by"] != "user" || restored.Metadata["other"] != true {
		t.Fatalf("non-atomic restore: %+v", restored)
	}
	if restored.Metadata["review_since"] == edited.Metadata["review_since"] {
		t.Fatal("undo reused old status entry time")
	}
	reviewPut(t, id, map[string]any{"status": "in_review", "expected_revision": approved.Revision, "review_metadata": map[string]any{"review_by": "specialist"}}, 409)
	var after IssueResponse
	testutil.Call(t, testHandler.GetIssue, withURLParam(newRequest("GET", "/api/issues/"+id, nil), "id", id)).Want(200).JSON(&after)
	if after.Revision != restored.Revision || after.Metadata["review_by"] != "user" {
		t.Fatal("conflict partially restored review")
	}
}

func TestReviewValidation(t *testing.T) {
	foreignUser := dbfx.User(t, "Other user", "review-other@example.test")
	foreignWS := dbfx.Workspace(t, "Other workspace", "review-other-workspace")
	dbfx.Member(t, foreignWS, foreignUser, "owner")
	foreignAgent := dbfx.Agent(t, "Other reviewer", "", testutil.Cols{"workspace_id": foreignWS})
	agent := dbfx.Agent(t, "Reviewer", "")
	id := dbfx.Issue(t, "Validation", testutil.Cols{"status": "in_review"})
	cases := []struct {
		key   string
		value any
		want  int
	}{
		{"review_action", strings.Repeat("文😀", 60), 200},
		{"review_action", strings.Repeat("文😀", 60) + "a", 400},
		{"review_action", " \t ", 400}, {"review_action", true, 400},
		{"review_action", nil, 400}, {"review_action", map[string]any{"text": "invalid"}, 400},
		{"review_by", "nobody", 400}, {"review_by", 1, 400},
		{"review_user", "bad", 400}, {"review_user", foreignUser, 400},
		{"review_agent", foreignAgent, 400}, {"review_agent", agent, 200},
		{"review_user", testUserID, 200}, {"review_action", "  Check 中文 😀  ", 200},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			before, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
			req := withURLParams(newRequest("PUT", "/api/issues/"+id+"/metadata/"+tc.key, map[string]any{"value": tc.value}), "id", id, "key", tc.key)
			response := testutil.Call(t, testHandler.SetIssueMetadataKey, req).Want(tc.want)
			if tc.want == 400 && !strings.Contains(response.Body.String(), tc.key) {
				t.Fatalf("field error must name %s: %s", tc.key, response.Body.String())
			}
			after, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
			if tc.want == 400 && (before.Revision != after.Revision || string(before.Metadata) != string(after.Metadata)) {
				t.Fatal("invalid write changed issue")
			}
			if parseIssueMetadata(before.Metadata)["review_since"] != parseIssueMetadata(after.Metadata)["review_since"] {
				t.Fatal("metadata reset review time")
			}
		})
	}
	for _, body := range []any{
		map[string]any{"review_metadata": map[string]any{}},
		map[string]any{"status": "done", "expected_revision": 1, "review_metadata": map[string]any{}},
	} {
		reviewPut(t, id, body, 400)
	}
	current, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
	for _, bad := range []any{nil, []any{}, map[string]any{"unrelated": "overwrite"}, map[string]any{"review_user": foreignUser}, map[string]any{"review_action": strings.Repeat("😀", 121)}} {
		reviewPut(t, id, map[string]any{"status": "in_review", "expected_revision": current.Revision, "review_metadata": bad}, 400)
	}
	reviewPut(t, id, map[string]any{"status": "done"}, 200)
	req := withURLParams(newRequest("PUT", "/api/issues/"+id+"/metadata/review_by", map[string]any{"value": "user"}), "id", id, "key", "review_by")
	testutil.Call(t, testHandler.SetIssueMetadataKey, req).Want(409)
}

func TestReviewStatusWriters(t *testing.T) {
	for _, status := range []string{"in_progress", "done", "blocked", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			id := dbfx.Issue(t, "Background status writer", testutil.Cols{"status": "in_review", "metadata": `{"review_by":"user","review_action":"Check","review_user":"old","review_agent":"old","review_other":"clear","keep":true}`})
			got, err := testHandler.Queries.UpdateIssueStatus(context.Background(), db.UpdateIssueStatusParams{ID: parseUUID(id), WorkspaceID: parseUUID(testWorkspaceID), Status: status})
			if err != nil {
				t.Fatal(err)
			}
			meta := parseIssueMetadata(got.Metadata)
			// Every review_* key (review_since included) is cleared on the way out;
			// the two review statuses share the rule, so entering blocked starts a
			// new round and must stamp a fresh review_since.
			for _, key := range reviewKeys {
				if _, ok := meta[key]; ok {
					t.Fatalf("status writer leaked %s: %s", key, got.Metadata)
				}
			}
			if _, ok := meta["review_other"]; ok {
				t.Fatalf("status writer kept a stale review_* key: %s", got.Metadata)
			}
			if meta["keep"] != true {
				t.Fatalf("status writer deleted unrelated metadata: %s", got.Metadata)
			}
			if status == "blocked" {
				if _, ok := meta["review_since"]; !ok {
					t.Fatalf("entering blocked did not stamp review_since: %s", got.Metadata)
				}
			} else if _, ok := meta["review_since"]; ok {
				t.Fatalf("leaving review kept review_since: %s", got.Metadata)
			}
		})
	}
}

func TestReviewBatchTransitionClearsKeys(t *testing.T) {
	id := dbfx.Issue(t, "Batch review", testutil.Cols{"status": "in_review", "metadata": `{"review_by":"user","review_action":"Check","keep":true}`})
	req := newRequest("PUT", "/api/issues/batch?workspace_id="+testWorkspaceID, map[string]any{"issue_ids": []string{id}, "updates": map[string]any{"status": "done"}})
	testutil.Call(t, testHandler.BatchUpdateIssues, req).Want(200)
	current, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
	if err != nil {
		t.Fatal(err)
	}
	metadata := parseIssueMetadata(current.Metadata)
	if current.Status != "done" || len(metadata) != 1 || metadata["keep"] != true {
		t.Fatalf("batch review snapshot: %s %s", current.Status, current.Metadata)
	}
	req = newRequest("PUT", "/api/issues/batch?workspace_id="+testWorkspaceID, map[string]any{"issue_ids": []string{id}, "updates": map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "user"}}})
	testutil.Call(t, testHandler.BatchUpdateIssues, req).Want(400)
}

func TestReviewRestorePreservesLargeUnrelatedNumbers(t *testing.T) {
	advancer := dbfx.Agent(t, "Exact metadata advancer", "")
	id := dbfx.Issue(t, "Exact metadata", testutil.Cols{"status": "done", "assignee_type": "agent", "assignee_id": advancer, "metadata": `{"large":9007199254740993}`})
	current, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
	if err != nil {
		t.Fatal(err)
	}
	reviewPut(t, id, map[string]any{"status": "in_review", "expected_revision": current.Revision, "review_metadata": map[string]any{"review_by": "user"}}, 200)
	var value string
	if err := testPool.QueryRow(context.Background(), `SELECT metadata->>'large' FROM issue WHERE id=$1`, id).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "9007199254740993" {
		t.Fatalf("unrelated number changed to %s", value)
	}
}

func TestReviewPRCompletionClearsKeys(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "review-completion-test")
	const installationID int64 = 885151
	issue := prAutoCompleteTestIssue(t, "Review PR completion", installationID)
	setWorkspacePRMergeStatusForTest(t, "done")
	dbfx.Exec(t, `UPDATE issue SET status='in_review',metadata='{"review_by":"user","review_action":"Check","keep":true}'::jsonb WHERE id=$1`, issue.ID)
	firePRWebhook(t, "review-completion-test", installationID, 1, issue.Identifier+": review", "", "fix/review", "merged")
	current, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(issue.ID))
	if err != nil {
		t.Fatal(err)
	}
	metadata := parseIssueMetadata(current.Metadata)
	if current.Status != "done" || len(metadata) != 1 || metadata["keep"] != true {
		t.Fatalf("PR completion snapshot: %s %s", current.Status, current.Metadata)
	}
}

func TestReviewLateMetadataWaitsForStatusLock(t *testing.T) {
	id := dbfx.Issue(t, "Concurrent review", testutil.Cols{"status": "in_review", "metadata": `{"review_by":"user"}`})
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = db.New(tx).UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: parseUUID(id), WorkspaceID: parseUUID(testWorkspaceID), Status: "done"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		req := withURLParams(newRequest("PUT", "/api/issues/"+id+"/metadata/review_by", map[string]any{"value": "user"}), "id", id, "key", "review_by")
		// Do not use testing assertions from the worker goroutine.
		rr := httptestRecorder(testHandler.SetIssueMetadataKey, req)
		done <- rr
	}()
	select {
	case <-done:
		t.Fatal("review write bypassed row lock")
	case <-time.After(50 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 409 {
			t.Fatalf("late write status=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late write did not finish")
	}
	got, _ := testHandler.Queries.GetIssue(ctx, parseUUID(id))
	if len(parseIssueMetadata(got.Metadata)) != 0 {
		t.Fatal("late write resurrected review metadata")
	}
}

// Keep asynchronous request execution separate from assertions on the test thread.
func httptestRecorder(h http.HandlerFunc, req *http.Request) int {
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr.Code
}
