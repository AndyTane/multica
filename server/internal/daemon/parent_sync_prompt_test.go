package daemon

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParentSyncClaimPrompt(t *testing.T) {
	var task Task
	err := json.Unmarshal([]byte(`{"issue_id":"child","trigger_comment_id":"new-comment","trigger_thread_id":"thread-new","trigger_comment_content":"Please review the child result","parent_sync_card_id":"parent","parent_sync_comment_id":"source"}`), &task)
	if err != nil {
		t.Fatal(err)
	}
	if task.ParentSyncCommentID != "source" {
		t.Fatal("claim lost source comment")
	}
	for _, multi := range []bool{false, true} {
		if multi {
			task.CoalescedComments = []CoalescedCommentData{{ID: "old-comment", ThreadID: "thread-old", Content: "Earlier update"}}
		}
		out := buildCommentPrompt(task, "codex")
		parent := strings.Index(out, "multica issue get parent --output json")
		child := strings.Index(out, "multica issue get child --output json")
		if parent < 0 || child < parent {
			t.Fatalf("parent must be read before child: %s", out)
		}
		for _, want := range []string{"not taking over the child's assignment", "Only after that command succeeds", "if it fails, reply normally", "no other @mention, reply, joined wakeup", "multica issue activity child no_action", "--parent new-comment"} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q", want)
			}
		}
		if multi && (!strings.Contains(out, "--parent old-comment") || !strings.Contains(out, "2 DISTINCT threads")) {
			t.Fatal("multi-thread routing lost")
		}
	}
}

func TestParentSyncOrdinaryCommentsAndJoinedWork(t *testing.T) {
	task := Task{IssueID: "child", TriggerCommentID: "comment", TriggerCommentContent: "Update"}
	out := buildCommentPrompt(task, "codex")
	if strings.Contains(out, "Parent card sync") || strings.Contains(out, "issue activity") {
		t.Fatal("ordinary comment granted parent-sync behavior")
	}
	if !strings.Contains(out, "Post your reply as a comment") {
		t.Fatal("ordinary reply obligation lost")
	}
	task.ParentSyncCardID = "parent"
	task.WakeupJoined = "Execute the acceptance check"
	out = buildCommentPrompt(task, "codex")
	if !strings.Contains(out, "no_action is unavailable") || strings.Contains(out, "Unless this pure parent-sync") {
		t.Fatal("joined work must reply normally")
	}
}
