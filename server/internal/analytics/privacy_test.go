package analytics

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/multica-ai/multica/server/internal/privacy"
)

func TestBuildPolicyDisablesAnalyticsRegardlessOfEnvironment(t *testing.T) {
	if privacy.ExternalReportingEnabled {
		t.Fatal("external reporting must remain disabled")
	}
	var requests atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	for _, disabled := range []string{"", "0", "false", "1", "true"} {
		t.Run(disabled, func(t *testing.T) {
			t.Setenv("ANALYTICS_DISABLED", disabled)
			t.Setenv("POSTHOG_API_KEY", "test-project-key")
			t.Setenv("POSTHOG_HOST", collector.URL)
			client := NewFromEnv()
			client.Capture(Event{Name: "test-private-event", DistinctID: "test-user"})
			client.Close()
			if _, ok := client.(NoopClient); !ok {
				t.Error("build policy constructed a live analytics client")
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("disabled analytics sent %d requests, want 0", got)
	}
}

func TestDisabledAnalyticsLeavesUserConnectionsAvailable(t *testing.T) {
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer endpoint.Close()

	transport := http.DefaultTransport
	client := NewFromEnv()
	client.Capture(Event{Name: "test-private-event"})
	client.Close()
	if http.DefaultTransport != transport {
		t.Fatal("analytics changed the global HTTP transport")
	}
	resp, err := http.Get(endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || requests.Load() != 1 {
		t.Fatalf("user endpoint: status=%d requests=%d", resp.StatusCode, requests.Load())
	}
}
