package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ChangeCurrentPassword verifies the current password before rotating it.
// A successful change bumps the account auth_version so every older session
// token stops authenticating; the current browser session is cleared and the
// client is asked to sign in again. PATs are not password sessions and stay.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handler) ChangeCurrentPassword(w http.ResponseWriter, r *http.Request) {
	if isMachineActor(r) {
		writeErrorCode(w, http.StatusForbidden, "human_actor_required", "this endpoint requires a signed-in person")
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	userUUID := parseUUID(userID)
	credential, err := h.Queries.GetLocalPasswordCredentialByUserID(ctx, userUUID)
	if err != nil {
		if isNotFound(err) {
			writeErrorCode(w, http.StatusBadRequest, "no_password_set", "this account has no local password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	if h.PasswordLimiter != nil {
		if retryAfter, allowed := h.PasswordLimiter.AllowAccount("user:" + userID); !allowed {
			middleware.WritePasswordRateLimited(w, retryAfter)
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	if req.CurrentPassword == "" {
		writeErrorCode(w, http.StatusBadRequest, "current_password_required", "current password is required")
		return
	}
	if !auth.VerifyLocalPassword(credential.PasswordHash, req.CurrentPassword) {
		writeErrorCode(w, http.StatusUnauthorized, "invalid_current_password", "current password is incorrect")
		return
	}
	newHash, err := auth.HashLocalPassword(req.NewPassword)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_password", auth.LocalPasswordPolicy)
		return
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if _, err := qtx.UpdateLocalPasswordCredentialHashByUserID(ctx, db.UpdateLocalPasswordCredentialHashByUserIDParams{
		UserID:       userUUID,
		PasswordHash: newHash,
	}); err != nil {
		slog.Warn("change password credential update failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	if _, err := qtx.BumpUserAuthVersion(ctx, userUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	auth.ClearAuthCookies(w)
	h.setNoStore(w)
	writeJSON(w, http.StatusOK, map[string]bool{"reauth_required": true})
}
