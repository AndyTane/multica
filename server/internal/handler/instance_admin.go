package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Instance user administration (P5).
//
// An instance administrator is NOT a workspace role: the flag lives on the
// user row, is granted only by the host-local bootstrap tool for the pinned
// operator identity, and can never be set through registration or PATCH /api/me.
// These endpoints manage accounts only; they do not grant access to workspace
// content, and machine credentials (task tokens / cloud PATs) are rejected.
//
// Every write that can change the administrator set or one account's
// invitations runs inside a transaction that first takes the same advisory
// lock: 'instance_admins' for anything reading the administrator count, and
// 'user:<uuid>' for anything mutating one account. Deletion takes both, in that
// order, and re-checks every condition inside the transaction; the preview
// endpoint returns exactly the dependency inventory that same function
// computes, so what the operator confirms is what the delete enforces.

type instanceAdminUser struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	IsInstanceAdmin bool    `json:"is_instance_admin"`
	Onboarded       bool    `json:"onboarded"`
	HasPassword     bool    `json:"has_password"`
	InviteExpiresAt *string `json:"invite_expires_at"`
	CreatedAt       string  `json:"created_at"`
}

type instanceUsersResponse struct {
	Users []instanceAdminUser `json:"users"`
}

type addInstanceUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type instanceInviteResponse struct {
	User            instanceAdminUser `json:"user"`
	InviteToken     string            `json:"invite_token"`
	InviteExpiresAt string            `json:"invite_expires_at"`
}

type registrationSettingsResponse struct {
	OpenRegistrationEnabled bool `json:"open_registration_enabled"`
}

type updateRegistrationSettingsRequest struct {
	OpenRegistrationEnabled *bool `json:"open_registration_enabled"`
}

// instanceUserDeleteBlocker names one reason the account cannot be removed.
// Codes are stable identifiers; the UI owns their localized text.
type instanceUserDeleteBlocker struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

// instanceUserDeletePreview is the GET /api/admin/users/{id}/delete-preview
// contract. can_delete is false whenever any blocker exists and also when the
// preview itself could not be computed: the caller must never treat a failed
// preview as permission.
type instanceUserDeletePreview struct {
	CanDelete bool                        `json:"can_delete"`
	Blockers  []instanceUserDeleteBlocker `json:"blockers"`
	Removes   []string                    `json:"removes"`
}

func isMachineActor(r *http.Request) bool {
	source := r.Header.Get("X-Actor-Source")
	return source == "task_token" || source == "cloud_pat"
}

// requireInstanceAdmin resolves the caller from the database and checks the
// instance-level flag. A workspace owner without the flag is rejected.
func (h *Handler) requireInstanceAdmin(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	if isMachineActor(r) {
		writeErrorCode(w, http.StatusForbidden, "human_actor_required", "this endpoint requires a signed-in person")
		return db.User{}, false
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return db.User{}, false
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusUnauthorized, "user not found")
			return db.User{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return db.User{}, false
	}
	if !user.IsInstanceAdmin {
		writeErrorCode(w, http.StatusForbidden, "instance_admin_required", "instance administrator permission is required")
		return db.User{}, false
	}
	return user, true
}

// lockAccountKey serializes the writers that share a logical account key for
// the rest of the transaction. It never acquires two locks in an order the
// other writers do not use: 'instance_admins' is always taken before any
// 'user:<uuid>' key, and a single-account writer takes only its own key.
func lockAccountKey(ctx context.Context, q *db.Queries, key string) error {
	return q.LockAccountKey(ctx, key)
}

func accountUserKey(id pgtype.UUID) string {
	return "user:" + uuidToString(id)
}

const instanceAdminsLockKey = "instance_admins"

func instanceAdminUserFrom(userID string, name, email string, isAdmin bool, onboarded bool, createdAt string, hasPassword bool, inviteExpiry *string) instanceAdminUser {
	return instanceAdminUser{
		ID:              userID,
		Name:            name,
		Email:           email,
		IsInstanceAdmin: isAdmin,
		Onboarded:       onboarded,
		HasPassword:     hasPassword,
		InviteExpiresAt: inviteExpiry,
		CreatedAt:       createdAt,
	}
}

func (h *Handler) instanceUserView(user db.User, hasPassword bool, inviteExpiry *string) instanceAdminUser {
	return instanceAdminUserFrom(
		uuidToString(user.ID),
		user.Name,
		user.Email,
		user.IsInstanceAdmin,
		user.OnboardedAt.Valid,
		timestampToString(user.CreatedAt),
		hasPassword,
		inviteExpiry,
	)
}

func (h *Handler) registrationSettings(r *http.Request) registrationSettingsResponse {
	return registrationSettingsResponse{OpenRegistrationEnabled: h.openRegistrationEnabled(r.Context())}
}

// ListInstanceUsers returns the account list shown on the admin page. It never
// includes invite secrets: only the active invitation expiry is exposed.
func (h *Handler) ListInstanceUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireInstanceAdmin(w, r); !ok {
		return
	}
	ctx := r.Context()
	rows, err := h.Queries.ListInstanceUsers(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	passwordUserIDs, err := h.Queries.ListUserIDsWithLocalPassword(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	hasPassword := make(map[string]bool, len(passwordUserIDs))
	for _, id := range passwordUserIDs {
		hasPassword[uuidToString(id)] = true
	}
	activeInvites, err := h.Queries.ListActiveRegistrationInvites(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	inviteExpiry := make(map[string]string, len(activeInvites))
	for _, invite := range activeInvites {
		inviteExpiry[uuidToString(invite.UserID)] = timestampToString(invite.ExpiresAt)
	}
	users := make([]instanceAdminUser, 0, len(rows))
	for _, row := range rows {
		id := uuidToString(row.ID)
		var expiry *string
		if value, ok := inviteExpiry[id]; ok {
			copied := value
			expiry = &copied
		}
		users = append(users, instanceAdminUserFrom(
			id,
			row.Name,
			row.Email,
			row.IsInstanceAdmin,
			row.OnboardedAt.Valid,
			timestampToString(row.CreatedAt),
			hasPassword[id],
			expiry,
		))
	}
	h.setNoStore(w)
	writeJSON(w, http.StatusOK, instanceUsersResponse{Users: users})
}

// AddInstanceUser creates a pending account and returns a one-time invitation.
func (h *Handler) AddInstanceUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.requireInstanceAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req addInstanceUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	name, err := auth.NormalizeRegistrationDisplayName(req.Name)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_display_name", "invalid display name")
		return
	}
	email, err := auth.NormalizeRegistrationEmail(req.Email)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_email", "invalid email address")
		return
	}
	ctx := r.Context()
	if _, err := h.Queries.GetUserByEmail(ctx, email); err == nil {
		writeErrorCode(w, http.StatusConflict, "email_taken", "that email is already registered")
		return
	} else if !isNotFound(err) {
		writeError(w, http.StatusInternalServerError, "failed to add user")
		return
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add user")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	user, err := qtx.CreateUser(ctx, db.CreateUserParams{Name: name, Email: email})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeErrorCode(w, http.StatusConflict, "email_taken", "that email is already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to add user")
		return
	}
	raw, invite, err := h.createRegistrationInviteWith(ctx, qtx, user.ID, admin.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add user")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add user")
		return
	}
	expires := timestampToString(invite.ExpiresAt)
	h.setNoStore(w)
	writeJSON(w, http.StatusCreated, instanceInviteResponse{
		User:            h.instanceUserView(user, false, &expires),
		InviteToken:     raw,
		InviteExpiresAt: expires,
	})
}

// createRegistrationInviteWith is the transaction-bound invitation mint used by
// admin handlers. The raw value is returned once and never stored.
func (h *Handler) createRegistrationInviteWith(ctx context.Context, qtx *db.Queries, userID, createdBy pgtype.UUID) (string, db.RegistrationInvite, error) {
	raw, err := auth.NewRegistrationInviteToken()
	if err != nil {
		return "", db.RegistrationInvite{}, err
	}
	invite, err := qtx.CreateRegistrationInvite(ctx, db.CreateRegistrationInviteParams{
		UserID:    userID,
		TokenHash: auth.RegistrationTokenHash(raw),
		CreatedBy: createdBy,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(registrationInviteTTL), Valid: true},
	})
	if err != nil {
		return "", db.RegistrationInvite{}, err
	}
	return raw, invite, nil
}

// RegenerateInstanceInvite revokes outstanding invitations for the target and
// mints a fresh one. An account that already has a password is refused: an
// initial-set invitation must never be a password reset backdoor. The
// re-check, the revoke and the insert share one transaction and one account
// lock, so two concurrent regenerations cannot leave two valid invitations.
func (h *Handler) RegenerateInstanceInvite(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.requireInstanceAdmin(w, r)
	if !ok {
		return
	}
	target, ok := h.loadInstanceTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := lockAccountKey(ctx, qtx, accountUserKey(target.ID)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	locked, err := qtx.GetUserForUpdate(ctx, target.ID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	if _, err := qtx.GetLocalPasswordCredentialByUserID(ctx, locked.ID); err == nil {
		writeErrorCode(w, http.StatusConflict, "already_activated", "this account already has a password")
		return
	} else if !isNotFound(err) {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	if _, err := qtx.RevokeRegistrationInvitesForUser(ctx, locked.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	raw, invite, err := h.createRegistrationInviteWith(ctx, qtx, locked.ID, admin.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	expires := timestampToString(invite.ExpiresAt)
	h.setNoStore(w)
	writeJSON(w, http.StatusOK, instanceInviteResponse{
		User:            h.instanceUserView(locked, false, &expires),
		InviteToken:     raw,
		InviteExpiresAt: expires,
	})
}

// RevokeInstanceInvite revokes every outstanding invitation for the target.
// Serialized against regeneration and redemption on the same account key so a
// revoke cannot interleave with a mint and leave an invitation the operator
// believes was cancelled.
func (h *Handler) RevokeInstanceInvite(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireInstanceAdmin(w, r); !ok {
		return
	}
	target, ok := h.loadInstanceTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke invitation")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := lockAccountKey(ctx, qtx, accountUserKey(target.ID)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke invitation")
		return
	}
	if _, err := qtx.RevokeRegistrationInvitesForUser(ctx, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke invitation")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke invitation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// loadInstanceTarget resolves the {id} path segment. A client-supplied value
// that is not a UUID is a bad request, never a panic.
func (h *Handler) loadInstanceTarget(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeErrorCode(w, http.StatusBadRequest, "invalid_user_id", "user id is required")
		return db.User{}, false
	}
	userUUID, ok := parseUUIDOrBadRequest(w, id, "user id")
	if !ok {
		return db.User{}, false
	}
	user, err := h.Queries.GetUser(r.Context(), userUUID)
	if err != nil {
		if isNotFound(err) {
			writeErrorCode(w, http.StatusNotFound, "user_not_found", "user not found")
			return db.User{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return db.User{}, false
	}
	return user, true
}

// instanceUserDeleteState is the single dependency decision used by both the
// preview endpoint and the delete transaction. It combines the schema-level
// dependency inventory with the two relationships that depend on WHO is
// deleting: an account cannot remove itself, and the last administrator cannot
// be removed at all.
func (h *Handler) instanceUserDeleteState(ctx context.Context, q *db.Queries, adminID pgtype.UUID, target db.User) (instanceUserDeletePreview, error) {
	preview := instanceUserDeletePreview{
		CanDelete: true,
		Blockers:  make([]instanceUserDeleteBlocker, 0, 4),
		Removes:   make([]string, 0, 8),
	}
	rows, err := q.InspectInstanceUserDependencies(ctx, target.ID)
	if err != nil {
		return instanceUserDeletePreview{CanDelete: false}, err
	}
	for _, row := range rows {
		switch row.Kind {
		case "blocker":
			preview.Blockers = append(preview.Blockers, instanceUserDeleteBlocker{Code: row.Code, Count: row.Count})
		case "remove":
			preview.Removes = append(preview.Removes, row.Code)
		}
	}
	if target.ID == adminID {
		preview.Blockers = append(preview.Blockers, instanceUserDeleteBlocker{Code: "cannot_delete_self", Count: 1})
	}
	if target.IsInstanceAdmin {
		admins, err := q.CountInstanceAdmins(ctx)
		if err != nil {
			return instanceUserDeletePreview{CanDelete: false}, err
		}
		if admins <= 1 {
			preview.Blockers = append(preview.Blockers, instanceUserDeleteBlocker{Code: "last_admin", Count: 1})
		}
	}
	preview.CanDelete = len(preview.Blockers) == 0
	return preview, nil
}

// GetInstanceUserDeletePreview reports exactly what DELETE would do: the same
// blocker codes with the same counts, and the account-scoped rows that would be
// removed. Admin-only, and machine credentials are refused like every other
// account-administration endpoint.
func (h *Handler) GetInstanceUserDeletePreview(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.requireInstanceAdmin(w, r)
	if !ok {
		return
	}
	target, ok := h.loadInstanceTarget(w, r)
	if !ok {
		return
	}
	preview, err := h.instanceUserDeleteState(r.Context(), h.Queries, admin.ID, target)
	if err != nil {
		slog.Warn("delete preview failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to inspect account dependencies")
		return
	}
	h.setNoStore(w)
	writeJSON(w, http.StatusOK, preview)
}

// instanceUserCleanup is the account-scoped teardown, in two groups: rows that
// belong to the account (removed) and nullable attribution on shared history
// (detached, never deleted).
func (h *Handler) instanceUserCleanup(ctx context.Context, qtx *db.Queries, userID pgtype.UUID) []func() error {
	return []func() error{
		func() error { return qtx.DetachUserAgentInvocationTargets(ctx, userID) },
		func() error { return qtx.DetachUserArchivedAgents(ctx, userID) },
		func() error { return qtx.DetachUserAgentTaskQueue(ctx, userID) },
		func() error { return qtx.DetachUserChannelChatContextGenerations(ctx, userID) },
		func() error { return qtx.DetachUserIssueWakeups(ctx, userID) },
		func() error { return qtx.DetachUserPluginPackages(ctx, userID) },
		func() error { return qtx.DetachUserProjectResources(ctx, userID) },
		func() error { return qtx.DetachUserRuntimeProfiles(ctx, userID) },
		func() error { return qtx.DetachUserSkills(ctx, userID) },
		func() error { return qtx.DetachUserWorkspaceMCPServers(ctx, userID) },
		func() error { return qtx.DeleteRegistrationInvitesForUser(ctx, userID) },
		func() error { return qtx.DeleteUserPersonalAccessTokens(ctx, userID) },
		func() error { return qtx.DeleteUserMemberships(ctx, userID) },
		func() error { return qtx.DeleteUserNotificationPreferences(ctx, userID) },
		func() error { return qtx.DeleteUserPinnedItems(ctx, userID) },
		func() error { return qtx.DeleteUserPasswordCredentialByUserID(ctx, userID) },
		func() error { return qtx.DeleteUserTaskTokens(ctx, userID) },
		func() error { return qtx.DeleteUserWorkspaceInvitations(ctx, userID) },
		func() error { return qtx.DeleteUserFeedback(ctx, userID) },
		func() error { return qtx.DeleteUserClientUsage(ctx, userID) },
		func() error { return qtx.DeleteUserIssueSubscriptions(ctx, userID) },
		func() error { return qtx.DeleteUserComposioConnections(ctx, userID) },
		func() error { return qtx.DeleteUserSeatCapacityOutbox(ctx, userID) },
		func() error { return qtx.DeleteUserChatPins(ctx, userID) },
		func() error { return qtx.DeleteUserAutopilotCollaborators(ctx, userID) },
		func() error { return qtx.DeleteUserAutopilotSubscribers(ctx, userID) },
		func() error { return qtx.DeleteUserIssueViewPreferences(ctx, userID) },
		func() error { return qtx.DeleteUserIssueViews(ctx, userID) },
		func() error { return qtx.DeleteUserChannelBindings(ctx, userID) },
		func() error { return qtx.DeleteUserLarkBindings(ctx, userID) },
		func() error { return qtx.DeleteUserRow(ctx, userID) },
	}
}

// DeleteInstanceUser removes an account only after proving, inside the same
// transaction that performs the delete, that no ownership dependency exists.
// Business content authored elsewhere is detached, never deleted.
//
// The transaction takes 'instance_admins' and then the target's account key;
// every condition is re-read under those locks (live caller, live target,
// administrator count, dependencies) and the dependency inventory is re-run
// after cleanup so a concurrent create either rolls this delete back or is
// reported instead of leaving an orphan.
func (h *Handler) DeleteInstanceUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.requireInstanceAdmin(w, r)
	if !ok {
		return
	}
	target, ok := h.loadInstanceTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := lockAccountKey(ctx, qtx, instanceAdminsLockKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	if err := lockAccountKey(ctx, qtx, accountUserKey(target.ID)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	liveTarget, err := qtx.GetUserForUpdate(ctx, target.ID)
	if err != nil {
		if isNotFound(err) {
			writeErrorCode(w, http.StatusNotFound, "user_not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	// The caller's own administrator state is re-read under the same locks: a
	// concurrent demotion or deletion of the caller must not be able to approve
	// this delete on a stale privilege.
	liveAdmin, err := qtx.GetUserForUpdate(ctx, admin.ID)
	if err != nil {
		if isNotFound(err) {
			writeErrorCode(w, http.StatusForbidden, "instance_admin_required", "instance administrator permission is required")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	if !liveAdmin.IsInstanceAdmin {
		writeErrorCode(w, http.StatusForbidden, "instance_admin_required", "instance administrator permission is required")
		return
	}
	if liveTarget.ID == liveAdmin.ID {
		writeErrorCode(w, http.StatusBadRequest, "cannot_delete_self", "you cannot delete your own account")
		return
	}
	state, err := h.instanceUserDeleteState(ctx, qtx, liveAdmin.ID, liveTarget)
	if err != nil {
		slog.Warn("delete dependency check failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to check dependencies")
		return
	}
	if !state.CanDelete {
		writeErrorCode(w, http.StatusConflict, state.Blockers[0].Code, dependencyMessage(state.Blockers))
		return
	}
	for _, step := range h.instanceUserCleanup(ctx, qtx, liveTarget.ID) {
		if err := step(); err != nil {
			slog.Warn("delete instance user step failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to delete user")
			return
		}
	}
	// Defense in depth: a dependency created concurrently (a workspace, agent,
	// runtime, channel installation, share link or captured source context that
	// committed while this transaction was running) is visible here and rolls
	// the delete back instead of surviving as an orphan.
	after, err := qtx.InspectInstanceUserDependencies(ctx, liveTarget.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify dependencies")
		return
	}
	if len(after) > 0 {
		slog.Warn("delete instance user raced with a new dependency", append(logger.RequestAttrs(r), "codes", dependencyCodes(after))...)
		writeErrorCode(w, http.StatusConflict, "dependency_changed", "account dependencies changed while deleting; retry after reviewing the preview")
		return
	}
	// The PAT cache can outlive the row it resolved: mark the account deleted
	// before commit so a cache hit can never resurrect it, and refuse the whole
	// delete when that marker cannot be written.
	userID := uuidToString(liveTarget.ID)
	if h.PATCache != nil {
		if err := h.PATCache.MarkUserDeleted(ctx, userID); err != nil {
			writeErrorCode(w, http.StatusServiceUnavailable, "revocation_unavailable", "cannot guarantee session revocation; retry later")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if h.PATCache != nil {
			h.PATCache.ClearUserDeleted(ctx, userID)
		}
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	if h.PATCache != nil {
		h.PATCache.InvalidateAllForUser(userID)
	}
	if h.Hub != nil {
		h.Hub.DisconnectUser(userID, "account_deleted")
	}
	w.WriteHeader(http.StatusNoContent)
}

func dependencyMessage(blockers []instanceUserDeleteBlocker) string {
	parts := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		parts = append(parts, blocker.Code)
	}
	return "account has dependencies: " + strings.Join(parts, ",")
}

func dependencyCodes(rows []db.InspectInstanceUserDependenciesRow) []string {
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		codes = append(codes, row.Code)
	}
	return codes
}

// GetRegistrationSettings returns the persistent open-registration switch.
func (h *Handler) GetRegistrationSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireInstanceAdmin(w, r); !ok {
		return
	}
	h.setNoStore(w)
	writeJSON(w, http.StatusOK, h.registrationSettings(r))
}

// UpdateRegistrationSettings persists the switch. Refusing to report success on
// a failed write is what keeps the admin UI honest.
func (h *Handler) UpdateRegistrationSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireInstanceAdmin(w, r); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req updateRegistrationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OpenRegistrationEnabled == nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request_body", "open_registration_enabled is required")
		return
	}
	value, err := json.Marshal(instanceRegistrationSetting{OpenRegistrationEnabled: *req.OpenRegistrationEnabled})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save setting")
		return
	}
	if _, err := h.Queries.UpsertInstanceSetting(r.Context(), db.UpsertInstanceSettingParams{
		Key:   openRegistrationSettingKey,
		Value: value,
	}); err != nil {
		slog.Warn("save open registration setting failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save setting")
		return
	}
	h.setNoStore(w)
	writeJSON(w, http.StatusOK, registrationSettingsResponse{OpenRegistrationEnabled: *req.OpenRegistrationEnabled})
}
