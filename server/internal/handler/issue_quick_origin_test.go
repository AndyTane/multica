package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestCreateIssueQuickCreateOriginBindsCurrentTask covers the B9 origin-binding
// rule: an agent may stamp a new issue with quick_create provenance only for the
// task it is actually running, never for another task of the same agent.
func TestCreateIssueQuickCreateOriginBindsCurrentTask(t *testing.T) {
	agentID := createHandlerTestAgent(t, "Quick origin binding agent", nil)
	makeTask := func() string {
		return dbfx.Task(t, agentID, testutil.Cols{
			"issue_id":            nil,
			"runtime_id":          testRuntimeID,
			"originator_user_id":  testUserID,
			"accountable_user_id": testUserID,
			"context":             `{"type":"quick_create","prompt":"x","requester_id":"` + testUserID + `","workspace_id":"` + testWorkspaceID + `"}`,
		})
	}
	currentTask := makeTask()
	otherTask := makeTask()

	create := func(t *testing.T, originID string, want int) {
		t.Helper()
		req := newRequest(http.MethodPost, "/api/issues", map[string]any{
			"title":       "Quick origin card " + originID,
			"origin_type": "quick_create",
			"origin_id":   originID,
		})
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", currentTask)
		testutil.Call(t, testHandler.CreateIssue, req).Want(want)
	}

	// The current task is a valid origin.
	var created IssueResponse
	req := newRequest(http.MethodPost, "/api/issues", map[string]any{
		"title":       "Quick origin bound card",
		"origin_type": "quick_create",
		"origin_id":   currentTask,
	})
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", currentTask)
	testutil.Call(t, testHandler.CreateIssue, req).Want(201).JSON(&created)

	// Another task of the same agent is not.
	create(t, otherTask, 400)

	var storedOriginID string
	dbfx.QueryRow(t, `SELECT origin_id::text FROM issue WHERE id = $1`, created.ID).Scan(&storedOriginID)
	if storedOriginID != currentTask {
		t.Fatalf("stored origin_id = %q, want the current task", storedOriginID)
	}
}
