package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/brand"
	"github.com/multica-ai/multica/server/internal/envcompat"
)

// TestCLIConfig_BackwardCompat_OldFileLoadsWithNilBackends verifies that a
// config.json written by an older daemon (no `backends` key at all) loads
// correctly into the new schema, with Backends == nil. This is the most
// important guarantee of issue #3875's PR: existing on-disk configs MUST
// continue to work byte-for-byte.
func TestCLIConfig_BackwardCompat_OldFileLoadsWithNilBackends(t *testing.T) {
	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	// Write a 4-field config exactly as the historical daemon would have.
	cfgDir := filepath.Join(tmp, ".multica")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	historical := `{
  "server_url": "https://api.multica.ai",
  "app_url": "https://multica.ai",
  "workspace_id": "ws-123",
  "token": "mul_abcdef"
}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(historical), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig on historical file: %v", err)
	}

	if cfg.ServerURL != "https://api.multica.ai" {
		t.Errorf("ServerURL: got %q, want historical value", cfg.ServerURL)
	}
	if cfg.Token != "mul_abcdef" {
		t.Errorf("Token: got %q, want historical value", cfg.Token)
	}
	if cfg.Backends != nil {
		t.Errorf("Backends should be nil for historical config, got %+v", cfg.Backends)
	}
}

// TestCLIConfig_BackwardCompat_NilBackendsOmittedFromJSON verifies that
// saving a config without backend overrides does NOT add a `backends` key
// to the on-disk JSON. This matters for users who never set overrides —
// their config files must stay byte-identical, so a future downgrade to
// an older daemon doesn't trip on an empty `backends: null` line.
func TestCLIConfig_BackwardCompat_NilBackendsOmittedFromJSON(t *testing.T) {
	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	cfg := CLIConfig{
		ServerURL: "https://api.multica.ai",
		Token:     "mul_xyz",
	}
	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatal(err)
	}

	configPath, err := CLIConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("config file is empty")
	}

	// The omitempty tag on Backends should keep it out of the JSON entirely.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal saved config: %v", err)
	}
	if _, ok := raw["backends"]; ok {
		t.Errorf("backends key should be omitted when nil, got: %s", string(data))
	}
}

// TestCLIConfig_OpenClawOverride_RoundTrip verifies that setting BinaryPath
// and StateDir survives a save/load cycle.
func TestCLIConfig_OpenClawOverride_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	original := CLIConfig{
		ServerURL: "https://api.multica.ai",
		Token:     "mul_xyz",
		Backends: &BackendOverrides{
			OpenClaw: &OpenClawOverride{
				BinaryPath: "/opt/openclaw-prod/bin/openclaw",
				StateDir:   "/var/lib/openclaw-prod",
			},
		},
	}
	if err := SaveCLIConfig(original); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCLIConfig()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Backends == nil || loaded.Backends.OpenClaw == nil {
		t.Fatalf("Backends.OpenClaw should be non-nil after round-trip, got %+v", loaded.Backends)
	}
	if loaded.Backends.OpenClaw.BinaryPath != original.Backends.OpenClaw.BinaryPath {
		t.Errorf("BinaryPath round-trip: got %q, want %q",
			loaded.Backends.OpenClaw.BinaryPath, original.Backends.OpenClaw.BinaryPath)
	}
	if loaded.Backends.OpenClaw.StateDir != original.Backends.OpenClaw.StateDir {
		t.Errorf("StateDir round-trip: got %q, want %q",
			loaded.Backends.OpenClaw.StateDir, original.Backends.OpenClaw.StateDir)
	}
}

// TestCLIConfig_OpenClawOverride_PartialFieldsOmitted verifies that an
// override with only one field set does not emit empty strings for the
// unset field. Important so users can intentionally set only BinaryPath
// (or only StateDir) and have the other follow the historical default,
// without an empty string overriding env-var precedence.
func TestCLIConfig_OpenClawOverride_PartialFieldsOmitted(t *testing.T) {
	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	cfg := CLIConfig{
		ServerURL: "https://api.multica.ai",
		Token:     "mul_xyz",
		Backends: &BackendOverrides{
			OpenClaw: &OpenClawOverride{
				StateDir: "/var/lib/openclaw-prod",
				// BinaryPath intentionally left empty
			},
		},
	}
	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatal(err)
	}

	configPath, err := CLIConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	openclaw, ok := raw["backends"].(map[string]any)["openclaw"].(map[string]any)
	if !ok {
		t.Fatalf("could not navigate to backends.openclaw in: %s", string(data))
	}
	if _, present := openclaw["binary_path"]; present {
		t.Errorf("binary_path should be omitted when empty, got: %s", string(data))
	}
	if _, present := openclaw["state_dir"]; !present {
		t.Errorf("state_dir should be present when set, got: %s", string(data))
	}
}

// TestCLIConfig_ProfileCommandOverrides_RoundTrip verifies that pinning a
// per-machine profile command path survives a save/load cycle AND that
// unrelated fields (server_url, token, backends) are preserved across the
// round-trip — the set-path / unset-path CLI commands rely on a
// load->modify->save cycle never dropping config the user already had.
func TestCLIConfig_ProfileCommandOverrides_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	original := CLIConfig{
		ServerURL:   "https://api.multica.ai",
		AppURL:      "https://multica.ai",
		WorkspaceID: "ws-123",
		Token:       "mul_xyz",
		Backends: &BackendOverrides{
			OpenClaw: &OpenClawOverride{StateDir: "/var/lib/openclaw-prod"},
		},
		ProfileCommandOverrides: map[string]string{
			"prof-1": "/opt/bin/company-codex",
			"prof-2": "/usr/local/bin/special-claude",
		},
	}
	if err := SaveCLIConfig(original); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCLIConfig()
	if err != nil {
		t.Fatal(err)
	}

	// The override map must round-trip intact.
	if len(loaded.ProfileCommandOverrides) != 2 {
		t.Fatalf("ProfileCommandOverrides len = %d, want 2: %+v", len(loaded.ProfileCommandOverrides), loaded.ProfileCommandOverrides)
	}
	if got := loaded.ProfileCommandOverrides["prof-1"]; got != "/opt/bin/company-codex" {
		t.Errorf("prof-1 override = %q, want /opt/bin/company-codex", got)
	}
	if got := loaded.ProfileCommandOverrides["prof-2"]; got != "/usr/local/bin/special-claude" {
		t.Errorf("prof-2 override = %q, want /usr/local/bin/special-claude", got)
	}

	// Every other field must be preserved (no clobbering on round-trip).
	if loaded.ServerURL != original.ServerURL {
		t.Errorf("ServerURL = %q, want %q", loaded.ServerURL, original.ServerURL)
	}
	if loaded.AppURL != original.AppURL {
		t.Errorf("AppURL = %q, want %q", loaded.AppURL, original.AppURL)
	}
	if loaded.WorkspaceID != original.WorkspaceID {
		t.Errorf("WorkspaceID = %q, want %q", loaded.WorkspaceID, original.WorkspaceID)
	}
	if loaded.Token != original.Token {
		t.Errorf("Token = %q, want %q", loaded.Token, original.Token)
	}
	if loaded.Backends == nil || loaded.Backends.OpenClaw == nil ||
		loaded.Backends.OpenClaw.StateDir != "/var/lib/openclaw-prod" {
		t.Errorf("Backends.OpenClaw not preserved: %+v", loaded.Backends)
	}
}

// TestCLIConfig_ProfileCommandOverrides_OmittedWhenEmpty verifies the
// omitempty tag keeps the key out of the on-disk JSON when no overrides are
// set, so configs for users who never pin a path stay byte-stable.
func TestCLIConfig_ProfileCommandOverrides_OmittedWhenEmpty(t *testing.T) {
	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	cfg := CLIConfig{ServerURL: "https://api.multica.ai", Token: "mul_xyz"}
	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatal(err)
	}

	configPath, err := CLIConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["profile_command_overrides"]; ok {
		t.Errorf("profile_command_overrides should be omitted when empty, got: %s", string(data))
	}
}

// TestCLIConfig_UnknownFieldsArePreserved verifies forward-compat: a future
// daemon that adds, say, a `backends.codex` key should not have its data
// destroyed when an older daemon (without knowledge of that key) reads and
// re-saves the file. Today Go's encoding/json silently DROPS unknown fields
// on round-trip. This test documents the gap so future maintainers know.
//
// Skipped today (encoding/json does not preserve unknown fields), but the
// test is written so a future change to a preserve-unknown encoder
// (json.RawMessage, mapstructure, etc.) will pick it up.
func TestCLIConfig_UnknownFieldsArePreserved(t *testing.T) {
	t.Skip("documenting known limitation: encoding/json drops unknown fields on round-trip; future PR can switch to a preserving encoder")

	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	cfgDir := filepath.Join(tmp, ".multica")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	withFutureField := `{
  "server_url": "https://api.multica.ai",
  "token": "mul_xyz",
  "backends": {
    "openclaw": {"state_dir": "/x"},
    "future_backend_xyz": {"some_setting": "preserve me"}
  }
}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(withFutureField), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// After round-trip, future_backend_xyz should still be in the file.
	data, _ := os.ReadFile(filepath.Join(cfgDir, "config.json"))
	if !strings.Contains(string(data), "future_backend_xyz") {
		t.Error("unknown field future_backend_xyz was dropped on round-trip")
	}
}

// TestCLIConfig_DaemonKnobs_RoundTrip verifies that every persisted
// daemon knob added on top of #3824 survives a Save -> Load cycle,
// including the tri-state AgentTimeout pointer (an explicit "0s" is
// distinguishable from "not set"). If a future refactor accidentally
// drops one of these fields from the schema, this test fails at write
// time instead of silently losing the operator's config on restart.
func TestCLIConfig_DaemonKnobs_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	zero := "0s"
	original := CLIConfig{
		HeartbeatInterval:              "5s",
		AgentTimeout:                   &zero,
		CodexSemanticInactivityTimeout: "15m",
		CodexHandshakeTimeout:          "45s",
		DisableAutoUpdate:              true,
		AutoUpdateCheckInterval:        "12h",
	}
	if err := SaveCLIConfig(original); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCLIConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HeartbeatInterval != "5s" ||
		loaded.CodexSemanticInactivityTimeout != "15m" ||
		loaded.CodexHandshakeTimeout != "45s" ||
		!loaded.DisableAutoUpdate ||
		loaded.AutoUpdateCheckInterval != "12h" {
		t.Errorf("scalar knobs lost on round-trip: %+v", loaded)
	}
	if loaded.AgentTimeout == nil || *loaded.AgentTimeout != "0s" {
		t.Errorf("AgentTimeout tri-state lost: got %v, want &\"0s\"", loaded.AgentTimeout)
	}
}

func TestCLIConfig_TaskRootOverridesOwnerHome(t *testing.T) {
	ownerHome := t.TempDir()
	taskRoot := filepath.Join(t.TempDir(), "task-multica")
	redirectTestHome(t, ownerHome)
	t.Setenv("MULTICA_TASK_CONFIG_ROOT", taskRoot)

	ownerPath := filepath.Join(ownerHome, ".multica", "config.json")
	if err := os.MkdirAll(filepath.Dir(ownerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	ownerBytes := []byte("{\n  \"server_url\": \"https://owner.invalid\",\n  \"token\": \"mul_owner_sentinel\"\n}\n")
	if err := os.WriteFile(ownerPath, ownerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(taskRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := SaveCLIConfigForProfile(CLIConfig{ServerURL: "https://task.invalid"}, "dev"); err != nil {
		t.Fatalf("SaveCLIConfigForProfile: %v", err)
	}

	path, err := CLIConfigPathForProfile("dev")
	if err != nil {
		t.Fatalf("CLIConfigPathForProfile: %v", err)
	}
	wantPath := filepath.Join(taskRoot, "profiles", "dev", "config.json")
	if path != wantPath {
		t.Fatalf("path = %q, want task-local path %q", path, wantPath)
	}
	loaded, err := LoadCLIConfigForProfile("dev")
	if err != nil {
		t.Fatalf("LoadCLIConfigForProfile: %v", err)
	}
	if loaded.ServerURL != "https://task.invalid" || loaded.Token != "" {
		t.Fatalf("loaded task config = %#v, want task-only settings", loaded)
	}
	after, err := os.ReadFile(ownerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(ownerBytes) {
		t.Fatalf("owner config was modified: got %q, want original sentinel", after)
	}
	for _, dir := range []string{taskRoot, filepath.Join(taskRoot, "profiles"), filepath.Join(taskRoot, "profiles", "dev")} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat task config directory %q: %v", dir, err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("task config directory %q mode = %#o, want 0700", dir, got)
		}
	}
	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatalf("stat task config file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("task config file mode = %#o, want 0600", got)
	}
}

// A machine that has never run the product resolves the current directory
// name; see TestCLIConfigRoot_UpgradedInstallKeepsLegacyDirectory for the
// upgrade path.
func TestCLIConfig_NoTaskRootKeepsInteractiveHomeResolution(t *testing.T) {
	home := t.TempDir()
	redirectTestHome(t, home)
	t.Setenv("MULTICA_TASK_CONFIG_ROOT", "")

	path, err := CLIConfigPathForProfile("dev")
	if err != nil {
		t.Fatalf("CLIConfigPathForProfile: %v", err)
	}
	want := filepath.Join(home, brand.ConfigDir, "profiles", "dev", "config.json")
	if path != want {
		t.Fatalf("path = %q, want interactive path %q", path, want)
	}
}

func TestCLIConfig_TaskRootRejectsProfilePathTraversal(t *testing.T) {
	t.Setenv("MULTICA_TASK_CONFIG_ROOT", filepath.Join(t.TempDir(), "task-multica"))

	for _, profile := range []string{".", "..", "../owner", "nested/profile", filepath.Join(string(filepath.Separator), "owner")} {
		if path, err := CLIConfigPathForProfile(profile); err == nil {
			t.Errorf("CLIConfigPathForProfile(%q) = %q, want invalid task profile error", profile, path)
		}
		if dir, err := ProfileDir(profile); err == nil {
			t.Errorf("ProfileDir(%q) = %q, want invalid task profile error", profile, dir)
		}
	}
}

func TestCLIConfig_TaskRootMustBeAbsolute(t *testing.T) {
	t.Setenv("MULTICA_TASK_CONFIG_ROOT", "relative/task-multica")

	if _, err := CLIConfigPath(); err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("CLIConfigPath error = %v, want absolute path validation", err)
	}
}

// TestCLIConfig_OpenClawCLITimeout_RoundTrip covers the knob added for #7112:
// a host whose openclaw CLI is slower than the built-in default needs to
// record that in the config file, not just in a shell export the GUI-launched
// daemon never sees.
func TestCLIConfig_OpenClawCLITimeout_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	redirectTestHome(t, tmp)

	original := CLIConfig{
		ServerURL: "https://api.multica.ai",
		Token:     "mul_xyz",
		Backends: &BackendOverrides{
			OpenClaw: &OpenClawOverride{CLITimeout: "45s"},
		},
	}
	if err := SaveCLIConfig(original); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCLIConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Backends == nil || loaded.Backends.OpenClaw == nil {
		t.Fatalf("Backends.OpenClaw should be non-nil after round-trip, got %+v", loaded.Backends)
	}
	if got := loaded.Backends.OpenClaw.CLITimeout; got != "45s" {
		t.Errorf("CLITimeout round-trip: got %q, want %q", got, "45s")
	}
	// The other fields stay omitted so they keep falling through to env /
	// discovery instead of being pinned to an empty string.
	if got := loaded.Backends.OpenClaw.BinaryPath; got != "" {
		t.Errorf("BinaryPath should stay empty, got %q", got)
	}
}

func TestCLIConfigRoot_UpgradedInstallKeepsLegacyDirectory(t *testing.T) {
	home := t.TempDir()
	redirectTestHome(t, home)
	t.Setenv(TaskConfigRootEnv, "")
	legacyDir := filepath.Join(home, brand.LegacyConfigDir)
	if err := os.MkdirAll(filepath.Join(legacyDir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, profile := range []string{"", "dev"} {
		wantDir := legacyDir
		wantConfig := filepath.Join(legacyDir, "config.json")
		if profile != "" {
			wantDir = filepath.Join(legacyDir, "profiles", profile)
			wantConfig = filepath.Join(wantDir, "config.json")
		}
		if got, err := ProfileDir(profile); err != nil || got != wantDir {
			t.Fatalf("ProfileDir(%q) = %q, %v; want %q", profile, got, err, wantDir)
		}
		if got, err := CLIConfigPathForProfile(profile); err != nil || got != wantConfig {
			t.Fatalf("CLIConfigPathForProfile(%q) = %q, %v; want %q", profile, got, err, wantConfig)
		}
	}

	// The legacy file is what loads, not a second copy under the new name.
	if err := SaveCLIConfig(CLIConfig{ServerURL: "https://legacy.example", Token: "mul_legacy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, brand.ConfigDir)); !os.IsNotExist(err) {
		t.Fatalf("upgraded install created %q; error = %v", brand.ConfigDir, err)
	}
	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerURL != "https://legacy.example" || cfg.Token != "mul_legacy" {
		t.Fatalf("loaded config = %#v, want the value saved through the legacy directory", cfg)
	}
}

func TestCLIConfigRoot_FreshInstallUsesCurrentDirectory(t *testing.T) {
	home := t.TempDir()
	redirectTestHome(t, home)
	t.Setenv(TaskConfigRootEnv, "")

	wantRoot := filepath.Join(home, brand.ConfigDir)
	if got, err := ProfileDir(""); err != nil || got != wantRoot {
		t.Fatalf("ProfileDir(\"\") = %q, %v; want %q", got, err, wantRoot)
	}
	if got, err := CLIConfigPathForProfile("dev"); err != nil || got != filepath.Join(wantRoot, "profiles", "dev", "config.json") {
		t.Fatalf("CLIConfigPathForProfile(dev) = %q, %v; want below %q", got, err, wantRoot)
	}
	if _, err := os.Stat(filepath.Join(home, brand.LegacyConfigDir)); !os.IsNotExist(err) {
		t.Fatalf("fresh install created the legacy directory; error = %v", err)
	}
}

// The renamed environment variable reaches the same read through the
// startup bridge, so an operator can switch names before the old one is
// retired.
func TestCLIConfigRoot_AcceptsRenamedTaskRootEnv(t *testing.T) {
	home := t.TempDir()
	redirectTestHome(t, home)
	taskRoot := filepath.Join(t.TempDir(), "task-u-workers")
	t.Setenv("U_WORKERS_TASK_CONFIG_ROOT", taskRoot)
	envcompat.Bridge()
	t.Cleanup(func() { _ = os.Unsetenv(TaskConfigRootEnv) })

	got, err := CLIConfigPathForProfile("dev")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(taskRoot, "profiles", "dev", "config.json")
	if got != want {
		t.Fatalf("CLIConfigPathForProfile(dev) = %q, want %q from the renamed env var", got, want)
	}
}

// A fresh install whose login is already in ~/.u-workers must keep using it
// when an unrelated legacy directory appears (the Pi runtime creates
// ~/.multica/pi-sessions). See CLAWSTA-99 review item 2.
func TestCLIConfigRoot_CurrentStateSurvivesUnrelatedLegacyDirectory(t *testing.T) {
	home := t.TempDir()
	redirectTestHome(t, home)
	t.Setenv(TaskConfigRootEnv, "")

	currentDir := filepath.Join(home, brand.ConfigDir)
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "config.json"), []byte(`{"server_url":"https://fresh.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// Another product runtime creates a directory under the legacy root.
	if err := os.MkdirAll(filepath.Join(home, brand.LegacyConfigDir, "pi-sessions"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := CLIConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(currentDir, "config.json")
	if got != want {
		t.Fatalf("CLIConfigPath() = %q, want %q: a Pi session directory must not move the CLI config root", got, want)
	}
	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerURL != "https://fresh.example" {
		t.Fatalf("loaded server_url = %q, want the value saved in the current directory", cfg.ServerURL)
	}
}
