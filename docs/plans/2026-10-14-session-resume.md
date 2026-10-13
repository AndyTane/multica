# Session Resume Implementation Plan

> **For Codex:** Use the executing-plans skill to implement the approved plan task-by-task.

**Goal:** Complete B by showing Claude/Codex session IDs and copying a command that safely opens the original CLI conversation on the runtime's computer.

**Architecture:** A small `multica agent resume <agent-id> <task-id>` CLI command retrieves the existing authorized run history, validates that the conversation is idle, resolves local directories, and opens Claude/Codex interactively. Runtime registration advertises the local Multica executable, profile and workspace root; the shared UI validates this optional metadata before copying the command. Codex's task home is resolved on the local computer from existing ownership markers and the task root index, avoiding guesses from remote paths or exposing account credentials.

**Tech Stack:** Go/Cobra, existing REST APIs, TypeScript/Zod, shared React run details, Vitest.

---

### Task 1: Local resume resolution and CLI

**Files:** `server/internal/daemon/execenv/root_identity.go`, new `server/cmd/multica/cmd_agent_resume.go` and colocated tests, `server/pkg/agent/agent.go` if a public interactive command factory is needed.

1. Add a read-only task-root lookup using the existing index validation.
2. Test directory resolution for managed/reused runs, local project runs, removed worktrees, missing logs and mismatched ownership, using temporary directories.
3. Implement the human-only CLI: read authorized task pages, reject active conversations and unsupported/malformed runs, resolve the provider and working directory, set only the required Codex home, then invoke the native CLI with inherited interactive stdio. Add `--print` to inspect the resolved launch without starting an agent.
4. Test API pagination and request validation with a fake HTTP server, and command construction with fake executable paths. Run only these Go tests; default tests never look up installed agent CLIs.

### Task 2: Copyable launcher metadata

**Files:** `server/internal/daemon/daemon.go`, `server/internal/handler/daemon.go`, new shared `packages/core/agents/session-resume.ts` and tests.

1. Register optional, non-secret recovery metadata: local Multica executable, daemon profile, workspaces root and OS.
2. Build a quoted PowerShell/POSIX command after Zod validation. Reject missing/malformed metadata, unsupported providers, unsafe IDs and unfinished runs. Older daemons retain session-ID copying without a fabricated recovery command.
3. Test spaces, quotes and shell metacharacters as literal arguments; do not render absolute local paths in UI text or tooltips.

### Task 3: Shared run details

**Files:** `packages/views/common/task-transcript/agent-transcript-dialog.tsx` and tests, five supported `agents.json` translations.

1. Add the session ID and copy-recovery action to the existing details popover; keep the DSH entry.
2. Explain that the copied command runs on the task's computer and requires an idle conversation. Use visible copy feedback and an explicit unavailable state for older daemons.
3. Verify wiring, accessibility, failure feedback and long-ID layout in the component suite; run core/views/web type checks and changed-file lint.

### Task 4: Local delivery

1. Build the CLI, backend and web images, deploy through the existing self-host Compose setup and restart an idle daemon while preserving its configuration.
2. Run `--print` for existing Claude/Codex tasks to verify the real paths without invoking models; verify runtime metadata and service health.
3. Update the B progress and usage guide, record actual checks and limits, and commit locally on `tane/main`. C/G and the recorded DSH steering boundary remain subsequent work.
