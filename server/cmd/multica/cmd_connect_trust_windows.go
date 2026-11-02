//go:build windows

package main

import (
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/multica-ai/multica/server/internal/wintrust"
)

// browserTrustConfirmationTimeout bounds how long the connect command waits for
// the operator to answer the Windows root-certificate security warning. The
// store API blocks until the dialog is answered, so a headless run would
// otherwise hang forever.
const browserTrustConfirmationTimeout = 3 * time.Minute

// installBrowserRootCA adds the deployment's public root CA to this Windows
// user's certificate store (Cert:\CurrentUser\Root), which is what lets the
// browser open the deployment's HTTPS entry for the sign-in page without a
// certificate warning.
//
// Windows shows a one-time security warning for every root-store install and
// the store call blocks until it is answered; there is no API to suppress it
// (the existing deploy/windows-https/import-client-root.ps1 confirms the same
// dialog). So this prints what will happen first, then bounds the wait with a
// timeout that tells the operator how to continue without browser trust.
//
// Nothing here disables verification: the certificate only becomes an
// additional trust anchor for this user.
func installBrowserRootCA(cert *x509.Certificate) (bool, error) {
	if _, err := wintrust.ParseCACertificate(cert.Raw); err != nil {
		return false, fmt.Errorf("root CA is not installable: %w", err)
	}
	fmt.Fprintln(os.Stderr, "  Windows will ask you to confirm trusting this root certificate — click 是 / Yes.")
	done := make(chan error, 1)
	go func() {
		_, err := wintrust.InstallUserRootCA(cert)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			return false, fmt.Errorf("trust the root CA for this Windows user: %w", err)
		}
		return true, nil
	case <-time.After(browserTrustConfirmationTimeout):
		return false, fmt.Errorf("timed out waiting for the Windows certificate confirmation (%s); answer the security warning with 是/Yes, or rerun with --skip-browser-trust and install the certificate later with deploy/windows-https/import-client-root.ps1", browserTrustConfirmationTimeout)
	}
}
