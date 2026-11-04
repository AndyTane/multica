package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OpenCode loads global files, a custom file, and custom-directory files in
// that order. Project configuration remains owned by the provider at task cwd.
// https://docs.opencode.ai/docs/config/
func loadOpenCodeMcpConfigs(home string) (map[string]any, bool, error) {
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	paths := []string{filepath.Join(configHome, "opencode", "opencode.json"), filepath.Join(configHome, "opencode", "opencode.jsonc")}
	if custom := strings.TrimSpace(os.Getenv("OPENCODE_CONFIG")); custom != "" {
		paths = append(paths, custom)
	}
	if dir := strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_DIR")); dir != "" {
		paths = append(paths, filepath.Join(dir, "opencode.json"), filepath.Join(dir, "opencode.jsonc"))
	}
	servers := map[string]any{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, true, fmt.Errorf("read OpenCode config: %s", path)
		}
		cfg, err := unmarshalRuntimeMcpConfig(raw, "jsonc")
		if err != nil {
			return nil, true, fmt.Errorf("invalid OpenCode JSON/JSONC config: %s", path)
		}
		if configured, ok := nestedRuntimeMcpMap(cfg, "mcp"); ok {
			for name, value := range configured {
				servers[name] = value
			}
		}
	}
	return servers, true, nil
}
