// Package wintrust adds a single public root CA certificate to the current
// Windows user's trust store without touching TLS verification anywhere: the
// operating system keeps deciding which roots are trusted, and this package only
// widens that set for one user.
//
// The certificate is always handed to the platform in its canonical DER form,
// so PEM armor, line endings, and surrounding text never reach the store or the
// fingerprint.
//
// The helpers are platform independent; only the two store operations are
// Windows-specific. Trusting a root is a decision Windows confirms with the
// user, so InstallUserRootCA is not a silent operation (see its documentation).
package wintrust

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// InstallResult describes an idempotent per-user trust install.
type InstallResult struct {
	// AlreadyTrusted reports that the certificate was already in
	// Cert:\CurrentUser\Root, so the store was left untouched.
	AlreadyTrusted bool
	// FingerprintSHA256 is the uppercase hex SHA-256 fingerprint of the
	// certificate.
	FingerprintSHA256 string
}

// ParseCACertificate parses the first certificate from PEM or DER bytes,
// rejects anything that is not a CA certificate, and returns the cert.
//
// PEM input may hold a whole chain; only the first CERTIFICATE block is used.
// The CA check is deliberately strict: a non-CA certificate trusted as a root
// would be accepted for every host name, so BasicConstraints CA:TRUE and
// keyCertSign (when a key usage extension is present) are both required.
func ParseCACertificate(data []byte) (*x509.Certificate, error) {
	der, err := certificateDER(data)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("wintrust: parse certificate: %w", err)
	}
	if err := validateCACertificate(cert); err != nil {
		return nil, err
	}
	return cert, nil
}

// FingerprintSHA256 returns the uppercase hex SHA-256 fingerprint.
func FingerprintSHA256(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	return fmt.Sprintf("%X", sha256.Sum256(cert.Raw))
}

// certificateDER returns the DER encoding of the first certificate in data.
func certificateDER(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("wintrust: no certificate data")
	}
	rest := data
	sawPEMBlock := false
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		sawPEMBlock = true
		if block.Type != "CERTIFICATE" {
			continue
		}
		if len(block.Bytes) == 0 {
			return nil, errors.New("wintrust: empty CERTIFICATE PEM block")
		}
		return block.Bytes, nil
	}
	if sawPEMBlock {
		return nil, errors.New("wintrust: no CERTIFICATE PEM block found")
	}
	// Not PEM at all: hand the bytes to the DER parser, which rejects anything
	// that is not a single DER-encoded certificate.
	return data, nil
}

// validateCACertificate enforces the properties a root anchor must have.
func validateCACertificate(cert *x509.Certificate) error {
	if cert == nil {
		return errors.New("wintrust: nil certificate")
	}
	if !cert.BasicConstraintsValid {
		return errors.New("wintrust: certificate has no basic constraints extension")
	}
	if !cert.IsCA {
		return errors.New("wintrust: certificate is not a CA certificate (basic constraints CA:FALSE)")
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return errors.New("wintrust: CA certificate cannot sign certificates (key usage lacks keyCertSign)")
	}
	return nil
}

// canonicalDER returns the DER encoding this package hands to the platform: the
// certificate's own Raw bytes. Raw is derived from the parsed certificate, so
// the same certificate always reaches the store and the fingerprint byte for
// byte, whatever the caller's input encoding was.
func canonicalDER(cert *x509.Certificate) ([]byte, error) {
	if cert == nil {
		return nil, errors.New("wintrust: nil certificate")
	}
	if len(cert.Raw) == 0 {
		return nil, errors.New("wintrust: certificate has no DER encoding")
	}
	return cert.Raw, nil
}
