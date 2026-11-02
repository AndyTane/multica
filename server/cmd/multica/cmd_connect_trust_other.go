//go:build !windows

package main

import (
	"crypto/x509"

	"github.com/multica-ai/multica/server/internal/wintrust"
)

// installBrowserRootCA is a no-op outside Windows: a browser launched by this
// platform uses its own trust store and the CLI/daemon trust the CA through
// internal/tlsx. The --trust-browser flag is accepted everywhere so the same
// command works on every platform.
func installBrowserRootCA(cert *x509.Certificate) (bool, error) {
	_, _ = wintrust.ParseCACertificate(cert.Raw)
	return false, nil
}
