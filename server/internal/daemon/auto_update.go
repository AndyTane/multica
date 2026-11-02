package daemon

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/brand"
)

// detectSelfVersion runs `<path> --version` and returns the version field.
// Indirection so tests never fork a real process.
var detectSelfVersion = func(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "--version")
	// Context cancellation only kills the direct child; a descendant that
	// inherited our stdout pipe keeps Output() blocked past the deadline.
	// WaitDelay force-closes the pipes after the kill — without it a replaced
	// binary that misbehaves would park the autoUpdateLoop goroutine forever
	// with the updating flag held. Same guard as detectCLIVersion.
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("run %s --version: %w", path, err)
	}
	return ParseSelfVersion(string(out)), nil
}

// ParseSelfVersion pulls the version out of `<cli> --version` output, whose
// first line is rendered by cmd/multica's version template:
//
//	u-workers 0.3.7 (commit: abc1234, built: 2026-07-29T10:00:00Z)
//	go: go1.26.1, os/arch: darwin/arm64
//
// The leading token is the command name the binary was invoked as, which is the
// new name for current installs and the legacy one for pre-rename installs, so
// both spellings are accepted. Matching only one of them is exactly the failure
// this function used to have: the whole line came back as the "version", it
// never equalled Config.CLIVersion, and the daemon re-execed itself on every
// check. Anything that doesn't match the template shape is returned trimmed,
// which compares unequal and is therefore reported rather than silently
// ignored.
//
// The extracted field is exactly what ldflags put in main.version, which is
// also what lands in Config.CLIVersion — so the two are directly comparable.
//
// Exported solely so cmd/multica can pin that contract from the producing side:
// this parser and the version template have to agree, and nothing else would
// notice if a template edit silently broke the auto-reload probe.
func ParseSelfVersion(raw string) string {
	line, _, _ := strings.Cut(raw, "\n")
	line = strings.TrimSpace(line)
	if fields := strings.Fields(line); len(fields) >= 2 && brand.IsCLICommandName(fields[0]) {
		return fields[1]
	}
	return line
}

// autoUpdateInitialDelay defers the first local binary probe after startup.
var autoUpdateInitialDelay = 2 * time.Minute

// selfReloadCheckInterval is how often the loop compares the version compiled
// into this process against the `--version` output of the binary it would
// re-exec. One fork/exec per tick, machine-level (not per workspace, not per
// runtime), so the cost is fixed no matter how big the daemon's workload is.
// Overridable for tests.
//
var selfReloadCheckInterval = 10 * time.Minute

// selfReloadProbeTimeout bounds the `--version` fork/exec. Generous: the point
// of the timeout is to stop a wedged binary from parking the loop goroutine,
// not to police a slow disk.
var selfReloadProbeTimeout = 10 * time.Second

// autoUpdateLoop only follows a binary explicitly replaced by the operator.
// Release polling and automatic downloads are disabled, regardless of legacy flags.
func (d *Daemon) autoUpdateLoop(ctx context.Context) {
	if d.cfg.LaunchedBy == "desktop" || !d.cfg.AutoReloadEnabled {
		return
	}
	if err := sleepWithContext(ctx, autoUpdateInitialDelay); err != nil {
		return
	}
	d.trySelfReload(ctx)
	ticker := time.NewTicker(selfReloadCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.trySelfReload(ctx)
		}
	}
}

// trySelfReload restarts the daemon when the multica binary on disk no longer
// reports the version compiled into this process.
//
// This is the out-of-band half of self-update. `brew upgrade multica`, a
// re-download, or a developer's `make build` all replace the binary at the same
// path, and the daemon then keeps serving the version it booted with: its own
// version string is frozen at compile time, and the pinned-path self-heal in
// resolveAgentEntry only fires when a path *vanishes*, which an in-place
// overwrite never does.
//
// The claim barrier prevents a local reload from interrupting active work.
func (d *Daemon) trySelfReload(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if d.RestartBinary() != "" {
		return
	}
	// Acquire update ownership rather than sampling it. A plain Load() is only a
	// hint: handleUpdate could win the CAS in the gap between the read and
	// triggerRestart, and this path would then cancel the root context while the
	// binary is still being replaced — re-execing a half-written file and cutting
	// the server-triggered update's status reporting off partway. The CAS is the
	// only thing that actually arbitrates, because the claim barrier does not:
	// handleUpdate reaches it through tryBeginServerUpdate, and the two never
	// inspect each other's state.
	//
	// Released on every exit below except a successful handoff, where the process
	// is about to go down and clearing it would let a second attempt start
	// mid-shutdown.
	if !d.updating.CompareAndSwap(false, true) {
		d.logger.Debug("auto-reload: skip — update already in progress")
		return
	}
	released := false
	defer func() {
		if !released {
			d.updating.Store(false)
		}
	}()

	// Probe the binary the restart would actually exec, not os.Executable():
	// under brew those differ, and following the stable symlink is the whole
	// point of restartTargetBinary.
	target, err := d.restartTargetBinary()
	if err != nil {
		d.logger.Warn("auto-reload: could not resolve own executable — will retry", "error", err)
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, selfReloadProbeTimeout)
	defer cancel()
	onDisk, err := detectSelfVersion(probeCtx, target)
	if err != nil {
		d.logger.Warn("auto-reload: version probe failed — will retry, not treating it as a change",
			"binary", target, "error", err)
		return
	}
	// Either side blank makes the comparison meaningless, and acting on it would
	// be worse than doing nothing: a blank on-disk version is a probe that
	// technically exited 0 without telling us anything, and a blank running
	// version would restart into a successor that reads blank too — a reload
	// every tick, forever.
	if onDisk == "" || d.cfg.CLIVersion == "" {
		d.logger.Warn("auto-reload: version unavailable — will retry, not treating it as a change",
			"binary", target, "on_disk", onDisk, "running", d.cfg.CLIVersion)
		return
	}
	if onDisk == d.cfg.CLIVersion {
		d.clearReloadPending()
		return
	}

	reason := fmt.Sprintf("multica binary on disk reports %s, running %s", onDisk, d.cfg.CLIVersion)
	d.setReloadPending(reason)

	if !d.trySetClaimBarrier() {
		d.logger.Info("auto-reload: deferring — task or claim in flight at barrier check", "reason", reason)
		return
	}
	barrierReleased := false
	defer func() {
		if !barrierReleased {
			d.releaseClaimBarrier()
		}
	}()

	d.logger.Info("auto-reload: restarting into the binary on disk", "reason", reason, "binary", target)
	if !d.triggerRestart() {
		// Resolution failed at the handoff. Both deferred restores run: a failed
		// restart must never cost the daemon its ability to claim, nor leave the
		// update flag held against a restart that is not coming. The next tick
		// re-probes from scratch, so there is no stale baseline to re-detect.
		return
	}
	released = true
	barrierReleased = true
}

// setReloadPending records that a multica version change is confirmed but the
// restart is waiting for the daemon to go idle. Purely diagnostic — surfaced on
// /health and `daemon status` — and it gates nothing, so it can never park the
// daemon the way a pending-state machine could.
func (d *Daemon) setReloadPending(reason string) {
	d.reloadPendingReason.Store(&reason)
}

// clearReloadPending drops the note once the on-disk version matches again
// (the operator reverted the binary before the daemon went idle).
func (d *Daemon) clearReloadPending() {
	d.reloadPendingReason.Store(nil)
}

// reloadPending reports the deferred-restart reason, empty when none.
func (d *Daemon) reloadPending() string {
	if reason := d.reloadPendingReason.Load(); reason != nil {
		return *reason
	}
	return ""
}
