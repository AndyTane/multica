package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Exercise the actual enqueue path, not just a hand-written attribution fixture.
func TestIssueWakeupDispatcherSourceSnapshot(t *testing.T) {
	for _, source := range []string{"direct_human", "trigger_owner", "none", "timer"} {
		t.Run(source, func(t *testing.T) {
			f, s, issue, agent := wakeFixture(t)
			ctx := context.Background()
			var sourceID pgtype.UUID
			if source != "none" && source != "timer" {
				sourceID = parseTestUUID(t, f.Task(t, agent, testutil.Cols{
					"runtime_id": testutil.Raw("(SELECT runtime_id FROM agent WHERE id='" + agent + "')"), "issue_id": issue, "status": "completed", "originator_user_id": f.UserID, "originator_source": source, "accountable_user_id": f.UserID,
				}))
			}
			input := WakeupInput{AgentID: agent, Kind: "event", EventTypes: []string{"comment.created"}, Instruction: "Continue the requested work"}
			if source == "timer" {
				at := time.Now().Add(time.Minute)
				input.Kind, input.At, input.EventTypes = "at", &at, nil
			}
			w, err := s.Create(ctx, issue, parseTestUUID(t, f.UserID), sourceID, input)
			if err != nil {
				t.Fatal(err)
			}
			if source == "timer" {
				f.Exec(t, "UPDATE issue_wakeup SET next_fire_at=now()-interval '1 second' WHERE id=$1", w.ID)
			} else {
				f.Comment(t, util.UUIDToString(issue), "New input after wait")
			}
			wakeDispatch(t, s, w)
			got, err := f.q.GetIssueWakeup(ctx, db.GetIssueWakeupParams{ID: w.ID, WorkspaceID: w.WorkspaceID})
			if err != nil {
				t.Fatal(err)
			}
			task, err := f.q.GetAgentTask(ctx, got.LastTaskID)
			if err != nil {
				t.Fatal(err)
			}
			if task.OriginatorSource.String != "trigger_owner" || task.TriggerEvidenceKind.String != "issue_wakeup" || task.TriggerEvidenceRefID != w.ID || task.DelegatedFromTaskID != sourceID || task.OriginatorUserID != parseTestUUID(t, f.UserID) {
				t.Fatalf("unexpected enqueue provenance: %+v", task)
			}
			card := f.Issue(t, "Card after actual wakeup", testutil.Cols{
				"creator_type": "agent", "creator_id": agent, "origin_type": "agent_create", "origin_id": task.ID,
			})
			// Even deletion of the current rule must not rewrite the run's historical meaning.
			f.Exec(t, "DELETE FROM issue_wakeup_receipt WHERE wakeup_id=$1", w.ID)
			f.Exec(t, "DELETE FROM issue_wakeup WHERE id=$1", w.ID)
			rows, err := f.q.ListIssueDispatchers(ctx, db.ListIssueDispatchersParams{WorkspaceID: parseTestUUID(t, f.WorkspaceID), Ids: []pgtype.UUID{parseTestUUID(t, card)}})
			if err != nil || len(rows) != 1 {
				t.Fatalf("dispatcher rows: %v %v", rows, err)
			}
			want := ""
			if source == "direct_human" {
				want = f.UserID
			}
			if util.UUIDToString(rows[0].DispatcherUserID) != want || rows[0].DispatcherActive != (want != "") {
				t.Fatalf("dispatcher=%+v want=%s", rows[0], want)
			}
		})
	}
}
