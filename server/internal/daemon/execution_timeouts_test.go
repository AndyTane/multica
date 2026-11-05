package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/agent"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// This temporary test subprocess acts like an OpenCode stream that reports its
// tool only after completion. It never resolves an installed model CLI.
func TestExecutionTimeoutProcess(t *testing.T) {
	if os.Getenv("CLAWSTA143_TIMEOUT_CHILD") != "1" {
		return
	}
	for _, arg := range os.Args {
		if arg == "--version" {
			fmt.Println("1.17.7")
			os.Exit(0)
		}
	}
	ms, _ := strconv.Atoi(os.Getenv("CLAWSTA143_CHILD_MS"))
	time.Sleep(time.Duration(ms) * time.Millisecond)
	fmt.Println(`{"type":"tool_use","part":{"tool":"bash","callID":"compile","state":{"status":"completed","input":{},"output":"done"}}}`)
	fmt.Printf("{\"type\":\"text\",\"part\":{\"text\":\"shell=%s\"}}\n", os.Getenv("OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS"))
	os.Exit(0)
}

func TestExecutionTimeoutSnapshot(t *testing.T) {
	local := Config{AgentIdleWatchdog: time.Second, AgentToolWatchdog: time.Second, AgentTimeout: time.Second, OpenCodeIdleWatchdog: 10 * time.Minute, CodexSemanticInactivityTimeout: time.Second, CodexFirstTurnNoProgressTimeout: time.Second}
	var wg sync.WaitGroup
	for i := int64(0); i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := &agent.ExecutionTimeouts{IdleSeconds: 7200 + i, ToolIdleSeconds: 8000 + i, RunSeconds: 0, OpenCodeIdleSeconds: 0}
			next := executionConfigForTask(local, p)
			if next.AgentIdleWatchdog != time.Duration(p.IdleSeconds)*time.Second || next.CodexSemanticInactivityTimeout != time.Duration(p.ToolIdleSeconds)*time.Second || next.CodexFirstTurnNoProgressTimeout != next.CodexSemanticInactivityTimeout || next.AgentTimeout != 0 || next.OpenCodeIdleWatchdog != 0 {
				t.Errorf("snapshot: %+v", next)
			}
		}()
	}
	wg.Wait()
	if local.AgentIdleWatchdog != time.Second || executionConfigForTask(local, nil).AgentTimeout != time.Second {
		t.Fatal("shared/local config changed")
	}
	shell := int64(5400)
	for _, provider := range []string{"dsh", "codex", "codearts"} {
		if openCodeShellTimeout(&agent.ExecutionTimeouts{OpenCodeShellSeconds: &shell}, provider) != 0 {
			t.Fatal("OpenCode environment leaked to another provider")
		}
	}
}

func TestExecutionTimeoutClientClaim(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, payload := range []string{``, `,"execution_timeouts":{"idle_seconds":3,"tool_idle_seconds":4,"run_seconds":0,"opencode_idle_seconds":0}`, `,"execution_timeouts":{"idle_seconds":3,"tool_idle_seconds":4,"run_seconds":0,"opencode_idle_seconds":0,"future_optional":true}`, `,"execution_timeouts":{"idle_seconds":1.5}`, `,"execution_timeouts_source":"workspace","execution_timeouts":null`} {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.Header.Get("X-Client-Capabilities"), protocol.DaemonCapabilityExecutionTimeoutsV1) {
					t.Error("client omitted capability")
				}
				task := `{"id":"isolated"` + payload + `}`
				if batch {
					fmt.Fprint(w, `{"tasks":[`+task+`]}`)
				} else {
					fmt.Fprint(w, `{"task":`+task+`}`)
				}
			}))
			client := NewClient(srv.URL)
			var task *Task
			var err error
			if batch {
				var tasks []*Task
				tasks, err = client.ClaimTasks(context.Background(), "daemon", []string{"runtime"}, 1)
				if len(tasks) > 0 {
					task = tasks[0]
				}
			} else {
				task, err = client.ClaimTask(context.Background(), "runtime")
			}
			srv.Close()
			if strings.Contains(payload, "1.5") || strings.Contains(payload, `"execution_timeouts":null`) {
				if err == nil {
					t.Fatal("malformed server policy silently accepted")
				}
				continue
			}
			if err != nil || task == nil {
				t.Fatalf("claim %+v %v", task, err)
			}
			if payload == "" && task.ExecutionTimeouts != nil || payload != "" && (task.ExecutionTimeouts == nil || task.ExecutionTimeouts.IdleSeconds != 3) {
				t.Fatalf("decoded policy %+v", task.ExecutionTimeouts)
			}
		}
	}
}

func TestExecutionTimeoutCodexProcess(t *testing.T) {
	if os.Getenv("CLAWSTA143_CODEX_CHILD") != "1" {
		return
	}
	for _, a := range os.Args {
		if a == "--version" {
			fmt.Println("codex-cli 0.0.0-test")
			os.Exit(0)
		}
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if decoder.Decode(&req) != nil {
			os.Exit(0)
		}
		if len(req.ID) == 0 {
			continue
		}
		result := any(map[string]any{})
		if req.Method == "thread/start" || req.Method == "thread/resume" {
			result = map[string]any{"thread": map[string]any{"id": "thr-timeout"}}
		}
		if req.Method == "turn/start" {
			result = map[string]any{"turn": map[string]any{"id": "turn-timeout"}}
		}
		encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		if req.Method == "turn/start" {
			fmt.Println(`{"jsonrpc":"2.0","method":"turn/started","params":{"threadId":"thr-timeout","turn":{"id":"turn-timeout"}}}`)
			time.Sleep(1200 * time.Millisecond)
			fmt.Println(`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thr-timeout","turn":{"id":"turn-timeout","status":"completed"}}}`)
			os.Exit(0)
		}
	}
}

func TestExecutionTimeoutCodexSemanticBudget(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, idle := range []int64{0, 3} {
		local := Config{AgentIdleWatchdog: time.Millisecond, AgentToolWatchdog: time.Millisecond, CodexSemanticInactivityTimeout: 30 * time.Millisecond, CodexFirstTurnNoProgressTimeout: 30 * time.Millisecond}
		p := &agent.ExecutionTimeouts{IdleSeconds: idle, ToolIdleSeconds: idle}
		cfg := executionConfigForTask(local, p)
		backend, err := agent.New("codex", agent.Config{ExecutablePath: exe, LaunchPrefix: []string{"-test.run=^TestExecutionTimeoutCodexProcess$", "--"}, Env: map[string]string{"CLAWSTA143_CODEX_CHILD": "1"}})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		session, err := backend.Execute(ctx, "wait", agent.ExecOptions{Cwd: t.TempDir(), Timeout: 5 * time.Second, SemanticInactivityTimeout: cfg.CodexSemanticInactivityTimeout, FirstTurnNoProgressTimeout: cfg.CodexFirstTurnNoProgressTimeout, DisableSemanticInactivity: idle == 0})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		go func() {
			for range session.Messages {
			}
		}()
		select {
		case result := <-session.Result:
			if result.Status != "completed" {
				t.Errorf("idle=%d result=%+v", idle, result)
			}
		case <-ctx.Done():
			t.Error("Codex fake did not complete")
		}
		cancel()
	}
}

func TestExecutionTimeoutRealSubprocess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		idle, run int64
		wait      int
		status    string
	}{
		{"crosses old local window", 3, 0, 1500, "completed"},
		{"new window terminates", 1, 0, 4000, "idle_watchdog"},
		{"hard limit despite long idle", 5, 1, 4000, "timeout"},
		{"explicit disable overrides local", 0, 0, 1200, "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDaemon(t)
			d.cfg = Config{AgentIdleWatchdog: 100 * time.Millisecond, AgentToolWatchdog: 100 * time.Millisecond, OpenCodeIdleWatchdog: 100 * time.Millisecond, AgentTimeout: 100 * time.Millisecond}
			policy := &agent.ExecutionTimeouts{IdleSeconds: tc.idle, ToolIdleSeconds: tc.idle, RunSeconds: tc.run, OpenCodeIdleSeconds: 0}
			shell := int64(5400)
			policy.OpenCodeShellSeconds = &shell
			run := executionConfigForTask(d.cfg, policy)
			backend, err := agent.New("opencode", agent.Config{ExecutablePath: exe, LaunchPrefix: []string{"-test.run=^TestExecutionTimeoutProcess$", "--"}, Env: map[string]string{"CLAWSTA143_TIMEOUT_CHILD": "1", "CLAWSTA143_CHILD_MS": strconv.Itoa(tc.wait), "OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS": "12"}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			start := time.Now()
			result, _, err := d.executeAndDrain(ctx, backend, "compile", agent.ExecOptions{ExecutionTimeouts: policy, OpenCodeShellTimeout: openCodeShellTimeout(policy, "opencode"), Timeout: run.AgentTimeout, IdleWatchdogTimeout: run.OpenCodeIdleWatchdog, Cwd: t.TempDir()}, slog.Default(), "isolated-timeout", "", new(atomic.Int32))
			if err != nil || result.Status != tc.status {
				t.Fatalf("elapsed=%s result=%+v err=%v", time.Since(start), result, err)
			}
			if tc.status == "completed" && !strings.Contains(result.Output, "shell=5400000") {
				t.Fatalf("subprocess env not applied: %+v", result)
			}
			if d.cfg.OpenCodeIdleWatchdog != 100*time.Millisecond {
				t.Fatal("daemon config changed")
			}
		})
	}
}

type timeoutEventBackend struct {
	tool     bool
	progress bool
}

func (b timeoutEventBackend) Execute(ctx context.Context, _ string, _ agent.ExecOptions) (*agent.Session, error) {
	messages := make(chan agent.Message, 8)
	results := make(chan agent.Result, 1)
	go func() {
		defer close(messages)
		if b.tool {
			messages <- agent.Message{Type: agent.MessageToolUse, Tool: "bash", CallID: "dsh-tool"}
		}
		end := time.NewTimer(1500 * time.Millisecond)
		defer end.Stop()
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				results <- agent.Result{Status: "cancelled"}
				return
			case <-tick.C:
				if b.progress {
					messages <- agent.Message{Type: agent.MessageText, Content: "progress"}
				}
			case <-end.C:
				if b.tool {
					messages <- agent.Message{Type: agent.MessageToolResult, Tool: "bash", CallID: "dsh-tool", Output: "done"}
				}
				results <- agent.Result{Status: "completed"}
				return
			}
		}
	}()
	return &agent.Session{Messages: messages, Result: results}, nil
}
func TestExecutionTimeoutEventsAndRefresh(t *testing.T) {
	d := newTestDaemon(t)
	d.cfg.AgentIdleWatchdog = 50 * time.Millisecond
	for _, b := range []timeoutEventBackend{{tool: true}, {progress: true}} {
		p := &agent.ExecutionTimeouts{IdleSeconds: 1, ToolIdleSeconds: 3}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result, _, err := d.executeAndDrain(ctx, b, "test", agent.ExecOptions{ExecutionTimeouts: p}, slog.Default(), "dsh-events", "", new(atomic.Int32))
		cancel()
		if err != nil || result.Status != "completed" {
			t.Fatalf("event policy %+v: %+v %v", b, result, err)
		}
	}
	// A second claim changes only the new execution; the daemon is not restarted.
	for _, idle := range []int64{3, 1} {
		p := &agent.ExecutionTimeouts{IdleSeconds: idle, ToolIdleSeconds: idle}
		result, _, err := d.executeAndDrain(context.Background(), timeoutEventBackend{}, "test", agent.ExecOptions{ExecutionTimeouts: p}, slog.Default(), "refresh", "", new(atomic.Int32))
		want := "completed"
		if idle == 1 {
			want = "idle_watchdog"
		}
		if err != nil || result.Status != want {
			t.Fatalf("policy %d: %+v %v", idle, result, err)
		}
	}
}

func TestExecutionTimeoutConcurrentRuns(t *testing.T) {
	d := newTestDaemon(t)
	d.cfg.AgentIdleWatchdog = 50 * time.Millisecond
	d.cfg.AgentToolWatchdog = 50 * time.Millisecond
	var wg sync.WaitGroup
	for _, idle := range []int64{1, 3} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := &agent.ExecutionTimeouts{IdleSeconds: idle, ToolIdleSeconds: idle}
			result, _, err := d.executeAndDrain(context.Background(), timeoutEventBackend{}, "concurrent", agent.ExecOptions{ExecutionTimeouts: p}, slog.Default(), fmt.Sprintf("concurrent-%d", idle), "", new(atomic.Int32))
			want := "completed"
			if idle == 1 {
				want = "idle_watchdog"
			}
			if err != nil || result.Status != want {
				t.Errorf("run %d crossed budgets: %+v %v", idle, result, err)
			}
		}()
	}
	wg.Wait()
	if d.cfg.AgentIdleWatchdog != 50*time.Millisecond {
		t.Fatal("concurrent run changed daemon settings")
	}
}
