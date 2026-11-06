package daemon

import (
	"encoding/json"
	"fmt"

	"github.com/multica-ai/multica/server/pkg/agent"
)

// A source asserting server control without a complete policy must never cause
// execution with the local fallback. This decoder covers HTTP and RPC claims.
func (t *Task) UnmarshalJSON(raw []byte) error {
	type taskWire Task
	var next taskWire
	if err := json.Unmarshal(raw, &next); err != nil {
		return err
	}
	if (next.ExecutionTimeoutsSource == "agent" || next.ExecutionTimeoutsSource == "workspace") && next.ExecutionTimeouts == nil {
		return fmt.Errorf("execution_timeouts is required for source %s", next.ExecutionTimeoutsSource)
	}
	if seconds := next.OpenCodeShellDefaultSeconds; seconds != nil && (*seconds <= 0 || *seconds > agent.MaxExecutionTimeoutSeconds) {
		return fmt.Errorf("opencode_shell_default_seconds must be a positive integer <= %d", agent.MaxExecutionTimeoutSeconds)
	}
	*t = Task(next)
	return nil
}
