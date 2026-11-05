package managedkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCodeEnvironment(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	executable := filepath.Join(external, "opencode.exe")
	config := filepath.Join(external, "opencode.jsonc")
	for _, p := range []string{executable, config} {
		if err := os.WriteFile(p, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := map[string]any{"schemaVersion": 1, "selections": []OpenCodeSelection{
		{Profile: "office", Executable: executable, ConfigPath: config, ConfigDir: external},
		{Profile: "other", Disabled: true},
	}}
	if err := WriteJSON(filepath.Join(root, "opencode-selection.json"), state); err != nil {
		t.Fatal(err)
	}
	for _, slot := range []string{".", "programs/v0.2.12-aaaaaaaaaaaa-1"} {
		if err := SetProgram(root, slot); err != nil {
			t.Fatal(err)
		}
		env, err := OpenCodeEnvironment(root, "office")
		if err != nil || env["MULTICA_OPENCODE_PATH"] != executable || env["OPENCODE_CONFIG"] != config || env["OPENCODE_CONFIG_DIR"] != external {
			t.Fatalf("selection after slot switch: %v, %v", env, err)
		}
	}
	if env, err := OpenCodeEnvironment(root, "unknown"); err != nil || len(env) != 0 {
		t.Fatalf("cross-profile selection: %v %v", env, err)
	}
	if env, err := OpenCodeEnvironment(root, "other"); err != nil || env["OPENCODE_CONFIG"] != "" || env["MULTICA_OPENCODE_PATH"] == "" {
		t.Fatalf("disabled provider: %v %v", env, err)
	}
	if err := os.Remove(executable); err != nil {
		t.Fatal(err)
	}
	if env, err := OpenCodeEnvironment(root, "office"); err != nil || env["MULTICA_OPENCODE_PATH"] != executable {
		t.Fatal("missing saved program must retain its reference for pending configuration", err)
	}
	if _, err := OpenCodeEnvironment(root, "other"); err != nil {
		t.Fatal("unrelated runtime blocked", err)
	}
}

func TestOpenCodeEnvironmentRejectsInvalidState(t *testing.T) {
	for _, raw := range []string{`null`, `{"schemaVersion":2}`, `{"schemaVersion":1,"selections":[{"profile":"x","executable":"relative.exe","configPath":"relative.json"}]}`, `{"schemaVersion":1,"selections":[{"profile":"x","disabled":true},{"profile":"x","disabled":true}]}`} {
		t.Run(raw, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "opencode-selection.json"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenCodeEnvironment(root, "x"); err == nil {
				t.Fatal("invalid selection accepted")
			}
		})
	}
}
