package privacy

import "testing"

func TestEnvironmentCannotEnableReporting(t *testing.T) {
	for _, value := range []string{"", "0", "false", "1", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ANALYTICS_DISABLED", value)
			t.Setenv("MULTICA_TELEMETRY_DISABLED", value)
			t.Setenv("POSTHOG_API_KEY", "fake-project-key")
			if ExternalReportingEnabled {
				t.Fatal("environment settings must not enable external reporting")
			}
		})
	}
}
