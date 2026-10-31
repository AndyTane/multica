package auth

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// Local password login.
//
// A self-hosted deployment can provision one account/password credential per
// existing user without depending on an outbound mail service. This file owns
// the credential policy (username normalization, password bounds, bcrypt
// parameters) so the login handler and the local admin tool enforce exactly the
// same rules. Credentials are created only by the host administrator through
// cmd/local-auth; there is no signup, reset-by-mail or web-side initialization.

const (
	// LocalUsernameMinRunes / LocalUsernameMaxRunes bound the canonical
	// account name. Three characters keeps a typo from becoming a credential;
	// 64 keeps the stored value and the login form bounded.
	LocalUsernameMinRunes = 3
	LocalUsernameMaxRunes = 64

	// LocalPasswordMinRunes is the shortest accepted password, counted in
	// characters rather than bytes so a 12-character CJK password is not
	// rejected as "too short" for being ~36 bytes.
	LocalPasswordMinRunes = 12

	// LocalPasswordMaxBytes is bcrypt's hard input limit. Longer input is
	// rejected outright: bcrypt's own GenerateFromPassword returns an error
	// past this point, and truncating would make the tail of a long
	// passphrase meaningless without telling the user.
	LocalPasswordMaxBytes = 72

	// LocalPasswordHashCost is the bcrypt cost new credentials are hashed
	// with. The workload is one compare per login attempt, already capped by
	// the password login rate limiter.
	LocalPasswordHashCost = 12
)

var (
	// ErrInvalidLocalUsername reports a username that cannot be normalized to
	// a legal account name.
	ErrInvalidLocalUsername = errors.New("invalid local username")
	// ErrInvalidLocalPassword reports a password outside the length policy.
	ErrInvalidLocalPassword = errors.New("invalid local password")
)

// LocalPasswordPolicy is the operator-facing summary of the password rules,
// used by the admin tool prompts. Keep it in sync with ValidateLocalPassword.
const LocalPasswordPolicy = "at least 12 characters and at most 72 bytes"

// NormalizeLocalUsername canonicalizes a username for storage and lookup.
//
// Rules: surrounding whitespace is trimmed, the value is lowercased, and the
// remaining characters must be letters (any script), digits, '.', '_' or '-'.
// The same function runs on provisioning and on login, so "Tane", " TANE "
// and "tane" all resolve to one credential. No Unicode NFC/NFD folding is
// performed: two visually identical code point sequences are two different
// names, which is the conservative choice for an account identifier.
func NormalizeLocalUsername(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return "", ErrInvalidLocalUsername
	}
	count := 0
	for _, r := range name {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", ErrInvalidLocalUsername
		}
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
		case r == '.', r == '_', r == '-':
		default:
			return "", ErrInvalidLocalUsername
		}
		count++
	}
	if count < LocalUsernameMinRunes || count > LocalUsernameMaxRunes {
		return "", ErrInvalidLocalUsername
	}
	return name, nil
}

// ValidateLocalPassword enforces the password length policy: measured in runes
// for the lower bound and in bytes for the bcrypt upper bound.
func ValidateLocalPassword(password string) error {
	if utf8.RuneCountInString(password) < LocalPasswordMinRunes {
		return fmt.Errorf("%w: %s", ErrInvalidLocalPassword, LocalPasswordPolicy)
	}
	if len(password) > LocalPasswordMaxBytes {
		return fmt.Errorf("%w: %s", ErrInvalidLocalPassword, LocalPasswordPolicy)
	}
	return nil
}

// HashLocalPassword validates and bcrypt-hashes a password. Callers never
// persist the plaintext; the hash is the only value that leaves this function.
func HashLocalPassword(password string) (string, error) {
	if err := ValidateLocalPassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), LocalPasswordHashCost)
	if err != nil {
		return "", fmt.Errorf("hash local password: %w", err)
	}
	return string(hash), nil
}

// VerifyLocalPassword reports whether password matches the stored bcrypt hash.
// Malformed hashes and bcrypt's own length errors both mean "does not match".
func VerifyLocalPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// DummyLocalPasswordHash is a real bcrypt hash of a random throwaway password,
// compared against on login when the username has no credential. Without it an
// unknown username would answer noticeably faster than a wrong password for a
// known one, turning the login endpoint into a username oracle. Keep it at
// LocalPasswordHashCost so both paths do the same work.
const DummyLocalPasswordHash = "$2a$12$ZetkJ/QBDO0MCBzLQUj1He3y2xOz7Wq8aatZbU51w09d71B5OKvZe"
