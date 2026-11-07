package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/multica-ai/multica/server/pkg/agent"
)

func TestClaudeNativePathDiscovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	native := filepath.Join(home, ".local", "bin", "claude.exe")
	writeExecStub(t, native)

	got, err := resolveAgentExecutablePath("claude")
	if err != nil {
		t.Fatalf("discover native install outside inherited PATH: %v", err)
	}
	assertSameFile(t, got, native)

	pathBin := t.TempDir()
	pathCLI := filepath.Join(pathBin, "claude.cmd")
	writeExecStub(t, pathCLI)
	t.Setenv("PATH", pathBin)
	got, err = resolveAgentExecutablePath("claude")
	if err != nil {
		t.Fatal(err)
	}
	assertSameFile(t, got, pathCLI)

	if _, err := resolveAgentExecutablePath(filepath.Join(home, "missing", "claude.cmd")); err == nil {
		t.Fatal("missing explicit override must not resolve the native install")
	}
	if err := os.Remove(native); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := resolveAgentExecutablePath("claude"); err == nil {
		t.Fatal("missing native install must remain unavailable")
	}
}

func TestClaudeNativeMigrationRepeatedLaunch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	native := filepath.Join(home, ".local", "bin", "claude.exe")
	writeExecStub(t, native)
	originalDetect := detectAgentVersion
	probes := 0
	detectAgentVersion = func(_ context.Context, cmd agent.Command) (string, error) {
		assertSameFile(t, cmd.Path, native)
		probes++
		return "2.1.309", nil
	}
	t.Cleanup(func() { detectAgentVersion = originalDetect })
	d := newSelfHealTestDaemon()
	entry := AgentEntry{Path: filepath.Join(home, "npm", "claude.cmd"), Command: "claude"}
	for i := 0; i < 2; i++ {
		got, version, err := d.resolveAgentEntryForLaunch(context.Background(), "claude", entry)
		if err != nil {
			t.Fatalf("launch %d after native migration: %v", i+1, err)
		}
		assertSameFile(t, got.Path, native)
		if version != "2.1.309" {
			t.Fatalf("version = %q, want verified native version", version)
		}
	}
	if probes != 1 {
		t.Fatalf("version probes = %d, want one shared verified replacement", probes)
	}
}

func TestClaudeNativeMigrationRejectsUnverifiedReplacement(t *testing.T) {
	for _, version := range []string{"", "1.0.0"} {
		t.Run("version="+version, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("USERPROFILE", home)
			t.Setenv("PATH", t.TempDir())
			t.Setenv("SHELL", "")
			writeExecStub(t, filepath.Join(home, ".local", "bin", "claude.exe"))
			originalDetect := detectAgentVersion
			detectAgentVersion = func(context.Context, agent.Command) (string, error) {
				if version == "" {
					return "", errors.New("version probe failed")
				}
				return version, nil
			}
			t.Cleanup(func() { detectAgentVersion = originalDetect })
			d := newSelfHealTestDaemon()
			entry := AgentEntry{Path: filepath.Join(home, "npm", "claude.cmd"), Command: "claude"}
			if _, _, err := d.resolveAgentEntryForLaunch(context.Background(), "claude", entry); err == nil {
				t.Fatal("replacement must pass version detection and minimum-version checks")
			}
			if len(d.resolvedPaths) != 0 {
				t.Fatal("unverified replacement was cached")
			}
		})
	}
}
