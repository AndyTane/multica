package execenv

import (
	"strings"
	"testing"
)

// TestBriefTeachesTheCLIAndConfigTransition pins the CLAWSTA-95 transition
// note in the brief header. The product is U Workers, the CLI's current
// command is `u-workers`, and the legacy `multica` executable plus the legacy
// MULTICA_* env prefix and ~/.multica config directory are still valid for
// this release. Both names have to stay named: the command examples keep the
// legacy spelling while the built-in skills' tool grants are keyed to it, so a
// copy edit that dropped one spelling would leave an agent on a machine with
// only the other one unable to tell that the two are the same binary.
func TestBriefTeachesTheCLIAndConfigTransition(t *testing.T) {
	ctx := TaskContextForEnv{IssueID: "i-1", AgentName: "Eve", AgentID: "eve-1"}
	out := buildMetaSkillContent("claude", ctx)

	if !strings.HasPrefix(out, "# U Workers Agent Runtime\n\n") {
		head := out
		if len(head) > 200 {
			head = head[:200]
		}
		t.Fatalf("brief does not open with the U Workers runtime header:\n%s", head)
	}
	for _, want := range []string{
		"Use the `u-workers` CLI",
		"also installed as `multica`",
		"Both executable names run identical commands",
		"`U_WORKERS_*`",
		"`MULTICA_*`",
		"the current name wins when both are set",
		"still use the legacy prefix",
		"`~/.u-workers`",
		"`~/.multica`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("brief lost the CLI/config transition statement %q", want)
		}
	}
	if strings.Contains(out, "Multica") {
		t.Errorf("brief still carries the old product name Multica")
	}
}
