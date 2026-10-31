# CLAWSTA-77 P1 isolated verification: local password login

Sanitized results for the local account/password login delivered in this branch
(`docs/plans/2026-10-31-local-password-login.md`, P1). Everything below ran
against a dedicated native Windows deployment on the build host: its own
PostgreSQL cluster, database (`multica_p1verify`), ports (API 18091, web 13011,
database 16443), generated credentials and root directory. The working desktop
deployment at `D:\UWorkers\clawsta80\native` was not touched, and the isolated
root/database were removed after the evidence was copied out.

- Branch base: `5af22c3eb` (main).
- Test account and passwords existed only inside that isolated environment. They
  are not recorded here or in the JSON evidence; no screenshot shows a filled
  credential.
- The web UI was served over loopback HTTP. This does not prove anything about
  TLS or cross-machine transport; that stays with the deployment/acceptance step.

## Automated API matrix

`api-evidence.json` — 26/26 checks passed, 0 failed. Highlights:

| Check | Result |
| --- | --- |
| Password route before `AUTH_LOCAL_PASSWORD_ENABLED` | 403 with `local_password_login_disabled` |
| `/api/config` capability flags | password `false` → `true`, email code `true` → `false` |
| Wrong password / unknown username | both 401 with byte-identical bodies |
| Correct password (case/whitespace variant of the username) | 200, token + user, HttpOnly auth cookie |
| `/api/me` and `/api/workspaces` with the cookie | 200, expected user email |
| Oversize password (>72 bytes) | 400 |
| Logout → `/api/me` → login again | 200 → 401 → 200 |
| Reset password with `local-auth reset` | old password 401, new password 200 |
| 6th attempt within a minute | 429 with `Retry-After: 59` |
| `/auth/send-code` and `/auth/verify-code` with email login disabled | 403 with `email_code_login_disabled` |

The admin tool was exercised exactly as an operator would: `local-auth create`,
`reset`, `list` through `--deployment-config`, with the password piped on stdin.
Its output never contained the password, and `list` never contained a hash.

## Real browser

`browser/browser-evidence.json` — 12/12 steps passed in Microsoft Edge
(Chromium 143, Playwright 1.58.2, headless, locale zh-CN) against
`http://127.0.0.1:13011`:

1. `/api/me` is 401 before login.
2. The login page shows only the account/password form (no email-code flow,
   because the server advertised password-only).
3. Submitting the credential lands on the protected workspace board
   `/p1-verify/issues`; `/api/me` is 200 and the page renders.
4. Reload keeps the session (`/api/me` 200, same path).
5. `POST /auth/logout` returns 200 and `/api/me` returns 401.
6. Signing in again from the login page returns to the protected board.
7. No uncaught page errors during the flow.

Screenshots: `browser/01-login-page.png` (before typing),
`browser/02-after-login.png`, `browser/03-relogin.png`.

## Backend checks (Windows build host)

Run from `server/` against a migrated PostgreSQL database:

| Command | Result |
| --- | --- |
| `go build ./...` | exit 0 |
| `go vet ./internal/auth ./internal/middleware ./internal/handler ./cmd/local-auth ./cmd/server ./internal/migrations` | exit 0 |
| `go test ./internal/auth ./internal/middleware ./cmd/local-auth -count=1` | ok, ok, ok |
| `go test ./internal/handler -run 'TestPasswordLogin|TestEmailCodeLogin|TestWorkspaceDeletionManifestCoversPublicSchema' -count=1` | ok (all password login, switch, rate limit, boundary, orphaned-credential and deletion-manifest cases pass) |
| Full `go test ./internal/handler -count=1` | one pre-existing Windows failure (`TestParseSkillArchive_RejectsUnsafeSkillMdPath` fails identically on pristine `5af22c3eb`) and one order-dependent flake (`TestBatchImportViaHeartbeat` passes when run alone on both baseline and this branch) |

## Frontend checks

`pnpm typecheck` exit 0 (10/10 turbo tasks) and `pnpm lint` exit 0 (0 errors; 20
pre-existing warnings, none in touched files). Full `pnpm test` is limited by
this Windows box's default 5s test timeout, not by assertions: with the timeout
raised, web 245/245 passed, views 5881/5882 (the one failure passes 98/98 when
re-run alone), desktop 648/648, core 2214 tests, ui-lab 46/46. Login-page +
locale-parity focused run: 247/247.

## Known limits

- Both login methods enabled at once was covered by component tests, not by this
  deployment run (the isolated deployment verified the password-only shape the
  trial needs). Switching between methods is exercised in
  `packages/views/auth/login-page.test.tsx`.
- The desktop production deployment was not replaced; deploying and verifying
  the real trial login stays with the acceptance step.
- HTTPS/trusted-proxy transport was not part of this isolated run.
