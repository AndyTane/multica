package configpolicy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/multica-ai/multica/server/internal/handler"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestConfigPublicAppURL(t *testing.T) {
	for _, tc := range []struct{ name, app, frontend, want string }{
		{"app priority", " https://work.example.test:14443/ ", "http://other.example.test", "https://work.example.test:14443"},
		{"frontend", "", "http://192.168.50.10:13443", "http://192.168.50.10:13443"},
		{"invalid app fallback", "not a URL", "https://work.example.test", "https://work.example.test"},
		{"loopback app fallback", "http://localhost:3000", "https://work.example.test", "https://work.example.test"},
		{"IPv6", "https://[fd00::10]:14443", "", "https://[fd00::10]:14443"},
		{"missing", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MULTICA_APP_URL", tc.app)
			t.Setenv("FRONTEND_ORIGIN", tc.frontend)
			t.Setenv("MULTICA_PUBLIC_URL", "https://api.example.test")
			t.Setenv("MULTICA_DAEMON_SERVER_URL", "https://daemon.example.test")
			assertPublicAppURL(t, tc.want)
		})
	}
}

func TestConfigOmitsUnsafePublicAppURL(t *testing.T) {
	for _, raw := range []string{
		"ftp://work.example.test", "//work.example.test", "https://user:password@work.example.test",
		"https://work.example.test/path", "https://work.example.test?token=secret", "https://work.example.test#secret",
		"https://work.example.test:99999", "https://work.example.test:0", "https://work.example.test:",
		"http://localhost:3000", "http://LOCALHOST.:3000", "http://foo.localhost", "http://127.4.3.2",
		"http://127.1", "http://2130706433", "http://0x7f000001", "http://0.0.0.0", "http://[::]",
		"http://[::1]", "http://[::ffff:127.0.0.1]", "http://[fe80::1%25eth0]",
		"https://bad_host", "https://bad..host", "https://work.example.test\\other", "https://work.example.test\n",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("MULTICA_APP_URL", raw)
			t.Setenv("FRONTEND_ORIGIN", "")
			assertPublicAppURL(t, "")
		})
	}
}

func assertPublicAppURL(t *testing.T, want string) {
	t.Helper()
	h := &handler.Handler{Queries: db.New(noSettingsDB{})}
	w := httptest.NewRecorder()
	h.GetConfig(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil || w.Code != http.StatusOK {
		t.Fatalf("invalid config response: status=%d err=%v", w.Code, err)
	}
	if want == "" {
		if _, found := cfg["public_app_url"]; found {
			t.Fatal("unconfigured/unsafe public_app_url must be omitted")
		}
		return
	}
	var got string
	if err := json.Unmarshal(cfg["public_app_url"], &got); err != nil || got != want {
		t.Fatalf("public_app_url = %q, want %q (err=%v)", got, want, err)
	}
}

// Public config also reads the registration switch; no persisted setting means closed.
type noSettingsDB struct{}

func (noSettingsDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, pgx.ErrNoRows
}
func (noSettingsDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, pgx.ErrNoRows
}
func (noSettingsDB) QueryRow(context.Context, string, ...interface{}) pgx.Row { return noSettingsRow{} }

type noSettingsRow struct{}

func (noSettingsRow) Scan(...interface{}) error { return pgx.ErrNoRows }
