package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
)

func newIssueReviewCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "review <id>",
		Short: "Atomically submit a card for approval, a reply, or an offline action",
		Args:  cobra.ExactArgs(1), RunE: runIssueReview,
	}
	c.Flags().String("by", "user", "Who reviews: user or specialist")
	c.Flags().String("kind", "approve", "approve (in_review), reply or act (blocked)")
	c.Flags().String("action", "", "One sentence describing what the reviewer should do (1–120 characters)")
	c.Flags().String("user", "", "Optional reviewing member UUID; defaults to the dispatcher")
	c.Flags().String("agent", "", "Optional specialist agent UUID")
	c.Flags().String("output", "table", "Output format: table or json")
	return c
}

func init() { issueCmd.AddCommand(newIssueReviewCmd()) }

func runIssueReview(cmd *cobra.Command, args []string) error {
	by, _ := cmd.Flags().GetString("by")
	kind, _ := cmd.Flags().GetString("kind")
	action, _ := cmd.Flags().GetString("action")
	action = strings.TrimSpace(action)
	user, _ := cmd.Flags().GetString("user")
	agent, _ := cmd.Flags().GetString("agent")
	if by != "user" && by != "specialist" {
		return fmt.Errorf("--by must be user or specialist")
	}
	if kind != "approve" && kind != "reply" && kind != "act" {
		return fmt.Errorf("--kind must be approve, reply or act")
	}
	if by == "specialist" && kind != "approve" {
		return fmt.Errorf("specialist review requires --kind approve")
	}
	if action == "" || utf8.RuneCountInString(action) > 120 {
		return fmt.Errorf("--action must contain 1–120 characters")
	}
	if user != "" && (by != "user" || !uuidRegexp.MatchString(user)) {
		return fmt.Errorf("--user requires --by user and a member UUID")
	}
	if agent != "" && (by != "specialist" || !uuidRegexp.MatchString(agent)) {
		return fmt.Errorf("--agent requires --by specialist and an agent UUID")
	}
	output, _ := cmd.Flags().GetString("output")
	if output != "json" && output != "table" {
		return fmt.Errorf("--output must be table or json")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	ref, err := resolveIssueRef(ctx, client, args[0])
	if err != nil {
		return err
	}
	var current struct {
		Revision int64 `json:"revision"`
	}
	if err := client.GetJSON(ctx, "/api/issues/"+ref.ID, &current); err != nil {
		return fmt.Errorf("read review revision: %w", err)
	}
	if current.Revision < 1 {
		return fmt.Errorf("server did not return an issue revision; review was not submitted")
	}
	status := "in_review"
	if kind != "approve" {
		status = "blocked"
	}
	mark := map[string]string{"review_by": by, "review_kind": kind, "review_action": action}
	if user != "" {
		mark["review_user"] = user
	}
	if agent != "" {
		mark["review_agent"] = agent
	}
	body := map[string]any{"status": status, "expected_revision": current.Revision, "review_metadata": mark}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+ref.ID+"/review", body, &result); err != nil {
		return fmt.Errorf("submit review (requires the atomic review endpoint; on conflict read the card before retrying): %w", err)
	}
	fmt.Fprintf(os.Stderr, "Issue %s submitted to %s (%s): %s\n", issueDisplayKey(result), by, kind, action)
	if output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	return nil
}
