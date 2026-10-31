# CLAWSTA-77 P6: production deployment, real-browser acceptance and merge

Baseline `e275b61a4` (PR #7, base `8c61cdef`) on branch `agent/dsh/p5-registration`.
This batch was explicitly authorized by the task book to run the pre-flight
checks, deploy to the desktop, verify with a real browser and merge the PR.

## 1. Pre-flight in isolation (before any production change)

Isolated PostgreSQL `multica_p5candidate` on the desktop host, reached through a
temporary SSH tunnel; the live database was only read, never written. Migrations
ran to `582_user_email_verified`.

- `go build ./...` and `go vet ./...` pass.
- Targeted handler matrix and every P5/P5-R1/P5-R2 regression: **42 passed, 0
  failed**. `internal/auth`, `internal/middleware`, `internal/realtime` and
  `cmd/local-auth` pass.
- Stale expectations fixed: `TestFindOrCreateUserGating/whitelisted_new_user_passes_policy`
  asserted that a matching allowlist admits a new address with the deployment
  switch off; it is now `whitelisted_new_user_needs_both_switches`.
  `TestGoogleLoginActionableErrorCodes/email_not_allowed` read the always-closed
  mock instance switch instead of the allowlist; the mock now opens it first.
- New real-handler coverage (not policy helpers): the 11-row
  `POST /api/register/open` matrix, the real `POST /auth/password` and
  `POST /api/register/invite` handlers while registration is closed (first
  password once, replay refused, no credential on replay), and the email-code and
  Google entry points behind their existing stubs. Every refusal asserts no user
  row, no credential, no auth/CSRF cookie and no session token; every admitted
  account stays `email_verified = false`.
- `TestP5R3InvitationRedemptionStaysAvailable` (policy helper only) was removed;
  it never proved redemption.

Details: `preflight-tests.json`.

## 2. Deployment

`ssh desktop` -> `D:/UWorkers/clawsta80/native`, using the existing native
lifecycle only (Backup -> Build -> Stop/Start; Caddy, the CA, the scheduled tasks,
the ports and the LAN restrictions were not touched).

- Pre-change state: release `v0.2.0-20261031111027`, app commit
  `065e20ad0`, schema 575, 1 user / 1 workspace / 1 member / 0 credentials.
- Backup: `backups/native-20261031-193723` (database.dump, uploads.tar.gz,
  deployment.json, sha256.json). Old releases and backups kept.
- Source: a real git checkout of the branch on the host
  (`src-888a2e76f`, HEAD `a30713a0a`). `native-windows.ps1 -Action Build`
  produced `v0.2.0-20261031202450`; the migrator ran to 582 before the API.
- `deployment.json` `Release`/`Commit` and `/health` all report
  `a30713a0a`.
- Instance administration was granted once, idempotently, to exactly
  `934e10e0-f7f6-4160-8055-379705825b4a` (email SHA-256
  `5983f955...12788`) with the host-local `local-auth grant-admin`; it is the
  only row with `is_instance_admin`.
- `open_registration` starts closed (no `instance_setting` row).
- `AUTH_LOCAL_PASSWORD_ENABLED=true`, `AUTH_EMAIL_CODE_ENABLED=false`,
  `APP_ENV=production`, no SMTP, no Google client, no fixed verification code.

Full numbers, checksums and the before/after audit: `deployment-evidence.json`.

## 3. Defect found during deployment and fixed

`local-auth bootstrap-host` panicked on the host:

```
panic: local-auth bootstrap-host flag redefined: deployment-config
```

The command block re-registered `--deployment-config`, which every command
already gets. The desktop entry is the only path that can give the pinned
identity a first password, so this blocked the hand-off. Fixed in `a30713a0a`
(register only the command's own flags) with
`TestCommandFlagsParseWithoutPanic` covering every command, then rebuilt and
redeployed. Re-checked on the host: a wrong UUID now returns
`no user with id ...` with no panic and no invitation.

## 4. Real-browser acceptance

Chromium through Playwright against `https://192.168.3.49:13443` with the
deployment's normal certificate validation: no `ignoreHTTPSErrors`, no SPKI
exception, no `-k`, no proxy. **19/19 steps passed**:

1. unauthenticated `/api/me` is 401;
2. the old `13001` entry answers GET/HEAD 308 to HTTPS and 405 for other methods;
3. password-only login page renders the account form, the registration entry and
   no email-code form;
4. `POST /api/register/open` is refused with 403 `registration_disabled`;
5. `/register` defaults to invitation mode (no open-registration field);
6. the pinned identity signs in and lands in `/uworkers-tane/issues`;
7. the instance administrator sees 用户与注册 and the open-registration switch is
   off;
8. the administrator adds a temporary account, copies the full
   `https://…/register#token=…` link and the clipboard matches;
9. the invitation page sets the first password and strips the fragment;
10. the temporary account creates its own workspace;
11. it reaches a protected board;
12. the session survives a refresh;
13. an ordinary account sees no administration entry and gets 403 from the admin
    API;
14. logout returns the session to 401;
15. the password signs in again;
16. the web password change lands on the login page with the password-changed
    notice;
17. the pre-change session cookie is rejected (401);
18. the old password is refused and the new one signs in;
19. the temporary workspace and account are deleted through the product paths
    (owner workspace delete, then instance-admin account delete).

Screenshots are in `browser/`; every e-mail node is blurred before capture and
no invitation, password, cookie or token appears in any file.

## 5. Desktop entry (setting the formal password)

`deploy/windows-https/Set-UWorkers-Login.ps1` now opens the web bootstrap flow:
it verifies the pinned UUID, the normalized-email SHA-256 and the live database
row, requires an HTTPS `PublicOrigin`, then either opens `/login` (a credential
already exists, never overwritten) or runs `local-auth bootstrap-host`, which
mints a 15-minute single-use invitation and opens the private
`/register#token=…` page. The secret exists only in the tool's memory and the
browser's address bar: it is never printed, logged or written to disk, and errors
never echo the URL. `-Preflight` runs the same checks with no side effect.

On the host the installed entry currently reports "尚未设置密码" and the deployed
release/commit. The click that opens a user-visible private window was **not**
performed from this SSH session; the pre-checks, the identity guard and the
command path were verified, and the actual first password remains Tane's own
action on the desktop.

## 6. End state

Only the original identity, workspace and data remain:

```
users=1 workspaces=1 members=1 credentials=0 registration_invites=0
instance_admins=1 instance_settings=0   schema=582_user_email_verified
```

`uworkers-tane` (`de27c87a-…`) keeps its name, creation time, owner membership
and the identity's `onboarded_at`; no temporary credential, invitation, session,
workspace or upload was left behind.

## 7. Rollback

Stop the deployment, point `deployment.json` `Release` at
`v0.2.0-20261031111027` (schema 575 is not compatible with the new binary, so
restore `backups/native-20261031-193723/database.dump` into a new database and
update `Database` before starting), or keep the new release and only revert the
data: the migration is forward-only and there is no down-migration for 576-582 in
production.
