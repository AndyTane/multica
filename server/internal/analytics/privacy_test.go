package analytics

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBuildPolicyDisablesAnalyticsRegardlessOfEnvironment(t *testing.T) {
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
