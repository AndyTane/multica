package daemon

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/tlsx"
)

func TestTaskMulticaEnvironmentIncludesClientTrust(t *testing.T) {
	ca := filepath.Join(t.TempDir(), "public-ca.pem")
	env := taskMulticaEnvironment(Task{}, "codex", "agent", "mat_synthetic", t.TempDir(), t.TempDir(), "https://task.example", ca, 19514, 1, t.TempDir())
	if got := env[tlsx.RootCAEnv]; got != ca {
		t.Fatalf("task CA = %q, want selected public CA %q", got, ca)
	}
	layerCustomEnvAndHermesHome(env, map[string]string{
		tlsx.RootCAEnv: "wrong-current-ca", "MULTICA_ROOT_CA_FILE": "wrong-legacy-ca",
		"MULTICA_TOKEN": "mul_synthetic_owner", "U_WORKERS_TOKEN": "mul_synthetic_owner",
	}, "", nil)
	if env[tlsx.RootCAEnv] != ca || env["MULTICA_ROOT_CA_FILE"] != "" || env["MULTICA_TOKEN"] != "mat_synthetic" || env["U_WORKERS_TOKEN"] != "" {
		t.Fatal("custom environment replaced task trust or identity")
	}
}

func TestPrepareTaskClientTrust(t *testing.T) {
	_, ca := newTaskTrustServer(t, nil, "valid")
	root := filepath.Join(t.TempDir(), "task-config")
	got, err := prepareTaskClientTrust(root, ca)
	if err != nil || got != ca {
		t.Fatalf("prepare task trust = %q, %v", got, err)
	}
	assertConfig := func(wantCA string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		var cfg map[string]string
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg["root_ca_file"] != wantCA || len(cfg) > 1 {
			t.Fatalf("task config must contain only selected public trust: %v", cfg)
		}
		if runtime.GOOS != "windows" {
			info, _ := os.Stat(filepath.Join(root, "config.json"))
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("config permissions = %o", info.Mode().Perm())
			}
			info, _ = os.Stat(root)
			if info.Mode().Perm() != 0o700 {
				t.Fatalf("directory permissions = %o", info.Mode().Perm())
			}
		}
	}
	assertConfig(ca)
	// Refreshing a reused task clears stale CA and locally persisted identity.
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"token":"mat_stale_synthetic","root_ca_file":"stale"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareTaskClientTrust(root, ""); err != nil {
		t.Fatal(err)
	}
	assertConfig("")
	// A relative path must be made absolute before the child changes cwd.
	t.Chdir(filepath.Dir(ca))
	if got, err := prepareTaskClientTrust(root, filepath.Base(ca)); err != nil || got != ca {
		t.Fatalf("relative task trust = %q, %v", got, err)
	}
	assertConfig(ca)
	bad := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(bad, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{bad, bad + ".missing"} {
		if _, err := prepareTaskClientTrust(root, path); err == nil {
			t.Fatal("invalid CA must prevent task preparation")
		}
		assertConfig(ca)
	}
	for _, invalidRoot := range []string{"", "relative-task-root", bad} {
		if _, err := prepareTaskClientTrust(invalidRoot, ca); err == nil {
			t.Fatalf("invalid task root %q accepted", invalidRoot)
		}
	}
	// Atomic installation failure must not leave a partial trust file behind.
	blocked := t.TempDir()
	if err := os.Mkdir(filepath.Join(blocked, "config.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareTaskClientTrust(blocked, ca); err == nil {
		t.Fatal("failed config install must prevent task preparation")
	}
	entries, err := os.ReadDir(blocked)
	if err != nil || len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Fatalf("temporary trust files left after failed install: %v, %v", entries, err)
	}
}

// Each fixture has an independent CA; httptest's shared default certificate
// cannot distinguish the configured root from an unrelated fixture's root.
func newTaskTrustServer(t *testing.T, handler http.Handler, leafMode string) (*httptest.Server, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic task trust CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), NotBefore: root.NotBefore, NotAfter: root.NotAfter,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:    x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if leafMode == "hostname-mismatch" {
		leaf.IPAddresses = nil
		leaf.DNSNames = []string{"unrelated.invalid"}
	}
	if leafMode == "expired" {
		leaf.NotBefore, leaf.NotAfter = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(t.TempDir(), "public-ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER, rootDER}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, caPath
}
