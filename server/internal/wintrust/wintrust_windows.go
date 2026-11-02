//go:build windows

package wintrust

import (
	"bytes"
	"crypto/x509"
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CertAddEncodedCertificateToStore is the one call that x/sys/windows does not
// wrap. The DLL is resolved from System32 only, so a DLL planted in the working
// directory cannot intercept the trust store write.
var procCertAddEncodedCertificateToStore = windows.NewLazySystemDLL("crypt32.dll").NewProc("CertAddEncodedCertificateToStore")

// certEncoding accepts a DER-encoded certificate: X509_ASN_ENCODING |
// PKCS_7_ASN_ENCODING.
const certEncoding = windows.X509_ASN_ENCODING | windows.PKCS_7_ASN_ENCODING

// userRootStorePath is the store named in errors and docs.
const userRootStorePath = `Cert:\CurrentUser\Root`

// InstallUserRootCA adds cert to the current user's Root store (Cert:\CurrentUser\Root).
// Verification is NOT disabled anywhere; this only adds a trust anchor for this user.
//
// The install is idempotent: when the certificate is already present the store
// is left untouched and AlreadyTrusted is set. The result is only meaningful
// when the error is nil.
//
// Windows treats a root store write as a user decision: on an interactive
// desktop the call shows the standard "Security Warning" confirmation dialog
// and blocks until it is answered. No documented switch suppresses that
// prompt, so the call must run where its dialog can be answered.
func InstallUserRootCA(cert *x509.Certificate) (InstallResult, error) {
	der, err := canonicalDER(cert)
	if err != nil {
		return InstallResult{}, err
	}
	if err := validateCACertificate(cert); err != nil {
		return InstallResult{}, err
	}
	store, err := openUserRootStore()
	if err != nil {
		return InstallResult{}, err
	}
	defer func() { _ = windows.CertCloseStore(store, 0) }()

	added, err := addEncodedCertificate(store, der)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{
		AlreadyTrusted:    !added,
		FingerprintSHA256: FingerprintSHA256(cert),
	}, nil
}

// IsUserRootCATrusted reports whether the cert is already in the current user's Root store.
func IsUserRootCATrusted(cert *x509.Certificate) (bool, error) {
	der, err := canonicalDER(cert)
	if err != nil {
		return false, err
	}
	store, err := openUserRootStore()
	if err != nil {
		return false, err
	}
	defer func() { _ = windows.CertCloseStore(store, 0) }()
	return storeContainsCertificate(store, der)
}

// openUserRootStore opens Cert:\CurrentUser\Root. The caller owns the handle and
// must pass it to CertCloseStore.
func openUserRootStore() (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString("ROOT")
	if err != nil {
		return 0, fmt.Errorf("wintrust: encode store name: %w", err)
	}
	store, err := windows.CertOpenStore(
		windows.CERT_STORE_PROV_SYSTEM_W,
		0,
		0,
		windows.CERT_SYSTEM_STORE_CURRENT_USER,
		uintptr(unsafe.Pointer(name)),
	)
	if err != nil {
		return 0, fmt.Errorf("wintrust: open %s: %w", userRootStorePath, err)
	}
	return store, nil
}

// addEncodedCertificate adds der to store and reports whether it was added. A
// certificate that is already present is not an error; added is false instead.
func addEncodedCertificate(store windows.Handle, der []byte) (added bool, err error) {
	// The disposition is CERT_STORE_ADD_NEW, so an existing certificate makes
	// the call fail with CRYPT_E_EXISTS rather than duplicating the entry.
	// ppCertContext stays nil: the store holds its own copy and this package
	// never reads it back.
	r1, _, callErr := procCertAddEncodedCertificateToStore.Call(
		uintptr(store),
		uintptr(certEncoding),
		uintptr(unsafe.Pointer(&der[0])),
		uintptr(len(der)),
		uintptr(windows.CERT_STORE_ADD_NEW),
		0,
	)
	if r1 != 0 {
		return true, nil
	}
	if isAlreadyExists(callErr) {
		return false, nil
	}
	return false, fmt.Errorf("wintrust: add certificate to %s: %w", userRootStorePath, callErr)
}

// storeContainsCertificate reports whether der is in store. The comparison is
// on the exact DER encoding, so another certificate that happens to share an
// issuer and serial number does not count.
func storeContainsCertificate(store windows.Handle, der []byte) (bool, error) {
	// CertEnumCertificatesInStore frees the previously returned context on the
	// next call, so only the context of the current iteration is alive.
	var previous *windows.CertContext
	for {
		ctx, enumErr := windows.CertEnumCertificatesInStore(store, previous)
		if ctx == nil {
			if enumErr == nil || isNotFound(enumErr) {
				return false, nil
			}
			return false, fmt.Errorf("wintrust: enumerate %s: %w", userRootStorePath, enumErr)
		}
		previous = ctx
		if bytes.Equal(unsafe.Slice(ctx.EncodedCert, int(ctx.Length)), der) {
			_ = windows.CertFreeCertificateContext(ctx)
			return true, nil
		}
	}
}

// isAlreadyExists reports whether err is Windows' "certificate is already in the
// store" result. CryptoAPI reports it as CRYPT_E_EXISTS from the crypt32 layer
// and as plain ERROR_ALREADY_EXISTS from the Win32 layer, sometimes in its
// HRESULT_FROM_WIN32 form, so all three spellings are matched.
func isAlreadyExists(err error) bool {
	switch errno(err) {
	case syscall.Errno(windows.CRYPT_E_EXISTS), windows.ERROR_ALREADY_EXISTS, 0x800700B7:
		return true
	}
	return false
}

// isNotFound reports whether err is the end-of-enumeration result.
func isNotFound(err error) bool {
	return errno(err) == syscall.Errno(windows.CRYPT_E_NOT_FOUND)
}

// errno extracts the Windows error number carried by err.
func errno(err error) syscall.Errno {
	var e syscall.Errno
	if !errors.As(err, &e) {
		return 0
	}
	return e
}
