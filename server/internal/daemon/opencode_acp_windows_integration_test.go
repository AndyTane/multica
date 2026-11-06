//go:build windows && agentintegration

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/multica-ai/multica/server/pkg/agent"
	"golang.org/x/sys/windows"
)

type acpSmokeProcess struct {
	PID, Parent uint32
	Name        string
}

// Observe the backend's final channel close so a cancelled drain cannot
// race its remaining teardown against test directory cleanup.
type acpSmokeBackend struct {
	agent.Backend
	ended chan struct{}
}

func (b *acpSmokeBackend) Execute(ctx context.Context, prompt string, opts agent.ExecOptions) (*agent.Session, error) {
	session, err := b.Backend.Execute(ctx, prompt, opts)
	if err != nil {
		close(b.ended)
		return nil, err
	}
	results := make(chan agent.Result, 1)
	original := session.Result
	session.Result = results
	go func() {
		defer close(b.ended)
		defer close(results)
		for result := range original {
			results <- result
		}
	}()
	return session, nil
}

func acpSmokeTree(t *testing.T) []acpSmokeProcess {
	t.Helper()
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	entry := windows.ProcessEntry32{}
	// ProcessEntry32 has platform-dependent padding.
	entry.Size = uint32(unsafe.Sizeof(entry))
	var all []acpSmokeProcess
	for err = windows.Process32First(h, &entry); err == nil; err = windows.Process32Next(h, &entry) {
		all = append(all, acpSmokeProcess{entry.ProcessID, entry.ParentProcessID, windows.UTF16ToString(entry.ExeFile[:])})
	}
	owned := map[uint32]bool{uint32(os.Getpid()): true}
	for changed := true; changed; {
		changed = false
		for _, p := range all {
			if owned[p.Parent] && !owned[p.PID] {
				owned[p.PID] = true
				changed = true
			}
		}
	}
	var tree []acpSmokeProcess
	for _, p := range all {
		if p.PID != uint32(os.Getpid()) && owned[p.PID] {
			tree = append(tree, p)
		}
	}
	return tree
}

func acpSmokeAlive(pid uint32) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	state, err := windows.WaitForSingleObject(h, 0)
	return err == nil && state == uint32(windows.WAIT_TIMEOUT)
}

// Exercise the real backend through the daemon's normal message drain and
// watchdog. Only the HTTP report sink is isolated; no CLI or model is faked.
func TestOpenCodeACPWindowsReal(t *testing.T) {
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("requires explicit real-agent authorization")
	}
	bin := os.Getenv("R2_OPENCODE_EXE")
	model := os.Getenv("R2_OPENCODE_MODEL")
	out := os.Getenv("R2_REAL_EVIDENCE")
	if bin == "" || model == "" || out == "" {
		t.Fatal("set R2_OPENCODE_EXE, R2_OPENCODE_MODEL, R2_REAL_EVIDENCE")
	}
	version, err := exec.Command(bin, "--version").Output()
	if err != nil || strings.TrimSpace(string(version)) != "1.18.3" {
		t.Fatalf("expected real OpenCode 1.18.3: %q %v", version, err)
	}
	if err = os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		seconds int
		cancel  bool
		shell   int64
	}{
		{"silent_420_seconds", 420, false, 600},
		{"cancel_process_tree", 420, true, 600},
		{"ACP_shell_default_4_seconds", 12, false, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cwd := t.TempDir()
			script := fmt.Sprintf(`import os,sys,time,json,subprocess
from pathlib import Path
if len(sys.argv)>1:
    time.sleep(%d)
else:
    child=subprocess.Popen([sys.executable,__file__,"leaf"])
    Path("started.json").write_text(json.dumps({"pid":os.getpid(),"child":child.pid,"shell_ms":os.environ.get("OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS"),"at":time.time()}))
    child.wait()
    Path("finished.txt").write_text("complete")
    print("R2_COMMAND_COMPLETED")
`, tc.seconds)
			if err := os.WriteFile(filepath.Join(cwd, "silent.py"), []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			logFile, err := os.Create(filepath.Join(out, tc.name+".log"))
			if err != nil {
				t.Fatal(err)
			}
			defer logFile.Close()
			logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}))
			var mu sync.Mutex
			var messages []TaskMessageData
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/messages") {
					var payload struct {
						Messages []TaskMessageData `json:"messages"`
					}
					if json.NewDecoder(r.Body).Decode(&payload) == nil {
						mu.Lock()
						messages = append(messages, payload.Messages...)
						mu.Unlock()
					}
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer sink.Close()
			d := &Daemon{client: NewClient(sink.URL), logger: logger, cfg: Config{AgentIdleWatchdog: 3 * time.Minute, OpenCodeIdleWatchdog: 3 * time.Minute, AgentToolWatchdog: 2 * time.Hour}}
			backend, err := agent.New("opencode", agent.Config{ExecutablePath: bin, CLIVersion: "1.18.3", BuiltinRuntime: true, Logger: logger, Env: map[string]string{"OPENCODE_DISABLE_AUTOUPDATE": "true"}})
			if err != nil {
				t.Fatal(err)
			}
			observed := &acpSmokeBackend{Backend: backend, ended: make(chan struct{})}
			ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
			defer cancel()
			prompt := "This is an authorized, isolated Windows process lifecycle test. Execute exactly one bash tool command `C:/Python314/python.exe ./silent.py` with no extra arguments. Do not inspect or change any other files, do not delegate, do not repeat the command, do not use background execution. Set the bash tool timeout to 600000 milliseconds. Wait for its result, then reply with a short completion summary."
			if tc.shell == 4 {
				prompt = "This is an authorized isolated timeout test. Execute exactly one bash tool command `C:/Python314/python.exe ./silent.py` with no extra arguments. IMPORTANT: omit the timeout argument completely so the configured runtime default is used. Do not repeat, background, inspect or change files, or delegate. After the tool returns, reply with a short summary."
			}
			policy := &agent.ExecutionTimeouts{IdleSeconds: 180, ToolIdleSeconds: 7200, OpenCodeIdleSeconds: 180, OpenCodeShellSeconds: &tc.shell}
			type runResult struct {
				result agent.Result
				err    error
			}
			done := make(chan runResult, 1)
			go func() {
				r, _, e := d.executeAndDrain(ctx, observed, prompt, agent.ExecOptions{Cwd: cwd, Model: model, Timeout: 17 * time.Minute, IdleWatchdogTimeout: 3 * time.Minute, ExecutionTimeouts: policy, OpenCodeShellTimeout: time.Duration(tc.shell) * time.Second}, logger, "r2-real-isolated", "", new(atomic.Int32))
				done <- runResult{r, e}
			}()
			var tree []acpSmokeProcess
			owned := map[uint32]acpSmokeProcess{}
			var started map[string]any
			deadline := time.NewTimer(6 * time.Minute)
			defer deadline.Stop()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			var result runResult
			waiting := true
			for waiting {
				select {
				case result = <-done:
					waiting = false
				case <-deadline.C:
					cancel()
					result = <-done
					waiting = false
				case <-ticker.C:
					for _, p := range acpSmokeTree(t) {
						owned[p.PID] = p
					}
					if started == nil {
						if data, e := os.ReadFile(filepath.Join(cwd, "started.json")); e == nil && json.Unmarshal(data, &started) == nil {
							deadline.Stop()
							for _, p := range acpSmokeTree(t) {
								owned[p.PID] = p
							}
							if tc.cancel {
								cancel()
							}
						}
					}
				}
			}
			cancel()
			select {
			case <-observed.ended:
			case <-time.After(45 * time.Second):
				t.Error("backend cleanup did not complete")
			}
			for _, p := range owned {
				tree = append(tree, p)
			}
			// All of these PIDs belonged to this test's descendant tree.
			var remaining []acpSmokeProcess
			for until := time.Now().Add(6 * time.Second); ; {
				remaining = nil
				for _, p := range tree {
					if acpSmokeAlive(p.PID) {
						remaining = append(remaining, p)
					}
				}
				if len(remaining) == 0 || time.Now().After(until) {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			mu.Lock()
			captured := append([]TaskMessageData(nil), messages...)
			mu.Unlock()
			proof := map[string]any{"version": "1.18.3", "model": model, "started": started, "processes_during": tree, "processes_remaining": remaining, "result": result.result, "messages": captured, "command_start_window_seconds": 360, "idle_watchdog_seconds": 180, "tool_watchdog_seconds": 7200}
			data, _ := json.MarshalIndent(proof, "", "  ")
			if err := os.WriteFile(filepath.Join(out, tc.name+".json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			logFile.Sync()
			logs, _ := os.ReadFile(logFile.Name())
			if !strings.Contains(string(logs), "opencode acp started") {
				t.Error("real process did not use ACP")
			}
			if result.err != nil {
				t.Errorf("daemon drain: %v", result.err)
			}
			if started == nil {
				t.Fatal("real command never started")
			}
			if started["shell_ms"] != fmt.Sprint(tc.shell*1000) {
				t.Errorf("child shell env = %v", started["shell_ms"])
			}
			if len(remaining) > 0 {
				t.Errorf("remaining owned processes: %+v", remaining)
			}
			var use, finish *TaskMessageData
			for i := range captured {
				m := &captured[i]
				if m.Type == "tool_use" && m.Tool == "bash" {
					use = m
				}
				if m.Type == "tool_result" && m.Tool == "bash" {
					finish = m
				}
			}
			if use == nil {
				t.Fatal("no bash tool_use start reached daemon")
			}
			if tc.cancel {
				if result.result.Status != "cancelled" && result.result.Status != "aborted" {
					t.Errorf("cancel status=%s", result.result.Status)
				}
			} else {
				if result.result.Status != "completed" {
					t.Errorf("status=%s error=%s", result.result.Status, result.result.Error)
				}
				if finish == nil {
					t.Fatal("no bash tool_result")
				}
				elapsed := finish.CreatedAt.Sub(use.CreatedAt)
				t.Logf("actual tool duration=%s, idle watchdog=3m, tool watchdog=2h", elapsed)
				if tc.shell == 4 {
					if _, ok := use.Input["timeout"]; ok {
						t.Error("model supplied an explicit timeout")
					}
					if elapsed < 3*time.Second || elapsed > 10*time.Second {
						t.Errorf("4s default not observed: %s", elapsed)
					}
					if _, err := os.Stat(filepath.Join(cwd, "finished.txt")); err == nil {
						t.Error("12s command finished despite 4s default")
					}
				} else if elapsed < 415*time.Second {
					t.Errorf("silent command did not run for 7m: %s", elapsed)
				} else if _, err := os.Stat(filepath.Join(cwd, "finished.txt")); err != nil || !strings.Contains(finish.Output, "R2_COMMAND_COMPLETED") {
					t.Error("silent command did not finish normally")
				}
			}
		})
	}
}
