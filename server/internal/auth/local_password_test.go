package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestNormalizeLocalUsername(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"lowercase_passthrough", "tane", "tane", true},
		{"uppercase_folds", "TANE", "tane", true},
		{"mixed_case_folds", "TaNe", "tane", true},
		{"surrounding_space_trimmed", "  tane  ", "tane", true},
		{"inner_space_rejected", "ta ne", "", false},
		{"empty_rejected", "", "", false},
		{"spaces_only_rejected", "   ", "", false},
		{"too_short_rejected", "ab", "", false},
		{"min_length_accepted", "abc", "abc", true},
		{"dots_underscores_dashes_accepted", "tane.dev_01-x", "tane.dev_01-x", true},
		{"at_sign_rejected", "tane@example.com", "", false},
		{"slash_rejected", "tane/dev", "", false},
		{"colon_rejected", "tane:dev", "", false},
		{"wildcard_rejected", "tane*", "", false},
		{"surrounding_newline_trimmed", "tane\n", "tane", true},
		{"tab_rejected", "ta\tne", "", false},
		{"cjk_letters_accepted", "\u5929\u4f7f\u7ec4", "\u5929\u4f7f\u7ec4", true},
		{"max_length_accepted", strings.Repeat("a", LocalUsernameMaxRunes), strings.Repeat("a", LocalUsernameMaxRunes), true},
		{"over_max_length_rejected", strings.Repeat("a", LocalUsernameMaxRunes+1), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLocalUsername(tt.raw)
			if tt.ok && err != nil {
				t.Fatalf("NormalizeLocalUsername(%q) returned error: %v", tt.raw, err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidLocalUsername) {
				t.Fatalf("NormalizeLocalUsername(%q) = %q, %v; want ErrInvalidLocalUsername", tt.raw, got, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeLocalUsername(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeLocalUsernameCaseConflictsCollapse(t *testing.T) {
	variants := []string{"Tane", "tane", " TANE ", "TaNe"}
	first, err := NormalizeLocalUsername(variants[0])
	if err != nil {
		t.Fatalf("normalize %q: %v", variants[0], err)
	}
	for _, variant := range variants[1:] {
		got, err := NormalizeLocalUsername(variant)
		if err != nil {
			t.Fatalf("normalize %q: %v", variant, err)
		}
		if got != first {
			t.Fatalf("username variant %q normalized to %q, want the same value as %q (%q); a case-variant must not create a second account", variant, got, variants[0], first)
		}
	}
}

func TestValidateLocalPasswordBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		password string
		ok       bool
	}{
		{"eleven_runes_rejected", strings.Repeat("a", LocalPasswordMinRunes-1), false},
		{"twelve_runes_accepted", strings.Repeat("a", LocalPasswordMinRunes), true},
		{"empty_rejected", "", false},
		{"eleven_cjk_runes_rejected", strings.Repeat("\u5bc6", LocalPasswordMinRunes-1), false},
		{"twelve_cjk_runes_accepted", strings.Repeat("\u5bc6", LocalPasswordMinRunes), true},
		{"seventy_two_bytes_accepted", strings.Repeat("a", LocalPasswordMaxBytes), true},
		{"seventy_three_bytes_rejected", strings.Repeat("a", LocalPasswordMaxBytes+1), false},
		{"cjk_over_byte_limit_rejected", strings.Repeat("\u5bc6", 25), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLocalPassword(tt.password)
			if tt.ok && err != nil {
				t.Fatalf("ValidateLocalPassword(len=%d) = %v, want nil", len(tt.password), err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidLocalPassword) {
				t.Fatalf("ValidateLocalPassword(len=%d) = %v, want ErrInvalidLocalPassword", len(tt.password), err)
			}
		})
	}
}

func TestHashAndVerifyLocalPasswordRoundTrip(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := HashLocalPassword(password)
	if err != nil {
		t.Fatalf("HashLocalPassword: %v", err)
	}
	if strings.Contains(hash, password) {
		t.Fatal("hash contains the plaintext password")
	}
	if !VerifyLocalPassword(hash, password) {
		t.Fatal("VerifyLocalPassword rejected the correct password")
	}
	if VerifyLocalPassword(hash, password+"x") {
		t.Fatal("VerifyLocalPassword accepted a wrong password")
	}
	if VerifyLocalPassword("not-a-bcrypt-hash", password) {
		t.Fatal("VerifyLocalPassword accepted a malformed hash")
	}
}

func TestHashLocalPasswordUsesCost12(t *testing.T) {
	hash, err := HashLocalPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashLocalPassword: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost != LocalPasswordHashCost {
		t.Fatalf("bcrypt cost = %d, want %d", cost, LocalPasswordHashCost)
	}
}

func TestHashLocalPasswordRejectsOutOfPolicyInput(t *testing.T) {
	if _, err := HashLocalPassword("short"); !errors.Is(err, ErrInvalidLocalPassword) {
		t.Fatalf("HashLocalPassword(short) = %v, want ErrInvalidLocalPassword", err)
	}
	if _, err := HashLocalPassword(strings.Repeat("a", LocalPasswordMaxBytes+1)); !errors.Is(err, ErrInvalidLocalPassword) {
		t.Fatalf("HashLocalPassword(oversize) = %v, want ErrInvalidLocalPassword", err)
	}
}

func TestResetChangesPassword(t *testing.T) {
	oldPassword := "first password value"
	newPassword := "second password value"
	oldHash, err := HashLocalPassword(oldPassword)
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	newHash, err := HashLocalPassword(newPassword)
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}
	if oldHash == newHash {
		t.Fatal("two hashes of different passwords are identical")
	}
	if !VerifyLocalPassword(newHash, newPassword) {
		t.Fatal("new hash rejected the new password")
	}
	if VerifyLocalPassword(newHash, oldPassword) {
		t.Fatal("new hash accepted the old password")
	}
	if VerifyLocalPassword(oldHash, newPassword) {
		t.Fatal("old hash accepted the new password")
	}
}

func TestDummyLocalPasswordHashIsUsableAtCost12(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(DummyLocalPasswordHash))
	if err != nil {
		t.Fatalf("DummyLocalPasswordHash is not a bcrypt hash: %v", err)
	}
	if cost != LocalPasswordHashCost {
		t.Fatalf("DummyLocalPasswordHash cost = %d, want %d so unknown usernames cost the same bcrypt work as real ones", cost, LocalPasswordHashCost)
	}
	// The plaintext behind the dummy hash is random and was never written down,
	// so a caller cannot present input that satisfies it.
	if VerifyLocalPassword(DummyLocalPasswordHash, "") {
		t.Fatal("DummyLocalPasswordHash matched an empty password")
	}
}
