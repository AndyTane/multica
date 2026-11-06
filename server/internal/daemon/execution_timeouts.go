package daemon

import (
	"time"

	"github.com/multica-ai/multica/server/pkg/agent"
)

// executionConfigForTask copies scalar budgets only; shared daemon configuration
// remains immutable while other tasks run or the next claim changes its policy.
func executionConfigForTask(local Config, policy *agent.ExecutionTimeouts) Config {
	if policy == nil {
		return local
	}
	local.AgentIdleWatchdog = time.Duration(policy.IdleSeconds) * time.Second
	local.AgentToolWatchdog = time.Duration(policy.ToolIdleSeconds) * time.Second
	local.AgentTimeout = time.Duration(policy.RunSeconds) * time.Second
	local.OpenCodeIdleWatchdog = time.Duration(policy.OpenCodeIdleSeconds) * time.Second
	local.CodexSemanticInactivityTimeout = max(local.AgentIdleWatchdog, local.AgentToolWatchdog)
	// First response is the same inactivity policy, not the transport handshake.
	local.CodexFirstTurnNoProgressTimeout = local.CodexSemanticInactivityTimeout
	return local
}

func openCodeShellTimeout(policy *agent.ExecutionTimeouts, provider string) time.Duration {
	if provider != "opencode" || policy == nil || policy.OpenCodeShellSeconds == nil {
		return 0
	}
	return time.Duration(*policy.OpenCodeShellSeconds) * time.Second
}

func openCodeShellDefaultTimeout(seconds *int64, provider string) time.Duration {
	if provider != "opencode" || seconds == nil {
		return 0
	}
	return time.Duration(*seconds) * time.Second
}

func effectiveIdleWatchdog(cfg Config, dedicated time.Duration) time.Duration {
	window := cfg.AgentIdleWatchdog
	if window > 0 && dedicated > 0 && dedicated < window {
		return dedicated
	}
	return window
}
