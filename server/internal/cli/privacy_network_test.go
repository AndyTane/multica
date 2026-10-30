package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/multica-ai/multica/server/internal/privacy"
)

// Task-scoped CLI tools and user-selected downloads must remain usable with
// reporting disabled. The fake server stands in for the configured workspace.
func TestReportingPolicyPreservesTaskToolsAndDownloads(t *testing.T) {
	if privacy.ExternalReportingEnabled {
		t.Fatal("expected production reporting policy")
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/api/issues/test" {
			if r.Header.Get("Authorization") != "Bearer mat_fake" || r.Header.Get("X-Workspace-ID") != "workspace-test" {
				t.Error("task tool lost its authorization context")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"title":"fake task"}`)
			return
		}
		if r.URL.Path != "/user-selected-download" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("workspace credential sent to an absolute download URL")
		}
		_, _ = io.WriteString(w, "fake download")
	}))
	defer server.Close()
	client := NewAPIClient(server.URL, "workspace-test", "mat_fake")
	client.AgentID, client.TaskID = "agent-test", "task-test"
	var issue struct{ Title string }
	if err := client.GetJSON(context.Background(), "/api/issues/test", &issue); err != nil {
		t.Fatal(err)
	}
	data, err := client.DownloadFile(context.Background(), server.URL+"/user-selected-download")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Title != "fake task" || string(data) != "fake download" || requests.Load() != 2 {
		t.Fatalf("task tool/download failed: title=%q, requests=%d", issue.Title, requests.Load())
	}
}
