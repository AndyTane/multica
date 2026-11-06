package agent

import (
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeShellDefaultEnvironmentTransports(t *testing.T) {
	const key = "OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS"
	for _, transport := range []struct {
		name, version string
		acp           bool
	}{
		{"run", "1.18.3", false}, {"ACP_1.x", "1.18.3", true}, {"ACP_2.x", "2.2.0", true},
	} {
		t.Run(transport.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, inherited, configured, want string
				explicit, derived                 time.Duration
			}{
				{"ordinary_install", "", "", "7200000", 0, 2 * time.Hour},
				{"tool_budget", "", "", "10800000", 0, 3 * time.Hour},
				{"custom_env", "", "18000000", "18000000", 0, 2 * time.Hour},
				{"inherited_env", "14400000", "", "14400000", 0, 2 * time.Hour},
				{"custom_over_inherited", "14400000", "18000000", "18000000", 0, 2 * time.Hour},
				{"explicit_server_over_env", "14400000", "18000000", "900000", 15 * time.Minute, 2 * time.Hour},
				{"explicit_server_only", "", "", "900000", 15 * time.Minute, 0},
				{"old_server", "", "", "", 0, 0},
				{"old_server_with_env", "", "18000000", "18000000", 0, 0},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Setenv(key, tc.inherited)
					if tc.inherited == "" {
						if err := os.Unsetenv(key); err != nil {
							t.Fatal(err)
						}
					}
					cfg := Config{BuiltinRuntime: true, CLIVersion: transport.version, Logger: slog.Default(), Env: map[string]string{"UNRELATED": "retained"}}
					configuredKey := key
					if runtime.GOOS == "windows" {
						configuredKey = strings.ToLower(key)
					}
					if tc.configured != "" {
						cfg.Env[configuredKey] = tc.configured
					}
					opts := ExecOptions{Cwd: t.TempDir(), OpenCodeShellTimeout: tc.explicit, OpenCodeShellDefaultTimeout: tc.derived}
					build := openCodeEnvironment
					if transport.acp {
						build = openCodeACPEnvironment
					}
					env, err := build(cfg, opts)
					if err != nil {
						t.Fatal(err)
					}
					values := map[string]string{}
					for _, entry := range env {
						k, v, ok := strings.Cut(entry, "=")
						if runtime.GOOS == "windows" && strings.EqualFold(k, key) {
							k = key
						}
						if ok {
							values[k] = v
						}
					}
					if values[key] != tc.want || values["UNRELATED"] != "retained" || values["PWD"] != opts.Cwd {
						t.Fatalf("child timeout=%q, want %q; unrelated=%q pwd=%q", values[key], tc.want, values["UNRELATED"], values["PWD"])
					}
					if cfg.Env[configuredKey] != tc.configured || os.Getenv(key) != tc.inherited {
						t.Fatal("per-run default mutated shared configuration or parent environment")
					}
				})
			}
		})
	}
}
