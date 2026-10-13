package handler

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestTaskToResponseDshSession(t *testing.T) {
	t.Setenv("MULTICA_DSH_WEB_URL", "http://127.0.0.1:6006")
	response := taskToResponse(db.AgentTaskQueue{SessionID: pgtype.Text{String: "multica-web-test", Valid: true}}, "")
	if response.SessionID != "multica-web-test" || response.SessionWebURL != "http://127.0.0.1:6006/" {
		t.Fatalf("session response: %#v", response)
	}
	for _, input := range []string{"https://example.com", "http://127.0.0.1:6006/?token=private", "http://user:password@127.0.0.1:6006", "javascript:alert(1)", "http://127.0.0.1:6006/other"} {
		t.Setenv("MULTICA_DSH_WEB_URL", input)
		if got := dshSessionWebURL("multica-web-test"); got != "" {
			t.Fatalf("unsafe destination %q returned %q", input, got)
		}
	}
	t.Setenv("MULTICA_DSH_WEB_URL", "http://localhost:6006/")
	if got := dshSessionWebURL("claude-session"); got != "" {
		t.Fatalf("unrelated provider received web destination %q", got)
	}
}
