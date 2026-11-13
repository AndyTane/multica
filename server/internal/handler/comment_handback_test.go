package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestCreateCommentHandsBackAwaitingReview(t *testing.T) {
	other := dbfx.User(t, "Other receiver", "handback-other@example.test")
	dbfx.Member(t, testWorkspaceID, other, "member")
	agent := createHandlerTestAgent(t, "Handback assignee", nil)
	for _, tc := range []struct {
		name, status, by, kind, receiver string
		agentAuthor, reply, wantReturn   bool
	}{
		{"approval receiver", "in_review", "user", "approve", testUserID, false, false, true},
		{"blocked reply in thread", "blocked", "user", "reply", testUserID, false, true, true},
		{"blocked act", "blocked", "user", "act", testUserID, false, false, true},
		{"dispatcher fallback", "in_review", "user", "approve", "", false, false, true},
		{"explicit other receiver", "in_review", "user", "approve", other, false, false, false},
		{"agent comment", "in_review", "user", "approve", testUserID, true, false, false},
		{"specialist", "in_review", "specialist", "approve", "", false, false, false},
		{"unmarked", "in_review", "", "", "", false, false, false},
		{"already working", "in_progress", "", "", "", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := map[string]any{"keep": "value"}
			if tc.by != "" {
				metadata["review_by"] = tc.by
				metadata["review_kind"] = tc.kind
			}
			if tc.receiver != "" {
				metadata["review_user"] = tc.receiver
			}
			rawMetadata, _ := json.Marshal(metadata)
			id := dbfx.Issue(t, tc.name, testutil.Cols{
				"status": tc.status, "assignee_type": "agent", "assignee_id": agent,
				"metadata": rawMetadata,
			})
			before, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
			body := map[string]any{"content": "Please continue"}
			if tc.reply {
				body["parent_id"] = dbfx.Comment(t, id, "Question", testutil.Cols{"author_type": "agent", "author_id": agent})
			}
			h := *testHandler
			h.Bus = events.New()
			var updates int
			h.Bus.Subscribe(protocol.EventIssueUpdated, func(e events.Event) {
				updates++
				payload := e.Payload.(map[string]any)
				if payload["status_changed"] != true || payload["prev_status"] != tc.status {
					t.Errorf("status activity payload = %#v", payload)
				}
			})
			req := withURLParam(newRequest(http.MethodPost, "/api/issues/"+id+"/comments", body), "id", id)
			if tc.agentAuthor {
				req.Header.Set("X-Agent-ID", agent)
				req.Header.Set("X-Actor-Source", "task_token")
			}
			var response CommentResponse
			testutil.Call(t, h.CreateComment, req).Want(http.StatusCreated).JSON(&response)
			current, metadata := reviewMetadataOf(t, id)
			wantStatus, wantUpdates := tc.status, 0
			wantRevision := before.Revision + 1
			if tc.wantReturn {
				wantStatus, wantUpdates = "in_progress", 1
				wantRevision++
				if metadata["review_by"] != nil || metadata["keep"] != "value" {
					t.Fatalf("review cleanup = %#v", metadata)
				}
				if response.IssueStatus != "in_progress" {
					t.Fatalf("receipt status = %q", response.IssueStatus)
				}
			} else if response.IssueStatus != "" {
				t.Fatalf("unexpected handback receipt: %+v", response)
			}
			if current.Status != wantStatus || updates != wantUpdates || response.IssueRevision != wantRevision || current.Revision != wantRevision {
				t.Fatalf("status=%s updates=%d receipt revision=%d stored=%d; want %s/%d/%d", current.Status, updates, response.IssueRevision, current.Revision, wantStatus, wantUpdates, wantRevision)
			}
			if tc.wantReturn {
				if got := countQueuedCommentTriggerTasks(t, id, agent); got != 1 {
					t.Fatalf("queued %d runs, want exactly one", got)
				}
				var trigger string
				dbfx.Pool.QueryRow(context.Background(), `SELECT trigger_comment_id::text FROM agent_task_queue WHERE issue_id=$1`, id).Scan(&trigger)
				if trigger != response.ID {
					t.Fatalf("run must come from the comment, trigger=%s", trigger)
				}
			}
		})
	}
}

func TestCreateCommentHandbackRollsBackOnStatusFailure(t *testing.T) {
	agent := createHandlerTestAgent(t, "Rollback assignee", nil)
	id := dbfx.Issue(t, "Atomic handback", testutil.Cols{
		"status": "in_review", "assignee_type": "agent", "assignee_id": agent,
		"metadata": `{"review_by":"user","review_kind":"approve"}`,
	})
	before, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
	dbfx.Exec(t, fmt.Sprintf(`CREATE FUNCTION test_handback_failure() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.id = '%s'::uuid AND NEW.status = 'in_progress' THEN RAISE EXCEPTION 'injected handback failure'; END IF; RETURN NEW; END $$`, id))
	dbfx.Cleanup(t, `DROP FUNCTION test_handback_failure() CASCADE`)
	dbfx.Exec(t, `CREATE TRIGGER test_handback_failure BEFORE UPDATE ON issue FOR EACH ROW EXECUTE FUNCTION test_handback_failure()`)
	req := withURLParam(newRequest(http.MethodPost, "/api/issues/"+id+"/comments", map[string]any{"content": "continue"}), "id", id)
	testutil.Call(t, testHandler.CreateComment, req).Want(http.StatusInternalServerError)
	current, metadata := reviewMetadataOf(t, id)
	var comments int
	dbfx.Pool.QueryRow(context.Background(), `SELECT count(*) FROM comment WHERE issue_id=$1`, id).Scan(&comments)
	if current.Status != before.Status || current.Revision != before.Revision || comments != 0 || metadata["review_by"] != "user" {
		t.Fatalf("failed transaction persisted: status=%s revision=%d comments=%d metadata=%v", current.Status, current.Revision, comments, metadata)
	}
	if got := countQueuedCommentTriggerTasks(t, id, agent); got != 0 {
		t.Fatalf("failed comment queued %d runs", got)
	}
}
