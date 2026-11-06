package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/multica-ai/multica/server/pkg/agent"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func allowExecutionTimeoutWrite(w http.ResponseWriter, r *http.Request, raw []byte) bool {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if _, sent := fields["execution_timeouts"]; sent && r.Header.Get("X-Agent-ID") != "" {
		writeError(w, http.StatusForbidden, "task tokens cannot change execution_timeouts")
		return false
	}
	return true
}

func resolveClaimExecutionTimeouts(r *http.Request, workspace, runtimeConfig []byte) (*agent.ExecutionTimeouts, string, error) {
	policy, source, err := agent.ResolveExecutionTimeouts(workspace, runtimeConfig)
	if err != nil {
		return nil, "", err
	}
	if policy != nil && !requestHasClientCapability(r, protocol.DaemonCapabilityExecutionTimeoutsV1) {
		return nil, "", fmt.Errorf("execution_timeouts_client_upgrade_required: upgrade this client to apply the server's run timeout settings")
	}
	return policy, source, nil
}

// Keep the inferred shell default separate from the complete platform policy:
// a nil policy must continue to preserve the client's other timeout settings.
func claimOpenCodeShellDefaultSeconds(policy *agent.ExecutionTimeouts) *int64 {
	if policy != nil && policy.OpenCodeShellSeconds != nil {
		return nil // Explicit shell settings retain their existing precedence.
	}
	seconds := int64(7200)
	if policy != nil && policy.ToolIdleSeconds > 0 {
		seconds = policy.ToolIdleSeconds
	}
	return &seconds
}
