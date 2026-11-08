package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// taskRuntimes reads each task's runtime_id, keyed by task id.
func taskRuntimes(t *testing.T, taskIDs ...string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(taskIDs))
	for _, id := range taskIDs {
		var runtimeID string
		if err := testPool.QueryRow(context.Background(),
			`SELECT runtime_id::text FROM agent_task_queue WHERE id = $1`, id).Scan(&runtimeID); err != nil {
			t.Fatalf("read task %s runtime: %v", id, err)
		}
		out[id] = runtimeID
	}
	return out
}

func patchAgentRuntime(t *testing.T, agentID, runtimeID string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := withURLParam(newRequest(http.MethodPatch, "/api/agents/"+agentID, map[string]any{
		"runtime_id": runtimeID,
	}), "id", agentID)
	testHandler.UpdateAgent(w, req)
	return w
}

func TestUpdateAgent_RuntimeSwitch_MovesPendingTasks(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	const oldDaemon = "runtime-switch-daemon-old"
	const newDaemon = "runtime-switch-daemon-new"
	oldRuntimeID := dbfx.Runtime(t, "runtime-switch-tasks-old", testutil.Cols{"daemon_id": oldDaemon})
	newRuntimeID := dbfx.Runtime(t, "runtime-switch-tasks-new", testutil.Cols{"daemon_id": newDaemon})
	agentID := dbfx.Agent(t, "runtime-switch-tasks-agent", oldRuntimeID)
	issueID := dbfx.Issue(t, "runtime switch pending tasks")
	queuedID := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": oldRuntimeID, "issue_id": issueID})
	deferredID := dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": oldRuntimeID, "issue_id": issueID, "status": "deferred",
	})
	runningID := dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": oldRuntimeID, "issue_id": issueID, "status": "running",
		"dispatched_at": testutil.Raw("now()"), "started_at": testutil.Raw("now()"),
	})
	finishedID := dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": oldRuntimeID, "issue_id": issueID, "status": "completed",
		"completed_at": testutil.Raw("now()"),
	})

	w := patchAgentRuntime(t, agentID, newRuntimeID)
	if w.Code != http.StatusOK {
		t.Fatalf("switch runtime: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := taskRuntimes(t, queuedID, deferredID, runningID, finishedID)
	want := map[string]string{
		queuedID:   newRuntimeID,
		deferredID: newRuntimeID,
		runningID:  oldRuntimeID,
		finishedID: oldRuntimeID,
	}
	for id, runtimeID := range want {
		if got[id] != runtimeID {
			t.Errorf("task %s runtime = %s, want %s", id, got[id], runtimeID)
		}
	}

	// Authorization to finish an in-flight run follows its original daemon.
	path := "/api/daemon/tasks/" + runningID + "/complete"
	complete := func(daemonID string, wantStatus int) {
		t.Helper()
		req := newDaemonTokenRequest(http.MethodPost, path, map[string]any{"output": "done"}, testWorkspaceID, daemonID)
		testutil.Call(t, testHandler.CompleteTask, withURLParam(req, "taskId", runningID)).Want(wantStatus)
	}
	complete(newDaemon, http.StatusNotFound)
	complete(oldDaemon, http.StatusOK)
}

func TestUpdateAgent_RuntimeSwitch_ForbiddenLeavesTasks(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	suffix := time.Now().UnixNano()
	strangerID := dbfx.User(t,
		fmt.Sprintf("runtime-switch-stranger-%d", suffix),
		fmt.Sprintf("runtime-switch-stranger-%d@example.com", suffix),
	)
	dbfx.Member(t, testWorkspaceID, strangerID, "member")
	oldRuntimeID := dbfx.Runtime(t, "runtime-switch-forbidden-old")
	strangerRuntimeID := dbfx.Runtime(t, "runtime-switch-forbidden-private", testutil.Cols{
		"visibility": "private", "owner_id": strangerID,
	})
	agentID := dbfx.Agent(t, "runtime-switch-forbidden-agent", oldRuntimeID)
	queuedID := dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": oldRuntimeID, "issue_id": dbfx.Issue(t, "runtime switch forbidden"),
	})

	w := patchAgentRuntime(t, agentID, strangerRuntimeID)
	if w.Code != http.StatusForbidden {
		t.Fatalf("switch to another member's private runtime: expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if got := taskRuntimes(t, queuedID)[queuedID]; got != oldRuntimeID {
		t.Fatalf("queued task runtime = %s, want unchanged %s", got, oldRuntimeID)
	}
}

func TestUpdateAgent_RuntimeSwitch_RollsBackOnNameConflict(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	oldRuntime := dbfx.Runtime(t, "runtime-switch-rollback-old")
	newRuntime := dbfx.Runtime(t, "runtime-switch-rollback-new")
	agentID := dbfx.Agent(t, "runtime-switch-rollback-agent", oldRuntime)
	const takenName = "runtime-switch-rollback-taken"
	dbfx.Agent(t, takenName, oldRuntime)
	queuedID := dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": oldRuntime, "issue_id": dbfx.Issue(t, "runtime switch rollback"),
	})

	req := withURLParam(newRequest(http.MethodPatch, "/api/agents/"+agentID, map[string]any{
		"name": takenName, "runtime_id": newRuntime,
	}), "id", agentID)
	testutil.Call(t, testHandler.UpdateAgent, req).Want(http.StatusConflict)

	if got := taskRuntimes(t, queuedID)[queuedID]; got != oldRuntime {
		t.Errorf("queued task runtime = %s, want %s", got, oldRuntime)
	}
	var boundRuntime string
	if err := testPool.QueryRow(context.Background(), "SELECT runtime_id::text FROM agent WHERE id = $1", agentID).Scan(&boundRuntime); err != nil {
		t.Fatal(err)
	}
	if boundRuntime != oldRuntime {
		t.Errorf("agent runtime = %s, want %s", boundRuntime, oldRuntime)
	}
}
