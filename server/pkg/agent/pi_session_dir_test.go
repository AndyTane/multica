package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/multica-ai/multica/server/internal/brand"
)

func redirectPiHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// The Pi session store follows the CLI config root. Regression for CLAWSTA-99
// review item 2: the Pi runtime creating ~/.multica/pi-sessions on a fresh
// install must not move the store (or the CLI root) away from the directory the
// user's state is actually in.
func TestPiSessionDirFollowsTheCLIConfigRoot(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, home string)
		want  string
	}{
		{
			name:  "fresh machine uses the current root",
			setup: func(*testing.T, string) {},
			want:  brand.ConfigDir,
		},
		{
			name: "upgraded install keeps the legacy root",
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, brand.LegacyConfigDir), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: brand.LegacyConfigDir,
		},
		{
			name: "current CLI state survives an unrelated legacy directory",
			setup: func(t *testing.T, home string) {
				t.Helper()
				current := filepath.Join(home, brand.ConfigDir)
				if err := os.MkdirAll(current, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(current, "config.json"), []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(home, brand.LegacyConfigDir, "pi-sessions"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: brand.ConfigDir,
		},
		{
			name: "current profiles directory alone is CLI state",
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, brand.ConfigDir, "profiles"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(home, brand.LegacyConfigDir), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: brand.ConfigDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			redirectPiHome(t, home)
			tt.setup(t, home)

			got, err := piSessionDir()
			if err != nil {
				t.Fatalf("piSessionDir: %v", err)
			}
			want := filepath.Join(home, tt.want, "pi-sessions")
			if got != want {
				t.Fatalf("piSessionDir() = %q, want %q", got, want)
			}
		})
	}
}
