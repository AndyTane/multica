# Multica awaiting review port implementation plan

**Goal:** Port the reviewed UWorkers PR #48 web and server feature onto the deployed Multica base fda0c5852 without schema changes.

**Architecture:** Keep the original review validation, atomic approve/undo and final feedback delivery checks. Store the review entry time in metadata.review_since. All application status writes maintain that key and remove every review_* key when leaving in_review; old cards use updated_at. CLI, daemon, desktop, branding and version manifests stay outside this patch.

**Tech Stack:** Go/pgx/sqlc, PostgreSQL, TypeScript/React/Next.js, TanStack Query, Vitest, Playwright.

## A1: Isolate and port

- Use the separate worktree D:/01_Projects/multica-awaiting-port and branch clawsta-151-awaiting-port, based on fda0c5852.
- Apply only feature deltas in packages/core, packages/views, apps/web, handler/comment.go, handler/issue*.go, issue SQL and the built-in issues skill.
- Keep every migration byte-identical to the deployed base. Adapt SQL review lifecycle for UpdateIssue, status updates and PR automation; regenerate sqlc.
- Cover entry timestamp, no-op preservation, all review_* cleanup, legacy sorting fallback, approve/undo, send back, revision conflicts and final recipient validation.

## A2: Verify and build

- Run affected Go tests against a new isolated native database, affected core/views/web tests, typecheck and lint. Record command exit codes and logs in evidence/clawsta-151/port/.
- Commit the feature branch. Build with native-windows.ps1 using an isolated deployment config and a new release path; never run Build against the live config because it rewrites deployment.json and the runtime script.
- Take a consistent supported Backup of the live deployment, restore into a new database and launch only the isolated application on other ports. Disable copied dispatch/wakeup/integration side effects before starting it.
- Mark copies of current in_review cards: 143 and 149 user, 148 specialist, preserving the R3 action text. Capture light/dark list, count, board, send back and approved toast using the actual release API/web.
- Stop the isolated application before returning. Attach a sanitized evidence ZIP and checksum, branch/commit, file list, test results, release identity and concrete rollback commands. Mention the front-end owner for A acceptance.

## B: Explicit acceptance gate

Only after the front-end owner accepts A: check workspace-wide idle state, record runtimes/wakeups/current task, register the 20 minute recovery wakeup, Stop -KeepDatabase, Backup, switch Release and Start using the deployed lifecycle. Verify API/web/login, CLI, all runtimes and wakeups, run a one-minute wakeup test, and check failed tasks. Roll back on any failure. Then mark only the three still-in_review live cards, review_by last, capture the real count/list and post B evidence.
