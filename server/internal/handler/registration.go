package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Web registration (P5): invitation redemption and opt-in open registration.
// Both finish through the existing login machinery and never create workspace
// membership, instance admin rights, or an onboarding decision.

const (
	registrationMaxBodyBytes       = 16 << 10
	registrationInviteTTL          = 24 * time.Hour
	openRegistrationSettingKey     = "open_registration"
	registrationInviteGenericError = "invalid or expired invitation"
)

type instanceRegistrationSetting struct {
	OpenRegistrationEnabled bool `json:"enabled"`
}

func (h *Handler) openRegistrationEnabled(ctx context.Context) bool {
	raw, err := h.Queries.GetInstanceSetting(ctx, openRegistrationSettingKey)
	if err != nil {
		if !isNotFound(err) {
			slog.Warn("read open registration setting failed", "error", err)
		}
		return false
	}
	var setting instanceRegistrationSetting
	if err := json.Unmarshal(raw, &setting); err != nil {
		return false
	}
	return setting.OpenRegistrationEnabled
}

func (h *Handler) setNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

// registrationOriginAllowed accepts same-origin browser writes and non-browser
// clients without an Origin, and rejects any foreign Origin.
func (h *Handler) registrationOriginAllowed(origin string) bool {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		return true
	}
	candidates := []string{os.Getenv("FRONTEND_ORIGIN"), os.Getenv("MULTICA_APP_URL")}
	for _, raw := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		candidates = append(candidates, raw)
	}
	for _, candidate := range candidates {
		if strings.TrimRight(strings.TrimSpace(candidate), "/") == origin {
			return true
		}
	}
	return false
}

func (h *Handler) requireRegistrationRequest(w http.ResponseWriter, r *http.Request) bool {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "application/json") {
		writeErrorCode(w, http.StatusUnsupportedMediaType, "invalid_content_type", "request must be application/json")
		return false
	}
	if !h.registrationOriginAllowed(r.Header.Get("Origin")) {
		writeErrorCode(w, http.StatusForbidden, "invalid_origin", "origin is not allowed")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, registrationMaxBodyBytes)
	return true
}

func (h *Handler) registrationEnabled(w http.ResponseWriter) bool {
	if !h.cfg.LocalPasswordLoginEnabled {
		writeErrorCode(w, http.StatusForbidden, "local_password_login_disabled", "local password login is not enabled on this server")
		return false
	}
	return true
}

func decodeRegistrationBody(w http.ResponseWriter, r *http.Request, dest any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return false
	}
	return true
}

func (h *Handler) finishRegistrationLogin(w http.ResponseWriter, r *http.Request, user db.User) {
	tokenString, err := h.issueJWT(user)
	if err != nil {
		slog.Warn("registration session mint failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	if err := auth.SetAuthCookies(w, tokenString); err != nil {
		slog.Warn("failed to set registration auth cookies", "error", err)
	}
	h.setNoStore(w)
	writeJSON(w, http.StatusOK, LoginResponse{Token: tokenString, User: h.userToResponse(user)})
}

func (h *Handler) createRegistrationInvite(ctx context.Context, userID, createdBy pgtype.UUID) (string, db.RegistrationInvite, error) {
	raw, err := auth.NewRegistrationInviteToken()
	if err != nil {
		return "", db.RegistrationInvite{}, err
	}
	invite, err := h.Queries.CreateRegistrationInvite(ctx, db.CreateRegistrationInviteParams{
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

// RedeemInvitationRequest is the POST /api/register/invite body.
type RedeemInvitationRequest struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// RedeemRegistrationInvite sets the first password for the invited user.
func (h *Handler) RedeemRegistrationInvite(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled(w) {
		return
	}
	if !h.requireRegistrationRequest(w, r) {
		return
	}
	var req RedeemInvitationRequest
	if !decodeRegistrationBody(w, r, &req) {
		return
	}
	token, err := auth.NormalizeRegistrationToken(req.Token)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_invite", registrationInviteGenericError)
		return
	}
	name, err := auth.NormalizeRegistrationDisplayName(req.Name)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_display_name", "invalid display name")
		return
	}
	username, err := auth.NormalizeLocalUsername(req.Username)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_username", "invalid username")
		return
	}
	passwordHash, err := auth.HashLocalPassword(req.Password)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_password", auth.LocalPasswordPolicy)
		return
	}
	ctx := r.Context()
	tokenHash := auth.RegistrationTokenHash(token)
	// Resolve the target outside the transaction only to learn which account
	// key to take. Every decision below re-reads the same rows after the lock,
	// so this read cannot authorize anything by itself.
	lookup, err := h.Queries.GetRegistrationInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		if !isNotFound(err) {
			slog.Warn("registration invite lookup failed", append(logger.RequestAttrs(r), "error", err)...)
		}
		writeErrorCode(w, http.StatusBadRequest, "invalid_invite", registrationInviteGenericError)
		return
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set password")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	// Redemption, regeneration, revocation and account deletion all serialize
	// on this account key. Without it a regenerate could revoke and re-mint
	// between this request's validity check and its consume.
	if err := lockAccountKey(ctx, qtx, accountUserKey(lookup.UserID)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set password")
		return
	}
	invite, err := qtx.GetRegistrationInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_invite", registrationInviteGenericError)
		return
	}
	if invite.ConsumedAt.Valid || invite.RevokedAt.Valid || !invite.ExpiresAt.Valid || !time.Now().Before(invite.ExpiresAt.Time) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_invite", registrationInviteGenericError)
		return
	}
	user, err := qtx.GetUser(ctx, invite.UserID)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_invite", registrationInviteGenericError)
		return
	}
	if auth.IsTemporarilyDisabledUser(uuidToString(user.ID), user.Email) {
		writeError(w, http.StatusForbidden, auth.TemporarilyDisabledUserError)
		return
	}
	// An account that already has a password is refused inside the lock: a
	// concurrent redemption or a rotate-after-redemption must not be able to
	// turn a first-time invitation into a password reset.
	if _, err := qtx.GetLocalPasswordCredentialByUserID(ctx, user.ID); err == nil {
		writeErrorCode(w, http.StatusConflict, "already_activated", "this account already has a password")
		return
	} else if !isNotFound(err) {
		writeError(w, http.StatusInternalServerError, "failed to check account state")
		return
	}
	consumed, err := qtx.ConsumeRegistrationInvite(ctx, invite.ID)
	if err != nil || consumed != 1 {
		writeErrorCode(w, http.StatusBadRequest, "invalid_invite", registrationInviteGenericError)
		return
	}
	if err := qtx.SetUserNameForRegistration(ctx, db.SetUserNameForRegistrationParams{ID: user.ID, Name: name}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set password")
		return
	}
	if _, err := qtx.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID:             user.ID,
		UsernameNormalized: username,
		PasswordHash:       passwordHash,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeErrorCode(w, http.StatusConflict, "username_taken", "that username is already in use")
			return
		}
		slog.Warn("registration credential insert failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to set password")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set password")
		return
	}
	updated, err := h.Queries.GetUser(ctx, user.ID)
	if err != nil {
		updated = user
	}
	h.finishRegistrationLogin(w, r, updated)
}

// OpenRegistrationRequest is the POST /api/register/open body.
type OpenRegistrationRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// OpenRegistration creates an ordinary new account while open registration is
// enabled by the instance administrator (default off).
func (h *Handler) OpenRegistration(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled(w) {
		return
	}
	if !h.requireRegistrationRequest(w, r) {
		return
	}
	ctx := r.Context()
	if !h.openRegistrationEnabled(ctx) {
		writeErrorCode(w, http.StatusForbidden, "registration_disabled", "open registration is disabled")
		return
	}
	var req OpenRegistrationRequest
	if !decodeRegistrationBody(w, r, &req) {
		return
	}
	email, err := auth.NormalizeRegistrationEmail(req.Email)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_email", "invalid email address")
		return
	}
	name, err := auth.NormalizeRegistrationDisplayName(req.Name)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_display_name", "invalid display name")
		return
	}
	username, err := auth.NormalizeLocalUsername(req.Username)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_username", "invalid username")
		return
	}
	passwordHash, err := auth.HashLocalPassword(req.Password)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_password", auth.LocalPasswordPolicy)
		return
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	// Open registration creates an UNVERIFIED identity: the address is what the
	// registrant typed, so it must not unlock invitations addressed to that
	// mailbox. Invitation redemption and admin provisioning stay verified.
	user, err := qtx.CreateUnverifiedUser(ctx, db.CreateUnverifiedUserParams{Name: name, Email: email})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeErrorCode(w, http.StatusConflict, "email_taken", "that email is already registered")
			return
		}
		slog.Warn("open registration user insert failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	if _, err := qtx.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID:             user.ID,
		UsernameNormalized: username,
		PasswordHash:       passwordHash,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeErrorCode(w, http.StatusConflict, "username_taken", "that username is already in use")
			return
		}
		slog.Warn("open registration credential insert failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	h.finishRegistrationLogin(w, r, user)
}

var _ = pgx.ErrNoRows
