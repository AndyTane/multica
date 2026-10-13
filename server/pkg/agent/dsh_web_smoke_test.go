//go:build agentintegration

package agent

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDshWebBridgeRealSession(t *testing.T) {
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("real-agent smoke tests require explicit opt-in")
	}
	bridge := os.Getenv("MULTICA_DSH_WEB_SMOKE_PATH")
	if bridge == "" {
		t.Fatal("MULTICA_DSH_WEB_SMOKE_PATH must name the built web bridge")
	}
	cwd := t.TempDir()
	requestPrefix := "multica-web-smoke-" + uuid.NewString()
	backend, err := New("dsh", Config{ExecutablePath: bridge, Logger: slog.Default(), TaskID: requestPrefix + "-1"})
	if err != nil {
		t.Fatal(err)
	}
	run := func(b Backend, marker, resume string) Result {
		t.Helper()
		session, err := b.Execute(context.Background(), "Reply with exactly "+marker+". Do not call tools, modify files, or perform any other work.", ExecOptions{
			Cwd: cwd, ResumeSessionID: resume, ThreadName: "DSH bridge verification", Timeout: 2 * time.Minute,
		})
		if err != nil {
			t.Fatal(err)
		}
		for range session.Messages {
		}
		result := <-session.Result
		if result.Status != "completed" || !strings.Contains(result.Output, marker) {
			t.Fatalf("DSH smoke status=%q output=%q error=%q", result.Status, result.Output, result.Error)
		}
		return result
	}
	first := run(backend, "MULTICA_DSH_WEB_SMOKE_1", "")
	if first.SessionID == "" {
		t.Fatal("web bridge did not return a session identity")
	}
	secondBackend, err := New("dsh", Config{ExecutablePath: bridge, Logger: slog.Default(), TaskID: requestPrefix + "-2"})
	if err != nil {
		t.Fatal(err)
	}
	second := run(secondBackend, "MULTICA_DSH_WEB_SMOKE_2", first.SessionID)
	if second.SessionID != first.SessionID {
		t.Fatalf("follow-up changed session: %q -> %q", first.SessionID, second.SessionID)
	}
	t.Logf("Browser-owned DSH session: %s", first.SessionID)
}
