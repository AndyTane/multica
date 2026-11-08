package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The reserved review metadata keys describe ONE review round. R4 grew the set
// to five: review_kind distinguishes "waiting for approval" (in_review) from
// "waiting for a reply" / "waiting for an action outside the system" (blocked).
//
// They are only ever written together through resolveReviewMetadata (or the
// single-key preparation path), and the maintain_issue_review trigger removes
// all five whenever the issue leaves either review status, so a round can never
// inherit a stale mark from the previous one (CLAWSTA-151 B7/B8).
var reviewKeys = []string{"review_by", "review_action", "review_user", "review_agent", "review_kind"}

var errReviewClosed = errors.New("review metadata requires status in_review or blocked")

const (
	reviewStatusInReview = "in_review"
	reviewStatusBlocked  = "blocked"

	reviewByUser       = "user"
	reviewBySpecialist = "specialist"

	reviewKindApprove = "approve"
	reviewKindReply   = "reply"
	reviewKindAct     = "act"
)

type reviewValidationError struct{ message string }

func (e *reviewValidationError) Error() string { return e.message }

func isReviewStatus(status string) bool {
	return status == reviewStatusInReview || status == reviewStatusBlocked
}

func isReviewKey(key string) bool {
	for _, k := range reviewKeys {
		if k == key {
			return true
		}
	}
	return false
}

// Validate one key's value on the same transaction that holds the issue row
// lock. Combination rules live in validateReviewCombination — a value that is
// legal on its own can still be illegal next to the rest of the object.
func validateReviewValue(ctx context.Context, q *db.Queries, ws pgtype.UUID, key string, raw json.RawMessage) (json.RawMessage, error) {
	fail := func(message string) (json.RawMessage, error) { return nil, &reviewValidationError{key + " " + message} }
	var value string
	if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return fail("must be a string")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fail("must not be blank")
	}
	switch key {
	case "review_by":
		if value != reviewByUser && value != reviewBySpecialist {
			return fail("must be user or specialist")
		}
	case "review_action":
		if utf8.RuneCountInString(value) > 120 {
			return fail("must contain at most 120 Unicode code points")
		}
	case "review_kind":
		if value != reviewKindApprove && value != reviewKindReply && value != reviewKindAct {
			return fail("must be approve, reply or act")
		}
	case "review_user", "review_agent":
		id, err := util.ParseUUID(value)
		if err != nil {
			return fail("must be a UUID")
		}
		if key == "review_user" {
			_, err = q.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: id, WorkspaceID: ws})
		} else {
			_, err = q.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: id, WorkspaceID: ws})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return fail("must refer to a target in this workspace")
		}
		if err != nil {
			return nil, fmt.Errorf("validate %s: %w", key, err)
		}
		value = uuidToString(id)
	}
	result, err := json.Marshal(value)
	return result, err
}

// reviewMark is the normalized review object parsed from the five raw keys.
// present records whether review_by is set: without it the remaining values are
// only a partially prepared draft and are not published yet.
type reviewMark struct {
	present bool
	by      string
	action  string
	user    pgtype.UUID
	agent   pgtype.UUID
	kind    string
}

func reviewRawMetadata(raw []byte) (map[string]json.RawMessage, error) {
	existing := map[string]json.RawMessage{}
	if len(raw) == 0 {
		return existing, nil
	}
	if err := json.Unmarshal(raw, &existing); err != nil {
		return nil, fmt.Errorf("read current metadata: %w", err)
	}
	if existing == nil {
		existing = map[string]json.RawMessage{}
	}
	return existing, nil
}

func reviewStringValue(entries map[string]json.RawMessage, key string) string {
	raw, ok := entries[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func reviewUUIDValue(entries map[string]json.RawMessage, key string) pgtype.UUID {
	var value string
	if err := json.Unmarshal(entries[key], &value); err != nil {
		return pgtype.UUID{}
	}
	id, err := util.ParseUUID(value)
	if err != nil {
		return pgtype.UUID{}
	}
	return id
}

func reviewMarkFromEntries(entries map[string]json.RawMessage) reviewMark {
	mark := reviewMark{
		by:     reviewStringValue(entries, "review_by"),
		action: reviewStringValue(entries, "review_action"),
		user:   reviewUUIDValue(entries, "review_user"),
		agent:  reviewUUIDValue(entries, "review_agent"),
		kind:   reviewStringValue(entries, "review_kind"),
	}
	mark.present = mark.by != ""
	return mark
}

// reviewEntries returns only the five reserved keys from a metadata bag, in
// their raw (number-preserving) form.
func reviewEntries(metadata map[string]json.RawMessage) map[string]json.RawMessage {
	entries := make(map[string]json.RawMessage, len(reviewKeys))
	for _, key := range reviewKeys {
		if raw, ok := metadata[key]; ok {
			entries[key] = raw
		}
	}
	return entries
}

// reviewDispatcher resolves the issue's dispatcher (B9) and whether that member
// is still in the workspace.
func reviewDispatcher(ctx context.Context, q *db.Queries, issue db.Issue) (pgtype.UUID, bool, error) {
	rows, err := q.ListIssueDispatchers(ctx, db.ListIssueDispatchersParams{
		WorkspaceID: issue.WorkspaceID,
		Ids:         []pgtype.UUID{issue.ID},
	})
	if err != nil {
		return pgtype.UUID{}, false, fmt.Errorf("resolve issue dispatcher: %w", err)
	}
	if len(rows) == 0 {
		return pgtype.UUID{}, false, nil
	}
	return rows[0].DispatcherUserID, rows[0].DispatcherActive, nil
}

// validateReviewAssignee checks the issue's assignee can actually advance the
// card after the human answers (R4.0-4) and returns the member id when the
// assignee is a member. An unassigned or unresolvable issue cannot publish a
// user review: the card would have nobody to hand the outcome to.
func validateReviewAssignee(ctx context.Context, q *db.Queries, issue db.Issue) (pgtype.UUID, error) {
	fail := func(message string) (pgtype.UUID, error) {
		return pgtype.UUID{}, &reviewValidationError{"review_by user " + message}
	}
	if !issue.AssigneeType.Valid || !issue.AssigneeID.Valid {
		return fail("requires a valid issue assignee")
	}
	switch issue.AssigneeType.String {
	case "agent":
		agent, err := q.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: issue.AssigneeID, WorkspaceID: issue.WorkspaceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fail("requires a valid issue assignee")
		}
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("validate issue assignee: %w", err)
		}
		if agent.ArchivedAt.Valid {
			return fail("requires a valid issue assignee")
		}
	case "squad":
		squad, err := q.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: issue.AssigneeID, WorkspaceID: issue.WorkspaceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fail("requires a valid issue assignee")
		}
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("validate issue assignee: %w", err)
		}
		if squad.ArchivedAt.Valid {
			return fail("requires a valid issue assignee")
		}
	case "member":
		if _, err := q.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: issue.AssigneeID, WorkspaceID: issue.WorkspaceID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fail("requires a valid issue assignee")
			}
			return pgtype.UUID{}, fmt.Errorf("validate issue assignee: %w", err)
		}
		return issue.AssigneeID, nil
	default:
		return fail("requires a valid issue assignee")
	}
	return pgtype.UUID{}, nil
}

// validateReviewCombination validates the complete review object against the
// target status (CLAWSTA-151 B7 table) and fills the defaults a publish writes:
// review_kind defaults to approve for in_review, and review_user defaults to
// the issue's dispatcher when the review waits on a member.
func validateReviewCombination(ctx context.Context, q *db.Queries, issue db.Issue, targetStatus string, mark *reviewMark) error {
	if !isReviewStatus(targetStatus) {
		return &reviewValidationError{"review metadata requires status in_review or blocked"}
	}
	if !mark.present {
		return &reviewValidationError{"review_by is required to publish review metadata"}
	}
	switch mark.by {
	case reviewByUser:
		if mark.kind == "" {
			if targetStatus == reviewStatusInReview {
				mark.kind = reviewKindApprove
			} else {
				return &reviewValidationError{"review_kind must be reply or act for status blocked"}
			}
		}
		if targetStatus == reviewStatusInReview && mark.kind != reviewKindApprove {
			return &reviewValidationError{"review_kind must be approve for status in_review"}
		}
		if targetStatus == reviewStatusBlocked && mark.kind != reviewKindReply && mark.kind != reviewKindAct {
			return &reviewValidationError{"review_kind must be reply or act for status blocked"}
		}
		dispatcher, active, err := reviewDispatcher(ctx, q, issue)
		if err != nil {
			return err
		}
		if !dispatcher.Valid {
			return &reviewValidationError{"review_by user has no trusted member source: restore verified parent or creation-run lineage before publishing a user review"}
		}
		if !active {
			return &reviewValidationError{"review_by user dispatcher has left the workspace: restore their membership before publishing a user review"}
		}
		memberAssignee, err := validateReviewAssignee(ctx, q, issue)
		if err != nil {
			return err
		}
		if !mark.user.Valid {
			mark.user = dispatcher
		}
		if memberAssignee.Valid && mark.user == memberAssignee {
			return &reviewValidationError{"review_user cannot be the issue assignee"}
		}
	case reviewBySpecialist:
		if mark.kind == "" {
			mark.kind = reviewKindApprove
		}
		if targetStatus == reviewStatusBlocked {
			return &reviewValidationError{"review_by specialist is not valid for status blocked"}
		}
		if mark.kind != reviewKindApprove {
			return &reviewValidationError{"review_kind must be approve for status in_review"}
		}
	default:
		return &reviewValidationError{"review_by must be user or specialist"}
	}
	return nil
}

// reviewMarkEntries renders the validated mark back to normalized raw values.
func reviewMarkEntries(mark reviewMark) map[string]json.RawMessage {
	entries := map[string]json.RawMessage{}
	set := func(key, value string) {
		if value == "" {
			return
		}
		encoded, err := json.Marshal(value)
		if err == nil {
			entries[key] = encoded
		}
	}
	set("review_by", mark.by)
	set("review_action", mark.action)
	set("review_kind", mark.kind)
	if mark.user.Valid {
		set("review_user", uuidToString(mark.user))
	}
	if mark.agent.Valid {
		set("review_agent", uuidToString(mark.agent))
	}
	return entries
}

// resolveReviewMetadata validates the atomic review_metadata payload — the
// complete five-key object for the target status — and returns normalized
// entries to write. An empty object clears the round's mark (B7): the caller
// writes it AFTER the status update so the trigger's clear cannot strip it.
func resolveReviewMetadata(ctx context.Context, q *db.Queries, current db.Issue, raw json.RawMessage, targetStatus string) (map[string]json.RawMessage, error) {
	var input map[string]json.RawMessage
	if string(raw) == "null" || json.Unmarshal(raw, &input) != nil || input == nil {
		return nil, &reviewValidationError{"review_metadata must be an object"}
	}
	for key := range input {
		if !isReviewKey(key) {
			return nil, &reviewValidationError{"review_metadata only accepts the five review keys"}
		}
	}
	if len(input) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	entries := make(map[string]json.RawMessage, len(input))
	for key, value := range input {
		normalized, err := validateReviewValue(ctx, q, current.WorkspaceID, key, value)
		if err != nil {
			return nil, err
		}
		entries[key] = normalized
	}
	mark := reviewMarkFromEntries(entries)
	if err := validateReviewCombination(ctx, q, current, targetStatus, &mark); err != nil {
		return nil, err
	}
	return reviewMarkEntries(mark), nil
}

// resolveReviewSingleKey validates a single review-key mutation through the
// metadata endpoints (B7: incremental preparation still works; the write that
// publishes review_by validates the whole group and fills defaults).
//
// publish=false means "write only this one key"; publish=true returns the
// complete normalized five-key object for an atomic multi-key write.
func resolveReviewSingleKey(ctx context.Context, q *db.Queries, current db.Issue, key string, value json.RawMessage) (map[string]json.RawMessage, bool, error) {
	metadata, err := reviewRawMetadata(current.Metadata)
	if err != nil {
		return nil, false, err
	}
	previous := reviewEntries(metadata)
	if value == nil && key == "review_user" && reviewStringValue(previous, "review_by") == reviewByUser {
		// B7: an issued user review must never degrade into "any member" by
		// dropping its receiver. Unpublish first, then prepare the new mark.
		return nil, false, &reviewValidationError{"review_user cannot be removed while review_by is user; delete review_by first"}
	}
	entries := reviewEntries(metadata)
	if value == nil {
		delete(entries, key)
	} else {
		entries[key] = value
	}
	mark := reviewMarkFromEntries(entries)
	if !mark.present {
		// Preparing an unpublished draft, or removing the last published key.
		// Individual values were already validated; only the kind/status pairing
		// can be wrong at this stage.
		if kind := reviewStringValue(entries, "review_kind"); kind != "" {
			if current.Status == reviewStatusInReview && kind != reviewKindApprove {
				return nil, false, &reviewValidationError{"review_kind must be approve for status in_review"}
			}
			if current.Status == reviewStatusBlocked && kind != reviewKindReply && kind != reviewKindAct {
				return nil, false, &reviewValidationError{"review_kind must be reply or act for status blocked"}
			}
		}
		return nil, false, nil
	}
	if err := validateReviewCombination(ctx, q, current, current.Status, &mark); err != nil {
		return nil, false, err
	}
	return reviewMarkEntries(mark), true, nil
}

// mergeReviewMetadata replaces the five reserved keys under metadata with the
// normalized entries from a publish, preserving every other key byte-for-byte.
// The result is capped by the same 50-key / 8KB limits as any metadata write.
func mergeReviewMetadata(metadata json.RawMessage, entries map[string]json.RawMessage) ([]byte, error) {
	existing, err := reviewRawMetadata(metadata)
	if err != nil {
		return nil, err
	}
	for _, key := range reviewKeys {
		delete(existing, key)
	}
	for key, value := range entries {
		existing[key] = value
	}
	if len(existing) > maxIssueMetadataKeys {
		return nil, &reviewValidationError{"metadata cannot exceed 50 keys"}
	}
	result, err := json.Marshal(existing)
	if err != nil {
		return nil, err
	}
	if len(result) > 8192 {
		return nil, &reviewValidationError{"metadata exceeds the 8KB size limit"}
	}
	return result, nil
}

// jsonMetadataEqual compares two JSON documents by value, preserving large JSON
// integers (json.Number keeps every digit) so the review publish can decide
// whether its second write actually changes the row.
func jsonMetadataEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var left, right any
	leftDecoder := json.NewDecoder(bytes.NewReader(a))
	leftDecoder.UseNumber()
	if err := leftDecoder.Decode(&left); err != nil {
		return false
	}
	rightDecoder := json.NewDecoder(bytes.NewReader(b))
	rightDecoder.UseNumber()
	if err := rightDecoder.Decode(&right); err != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}
