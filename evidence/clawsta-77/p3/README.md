# CLAWSTA-77 P3: identity, private login entry, HTTPS lifecycle

Sanitized results from the live deployment (`D:\UWorkers\clawsta80\native`) and
from the real cross-machine browser run on the client laptop. No login name,
password, password hash, token, cookie value, session id, connection string or
personal email is present in this directory.

## What changed in this batch

| Item | Result |
| --- | --- |
| App release | Unchanged: `v0.2.0-20261031111027`, app source commit `065e20ad00af6bf43da0fb24e3cfd522bf95ff7d` |
| Deployment scripts | Updated (HTTPS lifecycle + pinned login entry); the deployment script SHA-256 differs from the app commit by design — see `host/deployment-evidence.json` |
| Identity | One new independent identity exists on the deployment, display name Tane, onboarding completed through the normal browser flow, owner of its own workspace `UWorkers` (`/uworkers`), **not** a member of the existing `C1 Cross Machine` workspace |
| Existing acceptance account | Preserved unchanged as owner of `c1-cross-machine` |
| Local credentials | 0 — the formal password is still for Tane to type on the desktop; the temporary verification credential was deleted |
| HTTPS task | Execution time limit `PT0S` (unlimited), restart on failure 999×/1 minute, battery start/stop conditions disabled, `IgnoreNew` multiple instances, SYSTEM boot trigger; every `Start` re-applies it in place |
| Backup before the change | `native-20261031-114617` (previous `native-20261031-110818` kept) |
| Previous release | `v0.2.0-20261031030941` kept for rollback |

## Files

| File | Content |
| --- | --- |
| `host/deployment-evidence.json` | Identity/membership/onboarding flags, credential count, acceptance-account preservation, HTTPS task settings readback, supervisor verification, TLS fingerprints, firewall, app commit vs deployment-script SHA, entry/queue state |
| `host/entry-tests-preflight.json` | Pinned entry behaviour: healthy preflight then cancel (exit 0, no change), empty login name refused (5), invalid login name refused (5), identity mismatch refused before any prompt (3) |
| `host/entry-tests-overwrite.json` | Existing credential is never overwritten (exit 4) |
| `host/temporary-credential-lifecycle.json` | Temporary verification credential created via encrypted SSH stdin, used for the browser run, deleted; store empty afterwards |
| `browser/onboarding-evidence.json` | 16 browser steps from the laptop with normal certificate validation: first-use onboarding (welcome → own workspace → skip runtime → finish), own workspace board, C1 workspace denied (API 404 / UI "工作区不可用"), refresh, logout 401, re-login to own workspace |
| `browser/*.png` | Onboarding welcome, workspace form, runtime step, own workspace board, C1 denied, refresh, re-login |

## HTTPS lifecycle verification (P3)

- Task settings read back with the ScheduledTasks module after the fix and again
  after a controlled `Stop` + `Start`: `PT0S`, `RestartCount=999`,
  `RestartInterval=PT1M`, `DisallowStartIfOnBatteries=false`,
  `StopIfGoingOnBatteries=false`, `SYSTEM`/`ServiceAccount`/`Highest`.
- `Start` on a healthy running proxy is idempotent: same caddy PID, no second
  supervisor, settings re-applied in place.
- A controlled stop/start cycle removed the proxy cleanly (no orphan caddy, task
  back to `Ready`), then restored it with exactly one supervisor and one caddy
  process; HTTPS health 200 with `ssl_verify_result=0` (normal trust) and the
  login page 200.
- The fix also closes the previous orphan window: `Stop` waits for the
  supervisor's orderly stop and only then removes any leftover proxy process by
  this deployment's exact managed executable path — never by process name.

## Identity + private entry (how the remaining step works)

The pinned entry `native\Set-UWorkers-Login.cmd` (desktop shortcut
"设置 U Workers 登录") reads `native\local-login-identity.json`, verifies the
pinned user id and email against the database, refuses when a credential
already exists, then asks for a login name and the tool's hidden double password
prompt. It never stores or prints a password. Because the store is currently
empty, Tane's remaining action is only: double-click the shortcut, type a login
name and his password twice, then sign in at the HTTPS URL.

## Rollback

Unchanged from P2: stop the HTTPS entry, restore the previous release,
`WebPort 13001`, `WebHost 0.0.0.0` and the HTTP `PublicOrigin` from the backup
listed above. The new identity and its own workspace are additive and can be
kept or removed independently; the existing acceptance account and workspace
were not modified.
