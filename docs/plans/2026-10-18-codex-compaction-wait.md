# Codex Compaction Wait Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow normal multi-minute Codex compaction and identify maintenance timeouts without losing the existing session.

**Architecture:** Keep the current session, transport, completion notifications, cancellation, and caller deadline. Increase the default Codex maintenance budget to ten minutes. Report timeout and cancellation using bounded local descriptions rather than copying provider error payloads.

**Tech Stack:** Go, existing fake Codex protocol, Markdown documentation.

---

### Task 1: Reproduce the insufficient budget and generic failure

Modify `server/pkg/agent/context_compaction_test.go` and `context_compaction_process_test.go`. Require a ten-minute production budget and an explicit timeout result with the preserved session and no submitted task. Run `go test ./pkg/agent -run 'TestContextCompactionProviderBudgets|TestCodexContextCompactionProcessPreservesWorkAndSession/timeout' -count=1` from `server/`; confirm both regressions fail.

### Task 2: Correct waiting and diagnostics

Modify `server/pkg/agent/context_compaction.go`, `codex_context_compaction.go`, and `codex.go`. Preserve explicit timeout overrides and caller cancellation. Distinguish the maintenance deadline from the overall task deadline. Keep completed work intact after post-task maintenance failures. Cover safe error descriptions in `codex_context_compaction_test.go`.

### Task 3: Verify and document

Run the targeted compaction tests, then the agent package tests and vet. Update the current waiting policy in `docs/2026-10-15-multica-上下文自动压缩.md` and the original context-compaction plan. Run `git diff --check`. Use fake executables only; no real model calls, live session retries, host restarts, or daemon replacement are part of this code change. Commit the scoped fix with a conventional message.

### Validation

- Both focused regressions failed before the implementation and passed afterward. The broader compaction tests passed, as did `go vet ./pkg/agent` and `git diff --check`.
- The full agent package is not green on Windows. It includes unsupported shell fixtures, Unix permission expectations, and model-catalog discovery failures. The Antigravity executable fixture, execution-error shell expectations, off-Windows browser configuration expectation, Codex service-tier catalog test, and Codex managed-config permission test also fail on the unchanged checkout. No compaction test failed in the full run.
- Real-agent integration tests and live-session recovery were not run.
