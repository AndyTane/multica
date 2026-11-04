package managedkit

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/multica-ai/multica/server/internal/tlsx"
)

func TestReadinessProbeRequiresOriginalTrustAndNoRedirect(t *testing.T) {
	status := http.StatusOK
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if status == http.StatusFound {
			w.Header().Set("Location", "https://unselected.invalid/readyz")
		}
		w.WriteHeader(status)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "public-ca.crt")
	_ = os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	t.Setenv(tlsx.RootCAEnv, file)
	if err := tlsx.InstallFromEnv(); err != nil {
		t.Fatal(err)
	}
	if !serverReady(server.URL) {
		t.Fatal("trusted server readiness not recognized")
	}
	status = http.StatusServiceUnavailable
	if serverReady(server.URL) {
		t.Fatal("unready server accepted")
	}
	status = http.StatusFound
	if serverReady(server.URL) {
		t.Fatal("release-origin redirect followed")
	}
	server.Close()
	if serverReady(server.URL) {
		t.Fatal("offline server accepted")
	}
}
