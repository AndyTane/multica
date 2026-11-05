//go:build tasktrustintegration

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/brand"
	"github.com/multica-ai/multica/server/internal/daemon/execenv"
	"github.com/multica-ai/multica/server/internal/tlsx"
	"github.com/multica-ai/multica/server/pkg/agent"
)

const taskTrustIssueID = "11111111-1111-4111-8111-111111111111"
const taskTrustComment = "Task trust 验证通过"

// Capture only in the fake provider process, before this package's TestMain
// clears task identity and redirects HOME for ordinary unit tests.
var taskTrustProviderEnvironment = func() []string {
	if os.Getenv("TASK_TRUST_PROVIDER_HELPER") == "1" {
		return os.Environ()
	}
	return nil
}()

type taskTrustCLIResult struct {
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

// This is a fake provider, executed through the real OpenCode backend so its
// production environment filtering runs. It launches only our freshly built
// product CLI, never OpenCode or a model. Non-product helper keys are test-only.
func TestTaskTrustCLIProviderHelper(t *testing.T) {
	if os.Getenv("TASK_TRUST_PROVIDER_HELPER") != "1" {
		return
	}
	mode := os.Getenv("TASK_TRUST_MODE")
	childEnv := make([]string, 0)
	for _, entry := range taskTrustProviderEnvironment {
		key, _, _ := strings.Cut(entry, "=")
		if key == "U_WORKERS_OWNER_SENTINEL" || key == "MULTICA_OWNER_SENTINEL" {
			t.Fatal("owner product environment reached task provider")
		}
		if (mode == "drop-ca" || mode == "drop-identity") && (key == tlsx.RootCAEnv || key == "MULTICA_ROOT_CA_FILE") {
			continue
		}
		if mode == "drop-identity" && brand.IsProductEnvKey(key) {
			continue
		}
		if mode == "drop-token" && (key == "MULTICA_TOKEN" || key == "U_WORKERS_TOKEN") {
			continue
		}
		childEnv = append(childEnv, entry)
	}
	args := []string{"issue", "comment", "add", taskTrustIssueID, "--content-file", os.Getenv("TASK_TRUST_BODY"), "--output", "json"}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if shell := os.Getenv("TASK_TRUST_SHELL"); shell != "" {
		cmd = exec.CommandContext(ctx, shell, "-NoProfile", "-NonInteractive", "-Command",
			"& $env:TASK_TRUST_CLI issue comment add "+taskTrustIssueID+" --content-file $env:TASK_TRUST_BODY --output json; exit $LASTEXITCODE")
	} else {
		cmd = exec.CommandContext(ctx, os.Getenv("TASK_TRUST_CLI"), args...)
	}
	cmd.Env = childEnv
	out, err := cmd.CombinedOutput()
	report := taskTrustCLIResult{Output: string(out)}
	if err != nil {
		report.ExitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			report.ExitCode = exitErr.ExitCode()
		}
	}
	data, _ := json.Marshal(report)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "text", "part": map[string]string{"text": string(data)}})
}

func TestTaskClientTrustCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	binDir := t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	current := filepath.Join(binDir, "u-workers"+suffix)
	build := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", current, "../../cmd/multica")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build isolated CLI: %v\n%s", err, out)
	}
	bin, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(binDir, "multica"+suffix)
	if err := os.WriteFile(legacy, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	var writes atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mat_synthetic_task_trust" || r.Header.Get("X-Task-ID") != "synthetic-task" ||
			r.Header.Get("X-Agent-ID") != "synthetic-agent" || r.Header.Get("X-Workspace-ID") != "synthetic-workspace" {
			t.Error("request did not use the synthetic task identity")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/issues/"+taskTrustIssueID+"/comments":
			var body struct {
				Content string `json:"content"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Content != taskTrustComment {
				t.Error("UTF-8 comment body did not survive the subprocess")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writes.Add(1)
			fmt.Fprint(w, `{"id":"22222222-2222-4222-8222-222222222222","content":"accepted"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+taskTrustIssueID:
			fmt.Fprintf(w, `{"id":%q,"identifier":"TEST-1"}`, taskTrustIssueID)
		default:
			t.Errorf("unexpected fixture request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	server, ca := newTaskTrustServer(t, handler, "valid")
	plainServer := httptest.NewServer(handler)
	t.Cleanup(plainServer.Close)
	_, wrongCA := newTaskTrustServer(t, nil, "valid")
	wrongHost, wrongHostCA := newTaskTrustServer(t, handler, "hostname-mismatch")
	expired, expiredCA := newTaskTrustServer(t, handler, "expired")
	badCA := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(badCA, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	// All parent context is synthetic. The provider must strip these current
	// and legacy values, then use only daemon-assembled task context.
	t.Setenv("MULTICA_TOKEN", "mul_synthetic_owner")
	t.Setenv("U_WORKERS_TOKEN", "mul_synthetic_owner")
	t.Setenv("U_WORKERS_ROOT_CA_FILE", wrongCA)
	t.Setenv("MULTICA_ROOT_CA_FILE", wrongCA)
	t.Setenv("U_WORKERS_OWNER_SENTINEL", "must-be-filtered")
	t.Setenv("MULTICA_OWNER_SENTINEL", "must-be-filtered")
	testExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, configCA, envCA, legacyCA, url, mode, shell, wantError string
		currentNames, wantOK                                         bool
	}{
		{name: "explicit-current-CA", configCA: ca, envCA: ca, wantOK: true},
		{name: "legacy-CA-and-context-aliases", configCA: ca, legacyCA: ca, wantOK: true},
		{name: "current-context-aliases", configCA: ca, envCA: ca, currentNames: true, wantOK: true},
		{name: "current-CA-wins-over-legacy", configCA: ca, envCA: ca, legacyCA: wrongCA, wantOK: true},
		{name: "task-profile-after-shell-drops-CA", configCA: ca, envCA: ca, mode: "drop-ca", wantOK: true},
		{name: "explicit-env-over-task-profile", configCA: wrongCA, envCA: ca, wantOK: true},
		{name: "wrong-env-does-not-fall-back-to-profile", configCA: ca, envCA: wrongCA, wantError: "certificate"},
		{name: "no-extra-CA", wantError: "certificate"},
		{name: "ordinary-HTTP-without-extra-CA", url: plainServer.URL, wantOK: true},
		{name: "unrelated-task-profile-CA", configCA: wrongCA, mode: "drop-ca", wantError: "certificate"},
		{name: "malformed-env-CA", configCA: ca, envCA: badCA, wantError: "no certificate"},
		{name: "missing-env-CA", configCA: ca, envCA: badCA + ".missing", wantError: "read root CA"},
		{name: "hostname-verification", configCA: wrongHostCA, envCA: wrongHostCA, url: wrongHost.URL, wantError: "certificate"},
		{name: "expiry-verification", configCA: expiredCA, envCA: expiredCA, url: expired.URL, wantError: "certificate"},
		{name: "task-token-lost", configCA: ca, envCA: ca, mode: "drop-token", wantError: "task-scoped"},
		{name: "all-task-env-lost", configCA: ca, envCA: ca, mode: "drop-identity", wantError: "task-scoped"},
	}
	if runtime.GOOS == "windows" {
		for _, shell := range []string{"powershell.exe", "pwsh.exe"} {
			if path, err := exec.LookPath(shell); err == nil {
				tests = append(tests, struct {
					name, configCA, envCA, legacyCA, url, mode, shell, wantError string
					currentNames, wantOK                                         bool
				}{name: shell + "-CA-from-task-config", configCA: ca, envCA: ca, mode: "drop-ca", shell: path, wantOK: true})
			} else {
				t.Fatalf("required Windows verification shell missing: %s", shell)
			}
		}
	}
	for _, alias := range []string{current, legacy} {
		for _, tc := range tests {
			t.Run(filepath.Base(alias)+"/"+tc.name, func(t *testing.T) {
				work := t.TempDir()
				marker := filepath.Join(work, execenv.TaskContextMarkerRelPath)
				if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(marker, []byte(`{"managed_by":"multica-daemon-task"}`), 0o600); err != nil {
					t.Fatal(err)
				}
				bodyPath := filepath.Join(work, "comment.txt")
				if err := os.WriteFile(bodyPath, []byte(taskTrustComment), 0o600); err != nil {
					t.Fatal(err)
				}
				// Seed a synthetic owner profile with a usable CA and member token.
				// Missing task identity must not use either. Check bytes after run.
				ownerHome := t.TempDir()
				ownerDir := filepath.Join(ownerHome, ".multica")
				if err := os.MkdirAll(ownerDir, 0o700); err != nil {
					t.Fatal(err)
				}
				ownerData, _ := json.Marshal(map[string]string{"root_ca_file": ca, "token": "mul_synthetic_owner", "server_url": server.URL})
				ownerPath := filepath.Join(ownerDir, "config.json")
				if err := os.WriteFile(ownerPath, ownerData, 0o600); err != nil {
					t.Fatal(err)
				}
				root := t.TempDir()
				if _, err := prepareTaskClientTrust(root, tc.configCA); err != nil {
					t.Fatal(err)
				}
				url := tc.url
				if url == "" {
					url = server.URL
				}
				env := taskMulticaEnvironment(Task{ID: "synthetic-task", AgentID: "synthetic-agent", WorkspaceID: "synthetic-workspace"}, "Synthetic", "mat_synthetic_task_trust", root, t.TempDir(), url, tc.envCA, 19514, 1, t.TempDir())
				if tc.legacyCA != "" {
					env["MULTICA_ROOT_CA_FILE"] = tc.legacyCA
				}
				if tc.currentNames {
					for key, value := range env {
						if currentKey, ok := brand.EnvName(key); ok {
							delete(env, key)
							env[currentKey] = value
						}
					}
				}
				env["HOME"], env["USERPROFILE"] = ownerHome, ownerHome
				env["TASK_TRUST_PROVIDER_HELPER"], env["TASK_TRUST_CLI"] = "1", alias
				env["TASK_TRUST_BODY"], env["TASK_TRUST_MODE"], env["TASK_TRUST_SHELL"] = bodyPath, tc.mode, tc.shell
				backend, err := agent.New("opencode", agent.Config{ExecutablePath: testExe, LaunchPrefix: []string{"-test.run=^TestTaskTrustCLIProviderHelper$", "--"}, Env: env, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
				if err != nil {
					t.Fatal(err)
				}
				before := writes.Load()
				session, err := backend.Execute(ctx, "synthetic CLI transport check; no model", agent.ExecOptions{Cwd: work, Timeout: 30 * time.Second})
				if err != nil {
					t.Fatal(err)
				}
				for range session.Messages {
				}
				result := <-session.Result
				if result.Status != "completed" {
					t.Fatalf("fake provider failed: %s %s", result.Status, result.Error)
				}
				var report taskTrustCLIResult
				if err := json.Unmarshal([]byte(result.Output), &report); err != nil {
					t.Fatalf("decode CLI report: %v: %s", err, result.Output)
				}
				if tc.wantOK {
					if report.ExitCode != 0 || writes.Load() != before+1 {
						t.Fatalf("HTTPS write failed: exit=%d, output=%s", report.ExitCode, report.Output)
					}
				} else if report.ExitCode == 0 || writes.Load() != before || !strings.Contains(strings.ToLower(report.Output), strings.ToLower(tc.wantError)) {
					t.Fatalf("expected closed failure containing %q, exit=%d output=%s", tc.wantError, report.ExitCode, report.Output)
				}
				if strings.Contains(report.Output, "mul_synthetic_owner") {
					t.Fatal("owner credential reached CLI output")
				}
				got, err := os.ReadFile(ownerPath)
				if err != nil || string(got) != string(ownerData) {
					t.Fatal("synthetic owner config changed")
				}
			})
		}
	}
}
