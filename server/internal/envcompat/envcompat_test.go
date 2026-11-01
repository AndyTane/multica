package envcompat

import (
	"sort"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/brand"
)

func TestPlanBridgesEveryRenameMatrix(t *testing.T) {
	const (
		legacyServerURL = brand.LegacyEnvPrefix + "SERVER_URL"
		newServerURL    = brand.EnvPrefix + "SERVER_URL"
		legacyToken     = brand.LegacyEnvPrefix + "TOKEN"
		newToken        = brand.EnvPrefix + "TOKEN"
	)

	tests := []struct {
		name    string
		environ []string
		want    map[string]string
	}{
		{
			name:    "only the legacy name is set",
			environ: []string{legacyServerURL + "=https://legacy.example", legacyToken + "=legacy-token"},
			want: map[string]string{
				legacyServerURL: "https://legacy.example",
				newServerURL:    "https://legacy.example",
				legacyToken:     "legacy-token",
				newToken:        "legacy-token",
			},
		},
		{
			name:    "only the current name is set",
			environ: []string{newServerURL + "=https://current.example"},
			want: map[string]string{
				newServerURL:    "https://current.example",
				legacyServerURL: "https://current.example",
			},
		},
		{
			name: "both set: the current name wins",
			environ: []string{
				legacyServerURL + "=https://legacy.example",
				newServerURL + "=https://current.example",
			},
			want: map[string]string{
				newServerURL:    "https://current.example",
				legacyServerURL: "https://current.example",
			},
		},
		{
			name:    "neither name is set",
			environ: []string{"PATH=/usr/bin", "HOME=/home/user"},
			want:    map[string]string{},
		},
		{
			name:    "empty values are copied, not skipped",
			environ: []string{newServerURL + "="},
			want: map[string]string{
				newServerURL:    "",
				legacyServerURL: "",
			},
		},
		{
			name:    "names without either prefix are untouched",
			environ: []string{"DATABASE_URL=postgres://localhost/db"},
			want:    map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Effective environment = what was passed in, plus the bridge's
			// assignments applied on top.
			effective := map[string]string{}
			for _, entry := range tt.environ {
				name, value, ok := strings.Cut(entry, "=")
				if ok {
					effective[name] = value
				}
			}
			for _, assignment := range Plan(tt.environ) {
				effective[assignment[0]] = assignment[1]
			}

			for name, value := range effective {
				if !strings.HasPrefix(name, brand.EnvPrefix) && !strings.HasPrefix(name, brand.LegacyEnvPrefix) {
					continue
				}
				if _, expected := tt.want[name]; !expected {
					t.Fatalf("after Plan() the product name %q is set to %q, but the case expects only %v", name, value, tt.want)
				}
			}
			for name, want := range tt.want {
				if got := effective[name]; got != want {
					t.Fatalf("after Plan() %q = %q, want %q", name, got, want)
				}
			}
		})
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	environ := []string{
		brand.EnvPrefix + "ZED=z",
		brand.LegacyEnvPrefix + "ALPHA=a",
		brand.EnvPrefix + "MIKE=m",
	}
	first := Plan(environ)
	second := Plan(environ)
	if len(first) != len(second) {
		t.Fatalf("Plan() returned %d then %d assignments", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("Plan() is not deterministic at %d: %v then %v", i, first[i], second[i])
		}
	}
	targets := make([]string, len(first))
	for i, assignment := range first {
		targets[i] = assignment[0]
	}
	if !sort.StringsAreSorted(targets) {
		t.Fatalf("Plan() assignment targets = %v, want sorted", targets)
	}
}

func TestBridgeAppliesPlannedAssignments(t *testing.T) {
	legacy := brand.LegacyEnvPrefix + "HTTP_TIMEOUT"
	current := brand.EnvPrefix + "HTTP_TIMEOUT"
	applied := map[string]string{}
	bridge([]string{current + "=7s"}, func(key, value string) error {
		applied[key] = value
		return nil
	})
	if applied[legacy] != "7s" {
		t.Fatalf("bridge() applied %v, want %s=7s", applied, legacy)
	}
}

func TestBridgeLeavesUnrelatedNamesAlone(t *testing.T) {
	applied := map[string]string{}
	bridge([]string{"PATH=/usr/bin", "DATABASE_URL=postgres://localhost/db"}, func(key, value string) error {
		applied[key] = value
		return nil
	})
	if len(applied) != 0 {
		t.Fatalf("bridge() applied %v, want no assignments", applied)
	}
}
