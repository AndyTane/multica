package managedkit

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func fixture(t *testing.T, change func(map[string][]byte, *Inventory)) (string, Offer) {
	t.Helper()
	files := map[string][]byte{}
	for name := range programFiles {
		files[name] = []byte("test program")
	}
	files["connection.json"] = []byte(`{"profile":"","serverUrl":"","rootCa":""}`)
	o := Offer{Version: "v0.2.11", Commit: strings.Repeat("a", 40), Platform: "windows-amd64"}
	inv := Inventory{SchemaVersion: 2, Version: o.Version, Commit: o.Commit, Platform: o.Platform}
	if change != nil {
		change(files, &inv)
	}
	for name, b := range files {
		if programFiles[name] {
			inv.Files = append(inv.Files, File{Path: name, Bytes: int64(len(b)), SHA256: hash(b)})
		}
	}
	files["CLIENT-FILES.json"], _ = json.Marshal(inv)
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range files {
		w, _ := z.Create(name)
		_, _ = w.Write(data)
	}
	_ = z.Close()
	o.Bytes = int64(b.Len())
	o.SHA256 = hash(b.Bytes())
	archive := filepath.Join(t.TempDir(), "client.zip")
	_ = os.WriteFile(archive, b.Bytes(), 0600)
	return archive, o
}
func TestExtractReleaseBoundary(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string][]byte, *Inventory)
		bad    bool
	}{
		{"valid", nil, false},
		{"traversal", func(f map[string][]byte, i *Inventory) { f["../owned.exe"] = []byte("bad") }, true},
		{"absolute", func(f map[string][]byte, i *Inventory) { f["C:/owned.exe"] = []byte("bad") }, true},
		{"backslash", func(f map[string][]byte, i *Inventory) { f[`..\owned.exe`] = []byte("bad") }, true},
		{"runtime data", func(f map[string][]byte, i *Inventory) { f["state/login.json"] = []byte("bad") }, true},
		{"wrong platform", func(f map[string][]byte, i *Inventory) { i.Platform = "windows-arm64" }, true},
		{"wrong source", func(f map[string][]byte, i *Inventory) { i.Commit = strings.Repeat("b", 40) }, true},
		{"wrong version", func(f map[string][]byte, i *Inventory) { i.Version = "v0.2.12" }, true},
		{"different aliases", func(f map[string][]byte, i *Inventory) { f["multica.exe"] = []byte("different") }, true},
		{"saved connection", func(f map[string][]byte, i *Inventory) {
			f["connection.json"] = []byte(`{"profile":"user","serverUrl":"","rootCa":""}`)
		}, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			a, o := fixture(t, tt.change)
			err := Extract(a, filepath.Join(t.TempDir(), "program"), o)
			if (err != nil) != tt.bad {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
func TestHashAndSize(t *testing.T) {
	a, o := fixture(t, nil)
	for _, kind := range []string{"hash", "size", "architecture"} {
		t.Run(kind, func(t *testing.T) {
			bad := o
			switch kind {
			case "hash":
				bad.SHA256 = strings.Repeat("0", 64)
			case "size":
				bad.Bytes++
			case "architecture":
				bad.Platform = "linux-amd64"
			}
			if Extract(a, filepath.Join(t.TempDir(), "p"), bad) == nil {
				t.Fatal("invalid package accepted")
			}
		})
	}
}
func TestActiveOfferRequiresInstalledBuild(t *testing.T) {
	a, o := fixture(t, nil)
	root := t.TempDir()
	dir := filepath.Join(root, o.Version)
	_ = os.MkdirAll(dir, 0700)
	b, _ := os.ReadFile(a)
	_ = os.WriteFile(filepath.Join(dir, "client-kit.zip"), b, 0600)
	if ActiveOffer(root, o.Version, o.Commit) != nil {
		t.Fatal("activated before readiness")
	}
	_ = WriteJSON(filepath.Join(dir, "ready.json"), o)
	if ActiveOffer(root, o.Version, o.Commit[:9]) == nil {
		t.Fatal("ready build missing")
	}
	for _, v := range []string{"v0.2.10", "dev", "v0.2.11-dirty"} {
		if ActiveOffer(root, v, o.Commit) != nil {
			t.Fatal("offered across rollback/dev boundary")
		}
	}
	if ActiveOffer(root, o.Version, strings.Repeat("b", 9)) != nil {
		t.Fatal("source mismatch accepted")
	}
}
func TestRetryAndProgramIsolation(t *testing.T) {
	_, o := fixture(t, nil)
	j := Journal{Offer: o, Attempts: 3, Phase: "failed"}
	if CanAttempt(j, o, time.Now()) {
		t.Fatal("unbounded retries")
	}
	j.Attempts = 1
	j.RetryAfter = time.Now().Add(time.Hour)
	if CanAttempt(j, o, time.Now()) {
		t.Fatal("backoff ignored")
	}
	j.RetryAfter = time.Time{}
	for _, phase := range []string{"switching", "verifying", "completed", "interrupted"} {
		j.Phase = phase
		if CanAttempt(j, o, time.Now()) {
			t.Fatal("duplicate install")
		}
	}
	for _, p := range []string{"../other", "programs/../../escape", `programs\..\escape`, `C:\other`, "programs/.."} {
		if _, err := ProgramPath(t.TempDir(), p); err == nil {
			t.Fatalf("accepted %q", p)
		}
	}
}
func TestHelperLockSurvivesStaleFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	_ = os.WriteFile(path, []byte("stale owner"), 0600)
	unlock, err := acquireHelperLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if release, err := acquireHelperLock(path); err == nil {
		release()
		t.Fatal("two helpers acquired same installation")
	}
	unlock()
	unlock, err = acquireHelperLock(path)
	if err != nil {
		t.Fatal("lock was not released", err)
	}
	unlock()
}
