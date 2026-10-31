package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// p5Handler builds a handler wired to the shared test database with the P5
// limiters generous enough not to interfere with non-rate-limit cases.
func p5Handler(t *testing.T, cfg Config) *Handler {
	t.Helper()
	h := newTestHandler(cfg)
	h.Queries = testHandler.Queries
	h.TxStarter = testHandler.TxStarter
	h.DB = testHandler.DB
	h.PATCache = testHandler.PATCache
	// Wire the real hub and event bus: password-change and delete handlers
	// publish/close sessions through them, and a nil bus would panic on the
	// publish paths these tests exercise.
	h.Hub = testHandler.Hub
	h.Bus = testHandler.Bus
	h.PasswordLimiter = middleware.NewPasswordAttemptLimiter(1_000, 1_000, time.Minute)
	h.RegistrationLimiter = middleware.NewPasswordAttemptLimiter(1_000, 1_000, time.Minute)
	h.RedemptionLimiter = middleware.NewPasswordAttemptLimiter(1_000, 1_000, time.Minute)
	return h
}

func p5JSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	req := testutil.JSONRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func p5SeedUser(t *testing.T, email string) (string, pgtype.UUID) {
	t.Helper()
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	userID := dbfx.Insert(t, "user", testutil.Cols{"name": "P5 Test User", "email": email})
	uuid, err := util.ParseUUID(userID)
	if err != nil {
		t.Fatalf("parse user id: %v", err)
	}
	return userID, uuid
}

// TestP5InviteRedeemIsSingleUse covers: invitation redemption sets the first
// password, returns the standard login response, consumes the invitation, and
// a replay of the same token fails without touching the credential.
func TestP5InviteRedeemIsSingleUse(t *testing.T) {
	ctx := context.Background()
	email := "p5-invite-single@example.com"
	username := "p5.invite.single"
	_, userUUID := p5SeedUser(t, email)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = $1`, username)
	dbfx.Cleanup(t, `DELETE FROM registration_invite WHERE user_id = $1`, userUUID)

	raw, err := auth.NewRegistrationInviteToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	invite, err := testHandler.Queries.CreateRegistrationInvite(ctx, db.CreateRegistrationInviteParams{
		UserID:    userUUID,
		TokenHash: auth.RegistrationTokenHash(raw),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	_ = invite

	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})
	body := map[string]string{
		"token":    raw,
		"name":     "P5 Invitee",
		"username": username,
		"password": "correct horse battery staple",
	}
	resp := testutil.Call(t, h.RedeemRegistrationInvite, p5JSONRequest(t, http.MethodPost, "/api/register/invite", body)).Want(http.StatusOK)
	var login LoginResponse
	resp.JSON(&login)
	if login.Token == "" || login.User.Email != email {
		t.Fatalf("unexpected login response: %+v", login)
	}
	if strings.Contains(resp.Text(), body["password"]) {
		t.Fatal("registration response echoes the password")
	}
	if _, err := testHandler.Queries.GetLocalPasswordCredentialByUserID(ctx, userUUID); err != nil {
		t.Fatalf("credential was not created: %v", err)
	}
	// Replay must fail with the same generic code and leave the credential alone.
	testutil.Call(t, h.RedeemRegistrationInvite, p5JSONRequest(t, http.MethodPost, "/api/register/invite", body)).Want(http.StatusBadRequest)
}

// TestP5InviteRedemptionRequiresInviteTransport: the endpoint is JSON-only and
// refuses a foreign Origin before doing any work.
func TestP5InviteRedemptionTransportGuards(t *testing.T) {
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})
	req := testutil.JSONRequest(http.MethodPost, "/api/register/invite", map[string]string{"token": "x"})
	req.Header.Set("Content-Type", "text/plain")
	testutil.Call(t, h.RedeemRegistrationInvite, req).Want(http.StatusUnsupportedMediaType)

	req = testutil.JSONRequest(http.MethodPost, "/api/register/invite", map[string]string{"token": "x"})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	testutil.Call(t, h.RedeemRegistrationInvite, req).Want(http.StatusForbidden)
}

// TestP5OpenRegistrationSwitch covers the persistent instance switch: off by
// default (server-side rejection) and working when an administrator enables it.
func TestP5OpenRegistrationSwitch(t *testing.T) {
	ctx := context.Background()
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})
	email := "p5-open-registration@example.com"
	username := "p5.open.registration"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = $1`, username)
	setOpenRegistration(t, false)

	body := map[string]string{
		"email":    email,
		"name":     "P5 Open User",
		"username": username,
		"password": "correct horse battery staple",
	}
	h.cfg.AllowSignup = true
	// Default: closed.
	testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", body)).Want(http.StatusForbidden)

	if _, err := testHandler.Queries.UpsertInstanceSetting(ctx, db.UpsertInstanceSettingParams{
		Key:   openRegistrationSettingKey,
		Value: []byte(`{"enabled":true}`),
	}); err != nil {
		t.Fatalf("enable switch: %v", err)
	}
	resp := testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", body)).Want(http.StatusOK)
	var login LoginResponse
	resp.JSON(&login)
	if login.Token == "" || login.User.Email != email {
		t.Fatalf("unexpected signup response: %+v", login)
	}
	if login.User.IsInstanceAdmin {
		t.Fatal("open registration must create an ordinary user")
	}
	// Duplicate email must not create a second account or burn anything.
	testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", body)).Want(http.StatusConflict)
}

// TestP5ChangePasswordRevokesOldSessions: changing the password bumps
// auth_version so previously issued session tokens fail the middleware check,
// and the previous password stops working.
func TestP5ChangePasswordRevokesOldSessions(t *testing.T) {
	ctx := context.Background()
	email := "p5-change-password@example.com"
	username := "p5.change.password"
	userID, userUUID := p5SeedUser(t, email)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE user_id = $1`, userUUID)
	oldPassword := "correct horse battery staple"
	newPassword := "an entirely different passphrase"
	hash, err := auth.HashLocalPassword(oldPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := testHandler.Queries.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID:             userUUID,
		UsernameNormalized: username,
		PasswordHash:       hash,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})

	req := p5JSONRequest(t, http.MethodPost, "/api/me/password", map[string]string{
		"current_password": "wrong password entirely",
		"new_password":     newPassword,
	})
	req.Header.Set("X-User-ID", userID)
	testutil.Call(t, h.ChangeCurrentPassword, req).Want(http.StatusBadRequest)

	req = p5JSONRequest(t, http.MethodPost, "/api/me/password", map[string]string{
		"current_password": oldPassword,
		"new_password":     newPassword,
	})
	req.Header.Set("X-User-ID", userID)
	resp := testutil.Call(t, h.ChangeCurrentPassword, req).Want(http.StatusOK)
	if !strings.Contains(resp.Text(), "reauth_required") {
		t.Fatalf("unexpected change response: %s", resp.Text())
	}
	user, err := testHandler.Queries.GetUser(ctx, userUUID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.AuthVersion <= 0 {
		t.Fatal("password change did not bump auth_version")
	}
	credential, err := testHandler.Queries.GetLocalPasswordCredentialByUserID(ctx, userUUID)
	if err != nil {
		t.Fatalf("reload credential: %v", err)
	}
	if auth.VerifyLocalPassword(credential.PasswordHash, oldPassword) {
		t.Fatal("the old password still verifies")
	}
	if !auth.VerifyLocalPassword(credential.PasswordHash, newPassword) {
		t.Fatal("the new password does not verify")
	}
}

// TestP5InstanceAdminGuards: a workspace owner without the instance flag cannot
// read the user list, and an instance administrator can.
func TestP5InstanceAdminGuards(t *testing.T) {
	ctx := context.Background()
	plainEmail := "p5-plain-user@example.com"
	adminEmail := "p5-instance-admin@example.com"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email IN ($1, $2)`, plainEmail, adminEmail)
	plainID := dbfx.Insert(t, "user", testutil.Cols{"name": "P5 Plain", "email": plainEmail})
	adminID := dbfx.Insert(t, "user", testutil.Cols{"name": "P5 Admin", "email": adminEmail})
	_, adminUUID := p5SeedUserByID(t, adminID)
	dbfx.Cleanup(t, `UPDATE "user" SET is_instance_admin = false WHERE id IN ($1, $2)`, adminID, plainID)
	if err := testHandler.Queries.SetUserInstanceAdmin(ctx, db.SetUserInstanceAdminParams{ID: adminUUID, IsInstanceAdmin: true}); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})

	req := testutil.JSONRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("X-User-ID", plainID)
	testutil.Call(t, h.ListInstanceUsers, req).Want(http.StatusForbidden)

	req = testutil.JSONRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("X-User-ID", adminID)
	testutil.Call(t, h.ListInstanceUsers, req).Want(http.StatusOK)

	// Self-deletion is refused.
	req = testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+adminID, nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", adminID)
	testutil.Call(t, h.DeleteInstanceUser, req).Want(http.StatusBadRequest)
}

// TestP5AdminInviteRefusesExistingPassword: a first-time-set invitation must
// never be usable as a password reset override.
func TestP5AdminInviteRefusesExistingPassword(t *testing.T) {
	ctx := context.Background()
	adminEmail := "p5-reset-admin@example.com"
	email := "p5-invite-existing@example.com"
	username := "p5.invite.existing"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email IN ($1, $2)`, adminEmail, email)
	adminID := dbfx.Insert(t, "user", testutil.Cols{"name": "P5 Reset Admin", "email": adminEmail})
	adminUUID, err := util.ParseUUID(adminID)
	if err != nil {
		t.Fatalf("parse admin id: %v", err)
	}
	dbfx.Cleanup(t, `UPDATE "user" SET is_instance_admin = false WHERE id = $1`, adminID)
	if err := testHandler.Queries.SetUserInstanceAdmin(ctx, db.SetUserInstanceAdminParams{ID: adminUUID, IsInstanceAdmin: true}); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	_, userUUID := p5SeedUser(t, email)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE user_id = $1`, userUUID)
	hash, err := auth.HashLocalPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := testHandler.Queries.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID: userUUID, UsernameNormalized: username, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})
	targetID := uuidToString(userUUID)
	req := testutil.JSONRequest(http.MethodPost, "/api/admin/users/"+targetID+"/invite", nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", targetID)
	testutil.Call(t, h.RegenerateInstanceInvite, req).Want(http.StatusConflict)
}

func p5SeedUserByID(t *testing.T, userID string) (string, pgtype.UUID) {
	t.Helper()
	uuid, err := util.ParseUUID(userID)
	if err != nil {
		t.Fatalf("parse id: %v", err)
	}
	return userID, uuid
}
