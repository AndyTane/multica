package daemon

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

// The other half of the bootstrap. A daemon that started with nothing
// registered tracks no workspace, and every converge path iterates the
// workspaces it tracks — so the kick that normally brings a finished install
// online would have nothing to look at, and the runtime would wait for the
// periodic consistency sync instead.
func TestRegisterAfterDshProfileInstall_PicksUpTheWorkspaceWhenNoneIsTracked(t *testing.T) {
	fx := newBatchFixture(t)
	d := fx.daemon
	// Any provider will do: what is under test is that the workspace gets
	// picked up at all. Not dsh, deliberately — the dsh path would re-run the
	// profile probe against a fake binary and drop it again, which would make
	// this assert nothing.
	d.cfg.Agents = map[string]AgentEntry{"claude": {Path: "/fake/claude"}}
	fx.setWorkspaces(WorkspaceInfo{ID: "ws-1", Name: "one"})

	if got := d.trackedWorkspaceCount(); got != 0 {
		t.Fatalf("precondition: tracked workspaces = %d, want 0", got)
	}

	d.registerAfterDshProfileInstall(context.Background())

	if got := d.trackedWorkspaceCount(); got == 0 {
		t.Fatal("the workspace was never picked up; the runtime would wait for the periodic sync")
	}
}

// With a workspace already tracked there is something for a converge round to
// look at, so the cheap kick is enough and the install must not spend a
// workspace sync on it.
func TestRegisterAfterDshProfileInstall_KicksWhenAWorkspaceIsTracked(t *testing.T) {
	d := &Daemon{
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		workspaces:         map[string]*workspaceState{"ws-1": {}},
		agentDiscoveryKick: make(chan struct{}, 1),
		// client stays nil: reaching the sync path would panic, which is the
		// assertion — this branch must not take it.
	}

	d.registerAfterDshProfileInstall(context.Background())

	select {
	case <-d.agentDiscoveryKick:
	case <-time.After(time.Second):
		t.Fatal("no discovery round was requested after the install finished")
	}
}

// A converge round may condemn dsh and nothing else.
//
// On main only refreshAgentVersions acts on demotable verdicts, on its own
// ten-minute cadence, and that is the schedule every provider's verdict was
// tuned against. dsh needs a round it can be forced into, because its
// precondition — the runtime profile — can change without any version changing;
// letting that round condemn Claude Code or Codex too would move them onto a
// different schedule as a side effect of a DSH fix.
func TestDshOnlyVerdicts(t *testing.T) {
	demotable := map[string]runtimeVerdict{
		"claude": {reason: "version too old"},
		"codex":  {reason: "agent CLI is not executable"},
		"dsh":    {reason: dshMissingProfileReason},
	}

	got := dshOnlyVerdicts(demotable)
	if len(got) != 1 {
		t.Fatalf("verdicts = %v, want only dsh", got)
	}
	if got["dsh"].reason != dshMissingProfileReason {
		t.Errorf("dsh verdict = %q, want it carried through unchanged", got["dsh"].reason)
	}

	// Nothing to do is nil, so the caller's length check reads the same either
	// way.
	if got := dshOnlyVerdicts(map[string]runtimeVerdict{"claude": {reason: "x"}}); got != nil {
		t.Errorf("verdicts = %v, want nil when dsh is not condemned", got)
	}
	if got := dshOnlyVerdicts(nil); got != nil {
		t.Errorf("verdicts = %v, want nil for an empty round", got)
	}
}
