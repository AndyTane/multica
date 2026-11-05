# CLAWSTA-143 R1 implementation and verification

**Goal:** Implement the architect-owned timeout settings contract in `2026-11-05-clawsta-143-timeout-settings.md` without changing its acceptance requirements.

**Architecture:** Persist complete policies in the existing JSON containers, atomically merge settings patches, resolve one immutable policy for both claim endpoints, and execute with per-run options. Explicit capability negotiation prevents unsupported clients from starting policy-controlled runs.

**Tech stack:** Go, PostgreSQL/sqlc, React, TanStack Query, Zod, Vitest, isolated native test subprocesses.

## Implementation steps

1. Add the typed policy and strict field validation in `server/pkg/agent/execution_timeouts.go`. Verify required fields, zeroes, integer/duration overflow, optional shell defaults and precedence.
2. Validate workspace and agent writes, preserve unrelated settings and legacy-client omissions in atomic SQL updates, regenerate sqlc, and verify persistence and authorization in the handler suite.
3. Add the shared claim resolver and capability gate. Verify single and batch claims, inherited and explicit policies, and recognizable upgrade failures for old clients. Preserve old-server behavior on new clients and reject malformed claim policies.
4. Apply immutable snapshots in the daemon, including OpenCode and Codex inactivity budgets. Verify real temporary subprocess completion after the old local window, termination at the new window, hard limits, tool activity, progress resets, environment mapping and subsequent claim refresh.
5. Implement the common editor in the existing workspace and agent General settings, retain translation/accessibility/permission patterns, verify save/inheritance/errors/provider fields, and capture the real component with production styles.
6. Run repository checks, review the final diff, and deliver a dedicated-branch PR for backend acceptance and frontend review. Do not merge, tag, publish, update stable or replace a deployed program.

## Candidate version

The starting remote main and latest used tag were `03f57f4cf5292ef4c190a1ef96eccda50d344ab5` / `v0.2.13`. Application manifests advance to `0.2.14` for this candidate. No release tag or OSS artifact is created; the release coordinator must recheck unused version numbers and latest main before publishing.

## Behavioral notes for review

- Missing policy preserves client configuration. Opting in begins with the legacy values; choosing OpenCode “follow general” explicitly sends zero for the dedicated window.
- Clearing the reserved key uses a JSON null patch, which removes only that key. Omitted keys survive old-client writes. Other top-level JSON fields are merged atomically, including existing PR, gateway and compaction settings.
- Unsupported clients receive no runnable task for policy-controlled runs. Existing claim settlement conventions return an empty claim and persist a failed run with `agent_error.runtime_version_unsupported` and `execution_timeouts_client_upgrade_required` in the error.
- Workspace and agent responses advertise the read-only `execution_timeouts_supported` server capability. Legacy servers leave the policy editor read-only and retain client settings; storage of arbitrary JSON alone never implies execution support. An unbound runtime is not labeled as an outdated client.
- A run keeps its claim snapshot. A later claim gets the latest settings without restarting the daemon. No live timer is changed by a web edit.
- OpenCode’s optional shell duration changes its child-process default only. Explicit tool-call timeouts still take precedence. Other runtimes do not receive this environment mapping.
- Platform no-message monitoring and tool no-message windows are separate from engine tool execution limits. A zero general idle budget disables the platform watchdog group, following existing behavior.
- Codex’s semantic and first-response inactivity budgets derive from the chosen platform windows; zero/unlimited tool monitoring disables the redundant internal inactivity timer while the daemon retains applicable idle monitoring. Transport handshakes are unchanged.

## Evidence

The delivery comment and evidence archive contain actual check results, isolated component screenshots and subprocess logs. Temporary test PostgreSQL uses its own data directory and generated `.env.worktree`; it never opens the deployed database. The subprocess scenarios shorten time windows to seconds and are not a real forty-minute business compile.
