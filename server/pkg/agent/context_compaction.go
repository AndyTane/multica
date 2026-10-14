package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

const DefaultContextCompactionThreshold int64 = 80000
const contextTailLimit int64 = 8 * 1024 * 1024
const defaultContextCompactionTimeout = 2 * time.Minute

// ContextCompactionPolicy is stored in agent.runtime_config.context_compaction.
type ContextCompactionPolicy struct {
	Enabled         bool  `json:"enabled"`
	ThresholdTokens int64 `json:"threshold_tokens"`
}

func ParseContextCompaction(raw json.RawMessage) (ContextCompactionPolicy, error) {
	policy := ContextCompactionPolicy{Enabled: true, ThresholdTokens: DefaultContextCompactionThreshold}
	if len(raw) == 0 {
		return policy, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return policy, errors.New("runtime_config must be an object")
	}
	value, exists := root["context_compaction"]
	if !exists {
		return policy, nil
	}
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return policy, errors.New("context_compaction must be an object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err != nil {
		return policy, errors.New("context_compaction must be an object")
	}
	for _, key := range []string{"enabled", "threshold_tokens"} {
		if field, exists := fields[key]; exists && bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
			return policy, errors.New("context_compaction fields cannot be null")
		}
	}
	if err := json.Unmarshal(value, &policy); err != nil {
		return policy, errors.New("context_compaction requires a boolean enabled and integer threshold_tokens")
	}
	if policy.ThresholdTokens < 1000 || policy.ThresholdTokens > 2000000 {
		return policy, errors.New("context_compaction.threshold_tokens must be between 1000 and 2000000")
	}
	return policy, nil
}

func (p ContextCompactionPolicy) ShouldCompact(tokens int64) bool {
	return p.Enabled && p.ThresholdTokens > 0 && tokens >= p.ThresholdTokens
}

func compactionTimeout(opts ExecOptions) time.Duration {
	if opts.ContextCompactionTimeout > 0 {
		return opts.ContextCompactionTimeout
	}
	return defaultContextCompactionTimeout
}

func compactionDue(opts ExecOptions, tokens int64) bool {
	return opts.ContextCompaction != nil && opts.ContextCompaction.ShouldCompact(tokens)
}

func compactionEnabled(opts ExecOptions) bool {
	return opts.ContextCompaction != nil && opts.ContextCompaction.Enabled
}

// readContextTail reads only a bounded suffix. Zero means unknown, never billing totals.
func readContextTail(path, provider string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	start := max(int64(0), info.Size()-contextTailLimit)
	data, err := io.ReadAll(io.NewSectionReader(f, start, contextTailLimit))
	if err != nil {
		return 0
	}
	if start > 0 {
		index := bytes.IndexByte(data, '\n')
		if index < 0 {
			return 0
		}
		data = data[index+1:]
	}
	for len(data) > 0 {
		data = bytes.TrimRight(data, "\r\n")
		index := bytes.LastIndexByte(data, '\n')
		line := data[index+1:]
		var item struct {
			Type            string          `json:"type"`
			Subtype         string          `json:"subtype"`
			IsSidechain     bool            `json:"isSidechain"`
			ParentToolUseID string          `json:"parent_tool_use_id"`
			Message         json.RawMessage `json:"message"`
			Payload         json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(line, &item) == nil {
			if item.Type == "compacted" || item.Subtype == "compact_boundary" {
				return 0
			}
			var payload struct {
				Type string `json:"type"`
				Info *struct {
					LastTokenUsage *codexRawTokenUsage `json:"last_token_usage"`
				} `json:"info"`
			}
			_ = json.Unmarshal(item.Payload, &payload)
			if payload.Type == "context_compacted" {
				return 0
			}
			if provider == "claude" && item.Type == "assistant" && !item.IsSidechain && item.ParentToolUseID == "" {
				var message claudeMessageContent
				if json.Unmarshal(item.Message, &message) == nil {
					if tokens := claudeContextTokens(message.Usage); tokens > 0 {
						return tokens
					}
				}
			}
			if provider == "codex" && item.Type == "event_msg" && payload.Type == "token_count" && payload.Info != nil && payload.Info.LastTokenUsage != nil {
				usage := payload.Info.LastTokenUsage
				if tokens := usage.InputTokens + usage.OutputTokens; tokens > 0 {
					return tokens
				}
			}
		}
		if index < 0 {
			break
		}
		data = data[:index]
	}
	return 0
}

func claudeContextTokens(usage *claudeUsage) int64 {
	if usage == nil {
		return 0
	}
	return max(0, usage.InputTokens) + max(0, usage.CacheReadInputTokens) + max(0, usage.CacheCreationInputTokens) + max(0, usage.OutputTokens)
}
