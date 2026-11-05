package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// ExecutionTimeouts is a complete per-run platform policy. A nil policy retains
// client configuration; explicit zeroes have the documented disabling semantics.
type ExecutionTimeouts struct {
	IdleSeconds          int64  `json:"idle_seconds"`
	ToolIdleSeconds      int64  `json:"tool_idle_seconds"`
	RunSeconds           int64  `json:"run_seconds"`
	OpenCodeIdleSeconds  int64  `json:"opencode_idle_seconds"`
	OpenCodeShellSeconds *int64 `json:"opencode_shell_seconds,omitempty"`
}

const MaxExecutionTimeoutSeconds = int64(math.MaxInt64 / int64(time.Second))

func (p ExecutionTimeouts) Validate() error {
	for _, f := range []struct {
		name  string
		value int64
	}{
		{"idle_seconds", p.IdleSeconds}, {"tool_idle_seconds", p.ToolIdleSeconds},
		{"run_seconds", p.RunSeconds}, {"opencode_idle_seconds", p.OpenCodeIdleSeconds},
	} {
		if f.value < 0 || f.value > MaxExecutionTimeoutSeconds {
			return fmt.Errorf("execution_timeouts.%s must be a non-negative integer <= %d", f.name, MaxExecutionTimeoutSeconds)
		}
	}
	if p.OpenCodeShellSeconds != nil && (*p.OpenCodeShellSeconds <= 0 || *p.OpenCodeShellSeconds > MaxExecutionTimeoutSeconds) {
		return fmt.Errorf("execution_timeouts.opencode_shell_seconds must be a positive integer <= %d", MaxExecutionTimeoutSeconds)
	}
	return nil
}

// UnmarshalJSON rejects incomplete policies, wrong shapes and non-integer values.
// This also makes a malformed claim fail closed on the client.
func (p *ExecutionTimeouts) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return fmt.Errorf("execution_timeouts must be an object")
	}
	var next ExecutionTimeouts
	for _, f := range []struct {
		name   string
		target *int64
	}{
		{"idle_seconds", &next.IdleSeconds}, {"tool_idle_seconds", &next.ToolIdleSeconds},
		{"run_seconds", &next.RunSeconds}, {"opencode_idle_seconds", &next.OpenCodeIdleSeconds},
	} {
		value, exists := fields[f.name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, f.target) != nil {
			return fmt.Errorf("execution_timeouts.%s must be an integer", f.name)
		}
		delete(fields, f.name)
	}
	if raw, ok := fields["opencode_shell_seconds"]; ok {
		var n int64
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &n) != nil {
			return fmt.Errorf("execution_timeouts.opencode_shell_seconds must be a positive integer")
		}
		next.OpenCodeShellSeconds = &n
		delete(fields, "opencode_shell_seconds")
	}
	if err := next.Validate(); err != nil {
		return err
	}
	*p = next
	return nil
}

// ParseExecutionTimeouts reads the reserved key from workspace/agent JSON.
func ParseExecutionTimeouts(raw []byte) (*ExecutionTimeouts, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var container map[string]json.RawMessage
	if err := json.Unmarshal(raw, &container); err != nil || container == nil {
		return nil, fmt.Errorf("settings must be an object")
	}
	value, ok := container["execution_timeouts"]
	if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, nil
	}
	var policy ExecutionTimeouts
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err == nil {
		for key := range fields {
			switch key {
			case "idle_seconds", "tool_idle_seconds", "run_seconds", "opencode_idle_seconds", "opencode_shell_seconds":
			default:
				return nil, fmt.Errorf("execution_timeouts.%s is unknown", key)
			}
		}
	}
	if err := json.Unmarshal(value, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func ResolveExecutionTimeouts(workspace, runtimeConfig []byte) (*ExecutionTimeouts, string, error) {
	p, err := ParseExecutionTimeouts(runtimeConfig)
	if err != nil {
		return nil, "", err
	}
	if p != nil {
		return p, "agent", nil
	}
	p, err = ParseExecutionTimeouts(workspace)
	if p != nil {
		return p, "workspace", err
	}
	return nil, "client", err
}
