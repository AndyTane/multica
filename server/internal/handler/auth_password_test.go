package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// passwordTestHandler builds a handler with the shared test database and a
// limiter generous enough not to interfere with non-rate-limit cases.
func passwordTestHandler(t *testing.T, cfg Config) *Handler {
	t.Helper()
	h := newTestHandler(cfg)
	h.Queries = testHandler.Queries
	h.PasswordLimiter = middleware.NewPasswordAttemptLimiter(1_000, 1_000, time.Minute)
	return h
}

// seedLocalPasswordCredential provisions an existing user plus one credential,
// the same shape cmd/local-auth creates.
func seedLocalPasswordCredential(t *testing.T, username, password string) string {
	t.Helper()
	return seedLocalPasswordCredentialForEmail(t, username, password, "local-password-"+username+"@example.com")
}

func seedLocalPasswordCredentialForEmail(t *testing.T, username, password, email string) string {
	t.Helper()
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	userID := dbfx.Insert(t, "user", testutil.Cols{"name": "Local Password User", "email": email})
	hash, err := auth.HashLocalPassword(password)
	if err != nil {
		t.Fatalf("hash test password: %v", err)
	}
	dbfx.Insert(t, "local_password_credential", testutil.Cols{
		"user_id":             userID,
		"username_normalized": username,
		"password_hash":       hash,
	})
	return userID
}

const passwordTestValue = "correct horse battery staple"

func passwordLoginRequest(body any) *http.Request {
	return testutil.JSONRequest(http.MethodPost, "/auth/password", body)
}

func TestPasswordLoginSuccess(t *testing.T) {
	seedLocalPasswordCredential(t, "tane.login", passwordTestValue)
	h := passwordTestHandler(t, Config{LocalPasswordLoginEnabled: true})

	resp := testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": " TANE.Login ",
		"password": passwordTestValue,
	})).Want(http.StatusOK)

	var out LoginResponse
	resp.JSON(&out)
	if out.Token == "" {
		t.Fatal("login response has no token")
	}
	if out.User.ID == "" || out.User.Email == "" {
		t.Fatalf("login response user is incomplete: %+v", out.User)
	}
	if strings.Contains(resp.Text(), passwordTestValue) {
		t.Fatal("login response echoes the password")
	}
	if strings.Contains(resp.Text(), "password_hash") {
		t.Fatal("login response leaks credential storage details")
	}
	if !strings.Contains(out.User.Email, "@example.com") {
		t.Fatalf("unexpected user: %+v", out.User)
	}

	var authCookie *http.Cookie
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == auth.AuthCookieName {
			authCookie = cookie
		}
	}
	if authCookie == nil {
		t.Fatal("login response did not set the auth cookie")
	}
	if !authCookie.HttpOnly {
		t.Fatal("auth cookie is not HttpOnly")
	}
	if authCookie.Value == "" {
		t.Fatal("auth cookie is empty")
	}
}

func TestPasswordLoginRejectsWrongPasswordAndUnknownUsernameIdentically(t *testing.T) {
	seedLocalPasswordCredential(t, "tane.identical", passwordTestValue)
	h := passwordTestHandler(t, Config{LocalPasswordLoginEnabled: true})

	wrongPassword := testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "tane.identical",
		"password": "wrong password value",
	})).Want(http.StatusUnauthorized)

	unknownUser := testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "nobody.at.all",
		"password": passwordTestValue,
	})).Want(http.StatusUnauthorized)

	if wrongPassword.Text() != unknownUser.Text() {
		t.Fatalf("wrong-password body %q differs from unknown-username body %q; the two must be indistinguishable",
			strings.TrimSpace(wrongPassword.Text()), strings.TrimSpace(unknownUser.Text()))
	}
	var body map[string]string
	wrongPassword.JSON(&body)
	if body["error"] != passwordLoginGenericError {
		t.Fatalf("error = %q, want the generic credential failure", body["error"])
	}
}

func TestPasswordLoginRejectsDisabledUser(t *testing.T) {
	// One of the addresses in auth.temporarilyDisabledUserEmails: the same check
	// every other login path runs.
	seedLocalPasswordCredentialForEmail(t, "tane.disabled", passwordTestValue, "pdzzer68@embassybase.com")
	h := passwordTestHandler(t, Config{LocalPasswordLoginEnabled: true})

	resp := testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "tane.disabled",
		"password": passwordTestValue,
	})).Want(http.StatusForbidden)
	var body map[string]string
	resp.JSON(&body)
	if body["error"] != auth.TemporarilyDisabledUserError {
		t.Fatalf("error = %q, want %q", body["error"], auth.TemporarilyDisabledUserError)
	}
}

func TestPasswordLoginRejectsOrphanedCredential(t *testing.T) {
	userID := seedLocalPasswordCredential(t, "tane.orphan", passwordTestValue)
	dbfx.Exec(t, `DELETE FROM "user" WHERE id = $1`, userID)
	h := passwordTestHandler(t, Config{LocalPasswordLoginEnabled: true})

	resp := testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "tane.orphan",
		"password": passwordTestValue,
	})).Want(http.StatusUnauthorized)
	var body map[string]string
	resp.JSON(&body)
	if body["error"] != passwordLoginGenericError {
		t.Fatalf("error = %q, want the generic credential failure: a deleted user must not be distinguishable", body["error"])
	}
}

func TestPasswordLoginFeatureSwitch(t *testing.T) {
	seedLocalPasswordCredential(t, "tane.switched-off", passwordTestValue)
	h := passwordTestHandler(t, Config{LocalPasswordLoginEnabled: false})

	resp := testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "tane.switched-off",
		"password": passwordTestValue,
	})).Want(http.StatusForbidden)
	var body map[string]string
	resp.JSON(&body)
	if body["code"] != "local_password_login_disabled" {
		t.Fatalf("code = %q, want local_password_login_disabled", body["code"])
	}
}

func TestPasswordLoginRequestBoundaries(t *testing.T) {
	seedLocalPasswordCredential(t, "tane.bounds", passwordTestValue)
	h := passwordTestHandler(t, Config{LocalPasswordLoginEnabled: true})

	cases := []struct {
		name string
		body any
	}{
		{"malformed_json", "not json"},
		{"missing_password", map[string]string{"username": "tane.bounds"}},
		{"empty_password", map[string]string{"username": "tane.bounds", "password": ""}},
		{"invalid_username", map[string]string{"username": "bad name!", "password": passwordTestValue}},
		{"password_over_72_bytes", map[string]string{"username": "tane.bounds", "password": strings.Repeat("a", auth.LocalPasswordMaxBytes+1)}},
		{"oversize_body", `{"username":"tane.bounds","password":"` + strings.Repeat("a", passwordLoginMaxBodyBytes) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Call(t, h.PasswordLogin, passwordLoginRequest(tc.body)).Want(http.StatusBadRequest)
		})
	}
}

func TestPasswordLoginAccountRateLimit(t *testing.T) {
	seedLocalPasswordCredential(t, "tane.ratelimited", passwordTestValue)
	h := passwordTestHandler(t, Config{LocalPasswordLoginEnabled: true})
	h.PasswordLimiter = middleware.NewPasswordAttemptLimiter(2, 1_000, time.Minute)

	for attempt := 1; attempt <= 2; attempt++ {
		testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
			"username": "tane.ratelimited",
			"password": "wrong password value",
		})).Want(http.StatusUnauthorized)
	}
	resp := testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "tane.ratelimited",
		"password": "wrong password value",
	})).Want(http.StatusTooManyRequests)
	if resp.Header().Get("Retry-After") == "" {
		t.Fatal("429 response has no Retry-After header")
	}
	// The budget is charged before the password is checked, so even the correct
	// password is refused while the account is over its budget.
	testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "tane.ratelimited",
		"password": passwordTestValue,
	})).Want(http.StatusTooManyRequests)
}

func TestPasswordLoginSuccessDoesNotRefillTheAccountBudget(t *testing.T) {
	seedLocalPasswordCredential(t, "tane.no-refill", passwordTestValue)
	h := passwordTestHandler(t, Config{LocalPasswordLoginEnabled: true})
	h.PasswordLimiter = middleware.NewPasswordAttemptLimiter(3, 1_000, time.Minute)

	// A successful login still spends the attempt it needed.
	testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "tane.no-refill",
		"password": passwordTestValue,
	})).Want(http.StatusOK)
	for attempt := 2; attempt <= 3; attempt++ {
		testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
			"username": "tane.no-refill",
			"password": "wrong password value",
		})).Want(http.StatusUnauthorized)
	}
	testutil.Call(t, h.PasswordLogin, passwordLoginRequest(map[string]string{
		"username": "tane.no-refill",
		"password": passwordTestValue,
	})).Want(http.StatusTooManyRequests)
}

func TestEmailCodeLoginSwitch(t *testing.T) {
	for _, route := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"send_code", (&Handler{cfg: Config{EmailCodeLoginDisabled: true}}).SendCode},
		{"verify_code", (&Handler{cfg: Config{EmailCodeLoginDisabled: true}}).VerifyCode},
	} {
		t.Run(route.name, func(t *testing.T) {
			resp := testutil.Call(t, route.handler, testutil.JSONRequest(http.MethodPost, "/auth/"+route.name, map[string]string{
				"email": "someone@example.com",
				"code":  "123456",
			})).Want(http.StatusForbidden)
			var body map[string]string
			resp.JSON(&body)
			if body["code"] != "email_code_login_disabled" {
				t.Fatalf("code = %q, want email_code_login_disabled", body["code"])
			}
		})
	}
}

func TestEmailCodeLoginEnabledByDefault(t *testing.T) {
	// The zero Config must keep the historical behavior: the guard passes and
	// the handlers run. A malformed body proves execution reached the handler
	// (400) rather than the disabled guard (403).
	h := &Handler{cfg: Config{}}
	testutil.Call(t, h.SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code", "{")).Want(http.StatusBadRequest)
}
