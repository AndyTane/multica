//go:build agentintegration

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeContextCompactionRealSession(t *testing.T) {
	requireRealAgentSmoke(t)
	testRealContextCompaction(t, "claude")
}

func TestCodexContextCompactionRealSession(t *testing.T) {
	requireRealAgentSmoke(t)
	testRealContextCompaction(t, "codex")
}

// The opt-in check in each test precedes executable lookup and account access.
func testRealContextCompaction(t *testing.T, provider string) {
	t.Helper()
	path := os.Getenv("MULTICA_G_AGENT_PATH")
	if path == "" {
		var err error
		path, err = exec.LookPath(provider)
		if err != nil {
			t.Fatal(err)
		}
	}
	environment := make(map[string]string)
	if raw := os.Getenv("MULTICA_G_AGENT_ENV"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &environment); err != nil {
			t.Fatal("invalid test environment JSON")
		}
	}
	if provider == "codex" {
		source := environment["CODEX_HOME"]
		if source == "" {
			source = os.Getenv("CODEX_HOME")
		}
		if source == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			source = filepath.Join(home, ".codex")
		}
		isolated := t.TempDir()
		for _, name := range []string{"config.toml", "auth.json"} {
			data, err := os.ReadFile(filepath.Join(source, name))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Fatal("cannot read native test configuration")
			}
			if err := os.WriteFile(filepath.Join(isolated, name), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		environment["CODEX_HOME"] = isolated
	}
	b, err := New(provider, Config{ExecutablePath: path, Env: environment, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	const marker = "青竹花园-7D39"
	var material strings.Builder
	for bed := 1; bed <= 60; bed++ {
		fmt.Fprintf(&material, "花圃 %d：种植薄荷、罗勒和百里香，早晨检查土壤湿度。雨后减少浇水，保持通风；每周记录新叶和花期，收获时留下健康枝条。\n", bed)
	}
	seed := "我们在整理一份园艺项目资料。项目代号是「" + marker + "」。请记住代号供下一条消息询问，不要使用工具或操作文件。这条消息只回复 SEED_OK。以下是项目的花圃安排：\n" + material.String()
	opts := ExecOptions{Cwd: t.TempDir(), Model: os.Getenv("MULTICA_G_AGENT_MODEL"), McpConfig: json.RawMessage(`{"mcpServers":{}}`), Timeout: 5 * time.Minute}
	if provider == "claude" {
		opts.ExtraArgs = []string{"--tools", ""}
	}
	first, _ := collectRealCompactionRun(t, ctx, b, seed, opts)
	if first.Status != "completed" || first.SessionID == "" || normalizeCompactionTestReply(first.Output) != "SEED_OK" {
		t.Fatalf("seed failed: status=%s session_present=%t error=%s", first.Status, first.SessionID != "", first.Error)
	}
	opts.ResumeSessionID = first.SessionID
	opts.ContextCompaction = &ContextCompactionPolicy{Enabled: true, ThresholdTokens: 1000}
	second, completedCompactions := collectRealCompactionRun(t, ctx, b, "刚才的园艺项目代号是什么？只回复完整代号，不要使用工具或操作文件。", opts)
	if second.Status != "completed" || second.SessionID != first.SessionID || normalizeCompactionTestReply(second.Output) != marker {
		t.Fatalf("resume did not preserve memory: status=%s same_session=%t output=%q error=%s", second.Status, second.SessionID == first.SessionID, second.Output, second.Error)
	}
	if completedCompactions < 2 {
		t.Fatalf("expected both pre-task and post-task compaction, completed=%d", completedCompactions)
	}
	t.Logf("%s: same session, remembered marker, %d completed compactions", provider, completedCompactions)
}

func normalizeCompactionTestReply(reply string) string {
	return strings.Trim(strings.TrimSpace(reply), "\"'「」` ")
}

func TestContextCompactionTestReplyFormatting(t *testing.T) {
	for _, reply := range []string{"青竹花园-7D39", "「青竹花园-7D39」", "\"青竹花园-7D39\"", "`青竹花园-7D39`"} {
		if normalizeCompactionTestReply(reply) != "青竹花园-7D39" {
			t.Fatal("test reply formatting changed the remembered project code")
		}
	}
}

func collectRealCompactionRun(t *testing.T, ctx context.Context, b Backend, prompt string, opts ExecOptions) (Result, int) {
	t.Helper()
	session, err := b.Execute(ctx, prompt, opts)
	if err != nil {
		t.Fatal(err)
	}
	completed := 0
	for message := range session.Messages {
		if message.Type == MessageToolUse {
			t.Error("test conversation unexpectedly used a tool")
		}
		if message.Type == MessageLog && message.Content == "Context compaction completed." {
			completed++
		}
		if message.Type == MessageLog && message.Level == "warn" {
			t.Log(message.Content)
		}
	}
	return <-session.Result, completed
}
