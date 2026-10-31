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
// initial-set invitation must never be a password reset backdoor.
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
	if _, err := h.Queries.GetLocalPasswordCredentialByUserID(ctx, target.ID); err == nil {
		writeErrorCode(w, http.StatusConflict, "already_activated", "this account already has a password")
		return
	} else if !isNotFound(err) {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if _, err := qtx.RevokeRegistrationInvitesForUser(ctx, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue invitation")
		return
	}
	raw, invite, err := h.createRegistrationInviteWith(ctx, qtx, target.ID, admin.ID)
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
		User:            h.instanceUserView(target, false, &expires),
		InviteToken:     raw,
		InviteExpiresAt: expires,
	})
}

// RevokeInstanceInvite revokes every outstanding invitation for the target.
func (h *Handler) RevokeInstanceInvite(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireInstanceAdmin(w, r); !ok {
		return
	}
	target, ok := h.loadInstanceTarget(w, r)
	if !ok {
		return
	}
	if _, err := h.Queries.RevokeRegistrationInvitesForUser(r.Context(), target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke invitation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) loadInstanceTarget(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeErrorCode(w, http.StatusBadRequest, "invalid_user_id", "user id is required")
		return db.User{}, false
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(id))
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return db.User{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return db.User{}, false
	}
	return user, true
}

// DeleteInstanceUser removes an account after proving it has no ownership
// dependencies. Business content authored elsewhere is deliberately not deleted.
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
	if target.ID == admin.ID {
		writeErrorCode(w, http.StatusBadRequest, "cannot_delete_self", "you cannot delete your own account")
		return
	}
	if target.IsInstanceAdmin {
		admins, err := h.Queries.CountInstanceAdmins(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete user")
			return
		}
		if admins <= 1 {
			writeErrorCode(w, http.StatusConflict, "last_admin", "the last instance administrator cannot be deleted")
			return
		}
	}
	ownerships, err := h.Queries.ListUserWorkspaceOwnerships(ctx, target.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check dependencies")
		return
	}
	if len(ownerships) > 0 {
		slugs := make([]string, 0, len(ownerships))
		for _, row := range ownerships {
			slugs = append(slugs, row.Slug)
		}
		writeErrorCode(w, http.StatusConflict, "owns_workspaces", "transfer or delete the workspaces first: "+strings.Join(slugs, ","))
		return
	}
	if runtimes, err := h.Queries.CountUserOwnedRuntimes(ctx, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check dependencies")
		return
	} else if runtimes > 0 {
		writeErrorCode(w, http.StatusConflict, "owns_runtimes", "this user still owns agent runtimes")
		return
	}
	if agents, err := h.Queries.CountUserOwnedAgents(ctx, target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check dependencies")
		return
	} else if agents > 0 {
		writeErrorCode(w, http.StatusConflict, "owns_agents", "this user still owns agents")
		return
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	cleanup := []func() error{
		func() error { return qtx.DeleteRegistrationInvitesForUser(ctx, target.ID) },
		func() error { return qtx.DeleteUserPersonalAccessTokens(ctx, target.ID) },
		func() error { return qtx.DeleteUserMemberships(ctx, target.ID) },
		func() error { return qtx.DeleteUserNotificationPreferences(ctx, target.ID) },
		func() error { return qtx.DeleteUserPinnedItems(ctx, target.ID) },
		func() error { return qtx.DeleteUserPasswordCredential(ctx, target.ID) },
		func() error { return qtx.DeleteLocalPasswordCredentialByUserID(ctx, target.ID) },
		func() error { return qtx.DeleteUserTaskTokens(ctx, target.ID) },
		func() error { return qtx.DeleteUserWorkspaceInvitations(ctx, target.ID) },
		func() error { return qtx.DeleteUserFeedback(ctx, target.ID) },
		func() error { return qtx.DeleteUserClientUsage(ctx, target.ID) },
		func() error { return qtx.DeleteUserIssueSubscriptions(ctx, target.ID) },
		func() error { return qtx.DeleteUserComposioConnections(ctx, target.ID) },
		func() error { return qtx.DeleteUserSeatCapacityOutbox(ctx, target.ID) },
		func() error { return qtx.DeleteUserChatPins(ctx, target.ID) },
		func() error { return qtx.DeleteUserAutopilotCollaborators(ctx, target.ID) },
		func() error { return qtx.DeleteUserAutopilotSubscribers(ctx, target.ID) },
		func() error { return qtx.DeleteUserIssueViewPreferences(ctx, target.ID) },
		func() error { return qtx.DeleteUserRow(ctx, target.ID) },
	}
	for _, step := range cleanup {
		if err := step(); err != nil {
			slog.Warn("delete instance user step failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to delete user")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	if h.PATCache != nil {
		h.PATCache.InvalidateAllForUser(uuidToString(target.ID))
	}
	w.WriteHeader(http.StatusNoContent)
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
