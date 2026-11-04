package daemon

import (
	"context"
	"testing"
	"time"
)

func TestManagedUpdateDrainsClaimHandoff(t *testing.T) {
	d := &Daemon{}
	if !d.tryEnterClaim() {
		t.Fatal("initial claim rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- d.drainManagedClaims(ctx, func() bool { return false }) }()
	deadline := time.Now().Add(time.Second)
	for !claimsPaused(t, d) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if d.tryEnterClaim() {
		d.exitClaim()
		t.Fatal("new work entered during switch")
	}
	d.activeTasks.Add(1)
	d.exitClaim()
	select {
	case <-done:
		t.Fatal("claim became active task but switch continued")
	case <-time.After(150 * time.Millisecond):
	}
	d.activeTasks.Add(-1)
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("idle drain canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("idle drain stuck")
	}
	if !claimsPaused(t, d) {
		t.Fatal("switch released claim barrier")
	}
	d.releaseClaimBarrier()
	if !d.tryEnterClaim() {
		t.Fatal("failure did not resume claims")
	}
	d.exitClaim()
}
func TestManagedUpdatePauseAndCancellation(t *testing.T) {
	for _, paused := range []bool{false, true} {
		d := &Daemon{}
		d.activeTasks.Store(1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if d.drainManagedClaims(ctx, func() bool { return paused }) {
			t.Fatal("canceled or paused update proceeded")
		}
		d.releaseClaimBarrier()
		if claimsPaused(t, d) {
			t.Fatal("failed update parked daemon")
		}
	}
}
