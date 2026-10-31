# CLAWSTA-77 P5-R1: concentrated fixes (candidate)

Branch `agent/dsh/p5-registration`, PR #7. Continuation range for this batch:
`46814feec..16db41e34` (PR base `8c61cdef`). This is a candidate branch: the
live deployment was not replaced or restarted, HTTPS timer and the Tane account
were not touched, and no Multica/DSH software was operated.

| Commit | Scope |
| --- | --- |
| `f7ddb6e09` | Backend: session authority, verified email identity, account consistency, delete preview, host bootstrap |
| `16db41e34` | Frontend: settings admin tab, registration/password fixes, contracts, five-language copy |
| this commit | Evidence and task-book sync |

## Isolation

- Fresh database `multica_p5candidate` on the desktop PostgreSQL cluster, reached
  through a loopback SSH tunnel; `multica_clawsta80` was never opened for writes.
  Migrations applied through `582_user_email_verified`.
- Candidate API (`18082`), web build (`13003`) and Caddy (`13444`, `tls internal`)
  on this workstation. The staging CA is trusted in the current-user root store,
  so the browser ran with NORMAL certificate validation (`ignoreHTTPSErrors`
  never set).
- Test identities use `@staging.invalid`; passwords and invitation secrets were
  generated per run and existed only in memory, the browser and a scratch env
  file that was deleted at teardown. Nothing in this directory contains a real
  email, password, invitation, token or cookie.

## Closed findings

Backend review (`p5-review-result.md`, items 1-6):

1. Session authority covers the WebSocket handshake for header, cookie and
   first-frame credentials, plus existing connections: handlers close that
   account's sockets on password change and account deletion, and a 60s
   revalidation loop closes a connection whose account or auth_version moved
   without this replica observing the change. Other accounts keep their
   sockets (browser test + `internal/realtime` tests). The PAT cache checks a
   deleted-account marker on every hit and account deletion refuses to complete
   if the marker cannot be written, so a cached PAT cannot resurrect a deleted
   user.
2. Open self-registration creates an unverified address (migration 582, default
   true for existing rows). Workspace invitations are only listed, accepted,
   declined or settled by email for a verified account; UUID-bound invitations
   and existing verified logins are unchanged. Email-code and Google
   find-or-create paths still produce verified accounts by construction.
3. Account writes share one lock order (`instance_admins` before
   `user:<uuid>`): administrator deletion, invitation regenerate/revoke/redeem
   and password rotation re-read every condition inside the transaction.
   Regeneration revokes before minting under the account lock, so only the
   latest invitation is valid; redemption consumes once; a password change
   accepts the old credential exactly once and password login cannot mint a
   new-version token from an old password racing a change. Deletion re-runs the
   dependency inventory after cleanup and rolls back rather than leave an
   orphan.
4. A non-UUID administrator target is a 400, not a panic. The delete inventory
   covers the real schema (workspace/runtime/agent ownership, channel and Lark
   installations, share links, captured source context are blockers; identity
   bindings and account-scoped rows are removed; shared history keeps its rows
   with nullable attribution detached). `GET
   /api/admin/users/{id}/delete-preview` returns exactly
   `{can_delete, blockers:[{code,count}], removes:[code]}` from the same
   function DELETE enforces, admin-only, machine credentials refused.
5. `registration_invite` and `instance_setting` are KEEP in the
   workspace-deletion manifest; the real-schema manifest test passes.
6. `local-auth invite` withholds the raw secret unless `--stdout-token` is
   passed. New `bootstrap-host` helper: explicit operator UUID + pinned email
   digest, 15-minute invitation, existing password only opens sign-in, opens the
   private HTTPS setup page, and never prints, logs or writes the secret.

Frontend review (`p5-r1-frontend-review.md`, F1-F8, A1-A6, sections 3-4): all
closed. Instance user management moved into Settings as an admin-only tab shared
by web and desktop; `/admin` is removed rather than redirected (it was never in
a release and the settings route is workspace-scoped); the account tab button is
gone; registration mode waits for config, always offers the paste field,
understands a full link or a bare token, strips the fragment, and completes via
Enter with the password rule visible; open registration is a Switch that rolls
back on failure; invitations are full HTTPS links with copy and select fallback;
delete uses an AlertDialog fed by the preview and stays disabled on
failure/malformed/blocked; the password change form clears three fields on
failure and shows a one-shot localized notice after logout; new APIs have zod
schemas, parseWithFallback and malformed-response tests; errors map through one
code->i18n helper; `register` is a reserved slug; all new copy exists in
zh-Hans/en/ja/ko/fr with parity coverage.

## Tests actually run

Go (candidate database, real PostgreSQL):

| Command | Result |
| --- | --- |
| `go build ./...` | pass |
| `go vet ./...` | pass (empty log) |
| `go test ./internal/handler -run 'TestP5' -count=1` | pass (39.3s) |
| `go test ./internal/handler -run 'TestWorkspaceDeletionManifest' -count=1` | pass |
| `go test ./internal/realtime -count=1` | pass (1.1s) |
| `go test ./internal/auth -count=1` | pass (3.3s) |
| `go test ./cmd/local-auth -count=1` | pass (11.9s) |
| `go test ./internal/handler ./internal/realtime ./internal/auth ./cmd/local-auth -count=1` | realtime/auth/local-auth pass; **handler package not green** (see below) |

The full `internal/handler` package is not green in this Windows candidate
environment: 9 tests in unrelated modules fail and the package then hits the
10-minute timeout. The failures are pre-existing/environmental, not touched by
this batch:

- `TestStableIDSuffixMatchesDaemon` - Windows path separator vs the daemon's
  POSIX expectation.
- `TestAutopilotQuotaManualAndWebhookEnforcement`,
  `TestTriggerAutopilot_InternalFailureDoesNotEchoError`,
  `TestGetAutopilotRun_ReturnsFullPayload`,
  `TestWebhookHandler_*` (3), `TestPendingQueueHeadMatchesClaimForEqualCreatedAt`,
  `TestMigration467BackfillsTriggerCreatorFromAutopilot` - the dispatch path
  reports "agent runtime is offline at dispatch time" on this host.

All P5/P5-R1 tests, the manifest test and the other three packages pass.

Frontend:

| Command | Result |
| --- | --- |
| `pnpm --filter @multica/core typecheck` | pass |
| `pnpm --filter @multica/views typecheck` | pass |
| `pnpm --filter @multica/web typecheck` | pass |
| `pnpm --filter @multica/desktop typecheck` | pass |
| `pnpm --filter @multica/core test` | 173 files, 2224 tests pass |
| `pnpm --filter @multica/web test` | 25 files, 245 tests pass |
| `pnpm --filter @multica/desktop test` | 62 files, 652 tests pass; 1 pre-existing failure `scripts/package.test.mjs` (SyntaxError in a script test, unmodified) |
| `npx vitest run auth/registration-page.test.tsx settings/components/instance-users-tab.test.tsx settings/components/password-change-card.test.tsx locales/parity.test.ts` | 4 files, 233 tests pass |
| `pnpm --filter @multica/views lint` | 0 errors (20 pre-existing warnings elsewhere) |
| `pnpm --filter @multica/core lint` | 0 errors (2 pre-existing warnings) |
| `pnpm --filter @multica/web lint` | 0 errors (1 pre-existing warning) |
| `pnpm --filter @multica/desktop lint` | 0 errors (1 pre-existing warning) |
| `pnpm generate:reserved-slugs` | regenerated, `register` present |

A full `pnpm --filter @multica/views test` run reports 22 failures in 11 files
unrelated to this batch (autopilots, inbox, issues, search, mcp, code-vcs). Each
of those files passes when run alone; the failures only appear under the full
parallel run in this environment and none of them imports a changed file. They
are recorded here rather than called green.

## Real-browser acceptance

`browser/browser-evidence.json`: **28/28 steps pass** against the staging origin
with normal certificate validation, starting from clicks and keystrokes:

- config load does not flash a wrong registration mode; the invite field appears
  only after config resolves.
- admin signs in, opens Settings -> Users & registration, sees open registration
  off; a forced 500 on the switch rolls it back with an error; it can be enabled
  and disabled.
- adding a user returns a full `https://<origin>/register#token=` link; the copy
  button writes exactly that link to the clipboard.
- a fresh browser opens the login page, clicks Register, pastes the copied link
  and completes the form with Tab/Enter only -> onboarding.
- a second invitation opened directly from the fragment prefills and strips the
  fragment, then completes -> onboarding.
- an existing onboarded account receives a first password and lands in its own
  workspace.
- self row has no delete entry; the delete preview shows "Owns workspaces" with
  confirmation disabled; a dependency-free account deletes; the row disappears.
- an ordinary account has no admin tab and `GET /api/admin/users` returns 403.
- password change with a wrong current password clears all three fields, focuses
  the current password and localizes the error; a successful change shows the
  login notice, rejects the old password and accepts the new one.

Screenshots (sanitized, no secrets on screen): `01-register-invite-light.png`,
`02-register-invite-375-light.png` (375px), `03-register-open-light.png`,
`04-register-open-dark.png`, `05-admin-tab-light.png`,
`06-admin-tab-dark.png`, `07-delete-dialog-dark.png`,
`08-delete-dialog-light.png`. The invite panel is closed before any screenshot;
only synthetic `@staging.invalid` addresses appear.

## Bugs found while verifying (fixed in this batch)

- The delete-preview route had no registration, so the dialog showed a 404.
- Registration read the URL fragment in a state initializer; React can re-run an
  initializer, which stripped the secret on the first pass and produced an empty
  field. Reading it in a mount effect fixed the fragment path.
- A wrong current password answered 401, which the API client treats as an
  expired session: the user was logged out instead of seeing the field error.
  It is a 400 now.
- The one-shot password-changed notice was lost because the shell redirect to
  `/login` replaced the URL and re-rendered the page. The notice now uses a
  short-lived session marker alongside the query parameter.
- Interpolated copy used single braces; i18next needs `{{name}}`/`{{date}}`.

## Not done / next

- No deployment. The native backup/build/migrate/HTTPS switch, the real
  `bootstrap-host` installation and the actual host onboarding remain in the
  deployment batch, after review and merge authorization.
- PR #7 is not merged; this candidate is not the live release.
- The full `internal/handler` suite and the full `views` suite are not green in
  this environment for the pre-existing reasons above; they must be re-run on
  the CI/Linux runner before merge.
- Translations were authored in this batch and are parity-tested; a native
  language review is still worthwhile for ja/ko/fr copy.
- Previous failure history remains in `evidence/clawsta-77/p5/` and
  `evidence/clawsta-77/p3-r1/`, `p4/`; nothing there was rewritten.
