package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const compactionTestSession = "00000000-0000-4000-8000-000000000123"

func runFakeCodexCompaction() {
	reader := bufio.NewScanner(os.Stdin)
	write := func(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
	notify := func(method string, params any) { write(map[string]any{"method": method, "params": params}) }
	compactCount := 0
	for reader.Scan() {
		var req struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(reader.Bytes(), &req) != nil {
			os.Exit(41)
		}
		if req.ID == 0 {
			continue
		}
		file, err := os.OpenFile(os.Getenv("COMPACTION_FAKE_CALLS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(42)
		}
		_, _ = fmt.Fprintln(file, req.Method)
		_ = file.Close()
		reply := func(result any) { write(map[string]any{"id": req.ID, "result": result}) }
		switch req.Method {
		case "initialize":
			reply(map[string]any{})
		case "thread/start", "thread/resume":
			reply(map[string]any{"thread": map[string]any{"id": compactionTestSession}})
		case "thread/name/set":
			reply(map[string]any{})
		case "thread/compact/start":
			compactCount++
			mode := os.Getenv("COMPACTION_FAKE_FAILURE")
			if mode == "cold" || mode == "warm" && compactCount > 1 {
				write(map[string]any{"id": req.ID, "error": map[string]any{"code": -32601, "message": "compact unavailable"}})
				continue
			}
			reply(map[string]any{})
			if mode == "timeout" || mode == "warm-timeout" && compactCount > 1 {
				continue
			}
			if mode == "warm-exit" && compactCount > 1 {
				return
			}
			turn := fmt.Sprintf("compact-%d", compactCount)
			notify("turn/started", map[string]any{"threadId": compactionTestSession, "turn": map[string]any{"id": turn, "status": "inProgress"}})
			notify("item/completed", map[string]any{"threadId": compactionTestSession, "turnId": turn, "item": map[string]any{"id": "summary", "type": "contextCompaction"}})
			notify("item/completed", map[string]any{"threadId": compactionTestSession, "turnId": turn, "item": map[string]any{"id": "maintenance-text", "type": "agentMessage", "phase": "final_answer", "text": "MAINTENANCE SUMMARY MUST NOT REPLACE THE DELIVERABLE"}})
			notify("turn/completed", map[string]any{"threadId": compactionTestSession, "turn": map[string]any{"id": turn, "status": "completed"}})
		case "turn/start":
			reply(map[string]any{"turn": map[string]any{"id": "work"}})
			notify("turn/started", map[string]any{"threadId": compactionTestSession, "turn": map[string]any{"id": "work", "status": "inProgress"}})
			notify("thread/tokenUsage/updated", map[string]any{"threadId": compactionTestSession, "turnId": "work", "tokenUsage": map[string]any{"total": map[string]any{"inputTokens": 900000, "outputTokens": 100}, "last": map[string]any{"inputTokens": 1900, "cachedInputTokens": 1800, "outputTokens": 100}}})
			notify("item/completed", map[string]any{"threadId": compactionTestSession, "turnId": "work", "item": map[string]any{"id": "answer", "type": "agentMessage", "phase": "final_answer", "text": "original deliverable"}})
			notify("turn/completed", map[string]any{"threadId": compactionTestSession, "turn": map[string]any{"id": "work", "status": "completed"}})
		default:
			os.Exit(43)
		}
	}
}

func TestCodexContextCompactionProcessPreservesWorkAndSession(t *testing.T) {
	for _, failure := range []string{"", "cold", "warm", "timeout", "warm-timeout", "warm-exit", "disabled", "below-threshold"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			sessions := filepath.Join(root, "sessions")
			if err := os.Mkdir(sessions, 0o755); err != nil {
				t.Fatal(err)
			}
			seed := `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1900,"output_tokens":100}}}}` + "\n"
			if err := os.WriteFile(filepath.Join(sessions, "rollout-test-"+compactionTestSession+".jsonl"), []byte(seed), 0o600); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			calls := filepath.Join(root, "calls.txt")
			b := &codexBackend{cfg: Config{ExecutablePath: executable, Logger: slog.Default(), Env: map[string]string{"CODEX_HOME": root, "CLAUDE_FAKE_MODE": "g-codex", "COMPACTION_FAKE_CALLS": calls, "COMPACTION_FAKE_FAILURE": failure}}}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			policy := &ContextCompactionPolicy{Enabled: failure != "disabled", ThresholdTokens: 1000}
			if failure == "below-threshold" {
				policy.ThresholdTokens = 80000
			}
			session, err := b.Execute(ctx, "original job", ExecOptions{ResumeSessionID: compactionTestSession, ContextCompaction: policy, ContextCompactionTimeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			var transcript strings.Builder
			for message := range session.Messages {
				if message.Type == MessageText {
					transcript.WriteString(message.Content)
				}
			}
			result := <-session.Result
			data, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "cold" || failure == "timeout" {
				if result.Status != "failed" || strings.Contains(string(data), "turn/start") {
					t.Fatalf("cold failure submitted work: %+v %s", result, data)
				}
			} else {
				if result.Status != "completed" || result.Output != "original deliverable" || strings.Contains(transcript.String(), "MAINTENANCE") {
					t.Fatalf("maintenance changed work: %+v %q", result, transcript.String())
				}
				want := 2
				if failure == "disabled" || failure == "below-threshold" {
					want = 0
				}
				if strings.Count(string(data), "thread/compact/start") != want {
					t.Fatalf("expected cold + warm compaction: %s", data)
				}
				if !session.TerminalObserved() {
					t.Fatal("missing task terminal boundary")
				}
			}
			if result.SessionID != compactionTestSession {
				t.Fatalf("session changed: %+v", result)
			}
		})
	}
}
