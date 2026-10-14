package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/agent"
)

func TestAgentContextCompactionWriteValidation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	name := fmt.Sprintf("context-compaction-%d", time.Now().UnixNano())
	runtimeID := handlerTestRuntimeID(t)
	invalid := map[string]any{"context_compaction": map[string]any{"threshold_tokens": 0}}
	testutil.Call(t, testHandler.CreateAgent, newRequest(http.MethodPost, "/api/agents", map[string]any{
		"name": name, "runtime_id": runtimeID, "runtime_config": invalid,
	})).Want(http.StatusBadRequest)

	var created AgentResponse
	testutil.Call(t, testHandler.CreateAgent, newRequest(http.MethodPost, "/api/agents", map[string]any{
		"name": name, "runtime_id": runtimeID,
		"runtime_config": map[string]any{"unrelated": "preserved", "context_compaction": map[string]any{"enabled": false, "threshold_tokens": 120000}},
	})).Want(http.StatusCreated).JSON(&created)
	dbfx.Cleanup(t, `DELETE FROM agent WHERE id = $1`, created.ID)
	testutil.Call(t, testHandler.UpdateAgent, withURLParam(newRequest(http.MethodPut, "/api/agents/"+created.ID, map[string]any{
		"runtime_config": invalid,
	}), "id", created.ID)).Want(http.StatusBadRequest)

	var preserved AgentResponse
	testutil.Call(t, testHandler.UpdateAgent, withURLParam(newRequest(http.MethodPut, "/api/agents/"+created.ID, map[string]any{
		"description": "preserve compaction policy",
	}), "id", created.ID)).Want(http.StatusOK).JSON(&preserved)
	raw, err := json.Marshal(preserved.RuntimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := agent.ParseContextCompaction(raw)
	if err != nil || policy.Enabled || policy.ThresholdTokens != 120000 {
		t.Fatalf("policy changed after rejected or unrelated update: %#v, %v", policy, err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil || string(config["unrelated"]) != `"preserved"` {
		t.Fatal("unrelated runtime configuration was lost")
	}
}
