package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

type codexCompactionState struct {
	threadID       string
	turnID         string
	previousTurnID string
	sawItem        bool
	done           chan error
}

func (c *codexClient) isCompacting() bool {
	c.compactionMu.Lock()
	defer c.compactionMu.Unlock()
	return c.compaction != nil
}

func (c *codexClient) compactContext(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	state := &codexCompactionState{threadID: c.getThreadID(), previousTurnID: c.usageTurnID(), done: make(chan error, 1)}
	c.compactionMu.Lock()
	if c.compaction != nil {
		c.compactionMu.Unlock()
		return errors.New("context compaction is already active")
	}
	c.compaction = state
	c.compactionMu.Unlock()
	defer func() {
		c.compactionMu.Lock()
		if c.compaction == state {
			c.compaction = nil
		}
		c.compactionMu.Unlock()
	}()
	if _, err := c.request(ctx, "thread/compact/start", map[string]any{"threadId": state.threadID}); err != nil {
		return fmt.Errorf("context compaction request: %w", err)
	}
	select {
	case err := <-state.done:
		if err == nil {
			c.usageMu.Lock()
			c.contextTokens = 0
			c.usageMu.Unlock()
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-c.processDone:
		return errors.New("Codex exited during context compaction")
	}
}

// Maintenance notifications never flow into the task's output or terminal gate.
func (c *codexClient) handleCompactionNotification(method string, params map[string]any) bool {
	c.compactionMu.Lock()
	defer c.compactionMu.Unlock()
	state := c.compaction
	if state == nil {
		return false
	}
	if threadID, _ := params["threadId"].(string); threadID != "" && threadID != state.threadID {
		return true
	}
	finish := func(err error) {
		select {
		case state.done <- err:
		default:
		}
	}
	switch method {
	case "turn/started":
		id := extractNestedString(params, "turn", "id")
		if id != "" && id != state.previousTurnID && state.turnID == "" {
			state.turnID = id
		}
	case "item/started", "item/completed":
		if extractNestedString(params, "item", "type") == "contextCompaction" {
			if turnID, _ := params["turnId"].(string); turnID != "" {
				if turnID == state.previousTurnID || state.turnID != "" && turnID != state.turnID {
					return true
				}
				state.turnID = turnID
				state.sawItem = true
			}
		}
	case "thread/tokenUsage/updated":
		c.updateTokenUsageForTurn(params, state.turnID)
	case "turn/completed":
		id := extractNestedString(params, "turn", "id")
		if state.turnID == "" || id != state.turnID {
			return true
		}
		status := extractNestedString(params, "turn", "status")
		if status == "completed" && state.sawItem {
			finish(nil)
		} else if status != "completed" {
			finish(errors.New("Codex context compaction failed or was interrupted"))
		}
	case "error":
		if retry, _ := params["willRetry"].(bool); !retry {
			finish(errors.New("Codex reported a context compaction error"))
		}
	}
	return true
}

func codexPersistedContext(codexHome, threadID string) int64 {
	var latest string
	var newest time.Time
	for _, path := range findCodexSessionRollouts(codexSessionRoot(codexHome), threadID) {
		if info, err := os.Stat(path); err == nil && (latest == "" || info.ModTime().After(newest)) {
			latest = path
			newest = info.ModTime()
		}
	}
	return readContextTail(latest, "codex")
}

func (c *codexClient) currentContextTokens() int64 {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.contextTokens
}

func codexCompactionFailureReason(ctx context.Context, err error, timeout time.Duration) string {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return "was canceled"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "was interrupted by the task deadline"
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("timed out after %s", timeout)
	case errors.Is(err, context.Canceled):
		return "was canceled"
	default:
		// Provider errors can include opaque credentials; do not copy them here.
		return "failed"
	}
}

func (c *codexClient) compactForRun(ctx context.Context, opts ExecOptions, stage string, tokens int64) error {
	if c.onMessage != nil {
		c.onMessage(contextCompactionStatus(ContextCompactionStarted))
		c.onMessage(Message{Type: MessageLog, Level: "info", Content: fmt.Sprintf("Compacting context %s (%d tokens).", stage, tokens)})
	}
	err := c.compactContext(ctx, compactionTimeout(opts, "codex"))
	if c.onMessage != nil {
		if err != nil {
			c.onMessage(contextCompactionStatus(ContextCompactionFailed))
			reason := codexCompactionFailureReason(ctx, err, compactionTimeout(opts, "codex"))
			c.onMessage(Message{Type: MessageLog, Level: "warn", Content: fmt.Sprintf("Context compaction %s %s; the existing session was preserved.", stage, reason)})
		} else {
			c.onMessage(contextCompactionStatus(ContextCompactionCompleted))
			c.onMessage(Message{Type: MessageLog, Level: "info", Content: "Context compaction completed."})
		}
	}
	return err
}
