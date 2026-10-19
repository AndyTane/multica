package selfhosttelemetry

import (
	"log/slog"
	"strings"

	"github.com/multica-ai/multica/server/internal/privacy"
)

// Config is read once during API server startup. Endpoint selection is
// deliberately absent: self-host telemetry can only be sent to Multica's
// first-party collector compiled into the client.
type Config struct {
	Enabled bool
}

// ConfigFromDoNotTrack preserves the upstream opt-out parser, but the local
// build policy takes precedence over every environment value.
func ConfigFromDoNotTrack(raw string) Config {
	raw = strings.TrimSpace(raw)
	disabled := raw == "1" || strings.EqualFold(raw, "true")
	return Config{Enabled: privacy.ExternalReportingEnabled && !disabled}
}

// LogStartupStatus reports the effective policy without logging telemetry data.
func LogStartupStatus(logger *slog.Logger, config Config) {
	if !privacy.ExternalReportingEnabled {
		logger.Info("self-host telemetry disabled by local build policy")
		return
	}
	if config.Enabled {
		logger.Info("self-host telemetry enabled")
		return
	}
	logger.Info("self-host telemetry disabled via DO_NOT_TRACK")
}
