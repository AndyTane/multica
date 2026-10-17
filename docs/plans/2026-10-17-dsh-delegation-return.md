# DSH Delegation Return Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** An agent can delegate with an ordinary `@DSH` mention, finish its turn, and receive one platform-managed follow-up in the original thread when that DSH run completes, fails, or is cancelled. Claude and Codex use the same server-side contract.

**Architecture:** Reuse the existing mention queue, browser-owned DSH Web bridge, task results, and issue wakeups. At claim delivery, atomically register one-shot terminal-event wakeups for the exact delegation comments delivered to a DSH run. Resolve the caller from trusted comment/source-task records; never infer it from a model name, current session, or issue assignee. Ordinary non-DSH mentions and existing squad return routing retain their contracts.

**Tech Stack:** Go, Chi, PostgreSQL/sqlc, existing issue-wakeup scheduler, Node.js HTTP/WebSocket DSH bridge tests.

---

## Accepted design and boundaries

- Keep DSH execution independent of its caller. Do not keep caller-owned shell processes alive or add another background service.
- Use `session/follow` and `session/page`; do not read DSH private archives or change A2AHandoff task history.
- Bind each return to the actual Multica DSH run, caller agent/source run, and original comment thread. Different callers and coalesced inputs must retain their own return destinations.
- Register returns in the same transaction as claim authorization, credentials and delivered-comment receipt. A failed claim must leave neither credentials nor wakeups behind. A reclaimed delivery must not duplicate or rearm an existing return.
- Preserve membership/invocation checks, issue lifecycle, one-shot deduplication and existing wakeup joining behavior. Do not wake the caller for intermediate DSH messages.
- Let the saved wakeup own delegated DSH failure delivery so the existing generic failure-recovery path does not create a second recovery run.
- No frontend/API shape changes or database migrations are expected. The existing DSH agent/runtime setup remains the deployment prerequisite.
- Use fake agents and fake DSH hosts for all default tests. Do not send prompts to real agents or restart the running deployment.

## Task 1: Register durable returns at DSH claim delivery

**Files:** `server/internal/service/dsh_delegation.go`, adjacent tests, `server/internal/handler/daemon.go`, `server/pkg/db/queries/wakeup.sql`, generated sqlc files.

1. Add failing cases for Claude and Codex callers whose original runs have ended, then claim the separately queued DSH run.
2. Add SQL lookup of an existing return by workspace, issue, exact worker run, caller source run and parent comment.
3. Implement a transaction helper that validates delivered explicit mentions and trusted source identities; registers one-shot `task.completed`, `task.failed`, `task.cancelled` wakeups; and records normal wakeup activity.
4. Invoke the helper from the shared single/batch claim-delivery authorization path and publish activity only after successful commit.
5. Regenerate sqlc and verify caller isolation, coalesced comments, reclaim idempotency, permissions and transaction rollback.

## Task 2: Deliver results once across terminal outcomes

**Files:** `server/internal/service/task.go`, `server/internal/handler/dsh_delegation_test.go`, `server/internal/dshweb/bridge.test.mjs`.

1. Exercise completion after the caller has exited, failure, cancellation, repeated terminal processing, and a caller with a queued follow-up.
2. Exclude tasks owned by a saved DSH return from generic delegated-failure recovery; retain ordinary-agent failure behavior.
3. Add a fake-host delayed-completion bridge regression and retain the existing request-id recovery and v4 event API tests.
4. Verify a result inspection prompt refers to the exact worker run and does not automatically redispatch failed work.

## Task 3: Document the supported workflow and verify

**Files:** `server/internal/service/builtin_skills/multica-platform/references/mentions.md`, `docs/2026-10-14-multica-DSH-网页接入.md`, this plan.

1. Document the exact mention syntax, automatic return contract, supported callers and setup prerequisite.
2. Run scoped database-backed handler/service tests in an isolated checkout environment, DSH fake-host tests, affected Go compilation/vet, and `git diff --check`.
3. Record actual checks and any deployment or real-model validation still outstanding. Keep the running daemon and DSH host untouched.

## Acceptance scenarios

| Scenario | Expected behavior |
| --- | --- |
| Claude or Codex mentions DSH, then ends | Independent DSH run completes; one return targets the original agent and thread |
| DSH takes longer than the caller | Caller process cleanup cannot end the DSH run or erase its saved return |
| DSH posts progress | No return until the tracked run reaches a terminal state |
| DSH fails or is cancelled | One return with the actual terminal status; no automatic prompt resend |
| Two callers delegate to DSH | Each delivered delegation has its own caller/thread binding |
| Claim fails or is reclaimed | Rollback leaves no rule; successful reclaim reuses the original rule |
| Rule is consumed/disabled, caller unavailable, issue closed | Existing lifecycle and authorization rules apply; no rearming behind the user's back |

## Progress

- Implemented automatic direct-agent returns in the shared single/batch claim-delivery path. The caller identity comes from authenticated comment/source-task records and current invocation permissions.
- Bound each return to the exact delegation comment inside its existing thread and the exact DSH run. Claim rollback removes both the rule and credentials; reclaim reuses a saved rule, including a disabled one.
- Existing wakeup terminal-event capture and dispatch provide completion/failure/cancellation delivery. DSH-owned returns suppress a second generic delegated-failure recovery signal.
- Updated runtime instructions, repository workflow instructions, the platform mention reference and DSH setup documentation. No frontend/API response shape or migration changes.

## Verification

- The six handler scenarios (Claude/Codex × completed/failed/cancelled) failed on the unchanged checkout because no return was registered. They pass with this change, including a caller that has ended, progress that does not wake it, and preservation of the original comment thread.
- `go test ./internal/handler ./internal/service -run '^TestDSHDelegation' -count=1 -v`: all new scenarios pass. Service cases also cover claim-finalization rollback, reclaim without rearming a disabled rule, separate coalesced callers and ten rejected input/permission/lifecycle cases.
- `pnpm test:dsh-web`: 39/39 pass, including independent held-worker completion after another run finishes. All hosts and agents are simulated.
- `go test ./internal/daemon/execenv -run '^TestInjectRuntimeConfigMentionLoopHardening$' -count=1 -v`: passes.
- `go vet ./internal/handler ./internal/service ./internal/daemon/execenv ./cmd/multica-dsh-web`: passes.
- sqlc regenerated with the repository-pinned 1.31.1 release. The official Windows binary's SHA-256 digest was verified; `make` is unavailable on this host and source-tool dependency downloads were slow.
- Full tests for those four Go packages were attempted. The DSH command package passes. The Windows checkout has additional failures in path handling, CRLF-sensitive builtin-skill parsing, Unix file-permission assumptions, and OpenClaw home/cache tests (one panics). The unchanged checkout reproduces both handler path failures, all three builtin-skill failures, and the two Codex file-permission failures. OpenClaw failures were not individually rerun. These are outside the delegation change.
- Related existing handler/service regression suite and final diff check: pending final run.

## Delivery limits

- Development used a managed isolated checkout and its own migrated database on the known native PostgreSQL host. The API startup command could not finish because `make` is unavailable; HTTP handlers were tested directly in process against that isolated database.
- No real Claude/Codex/DSH prompts were sent. Frontend/mobile/E2E suites were not run because this change has no client code or API shape changes.
- The running native API, daemon and DSH web host were not replaced or restarted. New server code and daemon runtime instructions must be deployed before the automatic return is available to new delegations.
