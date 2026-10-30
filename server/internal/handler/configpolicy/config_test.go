// Package configpolicy exercises the public config handler without the parent
// handler suite's database-backed TestMain.
package configpolicy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func TestConfigDisablesUpstreamReportingAndBilling(t *testing.T) {
	t.Setenv("POSTHOG_API_KEY", "test-key")
	t.Setenv("POSTHOG_HOST", "https://collector.invalid")
	t.Setenv("ANALYTICS_ENVIRONMENT", "production")
	t.Setenv("FF_BILLING_WORKSPACE_SUBSCRIPTIONS", "on")
	t.Setenv("FF_COMPOSIO_MCP_APPS", "on")
	t.Setenv("GOOGLE_CLIENT_ID", "user-configured-client")
	t.Setenv("MULTICA_DAEMON_SERVER_URL", "https://user-api.example")
	t.Setenv("MULTICA_APP_URL", "https://user-app.example")
	h := &handler.Handler{FeatureFlags: featureflag.NewService(featureflag.NewEnvProvider("FF_"))}
	for _, disabled := range []string{"", "false", "0", "true", "1"} {
		t.Run(disabled, func(t *testing.T) {
			t.Setenv("ANALYTICS_DISABLED", disabled)
			w := httptest.NewRecorder()
			h.GetConfig(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d", w.Code)
			}
			var cfg handler.AppConfig
			if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.PosthogKey != "" || cfg.PosthogHost != "" || cfg.AnalyticsEnvironment != "" {
				t.Fatal("config exposed external reporting settings")
			}
			if cfg.FeatureFlags["billing_workspace_subscriptions"] {
				t.Fatal("environment re-enabled upstream cloud billing")
			}
			if !cfg.FeatureFlags["composio_mcp_apps"] || cfg.GoogleClientID != "user-configured-client" {
				t.Fatal("user-configured integration/auth settings were disabled")
			}
			if cfg.DaemonServerURL != "https://user-api.example" || cfg.DaemonAppURL != "https://user-app.example" {
				t.Fatal("user-configured endpoints were replaced")
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"posthog_key", "posthog_host", "analytics_environment"} {
				if string(raw[key]) != `""` {
					t.Fatalf("installed client field %s must stay present and empty", key)
				}
			}
		})
	}
}
