# Context Compaction Implementation Plan

> **For Codex:** Use executing-plans to implement the approved G requirements task by task.

**Goal:** Automatically compact Claude/Codex conversations above 80,000 context tokens before their process exits, with a pre-task guard for oversized resumed sessions and per-agent settings.

**Architecture:** Preserve each provider's session and long-lived transport. The daemon passes the per-agent policy stored in the existing `runtime_config.context_compaction` JSON. Claude sends `/compact` over its open stream; Codex calls `thread/compact/start` and waits for the associated terminal event. Maintenance events never replace the task deliverable. Warm maintenance failure retains the completed task with a visible warning; cold maintenance failure stops before submitting the new task. Claude attempts have a ten-minute bound starting after SDK initialization; Codex keeps two minutes. Both respect cancellation.

**Tech Stack:** Go agent backends and fake protocol processes, existing agent REST/config JSON, Zod, shared React agent settings, Vitest.

## Decisions

- C is skipped by user instruction. Existing 200,000-token native settings stay in place until real verification succeeds.
- Native threshold changes alone cannot compact between runs. Implement the requested warm compaction plus pre-task guard; avoid a second process after completing a task because it loses the warm connection.
- Use existing runtime configuration instead of new database columns. Absent settings mean enabled with threshold 80,000; the setting only applies to Claude/Codex. Validate writes; tolerate malformed historical responses.
- Measure the latest model context, including cached input. Cumulative billing counters must not determine the threshold. Bounded reads of existing logs supply resumed-session context when live notifications do not.
- Compaction must finish before task completion releases the conversation for another run. Keep the original result, report maintenance separately, disable supplements during maintenance, and expose the authoritative terminal boundary to watchdogs.
- Real-model tests require separate explicit authorization under AGENTS.md. Default tests use test-created executable paths and temporary homes only.

## Task 1: Policy, context measurement and API validation

Create `server/pkg/agent/context_compaction.go` and colocated tests. Cover defaults, enabled/disabled policy, integer limits, cache-inclusive latest usage, compaction boundaries, malformed logs and bounded tail reads. Validate `runtime_config.context_compaction` in existing create/update handlers; pass its decoded policy in `server/internal/daemon/daemon.go` execution options. Add a core Zod helper and malformed-response tests without changing existing JSON fields or unrelated configuration.

## Task 2: Codex maintenance lifecycle

Add `server/pkg/agent/codex_context_compaction.go` and tests. Route maintenance notifications separately from the task turn, match thread/turn identity, wait for completion after the RPC acknowledgement, and include usage without counting resumed history. Integrate pre-task and post-task calls in `codex.go`, retaining the task's original output/status. Verify RPC errors, timeouts, cancellation, late/unrelated events and persisted-session measurement with fake protocols.

## Task 3: Claude maintenance lifecycle

Add `server/pkg/agent/claude_context_compaction.go` and tests. Keep stdin open for `/compact`, wait for the boundary/result, then either send the original task (cold) or close input (warm). Suppress maintenance summaries and guard completed results during bounded cleanup. Test multiple requests in one process, usage, original replies, errors, timeouts and cancellation using a fake executable. Keep initialization outside the compaction budget and cover production-scale duration requirements.

## Task 4: Shared settings

Add a context-compaction settings component to `packages/views/agents/components/agent-overview-pane.tsx` for Claude/Codex. Default enabled; threshold 80,000 tokens, configurable from 1,000 to 2,000,000. Preserve unrelated runtime-config keys. Explain summarization, automatic triggering and model usage briefly, with all five translations. Component tests own save/error/access wiring; core tests own the validation matrix.

## Task 5: Verification and delivery

Run targeted Go/fake-protocol tests, vet, relevant core/views tests, type checks and changed-file lint. Prepare gated `agentintegration` tests for dedicated temporary test sessions without project changes, but do not run until authorized. Build CLI to an alternate filename, build web/backend images, deploy preserving data, and replace the idle daemon. Record actual results and remaining validation limits, update the stage plan to show C skipped, and commit scoped changes locally on `tane/main`.

## Delivery

Tasks 1–5 are complete. Backend: `ebdc53a9b`; shared settings: `608936d8d`. The user explicitly authorized real Claude/Codex compaction verification in this session. Both providers retained their session and remembered the first message after two compactions; Claude's quoted reply was checked against its native log after correcting the test comparison. The final CLI and both application images are deployed locally, with the original PostgreSQL container retained. The default handler database suite was unavailable; online API validation covered both write paths. Existing Windows fake-command failures prevented a green full agent suite. Details are in [the G delivery record](../2026-10-15-multica-上下文自动压缩.md).

Those real checks covered small sessions. A later production conversation exposed the two-minute Claude limit; its native history recorded a normal compaction taking over seven minutes. The budget and initialization boundary are corrected with fake regression coverage. The frontend agent temporarily keeps the new policy disabled, retaining native compaction; a real large-session revalidation remains pending because the user has already recovered the conversation and did not authorize another retry.
