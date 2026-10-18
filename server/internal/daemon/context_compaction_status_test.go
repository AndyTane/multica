package daemon

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/multica-ai/multica/server/pkg/agent"
)

func TestExecuteAndDrainReportsCompactionLifecycle(t *testing.T) {
	d, recorder := newTranscriptRecorder(t)
	messages := make(chan agent.Message, 5)
	messages <- agent.Message{Type: agent.MessageStatus, Status: "running"}
	for _, status := range []string{"context_compaction_started", "context_compaction_completed", "context_compaction_failed"} {
		messages <- agent.Message{Type: agent.MessageStatus, Status: status, Content: "Context maintenance"}
	}
	messages <- agent.Message{Type: agent.MessageText, Content: "Deliverable"}
	close(messages)
	results := make(chan agent.Result, 1)
	results <- agent.Result{Status: "completed", Output: "Deliverable"}
	close(results)
	backend := sessionBackend{session: &agent.Session{Messages: messages, Result: results}}
	var seq atomic.Int32
	if _, _, err := d.executeAndDrain(context.Background(), backend, "p", agent.ExecOptions{}, slog.Default(), "compaction-run", "", &seq); err != nil {
		t.Fatal(err)
	}
	rows := recorder.snapshot()
	if len(rows) != 4 {
		t.Fatalf("got %d transcript rows, want three maintenance events and one reply", len(rows))
	}
	for index, want := range []string{"context_compaction_started", "context_compaction_completed", "context_compaction_failed", "text"} {
		if rows[index].Type != want || rows[index].Seq != index+1 || rows[index].CreatedAt.IsZero() {
			t.Fatalf("row %d lost its lifecycle type, sequence or timestamp: %+v", index, rows[index])
		}
	}
}
