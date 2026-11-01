//go:build secondmachine_e2e

package secondmachine

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/daemon"
)

const e2eToken = "mul_e2e_token"

// ---- fake deployment -------------------------------------------------------

// fakeDeployment is a private-CA HTTPS server that answers the subset of the
// platform API the CLI and daemon use while connecting and registering.
type fakeDeployment struct {
	server *httptest.Server
	caPath string

	mu              sync.Mutex
	registrations   []registerCall
	deregistrations int
}

type registerCall struct {
	workspaceID string
	daemonID    string
	deviceName  string
	providers   []string
}

func newFakeDeployment(t *testing.T) *fakeDeployment {
	t.Helper()
	caCert, caKey := mintCA(t)
	leaf := mintServerCert(t, caCert, caKey)
	fake := &fakeDeployment{}

	workspaces := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"ws-e2e","name":"U Workers CI"}]`))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+e2eToken {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Tane","email":"tane@example.com"}`))
	})
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, _ *http.Request) { workspaces(w) })
	mux.HandleFunc("/api/daemon/workspaces", func(w http.ResponseWriter, _ *http.Request) { workspaces(w) })
	mux.HandleFunc("/api/tokens/current/renew", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"renewed":false}`))
	})
	mux.HandleFunc("/api/daemon/register", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			WorkspaceID string              `json:"workspace_id"`
			DaemonID    string              `json:"daemon_id"`
			DeviceName  string              `json:"device_name"`
			Runtimes    []map[string]string `json:"runtimes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		call := registerCall{workspaceID: body.WorkspaceID, daemonID: body.DaemonID, deviceName: body.DeviceName}
		resp := map[string]any{}
		var runtimes []map[string]any
		for i, rt := range body.Runtimes {
			call.providers = append(call.providers, rt["type"])
			runtimes = append(runtimes, map[string]any{
				"id": fmt.Sprintf("rt-%d", i+1), "name": rt["name"], "provider": rt["type"], "status": "online",
			})
		}
		resp["runtimes"] = runtimes
		resp["repos"] = []any{}
		resp["repos_version"] = ""
		fake.mu.Lock()
		fake.registrations = append(fake.registrations, call)
		fake.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/daemon/deregister", func(w http.ResponseWriter, _ *http.Request) {
		fake.mu.Lock()
		fake.deregistrations++
		fake.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Everything else the daemon may probe while idle: a benign empty answer.
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})

	fake.server = httptest.NewUnstartedServer(mux)
	fake.server.TLS = &tls.Config{Certificates: []tls.Certificate{leaf}}
	fake.server.StartTLS()
	t.Cleanup(fake.server.Close)

	caPath := filepath.Join(t.TempDir(), "root-ca.crt")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		t.Fatalf("write root CA: %v", err)
	}
	fake.caPath = caPath
	return fake
}

func (f *fakeDeployment) url() string { return f.server.URL }

func (f *fakeDeployment) registrationCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.registrations)
}

func (f *fakeDeployment) lastRegistration() (registerCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.registrations) == 0 {
		return registerCall{}, false
	}
	return f.registrations[len(f.registrations)-1], true
}

// ---- certificates ----------------------------------------------------------

func mintCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "U Workers second-machine e2e CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	return cert, key
}

func mintServerCert(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// ---- helpers ---------------------------------------------------------------

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve the test file path")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
}

// setupInstaller returns the single-file installer, either the one an operator
// already built (U_WORKERS_E2E_SETUP_EXE) or a fresh build from the repo script.
func setupInstaller(t *testing.T) string {
	t.Helper()
	if override := strings.TrimSpace(os.Getenv("U_WORKERS_E2E_SETUP_EXE")); override != "" {
		if _, err := os.Stat(override); err != nil {
			t.Fatalf("U_WORKERS_E2E_SETUP_EXE: %v", err)
		}
		return override
	}
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "build-windows-installer.ps1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-File", script, "-Version", "e2e")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build installer: %v\n%s", err, out)
	}
	exe := filepath.Join(root, "dist", "u-workers-setup-e2e-windows-amd64.exe")
	if _, statErr := os.Stat(exe); statErr != nil {
		t.Fatalf("installer not found after build: %v\n%s", statErr, out)
	}
	return exe
}

// fakeAgentSource answers --version like the real codex CLI, which is all the
// daemon probes before registering the runtime.
const fakeAgentSource = `package main

import (
	"fmt"
	"os"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" || arg == "version" {
			fmt.Println("codex-cli 9.9.9")
			return
		}
	}
}
`

func buildFakeAgent(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "fakecodex.go")
	if err := os.WriteFile(src, []byte(fakeAgentSource), 0o644); err != nil {
		t.Fatalf("write fake agent: %v", err)
	}
	exe := filepath.Join(dir, "fake-codex.exe")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", exe, src)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake agent: %v\n%s", err, out)
	}
	return exe
}

// cleanEnvironment builds an environment that looks like a fresh Windows user
// profile: a private home, no inherited product configuration, and nothing that
// makes the CLI believe it runs inside a platform task.
func cleanEnvironment(t *testing.T, home string, extra map[string]string) []string {
	t.Helper()
	dropPrefixes := []string{"MULTICA_", "U_WORKERS_"}
	dropExact := map[string]bool{
		"HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true,
		"XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true, "XDG_DATA_HOME": true,
	}
	var env []string
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		upper := strings.ToUpper(key)
		if dropExact[upper] {
			continue
		}
		skip := false
		for _, prefix := range dropPrefixes {
			if strings.HasPrefix(upper, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		"USERPROFILE="+home,
		"HOME="+home,
		"LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"),
		"APPDATA="+filepath.Join(home, "AppData", "Roaming"),
	)
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}

// runProcess runs one command with a deadline and returns combined output.
func runProcess(t *testing.T, dir string, env []string, timeout time.Duration, name string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		code = 1
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			code = exitErr.ExitCode()
		}
	}
	return string(out), code
}

func asExitError(err error, target **exec.ExitError) bool {
	for err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			*target = exitErr
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hashPort(profile string) int {
	h := 0
	for _, b := range []byte(profile) {
		h += int(b)
	}
	return daemon.DefaultHealthPort + 1 + (h % 1000)
}

func daemonHealth(t *testing.T, profile string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/health", hashPort(profile)), nil)
	if err != nil {
		t.Fatalf("build health request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	return payload
}

func healthRuntimeCount(health map[string]any) int {
	if health == nil {
		return 0
	}
	workspaces, _ := health["workspaces"].([]any)
	count := 0
	for _, item := range workspaces {
		ws, _ := item.(map[string]any)
		runtimes, _ := ws["runtimes"].([]any)
		count += len(runtimes)
	}
	return count
}

func stopDaemon(t *testing.T, cliExe, home, profile string, env []string) {
	t.Helper()
	out, code := runProcess(t, home, env, time.Minute, cliExe, "--profile", profile, "daemon", "stop")
	if code != 0 {
		t.Logf("daemon stop returned %d: %s", code, out)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if daemonHealth(t, profile) == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	pidPath := filepath.Join(home, ".u-workers", "profiles", profile, "daemon.pid")
	if data, err := os.ReadFile(pidPath); err == nil {
		t.Logf("daemon still answering; pid file %s = %s", pidPath, strings.TrimSpace(string(data)))
	}
}

// TestSecondMachinePackageFlow is the acceptance check for CLAWSTA-104: a clean
// Windows user profile gets the single-file installer, runs one connect command
// against a private-CA HTTPS deployment, and the daemon registers a runtime.
func TestSecondMachinePackageFlow(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the second-machine package is Windows-only")
	}
	t.Parallel()

	setupExe := setupInstaller(t)
	fake := newFakeDeployment(t)

	home := t.TempDir()
	profile := fmt.Sprintf("e2e%08x", time.Now().UnixNano()&0xffffffff)
	agentPath := buildFakeAgent(t, t.TempDir())
	env := cleanEnvironment(t, home, map[string]string{"MULTICA_CODEX_PATH": agentPath})
	installDir := filepath.Join(home, "bin")

	// Step 1: install the package into a clean profile.
	out, code := runProcess(t, home, env, 2*time.Minute, setupExe, "--install-dir", installDir, "--no-path", "--no-connect", "--silent")
	if code != 0 {
		t.Fatalf("installer failed with %d:\n%s", code, out)
	}
	cliExe := filepath.Join(installDir, "u-workers.exe")
	legacyExe := filepath.Join(installDir, "multica.exe")
	for _, exe := range []string{cliExe, legacyExe} {
		if _, err := os.Stat(exe); err != nil {
			t.Fatalf("installer did not install %s: %v", exe, err)
		}
	}
	if out, code := runProcess(t, home, env, time.Minute, cliExe, "version"); code != 0 || !strings.Contains(out, "u-workers") {
		t.Fatalf("installed u-workers version failed (%d):\n%s", code, out)
	}
	if out, code := runProcess(t, home, env, time.Minute, legacyExe, "version"); code != 0 || !strings.Contains(out, "multica") {
		t.Fatalf("legacy multica command failed (%d):\n%s", code, out)
	}

	// Step 2: the one command that connects, trusts the private CA, signs in
	// and starts the daemon.
	// Best-effort teardown registered before the daemon can start: a failed
	// assertion after this point must never leave a daemon behind.
	t.Cleanup(func() { stopDaemon(t, cliExe, home, profile, env) })
	connectOut, code := runProcess(t, home, env, 3*time.Minute, cliExe,
		"--profile", profile,
		"connect",
		"--server-url", fake.url(),
		"--root-ca", fake.caPath,
		"--token", e2eToken,
		"--skip-browser-trust",
	)
	if code != 0 {
		t.Fatalf("connect failed with %d:\n%s", code, connectOut)
	}
	if !strings.Contains(connectOut, "Connected to") {
		t.Fatalf("connect did not report success:\n%s", connectOut)
	}

	// The board-visible proof: the fake deployment received a runtime
	// registration for this machine.
	deadline := time.Now().Add(30 * time.Second)
	var registration registerCall
	found := false
	for time.Now().Before(deadline) {
		if call, ok := fake.lastRegistration(); ok {
			registration, found = call, true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !found {
		t.Fatalf("the daemon never registered a runtime with the server; connect output:\n%s", connectOut)
	}
	if registration.workspaceID != "ws-e2e" {
		t.Fatalf("registered workspace = %q, want ws-e2e", registration.workspaceID)
	}
	if registration.deviceName == "" {
		t.Fatal("registration carried no device name; the board row would be unnamed")
	}
	if !containsString(registration.providers, "codex") {
		t.Fatalf("registered providers = %v, want the discovered codex runtime among them", registration.providers)
	}

	// Daemon health agrees: running, with one runtime in its workspace.
	health := daemonHealth(t, profile)
	if health == nil {
		t.Fatal("daemon health endpoint is not answering after connect")
	}
	if status, _ := health["status"].(string); status != "running" {
		t.Fatalf("daemon status = %q, want running", status)
	}
	if got := healthRuntimeCount(health); got < 1 || got != len(registration.providers) {
		t.Fatalf("daemon reports %d runtimes, want one per registered provider (%d)", got, len(registration.providers))
	}

	// The connection is persisted: a later command reuses the saved private CA.
	cfgPath := filepath.Join(home, ".u-workers", "profiles", profile, "config.json")
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read saved config %s: %v", cfgPath, err)
	}
	var saved map[string]any
	if err := json.Unmarshal(cfgBytes, &saved); err != nil {
		t.Fatalf("parse saved config: %v", err)
	}
	if saved["server_url"] != fake.url() {
		t.Fatalf("saved server_url = %v, want %s", saved["server_url"], fake.url())
	}
	if saved["root_ca_file"] == "" || saved["root_ca_file"] == nil {
		t.Fatalf("root_ca_file was not persisted: %v", saved["root_ca_file"])
	}
	if saved["token"] != e2eToken {
		t.Fatal("token was not persisted for the daemon to use")
	}

	// A second command that talks to the server uses the persisted CA without
	// any --root-ca flag: this is the "install once, use `u-workers`" contract.
	authOut, code := runProcess(t, home, env, time.Minute, cliExe, "--profile", profile, "auth", "status")
	if code != 0 {
		t.Fatalf("auth status with the persisted root CA failed (%d):\n%s", code, authOut)
	}
	if !strings.Contains(authOut, "Tane") {
		t.Fatalf("auth status did not reach the server:\n%s", authOut)
	}
}

// TestSecondMachineRejectsUntrustedCA proves the connect path keeps certificate
// verification on: without --root-ca the private-CA deployment is unreachable
// and no configuration is written.
func TestSecondMachineRejectsUntrustedCA(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the second-machine package is Windows-only")
	}
	t.Parallel()

	setupExe := setupInstaller(t)
	fake := newFakeDeployment(t)
	home := t.TempDir()
	profile := fmt.Sprintf("e2e%08x", time.Now().UnixNano()&0xffffffff)
	env := cleanEnvironment(t, home, nil)
	installDir := filepath.Join(home, "bin")
	if out, code := runProcess(t, home, env, 2*time.Minute, setupExe, "--install-dir", installDir, "--no-path", "--no-connect", "--silent"); code != 0 {
		t.Fatalf("installer failed with %d:\n%s", code, out)
	}
	cliExe := filepath.Join(installDir, "u-workers.exe")
	out, code := runProcess(t, home, env, time.Minute, cliExe,
		"--profile", profile,
		"connect",
		"--server-url", fake.url(),
		"--token", e2eToken,
		"--skip-browser-trust",
	)
	if code == 0 {
		t.Fatalf("connect must fail without the private root CA:\n%s", out)
	}
	if !strings.Contains(out, "root-ca") && !strings.Contains(out, "certificate") {
		t.Fatalf("connect failure does not point at the certificate:\n%s", out)
	}
	cfgPath := filepath.Join(home, ".u-workers", "profiles", profile, "config.json")
	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatalf("a failed connect must not persist configuration, found %s", cfgPath)
	}
}
