package tlsx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mustSelfSignedCA mints a throwaway CA certificate for trust-pool tests.
func mustSelfSignedCA(t *testing.T) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "U Workers tlsx test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

func writeCertPEM(t *testing.T, dir, name string, cert *x509.Certificate) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func newTrustClient() *http.Client {
	return &http.Client{Transport: HTTPTransport(), Timeout: 5 * time.Second}
}

func TestParseCertificates(t *testing.T) {
	cert := mustSelfSignedCA(t)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})

	t.Run("pem", func(t *testing.T) {
		got, err := ParseCertificates(pemBytes)
		if err != nil {
			t.Fatalf("ParseCertificates: %v", err)
		}
		if len(got) != 1 || !got[0].Equal(cert) {
			t.Fatalf("parsed certificates = %d, want the input certificate", len(got))
		}
	})

	t.Run("der", func(t *testing.T) {
		got, err := ParseCertificates(cert.Raw)
		if err != nil {
			t.Fatalf("ParseCertificates(DER): %v", err)
		}
		if len(got) != 1 || !got[0].Equal(cert) {
			t.Fatalf("parsed DER certificates = %d, want 1", len(got))
		}
	})

	t.Run("garbage", func(t *testing.T) {
		if _, err := ParseCertificates([]byte("not a certificate")); err == nil {
			t.Fatal("want error for non-certificate input")
		}
	})
}

func TestLoadPoolEmptyPathConfiguredNothing(t *testing.T) {
	pool, err := LoadPool("  ")
	if err != nil {
		t.Fatalf("LoadPool(empty): %v", err)
	}
	if pool != nil {
		t.Fatal("LoadPool(empty) should return no pool")
	}
}

func TestLoadPoolMissingFileFails(t *testing.T) {
	if _, err := LoadPool(filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Fatal("want error for a missing root CA file")
	}
}

// TestTransportTrustsOnlyTheConfiguredRootCA pins the security contract: the
// configured CA makes the self-signed test server reachable, and removing it
// makes the same request fail certificate verification. If this ever passed
// with verification disabled (InsecureSkipVerify), the second half would not
// fail.
func TestTransportTrustsOnlyTheConfiguredRootCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dir := t.TempDir()
	caPath := writeCertPEM(t, dir, "root-ca.pem", server.Certificate())

	t.Setenv(RootCAEnv, caPath)
	if err := InstallFromEnv(); err != nil {
		t.Fatalf("InstallFromEnv: %v", err)
	}
	if Pool() == nil {
		t.Fatal("Pool() is nil after installing a root CA")
	}
	client := newTrustClient()
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("request with the configured root CA failed: %v", err)
	}
	resp.Body.Close()
	// Drop the pooled TLS connection: a reused connection skips the handshake
	// and would make the unconfigured request below succeed for the wrong reason.
	client.CloseIdleConnections()

	// Unconfigured: the same request must fail verification, proving the
	// success above came from the extra trust anchor and not from relaxed
	// certificate checks.
	t.Setenv(RootCAEnv, "")
	if err := InstallFromEnv(); err != nil {
		t.Fatalf("InstallFromEnv(empty): %v", err)
	}
	if Pool() != nil {
		t.Fatal("Pool() should be nil after clearing the root CA env")
	}
	// A fresh client and transport: reusing the one built above would keep its
	// old TLSClientConfig and test nothing.
	if _, err := newTrustClient().Get(server.URL); err == nil {
		t.Fatal("request without the root CA must fail certificate verification")
	}
}

// TestTransportRejectsAnUnrelatedRootCA covers a wrong --root-ca path: the
// file parses, but it did not sign the server certificate, so the request must
// still fail.
func TestTransportRejectsAnUnrelatedRootCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	unrelated := mustSelfSignedCA(t)
	path := writeCertPEM(t, t.TempDir(), "other-ca.pem", unrelated)
	t.Setenv(RootCAEnv, path)
	if err := InstallFromEnv(); err != nil {
		t.Fatalf("InstallFromEnv: %v", err)
	}
	client := newTrustClient()
	_, err := client.Get(server.URL)
	if err == nil {
		t.Fatal("request must fail when the configured CA did not sign the server")
	}
}

func TestTLSConfigMirrorsPool(t *testing.T) {
	t.Setenv(RootCAEnv, "")
	if err := InstallFromEnv(); err != nil {
		t.Fatalf("InstallFromEnv: %v", err)
	}
	if TLSConfig() != nil {
		t.Fatal("TLSConfig() should be nil without a configured root CA")
	}

	cert := mustSelfSignedCA(t)
	path := writeCertPEM(t, t.TempDir(), "root-ca.pem", cert)
	t.Setenv(RootCAEnv, path)
	if err := InstallFromEnv(); err != nil {
		t.Fatalf("InstallFromEnv: %v", err)
	}
	cfg := TLSConfig()
	if cfg == nil {
		t.Fatal("TLSConfig() is nil after installing a root CA")
	}
	if cfg.InsecureSkipVerify {
		t.Fatal("TLSConfig must never disable verification")
	}
	if cfg.RootCAs == nil {
		t.Fatal("TLSConfig must carry the configured roots")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %#x, want TLS 1.2", cfg.MinVersion)
	}
}
