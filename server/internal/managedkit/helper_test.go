package managedkit

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/tlsx"
)

// A synthetic child reports local health but never sends an authenticated ack.
// It must be stopped before recovery, including when the helper was resumed
// without an exec.Cmd handle. No installed daemon or real account is accessed.
func TestHelperRecoversLiveUnconfirmedCandidate(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("managed Windows process identity test")
	}
	if root := os.Getenv("MANAGED_RECOVERY_TEST_ROOT"); root != "" {
		exe, _ := os.Executable()
		if filepath.Dir(exe) == root {
			_ = os.WriteFile(filepath.Join(root, "previous-restarted"), []byte("ok"), 0600)
			return
		}
		j := LoadJournal(root)
		h := localHealth{PID: os.Getpid(), Profile: j.Profile, Server: j.ServerURL, Version: j.Offer.Version, Managed: &Status{InstallationID: j.InstallationID}}
		_ = http.ListenAndServe(net.JoinHostPort("127.0.0.1", strconv.Itoa(j.HealthPort)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(h) }))
		return
	}
	if testing.Short() {
		t.Skip("child process recovery")
	}
	root := t.TempDir()
	t.Setenv("MANAGED_RECOVERY_TEST_ROOT", root)
	self, _ := os.Executable()
	newProgram := "programs/v0.2.11-aaaaaaaaaaaa-1"
	newDir, _ := ProgramPath(root, newProgram)
	_ = os.MkdirAll(newDir, 0700)
	for _, dir := range []string{root, newDir} {
		if err := CopyExecutable(self, filepath.Join(dir, "u-workers.exe")); err != nil {
			t.Fatal(err)
		}
	}
	ready := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer ready.Close()
	ca := filepath.Join(root, "public-ca.crt")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ready.Certificate().Raw}), 0600)
	t.Setenv(tlsx.RootCAEnv, ca)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	offer := Offer{Version: "v0.2.11", Commit: strings.Repeat("a", 40), Platform: "windows-amd64", SHA256: strings.Repeat("a", 64), Bytes: 1}
	i := Installation{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Profile: "synthetic", ServerURL: ready.URL}
	if err := WriteJSON(filepath.Join(root, "kit-install.json"), i); err != nil {
		t.Fatal(err)
	}
	j := Journal{Offer: offer, InstallationID: i.ID, Profile: i.Profile, ServerURL: i.ServerURL, Phase: "verifying", Attempts: 1, OldProgram: ".", NewProgram: newProgram, OldPID: -1, HealthPort: port, RootCAFile: ca, Args: []string{"-test.run=^TestHelperRecoversLiveUnconfirmedCandidate$"}}
	if err := SaveJournal(root, j); err != nil {
		t.Fatal(err)
	}
	// Start the successor separately so this tests recovery of an existing live
	// process rather than only the helper's immediate child failure path.
	child, err := Spawn(filepath.Join(newDir, "u-workers.exe"), j.Args, root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for probeHealth(port).PID == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if probeHealth(port).PID != child.Process.Pid {
		t.Fatal("synthetic child health unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := runHelper(ctx, root, time.Second); err != nil {
		t.Fatal(err)
	}
	got := LoadJournal(root)
	if got.Phase != "rolled_back" || CurrentProgram(root) != "." || probeHealth(port).PID != 0 {
		t.Fatal("live unconfirmed candidate not recovered", got.Phase)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(root, "previous-restarted")); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("previous program was not restarted")
}
