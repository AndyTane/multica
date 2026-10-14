package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/taskfailure"
)

func runFakeClaudeCompaction() {
	reader := bufio.NewScanner(os.Stdin)
	count := 0
	var inputTotal, outputTotal int64
	write := func(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
	write(map[string]any{"type": "system", "subtype": "init", "session_id": compactionTestSession})
	for reader.Scan() {
		var frame struct {
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
			Message   struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(reader.Bytes(), &frame) != nil {
			os.Exit(51)
		}
		if frame.Type == "control_request" {
			if os.Getenv("COMPACTION_FAKE_FAILURE") == "slow-initialize" {
				time.Sleep(1200 * time.Millisecond)
			}
			write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": frame.RequestID, "response": map[string]any{}}})
			continue
		}
		if len(frame.Message.Content) != 1 {
			os.Exit(51)
		}
		text := frame.Message.Content[0].Text
		file, err := os.OpenFile(os.Getenv("COMPACTION_FAKE_CALLS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(52)
		}
		_, _ = fmt.Fprintln(file, text)
		_ = file.Close()
		output := "original deliverable"
		usage := claudeUsage{InputTokens: 1900, OutputTokens: 100}
		if text == "/compact" {
			count++
			mode := os.Getenv("COMPACTION_FAKE_FAILURE")
			if mode == "cold" || mode == "warm" && count > 1 {
				write(map[string]any{"type": "result", "subtype": "error", "is_error": true, "session_id": compactionTestSession, "result": "compact unavailable"})
				continue
			}
			if mode == "timeout" || mode == "warm-timeout" && count > 1 {
				_, _ = io.Copy(io.Discard, os.Stdin)
				return
			}
			if mode == "warm-exit" && count > 1 {
				return
			}
			if mode != "missing-boundary" {
				write(map[string]any{"type": "system", "subtype": "compact_boundary", "session_id": compactionTestSession})
			}
			output = "MAINTENANCE SUMMARY MUST NOT REPLACE THE DELIVERABLE"
			usage = claudeUsage{InputTokens: 500, OutputTokens: 100}
		}
		inputTotal += usage.InputTokens
		outputTotal += usage.OutputTokens
		write(map[string]any{"type": "assistant", "message": map[string]any{"id": fmt.Sprintf("message-%d", inputTotal), "model": "fake", "content": []map[string]string{{"type": "text", "text": output}}, "usage": usage}})
		write(map[string]any{"type": "result", "subtype": "success", "session_id": compactionTestSession, "result": output, "modelUsage": map[string]any{"fake": map[string]int64{"inputTokens": inputTotal, "outputTokens": outputTotal}}})
	}
}

func TestClaudeContextCompactionProcessPreservesWorkAndSession(t *testing.T) {
	for _, failure := range []string{"", "cold", "warm", "timeout", "warm-timeout", "warm-exit", "missing-boundary", "disabled", "below-threshold", "slow-initialize"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			project := filepath.Join(root, "projects", "fake-project")
			if err := os.MkdirAll(project, 0o755); err != nil {
				t.Fatal(err)
			}
			seed := `{"type":"assistant","message":{"usage":{"input_tokens":1900,"output_tokens":100}}}` + "\n"
			if err := os.WriteFile(filepath.Join(project, compactionTestSession+".jsonl"), []byte(seed), 0o600); err != nil {
				t.Fatal(err)
			}
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			calls := filepath.Join(root, "calls.txt")
			b := &claudeBackend{cfg: Config{ExecutablePath: self, Logger: slog.Default(), Env: map[string]string{"CLAUDE_CONFIG_DIR": root, "CLAUDE_FAKE_MODE": "g-claude", "COMPACTION_FAKE_CALLS": calls, "COMPACTION_FAKE_FAILURE": failure, "IS_SANDBOX": "1"}}}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			policy := &ContextCompactionPolicy{Enabled: failure != "disabled", ThresholdTokens: 1000}
			if failure == "below-threshold" {
				policy.ThresholdTokens = 80000
			}
			session, err := b.Execute(ctx, "original job", ExecOptions{Model: "fake", ResumeSessionID: compactionTestSession, ContextCompaction: policy, ContextCompactionTimeout: time.Second, EnableTaskSupplement: failure == "slow-initialize"})
			if err != nil {
				t.Fatal(err)
			}
			var transcript strings.Builder
			warned := false
			for message := range session.Messages {
				warned = warned || message.Type == MessageLog && message.Level == "warn"
				if message.Type == MessageText {
					transcript.WriteString(message.Content)
				}
			}
			result := <-session.Result
			data, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "cold" || failure == "timeout" || failure == "missing-boundary" {
				if result.Status != "failed" || strings.Contains(string(data), "original job") {
					t.Fatalf("cold failure submitted work: %+v %s", result, data)
				}
				if failure == "timeout" && (!strings.Contains(result.Error, "context compaction timed out after 1s") || result.ResumeRejected || taskfailure.Classify(result.Error) != taskfailure.ReasonAgentTimeout) {
					t.Fatalf("compaction timeout is not actionable or loses continuity: %+v", result)
				}
			} else {
				if result.Status != "completed" || result.Output != "original deliverable" || strings.Contains(transcript.String(), "MAINTENANCE") {
					t.Fatalf("maintenance changed work: %+v %q", result, transcript.String())
				}
				want := "/compact\noriginal job\n/compact\n"
				if failure == "disabled" || failure == "below-threshold" {
					want = "original job\n"
				}
				if string(data) != want {
					t.Fatalf("wrong cold/task/warm sequence: %q", data)
				}
				if !session.TerminalObserved() {
					t.Fatal("missing task terminal boundary")
				}
				if failure == "" && result.Usage["fake"].InputTokens != 2900 {
					t.Fatalf("maintenance usage lost or duplicated: %#v", result.Usage)
				}
				if strings.HasPrefix(failure, "warm") && !warned {
					t.Fatal("warm maintenance failure was not visible")
				}
			}
			if result.SessionID != compactionTestSession {
				t.Fatalf("session changed: %+v", result)
			}
		})
	}
}
