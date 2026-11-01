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

// TestBriefBansKillingEitherDaemonExecutableName pins the process-name rule for
// both install generations (CLAWSTA-100 review): a new install runs the daemon
// as u-workers(.exe) while an old one still runs it as multica(.exe). A brief
// that named only the legacy spelling let an agent on a new machine run
// `taskkill /IM u-workers.exe` without recognising it was killing the daemon.
func TestBriefBansKillingEitherDaemonExecutableName(t *testing.T) {
	ctx := TaskContextForEnv{IssueID: "i-1", AgentName: "Eve", AgentID: "eve-1"}
	out := buildMetaSkillContent("claude", ctx)

	start := strings.Index(out, "Never terminate")
	end := strings.Index(out, "Cancel only the exact child PID")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("brief lost the daemon process-name rule")
	}
	rule := out[start:end]
	for _, name := range []string{"`u-workers`", "`u-workers.exe`", "`multica`", "`multica.exe`"} {
		if !strings.Contains(rule, name) {
			t.Errorf("daemon process-name rule does not name %s:\n%s", name, rule)
		}
	}
}
