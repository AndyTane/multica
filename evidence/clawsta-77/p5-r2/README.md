# CLAWSTA-77 P5-R2: remaining backend boundary fixes (candidate)

Branch `agent/dsh/p5-registration`, PR #7 (base `8c61cdef`). Batch range
`953a8294f..<head>` where `<head>` is the evidence commit at the tip of this
branch. This is still a candidate: the live deployment was not replaced or
restarted, the HTTPS scheduled task and the Tane account were not touched, no
Multica/DSH process was operated, and PR #7 was not merged.

| Commit | Scope |
| --- | --- |
| `7dd1e6e2a` | B1: invitation binding requires a verified identity; one instance signup policy |
| `3649e7da9` | B2: chat/task deletion blockers; account write/delete state boundary |
| `2195e26db` | B3: one state boundary between WebSocket registration and revocation |
| `24b8bb00b` | U1/U2/U4: settings padding, generic cancel, blocker contract in five languages |
| `f455ec3b3` | U3: one-shot desktop password-changed notice |
| this commit | Evidence and task-book sync |

## Isolation

- Fresh database `multica_p5candidate` on the desktop PostgreSQL cluster,
  reached through a loopback SSH tunnel (`16432 -> 127.0.0.1:16432`). It was
  dropped and recreated for this batch and migrated through
  `582_user_email_verified`. The live `multica_clawsta80` database was never
  opened for writes.
- Candidate API on `18082`, candidate web build on `13003`, Caddy with
  `tls internal` on `https://localhost:13444`. Browser identity data is
  synthetic (`@staging.invalid`).
- Temporary secrets: one per-run password generated inside the shell, passed to
  the seed program on stdin and to the browser process through its environment
  (process memory only). It appears in no file, log, comment, evidence or
  screenshot. No credential from an earlier batch was reused, and the scratch
  directory was removed at teardown.

## Closed in this batch

### B1 invitation identity and instance signup policy

- `CreateInvitation` no longer turns `GetUserByEmail` into `invitee_user_id`
  unless that account has `email_verified`. The reverse ordering is now covered
  end to end through the real handlers: open registration with the victim's
  mailbox, then a workspace invitation to that mailbox, then list/accept as the
  squatter. The invitation is not bound (`invitee_user_id IS NULL`), is absent
  from the squatter's list and acceptance is 403. A verified account addressed by
  its own mailbox still binds and accepts.
- `findOrCreateUser` refuses to merge a verified mail claim (email code or
  Google) into an unverified same-email account with
  `ErrUnverifiedEmailAccount`; the row and its password stay untouched.
- Every account-creation path (`OpenRegistration`, email-code, Google) applies
  one policy: the deployment switch, the persistent `open_registration` setting
  (default off) and the configured email/domain allowlists. A pending workspace
  invitation is no longer an instance registration grant, and the policy read is
  re-done at account creation (a switch closed between the two steps still
  refuses). Production email/Google login remains disabled in this deployment;
  no verification service was added.

### B2 deletion truth and account write races

- `InspectInstanceUserDependencies` classifies `chat_session.creator_id`
  (`chat_history`, which would otherwise cascade-delete the session and its
  messages) and queued/dispatched/running `agent_task_queue` rows reachable
  through `accountable_user_id`, `initiator_user_id` or
  `originator_user_id` (`active_task`, one row counted once) as blockers.
  Preview and DELETE share the same function; DELETE re-checks inside its
  transaction. Chat history, chat messages, the live task and its attribution
  stay unchanged after the refused delete; once the task finishes, deletion
  succeeds and concluded tasks keep their rows with attribution detached.
- `UpsertClientUsage` now always runs in one transaction: account key,
  live-account re-read, then the workspace lock/membership check, then the
  upsert. Deletion takes the same account key before its inventory scan, so no
  usage row can be inserted after the final scan. CLI credential and invitation
  writes (`local-auth create/reset/invite/bootstrap-host`) take the same key and
  re-read the account before writing; a deterministic lock-holder test deletes
  the account while both writers are queued and shows zero orphan rows.
- The existing admin mutual-delete, invite regeneration/redemption and password
  concurrency regressions still pass.

### B3 WebSocket handshake / revocation window

- The hub keeps a revocation counter and a per-account last-revoked mark. A
  handshake captures the counter before validating its credential; registration
  checks the mark and inserts the client under the same lock that
  `DisconnectUser` stamps it. The reviewer's reproducer passes, a new valid
  session after revocation still connects and receives events, and other users
  keep their sockets.
- Wiring is verified through the real `ChangeCurrentPassword` and
  `DeleteInstanceUser` handlers against the isolated PostgreSQL database using a
  real `HandleWebSocket` + Hub + network connection: the affected socket closes
  immediately and a bystander still receives a workspace broadcast.

### U1-U4

- U1: open registration is a `SettingsRow` (label, consequence description,
  switch in the control slot); the add-user form, invitation panel and account
  rows have their own 16px padding inside the `px-0` card; the
  password-change card gets the same treatment. Browser DOM measurements report
  16px/14px for the registration row and 16px/16px for the form and card.
- U2: the delete dialog's cancel control reads `取消` / `Cancel`, never `完成`.
- U3: the desktop login page consumes the one-shot password-changed marker,
  clears it on first render and passes the localized notice to the shared
  `LoginPage`; a regression proves it shows once and not on the next mount.
- U4: `BLOCKER_CODES` and five-language copy include `chat_history`
  ("有聊天记录" / "Has chat history") and `active_task`
  ("有进行中的任务" / "Has work in progress"). Unknown codes are merged into one
  line summing their counts ("另有 N 项依赖需先处理" /
  "N more dependencies must be resolved first") instead of being dropped, and
  the confirm button remains gated by the server's `can_delete`; component
  tests cover new codes, the unknown fallback and preview failure/malformed.

## Tests actually run

Go, against the isolated candidate database unless noted:

| Command | Result |
| --- | --- |
| `go build ./...` | pass |
| `go vet ./...` | pass (empty log) |
| `go test ./internal/realtime -run 'TestReviewRevocationDuringHandshake|TestP5R2|TestP5R1' -count=1` | pass (1.5s); reviewer reproducer green |
| `go test ./internal/realtime ./internal/auth -count=1` | pass |
| `go test ./internal/handler -run 'TestP5R2|TestP5R1|TestP5' -count=1` | pass (67.9s) |
| `go test ./internal/handler -run 'TestSignup|TestFindOrCreate|TestGoogle|TestSendCode|TestVerifyCode|TestEmailCode|TestWorkspaceInvitation|TestAccountWrites|TestP5' -count=1` | pass (80.6s) |
| `go test ./cmd/local-auth -count=1` | pass (17.3s, 11 tests) |
| full `go test ./internal/handler -count=1` | see "Pre-existing failures" below |

The new Go regressions in this batch:
`TestP5R2InvitationCannotBeClaimedByAnEarlierUnverifiedRegistration`,
`TestP5R2FindOrCreateUserRefusesUnverifiedSameEmailAccount`,
`TestP5R2DeleteBlockedByChatHistory`, `TestP5R2DeleteBlockedByActiveTask`,
`TestP5R2ClientUsageWriteAndDeleteHaveNoOrphan` (both orders + in-flight
write barrier), `TestP5R2PasswordChangeAndDeleteCloseLiveWebSockets`,
`TestP5R2NewSessionAfterRevocationStillConnects`,
`TestP5R2HandshakeRevocationLeavesOtherUsersConnected`,
`TestAccountWritesCannotFollowADeletedAccount`,
`TestWorkspaceInvitationDoesNotAuthorizeInstanceSignup` and
`TestVerifyCodeRechecksInstanceRegistrationSwitch`.

Frontend:

| Command | Result |
| --- | --- |
| `pnpm --filter @multica/core typecheck` | pass |
| `pnpm --filter @multica/views typecheck` | pass |
| `pnpm --filter @multica/web typecheck` | pass |
| `pnpm --filter @multica/desktop typecheck` | pass |
| `pnpm --filter @multica/web test` | 25 files / 245 tests pass |
| `pnpm --filter @multica/desktop test` | 62 files / 653 tests pass; 1 pre-existing suite failure `scripts/package.test.mjs` (SyntaxError, unmodified) |
| `npx vitest run settings/components/instance-users-tab.test.tsx settings/components/password-change-card.test.tsx settings/components/settings-page.test.tsx auth/login-page.test.tsx locales/parity.test.ts` | 5 files / 289 tests pass (parity covers all five languages) |
| `npx vitest run src/renderer/src/pages/login.test.tsx` (desktop) | 7 tests pass, including the one-shot notice |
| `pnpm --filter @multica/{views,core,web,desktop} lint` | 0 errors (pre-existing warnings only) |
| `pnpm --filter @multica/web build` | pass (candidate build used for staging) |

## Real-browser acceptance

26/26 steps pass in a real Chromium against `https://localhost:13444`,
starting from clicks and keystrokes: admin password sign-in; admin tab with
measured row/form/card padding; dark theme; zh-Hans language switch and zh
admin tab; password-change card; delete preview blocked by `有聊天记录` and by
`有进行中的任务` with confirmation disabled and `取消` as the cancel control;
ordinary account has no admin tab and the admin API answers 403; a password
change lands on the login notice; the old password is rejected and the new one
signs in.

Screenshots (sanitized; only `@staging.invalid` synthetic accounts, no
passwords or invitation secrets) in `browser/`:

- `01-admin-tab-light.png`, `02-admin-tab-dark.png`
- `03-admin-tab-zh.png`, `04-password-card-zh.png`
- `05-delete-preview-chat-history.png`,
  `06-delete-preview-active-task.png`
- `07-password-changed-notice.png`
- `browser-evidence.json` (step log)

### Certificate-validation limitation

Installing the Caddy local CA into the Windows root store is blocked in this
non-interactive session: `Import-Certificate` reports "UI not allowed in this
operation", `X509Store.Add` blocks on the consent UI, a direct
`HKCU\...\Root\Certificates` write is removed by root-certificate protection
and the machine store is not elevated. Three orphaned Caddy roots from earlier
batches remain in the store, but their keys are gone.

The browser therefore ran with `ignoreHTTPSErrors: false` plus Chromium's
`--ignore-certificate-errors-spki-list` pinned to the staging leaf's exact
public key (no blanket ignore). Independently, a normal TLS client trusting the
staging root validated the chain and `localhost` hostname
(`TLS_CHAIN_VALID status=200`). This is the only deviation from "CA trusted in
the OS store"; the page code and origin are the real candidate build.

## Pre-existing failures / boundaries

- The full `internal/handler` suite still hits the recorded pre-existing
  failures in this Windows environment and does not finish (901.8s, then a
  15-minute package timeout inside `TestWorkerReplyDelivery`). Failing classes:
  Windows path separators, runtime-offline dispatch/claim assertions, and the
  order-dependent `TestCreateComment_ThreadReplyDoesNotEscalateToAssignee`
  subtests. A bounded baseline check at the batch base `953a8294f` — detached
  worktree, separately migrated `multica_p5candidate_base` — reproduced the same
  failure classes for the same test set, so they are not introduced here. The
  comment tests pass in isolation at both commits and their files run before
  every P5 test file. Details: `full-handler-failures.txt`. The targeted
  P5/P5-R1/P5-R2, signup/Google/email-code and local-auth suites are green. No
  new CI gate was added; the full package must be re-run on CI/Linux.
- The full `@multica/views` suite has the same parallelism-only failures as the
  previous batch: `npx vitest run` reports 13 failed files / 26 failed tests
  against 460 passed files / 5889 passed tests (448s). Every failing file
  (mcp config/dialog/tab, code-vcs, inbox filters, create-issue/create-project,
  cron grammar, autopilot schedule, issue property/status filters, registration
  page, markdown paste) passes when run alone — re-verified this batch, all 13
  PASS. None of them imports a changed file: the changed components
  (instance users tab, delete dialog, password-change card, desktop login) and
  the five-language parity suite are green. Desktop keeps the pre-existing
  `scripts/package.test.mjs` failure.
- Native language review of ja/ko/fr copy is still advisable; parity is tested.
- No deployment, merge, HTTPS change, Tane identity change or Multica/DSH
  operation. The deployment batch remains separate.
