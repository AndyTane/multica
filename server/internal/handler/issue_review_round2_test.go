package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// reviewFixture creates an issue with the columns the R4 review contract needs
// and returns its current row.
func reviewFixture(t *testing.T, title string, cols testutil.Cols) (string, db.Issue) {
	t.Helper()
	id := dbfx.Issue(t, title, cols)
	current, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
	if err != nil {
		t.Fatal(err)
	}
	return id, current
}

func reviewMetadataOf(t *testing.T, id string) (db.Issue, map[string]any) {
	t.Helper()
	current, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
	if err != nil {
		t.Fatal(err)
	}
	return current, parseIssueMetadata(current.Metadata)
}

func quickCreateOriginTask(t *testing.T, agentID, requesterID string, override map[string]any) string {
	t.Helper()
	contextValue := fmt.Sprintf(`{"type":"quick_create","prompt":"x","requester_id":%q,"workspace_id":%q}`, requesterID, testWorkspaceID)
	cols := testutil.Cols{
		"issue_id":            nil,
		"runtime_id":          testRuntimeID,
		"originator_user_id":  requesterID,
		"accountable_user_id": requesterID,
		"context":             contextValue,
	}
	for key, value := range override {
		cols[key] = value
	}
	return dbfx.Task(t, agentID, cols)
}

// TestReviewKindPublishMatrix pins the B7 table: which (status, review_by,
// review_kind) combinations may publish, and what a publish fills in.
func TestReviewKindPublishMatrix(t *testing.T) {
	advancer := dbfx.Agent(t, "Review matrix advancer", "")
	otherUser := dbfx.User(t, "Review matrix member", "review-matrix-member@example.test")
	dbfx.Member(t, testWorkspaceID, otherUser, "member")

	publish := func(t *testing.T, id string, body map[string]any, want int) IssueResponse {
		t.Helper()
		current, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
		body["expected_revision"] = current.Revision
		return reviewPut(t, id, body, want)
	}

	t.Run("in_review user fills approve and dispatcher", func(t *testing.T) {
		id, _ := reviewFixture(t, "Kind approve", testutil.Cols{"status": "in_review", "assignee_type": "agent", "assignee_id": advancer})
		resp := publish(t, id, map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "user"}}, 200)
		if resp.Metadata["review_kind"] != reviewKindApprove {
			t.Fatalf("review_kind = %v, want approve", resp.Metadata["review_kind"])
		}
		if resp.Metadata["review_user"] != testUserID {
			t.Fatalf("review_user = %v, want the dispatcher", resp.Metadata["review_user"])
		}
	})

	t.Run("in_review rejects reply kind", func(t *testing.T) {
		id, _ := reviewFixture(t, "Kind wrong in review", testutil.Cols{"status": "in_review", "assignee_type": "agent", "assignee_id": advancer})
		publish(t, id, map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "user", "review_kind": "reply"}}, 400)
	})

	t.Run("blocked requires reply or act", func(t *testing.T) {
		id, _ := reviewFixture(t, "Blocked kind required", testutil.Cols{"status": "blocked", "assignee_type": "agent", "assignee_id": advancer})
		publish(t, id, map[string]any{"status": "blocked", "review_metadata": map[string]any{"review_by": "user"}}, 400)
		publish(t, id, map[string]any{"status": "blocked", "review_metadata": map[string]any{"review_by": "user", "review_kind": "approve"}}, 400)
		resp := publish(t, id, map[string]any{"status": "blocked", "review_metadata": map[string]any{"review_by": "user", "review_kind": "act"}}, 200)
		if resp.Metadata["review_kind"] != reviewKindAct {
			t.Fatalf("blocked publish kept kind %v", resp.Metadata["review_kind"])
		}
	})

	t.Run("blocked rejects specialist", func(t *testing.T) {
		id, _ := reviewFixture(t, "Blocked specialist", testutil.Cols{"status": "blocked", "assignee_type": "agent", "assignee_id": advancer})
		publish(t, id, map[string]any{"status": "blocked", "review_metadata": map[string]any{"review_by": "specialist", "review_kind": "approve"}}, 400)
	})

	t.Run("specialist does not need a dispatcher", func(t *testing.T) {
		creator := dbfx.Agent(t, "Agent-created owner", "")
		originTask := dbfx.Task(t, creator, testutil.Cols{"issue_id": nil, "runtime_id": testRuntimeID, "originator_user_id": testUserID, "accountable_user_id": testUserID})
		id, _ := reviewFixture(t, "Specialist card", testutil.Cols{
			"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
			"creator_type": "agent", "creator_id": creator, "origin_type": "agent_create", "origin_id": originTask,
		})
		resp := publish(t, id, map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "specialist"}}, 200)
		if resp.Metadata["review_kind"] != reviewKindApprove {
			t.Fatalf("specialist kind = %v, want approve", resp.Metadata["review_kind"])
		}
		if _, ok := resp.Metadata["review_user"]; ok {
			t.Fatalf("specialist publish filled review_user: %v", resp.Metadata["review_user"])
		}
	})

	t.Run("explicit other member is honoured", func(t *testing.T) {
		id, _ := reviewFixture(t, "Explicit receiver", testutil.Cols{"status": "in_review", "assignee_type": "agent", "assignee_id": advancer})
		resp := publish(t, id, map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "user", "review_user": otherUser}}, 200)
		if resp.Metadata["review_user"] != otherUser {
			t.Fatalf("review_user = %v, want explicit member", resp.Metadata["review_user"])
		}
	})

	t.Run("member assignee cannot be the approver", func(t *testing.T) {
		id, _ := reviewFixture(t, "Member assignee", testutil.Cols{"status": "in_review", "assignee_type": "member", "assignee_id": testUserID})
		publish(t, id, map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "user"}}, 400)
	})

	t.Run("no dispatcher rejects user review", func(t *testing.T) {
		creator := dbfx.Agent(t, "Agent-create owner", "")
		originTask := dbfx.Task(t, creator, testutil.Cols{"issue_id": nil, "runtime_id": testRuntimeID, "originator_user_id": testUserID, "accountable_user_id": testUserID})
		id, _ := reviewFixture(t, "No dispatcher", testutil.Cols{
			"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
			"creator_type": "agent", "creator_id": creator, "origin_type": "agent_create", "origin_id": originTask,
		})
		publish(t, id, map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "user"}}, 400)
	})

	t.Run("quick create inherits the requester as dispatcher", func(t *testing.T) {
		creator := dbfx.Agent(t, "Quick-create creator", "")
		originTask := quickCreateOriginTask(t, creator, testUserID, nil)
		id, _ := reviewFixture(t, "Quick card", testutil.Cols{
			"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
			"creator_type": "agent", "creator_id": creator, "origin_type": "quick_create", "origin_id": originTask,
		})
		resp := publish(t, id, map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "user"}}, 200)
		if resp.Metadata["review_user"] != testUserID {
			t.Fatalf("review_user = %v, want quick-create requester", resp.Metadata["review_user"])
		}
	})

	t.Run("forged quick create context resolves no dispatcher", func(t *testing.T) {
		creator := dbfx.Agent(t, "Forged quick creator", "")
		originTask := quickCreateOriginTask(t, creator, otherUser, testutil.Cols{"originator_user_id": testUserID, "accountable_user_id": testUserID})
		id, _ := reviewFixture(t, "Forged quick card", testutil.Cols{
			"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
			"creator_type": "agent", "creator_id": creator, "origin_type": "quick_create", "origin_id": originTask,
		})
		publish(t, id, map[string]any{"status": "in_review", "review_metadata": map[string]any{"review_by": "user"}}, 400)
	})
}

// TestReviewTransitionsClearFiveKeys covers B8: leaving either review status
// clears the whole mark, both directions, and a non-status edit keeps it.
func TestReviewTransitionsClearFiveKeys(t *testing.T) {
	advancer := dbfx.Agent(t, "Transition advancer", "")
	id, current := reviewFixture(t, "Transition card", testutil.Cols{
		"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
		"metadata": `{"review_by":"user","review_action":"check","review_user":"` + testUserID + `","review_agent":"` + advancer + `","review_kind":"approve","other":true}`,
	})
	edited := reviewPut(t, id, map[string]any{"title": "Transition edited"}, 200)
	if _, ok := edited.Metadata["review_kind"]; !ok {
		t.Fatal("non-status edit cleared the review mark")
	}

	blocked := reviewPut(t, id, map[string]any{
		"status": "blocked", "expected_revision": edited.Revision,
		"review_metadata": map[string]any{"review_by": "user", "review_kind": "reply", "review_action": "answer me"},
	}, 200)
	if blocked.Metadata["review_kind"] != reviewKindReply || blocked.Metadata["other"] != true {
		t.Fatalf("blocked publish: %v", blocked.Metadata)
	}

	back := reviewPut(t, id, map[string]any{"status": "in_review", "expected_revision": blocked.Revision, "review_metadata": map[string]any{"review_by": "user"}}, 200)
	if back.Metadata["review_kind"] != reviewKindApprove {
		t.Fatalf("in_review publish after blocked: %v", back.Metadata)
	}

	cleared := reviewPut(t, id, map[string]any{"status": "in_progress", "expected_revision": back.Revision}, 200)
	for _, key := range reviewKeys {
		if _, ok := cleared.Metadata[key]; ok {
			t.Fatalf("leaving review leaked %s", key)
		}
	}
	if cleared.Metadata["other"] != true {
		t.Fatal("unrelated metadata removed")
	}

	// blocked -> done clears just like in_review -> done.
	blockedAgain := reviewPut(t, id, map[string]any{"status": "blocked", "expected_revision": cleared.Revision, "review_metadata": map[string]any{"review_by": "user", "review_kind": "act"}}, 200)
	done := reviewPut(t, id, map[string]any{"status": "done", "expected_revision": blockedAgain.Revision}, 200)
	for _, key := range reviewKeys {
		if _, ok := done.Metadata[key]; ok {
			t.Fatalf("blocked -> done leaked %s", key)
		}
	}
	if _, ok := parseIssueMetadata(current.Metadata)["review_since"]; ok {
		t.Fatal("raw fixture insert should not carry a review round time")
	}
}

// TestReviewEmptyObjectClearsMark checks the {} form of review_metadata.
func TestReviewEmptyObjectClearsMark(t *testing.T) {
	advancer := dbfx.Agent(t, "Clear advancer", "")
	id, current := reviewFixture(t, "Clear mark", testutil.Cols{
		"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
		"metadata": `{"review_by":"user","review_action":"check","review_kind":"approve","keep":true}`,
	})
	resp := reviewPut(t, id, map[string]any{"status": "in_review", "expected_revision": current.Revision, "review_metadata": map[string]any{}}, 200)
	for _, key := range reviewKeys {
		if _, ok := resp.Metadata[key]; ok {
			t.Fatalf("{} leaked %s", key)
		}
	}
	if resp.Metadata["keep"] != true {
		t.Fatal("clear removed unrelated metadata")
	}
}

// TestReviewMetadataOnlyWriteBumpsRevisionOnce pins the B8 revision contract:
// the status/metadata pair is one revision bump, and an identical repeat is a
// no-op.
func TestReviewMetadataOnlyWriteBumpsRevisionOnce(t *testing.T) {
	advancer := dbfx.Agent(t, "Revision advancer", "")
	// The local port keeps the round entry time in metadata.review_since (the
	// status_changed_at column does not exist on this deployment), so seed it the
	// way a real transition would and then prove a metadata-only publish keeps it.
	id, _ := reviewFixture(t, "Revision card", testutil.Cols{"status": "in_review", "assignee_type": "agent", "assignee_id": advancer, "metadata": `{"review_since":"2026-01-02T03:04:05Z"}`})
	current, _ := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))

	first := reviewPut(t, id, map[string]any{
		"status": "in_review", "expected_revision": current.Revision,
		"review_metadata": map[string]any{"review_by": "user", "review_action": "look"},
	}, 200)
	if first.Revision != current.Revision+1 {
		t.Fatalf("revision = %d, want %d", first.Revision, current.Revision+1)
	}
	if first.Metadata["review_since"] != "2026-01-02T03:04:05Z" {
		t.Fatalf("metadata-only write reset review_since: %v", first.Metadata["review_since"])
	}

	second := reviewPut(t, id, map[string]any{
		"status": "in_review", "expected_revision": first.Revision,
		"review_metadata": map[string]any{"review_by": "user", "review_action": "look"},
	}, 200)
	if second.Revision != first.Revision {
		t.Fatalf("identical repeat bumped revision to %d", second.Revision)
	}
}

// TestReviewInvalidPublishLeavesRowUntouched proves validation happens before
// the status write, so a rejected combination cannot half-apply.
func TestReviewInvalidPublishLeavesRowUntouched(t *testing.T) {
	advancer := dbfx.Agent(t, "Invalid publish advancer", "")
	id, current := reviewFixture(t, "Invalid publish", testutil.Cols{
		"status": "in_review", "assignee_type": "agent", "assignee_id": advancer,
		"metadata": `{"review_by":"user","review_kind":"approve","keep":true}`,
	})
	reviewPut(t, id, map[string]any{"status": "blocked", "expected_revision": current.Revision, "review_metadata": map[string]any{"review_by": "specialist"}}, 400)
	after, metadata := reviewMetadataOf(t, id)
	if after.Status != "in_review" || after.Revision != current.Revision {
		t.Fatalf("rejected publish changed the row: %s revision %d", after.Status, after.Revision)
	}
	if metadata["review_kind"] != reviewKindApprove || metadata["keep"] != true {
		t.Fatalf("rejected publish changed metadata: %v", metadata)
	}
}

// TestReviewSingleKeyPublication covers the CLI path: fields can be prepared
// one at a time, writing review_by publishes and fills defaults, and an issued
// user mark cannot be silently reduced by deleting its receiver.
func TestReviewSingleKeyPublication(t *testing.T) {
	advancer := dbfx.Agent(t, "Single key advancer", "")
	id, _ := reviewFixture(t, "Single key card", testutil.Cols{"status": "blocked", "assignee_type": "agent", "assignee_id": advancer})

	setKey := func(key string, value any, want int) {
		t.Helper()
		req := withURLParams(newRequest("PUT", "/api/issues/"+id+"/metadata/"+key, map[string]any{"value": value}), "id", id, "key", key)
		testutil.Call(t, testHandler.SetIssueMetadataKey, req).Want(want)
	}
	setKey("review_kind", "reply", 200)
	_, metadata := reviewMetadataOf(t, id)
	if metadata["review_kind"] != reviewKindReply || metadata["review_by"] != nil {
		t.Fatalf("prepare step: %v", metadata)
	}
	setKey("review_by", "user", 200)
	_, published := reviewMetadataOf(t, id)
	if published["review_kind"] != reviewKindReply || published["review_user"] != testUserID {
		t.Fatalf("publish step: %v", published)
	}

	delKey := func(key string, want int) {
		t.Helper()
		req := withURLParams(newRequest("DELETE", "/api/issues/"+id+"/metadata/"+key, nil), "id", id, "key", key)
		testutil.Call(t, testHandler.DeleteIssueMetadataKey, req).Want(want)
	}
	delKey("review_user", 400)
	delKey("review_by", 200)
	delKey("review_user", 200)
	_, unpublished := reviewMetadataOf(t, id)
	if _, ok := unpublished["review_by"]; ok {
		t.Fatalf("unpublish left review_by: %v", unpublished)
	}
	if _, ok := unpublished["review_user"]; ok {
		t.Fatalf("unpublish left review_user: %v", unpublished)
	}

	// A published mark cannot be edited into an invalid combination: blocked
	// requires reply/act, so switching the kind to approve is refused.
	setKey("review_by", "user", 200)
	setKey("review_kind", "approve", 400)
	setKey("review_action", "do it", 200)
}

// TestReviewStatusWritersClearBlockedMark extends the status-writer matrix to
// the second review status.
func TestReviewStatusWritersClearBlockedMark(t *testing.T) {
	for _, status := range []string{"in_progress", "done", "in_review", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			id := dbfx.Issue(t, "Blocked background status writer", testutil.Cols{"status": "blocked", "metadata": `{"review_by":"user","review_kind":"act","review_user":"` + testUserID + `","review_other":"clear","keep":"survives"}`})
			got, err := testHandler.Queries.UpdateIssueStatus(context.Background(), db.UpdateIssueStatusParams{ID: parseUUID(id), WorkspaceID: parseUUID(testWorkspaceID), Status: status})
			if err != nil {
				t.Fatal(err)
			}
			meta := parseIssueMetadata(got.Metadata)
			// This deployment clears EVERY review_* key on the way out (including
			// the local round timer), so a collision-named review_other must go and
			// only a genuinely unrelated key survives.
			if meta["keep"] != "survives" {
				t.Fatalf("status writer deleted unrelated metadata: %s", got.Metadata)
			}
			for _, key := range []string{"review_by", "review_action", "review_user", "review_kind", "review_other"} {
				if _, ok := meta[key]; ok {
					t.Fatalf("status writer leaked %s: %s", key, got.Metadata)
				}
			}
			// Entering in_review is itself a new review round on this deployment,
			// so it must restart the entry timer; every other target clears it.
			if status == "in_review" {
				if _, ok := meta["review_since"]; !ok {
					t.Fatalf("entering in_review did not stamp review_since: %s", got.Metadata)
				}
			} else if _, ok := meta["review_since"]; ok {
				t.Fatalf("leaving review kept review_since: %s", got.Metadata)
			}
		})
	}
}

// TestReviewKindRejectedOutsideReviewStatus keeps single-key review writes
// inside the two statuses that can wait on a person.
func TestReviewKindRejectedOutsideReviewStatus(t *testing.T) {
	id := dbfx.Issue(t, "Kind outside review", testutil.Cols{"status": "in_progress"})
	req := withURLParams(newRequest("PUT", "/api/issues/"+id+"/metadata/review_kind", map[string]any{"value": "act"}), "id", id, "key", "review_kind")
	testutil.Call(t, testHandler.SetIssueMetadataKey, req).Want(409)
}
