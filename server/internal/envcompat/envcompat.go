// Package envcompat lets one process answer to both the current and the
// legacy environment-variable spellings of every setting it reads.
//
// The rename from Multica to U Workers would otherwise require editing every
// os.Getenv call site and every operator's shell profile at the same moment.
// Instead each binary calls Bridge once at startup: every name under the new
// prefix is mirrored onto the legacy name and vice versa, and the code below
// keeps reading the single spelling it always read.
//
// Precedence is fixed and independent of environment order: when both
// spellings of a setting are present the current one wins; when only one is
// present its value is exposed under both names. Values are copied verbatim,
// including empty ones, because "set to empty" and "unset" are different
// decisions to the code that reads them.
//
// Bridge is a transition mechanism, not a permanent alias: the removal
// criteria live in docs/plans/2026-11-01-u-workers-brand-batch-c.md.
package envcompat

import (
	"os"
	"sort"
	"strings"

	"github.com/multica-ai/multica/server/internal/brand"
)

// Plan returns the assignments Bridge applies for environ. It is pure so the
// compatibility matrix can be tested without mutating the process
// environment, and sorted so output and failures are deterministic.
func Plan(environ []string) [][2]string {
	present := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		present[name] = value
	}

	assignments := make([][2]string, 0, len(present))
	for name, value := range present {
		// Current spelling present: it wins, and a legacy reader must observe
		// the same value even if the legacy name was also set.
		if legacy, ok := brand.LegacyEnvName(name); ok {
			assignments = append(assignments, [2]string{legacy, value})
			continue
		}
		// Legacy spelling present without a current one: expose it under the
		// current name so migrated code keeps working.
		if current, ok := brand.EnvName(name); ok {
			if _, currentSet := present[current]; !currentSet {
				assignments = append(assignments, [2]string{current, value})
			}
		}
	}

	sort.Slice(assignments, func(i, j int) bool {
		return assignments[i][0] < assignments[j][0]
	})
	return assignments
}

// Bridge rewrites this process's environment so both spellings of every
// product setting resolve to the effective value. Call it once, as early as
// possible in main, before the first environment read.
func Bridge() {
	bridge(os.Environ(), os.Setenv)
}

func bridge(environ []string, setenv func(key, value string) error) {
	for _, assignment := range Plan(environ) {
		// A failure here leaves the variable at its original value, which is
		// the pre-bridge behavior for that spelling; the process keeps running.
		_ = setenv(assignment[0], assignment[1])
	}
}
