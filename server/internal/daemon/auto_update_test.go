package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAutoUpdateLoopNeverFetchesReleases(t *testing.T) {
	var requests atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"tag_name":"v99.0.0"}`))
	}))
	defer source.Close()
	t.Setenv("MULTICA_RELEASE_API_BASE_URL", source.URL)
	t.Setenv("MULTICA_DAEMON_AUTO_UPDATE", "true")
	shortenLoopTimers(t)
	for _, reload := range []bool{false, true} {
		d, _ := newSelfReloadTestDaemon(t, "v1.0.0")
		d.cfg.AutoUpdateEnabled = true
		d.cfg.AutoReloadEnabled = reload
		d.cfg.AutoUpdateCheckInterval = time.Millisecond
		d.runUpdateFn = func(string) (string, error) { t.Error("automatic installer invoked"); return "", nil }
		stubSelfVersion(t, "v1.0.0", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		d.autoUpdateLoop(ctx)
		cancel()
	}
	if requests.Load() != 0 {
		t.Fatalf("automatic release requests = %d", requests.Load())
	}
}

func TestTryEnterClaim_RespectsBarrier(t *testing.T) {
	d := &Daemon{}

	if !d.tryEnterClaim() {
		t.Fatal("tryEnterClaim should succeed when barrier is unset")
	}
	d.exitClaim()
	if d.claimsInFlight != 0 {
		t.Fatalf("claimsInFlight not balanced: %d", d.claimsInFlight)
	}

	if !d.trySetClaimBarrier() {
		t.Fatal("trySetClaimBarrier should succeed when idle")
	}
	if d.tryEnterClaim() {
		t.Fatal("tryEnterClaim must refuse while barrier is held")
	}
	d.releaseClaimBarrier()
	if !d.tryEnterClaim() {
		t.Fatal("tryEnterClaim should succeed after barrier release")
	}
	d.exitClaim()
}
