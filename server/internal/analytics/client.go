// Package analytics preserves the event API used by handlers without collecting,
// buffering, or transmitting telemetry. Local diagnostics remain independent.
package analytics

import (
	"log/slog"
	"time"
)

// Event is the existing handler event contract. This build discards captures.
type Event struct {
	// Name of the event (e.g. "signup", "workspace_created").
	Name string

	// DistinctID identifies the person this event belongs to. For logged-in
	// users this is user.id; for anonymous events it should be the anon_id
	// that was previously used on the frontend so identity merging works.
	DistinctID string

	// WorkspaceID scopes the event to a workspace. Required when the event is
	// about a workspace-level action (workspace_created, issue_executed, ...).
	// Empty is allowed for pre-workspace events (signup).
	WorkspaceID string

	// Properties is the free-form bag of event attributes. Only serialisable
	// values (string, number, bool, nested maps/slices of the same) should
	// go here. Never put raw PII like full emails here — use email_domain.
	Properties map[string]any

	// SetOnce properties attach to the person record and are only written the
	// first time they appear. Use this for acquisition attribution
	// (initial_utm_source, etc.) so later events don't overwrite the origin.
	SetOnce map[string]any

	// Set properties attach to the person record and overwrite on every write.
	// Use this for mutable cohort signals (role, use_case, platform_preference)
	// that users can legitimately change during onboarding.
	Set map[string]any

	// Timestamp is optional.
	Timestamp time.Time
}

// Client is the narrow surface the rest of the codebase depends on.
type Client interface {
	Capture(e Event)
	// Close is safe during graceful shutdown; there are no pending events.
	Close()
}

// NewFromEnv always returns a no-op client. Historical reporting environment
// variables cannot re-enable a transport: this build does not include one.
func NewFromEnv() Client {
	slog.Info("analytics disabled by local build policy")
	return NoopClient{}
}

// NoopClient silently drops all events without retaining their contents.
type NoopClient struct{}

func (NoopClient) Capture(Event) {}
func (NoopClient) Close()        {}
