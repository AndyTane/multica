package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/middleware"
)

// Local password login (POST /auth/password).
//
// The endpoint authenticates an account/password credential provisioned by the
// host administrator (server/cmd/local-auth) and then finishes through the exact
// same session machinery as the email-code flow: JWT, HttpOnly auth cookie,
// CSRF cookie, and the LoginResponse shape. It never creates users, never
// consults signup policy, and reveals nothing about whether a username exists.

const (
	// passwordLoginMaxBodyBytes bounds the request before JSON decoding. The
	// legal payload is a few dozen bytes; the cap is what stops a client from
	// making the server buffer an arbitrary body.
	passwordLoginMaxBodyBytes = 4 << 10

	// passwordLoginGenericError is the only authentication failure message.
	// Unknown username, wrong password and an orphaned credential are
	// indistinguishable to the caller.
	passwordLoginGenericError = "invalid username or password"
)

// PasswordLoginRequest is the POST /auth/password body.
type PasswordLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// PasswordLogin authenticates a local account/password credential.
func (h *Handler) PasswordLogin(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.LocalPasswordLoginEnabled {
		writeFeatureDisabled(w, "local_password_login_disabled", "Local password login is not enabled on this server")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, passwordLoginMaxBodyBytes)
	var req PasswordLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	username, err := auth.NormalizeLocalUsername(req.Username)
	if err != nil {
		// A malformed username is a shape error, not a credential failure: it
		// is answered before any comparison so garbage input costs no bcrypt.
		writeError(w, http.StatusBadRequest, "invalid username")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(req.Password) > auth.LocalPasswordMaxBytes {
		// Explicit rejection, never truncation: bcrypt only consumes the first
		// 72 bytes, so accepting longer input would silently ignore the rest.
		writeError(w, http.StatusBadRequest, "password is too long")
		return
	}

	// Account budget: charged before hashing, so repeated guesses for one
	// account cost the server nothing beyond the counter increment. The source
	// budget ran in the route middleware before this handler.
	if h.PasswordLimiter != nil {
		if retryAfter, ok := h.PasswordLimiter.AllowAccount(username); !ok {
			middleware.WritePasswordRateLimited(w, retryAfter)
			return
		}
	}

	credential, err := h.Queries.GetLocalPasswordCredentialByUsername(r.Context(), username)
	if err != nil {
		if !isNotFound(err) {
			slog.Warn("local password credential lookup failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to sign in")
			return
		}
		// Unknown username: do the same bcrypt work a real compare would, so
		// response timing does not confirm which usernames exist.
		auth.VerifyLocalPassword(auth.DummyLocalPasswordHash, req.Password)
		writeError(w, http.StatusUnauthorized, passwordLoginGenericError)
		return
	}

	if !auth.VerifyLocalPassword(credential.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, passwordLoginGenericError)
		return
	}

	// Credential matched; now resolve the user it belongs to. A credential whose
	// user row is gone fails exactly like a wrong password, and a disabled user
	// gets the same 403 the other login methods return.
	user, err := h.Queries.GetUser(r.Context(), credential.UserID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusUnauthorized, passwordLoginGenericError)
			return
		}
		slog.Warn("local password user lookup failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to sign in")
		return
	}
	if auth.IsTemporarilyDisabledUser(uuidToString(user.ID), user.Email) {
		writeError(w, http.StatusForbidden, auth.TemporarilyDisabledUserError)
		return
	}

	tokenString, err := h.issueJWT(user)
	if err != nil {
		if errors.Is(err, auth.ErrTemporarilyDisabledUser) {
			writeError(w, http.StatusForbidden, auth.TemporarilyDisabledUserError)
			return
		}
		slog.Warn("local password login failed", append(logger.RequestAttrs(r), "error", err, "user_id", uuidToString(user.ID))...)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	if err := auth.SetAuthCookies(w, tokenString); err != nil {
		slog.Warn("failed to set auth cookies", "error", err)
	}

	if h.CFSigner != nil {
		for _, cookie := range h.CFSigner.SignedCookies(time.Now().Add(auth.AuthTokenTTL())) {
			http.SetCookie(w, cookie)
		}
	}

	// Never log the username, password, hash or token: the username is half a
	// credential and the failure paths above do not echo decoded input.
	slog.Info("user logged in via local password", append(logger.RequestAttrs(r), "user_id", uuidToString(user.ID))...)
	writeJSON(w, http.StatusOK, LoginResponse{
		Token: tokenString,
		User:  h.userToResponse(user),
	})
}

// emailCodeLoginDisabled is the shared guard for the two email-code routes. The
// switch is server-side on purpose: hiding the form in the UI but leaving the
// endpoints live would keep an unusable mail path attackable.
func (h *Handler) emailCodeLoginDisabled(w http.ResponseWriter) bool {
	if !h.cfg.EmailCodeLoginDisabled {
		return false
	}
	writeFeatureDisabled(w, "email_code_login_disabled", "Email code login is not enabled on this server")
	return true
}
