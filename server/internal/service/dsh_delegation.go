package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/eventcontract"
)

// PrepareDSHDelegationReturns runs in the claim-delivery transaction after the
// runtime's provider and invocation authority have been checked. Only inputs
// actually delivered to this run can create a return. The returned callback
// publishes the durable activity after that transaction commits successfully.
func (s *IssueWakeupService) PrepareDSHDelegationReturns(ctx context.Context, q *db.Queries, worker db.AgentTaskQueue, delivered []pgtype.UUID) (func(), error) {
	if !worker.IssueID.Valid || len(delivered) == 0 || worker.IsLeaderTask {
		return nil, nil
	}
	issue, err := q.LockWakeupIssue(ctx, worker.IssueID)
	if err != nil {
		return nil, err
	}
	active, err := wakeupIssueActive(ctx, q, issue)
	if err != nil || !active {
		return nil, err
	}
	var activities []wakeupActivity
	for _, commentID := range delivered {
		comment, err := q.GetCommentInWorkspace(ctx, db.GetCommentInWorkspaceParams{ID: commentID, WorkspaceID: issue.WorkspaceID})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if comment.IssueID != issue.ID || comment.DeletedAt.Valid || comment.AuthorType != "agent" || !comment.SourceTaskID.Valid || comment.AuthorID == worker.AgentID {
			continue
		}
		if fields := strings.Fields(comment.Content); len(fields) > 0 && strings.EqualFold(fields[0], "/note") {
			continue
		}
		mentioned := false
		for _, mention := range util.ParseMentions(comment.Content) {
			if mention.Type == "agent" && strings.EqualFold(mention.ID, util.UUIDToString(worker.AgentID)) {
				mentioned = true
				break
			}
		}
		if !mentioned {
			continue
		}
		source, err := q.GetAgentTask(ctx, comment.SourceTaskID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		// Squad leaders already have their worker-result return path. Direct
		// callers need a precise same-issue speaking identity, regardless of CLI.
		if source.AgentID != comment.AuthorID || source.IssueID != issue.ID || source.IsLeaderTask {
			continue
		}
		caller, err := q.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: source.AgentID, WorkspaceID: issue.WorkspaceID})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		member := source.OriginatorUserID
		if !member.Valid {
			member = source.AccountableUserID
		}
		if err := s.authorize(ctx, q, issue.WorkspaceID, member, caller); err != nil {
			if errors.Is(err, ErrWakeupForbidden) {
				continue
			}
			return nil, err
		}
		// Keep the exact delegation as the reply anchor. Its identity survives
		// edits to ancestors and distinguishes coalesced requests in one thread.
		parent := comment.ID
		_, err = q.GetDSHDelegationReturn(ctx, db.GetDSHDelegationReturnParams{
			WorkspaceID: issue.WorkspaceID, IssueID: issue.ID, WorkerTaskID: worker.ID,
			SourceTaskID: source.ID, ParentCommentID: parent,
		})
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		instruction := fmt.Sprintf("Inspect the terminal outcome and saved result of DSH run %s, delegated by your run %s in comment %s. Read the current issue and original thread, verify the work, and continue your coordination. A completed run is a candidate result, not proof of acceptance. If it failed or was cancelled, inspect the reason and report the next step; do not resend the DSH prompt automatically.",
			util.UUIDToString(worker.ID), util.UUIDToString(source.ID), util.UUIDToString(comment.ID))
		rule, err := q.CreateIssueWakeup(ctx, db.CreateIssueWakeupParams{
			ID: dbid.NewV7(), WorkspaceID: issue.WorkspaceID, IssueID: issue.ID,
			AgentID: caller.ID, CreatedBy: member, SourceTaskID: source.ID, ParentCommentID: parent,
			Instruction: instruction, Kind: "event", Mode: "once", Timezone: "UTC",
			EventTypes:   []string{eventcontract.EventTaskCompleted, eventcontract.EventTaskFailed, eventcontract.EventTaskCancelled},
			FilterTaskID: worker.ID,
		})
		if err != nil {
			return nil, err
		}
		activity, err := recordWakeupActivity(ctx, q, rule, wakeupActivityCreated, "agent", caller.ID, nil)
		if err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}
	return func() { s.publishWakeupActivities(activities...) }, nil
}
