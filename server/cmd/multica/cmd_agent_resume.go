package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/daemon/execenv"
	"github.com/spf13/cobra"
)

var agentResumeCmd = &cobra.Command{
	Use:   "resume <agent-id> <task-id>",
	Short: "Continue a finished run in the original Claude or Codex CLI",
	Long:  "Continue a finished run on the computer that executed it. Checks the session and local directories before opening the original CLI. The manual conversation is not a new Multica run and is not automatically delivered to the issue.",
	Args:  exactArgs(2), RunE: runAgentResume,
}

func init() {
	agentCmd.AddCommand(agentResumeCmd)
	registerAgentResumeFlags(agentResumeCmd)
}

func registerAgentResumeFlags(cmd *cobra.Command) {
	cmd.Flags().String("workspaces-root", "", "Workspace root of the daemon that executed this run")
	cmd.Flags().Bool("print", false, "Print the resolved launch as JSON without starting an agent")
}

type resumeTask struct {
	ID             string `json:"id"`
	AgentID        string `json:"agent_id"`
	RuntimeID      string `json:"runtime_id"`
	WorkspaceID    string `json:"workspace_id"`
	IssueID        string `json:"issue_id"`
	ChatSessionID  string `json:"chat_session_id"`
	Status         string `json:"status"`
	SessionID      string `json:"session_id"`
	WorkDir        string `json:"work_dir"`
	DurableWorkDir string `json:"durable_work_dir"`
}

type resumeLaunch struct {
	Provider   string   `json:"provider"`
	SessionID  string   `json:"session_id"`
	Directory  string   `json:"directory"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	CodexHome  string   `json:"codex_home,omitempty"`
}

// Tests replace lookup with a test-created path; they never discover installed agent CLIs.
var resumeLookupExecutable = exec.LookPath

func runAgentResume(cmd *cobra.Command, args []string) error {
	if err := requireHumanLocalCommand("agent resume"); err != nil {
		return cli.WithUserMessage(err.Error(), err)
	}
	for i, id := range args {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return resumeError("Agent and task IDs must be UUIDs.")
		}
		args[i] = parsed.String()
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(cmd.Context())
	defer cancel()
	task, err := loadResumeTask(ctx, client, args[0], args[1])
	if err != nil {
		return err
	}
	var runtimes []struct {
		ID       string `json:"id"`
		Provider string `json:"provider"`
	}
	if err := client.GetJSON(ctx, "/api/runtimes", &runtimes); err != nil {
		return fmt.Errorf("read run runtime: %w", err)
	}
	provider := ""
	for _, rt := range runtimes {
		if rt.ID == task.RuntimeID {
			provider = rt.Provider
			break
		}
	}
	if provider == "" {
		return resumeError("The run's runtime is unavailable in this workspace.")
	}
	profile, _ := cmd.Flags().GetString("profile")
	rootOverride, _ := cmd.Flags().GetString("workspaces-root")
	root, err := resolveWorkspacesRootForProfile(profile, rootOverride)
	if err != nil {
		return err
	}
	launch, err := resolveResumeLaunch(task, provider, root)
	if err != nil {
		return err
	}
	launch.Executable, err = resumeLookupExecutable(provider)
	if err != nil {
		return resumeError(fmt.Sprintf("The %s CLI is unavailable on this computer. Install it or use the original runtime computer.", provider))
	}
	printOnly, _ := cmd.Flags().GetBool("print")
	if printOnly {
		return cli.PrintJSON(cmd.OutOrStdout(), launch)
	}
	child := interactiveResumeCommand(cmd.Context(), launch)
	child.Dir = launch.Directory
	child.Env = resumeEnvironment(os.Environ(), launch.CodexHome)
	child.Stdin, child.Stdout, child.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
	return cli.WithUserMessage("The resumed "+provider+" session exited unsuccessfully.", child.Run())
}

func resumeError(message string) error { return &cli.UserMessageError{Msg: message} }

func loadResumeTask(ctx context.Context, client *cli.APIClient, agentID, taskID string) (resumeTask, error) {
	var target *resumeTask
	cursor := ""
	for page := 0; page < 100; page++ {
		path := "/api/agents/" + agentID + "/tasks?limit=200"
		if cursor != "" {
			path += "&before=" + url.QueryEscape(cursor)
		}
		var tasks []resumeTask
		headers, err := client.GetJSONWithHeaders(ctx, path, &tasks)
		if err != nil {
			return resumeTask{}, fmt.Errorf("read authorized run history: %w", err)
		}
		for _, task := range tasks {
			if task.AgentID != agentID {
				return resumeTask{}, resumeError("Run history has mismatched agent identity.")
			}
			if task.ID == taskID {
				copy := task
				target = &copy
			}
		}
		next := headers.Get("X-Agent-Tasks-Next-Cursor")
		if target != nil || next == "" {
			break
		}
		if next == cursor || page == 99 {
			return resumeTask{}, resumeError("Run history pagination did not finish.")
		}
		cursor = next
	}
	if target == nil {
		return resumeTask{}, resumeError("Run not found in authorized history.")
	}
	if !terminalResumeStatus(target.Status) {
		return resumeTask{}, resumeError("Wait for the Multica run to finish before resuming.")
	}
	var snapshot []resumeTask
	if err := client.GetJSON(ctx, "/api/agent-task-snapshot", &snapshot); err != nil {
		return resumeTask{}, fmt.Errorf("check active conversations: %w", err)
	}
	for _, other := range snapshot {
		if other.AgentID == target.AgentID && !terminalResumeStatus(other.Status) && (other.SessionID != "" && other.SessionID == target.SessionID ||
			target.IssueID != "" && other.IssueID == target.IssueID ||
			target.ChatSessionID != "" && other.ChatSessionID == target.ChatSessionID) {
			return resumeTask{}, resumeError("This conversation has an active Multica run; wait for it to finish.")
		}
	}
	return *target, nil
}

func terminalResumeStatus(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled"
}

func resolveResumeLaunch(task resumeTask, provider, root string) (resumeLaunch, error) {
	if provider != "claude" && provider != "codex" {
		return resumeLaunch{}, resumeError("Manual resume supports only Claude and Codex.")
	}
	for _, id := range []string{task.ID, task.AgentID, task.RuntimeID, task.WorkspaceID, task.SessionID} {
		if _, err := uuid.Parse(id); err != nil {
			return resumeLaunch{}, resumeError("Run is missing a valid session or identity.")
		}
	}
	if !terminalResumeStatus(task.Status) {
		return resumeLaunch{}, resumeError("Run is still active.")
	}
	workdir := ""
	for _, candidate := range []string{task.WorkDir, task.DurableWorkDir} {
		if !filepath.IsAbs(candidate) {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			workdir = candidate
			break
		}
	}
	if workdir == "" {
		return resumeLaunch{}, resumeError("Run directory is unavailable on this computer; use the original runtime computer or restore the directory.")
	}
	launch := resumeLaunch{Provider: provider, SessionID: task.SessionID, Directory: workdir, Args: []string{"--resume", task.SessionID}}
	if provider == "codex" {
		launch.Args = []string{"resume", task.SessionID, "--cd", workdir}
		candidates := []string{filepath.Dir(task.WorkDir)}
		if indexed, err := execenv.ExistingRootDir(root, task.WorkspaceID, task.ID); err == nil {
			candidates = append(candidates, indexed)
		}
		for _, candidate := range candidates {
			owner, err := execenv.ReadEnvRootOwner(candidate)
			if err != nil || owner.WorkspaceID != task.WorkspaceID {
				continue
			}
			if execenv.ValidateEnvRootOwnerPath(root, candidate, *owner) != nil {
				continue
			}
			if owner.TaskID != task.ID {
				provenance, err := execenv.ReadManagedEnvProvenance(candidate)
				if err != nil || provenance == nil || provenance.ManagedBy != execenv.ManagedEnvProvenanceManagedBy || provenance.AgentID != task.AgentID || provenance.WorkspaceID != task.WorkspaceID || provenance.IssueID != task.IssueID || provenance.ChatSessionID != task.ChatSessionID {
					continue
				}
			}
			home := filepath.Join(candidate, "codex-home")
			if execenv.CodexResumeRolloutPresent(home, task.SessionID) {
				launch.CodexHome = home
				break
			}
		}
		if launch.CodexHome == "" {
			return resumeLaunch{}, resumeError("The run's Codex session home or rollout is unavailable; it may have been cleaned up.")
		}
	}
	return launch, nil
}

func resumeEnvironment(environment []string, codexHome string) []string {
	clean := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		key, _, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "MULTICA_") || codexHome != "" && upper == "CODEX_HOME" {
			continue
		}
		clean = append(clean, item)
	}
	if codexHome != "" {
		clean = append(clean, "CODEX_HOME="+codexHome)
	}
	return clean
}

func interactiveResumeCommand(ctx context.Context, launch resumeLaunch) *exec.Cmd {
	if runtime.GOOS == "windows" {
		quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
		parts := []string{"&", quote(launch.Executable)}
		for _, arg := range launch.Args {
			parts = append(parts, quote(arg))
		}
		return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", strings.Join(parts, " ")+"; exit $LASTEXITCODE")
	}
	return exec.CommandContext(ctx, launch.Executable, launch.Args...)
}
