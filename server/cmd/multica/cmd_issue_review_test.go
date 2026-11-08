package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIssueReviewAtomicRequest(t *testing.T) {
	for _, kind := range []string{"approve", "reply", "act"} {
		t.Run(kind, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					_, _ = w.Write([]byte(`{"revision":7}`))
					return
				}
				writes++
				if r.Method != "POST" || r.URL.Path != "/api/issues/"+testIssueUUID+"/review" {
					t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
				}
				var body struct {
					Status   string            `json:"status"`
					Revision int64             `json:"expected_revision"`
					Mark     map[string]string `json:"review_metadata"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				status := "blocked"
				if kind == "approve" {
					status = "in_review"
				}
				if body.Status != status || body.Revision != 7 || body.Mark["review_kind"] != kind || body.Mark["review_action"] != "请确认中文 😀" || body.Mark["review_by"] != "user" {
					t.Errorf("bad atomic payload: %+v", body)
				}
				_, _ = w.Write([]byte(`{"identifier":"MUL-1"}`))
			}))
			defer srv.Close()
			t.Setenv("MULTICA_SERVER_URL", srv.URL)
			t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
			t.Setenv("MULTICA_TOKEN", "mat_test")
			cmd := newIssueReviewCmd()
			_ = cmd.Flags().Set("kind", kind)
			_ = cmd.Flags().Set("action", " 请确认中文 😀 ")
			if err := runIssueReview(cmd, []string{testIssueUUID}); err != nil {
				t.Fatal(err)
			}
			if writes != 1 {
				t.Fatalf("writes=%d", writes)
			}
		})
	}
}

func TestIssueReviewFailureNeverFallsBack(t *testing.T) {
	for _, code := range []int{400, 403, 404, 409, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					_, _ = w.Write([]byte(`{"revision":3}`))
					return
				}
				writes++
				http.Error(w, `{"error":"rejected"}`, code)
			}))
			defer srv.Close()
			t.Setenv("MULTICA_SERVER_URL", srv.URL)
			t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
			t.Setenv("MULTICA_TOKEN", "mat_test")
			cmd := newIssueReviewCmd()
			_ = cmd.Flags().Set("action", "Check")
			if err := runIssueReview(cmd, []string{testIssueUUID}); err == nil {
				t.Fatal("expected failure")
			}
			if writes != 1 {
				t.Fatalf("writes=%d; must not retry or fall back", writes)
			}
		})
	}
}

func TestIssueReviewRejectsIncompleteInput(t *testing.T) {
	for _, flags := range []map[string]string{{}, {"action": "  "}, {"action": "Check", "by": "other"}, {"action": "Check", "kind": "other"}, {"action": "Check", "by": "specialist", "kind": "act"}} {
		cmd := newIssueReviewCmd()
		for k, v := range flags {
			_ = cmd.Flags().Set(k, v)
		}
		if err := runIssueReview(cmd, []string{testIssueUUID}); err == nil {
			t.Fatalf("accepted %+v", flags)
		}
	}
}
