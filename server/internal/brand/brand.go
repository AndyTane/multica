// Package brand owns the product's user- and operator-visible identifiers.
//
// The product is being renamed from Multica to U Workers. Every external
// identifier therefore has two spellings during the transition: the new one
// (the primary constant in this package) and the legacy one that shipped
// before it. Readers accept both. Writers emit the new spelling and, where a
// peer built before the rename may still read it, the legacy spelling too.
//
// The transition and its removal criteria are documented in
// docs/plans/2026-11-01-u-workers-brand-batch-c.md. Keep one legacy alias per
// identifier: a second historical spelling would not be removable.
package brand

import (
	"path/filepath"
	"strings"
)

const (
	// DisplayName is the product name shown to people.
	DisplayName = "U Workers"
	// LegacyDisplayName is the product name before the rename.
	LegacyDisplayName = "Multica"

	// CLICommand is the executable name current releases install and document.
	CLICommand = "u-workers"
	// LegacyCLICommand is the executable name installed before the rename.
	LegacyCLICommand = "multica"

	// EnvPrefix is the prefix of the environment variables read by the CLI,
	// the daemon, and the server.
	EnvPrefix = "U_WORKERS_"
	// LegacyEnvPrefix is the environment prefix used before the rename.
	LegacyEnvPrefix = "MULTICA_"

	// ConfigDir is the per-user directory that owns CLI profiles, daemon
	// state, and config.json. It sits directly under the user's home.
	ConfigDir = ".u-workers"
	// LegacyConfigDir is the per-user directory used before the rename.
	// Existing installs keep resolving to it so an upgrade never loses state.
	LegacyConfigDir = ".multica"

	// AuthCookie carries the session token.
	AuthCookie = "u_workers_auth"
	// LegacyAuthCookie is the session token cookie issued before the rename.
	LegacyAuthCookie = "multica_auth"
	// CSRFCookie carries the token-bound CSRF value readable by browsers.
	CSRFCookie = "u_workers_csrf"
	// LegacyCSRFCookie is the token-bound CSRF cookie issued before the rename.
	LegacyCSRFCookie = "multica_csrf"
	// SessionCSRFCookie carries the session-bound CSRF value readable by browsers.
	SessionCSRFCookie = "u_workers_csrf_session"
	// LegacySessionCSRFCookie is the session-bound CSRF cookie from before the rename.
	LegacySessionCSRFCookie = "multica_csrf_session"

	// ReleaseArchivePrefix starts every versioned CLI release archive name,
	// for example u-workers-cli-0.3.2-windows-amd64.zip.
	ReleaseArchivePrefix = "u-workers-cli"
	// LegacyReleaseArchivePrefix is the versioned archive prefix from before
	// the rename.
	LegacyReleaseArchivePrefix = "multica-cli"
)

// HTTP header names. Response headers that cross the rename are written under
// both names; request headers are read under both. The pairing is explicit so
// a reader never has to build a name by string surgery.
const (
	HeaderNextBefore               = "X-U-Workers-Next-Before"
	LegacyHeaderNextBefore         = "X-Multica-Next-Before"
	HeaderNextBeforeID             = "X-U-Workers-Next-Before-Id"
	LegacyHeaderNextBeforeID       = "X-Multica-Next-Before-Id"
	HeaderRetryable                = "X-U-Workers-Retryable"
	LegacyHeaderRetryable          = "X-Multica-Retryable"
	HeaderPluginInstallation       = "X-U-Workers-Plugin-Installation"
	LegacyHeaderPluginInstallation = "X-Multica-Plugin-Installation"
	HeaderHookTimestamp            = "X-U-Workers-Timestamp"
	LegacyHeaderHookTimestamp      = "X-Multica-Timestamp"
	HeaderHookSignature            = "X-U-Workers-Signature"
	LegacyHeaderHookSignature      = "X-Multica-Signature"
	HeaderRateLimitScope           = "X-U-Workers-RateLimit-Scope"
	LegacyHeaderRateLimitScope     = "X-Multica-RateLimit-Scope"
)

// EnvName maps a legacy-prefixed environment variable name to its current
// name. ok is false when name does not carry the legacy prefix.
func EnvName(legacy string) (name string, ok bool) {
	rest, ok := strings.CutPrefix(legacy, LegacyEnvPrefix)
	if !ok {
		return "", false
	}
	return EnvPrefix + rest, true
}

// LegacyEnvName maps a current-prefixed environment variable name to its
// legacy name. ok is false when name does not carry the current prefix.
func LegacyEnvName(name string) (legacy string, ok bool) {
	rest, ok := strings.CutPrefix(name, EnvPrefix)
	if !ok {
		return "", false
	}
	return LegacyEnvPrefix + rest, true
}

// IsProductEnvKey reports whether key belongs to the product's environment
// namespace under either prefix. Every filter that keeps daemon-owned settings
// away from a task child must use it: Bridge mirrors each name onto the other
// prefix, so a filter that only knows one prefix lets the mirror through and
// the child's own Bridge then lets that value win. Environment names are
// case-insensitive on Windows, so the comparison is too.
func IsProductEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	return strings.HasPrefix(upper, EnvPrefix) || strings.HasPrefix(upper, LegacyEnvPrefix)
}

// ConfigRoot returns the per-user directory that owns CLI profiles and
// config.json for the given home directory.
//
// The choice follows where CLI state actually lives, not which directory merely
// exists. Other product runtimes create their own entries under the legacy
// directory (the Pi runtime writes ~/.multica/pi-sessions), so a fresh install
// — whose state is already in ~/.u-workers/config.json — must not be moved to
// the legacy directory by an unrelated directory appearing beside it.
//
// Order:
//  1. ~/.u-workers when it already holds CLI state (config.json or profiles/);
//  2. otherwise ~/.multica when it exists (an install from before the rename
//     keeps its login, profiles, and daemon state);
//  3. otherwise ~/.u-workers (a machine that has never run the product).
//
// exists reports whether a path exists (file or directory). Callers pass
// os.Stat's result so this stays testable without touching the filesystem.
func ConfigRoot(home string, exists func(path string) bool) string {
	current := filepath.Join(home, ConfigDir)
	if exists(filepath.Join(current, "config.json")) || exists(filepath.Join(current, "profiles")) {
		return current
	}
	legacy := filepath.Join(home, LegacyConfigDir)
	if exists(legacy) {
		return legacy
	}
	return current
}

// IsCLICommandName reports whether base is one of the CLI executable names
// this build answers to, with or without the platform's .exe suffix.
func IsCLICommandName(base string) bool {
	switch strings.TrimSuffix(strings.ToLower(base), ".exe") {
	case CLICommand, LegacyCLICommand:
		return true
	}
	return false
}

// CLICommandFor returns the command name to show in help and version output
// for the executable base name the process was started as. Builds that are
// invoked under an unrecognized name (tests, installers) keep the legacy name
// so existing scripted output does not change.
func CLICommandFor(base string) string {
	if strings.TrimSuffix(strings.ToLower(base), ".exe") == CLICommand {
		return CLICommand
	}
	return LegacyCLICommand
}
