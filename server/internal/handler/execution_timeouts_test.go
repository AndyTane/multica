package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/agent"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const timeoutPolicyJSON = `{"idle_seconds":7200,"tool_idle_seconds":8000,"run_seconds":0,"opencode_idle_seconds":0,"opencode_shell_seconds":5400}`

func saveTimeoutWorkspace(t *testing.T) {
	t.Helper()
	if testPool == nil {
		t.Fatal("isolated PostgreSQL required")
	}
	var previous []byte
	dbfx.QueryRow(t, `SELECT settings FROM workspace WHERE id=$1`, testWorkspaceID).Scan(&previous)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE workspace SET settings=$1 WHERE id=$2`, previous, testWorkspaceID)
	})
}

func TestExecutionTimeoutSettingsPersistence(t *testing.T) {
	saveTimeoutWorkspace(t)
	runtimeID := createClaimReclaimRuntime(t, context.Background(), "timeout settings")
	agentID, _ := createClaimReclaimAgentAndIssue(t, context.Background(), runtimeID, "timeout settings")
	dbfx.Exec(t, `UPDATE workspace SET settings='{"co_authored_by_enabled":false,"unrelated":{"enabled":true}}'::jsonb WHERE id=$1`, testWorkspaceID)
	dbfx.Exec(t, `UPDATE agent SET runtime_config='{"gateway":{"url":"http://example.test"},"context_compaction":{"enabled":false}}'::jsonb WHERE id=$1`, agentID)
	updateWS := func(patch string, status int) {
		t.Helper()
		req := withURLParam(newRequest("PATCH", "/", map[string]any{"settings": json.RawMessage(patch)}), "id", testWorkspaceID)
		result := testutil.Call(t, testHandler.UpdateWorkspace, req).Want(status)
		if status == http.StatusOK {
			var out WorkspaceResponse
			result.JSON(&out)
			if !out.ExecutionTimeoutsSupported {
				t.Fatal("workspace omitted server capability")
			}
		}
	}
	updateAgent := func(patch string, status int) {
		t.Helper()
		req := withURLParam(newRequest("PUT", "/", map[string]any{"runtime_config": json.RawMessage(patch)}), "id", agentID)
		result := testutil.Call(t, testHandler.UpdateAgent, req).Want(status)
		if status == http.StatusOK {
			var out AgentResponse
			result.JSON(&out)
			if !out.ExecutionTimeoutsSupported {
				t.Fatal("agent omitted server capability")
			}
		}
	}
	updateWS(`{"execution_timeouts":`+timeoutPolicyJSON+`}`, http.StatusOK)
	updateAgent(`{"execution_timeouts":`+strings.Replace(timeoutPolicyJSON, "7200", "0", 1)+`}`, http.StatusOK)
	read := func() (map[string]any, map[string]any) {
		t.Helper()
		var ws, rc []byte
		dbfx.QueryRow(t, `SELECT settings FROM workspace WHERE id=$1`, testWorkspaceID).Scan(&ws)
		dbfx.QueryRow(t, `SELECT runtime_config FROM agent WHERE id=$1`, agentID).Scan(&rc)
		var a, b map[string]any
		json.Unmarshal(ws, &a)
		json.Unmarshal(rc, &b)
		return a, b
	}
	ws, rc := read()
	if ws["unrelated"] == nil || ws["execution_timeouts"] == nil || rc["gateway"] == nil || rc["context_compaction"] == nil || rc["execution_timeouts"] == nil {
		t.Fatalf("partial update lost settings: %+v %+v", ws, rc)
	}
	// Existing editors replace ordinary keys, but an omitted reserved key
	// must not erase the policy saved by the new timeout editor.
	updateWS(`{"github_pr_merge_enabled":true}`, http.StatusOK)
	updateAgent(`{"model_hint":"preserve"}`, http.StatusOK)
	ws, rc = read()
	if ws["execution_timeouts"] == nil || rc["execution_timeouts"] == nil || ws["unrelated"] != nil || rc["gateway"] != nil || rc["context_compaction"] != nil {
		t.Fatalf("replacement contract violated: %+v %+v", ws, rc)
	}
	updateAgent(`{"execution_timeouts":null}`, http.StatusOK)
	_, rc = read()
	if _, exists := rc["execution_timeouts"]; exists {
		t.Fatal("inheritance did not delete reserved key")
	}
	for _, bad := range []string{`[]`, `"bad"`, `{"execution_timeouts":{}}`, `{"execution_timeouts":` + strings.Replace(timeoutPolicyJSON, "7200", "1.0", 1) + `}`, `{"execution_timeouts":` + strings.Replace(timeoutPolicyJSON, "7200", "-1", 1) + `}`} {
		updateWS(bad, http.StatusBadRequest)
		updateAgent(bad, http.StatusBadRequest)
	}
	// Task credentials cannot edit or clear policies even for their originating owner.
	for _, clear := range []string{`{"execution_timeouts":null}`, `{"execution_timeouts":` + timeoutPolicyJSON + `}`} {
		req := withURLParam(newRequest("PATCH", "/", map[string]any{"settings": json.RawMessage(clear)}), "id", testWorkspaceID)
		req.Header.Set("X-Agent-ID", agentID)
		testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusForbidden)
		req = withURLParam(newRequest("PUT", "/", map[string]any{"runtime_config": json.RawMessage(clear)}), "id", agentID)
		req.Header.Set("X-Agent-ID", agentID)
		testutil.Call(t, testHandler.UpdateAgent, req).Want(http.StatusForbidden)
	}
	// Cross-workspace reads and writes keep the existing visibility gate.
	otherWorkspace := dbfx.Insert(t, "workspace", testutil.Cols{"name": "Timeout other", "slug": "timeout-other"})
	req := withURLParam(newRequest("PUT", "/", map[string]any{"runtime_config": map[string]any{"execution_timeouts": nil}}), "id", agentID)
	req.Header.Set("X-Workspace-ID", otherWorkspace)
	testutil.Call(t, testHandler.UpdateAgent, req).Want(http.StatusNotFound)
	// The workspace endpoint is protected by the same admin middleware as the router.
	memberID := dbfx.Insert(t, "user", testutil.Cols{"name": "Timeout member", "email": "timeout-member@example.test"})
	dbfx.Insert(t, "member", testutil.Cols{"workspace_id": testWorkspaceID, "user_id": memberID, "role": "member"})
	req = withURLParam(newRequest("PATCH", "/", map[string]any{"settings": map[string]any{"execution_timeouts": nil}}), "id", testWorkspaceID)
	req.Header.Set("X-User-ID", memberID)
	guard := middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner", "admin")(http.HandlerFunc(testHandler.UpdateWorkspace))
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member workspace update: %d %s", w.Code, w.Body)
	}
	req = withURLParam(newRequest("PUT", "/", map[string]any{"runtime_config": map[string]any{"execution_timeouts": nil}}), "id", agentID)
	req.Header.Set("X-User-ID", memberID)
	testutil.Call(t, testHandler.UpdateAgent, req).Want(http.StatusForbidden)
}

func TestExecutionTimeoutClaimPolicy(t *testing.T) {
	saveTimeoutWorkspace(t)
	for _, batch := range []bool{false, true} {
		for _, tc := range []struct {
			name, workspace, override, caps, source string
			idle                                    int64
			rejected                                bool
		}{
			{"legacy", "{}", "{}", "", "client", 0, false},
			{"workspace", `{"execution_timeouts":` + timeoutPolicyJSON + `}`, "{}", protocol.DaemonCapabilityExecutionTimeoutsV1, "workspace", 7200, false},
			{"agent zero", `{"execution_timeouts":` + timeoutPolicyJSON + `}`, `{"execution_timeouts":` + strings.Replace(timeoutPolicyJSON, "7200", "0", 1) + `}`, protocol.DaemonCapabilityExecutionTimeoutsV1, "agent", 0, false},
			{"old client", `{"execution_timeouts":` + timeoutPolicyJSON + `}`, "{}", "", "", 0, true},
		} {
			t.Run(tc.name+map[bool]string{false: " single", true: " batch"}[batch], func(t *testing.T) {
				ctx := context.Background()
				rt := createClaimReclaimRuntime(t, ctx, "timeout claim")
				a, i := createClaimReclaimAgentAndIssue(t, ctx, rt, "timeout claim")
				dbfx.Exec(t, `UPDATE workspace SET settings=$1 WHERE id=$2`, []byte(tc.workspace), testWorkspaceID)
				dbfx.Exec(t, `UPDATE agent SET runtime_config=$1 WHERE id=$2`, []byte(tc.override), a)
				id := seedQueuedIssueTask(t, ctx, a, rt, i)
				var task *AgentTaskResponse
				if batch {
					w := postBatchClaimWithCapabilities(t, testWorkspaceID, []string{rt}, 1, tc.caps)
					if w.Code != http.StatusOK {
						t.Fatalf("batch: %d %s", w.Code, w.Body)
					}
					var out struct {
						Tasks []AgentTaskResponse `json:"tasks"`
					}
					json.Unmarshal(w.Body.Bytes(), &out)
					if len(out.Tasks) > 0 {
						task = &out.Tasks[0]
					}
				} else {
					req := withURLParam(newDaemonTokenRequest("POST", "/", nil, testWorkspaceID, "timeout-client"), "runtimeId", rt)
					req.Header.Set("X-Client-Capabilities", tc.caps)
					w := httptest.NewRecorder()
					testHandler.ClaimTaskByRuntime(w, req)
					want := http.StatusOK
					if w.Code != want {
						t.Fatalf("single: %d %s", w.Code, w.Body)
					}
					var out struct {
						Task *AgentTaskResponse `json:"task"`
					}
					json.Unmarshal(w.Body.Bytes(), &out)
					task = out.Task
				}
				if tc.rejected {
					if task != nil {
						t.Fatal("old client received policy run")
					}
					var status, reason string
					dbfx.QueryRow(t, `SELECT status,failure_reason FROM agent_task_queue WHERE id=$1`, id).Scan(&status, &reason)
					if status != "failed" || !strings.Contains(reason, "runtime_version_unsupported") {
						t.Fatalf("missing upgrade state %s %s", status, reason)
					}
					return
				}
				if task == nil || task.ExecutionTimeoutsSource != tc.source {
					t.Fatalf("task/source: %+v", task)
				}
				if tc.source == "client" {
					if task.ExecutionTimeouts != nil {
						t.Fatal("legacy got policy")
					}
				} else if task.ExecutionTimeouts == nil || task.ExecutionTimeouts.IdleSeconds != tc.idle || task.ExecutionTimeouts.OpenCodeIdleSeconds != 0 || *task.ExecutionTimeouts.OpenCodeShellSeconds != 5400 {
					t.Fatalf("claim policy %+v", task.ExecutionTimeouts)
				}
			})
		}
	}
}

func TestExecutionTimeoutCapabilityResolver(t *testing.T) {
	policy := []byte(`{"execution_timeouts":` + timeoutPolicyJSON + `}`)
	for _, caps := range []string{"", protocol.DaemonCapabilityExecutionTimeoutsV1} {
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("X-Client-Capabilities", caps)
		p, source, err := resolveClaimExecutionTimeouts(r, policy, nil)
		if caps == "" {
			if err == nil || !strings.Contains(err.Error(), "client_upgrade_required") {
				t.Fatalf("upgrade %v", err)
			}
		} else if err != nil || source != "workspace" || p.IdleSeconds != 7200 {
			t.Fatalf("resolved %+v %s %v", p, source, err)
		}
	}
	var malformed agent.ExecutionTimeouts
	if json.Unmarshal([]byte(`{"idle_seconds":1}`), &malformed) == nil {
		t.Fatal("malformed claim accepted")
	}
}

func TestExecutionTimeoutClaimRefresh(t *testing.T) {
	saveTimeoutWorkspace(t)
	ctx := context.Background()
	rt := createClaimReclaimRuntime(t, ctx, "timeout refresh")
	a, issue := createClaimReclaimAgentAndIssue(t, ctx, rt, "timeout refresh")
	claim := func() *AgentTaskResponse {
		t.Helper()
		seedQueuedIssueTask(t, ctx, a, rt, issue)
		req := withURLParam(newDaemonTokenRequest("POST", "/", nil, testWorkspaceID, "timeout-refresh"), "runtimeId", rt)
		req.Header.Set("X-Client-Capabilities", protocol.DaemonCapabilityExecutionTimeoutsV1)
		var out struct {
			Task *AgentTaskResponse `json:"task"`
		}
		testutil.Call(t, testHandler.ClaimTaskByRuntime, req).Want(http.StatusOK).JSON(&out)
		if out.Task == nil || out.Task.ExecutionTimeouts == nil {
			t.Fatal("missing timeout claim")
		}
		// Only isolated test rows are completed; no daemon or model is launched.
		dbfx.Exec(t, `UPDATE agent_task_queue SET status='completed',completed_at=now() WHERE id=$1`, out.Task.ID)
		return out.Task
	}
	setWorkspace := func(policy string) {
		t.Helper()
		req := withURLParam(newRequest("PATCH", "/", map[string]any{"settings": json.RawMessage(`{"execution_timeouts":` + policy + `}`)}), "id", testWorkspaceID)
		testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK)
	}
	setWorkspace(timeoutPolicyJSON)
	first := claim()
	setWorkspace(strings.Replace(timeoutPolicyJSON, "7200", "30", 1))
	second := claim()
	if first.ExecutionTimeouts.IdleSeconds != 7200 || second.ExecutionTimeouts.IdleSeconds != 30 || second.ExecutionTimeoutsSource != "workspace" {
		t.Fatalf("claim refresh failed: %+v %+v", first.ExecutionTimeouts, second.ExecutionTimeouts)
	}
	for _, clear := range []bool{false, true} {
		policy := `null`
		if !clear {
			policy = strings.Replace(timeoutPolicyJSON, "7200", "60", 1)
		}
		req := withURLParam(newRequest("PUT", "/", map[string]any{"runtime_config": json.RawMessage(`{"execution_timeouts":` + policy + `}`)}), "id", a)
		testutil.Call(t, testHandler.UpdateAgent, req).Want(http.StatusOK)
		next := claim()
		want := int64(60)
		source := "agent"
		if clear {
			want = 30
			source = "workspace"
		}
		if next.ExecutionTimeouts.IdleSeconds != want || next.ExecutionTimeoutsSource != source {
			t.Fatalf("override/clear claim: %+v source=%s", next.ExecutionTimeouts, next.ExecutionTimeoutsSource)
		}
	}
	if first.ExecutionTimeouts.IdleSeconds != 7200 {
		t.Fatal("old claim snapshot changed")
	}
}
