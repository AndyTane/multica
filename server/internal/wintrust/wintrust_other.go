//go:build !windows

package wintrust

import (
	"crypto/x509"
	"errors"
	"fmt"
	"runtime"
)

// errUnsupported is returned by both store operations on every non-Windows
// platform. The parsers and the fingerprint stay usable there.
var errUnsupported = errors.New("wintrust: the current user's root CA store exists only on Windows")

// InstallUserRootCA adds cert to the current user's Root store (Cert:\CurrentUser\Root).
// Verification is NOT disabled anywhere; this only adds a trust anchor for this user.
//
// This platform has no such store, so the call always fails.
func InstallUserRootCA(cert *x509.Certificate) (InstallResult, error) {
	return InstallResult{}, fmt.Errorf("%w (running on %s)", errUnsupported, runtime.GOOS)
}

// IsUserRootCATrusted reports whether the cert is already in the current user's Root store.
//
// This platform has no such store, so the call always fails.
func IsUserRootCATrusted(cert *x509.Certificate) (bool, error) {
	return false, fmt.Errorf("%w (running on %s)", errUnsupported, runtime.GOOS)
}
