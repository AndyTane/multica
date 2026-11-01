package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

// CLAWSTA-102: each of the four non-Feishu IM channels is off by default even
// with its at-rest key configured, and each comes back on its own switch. This
// goes through the REAL boot path — NewRouterWithOptions, the same assembly
// main() runs — and then reads the handler's own wire answer over HTTP, so the
// assertion is what the settings page actually receives rather than an
// internal field.
//
// DB-backed: the list endpoints sit behind the workspace-member middleware,
// which is what makes the response the same one a member gets. TestMain in
// integration_test.go skips the whole package when no database is reachable.
func TestChannelFeatureFlagsGateConfiguredChannels(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x47}, secretbox.KeySize))
	for _, env := range []string{
		"MULTICA_SLACK_SECRET_KEY",
		"MULTICA_TELEGRAM_SECRET_KEY",
		"MULTICA_DINGTALK_SECRET_KEY",
		"MULTICA_WECOM_SECRET_KEY",
	} {
		t.Setenv(env, key)
	}

	channels := []struct {
		name     string
		flagEnv  string
		endpoint string
	}{
		{"slack", "FF_CHANNEL_SLACK", "slack"},
		{"telegram", "FF_CHANNEL_TELEGRAM", "telegram"},
		{"dingtalk", "FF_CHANNEL_DINGTALK", "dingtalk"},
		{"wecom", "FF_CHANNEL_WECOM", "wecom"},
	}

	// boot starts one server on the env-backed flag service, exactly as a
	// restart with the current environment would.
	boot := func(t *testing.T) *httptest.Server {
		t.Helper()
		router, _ := NewRouterWithOptions(
			testPool,
			realtime.NewHub(),
			events.New(),
			analytics.NoopClient{},
			nil,
			RouterOptions{FeatureFlags: testFeatureFlags(t)},
		)
		server := httptest.NewServer(router)
		t.Cleanup(server.Close)
		return server
	}

	configured := func(t *testing.T, baseURL, endpoint string) bool {
		t.Helper()
		url := fmt.Sprintf("%s/api/workspaces/%s/%s/installations", baseURL, testWorkspaceID, endpoint)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build %s request: %v", endpoint, err)
		}
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", endpoint, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s installations = %d, want 200", endpoint, resp.StatusCode)
		}
		var body struct {
			Configured bool `json:"configured"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s installations: %v", endpoint, err)
		}
		return body.Configured
	}

	t.Run("default off", func(t *testing.T) {
		server := boot(t)
		for _, channel := range channels {
			if configured(t, server.URL, channel.endpoint) {
				t.Errorf("%s reports configured:true with its key set but its switch off; a key alone must not enable a channel", channel.name)
			}
		}
	})

	for _, channel := range channels {
		t.Run(channel.name+" switch restores only that channel", func(t *testing.T) {
			t.Setenv(channel.flagEnv, "true")
			server := boot(t)
			for _, other := range channels {
				want := other.name == channel.name
				if got := configured(t, server.URL, other.endpoint); got != want {
					t.Errorf("with %s=true, %s installations configured = %t, want %t", channel.flagEnv, other.name, got, want)
				}
			}
		})
	}

	t.Run("all four switches on", func(t *testing.T) {
		for _, channel := range channels {
			t.Setenv(channel.flagEnv, "true")
		}
		server := boot(t)
		for _, channel := range channels {
			if !configured(t, server.URL, channel.endpoint) {
				t.Errorf("%s stays configured:false with %s=true", channel.name, channel.flagEnv)
			}
		}
	})
}
