package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

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
		if _, err := testHandler.Queries.GetUserByEmail(ctx, emails[i]); err == nil {
			t.Fatalf("closed registration created a user for %s", emails[i])
		}
		if _, err := testHandler.Queries.GetLocalPasswordCredentialByUsername(ctx, usernames[i]); err == nil {
			t.Fatalf("closed registration created a credential for %s", usernames[i])
		}
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
	for i := range emails {
		testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", body(i))).Want(http.StatusOK)
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
	if _, err := testHandler.Queries.GetUserByEmail(ctx, outsider["email"]); err == nil {
		t.Fatal("non-matching address created a user under the allowlist")
	}
}

// TestP5R3InvitationRedemptionStaysAvailable keeps the "existing admin
// invitation/password setup keeps working" contract: the open-registration
// switch stays off in TestP5InviteRedeemIsSingleUse, and redemption must not
// depend on it. This asserts the policy entry point itself does not gate
// redemption.
func TestP5R3InvitationRedemptionStaysAvailable(t *testing.T) {
	ctx := context.Background()
	h := p5Handler(t, Config{AllowSignup: false, LocalPasswordLoginEnabled: true})
	dbfx.Cleanup(t, `DELETE FROM instance_setting WHERE key = $1`, openRegistrationSettingKey)
	dbfx.Exec(t, `DELETE FROM instance_setting WHERE key = $1`, openRegistrationSettingKey)
	// The admin invitation flow creates the account explicitly and does not run
	// the self-service signup policy; a deployment with signup hard-off must
	// still let an existing account set its first password.
	email := "p5-r3-invite-existing-account@example.test"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	if err := h.checkSignupAllowed(ctx, email, false); err != nil {
		t.Fatalf("existing account sign-in must not be gated: %v", err)
	}
	if err := h.checkSignupAllowed(ctx, email, true); err == nil {
		t.Fatal("new account creation must stay closed")
	}
}
