package handler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// CLAWSTA-151 B9/B10 resolve "who is this card waiting on" entirely in SQL over
// the issue_dispatcher view (migrations/586 and 589) and the reserved review metadata,
// so the list filter, the projection and the page's awaiting_problem reasons
// cannot drift apart.

// issueDispatcherUserExpr is the dispatcher member id for one issue row, or
// NULL when the card has no trusted dispatcher.
func issueDispatcherUserExpr(alias string) string {
	return fmt.Sprintf("(SELECT d.dispatcher_user_id FROM issue_dispatcher d WHERE d.id = %[1]s.id)", alias)
}

// issueDispatcherActiveExpr reports whether that dispatcher is still a member.
func issueDispatcherActiveExpr(alias string) string {
	return fmt.Sprintf("COALESCE((SELECT d.dispatcher_active FROM issue_dispatcher d WHERE d.id = %[1]s.id), FALSE)", alias)
}

// issueReviewMarkValidExpr is the B7 combination check for the row's CURRENT
// status: in_review publishes only approve (kind optional), blocked publishes
// only user+reply/act, and blocked+specialist is never valid.
func issueReviewMarkValidExpr(alias string) string {
	return fmt.Sprintf(`CASE
        WHEN %[1]s.metadata->>'review_by' = 'user' THEN
            CASE
                WHEN %[1]s.status = 'in_review' THEN COALESCE(%[1]s.metadata->>'review_kind', 'approve') = 'approve'
                WHEN %[1]s.status = 'blocked' THEN %[1]s.metadata->>'review_kind' IN ('reply', 'act')
                ELSE FALSE
            END
        WHEN %[1]s.metadata->>'review_by' = 'specialist' THEN
            %[1]s.status = 'in_review' AND COALESCE(%[1]s.metadata->>'review_kind', 'approve') = 'approve'
        ELSE FALSE
    END`, alias)
}

// issueReviewReceiverExpr is the member the current mark actually waits on: an
// explicit review_user that resolves to a workspace member, otherwise the
// active dispatcher (B9 fills the dispatcher when review_user is omitted). A
// mark with neither is undeliverable and yields NULL.
func issueReviewReceiverExpr(alias string) string {
	return fmt.Sprintf(`CASE
        WHEN %[1]s.metadata ? 'review_user' THEN
            (SELECT m.user_id FROM member m
              WHERE m.workspace_id = %[1]s.workspace_id
                AND m.user_id::text = %[1]s.metadata->>'review_user')
        ELSE
            (SELECT d.dispatcher_user_id FROM issue_dispatcher d
              WHERE d.id = %[1]s.id AND d.dispatcher_active)
    END`, alias)
}

// awaitingMinePredicate selects the cards genuinely waiting on meRef (B10):
// two review statuses, a live dispatcher, a published user mark whose combined
// object is legal, and an effective receiver that is me.
func awaitingMinePredicate(alias, meRef string) string {
	return fmt.Sprintf(`(%[1]s.status IN ('in_review', 'blocked')
        AND %[1]s.metadata->>'review_by' = 'user'
        AND %[3]s
        AND %[2]s
        AND %[4]s = %[5]s)`,
		alias, issueDispatcherActiveExpr(alias), issueReviewMarkValidExpr(alias),
		issueReviewReceiverExpr(alias), meRef)
}

// awaitingUnmarkedPredicate selects the cards nobody can be asked about (B10):
// the dispatcher's own unmarked/undeliverable cards, plus — for an owner/admin
// viewer — every anomaly whose mark cannot reach a live member (dispatcher or
// receiver gone, no trusted dispatcher left). A valid specialist mark and a
// execution card without trusted member lineage (no dispatcher, no user mark) are never
// counted. The two clauses cannot double count: it is one predicate.
func awaitingUnmarkedPredicate(alias, meRef string) string {
	dispatcher := issueDispatcherUserExpr(alias)
	active := issueDispatcherActiveExpr(alias)
	// COALESCE the three-valued combination check: an incomplete mark (e.g.
	// blocked + review_by=user with no review_kind) makes the CASE yield NULL,
	// and a bare NOT(NULL) would turn the whole predicate NULL instead of
	// "undeliverable". Such a card must surface in the dispatcher's unmarked
	// window with the invalid_review_mark reason the projection reports.
	valid := fmt.Sprintf("COALESCE((%s), FALSE)", issueReviewMarkValidExpr(alias))
	receiver := issueReviewReceiverExpr(alias)
	undeliverable := fmt.Sprintf("NOT (%s) OR (%s) IS NULL OR NOT (%s)", active, receiver, valid)
	return fmt.Sprintf(`(%[1]s.status IN ('in_review', 'blocked') AND (
        ((%[2]s) = %[4]s AND (
            %[1]s.metadata->>'review_by' IS NULL
            OR (%[1]s.metadata->>'review_by' = 'user' AND (%[5]s))
        ))
        OR (
            %[1]s.metadata->>'review_by' = 'user'
            AND (%[5]s)
        )
    ))`,
		alias, dispatcher, active, meRef, undeliverable)
}

// awaitingProblemCodes names why an unmarked card cannot be delivered. The
// values are stable machine codes; the awaiting page localizes them.
const (
	awaitingProblemNoDispatcher       = "no_dispatcher"
	awaitingProblemDispatcherInactive = "dispatcher_inactive"
	awaitingProblemReceiverInactive   = "receiver_inactive"
	awaitingProblemInvalidMark        = "invalid_review_mark"
)

// loadAwaitingProblems computes the optional awaiting_problem code for unmarked
// rows. Deliberately best-effort: a lookup failure leaves the field out rather
// than mislabeling the card.
func (h *Handler) loadAwaitingProblems(ctx context.Context, workspaceID pgtype.UUID, issues []db.Issue) map[string]string {
	if len(issues) == 0 {
		return nil
	}
	ids := make([]pgtype.UUID, len(issues))
	for i, issue := range issues {
		ids[i] = issue.ID
	}
	dispatchers, err := h.Queries.ListIssueDispatchers(ctx, db.ListIssueDispatchersParams{
		WorkspaceID: workspaceID,
		Ids:         ids,
	})
	if err != nil {
		slog.Warn("resolve awaiting problems failed", "workspace_id", uuidToString(workspaceID), "error", err)
		return nil
	}
	dispatcherByID := make(map[string]db.ListIssueDispatchersRow, len(dispatchers))
	for _, row := range dispatchers {
		dispatcherByID[uuidToString(row.ID)] = row
	}

	// Batch-resolve which explicit review_user members are still live.
	explicitReceivers := make([]pgtype.UUID, 0, len(issues))
	explicitReceiverOf := make(map[string]pgtype.UUID, len(issues))
	for _, issue := range issues {
		raw, parseErr := reviewRawMetadata(issue.Metadata)
		if parseErr != nil {
			continue
		}
		if receiver := reviewUUIDValue(raw, "review_user"); receiver.Valid {
			explicitReceiverOf[uuidToString(issue.ID)] = receiver
			explicitReceivers = append(explicitReceivers, receiver)
		}
	}
	liveMembers := map[string]struct{}{}
	if len(explicitReceivers) > 0 {
		rows, queryErr := h.DB.Query(ctx, `SELECT user_id::text FROM member WHERE workspace_id = $1 AND user_id = ANY($2::uuid[])`, workspaceID, explicitReceivers)
		if queryErr == nil {
			for rows.Next() {
				var userID string
				if scanErr := rows.Scan(&userID); scanErr == nil {
					liveMembers[userID] = struct{}{}
				}
			}
			rows.Close()
		}
	}

	problems := map[string]string{}
	for _, issue := range issues {
		key := uuidToString(issue.ID)
		row := dispatcherByID[key]
		raw, parseErr := reviewRawMetadata(issue.Metadata)
		if parseErr != nil {
			continue
		}
		mark := reviewMarkFromEntries(reviewEntries(raw))
		receiverValid := false
		if mark.present && mark.by == reviewByUser {
			if explicit, ok := explicitReceiverOf[key]; ok {
				_, receiverValid = liveMembers[uuidToString(explicit)]
			} else {
				receiverValid = row.DispatcherUserID.Valid && row.DispatcherActive
			}
		}
		switch {
		case !row.DispatcherUserID.Valid:
			problems[key] = awaitingProblemNoDispatcher
		case !row.DispatcherActive:
			problems[key] = awaitingProblemDispatcherInactive
		case mark.present && !reviewMarkCombinationValid(issue.Status, mark):
			problems[key] = awaitingProblemInvalidMark
		case mark.present && mark.by == reviewByUser && !receiverValid:
			problems[key] = awaitingProblemReceiverInactive
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return problems
}

// reviewMarkCombinationValid mirrors issueReviewMarkValidExpr in Go for the
// rows already loaded; keep the two in step (both are exercised by tests).
func reviewMarkCombinationValid(status string, mark reviewMark) bool {
	switch mark.by {
	case reviewByUser:
		switch status {
		case reviewStatusInReview:
			return mark.kind == "" || mark.kind == reviewKindApprove
		case reviewStatusBlocked:
			return mark.kind == reviewKindReply || mark.kind == reviewKindAct
		}
		return false
	case reviewBySpecialist:
		if status != reviewStatusInReview {
			return false
		}
		return mark.kind == "" || mark.kind == reviewKindApprove
	}
	return false
}

// parseAwaitingFilter validates the awaiting query parameter.
func parseAwaitingFilter(raw string) (string, bool) {
	switch raw {
	case "":
		return "", true
	case "mine", "unmarked":
		return raw, true
	default:
		return "", false
	}
}

// parseOptionalUserUUID parses an optional UUID query parameter.
func parseOptionalUserUUID(raw, field string) (pgtype.UUID, bool, bool) {
	if raw == "" {
		return pgtype.UUID{}, false, true
	}
	id, err := util.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, false, false
	}
	return id, true, true
}
