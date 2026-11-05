package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/daemon"
	"github.com/multica-ai/multica/server/internal/managedkit"
	"github.com/multica-ai/multica/server/internal/middleware"
)

// Explicit opt-in uses real built CLI processes, a disposable database and
// signed JWT authentication. The baseline engine is probed for --version only;
// no prompt, model call, installed profile or existing service is touched.
func TestPendingClientManagedProcessIntegration(t *testing.T) {
	first, second := os.Getenv("UWORKERS_TEST_CLI_FIRST"), os.Getenv("UWORKERS_TEST_CLI_SECOND")
	baseline, engine := os.Getenv("UWORKERS_TEST_CLI_BASELINE"), os.Getenv("UWORKERS_TEST_BASELINE_ENGINE")
	if first == "" || second == "" || baseline == "" || engine == "" {
		t.Skip("set UWORKERS_TEST_CLI_FIRST/SECOND/BASELINE and UWORKERS_TEST_BASELINE_ENGINE for isolated Windows processes")
	}
	if runtime.GOOS != "windows" || testHandler == nil {
		t.Fatal("Windows and an isolated migrated database are required")
	}
	root := t.TempDir()
	kit := filepath.Join(root, "kit")
	home := filepath.Join(root, "user")
	_ = os.MkdirAll(kit, 0700)
	profile := "r61-" + uuid.NewString()[:8]
	port := 19515
	for _, b := range []byte(profile) {
		port += int(b)
	}
	port = 19515 + (port-19515)%1000
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal("isolated profile port unavailable", err)
	}
	_ = ln.Close()
	cache := filepath.Join(root, "cache")
	t.Setenv("UWORKERS_CLIENT_CACHE", cache)
	oldVersion, oldCommit := testHandler.cfg.ServerVersion, testHandler.cfg.ServerCommit
	defer func() { testHandler.cfg.ServerVersion = oldVersion; testHandler.cfg.ServerCommit = oldCommit }()
	var gate sync.RWMutex
	r := chi.NewRouter()
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	r.Route("/api/daemon", func(r chi.Router) {
		r.Use(middleware.DaemonAuth(testHandler.Queries, nil, nil, nil))
		r.Get("/workspaces", testHandler.ListDaemonWorkspaces)
		r.Post("/register", testHandler.DaemonRegister)
		r.Post("/deregister", testHandler.DaemonDeregister)
		r.Post("/heartbeat", testHandler.DaemonHeartbeat)
		r.Post("/client-heartbeat", testHandler.DaemonClientHeartbeat)
		r.Get("/client-package", testHandler.DaemonClientPackage)
		r.Get("/workspaces/{workspaceId}/runtime-profiles", testHandler.DaemonListRuntimeProfiles)
		r.Get("/workspaces/{workspaceId}/repos", testHandler.GetDaemonWorkspaceRepos)
		r.Post("/tasks/claim", testHandler.ClaimTasksByRuntime)
		r.Post("/runtimes/{runtimeId}/recover-orphans", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) })
	})
	r.With(middleware.DaemonAuth(testHandler.Queries, nil, nil, nil)).Get("/api/workspaces", testHandler.ListWorkspaces)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
		gate.RLock()
		defer gate.RUnlock()
		if rq.Header.Get("X-Client-Version") == "v9.61.3" {
			writeError(w, 401, "injected authentication rejection")
			return
		}
		r.ServeHTTP(w, rq)
	}))
	defer server.Close()
	ca := filepath.Join(kit, "root-ca.crt")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": testUserID, "email": handlerTestEmail, "exp": time.Now().Add(time.Hour).Unix()}).SignedString(auth.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"server_url": server.URL, "root_ca_file": ca, "workspace_id": testWorkspaceID, "token": token, "device_name": "isolated pending client"}
	configPath := filepath.Join(home, ".u-workers", "profiles", profile, "config.json")
	if err := managedkit.WriteJSON(configPath, config); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(configPath)
	installation := managedkit.Installation{ID: uuid.NewString(), Profile: profile, ServerURL: server.URL}
	_ = managedkit.WriteJSON(filepath.Join(kit, "kit-install.json"), installation)
	_ = managedkit.WriteJSON(filepath.Join(kit, "connection.json"), map[string]string{"profile": profile, "serverUrl": server.URL, "rootCa": ca})
	copyFile := func(from, to string) {
		t.Helper()
		b, e := os.ReadFile(from)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(to, b, 0700); e != nil {
			t.Fatal(e)
		}
	}
	copyFile(baseline, filepath.Join(kit, "u-workers.exe"))
	var env []string
	for _, entry := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if strings.HasPrefix(key, "MULTICA_") || strings.HasPrefix(key, "U_WORKERS_") || strings.HasPrefix(key, "OPENCODE_") || key == "USERPROFILE" || key == "HOME" || key == "PATH" || key == "XDG_CONFIG_HOME" || key == "DSH_HOME" {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "USERPROFILE="+home, "HOME="+home, "PATH="+filepath.Join(os.Getenv("SystemRoot"), "System32"), "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "DSH_HOME="+filepath.Join(home, ".dsh"), "U_WORKERS_KIT_ROOT="+kit, "MULTICA_OPENCODE_PATH="+engine, "MULTICA_DAEMON_HEARTBEAT_INTERVAL=300ms", "MULTICA_DAEMON_POLL_INTERVAL=200ms", "MULTICA_WORKSPACES_ROOT="+filepath.Join(root, "workspaces"), "MULTICA_GC_ENABLED=false")
	// Disable all other probes explicitly; a developer's installed agents must
	// never turn this no-engine regression into a successful execution runtime.
	for _, name := range []string{"CLAUDE", "CODEX", "CODEBUDDY", "CODEARTS", "COPILOT", "DEVECO", "OPENCLAW", "HERMES", "PI", "OMP", "CURSOR", "KIMI", "REASONIX", "DSH", "KIRO", "AGY", "QODER", "QODERCLICN", "TRAECLI", "GROK", "QWEN", "QWENPAW", "MCODE", "DIM", "ZEROCLAW"} {
		env = append(env, "MULTICA_"+name+"_PATH="+filepath.Join(root, "missing-"+name+".exe"))
	}
	run := func(binary string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append([]string{"--profile", profile}, args...)...)
		cmd.Env = env
		cmd.Dir = kit
		return cmd.CombinedOutput()
	}
	health := func() daemon.HealthResponse {
		var h daemon.HealthResponse
		c := http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
		resp, e := c.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if e == nil {
			defer resp.Body.Close()
			_ = json.NewDecoder(resp.Body).Decode(&h)
		}
		return h
	}
	t.Cleanup(func() {
		_, _ = run(first, "daemon", "stop")
		deadline := time.Now().Add(15 * time.Second)
		for health().PID > 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if health().PID > 0 {
			t.Error("test daemon remained after cleanup")
		}
	})
	evidence := os.Getenv("UWORKERS_TEST_EVIDENCE")
	record := func(name string, value any) {
		t.Helper()
		if evidence != "" {
			if err := managedkit.WriteJSON(filepath.Join(evidence, name+".json"), value); err != nil {
				t.Fatal(err)
			}
		}
	}
	activate := func(binary, version string) {
		t.Helper()
		files := map[string][]byte{}
		for _, name := range []string{"Connect.cmd", "Reconnect.cmd", "Status.cmd", "Stop.cmd", "Import.cmd", "client.ps1", "client-settings.ps1", "README.md"} {
			b, e := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "client-kit", name))
			if e != nil {
				t.Fatal(e)
			}
			files[name] = b
		}
		files["connection.json"] = []byte(`{"profile":"","serverUrl":"","rootCa":""}`)
		files["u-workers.exe"], err = os.ReadFile(binary)
		if err != nil {
			t.Fatal(err)
		}
		files["multica.exe"] = files["u-workers.exe"]
		inv := managedkit.Inventory{SchemaVersion: 2, Version: version, Commit: strings.Repeat("6", 40), Platform: "windows-amd64"}
		for name, data := range files {
			sum := sha256.Sum256(data)
			inv.Files = append(inv.Files, managedkit.File{Path: name, Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
		}
		files["CLIENT-FILES.json"], _ = json.Marshal(inv)
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		for name, data := range files {
			w, e := z.Create(name)
			if e != nil {
				t.Fatal(e)
			}
			_, _ = w.Write(data)
		}
		if e := z.Close(); e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(buf.Bytes())
		offer := managedkit.Offer{Version: version, Commit: inv.Commit, Platform: inv.Platform, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(buf.Len())}
		dir := filepath.Join(cache, version)
		_ = os.MkdirAll(dir, 0700)
		_ = os.WriteFile(filepath.Join(dir, "client-kit.zip"), buf.Bytes(), 0600)
		_ = managedkit.WriteJSON(filepath.Join(dir, "ready.json"), offer)
		gate.Lock()
		testHandler.cfg.ServerVersion = version
		testHandler.cfg.ServerCommit = inv.Commit
		gate.Unlock()
		record("offer-"+version, offer)
	}
	wait := func(version, phase string, target ...string) {
		t.Helper()
		targetVersion := version
		if len(target) > 0 {
			targetVersion = target[0]
		}
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			j := managedkit.LoadJournal(kit)
			h := health()
			if j.Offer.Version == targetVersion && j.Phase == phase && h.Status == "running" && h.CLIVersion == version && h.Connection.State == "connected" && h.Execution.State == "pending_configuration" {
				record(phase+"-"+version+"-target-"+targetVersion, map[string]any{"health": h, "journal": j, "program": managedkit.CurrentProgram(kit)})
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		j := managedkit.LoadJournal(kit)
		t.Fatalf("switch did not finish: phase=%s reason=%s health=%+v", j.Phase, j.Reason, health())
	}
	activate(first, "v0.2.13")
	if out, e := run(filepath.Join(kit, "u-workers.exe"), "daemon", "start"); e != nil {
		t.Fatalf("baseline start failed: %v %s", e, out)
	}
	wait("v0.2.13", "completed")
	showStatus := func(label string, edition string) {
		t.Helper()
		program, _ := managedkit.ProgramPath(kit, managedkit.CurrentProgram(kit))
		cmd := exec.Command(edition, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(program, "client.ps1"), "-DataDirectory", kit, "-Action", "Status", "-NonInteractive")
		cmd.Env = env
		cmd.Dir = kit
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("Kit status: %v %s", e, out)
		}
		if !bytes.Contains(out, []byte("待配置，暂不能接任务")) && !bytes.Contains(out, []byte("执行环境已就绪")) {
			t.Fatalf("missing Chinese readiness: %s", out)
		}
		if evidence != "" {
			_ = os.WriteFile(filepath.Join(evidence, label+".txt"), out, 0600)
		}
	}
	ps51 := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	showStatus("missing-config-connected-ps51", ps51)
	if ps7, e := exec.LookPath("pwsh.exe"); e == nil {
		showStatus("missing-config-connected-ps7", ps7)
	}
	if out, e := run(first, "daemon", "stop"); e != nil {
		t.Fatalf("stop: %v %s", e, out)
	}
	program, _ := managedkit.ProgramPath(kit, managedkit.CurrentProgram(kit))
	if out, e := run(filepath.Join(program, "u-workers.exe"), "daemon", "start"); e != nil {
		t.Fatalf("reconnect: %v %s", e, out)
	}
	wait("v0.2.13", "completed")
	activate(second, "v9.61.2")
	wait("v9.61.2", "completed")
	after, _ := os.ReadFile(configPath)
	var beforeConfig, afterConfig map[string]any
	_ = json.Unmarshal(before, &beforeConfig)
	_ = json.Unmarshal(after, &afterConfig)
	for _, key := range []string{"token", "server_url", "workspace_id", "root_ca_file"} {
		if beforeConfig[key] != afterConfig[key] {
			t.Fatal("original profile changed", key)
		}
	}
	if got := health(); got.ActiveTaskCount != 0 || got.Execution.RuntimeCount != 0 {
		t.Fatal("pending client received execution capacity", got)
	}
	if out, e := run(second, "daemon", "status", "--output", "json"); e != nil {
		t.Fatalf("structured status: %v %s", e, out)
	} else {
		record("daemon-status", json.RawMessage(out))
	}
	// An invalid new process is a genuine failure: its bytes and manifest are
	// offered normally; no journal, acknowledgement or active pointer is edited.
	authRollback := false
	if third := os.Getenv("UWORKERS_TEST_CLI_THIRD"); third != "" {
		activate(third, "v9.61.3")
		wait("v9.61.2", "rolled_back", "v9.61.3")
		authRollback = true
	}
	bad := filepath.Join(root, "not-a-program.exe")
	_ = os.WriteFile(bad, []byte("not an executable"), 0700)
	activate(bad, "v9.61.4")
	wait("v9.61.2", "rolled_back", "v9.61.4")
	runKit := func(args ...string) {
		t.Helper()
		program, _ := managedkit.ProgramPath(kit, managedkit.CurrentProgram(kit))
		base := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(program, "client.ps1"), "-DataDirectory", kit, "-NonInteractive"}
		cmd := exec.Command(ps51, append(base, args...)...)
		cmd.Env = env
		cmd.Dir = kit
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("Kit operation: %v %s", e, out)
		}
	}
	runKit("-Action", "Stop")
	runKit("-Action", "Reconnect", "-OpenCodePath", filepath.Join(root, "missing.exe"))
	if health().Execution.State != "pending_configuration" {
		t.Fatal("missing program failed to keep connection")
	}
	showStatus("missing-program-connected-ps51", ps51)
	if readyConfig := os.Getenv("UWORKERS_TEST_READY_CONFIG"); readyConfig != "" {
		original, e := os.ReadFile(readyConfig)
		if e != nil {
			t.Fatal(e)
		}
		runKit("-Action", "Stop")
		runKit("-Action", "Reconnect", "-OpenCodePath", engine, "-OpenCodeConfig", readyConfig)
		if h := health(); h.Execution.State != "ready" || h.Execution.RuntimeCount != 1 {
			t.Fatal("one configured engine did not restore capacity", h)
		}
		showStatus("configured-ready-ps51", ps51)
		if ps7, e := exec.LookPath("pwsh.exe"); e == nil {
			showStatus("configured-ready-ps7", ps7)
		}
		after, e := os.ReadFile(readyConfig)
		if e != nil || !bytes.Equal(original, after) {
			t.Fatal("external configuration changed")
		}
		sum := sha256.Sum256(after)
		record("configured-readiness", map[string]any{"configuration_sha256": hex.EncodeToString(sum[:]), "original_file_unchanged": true, "registered_engines": 1, "model_called": false})
	}
	record("result", map[string]any{"baseline": "v0.2.11", "first": "v0.2.13", "second": "v9.61.2", "stop_reconnect": true, "zero_runtimes": true, "real_jwt_and_database": true, "profile_preserved": true, "crash_rollback": true, "authentication_rollback": authRollback, "health_port": strconv.Itoa(port)})
}
