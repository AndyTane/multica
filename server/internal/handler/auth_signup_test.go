package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func newTestHandler(cfg Config) *Handler {
	return &Handler{
		cfg: cfg,
	}
}

// TestSignupGating pins the policy every account-creation entry point shares:
// the deployment switch, the instance's open-registration setting (default
// off), and the email/domain allowlists.
func TestSignupGating(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		open bool
		// email is the address being evaluated; isNew distinguishes account
		// creation from a sign-in by an existing account.
		email string
		isNew bool
		want  error
	}{
		{"open_registration_enabled_new", Config{AllowSignup: true}, true, "a@x.com", true, nil},
		{"open_registration_disabled_new", Config{AllowSignup: true}, false, "a@x.com", true, ErrSignupProhibited},
		{"allow_signup_false_new", Config{AllowSignup: false}, true, "a@x.com", true, ErrSignupProhibited},
		{"allow_signup_false_existing", Config{AllowSignup: false}, true, "a@x.com", false, nil},
		{"domain_allowlist_match", Config{AllowSignup: false, AllowedEmailDomains: []string{"company.com"}}, false, "user@company.com", true, nil},
		{"domain_allowlist_mismatch_signup_disabled", Config{AllowSignup: false, AllowedEmailDomains: []string{"company.com"}}, true, "user@other.com", true, ErrSignupProhibited},
		{"domain_allowlist_mismatch_signup_enabled", Config{AllowSignup: true, AllowedEmailDomains: []string{"company.com"}}, true, "user@other.com", true, ErrEmailNotAllowed},
		{"email_allowlist_match", Config{AllowSignup: false, AllowedEmails: []string{"boss@x.com"}}, false, "boss@x.com", true, nil},
		{"email_allowlist_mismatch_signup_enabled", Config{AllowSignup: true, AllowedEmails: []string{"boss@x.com"}}, true, "user@other.com", true, ErrEmailNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.cfg)
			h.Queries = db.New(&mockDB{openRegistration: tt.open})
			err := h.checkSignupAllowed(context.Background(), tt.email, tt.isNew)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got err=%v want=%v", err, tt.want)
			}
		})
	}
}

func TestEmailCodeAllowlistErrors(t *testing.T) {
	for _, path := range []string{"send-code", "verify-code"} {
		t.Run(path, func(t *testing.T) {
			email := path + "-allowlist-regression@example.com"
			h := newTestHandler(Config{AllowSignup: true, AllowedEmailDomains: []string{"company.com"}})
			h.Queries = testHandler.Queries
			dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
			body := map[string]string{"email": email}
			handler := h.SendCode
			if path == "verify-code" {
				// A code can remain valid after the instance's signup policy changes.
				body["code"] = "123456"
				dbfx.Insert(t, "verification_code", testutil.Cols{
					"email":      email,
					"code":       body["code"],
					"expires_at": testutil.Raw("now() + interval '10 minutes'"),
				})
				handler = h.VerifyCode
			}
			req := testutil.JSONRequest(http.MethodPost, "/auth/"+path, body)
			resp := testutil.Call(t, handler, req).Want(http.StatusForbidden)
			got := resp.Map()
			if got["error"] != ErrEmailNotAllowed.Error() {
				t.Fatalf("expected an actionable allowlist error, got %v", got)
			}
			if _, hasCode := got["code"]; hasCode {
				t.Fatal("email-code errors must retain their existing response shape")
			}
			if len(resp.Result().Cookies()) != 0 {
				t.Fatal("rejected signup must not establish an authenticated session")
			}
			if count := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); count != 0 {
				t.Fatalf("rejected signup created %d users", count)
			}
		})
	}
}

// TestSignupGatingReturnsPolicyReadError: a failing instance-setting read is a
// server error, not a silent policy change.
func TestSignupGatingReturnsPolicyReadError(t *testing.T) {
	h := newTestHandler(Config{AllowSignup: true})
	h.Queries = db.New(&mockDB{instanceSettingErr: context.Canceled})

	err := h.checkSignupAllowed(context.Background(), "user@x.com", true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err=%v, want context canceled", err)
	}
}

// mockDB answers the single-row reads the signup policy makes. Anything else
// falls through to getUserErr, which is how the existing account-lookup tests
// drive their branches.
type mockDB struct {
	db.DBTX
	openRegistration   bool
	instanceSettingErr error
	getUserErr         error
	invitationLookups  int
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	switch {
	case strings.HasPrefix(sql, "-- name: GetInstanceSetting :one\n"):
		if m.instanceSettingErr != nil {
			return &mockRow{err: m.instanceSettingErr}
		}
		value := []byte(`{"enabled":false}`)
		if m.openRegistration {
			value = []byte(`{"enabled":true}`)
		}
		return &mockRow{bytesValue: value}
	case strings.HasPrefix(sql, "-- name: HasPendingInvitationForEmail :one\n"):
		m.invitationLookups++
		return &mockRow{boolValue: boolPointer(false)}
	}
	return &mockRow{err: m.getUserErr}
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func boolPointer(value bool) *bool { return &value }

type mockRow struct {
	pgx.Row
	err        error
	boolValue  *bool
	bytesValue []byte
}

func (m *mockRow) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}
	if m.boolValue != nil {
		value, ok := dest[0].(*bool)
		if !ok {
			return fmt.Errorf("expected *bool destination, got %T", dest[0])
		}
		*value = *m.boolValue
	}
	if m.bytesValue != nil {
		value, ok := dest[0].(*[]byte)
		if !ok {
			return fmt.Errorf("expected *[]byte destination, got %T", dest[0])
		}
		*value = m.bytesValue
	}
	return nil
}

func TestFindOrCreateUserGating(t *testing.T) {
	t.Run("new_user_blocked", func(t *testing.T) {
		cfg := Config{AllowSignup: false}
		h := newTestHandler(cfg)
		h.Queries = db.New(&mockDB{getUserErr: pgx.ErrNoRows})

		_, isNew, err := h.findOrCreateUser(context.Background(), "new@blocked.com")
		if err == nil {
			t.Fatal("expected error for new user when signup disabled")
		}
		if isNew {
			t.Fatal("isNew should be false when signup is blocked")
		}
		if !strings.Contains(err.Error(), "registration is disabled") {
			t.Fatalf("expected registration disabled error, got %v", err)
		}
	})

	t.Run("unverified_existing_account_is_not_merged", func(t *testing.T) {
		h := newTestHandler(Config{AllowSignup: true})
		// A zero-value user row is an unverified account.
		h.Queries = db.New(&mockDB{})

		_, isNew, err := h.findOrCreateUser(context.Background(), "squatted@example.com")
		if !errors.Is(err, ErrUnverifiedEmailAccount) {
			t.Fatalf("got err=%v, want ErrUnverifiedEmailAccount", err)
		}
		if isNew {
			t.Fatal("an existing row must not be reported as new")
		}
	})

	t.Run("whitelisted_new_user_passes_policy", func(t *testing.T) {
		cfg := Config{AllowSignup: false, AllowedEmails: []string{"whitelisted@test.com"}}
		h := newTestHandler(cfg)
		h.Queries = db.New(&mockDB{})
		if err := h.checkSignupAllowed(context.Background(), "whitelisted@test.com", true); err != nil {
			t.Fatalf("allowlisted address rejected: %v", err)
		}
	})
}

// TestSignupPolicyDoesNotConsultWorkspaceInvitations: a workspace invitation is
// not an instance registration grant. The policy must not even look for one, so
// a pending row cannot open signup in either switch position.
func TestSignupPolicyDoesNotConsultWorkspaceInvitations(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  Config
		open bool
		want error
	}{
		{"instance_closed", Config{AllowSignup: true}, false, ErrSignupProhibited},
		{"instance_open", Config{AllowSignup: true}, true, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDB{openRegistration: tt.open}
			h := newTestHandler(tt.cfg)
			h.Queries = db.New(mock)
			err := h.checkSignupAllowed(context.Background(), "invited@other.com", true)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got err=%v want=%v", err, tt.want)
			}
			if mock.invitationLookups != 0 {
				t.Fatalf("signup policy consulted workspace invitations %d times", mock.invitationLookups)
			}
		})
	}
}
