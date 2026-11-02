# Portable invitation R2 implementation plan

**Goal:** Give only a newly installed operator instance administration and let colleagues enter their joined workspace without reloading onboarding.

**Architecture:** Keep the existing portable installer and auth store. Return the operator UUID and creation result from the same database transaction; call the existing host-local `grant-admin` only for that new identity. After joining, refresh the account and workspace cache before routing. Keep the successful destination in component state so a failed refresh can retry without joining again.

**Tech stack:** Windows PowerShell 5.1/7, PostgreSQL, existing Go local-auth tool, Next.js, Zustand, TanStack Query, Vitest, Playwright.

## Design decisions

- Prefer the insert transaction's creation result over an install-report marker: interrupted installs and pre-existing accounts must not regain revoked permission. Keep adoption and upgrade paths unchanged. A failed initial grant stops installation; a later run finding an existing account requires explicit host authorization using the documented command.
- Prefer `refreshMe` and a required workspace-list fetch over fabricating `onboarded_at` or relaxing the layout guard. A successful join is distinct from a failed account/cache refresh. Retry only synchronization, preserving the invite's target, including for existing members.
- Keep current join-page language and styling. Add only an actionable synchronization error and retry control; no SMTP settings or invitation redesign.

## Implementation and verification

1. Add focused installer regressions in `deploy/portable/test/account-initialization.test.ps1`: new versus existing identity, exact grant target, grant failure, missing/ambiguous database result. Run under both PowerShell versions; verify failure before implementation.
2. Update `deploy/portable/install.ps1`: return `{ UserId, Created }` atomically, grant only when created, validate tool and command status. Run installer regression suites. In an isolated portable installation verify new admin, revoked permission after Setup/resume/upgrade, and unaffected colleagues.
3. Extend `apps/web/app/join/page.test.tsx`: delayed refresh before navigation, failed refresh and retry without a second join, workspace-list failure, existing-member target. Update `page.tsx`, run page/layout tests, lint and typecheck, then build.
4. Repeat the R1 browser scenario against the rebuilt isolated web app: activate a new colleague, join without refresh or personal workspace, see a fixture task, reload, and inspect one persisted membership. Inject a single identity-sync failure to test visible recovery.
5. Update `docs/windows-portable.md` with a tested v0.2.1 host procedure using the recorded UUID, `local-auth list`, explicit `grant-admin`, command exit checks and re-login. Record only R2 results under `evidence/clawsta-118/r2/`; preserve R1 history.
6. Inspect screenshots and credential redaction, stop isolated services, run `git diff --check`, commit named files, push the existing PR #22 branch, and rewrite the PR around both fixes and verification. No merge, release or company deployment changes.
