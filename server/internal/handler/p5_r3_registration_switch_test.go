package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// p5R3AssertNoSession fails when a refused account creation established a
// browser session. Every rejection in this file must leave the caller anonymous:
// no auth cookie, no CSRF cookie, no session token in the body.
func p5R3AssertNoSession(t *testing.T, resp *testutil.Response) {
	t.Helper()
	for _, cookie := range resp.Result().Cookies() {
		switch cookie.Name {
		case auth.AuthCookieName, auth.CSRFCookieName, auth.SessionCSRFCookieName:
			t.Fatalf("a refused registration set the %s cookie", cookie.Name)
		}
	}
	if strings.Contains(resp.Text(), "\"token\"") {
		t.Fatalf("a refused registration returned a session token: %s", resp.Text())
	}
}

// p5R3AssertNoAccount fails when a refused registration left a user row or a
// password credential behind.
func p5R3AssertNoAccount(t *testing.T, email, username string) {
	t.Helper()
	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); got != 0 {
		t.Fatalf("a refused registration created %d user rows for %s", got, email)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM local_password_credential WHERE username_normalized = $1`, username); got != 0 {
		t.Fatalf("a refused registration created %d credentials for %s", got, username)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM local_password_credential WHERE user_id IN (SELECT id FROM "user" WHERE email = $1)`, email); got != 0 {
		t.Fatalf("a refused registration created %d credentials for the %s account", got, email)
	}
}

// TestP5R3AllowlistCannotBypassClosedRegistration is the reviewer's case: with
// an email or domain allowlist configured, a self-registered address that
// matches the allowlist must still be refused while the instance registration
// switch is off, and no user or credential may be created. Turning the switch
// on lets the matching addresses through, while a non-matching address is still
// refused by the allowlist.
func TestP5R3AllowlistCannotBypassClosedRegistration(t *testing.T) {
	ctx := context.Background()
	h := p5Handler(t, Config{
		AllowSignup:               true,
		LocalPasswordLoginEnabled: true,
		AllowedEmails:             []string{"p5-r3-allowlisted@example.test"},
		AllowedEmailDomains:       []string{"r3-domain.example.test"},
	})
	emails := []string{
		"p5-r3-allowlisted@example.test",
		"p5-r3-domain-user@r3-domain.example.test",
	}
	usernames := []string{"p5.r3.allowlisted", "p5.r3.domain"}
	dbfx.Cleanup(t, `DELETE FROM instance_setting WHERE key = $1`, openRegistrationSettingKey)
	for _, email := range emails {
		dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	}
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, "p5-r3-outsider@example.test")
	for _, username := range append(usernames, "p5.r3.outsider") {
		dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = $1`, username)
	}
	// Start from the documented default: the instance switch is off.
	dbfx.Exec(t, `DELETE FROM instance_setting WHERE key = $1`, openRegistrationSettingKey)

	body := func(i int) map[string]string {
		return map[string]string{
			"email":    emails[i],
			"name":     "P5 R3 User",
			"username": usernames[i],
			"password": "correct horse battery staple",
		}
	}
	assertRejectedWithoutAccount := func(i int, code string) {
		t.Helper()
		resp := testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", body(i))).Want(http.StatusForbidden)
		if got := resp.Map()["code"]; got != code {
			t.Fatalf("expected error %q, got %v", code, got)
		}
		p5R3AssertNoAccount(t, emails[i], usernames[i])
		p5R3AssertNoSession(t, resp)
	}

	// Switch off (default): the allowlist must not admit either address.
	assertRejectedWithoutAccount(0, "registration_disabled")
	assertRejectedWithoutAccount(1, "registration_disabled")

	// Switch on: the allowlisted email and domain addresses may register.
	if _, err := testHandler.Queries.UpsertInstanceSetting(ctx, db.UpsertInstanceSettingParams{
		Key:   openRegistrationSettingKey,
		Value: []byte(`{"enabled":true}`),
	}); err != nil {
		t.Fatalf("enable open registration: %v", err)
	}
	t.Cleanup(func() { setOpenRegistrationValue(context.Background(), true) })
	for i := range emails {
		resp := testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", body(i))).Want(http.StatusOK)
		// A self-registered address is not a proven identity.
		if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1 AND email_verified = false`, emails[i]); got != 1 {
			t.Fatalf("open registration must create %s unverified, got %d matching rows", emails[i], got)
		}
		if len(resp.Result().Cookies()) == 0 {
			t.Fatal("successful registration must establish a session")
		}
	}

	// Open, but the address does not match the allowlist: refused, no account.
	outsider := map[string]string{
		"email":    "p5-r3-outsider@example.test",
		"name":     "P5 R3 Outsider",
		"username": "p5.r3.outsider",
		"password": "correct horse battery staple",
	}
	resp := testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", outsider)).Want(http.StatusForbidden)
	if got := resp.Map()["code"]; got != "email_not_allowed" {
		t.Fatalf("expected email_not_allowed for a non-matching address, got %v", got)
	}
	p5R3AssertNoAccount(t, outsider["email"], outsider["username"])
	p5R3AssertNoSession(t, resp)
}
