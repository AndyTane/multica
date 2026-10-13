package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/daemon/execenv"
	"github.com/spf13/cobra"
)

func resumeFixture(t *testing.T) (resumeTask, string, string) {
	t.Helper()
	root := t.TempDir()
	task := resumeTask{ID: "01a136db-92cf-75b1-a804-f42445c19c9b", AgentID: "68ca5d11-c32c-453c-82bf-ec44dbcf66a8", RuntimeID: "0ab891eb-04ec-4584-8f0a-4302e431e25a", WorkspaceID: "8daf5cf8-6287-44d5-84cd-6e7d72e80f35", IssueID: "01a131ac-a6ea-7a2a-bf32-3917adc4add8", SessionID: "01a136db-d24d-7533-a0a0-b646d5ee0439", Status: "completed"}
	envRoot, err := execenv.ResolveRootDir(execenv.RootDirParams{WorkspacesRoot: root, WorkspaceID: task.WorkspaceID, TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	task.WorkDir = filepath.Join(envRoot, "workdir")
	if err := os.MkdirAll(task.WorkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	owner, _ := json.Marshal(execenv.EnvRootOwner{WorkspaceID: task.WorkspaceID, TaskID: task.ID})
	if err := os.WriteFile(filepath.Join(envRoot, ".task_owner"), owner, 0o600); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(envRoot, "codex-home", "sessions", "2026", "10", "14")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "rollout-2026-10-14T04-02-22-"+task.SessionID+".jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return task, root, envRoot
}

func TestResolveResumeLaunchDirectories(t *testing.T) {
	task, root, envRoot := resumeFixture(t)
	launch, err := resolveResumeLaunch(task, "codex", root)
	if err != nil {
		t.Fatal(err)
	}
	if launch.CodexHome != filepath.Join(envRoot, "codex-home") || !reflect.DeepEqual(launch.Args, []string{"resume", task.SessionID, "--cd", task.WorkDir}) {
		t.Fatalf("launch: %#v", launch)
	}
	project := t.TempDir()
	task.DurableWorkDir = project
	launch, err = resolveResumeLaunch(task, "codex", root)
	if err != nil || launch.Directory != task.WorkDir {
		t.Fatalf("existing original worktree: %#v %v", launch, err)
	}
	if err := os.Remove(task.WorkDir); err != nil {
		t.Fatal(err)
	}
	launch, err = resolveResumeLaunch(task, "codex", root)
	if err != nil || launch.Directory != project {
		t.Fatalf("durable directory: %#v %v", launch, err)
	}
	task.WorkDir = project
	launch, err = resolveResumeLaunch(task, "codex", root)
	if err != nil || launch.CodexHome != filepath.Join(envRoot, "codex-home") {
		t.Fatalf("indexed in-place home: %#v %v", launch, err)
	}
	launch, err = resolveResumeLaunch(task, "claude", root)
	if err != nil || launch.CodexHome != "" || !reflect.DeepEqual(launch.Args, []string{"--resume", task.SessionID}) {
		t.Fatalf("Claude launch: %#v %v", launch, err)
	}
}

func TestResolveResumeLaunchRejectsMissingOrForeignState(t *testing.T) {
	task, root, envRoot := resumeFixture(t)
	for _, mutate := range []func(*resumeTask){
		func(run *resumeTask) { run.Status = "running" },
		func(run *resumeTask) { run.SessionID = "bad;command" },
		func(run *resumeTask) { run.WorkDir = filepath.Join(root, "missing") },
		func(run *resumeTask) { run.WorkspaceID = "00000000-0000-0000-0000-000000000001" },
		func(run *resumeTask) { run.SessionID = "00000000-0000-0000-0000-000000000001" },
	} {
		copy := task
		mutate(&copy)
		if _, err := resolveResumeLaunch(copy, "codex", root); err == nil {
			t.Fatalf("accepted invalid run: %#v", copy)
		}
	}
	owner, _ := json.Marshal(execenv.EnvRootOwner{WorkspaceID: "00000000-0000-0000-0000-000000000001", TaskID: task.ID})
	if err := os.WriteFile(filepath.Join(envRoot, ".task_owner"), owner, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveResumeLaunch(task, "codex", root); err == nil {
		t.Fatal("accepted foreign owner")
	}
}

func TestResolveResumeLaunchReusedRootRequiresConversationProvenance(t *testing.T) {
	task, root, _ := resumeFixture(t)
	old := task
	task.ID = "00000000-0000-0000-0000-000000000001"
	if _, err := resolveResumeLaunch(task, "codex", root); err == nil {
		t.Fatal("accepted unproven reused root")
	}
	if err := execenv.WriteManagedEnvProvenance(filepath.Dir(old.WorkDir), execenv.ManagedEnvProvenance{WorkspaceID: task.WorkspaceID, AgentID: task.AgentID, IssueID: task.IssueID}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveResumeLaunch(task, "codex", root); err != nil {
		t.Fatal(err)
	}
	task.AgentID = "00000000-0000-0000-0000-000000000002"
	if _, err := resolveResumeLaunch(task, "codex", root); err == nil {
		t.Fatal("accepted another agent's root")
	}
}

func TestAgentResumePrintAndActiveConversationGate(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("MULTICA_AGENT_ID", "")
	t.Setenv("MULTICA_TASK_ID", "")
	task, root, _ := resumeFixture(t)
	active := false
	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agents/" + task.AgentID + "/tasks":
			pages++
			if r.URL.Query().Get("before") == "" {
				w.Header().Set("X-Agent-Tasks-Next-Cursor", "next|page+1")
				_ = json.NewEncoder(w).Encode([]resumeTask{})
			} else {
				if r.URL.Query().Get("before") != "next|page+1" {
					t.Error("cursor was not escaped")
				}
				_ = json.NewEncoder(w).Encode([]resumeTask{task})
			}
		case "/api/agent-task-snapshot":
			rows := []resumeTask{}
			if active {
				other := task
				other.ID = "00000000-0000-0000-0000-000000000001"
				other.Status = "running"
				rows = append(rows, other)
			}
			_ = json.NewEncoder(w).Encode(rows)
		case "/api/runtimes":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": task.RuntimeID, "provider": "codex"}})
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)
	original := resumeLookupExecutable
	t.Cleanup(func() { resumeLookupExecutable = original })
	lookups := 0
	resumeLookupExecutable = func(name string) (string, error) { lookups++; return filepath.Join(root, "fake-codex.exe"), nil }
	cmd := &cobra.Command{Use: "resume"}
	registerAgentResumeFlags(cmd)
	cmd.Flags().String("profile", "", "")
	_ = cmd.Flags().Set("workspaces-root", root)
	_ = cmd.Flags().Set("print", "true")
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := runAgentResume(cmd, []string{task.AgentID, task.ID}); err != nil {
		t.Fatal(err)
	}
	if pages != 2 || lookups != 1 || !strings.Contains(output.String(), "codex_home") {
		t.Fatalf("print result: %d %d %s", pages, lookups, output.String())
	}
	active = true
	if err := runAgentResume(cmd, []string{task.AgentID, task.ID}); err == nil || !strings.Contains(cli.FormatError(err, false), "active Multica run") {
		t.Fatalf("active conversation must have visible guidance: %v", err)
	}
	if lookups != 1 {
		t.Fatal("active conversation reached executable lookup")
	}
	t.Setenv("MULTICA_TASK_ID", "task-context")
	if err := runAgentResume(cmd, []string{task.AgentID, task.ID}); err == nil {
		t.Fatal("accepted agent task context")
	}
}

func TestAgentResumeEnvironmentAndInteractiveArguments(t *testing.T) {
	got := resumeEnvironment([]string{"PATH=fake", "MULTICA_TOKEN=owner-secret", "MULTICA_TASK_ID=task", "CODEX_HOME=old"}, "new-home")
	if !reflect.DeepEqual(got, []string{"PATH=fake", "CODEX_HOME=new-home"}) {
		t.Fatalf("child environment: %#v", got)
	}
	launch := resumeLaunch{Executable: filepath.Join(t.TempDir(), "fake '$(whoami) codex.exe"), Args: []string{"resume", "01a136db-d24d-7533-a0a0-b646d5ee0439"}}
	cmd := interactiveResumeCommand(context.Background(), launch)
	if strings.Contains(cmd.Args[0], "powershell") && !strings.Contains(strings.Join(cmd.Args, " "), "fake ''$(whoami) codex.exe'") {
		t.Fatalf("Windows literal quote lost: %#v", cmd.Args)
	}
}

func TestAgentResumeFakeInteractiveProcess(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "work dir")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "fake-codex"
	script := "#!/bin/sh\n[ -z \"$MULTICA_TOKEN\" ] || exit 91\nprintf '%s\\n' \"$*\" \"$PWD\" \"$CODEX_HOME\"\nread -r input\nprintf '%s\\n' \"$input\"\n"
	if runtime.GOOS == "windows" {
		name = "fake codex.cmd"
		script = "@echo off\r\nif defined MULTICA_TOKEN exit /b 91\r\necho %1 %2\r\necho %CD%\r\necho %CODEX_HOME%\r\nset /p resumed_input=\r\necho %resumed_input%\r\nexit /b 0\r\n"
	}
	executable := filepath.Join(root, name)
	if err := os.WriteFile(executable, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	launch := resumeLaunch{Executable: executable, Args: []string{"resume", "01a136db-d24d-7533-a0a0-b646d5ee0439"}}
	cmd := interactiveResumeCommand(context.Background(), launch)
	cmd.Dir = directory
	cmd.Env = resumeEnvironment(append(os.Environ(), "MULTICA_TOKEN=do-not-inherit"), filepath.Join(root, "codex-home"))
	cmd.Stdin = strings.NewReader("manual-input\n")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("fake interactive process: %v: %s", err, output.String())
	}
	for _, want := range []string{strings.Join(launch.Args, " "), directory, filepath.Join(root, "codex-home"), "manual-input"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q in %q", want, output.String())
		}
	}
}
