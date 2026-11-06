package agent

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeShellTimeoutEnvironmentTransports(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		acp           bool
	}{
		{"run", "1.18.3", false},
		{"ACP_1.x", "1.18.3", true},
		{"ACP_2.x", "opencode v2.2.0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{BuiltinRuntime: true, CLIVersion: tc.version, Logger: slog.Default(), Env: map[string]string{
				"OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS": "120000",
			}}
			opts := ExecOptions{Cwd: t.TempDir(), OpenCodeShellTimeout: 41 * time.Minute,
				McpConfig: []byte(`{"mcpServers":{"probe":{"command":"node","args":["s.js"]}}}`)}
			build := openCodeEnvironment
			if tc.acp {
				build = openCodeACPEnvironment
			}
			env, err := build(cfg, opts)
			if err != nil {
				t.Fatal(err)
			}
			values := map[string]string{}
			for _, entry := range env {
				if k, v, ok := strings.Cut(entry, "="); ok {
					values[k] = v
				}
			}
			if got := values["OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS"]; got != "2460000" {
				t.Fatalf("%s shell timeout = %q, want 2460000", tc.name, got)
			}
			if values["PWD"] != opts.Cwd {
				t.Fatalf("PWD = %q", values["PWD"])
			}
			if tc.version == "opencode v2.2.0" {
				if values["OPENCODE_CONFIG_CONTENT"] != "" {
					t.Fatal("2.x MCP must use session/new")
				}
			} else if !strings.Contains(values["OPENCODE_CONFIG_CONTENT"], "probe") {
				t.Fatal("1.x MCP missing")
			}
			opts.OpenCodeShellTimeout = 0
			env, err = build(cfg, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(env, "\n"), "OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS=120000") {
				t.Fatal("unset server timeout must preserve the configured runtime default")
			}
		})
	}
}
