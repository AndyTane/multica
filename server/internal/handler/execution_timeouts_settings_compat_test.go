package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Exercise the real handlers and generated SQL: timeout-only saves are patches,
// while existing configuration editors still replace all non-timeout keys.
func TestExecutionTimeoutSettingsReplacementCompatibility(t *testing.T) {
	saveTimeoutWorkspace(t)
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "timeout replacement")
	agentID, _ := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, "timeout replacement")
	updatedPolicy := strings.Replace(timeoutPolicyJSON, "7200", "3600", 1)
	for _, endpoint := range []struct {
		name, field, method, id, initial, replacement, updateSQL, readSQL string
		handler                                                           http.HandlerFunc
	}{
		{
			name: "agent", field: "runtime_config", method: "PUT", id: agentID,
			initial:     `"mode":"gateway","gateway":{"host":"gateway.example.test","port":18789,"token":"fictional-test-token","tls":true},"context_compaction":{"enabled":false}`,
			replacement: `"mode":"gateway"`,
			updateSQL:   `UPDATE agent SET runtime_config=$1 WHERE id=$2`,
			readSQL:     `SELECT runtime_config FROM agent WHERE id=$1`,
			handler:     testHandler.UpdateAgent,
		},
		{
			name: "workspace", field: "settings", method: "PATCH", id: testWorkspaceID,
			initial:     `"github_enabled":true,"pr_merge_status":"none","pr_auto_complete_enabled":false,"unrelated":{"enabled":true}`,
			replacement: `"github_enabled":false,"pr_merge_status":"qa"`,
			updateSQL:   `UPDATE workspace SET settings=$1 WHERE id=$2`,
			readSQL:     `SELECT settings FROM workspace WHERE id=$1`,
			handler:     testHandler.UpdateWorkspace,
		},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			initial := `{` + endpoint.initial + `,"execution_timeouts":` + timeoutPolicyJSON + `}`
			for _, tc := range []struct{ name, payload, want string }{
				{"timeout only preserves other settings", `{"execution_timeouts":` + updatedPolicy + `}`, `{` + endpoint.initial + `,"execution_timeouts":` + updatedPolicy + `}`},
				{"inheritance removes only timeout", `{"execution_timeouts":null}`, `{` + endpoint.initial + `}`},
				// This is the existing gateway editor's clear-all payload. The
				// workspace case also deletes the omitted PR settings keys.
				{"old editor deletes omitted keys and keeps timeout", `{` + endpoint.replacement + `}`, `{` + endpoint.replacement + `,"execution_timeouts":` + timeoutPolicyJSON + `}`},
				{"empty object clears ordinary keys and keeps timeout", `{}`, `{"execution_timeouts":` + timeoutPolicyJSON + `}`},
				{"combined replacement saves new timeout", `{` + endpoint.replacement + `,"execution_timeouts":` + updatedPolicy + `}`, `{` + endpoint.replacement + `,"execution_timeouts":` + updatedPolicy + `}`},
				{"combined replacement clears timeout", `{` + endpoint.replacement + `,"execution_timeouts":null}`, `{` + endpoint.replacement + `}`},
				{"omitted container preserves everything", "", initial},
				{"null container preserves everything", `null`, initial},
			} {
				t.Run(tc.name, func(t *testing.T) {
					dbfx.Exec(t, endpoint.updateSQL, []byte(initial), endpoint.id)
					body := map[string]any{}
					if tc.payload != "" {
						body[endpoint.field] = json.RawMessage(tc.payload)
					}
					req := withURLParam(newRequest(endpoint.method, "/", body), "id", endpoint.id)
					result := testutil.Call(t, endpoint.handler, req).Want(http.StatusOK)
					var response map[string]any
					result.JSON(&response)
					var stored []byte
					dbfx.QueryRow(t, endpoint.readSQL, endpoint.id).Scan(&stored)
					assertTimeoutSettingsJSON(t, stored, tc.want)
					// Verify API reads cannot retain a cleared gateway either.
					if endpoint.name == "agent" && tc.payload == `{`+endpoint.replacement+`}` {
						config, ok := response[endpoint.field].(map[string]any)
						if !ok || config["gateway"] != nil {
							t.Fatal("clear-all response retained gateway")
						}
					}
				})
			}
			t.Run("legacy configuration without policy", func(t *testing.T) {
				dbfx.Exec(t, endpoint.updateSQL, []byte(`{`+endpoint.initial+`}`), endpoint.id)
				req := withURLParam(newRequest(endpoint.method, "/", map[string]any{endpoint.field: json.RawMessage(`{` + endpoint.replacement + `}`)}), "id", endpoint.id)
				testutil.Call(t, endpoint.handler, req).Want(http.StatusOK)
				var stored []byte
				dbfx.QueryRow(t, endpoint.readSQL, endpoint.id).Scan(&stored)
				assertTimeoutSettingsJSON(t, stored, `{`+endpoint.replacement+`}`)
			})
		})
	}
}

func assertTimeoutSettingsJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var actual, expected map[string]any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("saved settings differ: got %s, want %s", got, want)
	}
}
