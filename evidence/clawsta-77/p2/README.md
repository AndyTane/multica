# CLAWSTA-77 P2: deployed HTTPS password login (cross-machine)

Sanitized results from the actual deployment on the CLAWSTA-80 desktop
(`D:\UWorkers\clawsta80\native`) and from the real cross-machine browser run on
the client laptop (192.168.3.162 → 192.168.3.49). No credential, token, cookie
value or database connection string is present in this directory.

## What is deployed

| Item | Value |
| --- | --- |
| Release | `D:\UWorkers\clawsta80\native\releases\v0.2.0-20261031111027` |
| Source | branch `agent/dsh/c4a957713d0d`, commit `065e20ad00af6bf43da0fb24e3cfd522bf95ff7d` (also reported by `/health` and `deployment.json.Commit`) |
| Previous release | `v0.2.0-20261031030941` (preserved for rollback) |
| Backup before change | `native\backups\native-20261031-110818` (database dump, uploads, deployment.json, SHA-256 manifest) |
| Entry points | HTTPS `https://192.168.3.49:13443`; HTTP `http://192.168.3.49:13001` redirects (GET/HEAD 308, other methods 405) |
| Loopback only | Go API `127.0.0.1:18081`, Next.js `127.0.0.1:13002`, PostgreSQL `16432` |
| Capabilities | `local_password_login_enabled=true`, `email_code_login_enabled=false` |
| Proxy | Caddy v2.11.7 (verified SHA-512 against the published release checksums and its Sigstore signature), SYSTEM scheduled task `UWorkers-CLAWSTA80-HTTPS`, admin API off |
| CA | local CA, subject `CN=Caddy Local Authority - 2026 ECC Root`, SHA-256 `BC77D804B7EDA0FA405E3CBB833097A1348044654393704CC3FCDAAD86D6EC25`, SHA-1 `C1BB74EE54829B7FB54AEAE7D8F375497C648237`, trusted in the host machine store and in the client laptop's user store; public root at `uworkers-lan-root-ca.crt` |
| Leaf certificate | IP SAN `192.168.3.49`, auto-renewed by Caddy |

## Files

| File | Content |
| --- | --- |
| `deployment-evidence.json` | Deployment/lifecycle facts: release, commit, listeners, capabilities, CA/leaf fingerprints, scheduled task and process identity, crash-recovery test, firewall rules, HTTP checks, backup and rollback references |
| `browser/browser-evidence.json` | 18/18 cross-machine browser steps with TLS chain, redirect chain, cookie flags, websocket URL shape, form probe timing |
| `browser/*.png` | Live HTTPS login page, failed-attempt state (password field cleared before the capture), protected board, after-refresh, after-relogin |
| `ratelimit-evidence.json` | Proxy-chain rate limiting: 5 account attempts then 429, source budget reaching 429 with rotating spoofed `X-Forwarded-For`, spoofed header ignored after the limit |
| `run-summary.json` | Orchestration record: temporary test credential lifecycle (created, used, removed), no password recorded |
| `login-ui-review.md` | Focused login interface review and its three non-blocking findings |
| `deployed-Caddyfile.txt` | The Caddyfile that is running on the host |
| `uworkers-lan-root-ca.crt` | Public root CA for client trust (no private key) |

## Cross-machine browser result (summary)

Real Chromium over the LAN, normal certificate validation: HTTPS entry redirect
from the old HTTP URL, unauthenticated `/api/me` 401, Chinese password-only
login page, in-flight submit guard, failure/retry, login to the protected board,
`wss://` realtime through the proxy, refresh persistence, logout back to 401,
re-login. See `browser/browser-evidence.json`.

## Rate limits through the real proxy chain

Each attempt is charged before bcrypt. With earlier browser attempts counted,
the source bucket reached its 20/minute budget and the next request returned 429
with `Retry-After`; 30 requests cycling 30 different spoofed
`X-Forwarded-For` values still hit the limit at the expected point, and a fresh
spoof afterwards remained 429. Account budget: 5 attempts then 429.

## Status and remaining user action

- Deployed, trusted, reachable only from `192.168.3.49/24`; the API, database
  and Next.js server are not reachable from the LAN.
- A **temporary** random credential was created for the only existing account on
  the instance for this verification and deleted afterwards; the store is empty
  again. The formal credential was not set.
- The instance currently has **no Tane account**: the only user is the
  `clawsta80-acceptance@uworkers.test` acceptance account, and Tane's email-code
  attempt from 2026-10-31 05:53 never created a user (no SMTP on the host).
  Creating a user and workspace membership is outside the P2 authorization, so
  the formal password could not be bound. The local desktop entry
  (`Set-UWorkers-Login.cmd`, shortcut "设置 U Workers 登录") is installed and
  verified to detect the existing account and abort without changing anything;
  once a real account exists it binds a password with a hidden double prompt.
- Rollback: stop the HTTPS entry, restore the previous release and the
  HTTP-only listener from the backup above; details in `deploy/windows-https/README.md`.
