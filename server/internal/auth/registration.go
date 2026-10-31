package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Web registration helpers (P5).
//
// Invitation secrets are 256-bit random values encoded as URL-safe base64.
// Only the SHA-256 digest is stored; the raw value exists in memory on the
// host that created it and in the page fragment the invitee opens.

const (
	// RegistrationInviteTokenBytes is the entropy behind one invitation.
	RegistrationInviteTokenBytes = 32
	// RegistrationInviteTokenMinLength bounds obviously malformed input before
	// it reaches the database.
	RegistrationInviteTokenMinLength = 32
	// RegistrationInviteTokenMaxLength bounds the control characters and
	// enormous bodies a client might send.
	RegistrationInviteTokenMaxLength = 128

	// RegistrationDisplayNameMaxRunes bounds the display name the invitee /
	// registrant picks. It mirrors the profile field policy closely enough.
	RegistrationDisplayNameMaxRunes = 80
	// RegistrationEmailMaxLength matches the practical RFC 5321 mailbox bound.
	RegistrationEmailMaxLength = 254
)

var (
	ErrInvalidRegistrationEmail       = errors.New("invalid email address")
	ErrInvalidRegistrationDisplayName = errors.New("invalid display name")
	ErrInvalidRegistrationToken       = errors.New("invalid invitation token")
)

// NewRegistrationInviteToken returns a fresh 256-bit invitation secret.
func NewRegistrationInviteToken() (string, error) {
	buf := make([]byte, RegistrationInviteTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate registration invite: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// RegistrationTokenHash is the at-rest representation of an invitation token.
func RegistrationTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NormalizeRegistrationToken rejects malformed secrets before any database
// lookup so an enormous or empty value cannot reach the query layer.
func NormalizeRegistrationToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if len(token) < RegistrationInviteTokenMinLength || len(token) > RegistrationInviteTokenMaxLength {
		return "", ErrInvalidRegistrationToken
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return "", ErrInvalidRegistrationToken
		}
	}
	return token, nil
}

// NormalizeRegistrationEmail canonicalizes an email for uniqueness checks and
// storage. It is deliberately conservative: trim, lowercase, one @ and a dot
// in the domain. It is not RFC 5322 validation and it never treats an email as
// proof of ownership.
func NormalizeRegistrationEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > RegistrationEmailMaxLength {
		return "", ErrInvalidRegistrationEmail
	}
	if strings.ContainsAny(email, " \t\r\n<>()[]:;,") {
		return "", ErrInvalidRegistrationEmail
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", ErrInvalidRegistrationEmail
	}
	local, domain := email[:at], email[at+1:]
	if local == "" || domain == "" || !strings.Contains(domain, ".") {
		return "", ErrInvalidRegistrationEmail
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", ErrInvalidRegistrationEmail
	}
	return email, nil
}

// NormalizeRegistrationDisplayName trims and bounds a display name. Control
// characters are rejected; formatting characters are allowed because names in
// several supported locales need them.
func NormalizeRegistrationDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > RegistrationDisplayNameMaxRunes {
		return "", ErrInvalidRegistrationDisplayName
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", ErrInvalidRegistrationDisplayName
		}
	}
	return name, nil
}
