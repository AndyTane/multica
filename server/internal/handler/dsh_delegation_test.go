package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestDSHDelegationReturnsToCaller(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	for _, provider := range []string{"claude", "codex"} {
		for _, terminal := range []string{"completed", "failed", "cancelled"} {
			t.Run(provider+"/"+terminal, func(t *testing.T) {
				ctx := context.Background()
				callerRuntime := dbfx.Runtime(t, provider, testutil.Cols{"provider": provider})
				caller := dbfx.Agent(t, "Delegating "+provider, callerRuntime)
				dshRuntime := dbfx.Runtime(t, "DSH Web", testutil.Cols{"provider": "dsh"})
				dsh := dbfx.Agent(t, "DSH", dshRuntime)
				issue := dbfx.Issue(t, "Delegate through DSH", testutil.Cols{"status": "in_progress", "assignee_type": "agent", "assignee_id": caller})
				root := dbfx.Comment(t, issue, "Please investigate this issue")
				source := dbfx.Task(t, caller, testutil.Cols{
					"runtime_id": callerRuntime, "issue_id": issue, "status": "running", "started_at": testutil.Raw("now()"),
					"trigger_comment_id": root, "originator_user_id": testUserID, "accountable_user_id": testUserID,
				})
				dbfx.Cleanup(t, "DELETE FROM agent_task_queue WHERE issue_id=$1", issue)
				dbfx.Cleanup(t, "DELETE FROM issue_wakeup WHERE issue_id=$1", issue)
				dbfx.Cleanup(t, "DELETE FROM issue_wakeup_receipt WHERE wakeup_id IN (SELECT id FROM issue_wakeup WHERE issue_id=$1)", issue)
				var delegation CommentResponse
				req := withURLParam(newRequest(http.MethodPost, "/", map[string]any{
					"content": fmt.Sprintf("[@DSH](mention://agent/%s) collect the evidence", dsh), "parent_id": root,
				}), "id", issue)
				req.Header.Set("X-Agent-ID", caller)
				req.Header.Set("X-Task-ID", source)
				testutil.Call(t, testHandler.CreateComment, req).Want(http.StatusCreated).JSON(&delegation)
				// Ending the caller must not remove the separately queued worker.
				dbfx.Exec(t, "UPDATE agent_task_queue SET status='completed',completed_at=now() WHERE id=$1", source)
				worker := claimWorkerReplyRun(t, dshRuntime)
				if worker == nil || worker.TriggerCommentID == nil || *worker.TriggerCommentID != delegation.ID {
					t.Fatalf("DSH did not receive this delegation: %+v", worker)
				}
				var wakeID, wakeAgent, wakeSource, wakeParent string
				dbfx.QueryRow(t, `SELECT id,agent_id,source_task_id,parent_comment_id FROM issue_wakeup
					WHERE issue_id=$1 AND filter_task_id=$2`, issue, worker.ID).Scan(&wakeID, &wakeAgent, &wakeSource, &wakeParent)
				if wakeAgent != caller || wakeSource != source || wakeParent != delegation.ID {
					t.Fatalf("return destination = %s/%s/%s, want caller/source/delegation %s/%s/%s", wakeAgent, wakeSource, wakeParent, caller, source, delegation.ID)
				}
				if _, err := testHandler.TaskService.StartTask(ctx, parseUUID(worker.ID)); err != nil {
					t.Fatal(err)
				}
				progress := withURLParam(newRequest(http.MethodPost, "/", map[string]any{
					"content": "Collecting evidence; final result will follow", "parent_id": delegation.ID,
				}), "id", issue)
				progress.Header.Set("X-Agent-ID", dsh)
				progress.Header.Set("X-Task-ID", worker.ID)
				testutil.Call(t, testHandler.CreateComment, progress).Want(http.StatusCreated)
				wakeups := &service.IssueWakeupService{Tasks: testHandler.TaskService}
				if err := wakeups.TickWorkspaces(ctx, parseUUID(testWorkspaceID)); err != nil {
					t.Fatal(err)
				}
				if got := dbfx.Count(t, "SELECT count(*) FROM agent_task_queue WHERE issue_id=$1 AND agent_id=$2 AND status='queued'", issue, caller); got != 0 {
					t.Fatal("the running worker woke its caller before completion")
				}
				if terminal == "completed" {
					completeWorkerReplyRun(t, worker.ID)
				} else if terminal == "failed" {
					if _, err := testHandler.TaskService.FailTask(ctx, parseUUID(worker.ID), "DSH could not finish", "", "", "", "invalid_task_identity", false, "", ""); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := testHandler.TaskService.CancelTask(ctx, parseUUID(worker.ID)); err != nil {
						t.Fatal(err)
					}
				}
				for range 2 {
					if err := wakeups.TickWorkspaces(ctx, parseUUID(testWorkspaceID)); err != nil {
						t.Fatal(err)
					}
				}
				if got := dbfx.Count(t, "SELECT count(*) FROM agent_task_queue WHERE issue_id=$1 AND agent_id=$2 AND status='queued'", issue, caller); got != 1 {
					t.Fatalf("terminal delivery queued %d caller runs, want one", got)
				}
				if got := dbfx.Count(t, "SELECT count(*) FROM comment WHERE source_task_id=$1 AND author_type='system' AND type='progress_update'", worker.ID); got != 0 {
					t.Fatal("DSH return also created a generic delegated-failure recovery")
				}
				returned := claimWorkerReplyRun(t, callerRuntime)
				if returned == nil || returned.TriggerCommentID == nil || *returned.TriggerCommentID != delegation.ID || !strings.Contains(returned.HandoffNote, worker.ID) {
					t.Fatalf("return lost its original thread or worker result: %+v", returned)
				}
				stored, err := testHandler.Queries.GetAgentTask(ctx, parseUUID(returned.ID))
				if err != nil || stored.DelegatedFromTaskID != parseUUID(source) {
					t.Fatalf("return lost caller provenance: source=%s, error=%v", uuidToString(stored.DelegatedFromTaskID), err)
				}
				thread, err := testHandler.Queries.GetThreadRoot(ctx, db.GetThreadRootParams{CommentID: stored.TriggerCommentID, WorkspaceID: parseUUID(testWorkspaceID)})
				if err != nil || thread.ID != parseUUID(root) {
					t.Fatalf("return moved to another comment thread: root=%s, error=%v", uuidToString(thread.ID), err)
				}
			})
		}
	}
}
