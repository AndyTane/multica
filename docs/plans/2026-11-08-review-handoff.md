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
