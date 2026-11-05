package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/managedkit"
)

func TestPendingEngineStartupKeepsAuthenticatedWorkspace(t *testing.T) {
	fx := newBatchFixture(t)
	fx.daemon.cfg.Agents = map[string]AgentEntry{}
	fx.setWorkspaces(WorkspaceInfo{ID: "ws-empty", Name: "pending"})
	if err := fx.daemon.preflightAuth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fx.daemon.trackedWorkspaceCount() != 1 || len(fx.daemon.allRuntimeIDs()) != 0 || fx.registerCallCount() != 0 {
		t.Fatal("pending client invented a runtime or lost its workspace")
	}
	if fx.daemon.connectionStatus().State != "connected" || fx.daemon.executionReadiness().State != "pending_configuration" {
		t.Fatal("connection was coupled to execution")
	}
}

func TestPendingEngineLoadConfigDoesNotRequireBuiltins(t *testing.T) {
	redirectTestHome(t, t.TempDir())
	original := probeAgentCLIs
	probeAgentCLIs = func() map[string]AgentEntry { return map[string]AgentEntry{} }
	t.Cleanup(func() { probeAgentCLIs = original })
	t.Setenv("U_WORKERS_KIT_ROOT", "")
	if _, err := LoadConfig(Overrides{ServerURL: "https://localhost:443", Profile: "pending"}); err != nil {
		t.Fatal(err)
	}
}

func TestEngineReadinessIncludesCustomRuntime(t *testing.T) {
	d := &Daemon{workspaces: map[string]*workspaceState{"ws": {runtimeIDs: []string{"custom"}}}, runtimeIndex: map[string]Runtime{"custom": {ID: "custom", Provider: "dsh", ProfileID: "web"}}}
	if got := d.executionReadiness(); got.State != "ready" || got.RuntimeCount != 1 {
		t.Fatal("custom-only runtime was reported pending", got)
	}
}

func TestPendingEngineStartupRegistersCustomOnlyRuntime(t *testing.T) {
	fx := newBatchFixture(t)
	stubLookPath(t, map[string]string{"company-codex": "/fake/company-codex"})
	fx.daemon.cfg.Agents = map[string]AgentEntry{}
	fx.setWorkspaces(WorkspaceInfo{ID: "ws-1", Name: "custom only"})
	fx.profiles["ws-1"] = []RuntimeProfile{{ID: "custom-1", WorkspaceID: "ws-1", DisplayName: "Company Codex", ProtocolFamily: "codex", CommandName: "company-codex", Enabled: true}}
	if err := fx.daemon.preflightAuth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fx.daemon.executionReadiness(); got.State != "ready" || got.RuntimeCount != 1 {
		t.Fatal("custom-only registration not usable", got)
	}
}

func TestPendingEngineDoesNotClaimUnsupportedRuntime(t *testing.T) {
	redirectTestHome(t, t.TempDir())
	t.Setenv("U_WORKERS_KIT_ROOT", "")
	t.Setenv("OPENCODE_CONFIG", filepath.Join(t.TempDir(), "missing.jsonc"))
	d := &Daemon{workspaces: map[string]*workspaceState{"ws": {runtimeIDs: []string{"builtin", "custom"}}}, runtimeIndex: map[string]Runtime{
		"builtin": {ID: "builtin", Provider: "opencode"}, "custom": {ID: "custom", Provider: "opencode", ProfileID: "wrapper"},
	}}
	ids := d.claimableRuntimeIDs()
	if len(ids) != 1 || ids[0] != "custom" {
		t.Fatal("missing built-in config was claimable or disabled an independent custom wrapper", ids)
	}
}

func TestOpenCodePendingDiagnosticsAndRepair(t *testing.T) {
	home := t.TempDir()
	redirectTestHome(t, home)
	root := t.TempDir()
	t.Setenv("U_WORKERS_KIT_ROOT", root)
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "custom-xdg"))
	self, _ := os.Executable()
	config := filepath.Join(home, "outside kit", "opencode.jsonc")
	selection := managedkit.OpenCodeSelection{Profile: "pending", Executable: self, ConfigPath: config}
	put := func() {
		t.Helper()
		if err := managedkit.WriteJSON(filepath.Join(root, "opencode-selection.json"), map[string]any{"schemaVersion": 1, "selections": []managedkit.OpenCodeSelection{selection}}); err != nil {
			t.Fatal(err)
		}
	}
	put()
	before, _ := os.ReadFile(filepath.Join(root, "opencode-selection.json"))
	d := &Daemon{cfg: Config{Profile: "pending"}}
	if got := d.openCodeDiagnostic(self); got.State != "missing_config" || got.ConfigPath != config || got.ConfigExists {
		t.Fatal(got)
	}
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ raw, state string }{{`{"key":"do-not-print",broken`, "invalid_config"}, {`// JSONC\n{"provider":{},}`, "invalid_config"}, {"// JSONC\n{\"provider\":{},}", "configured"}} {
		if err := os.WriteFile(config, []byte(tc.raw), 0600); err != nil {
			t.Fatal(err)
		}
		got := d.openCodeDiagnostic(self)
		if got.State != tc.state || strings.Contains(got.Message, "do-not-print") {
			t.Fatal(got)
		}
	}
	selection.Executable = filepath.Join(home, "lost.exe")
	put()
	if got := d.openCodeDiagnostic(self); got.State != "missing_program" {
		t.Fatal(got)
	}
	selection.Disabled = true
	put()
	if got := d.openCodeDiagnostic(self); got.State != "disabled" {
		t.Fatal(got)
	}
	selection.Disabled = false
	selection.Executable = self
	put()
	after, _ := os.ReadFile(filepath.Join(root, "opencode-selection.json"))
	if string(before) != string(after) {
		t.Fatal("diagnostics mutated selection")
	}
	t.Setenv("DSH_HOME", filepath.Join(home, "external-dsh"))
	if got := d.dshDiagnostic(); got.ConfigPath != filepath.Join(home, "external-dsh", "profiles", "multica", "package.json") || !strings.Contains(got.Repair, "不是模型配置") {
		t.Fatal(got)
	}
	bridge := filepath.Join(home, "bridge", "dsh-web.json")
	t.Setenv("MULTICA_DSH_WEB_CONFIG", bridge)
	if got := d.dshWebDiagnostic(); got.ConfigPath != bridge || !strings.Contains(got.Repair, "origin 和 accessLog") {
		t.Fatal(got)
	}
}

func TestClientConnectionReceiptAndAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		receipt string
		wantErr bool
	}{
		{"verified no engines", 200, `{"status":"connected","workspace_id":"ws","daemon_id":"machine"}`, false},
		{"invalid identity", 200, `{"status":"connected","workspace_id":"other","daemon_id":"machine"}`, true},
		{"invalid auth", 401, `{}`, true},
		{"server failure", 500, `{}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer synthetic-control" || r.URL.Path != "/api/daemon/client-heartbeat" {
					t.Error("control lost authorization or path")
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["workspace_id"] != "ws" || body["daemon_id"] != "machine" {
					t.Error(body)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.receipt))
			}))
			defer server.Close()
			d := &Daemon{cfg: Config{DaemonID: "machine"}, client: NewClient(server.URL)}
			d.client.client = server.Client()
			d.client.SetToken("synthetic-control")
			err := d.clientConnectionTick(context.Background(), "ws", false)
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
			if !tc.wantErr && d.connectionStatus().State != "connected" {
				t.Fatal("authenticated empty client not connected")
			}
			if tc.wantErr && d.connectionVerified.Load() != 0 {
				t.Fatal("unverified connection credited")
			}
		})
	}
}

func TestClientConnectionExpiresWithoutServerReceipt(t *testing.T) {
	d := &Daemon{cfg: Config{HeartbeatInterval: time.Second}}
	d.connectionVerified.Store(time.Now().Add(-4 * time.Second).UnixNano())
	if d.connectionStatus().State != "disconnected" {
		t.Fatal("stale receipt still connected")
	}
}

func TestDshBridgePathsPreserveTaskCredentials(t *testing.T) {
	t.Setenv("MULTICA_DSH_WEB_CONFIG", filepath.Join(t.TempDir(), "selected.json"))
	t.Setenv("MULTICA_DSH_NODE_PATH", filepath.Join(t.TempDir(), "node.exe"))
	t.Setenv("MULTICA_TOKEN", "synthetic-owner-token")
	env := map[string]string{"MULTICA_TOKEN": "synthetic-task-token"}
	applyDshBridgePaths(env)
	if len(env) != 3 || env["MULTICA_TOKEN"] != "synthetic-task-token" || env["MULTICA_DSH_WEB_CONFIG"] != os.Getenv("MULTICA_DSH_WEB_CONFIG") {
		t.Fatal("bridge references broadened credential inheritance")
	}
}
