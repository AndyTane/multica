package daemon

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"testing"
)

func TestRegisterRuntimes_IncludesSessionResumeLauncher(t *testing.T) {
	fx := newProfileRegisterFixture(t, nil, http.StatusOK)
	fx.daemon.cfg.Profile = "dev"
	fx.daemon.cfg.WorkspacesRoot = t.TempDir()
	builtins := []map[string]string{{"name": "Fake Codex", "type": "codex", "status": "online"}}
	for _, builtinOnly := range []bool{false, true} {
		var err error
		if builtinOnly {
			_, err = fx.daemon.registerBuiltinRuntimesForWorkspaceLocked(context.Background(), "ws-1", builtins)
		} else {
			_, _, err = fx.daemon.registerRuntimesForWorkspaceBatchLocked(context.Background(), "ws-1", builtins)
		}
		if err != nil {
			t.Fatal(err)
		}
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		got := fx.sentResume
		if got == nil || got.CLIPath != executable || got.Profile != "dev" || got.WorkspacesRoot != fx.daemon.cfg.WorkspacesRoot || got.OS != runtime.GOOS {
			t.Fatalf("builtinOnly=%v: launcher = %#v", builtinOnly, got)
		}
	}
}
