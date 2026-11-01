package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/tlsx"
)

// newConnectTestCommand builds a standalone connect command so tests can set
// its local flags without leaking changed-flag state between cases.
func newConnectTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "connect", RunE: runConnect}
	cmd.Flags().String("server-url", "", "")
	cmd.Flags().String("app-url", "", "")
	cmd.Flags().String("root-ca", "", "")
	cmd.Flags().Bool("skip-browser-trust", false, "")
	cmd.Flags().Bool("no-daemon", false, "")
	cmd.Flags().String("token", "", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String(callbackHostFlag, "", "")
	cmd.Flags().Lookup("token").NoOptDefVal = tokenPromptSentinel
	return cmd
}

// isolateConnectHome points the CLI config root at a fresh user home and clears
// any ambient trust configuration, so a test never reads or writes the real
// user's credentials.
func isolateConnectHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	// Run from a directory with no daemon-task marker, and clear the task
	// environment: this test process may itself be running inside a Multica
	// task, where human-local commands are refused by design.
	t.Chdir(t.TempDir())
	for _, key := range []string{"MULTICA_AGENT_ID", "MULTICA_TASK_ID", "MULTICA_DAEMON_PORT", "MULTICA_TASK_CONFIG_ROOT", "U_WORKERS_TASK_CONFIG_ROOT"} {
		t.Setenv(key, "")
	}
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("MULTICA_APP_URL", "")
	t.Setenv("MULTICA_SERVER_URL", "")
	t.Setenv(tlsx.RootCAEnv, "")
	if err := tlsx.InstallFromEnv(); err != nil {
		t.Fatalf("reset root CA state: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(tlsx.RootCAEnv)
		_ = tlsx.InstallFromEnv()
	})
	return home
}

// fakeDeployment is a TLS test server that stands in for a LAN self-hosted
// deployment: its leaf certificate is issued by a private CA, exactly like the
// Caddy local CA in deploy/windows-https.
type fakeDeployment struct {
	server *httptest.Server
	caPath string
}

func newFakeDeployment(t *testing.T) *fakeDeployment {
	t.Helper()
	dir := t.TempDir()
	caCert, caKey := mintCA(t, "U Workers test root CA")
	leaf := mintServerCert(t, caCert, caKey)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mul_test_token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"Tane","email":"tane@example.com"}`))
	})
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":"ws-1","name":"U Workers"}]`))
	})

	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{leaf}}
	server.StartTLS()
	t.Cleanup(server.Close)

	caPath := filepath.Join(dir, "root-ca.crt")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		t.Fatalf("write root CA: %v", err)
	}
	return &fakeDeployment{server: server, caPath: caPath}
}

func mintCA(t *testing.T, commonName string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
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

// TestConnectWithRootCAAndToken covers the whole second-machine configure path
// without a browser and without starting a daemon: verify the private CA, save
// both URLs plus the trust anchor, and sign in with a PAT.
func TestConnectWithRootCAAndToken(t *testing.T) {
	isolateConnectHome(t)
	deployment := newFakeDeployment(t)

	cmd := newConnectTestCommand()
	mustSetFlag(t, cmd, "server-url", deployment.server.URL)
	mustSetFlag(t, cmd, "root-ca", deployment.caPath)
	mustSetFlag(t, cmd, "token", "mul_test_token")
	mustSetFlag(t, cmd, "no-daemon", "true")
	mustSetFlag(t, cmd, "skip-browser-trust", "true")

	if err := runConnect(cmd, nil); err != nil {
		t.Fatalf("runConnect: %v", err)
	}

	cfg, err := cli.LoadCLIConfigForProfile("")
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if cfg.ServerURL != deployment.server.URL {
		t.Fatalf("server_url = %q, want %q", cfg.ServerURL, deployment.server.URL)
	}
	if cfg.AppURL != deployment.server.URL {
		t.Fatalf("app_url = %q, want the same-origin https default %q", cfg.AppURL, deployment.server.URL)
	}
	if cfg.Token != "mul_test_token" {
		t.Fatalf("token not saved")
	}
	if cfg.WorkspaceID != "ws-1" {
		t.Fatalf("workspace_id = %q, want the discovered workspace", cfg.WorkspaceID)
	}
	wantTrustPath, err := cli.RootCAFilePath("")
	if err != nil {
		t.Fatalf("root CA path: %v", err)
	}
	if cfg.RootCAFile != wantTrustPath {
		t.Fatalf("root_ca_file = %q, want %q", cfg.RootCAFile, wantTrustPath)
	}
	trusted, err := os.ReadFile(wantTrustPath)
	if err != nil {
		t.Fatalf("read stored root CA: %v", err)
	}
	if !strings.Contains(string(trusted), "BEGIN CERTIFICATE") {
		t.Fatal("stored root CA is not a PEM certificate")
	}
	if tlsx.Pool() == nil {
		t.Fatal("the connected process should trust the root CA after connect")
	}
}

// TestConnectRejectsUnrelatedRootCA pins the security behavior: a certificate
// that did not sign the server chain is rejected before anything is persisted.
func TestConnectRejectsUnrelatedRootCA(t *testing.T) {
	isolateConnectHome(t)
	deployment := newFakeDeployment(t)

	unrelatedCA, _ := mintCA(t, "Unrelated test CA")
	unrelatedPath := filepath.Join(t.TempDir(), "unrelated.crt")
	if err := os.WriteFile(unrelatedPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: unrelatedCA.Raw}), 0o644); err != nil {
		t.Fatalf("write unrelated CA: %v", err)
	}

	cmd := newConnectTestCommand()
	mustSetFlag(t, cmd, "server-url", deployment.server.URL)
	mustSetFlag(t, cmd, "root-ca", unrelatedPath)
	mustSetFlag(t, cmd, "token", "mul_test_token")
	mustSetFlag(t, cmd, "no-daemon", "true")
	mustSetFlag(t, cmd, "skip-browser-trust", "true")

	err := runConnect(cmd, nil)
	if err == nil {
		t.Fatal("connect must reject a root CA that did not issue the server certificate")
	}
	if !strings.Contains(err.Error(), "not issued by") {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg, _ := cli.LoadCLIConfigForProfile("")
	if cfg.RootCAFile != "" {
		t.Fatalf("a rejected root CA must not be persisted, got %q", cfg.RootCAFile)
	}
}

func TestSameOriginAppURL(t *testing.T) {
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"https with port", "https://192.168.3.49:13443", "https://192.168.3.49:13443", true},
		{"https default port", "https://uworkers.example", "https://uworkers.example", true},
		{"plain http cannot be derived", "http://192.168.3.49:13001", "", false},
		{"garbage", "not a url", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := sameOriginAppURL(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("sameOriginAppURL(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestDaemonRegisteredRuntimes(t *testing.T) {
	health := map[string]any{
		"workspaces": []any{
			map[string]any{"id": "ws-1", "runtimes": []any{"rt-a"}},
			map[string]any{"id": "ws-2", "runtimes": []any{"rt-b", "rt-c"}},
		},
	}
	got := daemonRegisteredRuntimes(health)
	if len(got) != 3 {
		t.Fatalf("daemonRegisteredRuntimes = %v, want three runtimes", got)
	}
	if got := daemonRegisteredRuntimes(map[string]any{}); got != nil {
		t.Fatalf("empty health should report no runtimes, got %v", got)
	}
}

func TestConnectRequiresAppURLForPlainHTTP(t *testing.T) {
	isolateConnectHome(t)
	cmd := newConnectTestCommand()
	mustSetFlag(t, cmd, "server-url", "http://127.0.0.1:9")
	mustSetFlag(t, cmd, "no-daemon", "true")
	err := runConnect(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "web app URL") {
		t.Fatalf("want a web app URL error for a plain-HTTP server without --app-url, got %v", err)
	}
}

func mustSetFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set --%s: %v", name, err)
	}
}

// TestReview107RejectCAOverHTTP pins that a plain-HTTP server URL cannot turn an
// arbitrary root CA into a trusted, persisted certificate: there is no server
// certificate to verify against.
func TestReview107RejectCAOverHTTP(t *testing.T) {
	isolateConnectHome(t)
	deployment := newFakeDeployment(t)
	anchors, err := loadConnectRootCA(deployment.caPath)
	if err != nil {
		t.Fatal(err)
	}
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer plain.Close()
	if err := verifyServerTrustsCA(plain.URL, anchors); err == nil {
		t.Fatal("accepted an arbitrary CA as verified against an HTTP-only server")
	}
}

// TestReview107ReconnectUsesSavedTrust pins that the reachability check before a
// second connect uses the CA already saved by the first connect (the operator
// chose to skip the browser trust), not only the system pool.
func TestReview107ReconnectUsesSavedTrust(t *testing.T) {
	isolateConnectHome(t)
	deployment := newFakeDeployment(t)
	t.Setenv(tlsx.RootCAEnv, deployment.caPath)
	if err := tlsx.InstallFromEnv(); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: tlsx.HTTPTransport()}
	resp, err := client.Get(deployment.server.URL + "/health")
	if err != nil {
		t.Fatalf("control: configured trust failed: %v", err)
	}
	resp.Body.Close()
	if err := verifyServerReachable(deployment.server.URL); err != nil {
		t.Fatalf("connect ignored configured trust: %v", err)
	}
}
