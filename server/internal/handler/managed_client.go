package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/multica-ai/multica/server/internal/managedkit"
)

func (h *Handler) managedClientOffer() *managedkit.Offer {
	return managedkit.ActiveOffer(os.Getenv("UWORKERS_CLIENT_CACHE"), h.cfg.ServerVersion, h.cfg.ServerCommit)
}

// DaemonClientPackage uses the same runtime/workspace gate as task operations.
// It never proxies a caller-supplied URL or exposes publisher credentials.
func (h *Handler) DaemonClientPackage(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireDaemonRuntimeAccess(w, r, r.URL.Query().Get("runtime_id")); !ok {
		return
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
