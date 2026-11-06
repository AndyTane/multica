package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestOpenCodeShellDefaultClaims(t *testing.T) {
	const toolPolicy = `{"execution_timeouts":{"idle_seconds":17,"tool_idle_seconds":10800,"run_seconds":0,"opencode_idle_seconds":23}}`
	const explicitPolicy = `{"execution_timeouts":{"idle_seconds":19,"tool_idle_seconds":1800,"run_seconds":0,"opencode_idle_seconds":29,"opencode_shell_seconds":900}}`
	const zeroPolicy = `{"execution_timeouts":{"idle_seconds":31,"tool_idle_seconds":0,"run_seconds":0,"opencode_idle_seconds":37}}`
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%v", batch), func(t *testing.T) {
			saveTimeoutWorkspace(t)
			ctx := context.Background()
			rt := createClaimReclaimRuntime(t, ctx, "shell default")
			a, issue := createClaimReclaimAgentAndIssue(t, ctx, rt, "shell default")
			var firstSnapshot []byte
			var first *AgentTaskResponse
			for _, tc := range []struct {
				name, workspace, override, source string
				seconds                           int64
			}{
				{"no_policy", `{}`, `{}`, "client", 7200},
				{"workspace_tool", toolPolicy, `{}`, "workspace", 10800},
				{"explicit_agent_shell", toolPolicy, explicitPolicy, "agent", 0},
				{"zero_agent_tool", toolPolicy, zeroPolicy, "agent", 7200},
				{"clear_agent", toolPolicy, `{}`, "workspace", 10800},
				{"clear_workspace", `{}`, `{}`, "client", 7200},
			} {
				t.Run(tc.name, func(t *testing.T) {
					dbfx.Exec(t, `UPDATE workspace SET settings=$1 WHERE id=$2`, []byte(tc.workspace), testWorkspaceID)
					dbfx.Exec(t, `UPDATE agent SET runtime_config=$1 WHERE id=$2`, []byte(tc.override), a)
					seedQueuedIssueTask(t, ctx, a, rt, issue)
					var out struct {
						Task  *AgentTaskResponse  `json:"task"`
						Tasks []AgentTaskResponse `json:"tasks"`
					}
					if batch {
						w := postBatchClaimWithCapabilities(t, testWorkspaceID, []string{rt}, 1, protocol.DaemonCapabilityExecutionTimeoutsV1)
						if w.Code != http.StatusOK {
							t.Fatalf("batch claim: %d %s", w.Code, w.Body)
						}
						if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
							t.Fatal(err)
						}
						if len(out.Tasks) != 1 {
							t.Fatalf("batch tasks: %d", len(out.Tasks))
						}
						out.Task = &out.Tasks[0]
					} else {
						req := withURLParam(newDaemonTokenRequest("POST", "/", nil, testWorkspaceID, "shell-default"), "runtimeId", rt)
						req.Header.Set("X-Client-Capabilities", protocol.DaemonCapabilityExecutionTimeoutsV1)
						testutil.Call(t, testHandler.ClaimTaskByRuntime, req).Want(http.StatusOK).JSON(&out)
					}
					if out.Task == nil || out.Task.ExecutionTimeoutsSource != tc.source {
						t.Fatalf("claim/source: %+v", out.Task)
					}
					raw, err := json.Marshal(out.Task)
					if err != nil {
						t.Fatal(err)
					}
					// Inspect the actual response wire, including omission for explicit policy.
					var wire map[string]json.RawMessage
					if err := json.Unmarshal(raw, &wire); err != nil {
						t.Fatal(err)
					}
					value, present := wire["opencode_shell_default_seconds"]
					var seconds int64
					if present {
						if err := json.Unmarshal(value, &seconds); err != nil {
							t.Fatal(err)
						}
					}
					if seconds != tc.seconds || present != (tc.seconds > 0) {
						t.Errorf("default = %s (present %v), want %d", value, present, tc.seconds)
					}
					if tc.source == "client" && out.Task.ExecutionTimeouts != nil {
						t.Error("shell default synthesized a complete platform policy")
					}
					if tc.name == "explicit_agent_shell" && (out.Task.ExecutionTimeouts == nil || out.Task.ExecutionTimeouts.OpenCodeShellSeconds == nil || *out.Task.ExecutionTimeouts.OpenCodeShellSeconds != 900) {
						t.Error("explicit shell policy was changed")
					}
					var workspace, override []byte
					dbfx.QueryRow(t, `SELECT settings FROM workspace WHERE id=$1`, testWorkspaceID).Scan(&workspace)
					dbfx.QueryRow(t, `SELECT runtime_config FROM agent WHERE id=$1`, a).Scan(&override)
					assertTimeoutSettingsJSON(t, workspace, tc.workspace)
					assertTimeoutSettingsJSON(t, override, tc.override)
					if first == nil {
						first, firstSnapshot = out.Task, raw
					}
					// Complete only this fixture row so the next claim reads fresh policy.
					dbfx.Exec(t, `UPDATE agent_task_queue SET status='completed',completed_at=now() WHERE id=$1`, out.Task.ID)
				})
			}
			frozen, err := json.Marshal(first)
			if err != nil || string(frozen) != string(firstSnapshot) {
				t.Fatal("later claims changed the first run snapshot")
			}
		})
	}
}
