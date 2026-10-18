# Context Compaction Status Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the existing waiting label with context compaction activity while maintenance is actually running.

**Architecture:** Emit dedicated maintenance lifecycle messages over the existing task-message transport. Preserve them in the transcript cache, derive the latest state by sequence, and keep them out of agent replies, tool counts, and technical task outcomes. Use existing status styling and localized copy; no percentage estimates or new task status enum.

**Tech Stack:** Go agent/daemon, Zod, TanStack Query, React, React Native, Vitest.

---

### Task 1: Carry maintenance lifecycle events

- Modify `server/pkg/agent/context_compaction.go`, `codex_context_compaction.go`, and `claude.go` to emit started/completed/failed status messages with readable fallback content.
- Extend `server/internal/daemon/daemon.go` to persist only these statuses using the existing transcript batch; retain session pinning.
- Extend the existing fake Codex/Claude process tests and add a daemon transcript regression. Verify cold/warm success, timeout, failure, disabled compaction, and unchanged task output.

### Task 2: Parse and derive the display state

- Extend core and mobile task-message schemas with the maintenance message types and retain unknown-type fallback behavior.
- Add a pure helper in `packages/core/agents/context-compaction.ts`: use sequence ordering, clear on completion/new task activity, and separate failure from running.
- Update timeline types and step construction to retain maintenance evidence without counting it as agent work.
- Add malformed-response and state-transition tests.

### Task 3: Display and verify

- Reuse the existing waiting indicator/running badge in `agent-transcript-dialog.tsx`; integrate inline run and chat status labels with the same state helper. Do not add a separate banner, progress bar, or failure panel.
- Mirror chat status in mobile and keep maintenance events out of process-step counts. Check all five supported translations.
- Add component tests for live start, completion, failure, terminal runs, reply-preserving warm maintenance, and Chinese copy.
- Run targeted Go/Vitest tests, frontend/mobile type and lint checks, and diff checks. Inspect a fake-data UI preview where available; no real-agent calls or deployment.

### Delivery

- Keep this change on the existing `codex/codex-compaction-wait` branch with the related timeout fix.
- Commit scoped code and report verification limits. Do not replace the running daemon as part of this UI change.

### Verification results

- Fake Codex/Claude compaction process regressions and daemon transcript reporting passed; no installed agent CLI or real model was used.
- Targeted frontend checks: core 31 tests, views 184 tests, mobile schema 4 tests passed. The existing compaction-settings test fixture was completed after the repository typecheck rejected its partial `Agent` cast.
- ESLint passed for changed core, views, and mobile TypeScript files. Mobile typecheck passed.
- Repository `pnpm typecheck` passed for all 10 packages, including web and desktop, after completing the existing test fixture.
- Existing component geometry and waiting animation are reused. Browser/native-device visual checks and live deployment were not performed.
