package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type dshReturnFixture struct {
	principalFixture
	wake    *IssueWakeupService
	issue   pgtype.UUID
	caller  string
	source  string
	root    string
	comment string
	worker  db.AgentTaskQueue
}

func newDSHReturnFixture(t *testing.T) dshReturnFixture {
	t.Helper()
	f, wake, issue, caller := wakeFixture(t)
	workerRuntime := f.Runtime(t, "DSH return runtime", testutil.Cols{"provider": "dsh"})
	workerAgent := f.Agent(t, "DSH return worker", workerRuntime)
	root := f.Comment(t, util.UUIDToString(issue), "Original coordination thread")
	source := f.Task(t, caller, testutil.Cols{
		"issue_id": issue, "status": "completed", "originator_user_id": f.UserID, "accountable_user_id": f.UserID,
		"completed_at": testutil.Raw("now()"),
	})
	comment := f.Comment(t, util.UUIDToString(issue), fmt.Sprintf("[@DSH](mention://agent/%s) collect evidence", workerAgent), testutil.Cols{
		"parent_id": root, "author_type": "agent", "author_id": caller, "source_task_id": source,
	})
	workerID := f.Task(t, workerAgent, testutil.Cols{
		"issue_id": issue, "status": "dispatched", "trigger_comment_id": comment,
		"runtime_id":             workerRuntime,
		"dispatched_at":          testutil.Raw("now()"),
		"delegated_from_task_id": source, "originator_user_id": f.UserID, "accountable_user_id": f.UserID,
	})
	worker, err := f.q.GetAgentTask(context.Background(), parseTestUUID(t, workerID))
	if err != nil {
		t.Fatal(err)
	}
	return dshReturnFixture{f, wake, issue, caller, source, root, comment, worker}
}

func TestDSHDelegationReturnFinalizationFailureRollsBackRuleAndCredential(t *testing.T) {
	f := newDSHReturnFixture(t)
	ctx := context.Background()
	// The return is prepared successfully, then a receipt outside this run's
	// delivered input plan makes finalization fail after credential insertion.
	_, err := f.wake.Tasks.FinalizeTaskClaim(ctx, f.worker, db.CreateTaskTokenParams{
		TokenHash: "dsh-return-rollback-" + util.UUIDToString(f.worker.ID),
		TaskID:    f.worker.ID, AgentID: f.worker.AgentID, WorkspaceID: parseTestUUID(t, f.WorkspaceID),
		UserID: parseTestUUID(t, f.UserID), ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}, []pgtype.UUID{parseTestUUID(t, f.root)}, true, func(q *db.Queries, _ *db.CreateTaskTokenParams) error {
		_, err := f.wake.PrepareDSHDelegationReturns(ctx, q, f.worker, []pgtype.UUID{parseTestUUID(t, f.comment)})
		return err
	}, nil)
	if err == nil {
		t.Fatal("an invalid claim receipt was accepted")
	}
	if got := f.Count(t, "SELECT count(*) FROM issue_wakeup WHERE issue_id=$1", f.issue); got != 0 {
		t.Fatal("failed delivery committed the return rule")
	}
	if got := f.Count(t, "SELECT count(*) FROM task_token WHERE task_id=$1", f.worker.ID); got != 0 {
		t.Fatal("failed delivery committed its task credential")
	}
}

func prepareDSHReturn(t *testing.T, f dshReturnFixture, commit bool, comments ...string) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	ids := make([]pgtype.UUID, len(comments))
	for i, comment := range comments {
		ids[i] = parseTestUUID(t, comment)
	}
	publish, err := f.wake.PrepareDSHDelegationReturns(ctx, f.q.WithTx(tx), f.worker, ids)
	if err != nil {
		t.Fatal(err)
	}
	if commit {
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if publish != nil {
			publish()
		}
	}
}

func TestDSHDelegationReturnClaimRollbackAndReclaim(t *testing.T) {
	f := newDSHReturnFixture(t)
	prepareDSHReturn(t, f, false, f.comment)
	if got := f.Count(t, "SELECT count(*) FROM issue_wakeup WHERE issue_id=$1", f.issue); got != 0 {
		t.Fatal("a rolled-back claim left a return behind")
	}
	prepareDSHReturn(t, f, true, f.comment)
	f.Exec(t, "UPDATE issue_wakeup SET enabled=false,disabled_at=now() WHERE issue_id=$1", f.issue)
	prepareDSHReturn(t, f, true, f.comment)
	if got := f.Count(t, "SELECT count(*) FROM issue_wakeup WHERE issue_id=$1", f.issue); got != 1 {
		t.Fatalf("reclaim created %d rules, want one", got)
	}
	if got := f.Count(t, "SELECT count(*) FROM issue_wakeup WHERE issue_id=$1 AND enabled", f.issue); got != 0 {
		t.Fatal("reclaim rearmed a disabled return")
	}
	if got := f.Count(t, "SELECT count(*) FROM activity_log WHERE issue_id=$1 AND action='wakeup_created'", f.issue); got != 1 {
		t.Fatal("rollback or reclaim duplicated return activity")
	}
}

func TestDSHDelegationReturnKeepsCoalescedCallersSeparate(t *testing.T) {
	f := newDSHReturnFixture(t)
	otherHuman := f.member(t, "other-dsh-caller")
	other := f.privateAgentOwnedBy(t, otherHuman, "other-caller")
	f.Exec(t, "UPDATE agent SET visibility='workspace',permission_mode='public_to' WHERE id=$1", f.worker.AgentID)
	f.Exec(t, "INSERT INTO agent_invocation_target (agent_id,target_type,target_id) VALUES ($1,'workspace',$2)", f.worker.AgentID, f.WorkspaceID)
	f.Exec(t, "UPDATE agent_runtime SET visibility='public' WHERE id=$1", f.worker.RuntimeID)
	otherRoot := f.Comment(t, util.UUIDToString(f.issue), "Other coordination thread")
	otherSource := f.Task(t, other, testutil.Cols{
		"issue_id": f.issue, "status": "completed", "originator_user_id": otherHuman, "accountable_user_id": otherHuman,
		"completed_at": testutil.Raw("now()"),
	})
	otherComment := f.Comment(t, util.UUIDToString(f.issue), fmt.Sprintf("[@DSH](mention://agent/%s) include this evidence", util.UUIDToString(f.worker.AgentID)), testutil.Cols{
		"parent_id": otherRoot, "author_type": "agent", "author_id": other, "source_task_id": otherSource,
	})
	prepareDSHReturn(t, f, true, f.comment, otherComment)
	for _, want := range []struct{ caller, source, parent, human string }{{f.caller, f.source, f.comment, f.UserID}, {other, otherSource, otherComment, otherHuman}} {
		if got := f.Count(t, `SELECT count(*) FROM issue_wakeup WHERE filter_task_id=$1 AND agent_id=$2 AND source_task_id=$3 AND parent_comment_id=$4 AND created_by=$5`, f.worker.ID, want.caller, want.source, want.parent, want.human); got != 1 {
			t.Fatalf("missing caller-specific return for %+v", want)
		}
	}
}

func TestDSHDelegationReturnRejectsUnprovenInputs(t *testing.T) {
	for _, mode := range []string{"not_delivered", "member", "not_mentioned", "note", "foreign_source_issue", "wrong_source_agent", "squad_leader", "archived_caller", "revoked_invocation", "closed_issue", "deleted_comment"} {
		t.Run(mode, func(t *testing.T) {
			f := newDSHReturnFixture(t)
			comments := []string{f.comment}
			switch mode {
			case "not_delivered":
				comments = nil
			case "member":
				f.Exec(t, "UPDATE comment SET author_type='member',author_id=$2 WHERE id=$1", f.comment, f.UserID)
			case "not_mentioned":
				f.Exec(t, "UPDATE comment SET content='An ordinary progress update' WHERE id=$1", f.comment)
			case "note":
				f.Exec(t, "UPDATE comment SET content='/note '||content WHERE id=$1", f.comment)
			case "foreign_source_issue":
				other := f.Issue(t, "Unrelated source issue")
				f.Exec(t, "UPDATE agent_task_queue SET issue_id=$2 WHERE id=$1", f.source, other)
			case "wrong_source_agent":
				f.Exec(t, "UPDATE agent_task_queue SET agent_id=$2 WHERE id=$1", f.source, f.worker.AgentID)
			case "squad_leader":
				f.Exec(t, "UPDATE agent_task_queue SET is_leader_task=true WHERE id=$1", f.source)
			case "archived_caller":
				f.Exec(t, "UPDATE agent SET archived_at=now() WHERE id=$1", f.caller)
			case "revoked_invocation":
				other := f.member(t, "revoked-dsh-caller")
				f.Exec(t, "UPDATE agent_task_queue SET originator_user_id=$2,accountable_user_id=$2 WHERE id=$1", f.source, other)
			case "closed_issue":
				wakeSetStatus(t, f.principalFixture, f.issue, "done")
			case "deleted_comment":
				f.Exec(t, "UPDATE comment SET deleted_at=now() WHERE id=$1", f.comment)
			}
			prepareDSHReturn(t, f, true, comments...)
			if got := f.Count(t, "SELECT count(*) FROM issue_wakeup WHERE issue_id=$1", f.issue); got != 0 {
				t.Fatalf("%s created an unproven return", mode)
			}
		})
	}
}
