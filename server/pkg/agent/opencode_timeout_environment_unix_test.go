//go:build unix

package agent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCodeRunChildReceivesShellTimeout(t *testing.T) {
	dir := t.TempDir()
	bin, capture := filepath.Join(dir, "opencode"), filepath.Join(dir, "env")
	writeTestExecutable(t, bin, []byte(`#!/bin/sh
printf '%s' "$OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS" > "$TIMEOUT_CAPTURE"
cat > /dev/null
echo '{"type":"text","part":{"text":"ok"}}'
echo '{"type":"step_finish","part":{"type":"step-finish"}}'
`))
	backend, err := New("opencode", Config{ExecutablePath: bin, BuiltinRuntime: true, CLIVersion: "1.18.3", Logger: slog.Default(), Env: map[string]string{opencodeTransportEnv: "run", "TIMEOUT_CAPTURE": capture}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := backend.Execute(context.Background(), "test", ExecOptions{Cwd: dir, Timeout: 10 * time.Second, OpenCodeShellTimeout: 41 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	for range session.Messages {
	}
	if result := <-session.Result; result.Status != "completed" {
		t.Fatalf("result=%+v", result)
	}
	value, err := os.ReadFile(capture)
	if err != nil || string(value) != "2460000" {
		t.Fatalf("run child timeout=%q err=%v", value, err)
	}
}
