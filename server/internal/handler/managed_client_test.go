package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/managedkit"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestManagedClientPackageAuthorization(t *testing.T) {
	if testHandler == nil {
		t.Fatal("isolated database required")
	}
	cache := t.TempDir()
	t.Setenv("UWORKERS_CLIENT_CACHE", cache)
	oldVersion, oldCommit := testHandler.cfg.ServerVersion, testHandler.cfg.ServerCommit
	t.Cleanup(func() { testHandler.cfg.ServerVersion = oldVersion; testHandler.cfg.ServerCommit = oldCommit })
	data := []byte("verified client archive boundary fixture")
	sum := sha256.Sum256(data)
	offer := managedkit.Offer{Version: "v0.2.11", Commit: strings.Repeat("a", 40), Platform: "windows-amd64", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}
	dir := filepath.Join(cache, offer.Version)
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "client-kit.zip"), data, 0600)
	_ = managedkit.WriteJSON(filepath.Join(dir, "ready.json"), offer)
	testHandler.cfg.ServerVersion = offer.Version
	testHandler.cfg.ServerCommit = offer.Commit[:9]
	path := "/api/daemon/client-package?runtime_id=" + testRuntimeID + "&sha256=" + offer.SHA256
	testutil.Call(t, testHandler.DaemonClientPackage, newRequest(http.MethodGet, path, nil)).Want(http.StatusOK)
	testutil.Call(t, testHandler.DaemonClientPackage, newDaemonTokenRequest(http.MethodGet, path, nil, uuid.NewString(), "foreign-daemon")).Want(http.StatusNotFound)
	testutil.Call(t, testHandler.DaemonClientPackage, newRequest(http.MethodGet, "/api/daemon/client-package?runtime_id="+uuid.NewString()+"&sha256="+offer.SHA256, nil)).Want(http.StatusNotFound)
	testHandler.cfg.ServerVersion = "v0.2.10"
	testutil.Call(t, testHandler.DaemonClientPackage, newRequest(http.MethodGet, path, nil)).Want(http.StatusNotFound)
	testHandler.cfg.ServerVersion = offer.Version
	_ = os.WriteFile(filepath.Join(dir, "client-kit.zip"), []byte(strings.Repeat("x", len(data))), 0600)
	testutil.Call(t, testHandler.DaemonClientPackage, newRequest(http.MethodGet, path, nil)).Want(http.StatusServiceUnavailable)
}
