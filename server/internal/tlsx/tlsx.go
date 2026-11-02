// Package tlsx extends this process's TLS trust with one explicitly configured
// root CA certificate.
//
// It exists for self-hosted deployments whose server uses a private CA (the
// Windows LAN deployment in deploy/windows-https is one): the CLI, the daemon,
// and the WebSocket dialer must accept that one CA without weakening
// verification. Nothing here can disable verification — the only effect is an
// extra trust anchor appended to the system pool, so certificate, hostname and
// expiry checks keep running exactly as before.
package tlsx

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

// RootCAEnv names the file that holds the extra root CA certificate.
//
// The spelling is the current U_WORKERS_ one; internal/envcompat maps the
// legacy MULTICA_ spelling onto it at process start, so an operator can use
// either name (same rule as every other product variable).
const RootCAEnv = "U_WORKERS_ROOT_CA_FILE"

// cachedPool holds the pool loaded by InstallFromEnv. Clients read it through
// Pool, so one bootstrap call covers every outbound HTTPS client in the process.
var cachedPool atomic.Pointer[x509.CertPool]

// EnvPath returns the configured root CA path, or "" when none is configured.
func EnvPath() string {
	return strings.TrimSpace(os.Getenv(RootCAEnv))
}

// FingerprintSHA256 returns the uppercase hex SHA-256 fingerprint of a
// certificate. It is the value an operator compares against the fingerprint
// printed on the server when exporting its root CA.
func FingerprintSHA256(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	return fmt.Sprintf("%X", sha256.Sum256(cert.Raw))
}

// ParseCertificates parses PEM (one or more CERTIFICATE blocks) or a single
// DER certificate. It returns an error when no certificate can be parsed so a
// wrong file fails loudly instead of silently leaving the system pool in place.
func ParseCertificates(data []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := data
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PEM certificate: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		if cert, err := x509.ParseCertificate(data); err == nil {
			certs = append(certs, cert)
		}
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificate found (expected PEM or DER)")
	}
	return certs, nil
}

// LoadPool reads the root CA file at path and returns the system pool with
// those certificates appended. An empty path yields (nil, nil): "no extra root
// CA configured", which callers treat as "use the system trust store as-is".
//
// A path that exists but cannot be read or parsed is an error: falling back to
// system trust would turn a wrong --root-ca path into a confusing TLS failure
// later, at the first API request.
func LoadPool(path string) (*x509.CertPool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read root CA file %s: %w", path, err)
	}
	certs, err := ParseCertificates(data)
	if err != nil {
		return nil, fmt.Errorf("root CA file %s: %w", path, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, cert := range certs {
		pool.AddCert(cert)
	}
	return pool, nil
}

// InstallFromEnv loads the root CA named by RootCAEnv and caches it for every
// client in this process. It is a no-op when the variable is unset or empty.
//
// Callers run it once at process bootstrap, and again after the connect command
// persists a new CA in the middle of a command; it is cheap and idempotent.
func InstallFromEnv() error {
	path := EnvPath()
	if path == "" {
		cachedPool.Store(nil)
		return nil
	}
	pool, err := LoadPool(path)
	if err != nil {
		return fmt.Errorf("%s=%s: %w", RootCAEnv, path, err)
	}
	cachedPool.Store(pool)
	return nil
}

// Pool returns the pool loaded by InstallFromEnv, or nil when no extra root CA
// is configured.
func Pool() *x509.CertPool {
	return cachedPool.Load()
}

// HTTPTransport returns a clone of http.DefaultTransport (same proxy, timeouts
// and HTTP/2 behavior) with the configured root CA installed. The result is
// always non-nil so callers can pass it straight to http.Client.Transport.
func HTTPTransport() *http.Transport {
	var transport *http.Transport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	pool := Pool()
	if pool == nil {
		return transport
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	transport.TLSClientConfig.RootCAs = pool
	return transport
}

// TLSConfig returns a TLS config carrying the configured root CA, for callers
// that cannot use HTTPTransport (the gorilla/websocket dialer). It returns nil
// when no extra CA is configured, which means "use the default TLS config".
func TLSConfig() *tls.Config {
	pool := Pool()
	if pool == nil {
		return nil
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}
