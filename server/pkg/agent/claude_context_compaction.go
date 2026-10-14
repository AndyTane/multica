package agent

import (
	"context"
	"sync"
	"time"
)

// The completion and timer compete under one lock so a late timer cannot cancel the next task.
type claudeContextMaintenance struct {
	mu         sync.Mutex
	active     bool
	expired    bool
	generation int
	timer      *time.Timer
	cancel     context.CancelFunc
	timeout    time.Duration
}

func (m *claudeContextMaintenance) begin() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generation++
	generation := m.generation
	m.active = true
	m.expired = false
	m.timer = time.AfterFunc(m.timeout, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.active && m.generation == generation {
			m.expired = true
			m.cancel()
		}
	})
}

func (m *claudeContextMaintenance) finish() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = false
	if m.timer != nil {
		m.timer.Stop()
	}
	return !m.expired
}

func (m *claudeContextMaintenance) isActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

func (m *claudeContextMaintenance) didExpire() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.expired
}

func claudePersistedContext(environment []string, cwd, sessionID string) int64 {
	root, err := claudeConfigDir(environment, cwd)
	if err != nil {
		return 0
	}
	path, err := findClaudeSessionFile(root, sessionID)
	if err != nil {
		return 0
	}
	return readContextTail(path, "claude")
}

func addCompactionUsage(work, maintenance map[string]TokenUsage) map[string]TokenUsage {
	merged := make(map[string]TokenUsage, len(work)+len(maintenance))
	for model, usage := range work {
		merged[model] = usage
	}
	for model, usage := range maintenance {
		total := merged[model]
		total.InputTokens += usage.InputTokens
		total.OutputTokens += usage.OutputTokens
		total.CacheReadTokens += usage.CacheReadTokens
		total.CacheWriteTokens += usage.CacheWriteTokens
		total.CostUSDTicks += usage.CostUSDTicks
		merged[model] = total
	}
	return merged
}
