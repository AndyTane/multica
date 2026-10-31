package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// P5-R1 regressions: session revocation across HTTP and WebSocket, unverified
// email identity, account consistency under concurrency, delete preview /
// delete parity, and the exact schema bindings the delete handles.

// p5r1ResetAdmins clears the instance administrator flag for every account so a
// test that counts administrators starts from a known state. Individual tests
// grant the flag to the accounts they need.
func p5r1ResetAdmins(t *testing.T) {
	t.Helper()
	if testPool == nil {
		t.Skip("database not available")
	}
	dbfx.Exec(t, `UPDATE "user" SET is_instance_admin = false`)
}

func p5r1CreateUser(t *testing.T, email string, admin bool) (string, pgtype.UUID) {
	t.Helper()
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	id := dbfx.Insert(t, "user", testutil.Cols{"name": "P5R1 " + email, "email": email})
	uuid, err := util.ParseUUID(id)
	if err != nil {
		t.Fatalf("parse user id: %v", err)
	}
	if admin {
		if err := testHandler.Queries.SetUserInstanceAdmin(context.Background(), db.SetUserInstanceAdminParams{
			ID: uuid, IsInstanceAdmin: true,
		}); err != nil {
			t.Fatalf("grant admin: %v", err)
		}
	}
	return id, uuid
}

func p5r1SignJWT(t *testing.T, userID string, version int64) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"sid": "p5r1",
		"ver": version,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	signed, err := token.SignedString(auth.JWTSecret())
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}

func p5r1MiddlewareRequest(t *testing.T, h *Handler, token string) int {
	t.Helper()
	protected := middleware.Auth(h.Queries, nil, nil, nil)(http.HandlerFunc(h.GetMe))
	req := testutil.JSONRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := testutil.Call(t, protected.ServeHTTP, req)
	return rec.Code
}

type p5r1DeletePreview struct {
	CanDelete bool `json:"can_delete"`
	Blockers  []struct {
		Code  string `json:"code"`
		Count int64  `json:"count"`
	} `json:"blockers"`
	Removes []string `json:"removes"`
}

func p5r1Preview(t *testing.T, h *Handler, adminID, targetID string) (int, p5r1DeletePreview) {
	t.Helper()
	req := testutil.JSONRequest(http.MethodGet, "/api/admin/users/"+targetID+"/delete-preview", nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", targetID)
	resp := testutil.Call(t, h.GetInstanceUserDeletePreview, req)
	var preview p5r1DeletePreview
	if resp.Code == http.StatusOK {
		if err := json.Unmarshal(resp.Body.Bytes(), &preview); err != nil {
			t.Fatalf("decode preview: %v (body %s)", err, resp.Body.String())
		}
	}
	return resp.Code, preview
}

func p5r1BlockerCodes(preview p5r1DeletePreview) []string {
	codes := make([]string, 0, len(preview.Blockers))
	for _, blocker := range preview.Blockers {
		codes = append(codes, blocker.Code)
	}
	return codes
}

func p5r1Contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestP5R1DeletePreviewContract: the preview reports the same inventory DELETE
// uses, refuses to authorize anything itself (can_delete is decided by the same
// function), and rejects non-admins, machine credentials and non-UUID ids.
func TestP5R1DeletePreviewContract(t *testing.T) {
	p5r1ResetAdmins(t)
	adminID, _ := p5r1CreateUser(t, "p5r1-preview-admin@example.com", true)
	freeID, _ := p5r1CreateUser(t, "p5r1-preview-free@example.com", false)
	ownerEmail := "p5r1-preview-owner@example.com"
	ownerID, ownerUUID := p5r1CreateUser(t, ownerEmail, false)
	workspaceID := dbfx.Workspace(t, "P5R1 Owned", "p5r1-owned-workspace")
	dbfx.Member(t, workspaceID, ownerID, "owner")
	dbfx.Cleanup(t, `DELETE FROM workspace WHERE id = $1`, workspaceID)

	h := p5Handler(t, Config{})

	status, preview := p5r1Preview(t, h, adminID, freeID)
	if status != http.StatusOK {
		t.Fatalf("preview status = %d", status)
	}
	if !preview.CanDelete {
		t.Fatalf("a bare account must be deletable, blockers=%v", p5r1BlockerCodes(preview))
	}
	if !p5r1Contains(preview.Removes, "account") {
		t.Fatalf("removes = %v, want account", preview.Removes)
	}
	if len(preview.Blockers) != 0 {
		t.Fatalf("blockers = %v, want none", p5r1BlockerCodes(preview))
	}

	status, preview = p5r1Preview(t, h, adminID, ownerID)
	if status != http.StatusOK {
		t.Fatalf("owner preview status = %d", status)
	}
	if preview.CanDelete {
		t.Fatal("workspace owner must not be deletable")
	}
	if !p5r1Contains(p5r1BlockerCodes(preview), "workspace_owner") {
		t.Fatalf("blockers = %v, want workspace_owner", p5r1BlockerCodes(preview))
	}
	// The preview still reports what the delete would remove, so the operator
	// sees the whole picture next to the blocker that disables confirmation.
	if !p5r1Contains(preview.Removes, "membership") {
		t.Fatalf("removes = %v, want membership", preview.Removes)
	}
	_ = ownerUUID

	status, preview = p5r1Preview(t, h, adminID, adminID)
	if status != http.StatusOK || preview.CanDelete {
		t.Fatalf("self preview status=%d can_delete=%v", status, preview.CanDelete)
	}
	if !p5r1Contains(p5r1BlockerCodes(preview), "cannot_delete_self") {
		t.Fatalf("self blockers = %v", p5r1BlockerCodes(preview))
	}

	// A non-UUID path segment is a client error, never a panic.
	req := testutil.JSONRequest(http.MethodGet, "/api/admin/users/not-a-uuid/delete-preview", nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", "not-a-uuid")
	testutil.Call(t, h.GetInstanceUserDeletePreview, req).Want(http.StatusBadRequest)

	// An ordinary account cannot read the preview.
	req = testutil.JSONRequest(http.MethodGet, "/api/admin/users/"+freeID+"/delete-preview", nil)
	req.Header.Set("X-User-ID", freeID)
	req = withURLParam(req, "id", freeID)
	testutil.Call(t, h.GetInstanceUserDeletePreview, req).Want(http.StatusForbidden)

	// Machine credentials are refused even with the caller's user id set.
	req = testutil.JSONRequest(http.MethodGet, "/api/admin/users/"+freeID+"/delete-preview", nil)
	req.Header.Set("X-User-ID", adminID)
	req.Header.Set("X-Actor-Source", "task_token")
	req = withURLParam(req, "id", freeID)
	testutil.Call(t, h.GetInstanceUserDeletePreview, req).Want(http.StatusForbidden)
}

// TestP5R1DeleteRejectsNonUUIDAndRevalidates: DELETE refuses a malformed id and
// refuses a target that gained a dependency after the operator last looked.
func TestP5R1DeleteRejectsNonUUIDAndRevalidates(t *testing.T) {
	p5r1ResetAdmins(t)
	adminID, _ := p5r1CreateUser(t, "p5r1-delete-admin@example.com", true)
	targetEmail := "p5r1-delete-target@example.com"
	targetID, _ := p5r1CreateUser(t, targetEmail, false)
	h := p5Handler(t, Config{})

	req := testutil.JSONRequest(http.MethodDelete, "/api/admin/users/not-a-uuid", nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", "not-a-uuid")
	testutil.Call(t, h.DeleteInstanceUser, req).Want(http.StatusBadRequest)

	// The dependency is created after the preview said the account was free:
	// DELETE must re-check inside its transaction and refuse.
	status, preview := p5r1Preview(t, h, adminID, targetID)
	if status != http.StatusOK || !preview.CanDelete {
		t.Fatalf("precondition preview status=%d can_delete=%v", status, preview.CanDelete)
	}
	agentID := dbfx.Agent(t, "P5R1 Owned Agent", dbfx.Runtime(t, "P5R1 Runtime"))
	dbfx.Exec(t, `UPDATE agent SET owner_id = $1 WHERE id = $2`, targetID, agentID)
	dbfx.Cleanup(t, `UPDATE agent SET owner_id = NULL WHERE id = $1`, agentID)

	req = testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+targetID, nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", targetID)
	resp := testutil.Call(t, h.DeleteInstanceUser, req).Want(http.StatusConflict)
	if !strings.Contains(resp.Body.String(), "agent_owner") {
		t.Fatalf("delete conflict body = %s, want agent_owner", resp.Body.String())
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE id = $1`, targetID); got != 1 {
		t.Fatalf("refused delete removed the account (count=%d)", got)
	}
}

// TestP5R1DeleteHandlesIdentityBindingsAndKeepsSharedHistory: the delete removes
// the account's identity bindings and subscriptions, detaches authorship on
// shared history instead of deleting it, and reports both in the preview.
func TestP5R1DeleteHandlesIdentityBindingsAndKeepsSharedHistory(t *testing.T) {
	p5r1ResetAdmins(t)
	adminID, _ := p5r1CreateUser(t, "p5r1-bind-admin@example.com", true)
	targetEmail := "p5r1-bind-target@example.com"
	targetID, targetUUID := p5r1CreateUser(t, targetEmail, false)
	ctx := context.Background()

	hash, err := auth.HashLocalPassword("a password for the binding test")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = 'p5r1.binding'`)
	dbfx.Cleanup(t, `DELETE FROM registration_invite WHERE token_hash = 'p5r1-binding-hash'`)
	if _, err := testHandler.Queries.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID: targetUUID, UsernameNormalized: "p5r1.binding", PasswordHash: hash,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	if _, err := testHandler.Queries.CreateRegistrationInvite(ctx, db.CreateRegistrationInviteParams{
		UserID: targetUUID, TokenHash: "p5r1-binding-hash", ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("seed invite: %v", err)
	}
	dbfx.Insert(t, "personal_access_token", testutil.Cols{
		"user_id": targetID, "name": "test", "token_hash": "p5r1-pat-hash", "token_prefix": "mul_p5r1",
	})
	installationID := "11111111-2222-3333-4444-555555555555"
	dbfx.Insert(t, "channel_user_binding", testutil.Cols{
		"workspace_id": testWorkspaceID, "multica_user_id": targetID,
		"installation_id": installationID, "channel_type": "slack", "channel_user_id": "U-p5r1",
	})
	// lark_user_binding carries real foreign keys to its installation and to the
	// workspace membership, so this case needs both (unlike the FK-less tables).
	dbfx.Member(t, testWorkspaceID, targetID, "member")
	larkInstallation := dbfx.Insert(t, "lark_installation", testutil.Cols{
		"workspace_id": testWorkspaceID, "agent_id": dbfx.Agent(t, "P5R1 Lark Agent", dbfx.Runtime(t, "P5R1 Lark Runtime")),
		"app_id": "cli_p5r1", "app_secret_encrypted": "test-only-placeholder", "bot_open_id": "ou_bot_p5r1",
		"installer_user_id": testUserID,
	})
	dbfx.Cleanup(t, `DELETE FROM lark_user_binding WHERE installation_id = $1`, larkInstallation)
	dbfx.Cleanup(t, `DELETE FROM lark_installation WHERE id = $1`, larkInstallation)
	dbfx.Insert(t, "lark_user_binding", testutil.Cols{
		"workspace_id": testWorkspaceID, "multica_user_id": targetID,
		"installation_id": larkInstallation, "lark_open_id": "ou-p5r1",
	})
	issueID := dbfx.Issue(t, "P5R1 shared history")
	dbfx.InsertNoID(t, "issue_subscriber", testutil.Cols{
		"issue_id": issueID, "user_type": "member", "user_id": targetID, "reason": "manual",
	}, "issue_id = $1 AND user_id = $2", issueID, targetID)
	skillID := dbfx.Insert(t, "skill", testutil.Cols{
		"workspace_id": testWorkspaceID, "name": "P5R1 Shared Skill", "created_by": targetID,
	})
	dbfx.Cleanup(t, `DELETE FROM skill WHERE id = $1`, skillID)
	dbfx.Cleanup(t, `DELETE FROM issue_subscriber WHERE issue_id = $1`, issueID)

	h := p5Handler(t, Config{})
	status, preview := p5r1Preview(t, h, adminID, targetID)
	if status != http.StatusOK {
		t.Fatalf("preview status = %d", status)
	}
	if !preview.CanDelete {
		t.Fatalf("target must be deletable, blockers=%v", p5r1BlockerCodes(preview))
	}
	for _, want := range []string{"identity_binding", "password", "personal_access_token", "issue_subscription", "registration_invite"} {
		if !p5r1Contains(preview.Removes, want) {
			t.Errorf("removes = %v, want %s", preview.Removes, want)
		}
	}

	req := testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+targetID, nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", targetID)
	testutil.Call(t, h.DeleteInstanceUser, req).Want(http.StatusNoContent)

	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE id = $1`, targetID); got != 0 {
		t.Fatalf("account still exists")
	}
	for _, query := range []string{
		`SELECT count(*) FROM channel_user_binding WHERE multica_user_id = $1`,
		`SELECT count(*) FROM lark_user_binding WHERE multica_user_id = $1`,
		`SELECT count(*) FROM personal_access_token WHERE user_id = $1`,
		`SELECT count(*) FROM local_password_credential WHERE user_id = $1`,
		`SELECT count(*) FROM registration_invite WHERE user_id = $1`,
	} {
		if got := dbfx.Count(t, query, targetID); got != 0 {
			t.Fatalf("%s left %d rows", query, got)
		}
	}
	// Shared history survives with attribution detached.
	if got := dbfx.Count(t, `SELECT count(*) FROM skill WHERE id = $1 AND created_by IS NULL`, skillID); got != 1 {
		t.Fatalf("shared skill was not detached from the deleted account")
	}
}

// TestP5R1MutualAdminDeleteLeavesAnAdministrator drives the exact race the review
// found: two administrators deleting each other. The account lock plus the
// in-transaction re-check must leave at least one administrator behind.
func TestP5R1MutualAdminDeleteLeavesAnAdministrator(t *testing.T) {
	p5r1ResetAdmins(t)
	firstID, _ := p5r1CreateUser(t, "p5r1-mutual-first@example.com", true)
	secondID, _ := p5r1CreateUser(t, "p5r1-mutual-second@example.com", true)
	h := p5Handler(t, Config{})

	statuses := make([]int, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		req := testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+secondID, nil)
		req.Header.Set("X-User-ID", firstID)
		req = withURLParam(req, "id", secondID)
		statuses[0] = testutil.Call(t, h.DeleteInstanceUser, req).Code
	}()
	go func() {
		defer wg.Done()
		req := testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+firstID, nil)
		req.Header.Set("X-User-ID", secondID)
		req = withURLParam(req, "id", firstID)
		statuses[1] = testutil.Call(t, h.DeleteInstanceUser, req).Code
	}()
	wg.Wait()

	successes := 0
	for _, status := range statuses {
		if status == http.StatusNoContent {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one delete to succeed, statuses=%v", statuses)
	}
	admins, err := testHandler.Queries.CountInstanceAdmins(context.Background())
	if err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if admins < 1 {
		t.Fatalf("mutual delete left %d administrators", admins)
	}
}

// TestP5R1InviteRegenerationIsSerialized: concurrent regenerations for one
// account leave exactly one valid invitation, and only that invitation's secret
// is redeemable.
func TestP5R1InviteRegenerationIsSerialized(t *testing.T) {
	p5r1ResetAdmins(t)
	adminID, _ := p5r1CreateUser(t, "p5r1-regen-admin@example.com", true)
	targetEmail := "p5r1-regen-target@example.com"
	targetID, targetUUID := p5r1CreateUser(t, targetEmail, false)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE user_id = $1`, targetUUID)
	dbfx.Cleanup(t, `DELETE FROM registration_invite WHERE user_id = $1 OR created_by = $1`, targetID)
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})

	const attempts = 6
	tokens := make([]string, attempts)
	statuses := make([]int, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			req := testutil.JSONRequest(http.MethodPost, "/api/admin/users/"+targetID+"/invite", nil)
			req.Header.Set("X-User-ID", adminID)
			req = withURLParam(req, "id", targetID)
			resp := testutil.Call(t, h.RegenerateInstanceInvite, req)
			statuses[index] = resp.Code
			if resp.Code == http.StatusOK {
				var body struct {
					InviteToken string `json:"invite_token"`
				}
				if err := json.Unmarshal(resp.Body.Bytes(), &body); err == nil {
					tokens[index] = body.InviteToken
				}
			}
		}(i)
	}
	wg.Wait()
	for i, status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("regenerate %d status = %d", i, status)
		}
		if tokens[i] == "" {
			t.Fatalf("regenerate %d returned no token", i)
		}
	}
	if active := dbfx.Count(t, `SELECT count(*) FROM registration_invite WHERE user_id = $1 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > now()`, targetUUID); active != 1 {
		t.Fatalf("active invitations = %d, want exactly 1", active)
	}
	valid := 0
	for _, token := range tokens {
		if dbfx.Count(t, `SELECT count(*) FROM registration_invite WHERE user_id = $1 AND token_hash = $2 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > now()`, targetUUID, auth.RegistrationTokenHash(token)) == 1 {
			valid++
		}
	}
	if valid != 1 {
		t.Fatalf("%d of the returned invitation secrets are valid, want exactly 1", valid)
	}
}

// TestP5R1RedemptionIsSingleUseUnderConcurrency: eight concurrent redemptions of
// one secret produce one credential and one success.
func TestP5R1RedemptionIsSingleUseUnderConcurrency(t *testing.T) {
	targetEmail := "p5r1-redeem-target@example.com"
	_, targetUUID := p5r1CreateUser(t, targetEmail, false)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE user_id = $1`, targetUUID)
	dbfx.Cleanup(t, `DELETE FROM registration_invite WHERE user_id = $1`, targetUUID)
	dbfx.Exec(t, `DELETE FROM local_password_credential WHERE username_normalized LIKE 'p5r1.race.%'`)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized LIKE 'p5r1.race.%'`)

	raw, err := auth.NewRegistrationInviteToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := testHandler.Queries.CreateRegistrationInvite(context.Background(), db.CreateRegistrationInviteParams{
		UserID: targetUUID, TokenHash: auth.RegistrationTokenHash(raw), ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("seed invite: %v", err)
	}
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})

	const attempts = 8
	statuses := make([]int, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			req := p5JSONRequest(t, http.MethodPost, "/api/register/invite", map[string]string{
				"token":    raw,
				"name":     "P5R1 Race",
				"username": "p5r1.race." + string(rune('a'+index)),
				"password": "a sufficiently long password",
			})
			statuses[index] = testutil.Call(t, h.RedeemRegistrationInvite, req).Code
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, status := range statuses {
		if status == http.StatusOK {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("redemption successes = %d, want 1 (statuses %v)", successes, statuses)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM local_password_credential WHERE user_id = $1`, targetUUID); got != 1 {
		t.Fatalf("credentials for the account = %d, want 1", got)
	}
}

// TestP5R1PasswordChangeAcceptsOldCredentialOnce: two concurrent changes using the
// same current password cannot both succeed.
func TestP5R1PasswordChangeAcceptsOldCredentialOnce(t *testing.T) {
	_, userUUID := p5r1CreateUser(t, "p5r1-change-race@example.com", false)
	userID := uuidToString(userUUID)
	username := "p5r1.change.race"
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = $1`, username)
	oldPassword := "the original correct password"
	hash, err := auth.HashLocalPassword(oldPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := testHandler.Queries.CreateLocalPasswordCredential(context.Background(), db.CreateLocalPasswordCredentialParams{
		UserID: userUUID, UsernameNormalized: username, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})

	newPasswords := []string{"first replacement password", "second replacement password"}
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			req := p5JSONRequest(t, http.MethodPost, "/api/me/password", map[string]string{
				"current_password": oldPassword,
				"new_password":     newPasswords[index],
			})
			req.Header.Set("X-User-ID", userID)
			statuses[index] = testutil.Call(t, h.ChangeCurrentPassword, req).Code
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, status := range statuses {
		if status == http.StatusOK {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("password changes succeeded %d times, want exactly 1 (statuses %v)", successes, statuses)
	}
	for _, status := range statuses {
		if status != http.StatusOK && status != http.StatusBadRequest {
			t.Fatalf("losing password change status = %d, want 400", status)
		}
	}
	credential, err := testHandler.Queries.GetLocalPasswordCredentialByUserID(context.Background(), userUUID)
	if err != nil {
		t.Fatalf("reload credential: %v", err)
	}
	if auth.VerifyLocalPassword(credential.PasswordHash, oldPassword) {
		t.Fatal("the old password still verifies")
	}
	verifiedNew := 0
	for _, candidate := range newPasswords {
		if auth.VerifyLocalPassword(credential.PasswordHash, candidate) {
			verifiedNew++
		}
	}
	if verifiedNew != 1 {
		t.Fatalf("%d of the competing new passwords verify, want exactly 1", verifiedNew)
	}
}

// TestP5R1PasswordLoginCannotOutliveAConcurrentChange: logins racing a password
// change must never end up holding a token for the post-change session. Every
// old-password login that succeeded is checked through the HTTP middleware after
// the change committed.
func TestP5R1PasswordLoginCannotOutliveAConcurrentChange(t *testing.T) {
	_, userUUID := p5r1CreateUser(t, "p5r1-login-race@example.com", false)
	userID := uuidToString(userUUID)
	username := "p5r1.login.race"
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = $1`, username)
	oldPassword := "the password before the change"
	newPassword := "the password after the change"
	hash, err := auth.HashLocalPassword(oldPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := testHandler.Queries.CreateLocalPasswordCredential(context.Background(), db.CreateLocalPasswordCredentialParams{
		UserID: userUUID, UsernameNormalized: username, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})

	const loginAttempts = 8
	results := make([]int, loginAttempts)
	tokens := make([]string, loginAttempts)
	var wg sync.WaitGroup
	wg.Add(loginAttempts + 1)
	go func() {
		defer wg.Done()
		req := p5JSONRequest(t, http.MethodPost, "/api/me/password", map[string]string{
			"current_password": oldPassword,
			"new_password":     newPassword,
		})
		req.Header.Set("X-User-ID", userID)
		testutil.Call(t, h.ChangeCurrentPassword, req)
	}()
	for i := 0; i < loginAttempts; i++ {
		go func(index int) {
			defer wg.Done()
			req := p5JSONRequest(t, http.MethodPost, "/auth/password", map[string]string{
				"username": username, "password": oldPassword,
			})
			resp := testutil.Call(t, h.PasswordLogin, req)
			results[index] = resp.Code
			if resp.Code == http.StatusOK {
				var body struct {
					Token string `json:"token"`
				}
				if err := json.Unmarshal(resp.Body.Bytes(), &body); err == nil {
					tokens[index] = body.Token
				}
			}
		}(i)
	}
	wg.Wait()

	current, err := testHandler.Queries.GetUser(context.Background(), userUUID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	for i, status := range results {
		if status != http.StatusOK {
			continue
		}
		if p5r1MiddlewareRequest(t, h, tokens[i]) != http.StatusUnauthorized {
			t.Fatalf("an old-password login token from attempt %d still authenticates after the change", i)
		}
	}
	// And the new password yields a session for the current version.
	req := p5JSONRequest(t, http.MethodPost, "/auth/password", map[string]string{
		"username": username, "password": newPassword,
	})
	resp := testutil.Call(t, h.PasswordLogin, req).Want(http.StatusOK)
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if p5r1MiddlewareRequest(t, h, body.Token) != http.StatusOK {
		t.Fatal("a fresh login with the new password does not authenticate")
	}
	if current.AuthVersion < 1 {
		t.Fatalf("auth_version = %d, want >= 1", current.AuthVersion)
	}
}

// TestP5R1UnverifiedEmailCannotClaimInvitations: an account created by open
// registration cannot list or accept a workspace invitation addressed to the
// mailbox it typed. The verified control case keeps working.
func TestP5R1UnverifiedEmailCannotClaimInvitations(t *testing.T) {
	ctx := context.Background()
	victimEmail := "p5r1-victim@example.com"
	targetEmail := "p5r1-victim-target@example.com"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email IN ($1, $2)`, victimEmail, targetEmail)
	dbfx.Cleanup(t, `DELETE FROM workspace_invitation WHERE invitee_email IN ($1, $2)`, victimEmail, targetEmail)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized LIKE 'p5r1.claim%'`)

	// An invitation addressed to the victim mailbox, not yet bound to any user.
	row := dbfx.Insert(t, "workspace_invitation", testutil.Cols{
		"workspace_id": testWorkspaceID, "inviter_id": testUserID,
		"invitee_email": victimEmail, "role": "member", "status": "pending",
	})
	// Open registration is the switch the attacker needs.
	if _, err := testHandler.Queries.UpsertInstanceSetting(ctx, db.UpsertInstanceSettingParams{
		Key: openRegistrationSettingKey, Value: []byte(`{"enabled":true}`),
	}); err != nil {
		t.Fatalf("enable open registration: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testHandler.Queries.UpsertInstanceSetting(context.Background(), db.UpsertInstanceSettingParams{
			Key: openRegistrationSettingKey, Value: []byte(`{"enabled":false}`),
		})
	})

	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true})
	h.Bus = testHandler.Bus
	resp := testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", map[string]string{
		"email": victimEmail, "name": "P5R1 Claim", "username": "p5r1.claim.attacker",
		"password": "a sufficiently long password",
	})).Want(http.StatusOK)
	var registered struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &registered); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	attackerID := registered.User.ID
	if attackerID == "" {
		t.Fatal("registration returned no user id")
	}
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE id = $1`, attackerID)
	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE id = $1 AND email_verified = false`, attackerID); got != 1 {
		t.Fatal("open registration did not create an unverified identity")
	}

	req := testutil.JSONRequest(http.MethodGet, "/api/invitations", nil)
	req.Header.Set("X-User-ID", attackerID)
	listResp := testutil.Call(t, h.ListMyInvitations, req).Want(http.StatusOK)
	if strings.Contains(listResp.Body.String(), victimEmail) {
		t.Fatalf("unverified account listed an invitation for %s: %s", victimEmail, listResp.Body.String())
	}

	req = testutil.JSONRequest(http.MethodPost, "/api/invitations/"+row+"/accept", nil)
	req.Header.Set("X-User-ID", attackerID)
	req = withURLParam(req, "id", row)
	testutil.Call(t, h.AcceptInvitation, req).Want(http.StatusForbidden)

	// Control: a verified account addressed by the same mailbox still works.
	verifiedID, _ := p5r1CreateUser(t, targetEmail, false)
	verifiedInvite := dbfx.Insert(t, "workspace_invitation", testutil.Cols{
		"workspace_id": testWorkspaceID, "inviter_id": testUserID,
		"invitee_email": targetEmail, "role": "member", "status": "pending",
	})
	dbfx.Cleanup(t, `DELETE FROM workspace_invitation WHERE id = $1`, verifiedInvite)
	dbfx.Cleanup(t, `DELETE FROM member WHERE user_id = $1`, verifiedID)
	req = testutil.JSONRequest(http.MethodGet, "/api/invitations", nil)
	req.Header.Set("X-User-ID", verifiedID)
	listResp = testutil.Call(t, h.ListMyInvitations, req).Want(http.StatusOK)
	if !strings.Contains(listResp.Body.String(), targetEmail) {
		t.Fatalf("verified account did not list its invitation: %s", listResp.Body.String())
	}
	req = testutil.JSONRequest(http.MethodPost, "/api/invitations/"+verifiedInvite+"/accept", nil)
	req.Header.Set("X-User-ID", verifiedID)
	req = withURLParam(req, "id", verifiedInvite)
	testutil.Call(t, h.AcceptInvitation, req).Want(http.StatusOK)
}
