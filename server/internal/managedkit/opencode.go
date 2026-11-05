package managedkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OpenCodeSelection contains references only. Model configuration and credentials
// stay in the user's files, outside immutable program slots and release archives.
type OpenCodeSelection struct {
	Profile    string `json:"profile"`
	Disabled   bool   `json:"disabled"`
	Executable string `json:"executable"`
	ConfigPath string `json:"configPath"`
	ConfigDir  string `json:"configDir"`
	ConfigHome string `json:"configHome"`
}

func ReadOpenCodeSelection(root, profile string) (*OpenCodeSelection, error) {
	if root == "" {
		return nil, nil
	}
	path := filepath.Join(root, "opencode-selection.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64<<10 {
		return nil, errors.New("invalid opencode-selection.json; preserve it and retry Connect.cmd")
	}
	var state struct {
		SchemaVersion int                 `json:"schemaVersion"`
		Selections    []OpenCodeSelection `json:"selections"`
	}
	if ReadJSON(path, &state) != nil || state.SchemaVersion != 1 {
		return nil, errors.New("invalid opencode-selection.json; preserve it and retry Connect.cmd")
	}
	var selected *OpenCodeSelection
	for i := range state.Selections {
		if state.Selections[i].Profile == profile {
			if selected != nil {
				return nil, errors.New("duplicate OpenCode selection for profile")
			}
			selected = &state.Selections[i]
		}
	}
	if selected == nil {
		return nil, nil
	}
	return selected, nil
}

func OpenCodeEnvironment(root, profile string) (map[string]string, error) {
	selected, err := ReadOpenCodeSelection(root, profile)
	if err != nil || selected == nil {
		return nil, err
	}
	if selected.Disabled {
		return map[string]string{"MULTICA_OPENCODE_PATH": filepath.Join(root, ".managed", "opencode-disabled")}, nil
	}
	for _, entry := range []struct{ label, path string }{{"program", selected.Executable}, {"config", selected.ConfigPath}} {
		if !filepath.IsAbs(entry.path) || strings.HasPrefix(entry.path, `\\`) {
			return nil, fmt.Errorf("OpenCode %s must be a local absolute path; retry Connect.cmd", entry.label)
		}
		// File availability and syntax are engine readiness, not client liveness.
		// Keep the selected reference even when the user must repair the file.
	}
	env := map[string]string{"MULTICA_OPENCODE_PATH": selected.Executable, "OPENCODE_CONFIG": selected.ConfigPath}
	for key, value := range map[string]string{"OPENCODE_CONFIG_DIR": selected.ConfigDir, "XDG_CONFIG_HOME": selected.ConfigHome} {
		if value != "" {
			if !filepath.IsAbs(value) || strings.HasPrefix(value, `\\`) {
				return nil, fmt.Errorf("OpenCode %s must be a local absolute directory", key)
			}
			env[key] = value
		}
	}
	return env, nil
}
