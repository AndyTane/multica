package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TestP5R3OpenRegistrationHandlerMatrix drives the real POST /api/register/open
// handler through the full permission matrix the deployment policy defines:
// the deployment ALLOW_SIGNUP switch, the persistent instance open-registration
// switch (default off), and the optional email/domain allowlists. Both switches
// must be on for a self-service account to be created; an allowlist matches only
// the address set that is already admitted, so it can never open a closed switch.
//
// Every refused row asserts the absence of a user row, a credential, an auth
// cookie and a session token; every admitted row asserts the opposite plus the
// unverified email a self-registration is only allowed to hold.
func TestP5R3OpenRegistrationHandlerMatrix(t *testing.T) {
	ctx := context.Background()
	previousOpen, err := testHandler.readOpenRegistration(ctx)
	if err != nil {
		t.Fatalf("read the current open-registration switch: %v", err)
	}
	t.Cleanup(func() { setOpenRegistrationValue(context.Background(), previousOpen) })

	const domainAllow = "p5r3-matrix-domain.example.test"
	const emailAllow = "p5r3-matrix-allowlisted@example.test"

	type matrixRow struct {
		name           string
		deployment     bool
		instance       bool
		email          string
		allowedEmails  []string
		allowedDomains []string
		wantStatus     int
		wantCode       string
	}
	rows := []matrixRow{
		// Deployment switch off, instance switch on or off: the allowlist match
		// must not substitute for the hard deployment switch.
		{"deployment_off_instance_on_email_allowlist", false, true, emailAllow, []string{emailAllow}, nil, http.StatusForbidden, "registration_disabled"},
		{"deployment_off_instance_off_email_allowlist", false, false, emailAllow, []string{emailAllow}, nil, http.StatusForbidden, "registration_disabled"},
		{"deployment_off_instance_on_domain_allowlist", false, true, "p5r3-matrix-d1@" + domainAllow, nil, []string{domainAllow}, http.StatusForbidden, "registration_disabled"},
		{"deployment_off_instance_off_domain_allowlist", false, false, "p5r3-matrix-d2@" + domainAllow, nil, []string{domainAllow}, http.StatusForbidden, "registration_disabled"},
		// Deployment switch on, instance switch off: matching address, matching
		// domain and no allowlist at all are all admitted only by the switch.
		{"deployment_on_instance_off_email_allowlist", true, false, emailAllow, []string{emailAllow}, nil, http.StatusForbidden, "registration_disabled"},
		{"deployment_on_instance_off_domain_allowlist", true, false, "p5r3-matrix-d3@" + domainAllow, nil, []string{domainAllow}, http.StatusForbidden, "registration_disabled"},
		{"deployment_on_instance_off_no_allowlist", true, false, "p5r3-matrix-no-list-closed@example.test", nil, nil, http.StatusForbidden, "registration_disabled"},
		// Both switches on: a matching allowlist admits, no allowlist admits,
		// and a non-matching address is refused by the allowlist.
		{"both_on_email_allowlist", true, true, emailAllow, []string{emailAllow}, nil, http.StatusOK, ""},
		{"both_on_domain_allowlist", true, true, "p5r3-matrix-d4@" + domainAllow, nil, []string{domainAllow}, http.StatusOK, ""},
		{"both_on_no_allowlist", true, true, "p5r3-matrix-no-list-open@example.test", nil, nil, http.StatusOK, ""},
		{"both_on_no_match", true, true, "p5r3-matrix-outsider@example.test", []string{emailAllow}, nil, http.StatusForbidden, "email_not_allowed"},
	}

	for i, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			username := fmt.Sprintf("p5r3.matrix.%d", i)
			dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, row.email)
			dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = $1`, username)
			h := p5Handler(t, Config{
				AllowSignup:               row.deployment,
				LocalPasswordLoginEnabled: true,
				AllowedEmails:             row.allowedEmails,
				AllowedEmailDomains:       row.allowedDomains,
			})
			setOpenRegistrationValue(ctx, row.instance)
			t.Cleanup(func() { setOpenRegistrationValue(context.Background(), previousOpen) })

			body := map[string]string{
				"email":    row.email,
				"name":     "P5 R3 Matrix",
				"username": username,
				"password": "correct horse battery staple",
			}
			resp := testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", body)).Want(row.wantStatus)
			if row.wantStatus != http.StatusOK {
				if got := resp.Map()["code"]; got != row.wantCode {
					t.Fatalf("expected error code %q, got %v: %s", row.wantCode, got, resp.Text())
				}
				p5R3AssertNoAccount(t, row.email, username)
				p5R3AssertNoSession(t, resp)
				return
			}

			var login LoginResponse
			resp.JSON(&login)
			if login.Token == "" || login.User.Email != row.email {
				t.Fatalf("unexpected registration response: %+v", login)
			}
			if login.User.IsInstanceAdmin {
				t.Fatal("self-service registration must never create an instance administrator")
			}
			if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, row.email); got != 1 {
				t.Fatalf("expected exactly one account for %s, got %d", row.email, got)
			}
			if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1 AND email_verified = false`, row.email); got != 1 {
				t.Fatalf("self-registered %s must stay unverified, got %d unverified rows", row.email, got)
			}
			if got := dbfx.Count(t, `SELECT count(*) FROM local_password_credential WHERE username_normalized = $1`, username); got != 1 {
				t.Fatalf("expected exactly one credential for %s, got %d", username, got)
			}
			authCookie := false
			for _, cookie := range resp.Result().Cookies() {
				if cookie.Name == auth.AuthCookieName && cookie.Value == login.Token {
					authCookie = true
				}
			}
			if !authCookie {
				t.Fatal("an admitted registration must set the auth cookie that matches its token")
			}
		})
	}
}

// p5R3SeedCredential gives userUUID a password credential for username.
func p5R3SeedCredential(t *testing.T, userUUID pgtype.UUID, username, password string) {
	t.Helper()
	ctx := context.Background()
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = $1`, username)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE user_id = $1`, userUUID)
	hash, err := auth.HashLocalPassword(password)
	if err != nil {
		t.Fatalf("hash the fixture password: %v", err)
	}
	if _, err := testHandler.Queries.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID:             userUUID,
		UsernameNormalized: username,
		PasswordHash:       hash,
	}); err != nil {
		t.Fatalf("seed the fixture credential: %v", err)
	}
}

// TestP5R3ExistingPasswordLoginWhileRegistrationClosed proves the closed
// registration switches do not gate sign-in: the real POST /auth/password
// handler still authenticates an existing credential, while the same address is
// refused by the self-service registration endpoint.
func TestP5R3ExistingPasswordLoginWhileRegistrationClosed(t *testing.T) {
	setOpenRegistration(t, false)
	email := "p5r3-existing-login@example.test"
	username := "p5r3.existing.login"
	password := "correct horse battery staple"
	_, userUUID := p5SeedUser(t, email)
	p5R3SeedCredential(t, userUUID, username, password)
	h := p5Handler(t, Config{AllowSignup: false, LocalPasswordLoginEnabled: true})

	// The same address cannot be self-registered while the switches are off.
	testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", map[string]string{
		"email": email, "name": "P5 R3 Existing", "username": username, "password": password,
	})).Want(http.StatusForbidden)

	resp := testutil.Call(t, h.PasswordLogin, testutil.JSONRequest(http.MethodPost, "/auth/password", map[string]string{
		"username": username, "password": password,
	})).Want(http.StatusOK)
	var login LoginResponse
	resp.JSON(&login)
	if login.Token == "" || login.User.Email != email {
		t.Fatalf("existing account could not sign in while registration is closed: %+v", login)
	}
	authCookie := false
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == auth.AuthCookieName && cookie.Value == login.Token {
			authCookie = true
		}
	}
	if !authCookie {
		t.Fatal("password login must set the auth cookie")
	}

	// A wrong password is still refused.
	testutil.Call(t, h.PasswordLogin, testutil.JSONRequest(http.MethodPost, "/auth/password", map[string]string{
		"username": username, "password": "definitely not the password",
	})).Want(http.StatusUnauthorized)
}

// TestP5R3InviteRedemptionWorksWhileRegistrationClosed exercises the real
// invitation redemption handler: an administrator-minted single-use invitation
// sets the first password once while the instance registration switch is off,
// the replay is refused, and the account can then sign in with its password.
func TestP5R3InviteRedemptionWorksWhileRegistrationClosed(t *testing.T) {
	ctx := context.Background()
	setOpenRegistration(t, false)
	email := "p5r3-invite-closed@example.test"
	username := "p5r3.invite.closed"
	password := "correct horse battery staple"
	_, userUUID := p5SeedUser(t, email)
	dbfx.Cleanup(t, `DELETE FROM registration_invite WHERE user_id = $1`, userUUID)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = $1`, username)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE user_id = $1`, userUUID)

	raw, err := auth.NewRegistrationInviteToken()
	if err != nil {
		t.Fatalf("mint an invitation token: %v", err)
	}
	if _, err := testHandler.Queries.CreateRegistrationInvite(ctx, db.CreateRegistrationInviteParams{
		UserID:    userUUID,
		TokenHash: auth.RegistrationTokenHash(raw),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("create the invitation: %v", err)
	}
	h := p5Handler(t, Config{AllowSignup: false, LocalPasswordLoginEnabled: true})

	// The instance is genuinely closed for self-service signup at this moment.
	testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", map[string]string{
		"email": "p5r3-invite-bystander@example.test", "name": "Bystander", "username": "p5r3.invite.bystander", "password": password,
	})).Want(http.StatusForbidden)

	body := map[string]string{"token": raw, "name": "P5 R3 Invitee", "username": username, "password": password}
	resp := testutil.Call(t, h.RedeemRegistrationInvite, p5JSONRequest(t, http.MethodPost, "/api/register/invite", body)).Want(http.StatusOK)
	var login LoginResponse
	resp.JSON(&login)
	if login.Token == "" || login.User.Email != email {
		t.Fatalf("invitation redemption did not sign the invited account in: %+v", login)
	}
	if strings.Contains(resp.Text(), password) {
		t.Fatal("the redemption response echoes the password")
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM local_password_credential WHERE username_normalized = $1`, username); got != 1 {
		t.Fatalf("expected exactly one credential after redemption, got %d", got)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM registration_invite WHERE user_id = $1 AND consumed_at IS NOT NULL`, userUUID); got != 1 {
		t.Fatalf("expected the invitation to be consumed exactly once, got %d consumed rows", got)
	}

	// Replay of the same secret is refused and cannot overwrite the credential.
	replay := testutil.Call(t, h.RedeemRegistrationInvite, p5JSONRequest(t, http.MethodPost, "/api/register/invite", map[string]string{
		"token": raw, "name": "P5 R3 Invitee", "username": "p5r3.invite.replay", "password": "an entirely different passphrase",
	})).Want(http.StatusBadRequest)
	if got := replay.Map()["code"]; got != "invalid_invite" {
		t.Fatalf("replay should report invalid_invite, got %v", got)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM local_password_credential WHERE username_normalized = $1`, "p5r3.invite.replay"); got != 0 {
		t.Fatalf("a replayed invitation created %d credentials", got)
	}

	// The password set through the invitation signs in while registration stays closed.
	testutil.Call(t, h.PasswordLogin, testutil.JSONRequest(http.MethodPost, "/auth/password", map[string]string{
		"username": username, "password": password,
	})).Want(http.StatusOK)
}

// TestP5R3EmailCodeCannotBypassClosedRegistration enables the email-code entry
// point (the deployment has it off, so this only runs in the isolated test
// environment) and proves a matching allowlist cannot create an account while
// the instance registration switch is off. It never contacts a mail provider:
// the service stays in its local dev mode and no message leaves the host.
func TestP5R3EmailCodeCannotBypassClosedRegistration(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("SMTP_HOST", "")
	setOpenRegistration(t, false)
	email := "p5r3-emailcode-closed@example.test"
	code := "654321"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, email)
	h := p5Handler(t, Config{AllowSignup: true, AllowedEmails: []string{email}})
	h.EmailService = service.NewEmailService()

	// The code request itself is refused: no account, no mail, no session.
	sendResp := testutil.Call(t, h.SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code", map[string]string{"email": email})).Want(http.StatusForbidden)
	p5R3AssertNoAccount(t, email, email)
	p5R3AssertNoSession(t, sendResp)
	if got := dbfx.Count(t, `SELECT count(*) FROM verification_code WHERE email = $1`, email); got != 0 {
		t.Fatalf("a refused code request stored %d verification codes", got)
	}

	// Verify the account-creation path directly: an otherwise valid code still
	// cannot create the account while the switch is off.
	dbfx.Insert(t, "verification_code", testutil.Cols{
		"email":      email,
		"code":       code,
		"expires_at": testutil.Raw("now() + interval '10 minutes'"),
	})
	verifyResp := testutil.Call(t, h.VerifyCode, testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{
		"email": email, "code": code,
	})).Want(http.StatusForbidden)
	p5R3AssertNoAccount(t, email, email)
	p5R3AssertNoSession(t, verifyResp)

	// Positive control: the same entry point admits the allowlisted address once
	// an administrator opens the instance switch.
	setOpenRegistration(t, true)
	dbfx.Insert(t, "verification_code", testutil.Cols{
		"email":      email,
		"code":       code,
		"expires_at": testutil.Raw("now() + interval '10 minutes'"),
	})
	resp := testutil.Call(t, h.VerifyCode, testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{
		"email": email, "code": code,
	})).Want(http.StatusOK)
	var login LoginResponse
	resp.JSON(&login)
	if login.Token == "" || login.User.Email != email {
		t.Fatalf("unexpected email-code sign-in response: %+v", login)
	}
}

// TestP5R3GoogleCannotBypassClosedRegistration enables the Google entry point
// behind a stubbed provider (no request reaches Google) and proves the closed
// instance switch still refuses a brand-new address that matches the allowlist.
func TestP5R3GoogleCannotBypassClosedRegistration(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	setOpenRegistration(t, false)
	email := "p5r3-google-closed@example.test"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	h := p5Handler(t, Config{AllowSignup: true, AllowedEmails: []string{email}})
	h.googleOAuthHTTPClient = &http.Client{Transport: googleRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "oauth2.googleapis.com":
			return googleResponse(req, http.StatusOK, `{"access_token":"test-token"}`), nil
		case "www.googleapis.com":
			return googleResponse(req, http.StatusOK, `{"email":"`+email+`"}`), nil
		default:
			t.Fatalf("unexpected Google OAuth request: %s", req.URL)
			return nil, nil
		}
	})}
	req := httptest.NewRequest(http.MethodPost, "/auth/google", strings.NewReader(`{"code":"test-code","redirect_uri":"http://localhost/auth/callback"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := testutil.Call(t, h.GoogleLogin, req).Want(http.StatusForbidden)
	if got := resp.Map()["code"]; got != "signup_prohibited" {
		t.Fatalf("expected signup_prohibited for a new Google address, got %v: %s", got, resp.Text())
	}
	p5R3AssertNoAccount(t, email, email)
	p5R3AssertNoSession(t, resp)
}
