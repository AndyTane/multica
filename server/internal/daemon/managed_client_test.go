package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/managedkit"
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

func TestManagedDownloadNeverRedirectsCredentials(t *testing.T) {
	var externalCalls atomic.Int32
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { externalCalls.Add(1) }))
	defer other.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-managed-test" {
			t.Error("missing existing daemon authentication")
		}
		if r.URL.Path != "/api/daemon/client-package" {
			t.Error("uncontrolled download path")
		}
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer source.Close()
	d := &Daemon{cfg: Config{ServerBaseURL: source.URL}, client: NewClient(source.URL)}
	d.client.bundleClient = source.Client()
	d.client.SetToken("synthetic-managed-test")
	o := managedkit.Offer{Version: "v0.2.11", Commit: strings.Repeat("a", 40), Platform: "windows-amd64", SHA256: strings.Repeat("a", 64), Bytes: 1}
	if err := d.downloadManagedClient(context.Background(), "runtime", o, filepath.Join(t.TempDir(), "package.zip")); err == nil {
		t.Fatal("redirect accepted")
	}
	if externalCalls.Load() != 0 {
		t.Fatal("daemon token forwarded to redirect target")
	}
}
func TestManagedDownloadValidatesBytes(t *testing.T) {
	data := []byte("synthetic archive")
	sum := sha256.Sum256(data)
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer source.Close()
	d := &Daemon{cfg: Config{ServerBaseURL: source.URL}, client: NewClient(source.URL)}
	d.client.bundleClient = source.Client()
	o := managedkit.Offer{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}
	path := filepath.Join(t.TempDir(), "package.zip")
	if err := d.downloadManagedClient(context.Background(), "runtime", o, path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(data) {
		t.Fatal("downloaded bytes changed")
	}
	o.Bytes++
	if err := d.downloadManagedClient(context.Background(), "runtime", o, filepath.Join(t.TempDir(), "bad-size.zip")); err == nil {
		t.Fatal("wrong size accepted")
	}
	o.Bytes--
	o.SHA256 = strings.Repeat("0", 64)
	if err := d.downloadManagedClient(context.Background(), "runtime", o, filepath.Join(t.TempDir(), "bad-hash.zip")); err == nil {
		t.Fatal("wrong hash accepted")
	}
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
