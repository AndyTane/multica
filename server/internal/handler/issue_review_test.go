package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestReviewLifecycleAndAtomicUndo(t *testing.T) {
	id := dbfx.Issue(t, "Review lifecycle", testutil.Cols{"status": "in_review", "metadata": `{"review_by":"user","review_action":"Check it","review_user":"` + testUserID + `","review_agent":"legacy","other":true,"review_custom":"keep"}`, "created_at": time.Now().Add(-time.Hour)})
	current, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
	if err != nil {
		t.Fatal(err)
	}
	if !current.StatusChangedAt.Time.Equal(current.CreatedAt.Time) {
		t.Fatal("create must use created_at")
	}
	same := reviewPut(t, id, map[string]any{"status": "in_review"}, 200)
	edited := reviewPut(t, id, map[string]any{"title": "Review edited"}, 200)
	if same.StatusChangedAt != edited.StatusChangedAt || same.StatusChangedAt != timestampToString(current.StatusChangedAt) {
		t.Fatal("non-transitions reset review time")
	}
	saved := map[string]any{"review_by": "user", "review_action": "Check it", "review_user": testUserID}
	approved := reviewPut(t, id, map[string]any{"status": "done", "expected_revision": edited.Revision}, 200)
	for _, k := range reviewKeys {
		if _, ok := approved.Metadata[k]; ok {
			t.Fatalf("leaked %s after approve", k)
		}
	}
	if approved.Metadata["other"] != true || approved.Metadata["review_custom"] != "keep" {
		t.Fatal("unrelated metadata removed")
	}
	restored := reviewPut(t, id, map[string]any{"status": "in_review", "expected_revision": approved.Revision, "review_metadata": saved}, 200)
	if restored.Revision != approved.Revision+1 || restored.Metadata["review_by"] != "user" || restored.Metadata["other"] != true {
		t.Fatalf("non-atomic restore: %+v", restored)
	}
	if restored.StatusChangedAt == edited.StatusChangedAt {
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
		{"review_by", "nobody", 400}, {"review_by", 1, 400},
		{"review_user", "bad", 400}, {"review_user", foreignUser, 400},
		{"review_agent", foreignAgent, 400}, {"review_agent", agent, 200},
		{"review_user", testUserID, 200}, {"review_action", "  Check 中文 😀  ", 200},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			before, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
			req := withURLParams(newRequest("PUT", "/api/issues/"+id+"/metadata/"+tc.key, map[string]any{"value": tc.value}), "id", id, "key", tc.key)
			testutil.Call(t, testHandler.SetIssueMetadataKey, req).Want(tc.want)
			after, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
			if tc.want == 400 && (before.Revision != after.Revision || string(before.Metadata) != string(after.Metadata)) {
				t.Fatal("invalid write changed issue")
			}
			if !before.StatusChangedAt.Time.Equal(after.StatusChangedAt.Time) {
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
			id := dbfx.Issue(t, "Background status writer", testutil.Cols{"status": "in_review", "metadata": `{"review_by":"user","review_action":"Check","review_user":"old","review_agent":"old","review_other":"keep"}`})
			got, err := testHandler.Queries.UpdateIssueStatus(context.Background(), db.UpdateIssueStatusParams{ID: parseUUID(id), WorkspaceID: parseUUID(testWorkspaceID), Status: status})
			if err != nil {
				t.Fatal(err)
			}
			meta := parseIssueMetadata(got.Metadata)
			if len(meta) != 1 || meta["review_other"] != "keep" {
				t.Fatalf("status writer leaked/deleted keys: %s", got.Metadata)
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
	id := dbfx.Issue(t, "Exact metadata", testutil.Cols{"status": "done", "metadata": `{"large":9007199254740993}`})
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

func TestReviewLateMetadataWaitsForStatusLock(t *testing.T) {
	id := dbfx.Issue(t, "Concurrent review", testutil.Cols{"status": "in_review", "metadata": `{"review_by":"user"}`})
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "UPDATE issue SET status='done',revision=revision+1 WHERE id=$1", id); err != nil {
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

func TestReviewMigrationBackfill(t *testing.T) {
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	schema := "review_migration_test"
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`,public;
 CREATE TABLE issue(id uuid PRIMARY KEY, workspace_id uuid, status text, metadata jsonb DEFAULT '{}',created_at timestamptz,updated_at timestamptz,revision bigint);
 CREATE TABLE activity_log(id uuid,workspace_id uuid,issue_id uuid,action text,details jsonb,created_at timestamptz);`); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 5; n++ {
		id := fmt.Sprintf("00000000-0000-0000-0000-%012d", n)
		if _, err = tx.Exec(ctx, `INSERT INTO issue VALUES($1,$2,'in_review','{}','2026-01-01','2026-03-01',7)`, id, testWorkspaceID); err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			if _, err = tx.Exec(ctx, `INSERT INTO activity_log VALUES(gen_random_uuid(),$2,$1,'status_changed','{"from":"in_progress","to":"in_review"}','2026-02-01')`, id, testWorkspaceID); err != nil {
				t.Fatal(err)
			}
		}
		if n == 3 {
			_, err = tx.Exec(ctx, `INSERT INTO activity_log VALUES(gen_random_uuid(),$2,$1,'status_changed','{"from":"in_review","to":"done"}','2026-02-02')`, id, testWorkspaceID)
		}
		if n == 4 {
			_, err = tx.Exec(ctx, `INSERT INTO activity_log VALUES(gen_random_uuid(),$2,$1,'status_changed','{"from":"in_review","to":"in_review"}','2026-02-02')`, id, testWorkspaceID)
		}
		if n == 5 {
			_, err = tx.Exec(ctx, `INSERT INTO activity_log VALUES(gen_random_uuid(),$2,$1,'status_changed','{"to":"in_review"}','2026-02-02')`, id, testWorkspaceID)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	migration, err := os.ReadFile("../../migrations/584_issue_review.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, `SELECT id::text,status_changed_at::date::text,updated_at::date::text,revision FROM issue ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
		var id, date, updated string
		var revision int64
		if err = rows.Scan(&id, &date, &updated, &revision); err != nil {
			t.Fatal(err)
		}
		want := "2026-03-01"
		if n == 1 {
			want = "2026-02-01"
		}
		if date != want || updated != "2026-03-01" || revision != 7 {
			t.Fatalf("backfill %s: %s/%s/%d", id, date, updated, revision)
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	down, err := os.ReadFile("../../migrations/584_issue_review.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	var hasColumn bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=$1 AND table_name='issue' AND column_name='status_changed_at')`, schema).Scan(&hasColumn); err != nil {
		t.Fatal(err)
	}
	if hasColumn {
		t.Fatal("down migration kept review column")
	}
}

// Keep asynchronous request execution separate from assertions on the test thread.
func httptestRecorder(h http.HandlerFunc, req *http.Request) int {
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr.Code
}
