package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExecutionTimeoutPolicyValidation(t *testing.T) {
	valid := `{"idle_seconds":7200,"tool_idle_seconds":7200,"run_seconds":0,"opencode_idle_seconds":0}`
	for _, tc := range []struct{ name, raw, field string }{
		{"valid", valid, ""},
		{"zero", strings.ReplaceAll(valid, "7200", "0"), ""},
		{"missing", `{}`, "idle_seconds"},
		{"negative", strings.Replace(valid, "7200", "-1", 1), "idle_seconds"},
		{"fraction", strings.Replace(valid, "7200", "1.5", 1), "idle_seconds"},
		{"decimal", strings.Replace(valid, "7200", "1.0", 1), "idle_seconds"},
		{"string", strings.Replace(valid, "7200", `"7200"`, 1), "idle_seconds"},
		{"null", strings.Replace(valid, "7200", `null`, 1), "idle_seconds"},
		{"duration overflow", strings.Replace(valid, "7200", "9223372037", 1), "idle_seconds"},
		{"integer overflow", strings.Replace(valid, "7200", "9223372036854775808", 1), "idle_seconds"},
		{"unknown", strings.Replace(valid, "}", `,"bogus":1}`, 1), "bogus"},
		{"shell zero", strings.Replace(valid, "}", `,"opencode_shell_seconds":0}`, 1), "opencode_shell_seconds"},
		{"shell valid", strings.Replace(valid, "}", `,"opencode_shell_seconds":5400}`, 1), ""},
		{"array", `[]`, "execution_timeouts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseExecutionTimeouts([]byte(`{"execution_timeouts":` + tc.raw + `}`))
			if tc.field == "" && err != nil || tc.field != "" && (err == nil || !strings.Contains(err.Error(), tc.field)) {
				t.Fatalf("policy %s: error %v, want field %q", tc.raw, err, tc.field)
			}
		})
	}
	var future ExecutionTimeouts
	if err := json.Unmarshal([]byte(strings.Replace(valid, "}", `,"future_optional":true}`, 1)), &future); err != nil || future.IdleSeconds != 7200 {
		t.Fatalf("future claim optional field: %+v %v", future, err)
	}
	container := []byte(`{"other":true,"execution_timeouts":` + valid + `}`)
	for _, tc := range []struct {
		workspace, agent []byte
		source           string
		idle             int64
	}{
		{nil, nil, "client", 0}, {container, nil, "workspace", 7200},
		{container, []byte(`{"execution_timeouts":null}`), "workspace", 7200},
		{container, []byte(`{"execution_timeouts":` + strings.ReplaceAll(valid, "7200", "0") + `}`), "agent", 0},
	} {
		p, source, err := ResolveExecutionTimeouts(tc.workspace, tc.agent)
		if err != nil || source != tc.source || p != nil && p.IdleSeconds != tc.idle {
			t.Fatalf("resolve: %+v %s %v", p, source, err)
		}
	}
}
