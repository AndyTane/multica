package agent

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/envcompat"
)

// A daemon bridges its own environment at startup, so every product setting it
// inherited also exists under the other prefix. The task child must still see
// only the values the daemon assembled for this task once the CLI inside it
// runs its own Bridge.
func TestMergeEnvKeepsTaskValuesAfterChildBridge(t *testing.T) {
	t.Parallel()

	daemonEnv := []string{
		"PATH=/usr/bin",
		"MULTICA_SERVER_URL=https://daemon.example",
		"U_WORKERS_SERVER_URL=https://daemon.example",
		"MULTICA_TOKEN=owner-pat",
		"U_WORKERS_TOKEN=owner-pat",
	}
	child := mergeEnv(daemonEnv, map[string]string{
		"MULTICA_SERVER_URL":       "https://task.example",
		"MULTICA_TOKEN":            "mat_task",
		"MULTICA_TASK_CONFIG_ROOT": "/task/root",
	})

	effective := map[string]string{}
	for _, entry := range child {
		for i := 0; i < len(entry); i++ {
			if entry[i] == '=' {
				effective[entry[:i]] = entry[i+1:]
				break
			}
		}
	}
	for _, assignment := range envcompat.Plan(child) {
		effective[assignment[0]] = assignment[1]
	}

	for name, want := range map[string]string{
		"MULTICA_SERVER_URL":         "https://task.example",
		"U_WORKERS_SERVER_URL":       "https://task.example",
		"MULTICA_TOKEN":              "mat_task",
		"U_WORKERS_TOKEN":            "mat_task",
		"MULTICA_TASK_CONFIG_ROOT":   "/task/root",
		"U_WORKERS_TASK_CONFIG_ROOT": "/task/root",
	} {
		if got := effective[name]; got != want {
			t.Errorf("%s after child bridge = %q, want %q (child env %v)", name, got, want, child)
		}
	}
}
