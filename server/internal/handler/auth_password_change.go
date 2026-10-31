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
//
// The verification and the write share one transaction that holds the account
// key, and the update is conditional on the stored hash still being the one
// that was verified. Two concurrent requests carrying the same old password
// therefore cannot both succeed, and a request that raced a completed change
// reports a stale current password instead of overwriting the winner.
//
// A successful change bumps the account auth_version so every older session
// token stops authenticating; live WebSocket connections for that account are
// closed as well, while other accounts keep their sessions. PATs are not
// password sessions and stay.
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
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := lockAccountKey(ctx, qtx, accountUserKey(userUUID)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	credential, err := qtx.GetLocalPasswordCredentialByUserID(ctx, userUUID)
	if err != nil {
		if isNotFound(err) {
			writeErrorCode(w, http.StatusBadRequest, "no_password_set", "this account has no local password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	if !auth.VerifyLocalPassword(credential.PasswordHash, req.CurrentPassword) {
		// 400, not 401: this request IS authenticated. A 401 here reads as an
		// expired session to the client, which tears the session down and hides
		// the very field the user has to correct.
		writeErrorCode(w, http.StatusBadRequest, "invalid_current_password", "current password is incorrect")
		return
	}
	newHash, err := auth.HashLocalPassword(req.NewPassword)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_password", auth.LocalPasswordPolicy)
		return
	}
	rotated, err := qtx.UpdateLocalPasswordCredentialHashConditional(ctx, db.UpdateLocalPasswordCredentialHashConditionalParams{
		UserID:  userUUID,
		OldHash: credential.PasswordHash,
		NewHash: newHash,
	})
	if err != nil {
		slog.Warn("change password credential update failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	if rotated != 1 {
		// The stored hash moved between the read and the update. The password
		// this request verified is no longer current; never overwrite.
		writeErrorCode(w, http.StatusBadRequest, "invalid_current_password", "current password is incorrect")
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
	// Optional in tests; production always wires the hub. Only this account's
	// connections close.
	if h.Hub != nil {
		h.Hub.DisconnectUser(userID, "session_revoked")
	}
	h.setNoStore(w)
	writeJSON(w, http.StatusOK, map[string]bool{"reauth_required": true})
}
