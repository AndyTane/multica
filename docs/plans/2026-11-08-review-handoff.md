# Review handoff implementation plan

**Goal:** Make a complete, atomic handoff reachable from the runtime brief, and preserve the awaiting-review feature in the PR targeting tane/main.

**Architecture:** Reuse UpdateIssue's existing transaction and review validation. Expose a dedicated POST /issues/{id}/review route so old servers reject the operation rather than ignoring review fields. A CLI command reads revision and submits once; existing web PUT behavior remains unchanged.

**Tech stack:** Go, Cobra, Chi, PostgreSQL; existing Next.js awaiting-review view.

1. Integrate origin/tane/main into the feature branch; retain all three awaiting-port commits. Do not merge the PR without Tane's approval.
2. Add `server/internal/handler/issue_submit_review.go` and its atomicity tests. Require an action, reviewer and revision; reuse transaction validation and status side effects.
3. Add `server/cmd/multica/cmd_issue_review.go` and command tests. Cover approve/reply/act, invalid inputs, conflict and unsupported-server failures without fallback.
4. Update `server/internal/daemon/execenv/runtime_config_sections.go` and the built-in skill's review instructions. Explain user, specialist and dispatcherless execution cards together with status rules.
5. Run CLI/execenv tests and targeted handler review tests against an isolated database; run affected frontend checks after branch integration. Build a new native release and a stamped CLI.
6. Open a PR to tane/main. Verify idle state, preserve backup and rollback, and perform the authorized local cutover. Verify a real user handoff and capture light/dark screenshots; request frontend acceptance with the concrete evidence.

UWorkers decision: do not inject a command its shipped CLI does not yet support. Port the complete command, endpoint and brief together in a separate versioned delivery after Multica acceptance; no UWorkers production change in this batch.

## R4 parent-sync completion

Port the daemon/CLI subset of `53e78e3e5` onto `676f67d66`, preserving current upstream fixes and the atomic review command. Do not wholesale copy the UWorkers source branch. Audit both requested directories and classify unrelated deployment/branding changes separately.

1. Restore parent-sync task JSON fields, parent-first reading and single/multiple-thread reply instructions in `daemon/types.go`, `prompt.go`, and `execenv/reply_instructions.go`. A sync marker does not prove a pure sync: ordinary obligations still require replies; only a successful server evaluation permits silence.
2. Restore `issue activity ... no_action --reason`; add explicit `comment add --sync-parent-assignee` using the existing server contract. Default comments remain unchanged. Keep concise runtime guidance alongside the current workflow.
3. Test JSON-to-prompt routing, parent-before-child ordering, distinct-thread replies, normal-comment isolation, no-action request validation/API failures, and explicit sync request behavior. Re-run existing prompt/reply/review tests.
4. Commit and push the same PR. Build a stamped CLI, install for idle auto-reload, and verify all online runtimes have upgraded before enabling `MULTICA_PARENT_SYNC_AGENT_DELIVERY`. Preserve config backup and rollback; any service start uses WMI outside the agent process tree.
5. With the upgraded daemon, sync a genuine sub-issue update to its parent owner and retain source comment/task/parent-read/thread-reply evidence. Delegate light/dark UI validation only once the feature is live. Retain the separate awaiting-review screenshot requirement; do not invent a card or a dispatcher to satisfy it. Never merge without Tane's authorization.
