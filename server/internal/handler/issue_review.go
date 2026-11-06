package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var reviewKeys = []string{"review_by", "review_action", "review_user", "review_agent"}
var errReviewClosed = errors.New("review metadata requires status in_review")

type reviewValidationError struct{ message string }

func (e *reviewValidationError) Error() string { return e.message }

func isReviewKey(key string) bool {
	for _, k := range reviewKeys {
		if k == key {
			return true
		}
	}
	return false
}

// Validate on the same transaction that holds the issue row lock.
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
		if value != "user" && value != "specialist" {
			return fail("must be user or specialist")
		}
	case "review_action":
		if utf8.RuneCountInString(value) > 120 {
			return fail("must contain at most 120 Unicode code points")
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

func prepareReviewMetadata(ctx context.Context, q *db.Queries, current db.Issue, raw json.RawMessage) ([]byte, error) {
	var input map[string]json.RawMessage
	if string(raw) == "null" || json.Unmarshal(raw, &input) != nil || input == nil {
		return nil, &reviewValidationError{"review_metadata must be an object"}
	}
	// Keep unrelated JSON numbers exact, including integers larger than 2^53.
	var existing map[string]json.RawMessage
	if err := json.Unmarshal(current.Metadata, &existing); err != nil {
		return nil, fmt.Errorf("read current metadata: %w", err)
	}
	if existing == nil {
		existing = make(map[string]json.RawMessage)
	}
	for _, k := range reviewKeys {
		delete(existing, k)
	}
	for k, v := range input {
		if !isReviewKey(k) {
			return nil, &reviewValidationError{"review_metadata only accepts the four review keys"}
		}
		normalized, err := validateReviewValue(ctx, q, current.WorkspaceID, k, v)
		if err != nil {
			return nil, err
		}
		existing[k] = normalized
	}
	if len(existing) > maxIssueMetadataKeys {
		return nil, &reviewValidationError{"metadata cannot exceed 50 keys"}
	}
	result, err := json.Marshal(existing)
	if len(result) > 8192 {
		return nil, &reviewValidationError{"metadata exceeds the 8KB size limit"}
	}
	return result, err
}
