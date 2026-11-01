package brand

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryRenameHasDistinctSpellings(t *testing.T) {
	pairs := map[string][2]string{
		"display name":       {DisplayName, LegacyDisplayName},
		"cli command":        {CLICommand, LegacyCLICommand},
		"env prefix":         {EnvPrefix, LegacyEnvPrefix},
		"config directory":   {ConfigDir, LegacyConfigDir},
		"auth cookie":        {AuthCookie, LegacyAuthCookie},
		"csrf cookie":        {CSRFCookie, LegacyCSRFCookie},
		"session csrf":       {SessionCSRFCookie, LegacySessionCSRFCookie},
		"release archive":    {ReleaseArchivePrefix, LegacyReleaseArchivePrefix},
		"next-before header": {HeaderNextBefore, LegacyHeaderNextBefore},
		"retryable header":   {HeaderRetryable, LegacyHeaderRetryable},
		"plugin header":      {HeaderPluginInstallation, LegacyHeaderPluginInstallation},
		"hook timestamp":     {HeaderHookTimestamp, LegacyHeaderHookTimestamp},
		"hook signature":     {HeaderHookSignature, LegacyHeaderHookSignature},
		"rate limit scope":   {HeaderRateLimitScope, LegacyHeaderRateLimitScope},
	}
	for name, pair := range pairs {
		if pair[0] == pair[1] {
			t.Errorf("%s: current and legacy spellings are identical (%q)", name, pair[0])
		}
		if pair[0] == "" || pair[1] == "" {
			t.Errorf("%s: empty spelling in pair %v", name, pair)
		}
	}
}

func TestCookieNamesStayWithinTheCookieGrammar(t *testing.T) {
	// RFC 6265 cookie-name = token; these must not need quoting.
	for _, name := range []string{AuthCookie, LegacyAuthCookie, CSRFCookie, LegacyCSRFCookie, SessionCSRFCookie, LegacySessionCSRFCookie} {
		if strings.ContainsAny(name, " ,;=\"") {
			t.Errorf("cookie name %q contains a character the cookie grammar forbids", name)
		}
	}
}

func TestEnvNameRoundTrips(t *testing.T) {
	legacy := LegacyEnvPrefix + "SERVER_URL"
	current, ok := EnvName(legacy)
	if !ok || current != EnvPrefix+"SERVER_URL" {
		t.Fatalf("EnvName(%q) = %q, %v", legacy, current, ok)
	}
	back, ok := LegacyEnvName(current)
	if !ok || back != legacy {
		t.Fatalf("LegacyEnvName(%q) = %q, %v", current, back, ok)
	}
	if _, ok := EnvName("PATH"); ok {
		t.Fatal("EnvName(PATH) reported the legacy prefix")
	}
	if _, ok := LegacyEnvName("PATH"); ok {
		t.Fatal("LegacyEnvName(PATH) reported the current prefix")
	}
}

func TestConfigRootPrefersAnExistingLegacyDirectory(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, LegacyConfigDir)

	got := ConfigRoot(home, func(path string) bool { return path == legacy })
	if got != legacy {
		t.Fatalf("ConfigRoot() = %q, want the existing legacy directory %q", got, legacy)
	}

	got = ConfigRoot(home, func(string) bool { return false })
	want := filepath.Join(home, ConfigDir)
	if got != want {
		t.Fatalf("ConfigRoot() = %q, want %q on a machine without legacy state", got, want)
	}
}

func TestCLICommandForReportsTheInvokedName(t *testing.T) {
	tests := map[string]string{
		"u-workers":     CLICommand,
		"u-workers.exe": CLICommand,
		"U-Workers":     CLICommand,
		"multica":       LegacyCLICommand,
		"multica.exe":   LegacyCLICommand,
		"cmd.test":      LegacyCLICommand,
		"":              LegacyCLICommand,
	}
	for base, want := range tests {
		if got := CLICommandFor(base); got != want {
			t.Errorf("CLICommandFor(%q) = %q, want %q", base, got, want)
		}
	}
}
