package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/pkg/agent"
)

func TestOpenCodeShellDefaultClientClaims(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%v", batch), func(t *testing.T) {
			seconds := int64(7200)
			response := handler.AgentTaskResponse{ID: "isolated", OpenCodeShellDefaultSeconds: &seconds}
			var responseMu sync.Mutex
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				responseMu.Lock()
				defer responseMu.Unlock()
				if batch {
					json.NewEncoder(w).Encode(map[string]any{"tasks": []handler.AgentTaskResponse{response}})
				} else {
					json.NewEncoder(w).Encode(map[string]any{"task": response})
				}
			}))
			defer srv.Close()
			client := NewClient(srv.URL)
			claim := func() *Task {
				t.Helper()
				if batch {
					tasks, err := client.ClaimTasks(context.Background(), "daemon", []string{"runtime"}, 1)
					if err != nil || len(tasks) != 1 {
						t.Fatalf("batch: %+v %v", tasks, err)
					}
					return tasks[0]
				}
				task, err := client.ClaimTask(context.Background(), "runtime")
				if err != nil || task == nil {
					t.Fatalf("single: %+v %v", task, err)
				}
				return task
			}
			first := claim()
			local := Config{AgentIdleWatchdog: 11 * time.Second, AgentToolWatchdog: 13 * time.Second, AgentTimeout: 17 * time.Second, OpenCodeIdleWatchdog: 19 * time.Second, CodexSemanticInactivityTimeout: 23 * time.Second, CodexFirstTurnNoProgressTimeout: 29 * time.Second}
			if first.ExecutionTimeouts != nil || !reflect.DeepEqual(executionConfigForTask(local, first.ExecutionTimeouts), local) {
				t.Fatal("derived shell default overwrote local platform budgets")
			}
			if got := openCodeShellDefaultTimeout(first.OpenCodeShellDefaultSeconds, "opencode"); got != 2*time.Hour {
				t.Fatalf("initial default: %s", got)
			}
			responseMu.Lock()
			seconds = 10800
			responseMu.Unlock()
			second := claim()
			if got := openCodeShellDefaultTimeout(second.OpenCodeShellDefaultSeconds, "opencode"); got != 3*time.Hour {
				t.Fatalf("refreshed default: %s", got)
			}
			responseMu.Lock()
			response.OpenCodeShellDefaultSeconds = nil // Older servers omit the optional field.
			responseMu.Unlock()
			legacy := claim()
			if legacy.OpenCodeShellDefaultSeconds != nil || openCodeShellDefaultTimeout(legacy.OpenCodeShellDefaultSeconds, "opencode") != 0 {
				t.Fatal("missing field must preserve old client behavior")
			}
			if *first.OpenCodeShellDefaultSeconds != 7200 {
				t.Fatal("later claim mutated the first run's snapshot")
			}
			for _, provider := range []string{"claude", "codex", "dsh", "codearts"} {
				if openCodeShellDefaultTimeout(second.OpenCodeShellDefaultSeconds, provider) != 0 {
					t.Fatalf("shell default leaked to %s", provider)
				}
			}
		})
	}
}

func TestOpenCodeShellDefaultClaimValidation(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"null", true}, {"1", true}, {fmt.Sprint(agent.MaxExecutionTimeoutSeconds), true},
		{"0", false}, {"-1", false}, {"1.1", false}, {`"7200"`, false},
		{fmt.Sprint(agent.MaxExecutionTimeoutSeconds + 1), false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			var task Task
			err := json.Unmarshal([]byte(`{"id":"isolated","opencode_shell_default_seconds":`+tc.value+`}`), &task)
			if (err == nil) != tc.valid {
				t.Fatalf("decode: %v, valid=%v", err, tc.valid)
			}
		})
	}
}
