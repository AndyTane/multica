package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/managedkit"
)

func TestOpenCodeSelectedJSONCAndMcpPrecedence(t *testing.T) {
	home := t.TempDir()
	redirectTestHome(t, home)
	xdg := filepath.Join(home, "xdg")
	custom := filepath.Join(home, "custom.jsonc")
	dir := filepath.Join(home, "external")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("OPENCODE_CONFIG", custom)
	t.Setenv("OPENCODE_CONFIG_DIR", dir)
	for path, raw := range map[string]string{
		filepath.Join(xdg, "opencode", "opencode.jsonc"): `{"mcp":{"global":{"command":"global"},"shared":{"command":"global"}},}`,
		custom:                               `{ /* retained outside Kit */ "mcp":{"custom":{"command":"custom"},"shared":{"command":"custom"}},}`,
		filepath.Join(dir, "opencode.jsonc"): `{"mcp":{"directory":{"command":"directory"},"shared":{"command":"directory"}}}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	servers, ok, err := loadRuntimeMcpServerConfigs("opencode")
	if err != nil || !ok || len(servers) != 4 {
		t.Fatalf("MCP paths: %v %v", servers, err)
	}
	if servers["shared"].(map[string]any)["command"] != "directory" {
		t.Fatal("custom directory precedence lost")
	}
	summaries, ok, err := listRuntimeLocalMcpServers("opencode")
	if err != nil || !ok || len(summaries) != 4 {
		t.Fatalf("redacted MCP inventory: %v %v", summaries, err)
	}
}

func TestLoadConfigUsesKitOpenCodeSelection(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	redirectTestHome(t, home)
	executable := filepath.Join(root, "opencode.exe")
	config := filepath.Join(home, "outside-kit.jsonc")
	for p, raw := range map[string]string{executable: "fake", config: `{"mcp":{},}`} {
		if err := os.WriteFile(p, []byte(raw), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := managedkit.WriteJSON(filepath.Join(root, "opencode-selection.json"), map[string]any{"schemaVersion": 1, "selections": []managedkit.OpenCodeSelection{{Profile: "office", Executable: executable, ConfigPath: config}}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("U_WORKERS_KIT_ROOT", root)
	t.Setenv("MULTICA_OPENCODE_PATH", "")
	t.Setenv("OPENCODE_CONFIG", "")
	prior := probeAgentCLIs
	probeAgentCLIs = func() map[string]AgentEntry {
		if os.Getenv("MULTICA_OPENCODE_PATH") != executable || os.Getenv("OPENCODE_CONFIG") != config {
			t.Fatal("selection not applied before discovery")
		}
		return map[string]AgentEntry{"opencode": {Path: executable, Command: executable}}
	}
	t.Cleanup(func() { probeAgentCLIs = prior })
	cfg, err := LoadConfig(Overrides{Profile: "office", ServerURL: "https://server.invalid", WorkspacesRoot: t.TempDir()})
	if err != nil || cfg.Agents["opencode"].Path != executable {
		t.Fatalf("daemon selection: %v", err)
	}
	if err := os.WriteFile(config, []byte(`{"private":"DO_NOT_PRINT",broken`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(Overrides{Profile: "office"})
	if err != nil {
		t.Fatal("engine configuration must not terminate the client", err)
	}
	problem := (&Daemon{cfg: cfg}).openCodeDiagnostic(executable)
	if problem.State != "invalid_config" || strings.Contains(problem.Message, "DO_NOT_PRINT") {
		t.Fatalf("invalid selected config should remain pending without content: %+v", problem)
	}
}
