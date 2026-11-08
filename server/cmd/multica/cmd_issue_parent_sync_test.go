package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func activityTestCmd(reason string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("reason", reason, "")
	cmd.Flags().String("output", "json", "")
	return cmd
}

func TestIssueActivityRequestAndFailure(t *testing.T) {
	for _, status := range []int{200, 400, 403, 404, 409, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/api/issues/"+testIssueUUID+"/evaluated" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				writes++
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["outcome"] != "no_action" || body["reason"] != "已读，无需调整 😀" {
					t.Errorf("bad payload: %v", body)
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"outcome":"no_action"}`))
			}))
			defer srv.Close()
			setCLITestServerEnv(t, srv.URL)
			t.Setenv("MULTICA_TOKEN", "mat_test")
			err := runIssueActivity(activityTestCmd(" 已读，无需调整 😀 "), []string{testIssueUUID, "no_action"})
			if (err == nil) != (status == 200) {
				t.Fatalf("status %d: %v", status, err)
			}
			if writes != 1 {
				t.Fatalf("must submit once, got %d", writes)
			}
		})
	}
}

func TestIssueActivityRejectsInvalidInput(t *testing.T) {
	for _, reason := range []string{"", " \t", strings.Repeat("中", 121)} {
		if err := runIssueActivity(activityTestCmd(reason), []string{testIssueUUID, "no_action"}); err == nil {
			t.Fatal("invalid reason accepted")
		}
	}
	if err := runIssueActivity(activityTestCmd("valid"), []string{testIssueUUID, "action"}); err == nil {
		t.Fatal("unsupported outcome accepted")
	}
	if issueCommentAddCmd.Flags().Lookup("sync-parent-assignee") == nil || issueActivityCmd.Parent() != issueCmd {
		t.Fatal("CLI command not registered")
	}
}

func TestIssueCommentParentSyncExplicitOptIn(t *testing.T) {
	for _, sync := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "sync"}[sync], func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writes++
				if r.Method != "POST" || r.URL.Path != "/api/issues/"+testIssueUUID+"/comments" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				_, present := body["sync_parent_assignee"]
				if present != sync || (sync && body["sync_parent_assignee"] != true) {
					t.Errorf("bad sync flag: %v", body)
				}
				if body["content"] != "真实交付\n请读取主卡" || body["parent_id"] != "trigger" {
					t.Errorf("lost content/routing: %v", body)
				}
				_, _ = w.Write([]byte(`{"id":"comment"}`))
			}))
			defer srv.Close()
			setCLITestServerEnv(t, srv.URL)
			t.Setenv("MULTICA_TOKEN", "mat_test")
			t.Chdir(t.TempDir())
			if err := os.WriteFile("reply.md", []byte("真实交付\n请读取主卡"), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := newIssueCommentAddTestCmd()
			cmd.Flags().Bool("sync-parent-assignee", sync, "")
			_ = cmd.Flags().Set("content-file", "reply.md")
			_ = cmd.Flags().Set("parent", "trigger")
			if err := runIssueCommentAdd(cmd, []string{testIssueUUID}); err != nil {
				t.Fatal(err)
			}
			if writes != 1 {
				t.Fatalf("writes=%d", writes)
			}
		})
	}
}
