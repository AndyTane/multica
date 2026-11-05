package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/tlsx"
)

// prepareTaskClientTrust refreshes daemon-owned task config with public trust
// only. Never load or copy the owner's profile here. The CLI can use this file
// if a shell drops the CA variable but retains the task config root and identity.
func prepareTaskClientTrust(configRoot, rootCAFile string) (string, error) {
	if strings.TrimSpace(configRoot) == "" || !filepath.IsAbs(configRoot) {
		return "", fmt.Errorf("task client trust requires an absolute task config root")
	}
	rootCAFile = strings.TrimSpace(rootCAFile)
	if rootCAFile != "" {
		var err error
		// Resolve before the provider switches to the task working directory.
		rootCAFile, err = filepath.Abs(rootCAFile)
		if err != nil {
			return "", fmt.Errorf("resolve task root CA: %w", err)
		}
		if _, err := tlsx.LoadPool(rootCAFile); err != nil {
			return "", fmt.Errorf("prepare task root CA: %w", err)
		}
	}
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		return "", fmt.Errorf("create task trust config directory: %w", err)
	}
	if err := os.Chmod(configRoot, 0o700); err != nil {
		return "", fmt.Errorf("restrict task trust config directory: %w", err)
	}
	// Refresh even without a CA: a reused task directory must not retain trust
	// from an earlier dispatch. Authentication remains in the task environment.
	data, err := json.MarshalIndent(cli.CLIConfig{RootCAFile: rootCAFile}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode task trust config: %w", err)
	}
	tmp, err := os.CreateTemp(configRoot, ".trust-*.json.tmp")
	if err != nil {
		return "", fmt.Errorf("create task trust config: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write task trust config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close task trust config: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return "", fmt.Errorf("restrict task trust config: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(configRoot, "config.json")); err != nil {
		return "", fmt.Errorf("install task trust config: %w", err)
	}
	return rootCAFile, nil
}
