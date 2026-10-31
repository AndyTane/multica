package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// P5-R2 backend regressions: invitation identity ordering, deletion blockers
// for chat history and live work, the account write/delete state boundary, and
// the real handler wiring that closes WebSocket sessions.

// TestP5R2InvitationCannotBeClaimedByAnEarlierUnverifiedRegistration is the
// reverse ordering the review found: an attacker registers an account with the
// victim's mailbox while open registration is on, and the workspace invitation
// is created afterwards through the real handler. CreateInvitation must not
// derive a UUID binding from an unverified account, so the attacker can neither
// list nor accept it. A verified account still accepts a legitimate invitation.
func TestP5R2InvitationCannotBeClaimedByAnEarlierUnverifiedRegistration(t *testing.T) {
	p5r1ResetAdmins(t)
	victimEmail := "p5r2-order-victim@example.com"
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, victimEmail)
	dbfx.Cleanup(t, `DELETE FROM workspace_invitation WHERE invitee_email = $1`, victimEmail)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized LIKE 'p5r2.order%'`)
	dbfx.Cleanup(t, `DELETE FROM member WHERE user_id IN (SELECT id FROM "user" WHERE email = $1)`, victimEmail)

	setOpenRegistration(t, true)
	h := p5Handler(t, Config{LocalPasswordLoginEnabled: true, AllowSignup: true})
	h.Bus = testHandler.Bus

	// The attacker registers the victim's mailbox without proving it.
	resp := testutil.Call(t, h.OpenRegistration, p5JSONRequest(t, http.MethodPost, "/api/register/open", map[string]string{
		"email": victimEmail, "name": "Squatter", "username": "p5r2.order.attacker",
		"password": "a sufficiently long password",
	})).Want(http.StatusOK)
	var registration LoginResponse
	resp.JSON(&registration)
	attackerID := registration.User.ID
	if attackerID == "" {
		t.Fatal("registration returned no user id")
	}
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE id = $1`, attackerID)

	// A workspace owner invites that mailbox through the real endpoint.
	inviteReq := p5JSONRequest(t, http.MethodPost, "/api/workspaces/"+testWorkspaceID+"/members", map[string]string{
		"email": victimEmail, "role": "member",
	})
	inviteReq.Header.Set("X-User-ID", testUserID)
	inviteReq = withURLParam(inviteReq, "id", testWorkspaceID)
	inviteResp := testutil.Call(t, h.CreateInvitation, inviteReq).Want(http.StatusCreated)
	var invitation InvitationResponse
	inviteResp.JSON(&invitation)
	if invitation.InviteeUserID != nil {
		t.Fatalf("invitation bound to the unverified account: %v", *invitation.InviteeUserID)
	}
	dbfx.Cleanup(t, `DELETE FROM workspace_invitation WHERE id = $1`, invitation.ID)
	if got := dbfx.Count(t, `SELECT count(*) FROM workspace_invitation WHERE invitee_email = $1 AND invitee_user_id IS NOT NULL`, victimEmail); got != 0 {
		t.Fatalf("%d invitations derived a UUID binding from an unverified account", got)
	}

	// The squatter cannot see it in the invitation list.
	req := testutil.JSONRequest(http.MethodGet, "/api/invitations", nil)
	req.Header.Set("X-User-ID", attackerID)
	listBody := testutil.Call(t, h.ListMyInvitations, req).Want(http.StatusOK).Body.String()
	if strings.Contains(listBody, victimEmail) {
		t.Fatalf("unverified account listed the invitation: %s", listBody)
	}

	// Nor claim it.
	req = testutil.JSONRequest(http.MethodPost, "/api/invitations/"+invitation.ID+"/accept", nil)
	req.Header.Set("X-User-ID", attackerID)
	req = withURLParam(req, "id", invitation.ID)
	testutil.Call(t, h.AcceptInvitation, req).Want(http.StatusForbidden)
	if got := dbfx.Count(t, `SELECT count(*) FROM member WHERE user_id = $1`, attackerID); got != 0 {
		t.Fatalf("refused acceptance granted %d memberships", got)
	}

	// Control: a verified account addressed by the same mailbox is bound and
	// accepts normally.
	verifiedEmail := "p5r2-order-verified@example.com"
	verifiedID, _ := p5r1CreateUser(t, verifiedEmail, false)
	dbfx.Cleanup(t, `DELETE FROM member WHERE user_id = $1`, verifiedID)
	dbfx.Cleanup(t, `DELETE FROM workspace_invitation WHERE invitee_email = $1`, verifiedEmail)
	verifiedInvite := dbfx.Insert(t, "workspace_invitation", testutil.Cols{
		"workspace_id": testWorkspaceID, "inviter_id": testUserID,
		"invitee_email": verifiedEmail, "role": "member", "status": "pending",
	})
	dbfx.Cleanup(t, `DELETE FROM workspace_invitation WHERE id = $1`, verifiedInvite)
	req = testutil.JSONRequest(http.MethodPost, "/api/invitations/"+verifiedInvite+"/accept", nil)
	req.Header.Set("X-User-ID", verifiedID)
	req = withURLParam(req, "id", verifiedInvite)
	testutil.Call(t, h.AcceptInvitation, req).Want(http.StatusOK)
	if got := dbfx.Count(t, `SELECT count(*) FROM member WHERE user_id = $1`, verifiedID); got != 1 {
		t.Fatalf("verified identity acquired %d memberships, want 1", got)
	}
}

// TestP5R2FindOrCreateUserRefusesUnverifiedSameEmailAccount: a verified
// mail-claim login must not silently reuse (and thereby inherit the password
// of) an account created by open registration.
func TestP5R2FindOrCreateUserRefusesUnverifiedSameEmailAccount(t *testing.T) {
	email := "p5r2-squatted@example.com"
	p5SeedUserWithFlag(t, email, false)
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE username_normalized = 'p5r2.squatter' AND user_id IN (SELECT id FROM "user" WHERE email = $1)`, email)
	user := mustLoadUser(t, email)
	hash, err := auth.HashLocalPassword("the attacker's chosen password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := testHandler.Queries.CreateLocalPasswordCredential(context.Background(), db.CreateLocalPasswordCredentialParams{
		UserID: user.ID, UsernameNormalized: "p5r2.squatter", PasswordHash: hash,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	if _, isNew, err := testHandler.findOrCreateUser(context.Background(), email); err == nil || isNew {
		t.Fatalf("unverified same-email account was merged: isNew=%t err=%v", isNew, err)
	} else if !strings.Contains(err.Error(), "unverified account") {
		t.Fatalf("unexpected error: %v", err)
	}
	// The row and its password survived untouched; no second account appeared.
	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); got != 1 {
		t.Fatalf("user rows = %d, want 1", got)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM local_password_credential WHERE user_id = $1`, user.ID); got != 1 {
		t.Fatalf("credentials = %d, want 1", got)
	}
}

// TestP5R2DeleteBlockedByChatHistory: chat_session cascades from "user" and
// chat_message from the session. Deletion must be refused instead of silently
// destroying that history, in both the preview and the mutation.
func TestP5R2DeleteBlockedByChatHistory(t *testing.T) {
	p5r1ResetAdmins(t)
	adminID, _ := p5r1CreateUser(t, "p5r2-chat-admin@example.com", true)
	targetID, _ := p5r1CreateUser(t, "p5r2-chat-target@example.com", false)
	agentID := dbfx.Agent(t, "P5R2 Chat Agent", dbfx.Runtime(t, "P5R2 Chat Runtime"))
	sessionID := dbfx.ChatSession(t, agentID, testutil.Cols{"creator_id": targetID})
	messageID := dbfx.Insert(t, "chat_message", testutil.Cols{
		"chat_session_id": sessionID, "role": "user", "content": "P5R2 must not delete this history",
	})
	dbfx.Cleanup(t, `DELETE FROM chat_message WHERE id = $1`, messageID)

	h := p5Handler(t, Config{})
	status, preview := p5r1Preview(t, h, adminID, targetID)
	if status != http.StatusOK {
		t.Fatalf("preview status = %d", status)
	}
	if preview.CanDelete {
		t.Fatal("an account with chat history must not be deletable")
	}
	if !blockerWithCount(preview, "chat_history", 1) {
		t.Fatalf("blockers = %v, want chat_history=1", preview.Blockers)
	}
	if p5r1Contains(preview.Removes, "chat_history") {
		t.Fatal("chat history must never be classified as a removable row")
	}

	req := testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+targetID, nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", targetID)
	resp := testutil.Call(t, h.DeleteInstanceUser, req).Want(http.StatusConflict)
	if !strings.Contains(resp.Body.String(), "chat_history") {
		t.Fatalf("delete conflict body = %s, want chat_history", resp.Body.String())
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM chat_session WHERE id = $1`, sessionID); got != 1 {
		t.Fatalf("chat session rows = %d, want 1", got)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM chat_message WHERE id = $1`, messageID); got != 1 {
		t.Fatalf("chat message rows = %d, want 1", got)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE id = $1`, targetID); got != 1 {
		t.Fatal("refused delete removed the account")
	}
}

// TestP5R2DeleteBlockedByActiveTask: queued/dispatched/running work attributed
// to the account blocks deletion. One row counts once even when it matches all
// three attribution columns, and a finished task keeps its history.
func TestP5R2DeleteBlockedByActiveTask(t *testing.T) {
	p5r1ResetAdmins(t)
	adminID, _ := p5r1CreateUser(t, "p5r2-task-admin@example.com", true)
	targetID, _ := p5r1CreateUser(t, "p5r2-task-target@example.com", false)
	runtimeID := dbfx.Runtime(t, "P5R2 Task Runtime")
	agentID := dbfx.Agent(t, "P5R2 Task Agent", runtimeID)
	activeTask := dbfx.Task(t, agentID, testutil.Cols{
		"status":              "running",
		"runtime_id":          runtimeID,
		"accountable_user_id": targetID,
		"initiator_user_id":   targetID,
		"originator_user_id":  targetID,
	})
	finishedTask := dbfx.Task(t, agentID, testutil.Cols{
		"status":              "completed",
		"runtime_id":          runtimeID,
		"accountable_user_id": targetID,
		"initiator_user_id":   targetID,
		"originator_user_id":  targetID,
		"completed_at":        testutil.Raw("now()"),
	})

	h := p5Handler(t, Config{})
	_, preview := p5r1Preview(t, h, adminID, targetID)
	if preview.CanDelete {
		t.Fatal("an account with live work must not be deletable")
	}
	if !blockerWithCount(preview, "active_task", 1) {
		t.Fatalf("blockers = %v, want active_task=1 (the same task counted once)", preview.Blockers)
	}

	req := testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+targetID, nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", targetID)
	resp := testutil.Call(t, h.DeleteInstanceUser, req).Want(http.StatusConflict)
	if !strings.Contains(resp.Body.String(), "active_task") {
		t.Fatalf("delete conflict body = %s, want active_task", resp.Body.String())
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE id = $1 AND status = 'running' AND accountable_user_id = $2`, activeTask, targetID); got != 1 {
		t.Fatal("refused delete changed the live task")
	}

	// Once the work finishes, the account is deletable again and the finished
	// task survives with its attribution detached.
	dbfx.Exec(t, `UPDATE agent_task_queue SET status = 'completed', completed_at = now() WHERE id = $1`, activeTask)
	_, preview = p5r1Preview(t, h, adminID, targetID)
	if !preview.CanDelete {
		t.Fatalf("finished work still blocks deletion: %v", preview.Blockers)
	}
	req = testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+targetID, nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", targetID)
	testutil.Call(t, h.DeleteInstanceUser, req).Want(http.StatusNoContent)
	for _, taskID := range []string{activeTask, finishedTask} {
		if got := dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE id = $1 AND accountable_user_id IS NULL AND initiator_user_id IS NULL AND originator_user_id IS NULL`, taskID); got != 1 {
			t.Fatalf("finished task %s was not preserved with detached attribution", taskID)
		}
	}
}

// TestP5R2ClientUsageWriteAndDeleteHaveNoOrphan covers both orders of the
// no-FK write the review named. client_usage_daily.user_id has no foreign key,
// so only the shared account lock plus the live-user recheck can keep the row
// from outliving the account.
func TestP5R2ClientUsageWriteAndDeleteHaveNoOrphan(t *testing.T) {
	p5r1ResetAdmins(t)
	adminID, _ := p5r1CreateUser(t, "p5r2-usage-admin@example.com", true)

	writeUsage := func(t *testing.T, userID string) int {
		t.Helper()
		w := testutil.Call(t, p5Handler(t, Config{}).UpsertClientUsage, func() *http.Request {
			req := p5JSONRequest(t, http.MethodPost, "/api/client-usage", map[string]string{
				"install_id": "12121212-3434-5656-7878-909090909090",
			})
			req.Header.Set("X-User-ID", userID)
			return req.WithContext(middleware.SetClientMetadata(req.Context(), "web", "0.0.1", "linux"))
		}())
		return w.Code
	}
	deleteUser := func(t *testing.T, userID string) int {
		t.Helper()
		req := testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+userID, nil)
		req.Header.Set("X-User-ID", adminID)
		req = withURLParam(req, "id", userID)
		return testutil.Call(t, p5Handler(t, Config{}).DeleteInstanceUser, req).Code
	}

	t.Run("write_commits_before_delete", func(t *testing.T) {
		userID, _ := p5r1CreateUser(t, "p5r2-usage-first@example.com", false)
		if code := writeUsage(t, userID); code != http.StatusNoContent {
			t.Fatalf("usage write status = %d", code)
		}
		if got := dbfx.Count(t, `SELECT count(*) FROM client_usage_daily WHERE user_id = $1`, userID); got != 1 {
			t.Fatalf("usage rows = %d, want 1", got)
		}
		if code := deleteUser(t, userID); code != http.StatusNoContent {
			t.Fatalf("delete status = %d", code)
		}
		if got := dbfx.Count(t, `SELECT count(*) FROM client_usage_daily WHERE user_id = $1`, userID); got != 0 {
			t.Fatalf("orphaned usage rows after delete = %d", got)
		}
	})

	t.Run("delete_commits_before_write", func(t *testing.T) {
		userID, _ := p5r1CreateUser(t, "p5r2-usage-second@example.com", false)
		if code := deleteUser(t, userID); code != http.StatusNoContent {
			t.Fatalf("delete status = %d", code)
		}
		if code := writeUsage(t, userID); code != http.StatusUnauthorized {
			t.Fatalf("late usage write status = %d, want 401", code)
		}
		if got := dbfx.Count(t, `SELECT count(*) FROM client_usage_daily WHERE user_id = $1`, userID); got != 0 {
			t.Fatalf("late usage write left %d orphan rows", got)
		}
	})

	// The delete transaction cannot pass the account key while a write holds it,
	// so a write that is already in flight is either included in the inventory
	// or waited for — never inserted after the final scan.
	t.Run("in_flight_write_blocks_delete_until_commit", func(t *testing.T) {
		userID, userUUID := p5r1CreateUser(t, "p5r2-usage-inflight@example.com", false)
		ctx := context.Background()
		tx, err := testPool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx)
		qtx := testHandler.Queries.WithTx(tx)
		if err := qtx.LockAccountKey(ctx, accountUserKey(userUUID)); err != nil {
			t.Fatalf("lock account: %v", err)
		}
		if _, err := qtx.UpsertClientUsageDaily(ctx, db.UpsertClientUsageDailyParams{
			UserID: userUUID, ClientType: "web", InstallID: mustUUID(t, "12121212-3434-5656-7878-909090909091"),
			ClientVersion: "0.0.1", Os: "linux",
		}); err != nil {
			t.Fatalf("in-flight usage insert: %v", err)
		}

		deleted := make(chan int, 1)
		go func() { deleted <- deleteUser(t, userID) }()
		select {
		case code := <-deleted:
			t.Fatalf("delete completed (status %d) while the account write lock was held", code)
		case <-time.After(200 * time.Millisecond):
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit in-flight write: %v", err)
		}
		if code := <-deleted; code != http.StatusNoContent {
			t.Fatalf("delete status = %d after the write committed", code)
		}
		if got := dbfx.Count(t, `SELECT count(*) FROM client_usage_daily WHERE user_id = $1`, userID); got != 0 {
			t.Fatalf("delete left %d usage rows behind", got)
		}
	})
}

// p5r2SessionSession wires a real WebSocket to the shared test hub with a
// database-backed session validator, so handler calls in these tests exercise
// the same authority production uses.
type p5r2SessionSession struct{}

func (p5r2SessionSession) ValidateSession(ctx context.Context, userID string, authVersion int64) bool {
	user, err := testHandler.Queries.GetUser(ctx, parseUUID(userID))
	if err != nil {
		return false
	}
	return authVersion < 0 || user.AuthVersion == authVersion
}

type p5r2Membership struct{}

func (p5r2Membership) IsMember(context.Context, string, string) bool { return true }

func p5r2Connect(t *testing.T, serverURL, token string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(serverURL, "http")+"/ws?workspace_id="+testWorkspaceID, header)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	return conn
}

func p5r2WaitForEvent(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	return string(frame)
}

func p5r2WaitForClose(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				break
			}
			return
		}
	}
	t.Fatal("the revoked WebSocket stayed open")
}

// TestP5R2PasswordChangeAndDeleteCloseLiveWebSockets drives the real handlers
// and checks the wiring into the hub: only the affected account's connection
// closes, and another user keeps receiving events.
func TestP5R2PasswordChangeAndDeleteCloseLiveWebSockets(t *testing.T) {
	p5r1ResetAdmins(t)
	ctx := context.Background()
	adminID, _ := p5r1CreateUser(t, "p5r2-ws-admin@example.com", true)

	httpServer := httptestNewServer(t, testHandler.Hub)
	defer httpServer.Close()

	// User A has a password so the change endpoint can rotate it.
	userAEmail := "p5r2-ws-password@example.com"
	userAID, userAUUID := p5r1CreateUser(t, userAEmail, false)
	dbfx.Cleanup(t, `DELETE FROM local_password_credential WHERE user_id = $1`, userAUUID)
	oldPassword := "the originally assigned passphrase"
	hash, err := auth.HashLocalPassword(oldPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := testHandler.Queries.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID: userAUUID, UsernameNormalized: "p5r2.ws.password", PasswordHash: hash,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	// User B is an unaffected bystander.
	_, userBUUID := p5r1CreateUser(t, "p5r2-ws-bystander@example.com", false)

	userA, err := testHandler.Queries.GetUser(ctx, userAUUID)
	if err != nil {
		t.Fatalf("load user A: %v", err)
	}
	userB, err := testHandler.Queries.GetUser(ctx, userBUUID)
	if err != nil {
		t.Fatalf("load user B: %v", err)
	}
	tokenA, err := testHandler.issueJWT(userA)
	if err != nil {
		t.Fatalf("token A: %v", err)
	}
	tokenB, err := testHandler.issueJWT(userB)
	if err != nil {
		t.Fatalf("token B: %v", err)
	}

	baseline := p5r2ConnectionCount(t)
	connA := p5r2Connect(t, httpServer.URL, tokenA)
	defer connA.Close()
	p5r2WaitForConnections(t, baseline+1)
	connB := p5r2Connect(t, httpServer.URL, tokenB)
	defer connB.Close()
	p5r2WaitForConnections(t, baseline+2)

	// Registration is asynchronous; a broadcast after the hub reports both
	// connections proves both sockets are in the workspace room.
	testHandler.Hub.BroadcastToWorkspace(testWorkspaceID, []byte(`{"type":"p5r2_warmup"}`))
	if frame := p5r2WaitForEvent(t, connA); !strings.Contains(frame, "p5r2_warmup") {
		t.Fatalf("unexpected warmup frame for A: %s", frame)
	}
	if frame := p5r2WaitForEvent(t, connB); !strings.Contains(frame, "p5r2_warmup") {
		t.Fatalf("unexpected warmup frame for B: %s", frame)
	}

	changeReq := p5JSONRequest(t, http.MethodPost, "/api/me/password", map[string]string{
		"current_password": oldPassword,
		"new_password":     "a completely different passphrase",
	})
	changeReq.Header.Set("X-User-ID", userAID)
	testutil.Call(t, p5Handler(t, Config{LocalPasswordLoginEnabled: true}).ChangeCurrentPassword, changeReq).Want(http.StatusOK)

	p5r2WaitForClose(t, connA)
	p5r2WaitForConnections(t, baseline+1)
	testHandler.Hub.BroadcastToWorkspace(testWorkspaceID, []byte(`{"type":"p5r2_after_password_change"}`))
	if frame := p5r2WaitForEvent(t, connB); !strings.Contains(frame, "p5r2_after_password_change") {
		t.Fatalf("bystander received an unexpected frame: %s", frame)
	}

	// Account deletion closes the target's socket through the same wiring.
	deleteTargetID, deleteTargetUUID := p5r1CreateUser(t, "p5r2-ws-delete@example.com", false)
	deleteTarget, err := testHandler.Queries.GetUser(ctx, deleteTargetUUID)
	if err != nil {
		t.Fatalf("load delete target: %v", err)
	}
	deleteToken, err := testHandler.issueJWT(deleteTarget)
	if err != nil {
		t.Fatalf("delete target token: %v", err)
	}
	connDelete := p5r2Connect(t, httpServer.URL, deleteToken)
	defer connDelete.Close()
	p5r2WaitForConnections(t, baseline+2)
	testHandler.Hub.BroadcastToWorkspace(testWorkspaceID, []byte(`{"type":"p5r2_warmup_delete"}`))
	if frame := p5r2WaitForEvent(t, connDelete); !strings.Contains(frame, "p5r2_warmup_delete") {
		t.Fatalf("unexpected warmup frame for the delete target: %s", frame)
	}

	req := testutil.JSONRequest(http.MethodDelete, "/api/admin/users/"+deleteTargetID, nil)
	req.Header.Set("X-User-ID", adminID)
	req = withURLParam(req, "id", deleteTargetID)
	testutil.Call(t, p5Handler(t, Config{}).DeleteInstanceUser, req).Want(http.StatusNoContent)
	p5r2WaitForClose(t, connDelete)
	p5r2WaitForConnections(t, baseline+1)
}

func p5r2ConnectionCount(t *testing.T) int {
	t.Helper()
	count, ok := testHandler.Hub.Snapshot()["connections"].(int)
	if !ok {
		t.Fatal("hub snapshot did not report a connection count")
	}
	return count
}

// p5r2WaitForConnections uses the hub's exported snapshot as the registration
// barrier, so these tests do not guess with sleeps.
func p5r2WaitForConnections(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if connections, ok := testHandler.Hub.Snapshot()["connections"].(int); ok && connections == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("hub connections = %v, want %d", testHandler.Hub.Snapshot()["connections"], want)
}

func blockerWithCount(preview p5r1DeletePreview, code string, count int64) bool {
	for _, blocker := range preview.Blockers {
		if blocker.Code == code {
			return blocker.Count == count
		}
	}
	return false
}

func p5SeedUserWithFlag(t *testing.T, email string, verified bool) string {
	t.Helper()
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	id := dbfx.Insert(t, "user", testutil.Cols{"name": "P5R2 Flag User", "email": email, "email_verified": verified})
	return id
}

func mustLoadUser(t *testing.T, email string) db.User {
	t.Helper()
	user, err := testHandler.Queries.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("load user %s: %v", email, err)
	}
	return user
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	return parseUUID(value)
}

// httptestNewServer mounts the production WebSocket handler on the shared hub.
func httptestNewServer(t *testing.T, hub *realtime.Hub) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realtime.HandleWebSocket(hub, p5r2Membership{}, p5r2SessionSession{}, nil, nil, w, r)
	}))
	t.Cleanup(server.Close)
	return server
}
