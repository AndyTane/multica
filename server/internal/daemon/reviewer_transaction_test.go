package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/managedkit"
)

// Retains the reviewer's copied-executable regression. The helper completion
// boundary and pinned executable inventory are now part of the handshake.
// No service, account, installed CLI or network is used by these tests.
func TestReviewVerifiedSuccessorWithoutCurrentOffer(t *testing.T) {
	if root := os.Getenv("REVIEW_TRANSACTION_ROOT"); root == "" {
		root = t.TempDir()
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(self)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "u-workers.exe")
		if err = os.WriteFile(target, data, 0700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(target, "-test.run=^TestReviewVerifiedSuccessorWithoutCurrentOffer$", "-test.v")
		cmd.Env = append(os.Environ(), "REVIEW_TRANSACTION_ROOT="+root)
		out, err := cmd.CombinedOutput()
		t.Log(string(out))
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	root := os.Getenv("REVIEW_TRANSACTION_ROOT")
	t.Setenv("U_WORKERS_KIT_ROOT", root)
	const profile = "review-synthetic"
	const server = "https://server.invalid"
	const installID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	offer := managedkit.Offer{Version: "v0.2.11", Commit: strings.Repeat("a", 40), Platform: "windows-amd64", SHA256: strings.Repeat("a", 64), Bytes: 1}
	installation := managedkit.Installation{ID: installID, Profile: profile, ServerURL: server, Paused: true}
	save := func(path string, value any) {
		t.Helper()
		if err := managedkit.WriteJSON(path, value); err != nil {
			t.Fatal(err)
		}
	}
	save(filepath.Join(root, "kit-install.json"), installation)
	exe, _ := os.Executable()
	data, _ := os.ReadFile(exe)
	sum := sha256.Sum256(data)
	inventory := managedkit.Inventory{SchemaVersion: 2, Version: offer.Version, Commit: offer.Commit, Platform: offer.Platform, Files: []managedkit.File{{Path: "u-workers.exe", Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}}
	save(filepath.Join(root, "CLIENT-FILES.json"), inventory)
	next := offer
	next.Version = "v0.2.12"
	next.SHA256 = strings.Repeat("b", 64)
	previous := offer
	previous.Version = "v0.2.10"
	previous.SHA256 = strings.Repeat("c", 64)
	for _, tt := range []struct {
		name     string
		incoming *managedkit.Offer
	}{{"same-offer-control", &offer}, {"cache-unavailable", nil}, {"server-rollback", &previous}, {"new-target", &next}} {
		t.Run(tt.name, func(t *testing.T) {
			j := managedkit.Journal{Offer: offer, InstallationID: installID, Phase: "verifying", Profile: profile, ServerURL: server, NewProgram: "."}
			save(managedkit.JournalPath(root), j)
			unlock, err := managedkit.AcquireUpdateLock(root)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock()
			d := &Daemon{cfg: Config{Profile: profile, ServerBaseURL: server, CLIVersion: offer.Version}, pauseClaims: true}
			d.managedVerifying.Store(true)
			d.handleManagedClient(context.Background(), "synthetic-runtime", tt.incoming)
			if !claimsPaused(t, d) || !d.managedVerifying.Load() {
				t.Fatal("claims released before helper committed completion")
			}
			var ack struct {
				SHA string `json:"sha256"`
			}
			if managedkit.ReadJSON(filepath.Join(root, ".managed", "ack.json"), &ack) != nil || ack.SHA != offer.SHA256 {
				t.Fatal("authenticated pinned successor not acknowledged")
			}
			current := managedkit.LoadJournal(root)
			if current.Phase != "verifying" || current.Offer.SHA256 != offer.SHA256 {
				t.Fatal("next offer overwrote unfinished transaction")
			}
			// The exclusive helper commits completion. The next heartbeat may release
			// claims even when its current download offer is absent or different.
			j.Phase = "completed"
			save(managedkit.JournalPath(root), j)
			d.handleManagedClient(context.Background(), "synthetic-runtime", tt.incoming)
			if !d.tryEnterClaim() {
				t.Fatal("authenticated, completed successor remains unable to claim")
			}
			d.exitClaim()
			if managedkit.LoadJournal(root).Offer.SHA256 != offer.SHA256 {
				t.Fatal("transaction replaced before helper lock release")
			}
		})
	}
	for _, phase := range []string{"switching", "verifying", "interrupted"} {
		t.Run("admission-"+phase, func(t *testing.T) {
			installation.Paused = false
			save(filepath.Join(root, "kit-install.json"), installation)
			j := managedkit.Journal{Offer: offer, InstallationID: installID, Phase: phase, Profile: profile, ServerURL: server, NewProgram: "."}
			save(managedkit.JournalPath(root), j)
			unlock, err := managedkit.AcquireUpdateLock(root)
			if err != nil {
				t.Fatal(err)
			}
			d := &Daemon{cfg: Config{Profile: profile, ServerBaseURL: server, CLIVersion: offer.Version}, pauseClaims: true}
			d.handleManagedClient(context.Background(), "synthetic-runtime", &next)
			unlock()
			// Also test the persisted handoff gap after a crash released the OS
			// lock. A fresh process CAS must not admit a different SHA here.
			d.handleManagedClient(context.Background(), "synthetic-runtime", &next)
			current := managedkit.LoadJournal(root)
			if current.Phase != phase || current.Offer.SHA256 != offer.SHA256 || !claimsPaused(t, d) {
				t.Fatal("new process/offer stole journal or another transaction's claim ownership")
			}
		})
	}
	t.Run("explicit-stop-preserves-barrier", func(t *testing.T) {
		j := managedkit.Journal{Offer: offer, InstallationID: installID, Phase: "interrupted", Profile: profile, ServerURL: server, NewProgram: "."}
		save(managedkit.JournalPath(root), j)
		_ = os.Remove(filepath.Join(root, ".managed", "ack.json"))
		stopPath := filepath.Join(root, ".managed", "stop.json")
		save(stopPath, map[string]string{"sha256": offer.SHA256})
		defer os.Remove(stopPath)
		d := &Daemon{cfg: Config{Profile: profile, ServerBaseURL: server, CLIVersion: offer.Version}, pauseClaims: true}
		d.managedVerifying.Store(true)
		d.handleManagedClient(context.Background(), "synthetic-runtime", &next)
		if !claimsPaused(t, d) || managedkit.LoadJournal(root).Phase != "interrupted" {
			t.Fatal("heartbeat overrode explicit stop")
		}
		if _, err := os.Stat(filepath.Join(root, ".managed", "ack.json")); !os.IsNotExist(err) {
			t.Fatal("stopped candidate acknowledged")
		}
	})
	t.Run("wrong-pinned-program", func(t *testing.T) {
		j := managedkit.Journal{Offer: offer, InstallationID: installID, Phase: "verifying", Profile: profile, ServerURL: server, NewProgram: "."}
		save(managedkit.JournalPath(root), j)
		inventory.Commit = strings.Repeat("f", 40)
		save(filepath.Join(root, "CLIENT-FILES.json"), inventory)
		unlock, err := managedkit.AcquireUpdateLock(root)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		_ = os.Remove(filepath.Join(root, ".managed", "ack.json"))
		d := &Daemon{cfg: Config{Profile: profile, ServerBaseURL: server, CLIVersion: offer.Version}, pauseClaims: true}
		d.managedVerifying.Store(true)
		d.handleManagedClient(context.Background(), "synthetic-runtime", nil)
		if !claimsPaused(t, d) {
			t.Fatal("unverified executable released claims")
		}
		if _, err := os.Stat(filepath.Join(root, ".managed", "ack.json")); !os.IsNotExist(err) {
			t.Fatal("unverified executable acknowledged")
		}
	})
}
