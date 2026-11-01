package wintrust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// The public API is frozen: these declarations fail the build if a signature
// drifts on either platform.
var (
	_ func([]byte) (*x509.Certificate, error)        = ParseCACertificate
	_ func(*x509.Certificate) string                 = FingerprintSHA256
	_ func(*x509.Certificate) (InstallResult, error) = InstallUserRootCA
	_ func(*x509.Certificate) (bool, error)          = IsUserRootCATrusted
	_ InstallResult                                  = InstallResult{AlreadyTrusted: true, FingerprintSHA256: "AA"}
)

// newTestCA returns a self-signed CA certificate and its PEM encoding. mutate,
// when set, adjusts the template before signing so rejection cases can be
// derived from an otherwise valid CA. Nothing here touches the certificate
// store, so these tests run on every platform.
func newTestCA(t *testing.T, mutate func(*x509.Certificate)) (*x509.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "wintrust test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	if mutate != nil {
		mutate(template)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse created certificate: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if pemBytes == nil {
		t.Fatal("encode certificate as PEM: no output")
	}
	return cert, pemBytes
}

func TestParseCACertificatePEM(t *testing.T) {
	want, pemBytes := newTestCA(t, nil)

	got, err := ParseCACertificate(pemBytes)
	if err != nil {
		t.Fatalf("ParseCACertificate(PEM): %v", err)
	}
	if got.Subject.CommonName != "wintrust test CA" {
		t.Errorf("CommonName = %q, want %q", got.Subject.CommonName, "wintrust test CA")
	}
	if !got.IsCA {
		t.Error("IsCA = false, want true")
	}
	if string(got.Raw) != string(want.Raw) {
		t.Error("parsed certificate differs from the generated one")
	}
}

func TestParseCACertificateDER(t *testing.T) {
	want, _ := newTestCA(t, nil)

	got, err := ParseCACertificate(want.Raw)
	if err != nil {
		t.Fatalf("ParseCACertificate(DER): %v", err)
	}
	if strings.ToUpper(hex.EncodeToString(got.Raw)) != strings.ToUpper(hex.EncodeToString(want.Raw)) {
		t.Error("DER round trip changed the certificate")
	}
}

func TestParseCACertificateUsesFirstPEMCertificate(t *testing.T) {
	first, firstPEM := newTestCA(t, nil)
	_, secondPEM := newTestCA(t, nil)

	// A PEM file may carry a chain; only the first certificate is the root the
	// caller asked for.
	bundle := append(append([]byte{}, firstPEM...), secondPEM...)
	got, err := ParseCACertificate(bundle)
	if err != nil {
		t.Fatalf("ParseCACertificate(chain): %v", err)
	}
	if FingerprintSHA256(got) != FingerprintSHA256(first) {
		t.Errorf("fingerprint = %s, want %s (the first certificate)", FingerprintSHA256(got), FingerprintSHA256(first))
	}
}

func TestParseCACertificateRejectsNonCA(t *testing.T) {
	_, pemBytes := newTestCA(t, func(c *x509.Certificate) {
		c.IsCA = false
		c.BasicConstraintsValid = true
		c.KeyUsage = x509.KeyUsageDigitalSignature
	})

	_, err := ParseCACertificate(pemBytes)
	if err == nil {
		t.Fatal("ParseCACertificate accepted a non-CA certificate")
	}
	if !strings.Contains(err.Error(), "not a CA certificate") {
		t.Errorf("error = %v, want it to name the CA constraint", err)
	}
}

func TestParseCACertificateRejectsMissingBasicConstraints(t *testing.T) {
	_, pemBytes := newTestCA(t, func(c *x509.Certificate) {
		c.IsCA = false
		c.BasicConstraintsValid = false
		c.KeyUsage = x509.KeyUsageDigitalSignature
	})

	_, err := ParseCACertificate(pemBytes)
	if err == nil {
		t.Fatal("ParseCACertificate accepted a certificate without basic constraints")
	}
	if !strings.Contains(err.Error(), "basic constraints") {
		t.Errorf("error = %v, want it to name the missing basic constraints", err)
	}
}

func TestParseCACertificateRejectsKeyUsageWithoutCertSign(t *testing.T) {
	_, pemBytes := newTestCA(t, func(c *x509.Certificate) {
		c.KeyUsage = x509.KeyUsageDigitalSignature
	})

	_, err := ParseCACertificate(pemBytes)
	if err == nil {
		t.Fatal("ParseCACertificate accepted a CA that cannot sign certificates")
	}
	if !strings.Contains(err.Error(), "keyCertSign") {
		t.Errorf("error = %v, want it to name keyCertSign", err)
	}
}

func TestParseCACertificateRejectsInvalidInput(t *testing.T) {
	ca, _ := newTestCA(t, nil)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{0x30, 0x00}})
	emptyCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE"})

	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{name: "empty", data: nil, wantErr: "no certificate data"},
		{name: "not a certificate", data: []byte("not a certificate"), wantErr: "parse certificate"},
		{name: "truncated DER", data: ca.Raw[:10], wantErr: "parse certificate"},
		{name: "PEM without a certificate", data: privateKeyPEM, wantErr: "no CERTIFICATE PEM block"},
		{name: "empty certificate block", data: emptyCertPEM, wantErr: "empty CERTIFICATE PEM block"},
		{name: "trailing bytes after DER", data: append(append([]byte{}, ca.Raw...), 0x00), wantErr: "parse certificate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseCACertificate(tc.data); err == nil {
				t.Fatalf("ParseCACertificate(%q) succeeded, want error", tc.name)
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestFingerprintSHA256(t *testing.T) {
	cert, pemBytes := newTestCA(t, nil)

	sum := sha256.Sum256(cert.Raw)
	want := strings.ToUpper(hex.EncodeToString(sum[:]))

	got := FingerprintSHA256(cert)
	if got != want {
		t.Fatalf("FingerprintSHA256 = %s, want %s", got, want)
	}
	if len(got) != 64 {
		t.Errorf("fingerprint length = %d, want 64", len(got))
	}
	for _, r := range got {
		if !strings.ContainsRune("0123456789ABCDEF", r) {
			t.Errorf("fingerprint %q contains %q, want uppercase hex only", got, r)
		}
	}

	// The fingerprint is a property of the certificate, not of its encoding.
	parsed, err := ParseCACertificate(pemBytes)
	if err != nil {
		t.Fatalf("ParseCACertificate: %v", err)
	}
	if FingerprintSHA256(parsed) != want {
		t.Errorf("fingerprint of the parsed certificate = %s, want %s", FingerprintSHA256(parsed), want)
	}
}

func TestFingerprintSHA256NormalizesPEMArmor(t *testing.T) {
	cert, pemBytes := newTestCA(t, nil)
	want := FingerprintSHA256(cert)

	// Same certificate, different armor: CRLF line endings and 64-column
	// wrapping, which is what a certificate exported by another tool may look
	// like.
	body := strings.TrimSpace(string(pemBytes))
	body = strings.TrimSuffix(strings.TrimPrefix(body, "-----BEGIN CERTIFICATE-----"), "-----END CERTIFICATE-----")
	var wrapped strings.Builder
	wrapped.WriteString("-----BEGIN CERTIFICATE-----\r\n")
	for i := 0; i < len(body); i += 64 {
		end := i + 64
		if end > len(body) {
			end = len(body)
		}
		wrapped.WriteString(strings.TrimSpace(body[i:end]))
		wrapped.WriteString("\r\n")
	}
	wrapped.WriteString("-----END CERTIFICATE-----\r\n")

	parsed, err := ParseCACertificate([]byte(wrapped.String()))
	if err != nil {
		t.Fatalf("ParseCACertificate(re-wrapped PEM): %v", err)
	}
	if got := FingerprintSHA256(parsed); got != want {
		t.Errorf("fingerprint = %s, want %s", got, want)
	}
}

func TestFingerprintSHA256NilCertificate(t *testing.T) {
	if got := FingerprintSHA256(nil); got != "" {
		t.Errorf("FingerprintSHA256(nil) = %q, want an empty string", got)
	}
}

func TestCanonicalDER(t *testing.T) {
	cert, _ := newTestCA(t, nil)

	der, err := canonicalDER(cert)
	if err != nil {
		t.Fatalf("canonicalDER: %v", err)
	}
	if string(der) != string(cert.Raw) {
		t.Error("canonicalDER did not return the certificate's DER encoding")
	}

	if _, err := canonicalDER(nil); err == nil {
		t.Error("canonicalDER(nil) succeeded, want error")
	}
	if _, err := canonicalDER(&x509.Certificate{}); err == nil {
		t.Error("canonicalDER(certificate without Raw) succeeded, want error")
	}
}
