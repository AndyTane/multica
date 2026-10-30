package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/storage"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func TestRouterRetiresCloudWithoutDisablingLocalServices(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	t.Setenv("MULTICA_CLOUD_URL", upstream.URL)
	t.Setenv("S3_BUCKET", "")
	t.Setenv("COMPOSIO_API_KEY", "")
	t.Setenv("LOCAL_UPLOAD_DIR", t.TempDir())
	router, h := NewRouterWithOptions(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil, RouterOptions{})
	if h.CloudRuntime != nil || h.Entitlements != nil || h.SeatCapacity != nil || h.SeatCapacityWorker != nil {
		t.Fatal("retired cloud services were assembled")
	}
	if h.TaskService.Entitlements != nil || h.IssueService.Entitlements != nil || h.AutopilotService.Entitlements != nil {
		t.Fatal("local business services still depend on cloud entitlements")
	}
	if h.Composio != nil {
		t.Fatal("unconfigured Composio was initialized")
	}
	if _, ok := h.Storage.(*storage.LocalStorage); !ok {
		t.Fatalf("default storage = %T, want local", h.Storage)
	}
	for _, path := range []string{
		"/api/cloud-runtime", "/api/cloud-runtime/nodes", "/api/cloud-billing/balance",
		"/api/cloud-subscriptions/summary", "/api/webhooks/stripe", "/api/me/onboarding/cloud-waitlist",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
			if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "U Workers") {
				t.Errorf("%s %s = %d %s", method, path, response.Code, response.Body.String())
			}
		}
	}
	// A cloud token cannot initiate remote validation. Ordinary local session
	// and PAT middleware remains installed and requires authentication.
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer mcn_test")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("retired cloud token response = %d", response.Code)
	}
	if calls.Load() != 0 {
		t.Fatalf("upstream calls = %d, want 0", calls.Load())
	}
}

func TestRouterPreservesConfiguredIntegrationsAndStorage(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || string(body) != "test attachment" {
			t.Errorf("unexpected configured storage request: %s %s", r.Method, body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	for key, value := range map[string]string{
		"S3_BUCKET": "user-bucket", "S3_REGION": "us-east-1",
		"AWS_ENDPOINT_URL": upstream.URL, "S3_USE_PATH_STYLE": "true",
		"AWS_ACCESS_KEY_ID": "test-key", "AWS_SECRET_ACCESS_KEY": "test-secret",
		"CLOUDFRONT_DOMAIN": "", "COMPOSIO_API_KEY": "test-composio-key",
		"COMPOSIO_STATE_SECRET":      "test-state-secret-at-least-32-bytes",
		"COMPOSIO_CALLBACK_BASE_URL": "https://workspace.example.test",
		"FF_COMPOSIO_MCP_APPS":       "on",
	} {
		t.Setenv(key, value)
	}
	flags, err := featureflag.NewServiceFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	_, h := NewRouterWithOptions(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil, RouterOptions{FeatureFlags: flags})
	if h.Composio == nil || h.TaskService.Composio == nil {
		t.Fatal("explicitly configured Composio integration was disabled")
	}
	if _, ok := h.Storage.(*storage.S3Storage); !ok {
		t.Fatalf("configured storage = %T, want S3", h.Storage)
	}
	if calls.Load() != 0 {
		t.Fatalf("storage was probed during assembly: %d calls", calls.Load())
	}
	if _, err := h.Storage.Upload(context.Background(), "attachment.txt", []byte("test attachment"), "text/plain", "attachment.txt"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("configured upload calls = %d, want 1", calls.Load())
	}
}
