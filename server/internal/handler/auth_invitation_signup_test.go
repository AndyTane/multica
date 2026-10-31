package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func signupInvitation(t *testing.T, email string) string {
	t.Helper()
	return dbfx.Insert(t, "workspace_invitation", testutil.Cols{
		"workspace_id":  parseUUID(testWorkspaceID),
		"inviter_id":    parseUUID(testUserID),
		"invitee_email": email,
		"role":          "member",
		"status":        "pending",
		"expires_at":    testutil.Raw("now() + interval '1 hour'"),
	})
}

func restrictSignupHandler(t *testing.T) *Handler {
	t.Helper()
	// Keep email delivery local even when the developer has configured a provider.
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("SMTP_HOST", "")
	h := *testHandler
	h.cfg = Config{AllowSignup: true}
	h.EmailService = service.NewEmailService()
	return &h
}

// setOpenRegistration flips the instance switch and restores the previous
// value when the test ends, so tests do not leak the policy into one another.
func setOpenRegistration(t *testing.T, enabled bool) {
	t.Helper()
	previous, err := testHandler.readOpenRegistration(context.Background())
	if err != nil {
		t.Fatalf("read open registration: %v", err)
	}
	setOpenRegistrationValue(context.Background(), enabled)
	t.Cleanup(func() { setOpenRegistrationValue(context.Background(), previous) })
}

func setOpenRegistrationValue(ctx context.Context, enabled bool) {
	value := []byte(`{"enabled":false}`)
	if enabled {
		value = []byte(`{"enabled":true}`)
	}
	_, _ = testHandler.Queries.UpsertInstanceSetting(ctx, db.UpsertInstanceSettingParams{
		Key: openRegistrationSettingKey, Value: value,
	})
}

// TestWorkspaceInvitationDoesNotAuthorizeInstanceSignup is the B1 regression for
// the email-code/Google paths: an operator inviting a mailbox to one workspace
// must not open instance registration for whoever types that mailbox.
func TestWorkspaceInvitationDoesNotAuthorizeInstanceSignup(t *testing.T) {
	email := "invite-not-a-grant@example.com"
	signupInvitation(t, email)
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, email)
	setOpenRegistration(t, false)
	h := restrictSignupHandler(t)

	testutil.Call(t, h.SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code", map[string]string{"email": email})).Want(http.StatusForbidden)
	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); got != 0 {
		t.Fatalf("workspace invitation created %d accounts", got)
	}

	// The instance switch is what admits a new identity.
	setOpenRegistration(t, true)
	testutil.Call(t, h.SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code", map[string]string{"email": email})).Want(http.StatusOK)
}

// TestVerifyCodeRechecksInstanceRegistrationSwitch: the policy is re-read at
// account creation, so closing registration between the two steps still refuses.
func TestVerifyCodeRechecksInstanceRegistrationSwitch(t *testing.T) {
	email := "invite-recheck-open-switch@example.com"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, email)
	setOpenRegistration(t, true)
	h := restrictSignupHandler(t)

	testutil.Call(t, h.SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code", map[string]string{"email": email})).Want(http.StatusOK)
	code, err := h.Queries.GetLatestVerificationCode(context.Background(), email)
	if err != nil {
		t.Fatalf("load verification code: %v", err)
	}

	setOpenRegistration(t, false)
	resp := testutil.Call(t, h.VerifyCode, testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{"email": email, "code": code.Code})).Want(http.StatusForbidden)
	if len(resp.Result().Cookies()) != 0 {
		t.Fatal("refused verification established a session")
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); got != 0 {
		t.Fatalf("closed registration created %d accounts", got)
	}

	// Reopening the switch makes a fresh code work again. The code is seeded
	// directly so the per-email send cooldown does not make this test about
	// rate limiting.
	setOpenRegistration(t, true)
	dbfx.Insert(t, "verification_code", testutil.Cols{
		"email": email, "code": "654321", "expires_at": testutil.Raw("now() + interval '10 minutes'"),
	})
	testutil.Call(t, h.VerifyCode, testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{"email": email, "code": "654321"})).Want(http.StatusOK)
	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); got != 1 {
		t.Fatalf("expected one account, got %d", got)
	}
}

// TestGoogleLoginUsesInstanceRegistrationSwitch: Google's verified-email claim
// still has to obey the same instance policy as every other creation path.
func TestGoogleLoginUsesInstanceRegistrationSwitch(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	email := "google-instance-switch@example.com"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("open=%t", enabled), func(t *testing.T) {
			setOpenRegistration(t, enabled)
			h := *testHandler
			h.cfg = Config{AllowSignup: true}
			h.googleOAuthHTTPClient = &http.Client{Transport: googleRoundTripper(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Host {
				case "oauth2.googleapis.com":
					return googleResponse(req, http.StatusOK, `{"access_token":"test-token"}`), nil
				case "www.googleapis.com":
					return googleResponse(req, http.StatusOK, fmt.Sprintf(`{"email":%q}`, " "+strings.ToUpper(email)+" ")), nil
				default:
					t.Fatalf("unexpected Google request: %s", req.URL)
					return nil, nil
				}
			})}
			want := http.StatusForbidden
			if enabled {
				want = http.StatusOK
			}
			resp := testutil.Call(t, h.GoogleLogin, testutil.JSONRequest(http.MethodPost, "/auth/google", map[string]string{"code": "test-code", "redirect_uri": "http://localhost/auth/callback"})).Want(want)
			if enabled {
				var login LoginResponse
				resp.JSON(&login)
				if login.Token == "" || login.User.Email != email {
					t.Fatalf("expected authenticated account, got %+v", login.User)
				}
			} else if len(resp.Result().Cookies()) != 0 || dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email) != 0 {
				t.Fatal("closed registration must not create an account or session")
			}
		})
	}
}

// failingInstancePolicyDB fails the single-row read the signup policy needs and
// delegates every other query, so the HTTP error path (not a business refusal)
// is what the test observes.
type failingInstancePolicyDB struct{ db.DBTX }

func (m failingInstancePolicyDB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if strings.HasPrefix(sql, "-- name: GetInstanceSetting :one\n") {
		return &mockRow{err: context.Canceled}
	}
	return m.DBTX.QueryRow(ctx, sql, args...)
}

func TestSignupPolicyReadFailure(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	for _, path := range []string{"send-code", "verify-code", "google"} {
		t.Run(path, func(t *testing.T) {
			email := "policy-read-error-" + path + "@example.com"
			h := *testHandler
			h.cfg = Config{AllowSignup: true}
			h.Queries = db.New(failingInstancePolicyDB{testPool})
			handler := h.SendCode
			body := map[string]string{"email": email}
			switch path {
			case "verify-code":
				handler = h.VerifyCode
				body["code"] = "123456"
				dbfx.Insert(t, "verification_code", testutil.Cols{
					"email": email, "code": body["code"], "expires_at": testutil.Raw("now() + interval '10 minutes'"),
				})
			case "google":
				handler = h.GoogleLogin
				body = map[string]string{"code": "test-code", "redirect_uri": "http://localhost/auth/callback"}
				h.googleOAuthHTTPClient = &http.Client{Transport: googleRoundTripper(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Host {
					case "oauth2.googleapis.com":
						return googleResponse(req, http.StatusOK, `{"access_token":"test-token"}`), nil
					case "www.googleapis.com":
						return googleResponse(req, http.StatusOK, fmt.Sprintf(`{"email":%q}`, email)), nil
					default:
						t.Fatalf("unexpected Google request: %s", req.URL)
						return nil, nil
					}
				})}
			}
			dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
			resp := testutil.Call(t, handler, testutil.JSONRequest(http.MethodPost, "/auth/"+path, body)).Want(http.StatusInternalServerError)
			if len(resp.Result().Cookies()) != 0 || dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email) != 0 {
				t.Fatal("policy read failure must not create an account or session")
			}
		})
	}
}
