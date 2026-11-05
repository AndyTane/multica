package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/multica-ai/multica/server/internal/managedkit"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// requireClientWorkspace permits connection/update control, not task execution.
func (h *Handler) requireClientWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, daemonID string) (string, bool) {
	id, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return "", false
	}
	workspaceID = uuidToString(id)
	if daemonID == "" || len(daemonID) > 256 {
		writeError(w, 400, "daemon_id is required")
		return "", false
	}
	if scoped := middleware.DaemonWorkspaceIDFromContext(r.Context()); scoped != "" {
		if scoped != workspaceID || middleware.DaemonIDFromContext(r.Context()) != daemonID {
			writeError(w, 404, "workspace not found")
			return "", false
		}
	} else if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return "", false
	}
	if _, err := h.Queries.GetWorkspace(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return "", false
	}
	return workspaceID, true
}

// DaemonClientHeartbeat is an authenticated machine control path. It creates
// no runtime, does not claim work and needs no synthetic execution capability.
func (h *Handler) DaemonClientHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspaceID string   `json:"workspace_id"`
		DaemonID    string   `json:"daemon_id"`
		Unavailable []string `json:"unavailable_builtin_providers"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req) != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	wsID, ok := h.requireClientWorkspace(w, r, req.WorkspaceID, req.DaemonID)
	if !ok {
		return
	}
	// A new process reconciles positively missing engines before claiming.
	// Never delete rows/tasks or touch another machine, owner or custom profile.
	if len(req.Unavailable) > 0 {
		if len(req.Unavailable) > 128 {
			writeError(w, 400, "too many providers")
			return
		}
		rows, err := h.Queries.ListAgentRuntimes(r.Context(), parseUUID(wsID))
		if err != nil {
			writeError(w, 500, "cannot reconcile engines")
			return
		}
		missing := map[string]bool{}
		for _, p := range req.Unavailable {
			missing[p] = true
		}
		for _, rt := range rows {
			if rt.ProfileID.Valid || rt.DaemonID.String != req.DaemonID || !missing[rt.Provider] || rt.Status == "offline" {
				continue
			}
			if middleware.DaemonWorkspaceIDFromContext(r.Context()) == "" && uuidToString(rt.OwnerID) != requestUserID(r) {
				continue
			}
			if err := h.setRuntimeOffline(r.Context(), rt.ID, nil); err != nil {
				writeError(w, 500, "cannot retire unavailable engine")
				return
			}
		}
		h.publish(protocol.EventDaemonRegister, wsID, "system", "", map[string]any{"action": "reconcile"})
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, 200, map[string]any{"status": "connected", "workspace_id": wsID, "daemon_id": req.DaemonID, "managed_client_update": h.managedClientOffer()})
}

func (h *Handler) managedClientOffer() *managedkit.Offer {
	return managedkit.ActiveOffer(os.Getenv("UWORKERS_CLIENT_CACHE"), h.cfg.ServerVersion, h.cfg.ServerCommit)
}

// DaemonClientPackage uses the same runtime/workspace gate as task operations.
// It never proxies a caller-supplied URL or exposes publisher credentials.
func (h *Handler) DaemonClientPackage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("workspace_id") != "" {
		if _, ok := h.requireClientWorkspace(w, r, r.URL.Query().Get("workspace_id"), strings.TrimSpace(r.URL.Query().Get("daemon_id"))); !ok {
			return
		}
	} else {
		if _, ok := h.requireDaemonRuntimeAccess(w, r, r.URL.Query().Get("runtime_id")); !ok {
			return
		}
	}
	offer := h.managedClientOffer()
	if offer == nil || r.URL.Query().Get("sha256") != offer.SHA256 {
		writeError(w, http.StatusNotFound, "client update is not ready")
		return
	}
	path := filepath.Join(os.Getenv("UWORKERS_CLIENT_CACHE"), offer.Version, "client-kit.zip")
	if managedkit.CheckFile(path, offer.SHA256, offer.Bytes) != nil {
		writeError(w, http.StatusServiceUnavailable, "client update cache failed verification")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, path)
}
