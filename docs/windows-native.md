# Windows native self-hosting

This workflow runs PostgreSQL, the Go API, and the production web application as native Windows processes. Docker is needed only to export an existing Docker deployment. The CLI/agent daemon remains a separate process and keeps its existing identity and credentials.

Use `scripts/native-windows.ps1` from the repository root. Keep the checkout and installed dependencies available: Windows standalone dependency junctions may refer to their original locations, so a built release is valid only on the machine that built it (moving it to another machine requires a rebuild there). Build the frontend on Windows; Linux binaries and native Node dependencies are not portable to Windows.

## Preparation

Requirements: the repository's Go, Node.js, and pnpm versions, Windows PowerShell 5.1 or newer, Git, and Windows `tar.exe`. Use an absolute deployment directory outside the checkout. Commands below use `D:\MulticaData\native`.

Download the PostgreSQL 17 Windows binary archive from [EDB](https://www.enterprisedb.com/download-postgresql-binaries), the binary distribution linked by [PostgreSQL](https://www.postgresql.org/download/windows/). Extract its `pgsql` directory into `<root>\runtime\pgsql`. Standard PostgreSQL includes the required `pgcrypto` and `pg_trgm` extensions; this application does not require pgvector.

```powershell
.\scripts\native-windows.ps1 -Action Prepare -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Backup -BackupSource Docker -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Build -Root D:\MulticaData\native
```

`Prepare` imports the current `multica-*` containers' application settings and database credentials. It restricts the deployment directory to the current Windows user and SYSTEM. Configuration and backups contain credentials; keep them outside the repository and do not publish them.

The initial ports are API `18080`, web `13000`, and database `15432`. All bind to loopback. PostgreSQL uses an ICU English locale; validate search and sorting when moving from a Linux libc locale. The default database is `multica_native_rehearsal`.


## Desktop packaging on Windows

Build the unpacked Windows desktop artifact from the repository root:

```powershell
pnpm --filter @multica/desktop package -- --win --dir
```

The command must exit 0; only then has electron-builder rewritten the
executable's version resources. Verify the identity with:

```powershell
(Get-Item 'apps\desktop\dist\win-unpacked\U Workers.exe').VersionInfo |
  Select-Object ProductName, FileDescription
# ProductName / FileDescription must both be "U Workers", not "Electron"
```

### winCodeSign extraction on an unprivileged Windows account

electron-builder unpacks its cached `winCodeSign` toolset with 7-Zip before it
rewrites the executable. That archive also contains macOS dylib symlinks, and
7-Zip fails on them without the privilege to create symlinks
(`Cannot create symbolic link ... client does not hold the required privilege`).
The packaging run then exits before rcedit runs, so a *partially* built
`U Workers.exe` still reports `ProductName: Electron`. The code is fine; the
toolset cache is not.

Do not enable Developer Mode and do not change repository configuration.
Pre-extract the cache once, skipping the macOS-only payload, then rerun the
package command:

```powershell
$cache = "$env:LOCALAPPDATA\electron-builder\Cache\winCodeSign"
$sevenZip = (Get-ChildItem node_modules\.pnpm -Recurse -Filter 7za.exe |
  Where-Object { $_.FullName -match '7zip-bin.*win\\x64' } |
  Select-Object -First 1).FullName

# Keep one downloaded archive, drop the failed extraction leftovers.
$archive = Get-ChildItem $cache -Filter *.7z | Select-Object -First 1
$temp = Join-Path $env:TEMP 'winCodeSign-2.6.0.7z'
Copy-Item $archive.FullName $temp
Get-ChildItem $cache | Remove-Item -Recurse -Force

# Extract everything except the darwin payload (the symlinks Windows cannot create).
& $sevenZip x -bd -y -x'!darwin*' $temp -o"$cache\winCodeSign-2.6.0" | Out-Null
Test-Path "$cache\winCodeSign-2.6.0\rcedit-x64.exe"   # must be True
```

A correct cache holds `winCodeSign-2.6.0\rcedit-x64.exe`; electron-builder
reuses it without extracting again. The failed extraction leaves numeric
directories such as `197572275` plus `197572275.7z` next to the final
directory; remove them.

### Packaged main process and workspace TypeScript

Shared packages under `packages/` export raw TypeScript that consuming apps
compile (see repository AGENTS.md). In the Electron main process such a package
must be bundled, not externalized: an externalized `require("@multica/core/...")`
resolves to `app.asar/node_modules/.../*.ts`, and Node 24 refuses to strip types
inside `node_modules`. The packaged app then exits with "A JavaScript error
occurred in the main process" and `ERR_UNSUPPORTED_NODE_MODULES_TYPE_STRIPPING`.
`apps/desktop/electron.vite.config.ts` therefore excludes `@multica/core` from
`externalizeDepsPlugin`.

## Clean machine deployment (no Docker source)

`Prepare` without `-Clean` imports application settings and database credentials
from the existing `multica-*` Docker containers, so it only works where those
containers exist. A machine that has never run this deployment uses the clean
path, which generates its own credentials, verifies the PostgreSQL archive and
records the pinned toolchain:

```powershell
.\scripts\native-windows.ps1 -Action Prepare -Clean -Root D:\UWorkers\clawsta80\native `
  -Database multica_clawsta80 -DatabasePort 16432 -ApiPort 18081 -WebPort 13001 `
  -PublicOrigin http://192.168.3.49:13001 `
  -CorsOrigins "http://192.168.3.49:13001,http://100.73.70.87:13001" `
  -WebHost 0.0.0.0 `
  -PgArchive D:\UWorkers\clawsta80\downloads\postgresql-17.11-1-windows-x64-binaries.zip `
  -PgArchiveSha256 6EABDF00D2893713B75DB4336A23C3FDF505F056E217EC6E2E95D901750CFEA3 `
  -Version v0.2.0 `
  -NodeExe "C:\Program Files\nodejs\node.exe" `
  -GoExe D:\UWorkers\clawsta80\tools\go\bin\go.exe `
  -PnpmCmd D:\UWorkers\clawsta80\tools\pnpm\bin\pnpm.cjs `
  -GoModCache D:\UWorkers\clawsta80\tools\gomodcache
.\scripts\native-windows.ps1 -Action Build -Root D:\UWorkers\clawsta80\native
.\scripts\native-windows.ps1 -Action Start -Root D:\UWorkers\clawsta80\native
```

- `-PublicOrigin` becomes `FRONTEND_ORIGIN` and `MULTICA_APP_URL`; `-CorsOrigins`
  is the exact browser-origin allowlist, and every entry must be an origin
  (scheme, host, optional port), not an application path.
- `-WebHost` defaults to loopback. Use `0.0.0.0` only together with a firewall
  rule restricted to the LAN subnet that should reach the deployment (plus any
  Tailscale/remote address); the API and database stay on loopback.
- The PostgreSQL archive is checksum-verified before extraction, is extracted
  only when `runtime\pgsql\bin\psql.exe` is missing, and never replaces an
  existing `runtime\pgsql`. Download it from the EDB binary archive page in
  Preparation.
- `-Version` is required when the checkout has no reachable `v*` tag, as is
  normal for a PR-branch clone. `-GoExe`, `-PnpmCmd` and `-GoModCache` pin the
  build to task-local tools; with `-GoModCache` the build runs with
  `GOPROXY=off` against that verified module cache (useful where
  `proxy.golang.org` is unreachable).
- `Prepare -Clean` refuses an existing `deployment.json` and prints no secrets;
  generated credentials stay in that ACL-protected file.
- `Configure` can later change `-ApiPort`, `-WebPort`, `-Database`, `-WebHost`,
  `-PublicOrigin` and `-CorsOrigins` while the deployment is stopped.

## Restore and verify

```powershell
.\scripts\native-windows.ps1 -Action Restore -Root D:\MulticaData\native `
  -BackupPath D:\MulticaData\native\backups\<timestamp> -Database multica_native_rehearsal
.\scripts\native-windows.ps1 -Action Start -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Status -Root D:\MulticaData\native
```

Restore verifies backup checksums, uses `pg_restore --exit-on-error`, refuses an existing target database or a running native supervisor, restores uploads, and runs `ANALYZE`. Start runs migrations before the API, checks readiness, and proves the responding API PID belongs to this deployment.

A restored database includes scheduled jobs and channel installations. Do not run its API concurrently with production when that could dispatch duplicate work or connect the same bot twice. Check these tables first or stop the old API during application rehearsal. Database-only restore checks can run while production remains available.

Verify `/readyz`, existing-account login, issue data, attachment downloads, WebSocket connections, and the existing daemon's reconnect. Do not start real agent runs as part of a default smoke check.

Browser realtime uses the page's host and protocol by default: HTTP pages connect to `ws://<host>/ws`, and HTTPS pages connect to `wss://<host>/ws` through the frontend proxy. Leave `FrontendEnvironment.NEXT_PUBLIC_WS_URL` empty when using both LAN and HTTPS entry points; a fixed LAN IP or `ws://` override breaks external HTTPS access. Explicit overrides remain supported for separately hosted WebSocket services. Add each browser origin to `BackendEnvironment.CORS_ALLOWED_ORIGINS` and apply configuration changes during an idle managed restart. HTTPS proxies must preserve WebSocket upgrades and the client-facing host and protocol.

## Cutover

1. Verify the database restore and native build before stopping the current application.
2. Pause task dispatch and stop the Docker frontend/API. Keep PostgreSQL running for the final export. `Backup` can copy uploads from a stopped backend container.
3. Take a final backup, restore it into a new native production database, and stop the rehearsal stack.
4. Configure the original ports, then start the native stack:

```powershell
.\scripts\native-windows.ps1 -Action Configure -Root D:\MulticaData\native `
  -Database multica -ApiPort 8080 -WebPort 3000
.\scripts\native-windows.ps1 -Action Start -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action InstallStartup -Root D:\MulticaData\native
```

The startup entry runs the supervisor hidden when the current user signs in. This is per-user login startup, not a system service that runs before login. PostgreSQL, the API, and frontend are restarted if they exit. Logs are separated by process start in `<root>\logs`.

The agent daemon discovers Claude's native `%USERPROFILE%\.local\bin\claude.exe` installation even when its inherited PATH predates the installation. If an npm upgrade removes the old `claude.cmd` entry, the daemon re-resolves the command and verifies the replacement version during its existing refresh or next task launch, without restarting running tasks. Explicit executable-path overrides remain authoritative and must be updated when their target moves.

Preserve the Docker containers and volumes until the native deployment has been accepted. Stop Docker Desktop to release its overhead. Ubuntu or other WSL distributions may still consume memory independently.

## LAN access and persistence

The API and PostgreSQL stay loopback-only; only the web listener may be
reachable from other machines. With `-WebHost 0.0.0.0` the Next server listens
on every interface while `REMOTE_API_URL` still resolves to `127.0.0.1:<api>`, so
browser traffic goes through the Next rewrite (including `/ws`) instead of
opening the API port. Add the exact browser origins to `CORS_ALLOWED_ORIGINS`
and leave `NEXT_PUBLIC_WS_URL` empty so the page host and protocol are used.

Allow the web port from the network that should reach it, not from one client,
and keep the batch-specific rule name so widening and removal stay exact. This
deployment's rule admits the whole `192.168.3.0/24` LAN and keeps the Tailscale
exception:

```powershell
# Create the rule on a new host (replace 192.168.3.0/24 with that site's LAN CIDR).
New-NetFirewallRule -DisplayName 'UWorkers-CLAWSTA80-Web-13001' -Direction Inbound `
  -Action Allow -Protocol TCP -LocalPort 13001 -RemoteAddress 192.168.3.0/24,100.102.214.78

# Widen an existing rule the same way. PowerShell reads the value back in mask
# form (192.168.3.0/255.255.255.0).
Set-NetFirewallRule -DisplayName 'UWorkers-CLAWSTA80-Web-13001' `
  -RemoteAddress 192.168.3.0/24,100.102.214.78

# Read the effective scope back; address and port filters are separate objects.
Get-NetFirewallRule -DisplayName 'UWorkers-CLAWSTA80-Web-13001' |
  Select-Object DisplayName, Enabled, Direction, Action, Profile
(Get-NetFirewallRule -DisplayName 'UWorkers-CLAWSTA80-Web-13001' |
  Get-NetFirewallAddressFilter).RemoteAddress
(Get-NetFirewallRule -DisplayName 'UWorkers-CLAWSTA80-Web-13001' |
  Get-NetFirewallPortFilter) | Select-Object Protocol, LocalPort

# Return to the narrower C1 scope (one client plus its Tailscale address).
Set-NetFirewallRule -DisplayName 'UWorkers-CLAWSTA80-Web-13001' `
  -RemoteAddress 192.168.3.162,100.102.214.78

# Or drop the rule entirely.
Remove-NetFirewallRule -DisplayName 'UWorkers-CLAWSTA80-Web-13001'
```

Never widen this to `Any` or the public internet, and do not add the API or
database ports to the rule: every LAN device uses the same one web origin, so
`CORS_ALLOWED_ORIGINS` stays the exact origin list (never `*`). A company
network replaces `192.168.3.0/24` with its own site CIDR; when the rule is
delivered by Group Policy, apply the same display name in the GPO instead. The
B1 re-check (2026-10-31) widened the C1 rule this way and re-read the effective
scope; sanitized results are in
[evidence/clawsta-83/01-lan-firewall.json](../evidence/clawsta-83/01-lan-firewall.json).

`InstallTask` registers the hidden supervisor as a Windows scheduled task that
runs as SYSTEM at boot and restarts on failure, so the deployment does not
depend on an interactive login or the SSH session that started it.
`InstallStartup` remains the per-user login alternative. When the task exists,
`Start` runs it; otherwise it launches the supervisor child directly.

For Windows OpenSSH, invoke scripts with a process-scoped execution policy:
`powershell -NoProfile -ExecutionPolicy Bypass -File <script> -Action Status -Root <root>`.
The default policy on the C1 host rejected a bare script invocation; no global
execution policy was changed.

```powershell
.\scripts\native-windows.ps1 -Action InstallTask -Root D:\UWorkers\clawsta80\native
.\scripts\native-windows.ps1 -Action Start -Root D:\UWorkers\clawsta80\native
.\scripts\native-windows.ps1 -Action Status -Root D:\UWorkers\clawsta80\native
.\scripts\native-windows.ps1 -Action Stop -Root D:\UWorkers\clawsta80\native
.\scripts\native-windows.ps1 -Action RemoveTask -Root D:\UWorkers\clawsta80\native
```

`Status` reports the scheduled task name/registration plus the supervisor, backend and
frontend PID and start time; logs stay in `<root>\logs`, separated per process
start. Verified on the first clean deployment: LAN browser login, issue
read/write, attachment round-trip and a second-session realtime update, with the
host still serving after the starting SSH session closed. See
[deployment-environment-issues.md](deployment-environment-issues.md) for the
issues found on that machine and `evidence/clawsta-80/cross-machine/` for the
sanitized results.

## HTTPS entry (P2/P3)

The LAN deployment fronts the loopback API and web server with Caddy; plain HTTP
on the old port only redirects. The exact files that are deployed live in
[deploy/windows-https/](../deploy/windows-https/); verified results are in
[evidence/clawsta-77/p2/](../evidence/clawsta-77/p2/README.md),
[evidence/clawsta-77/p3/](../evidence/clawsta-77/p3/README.md) and the P3-R1
identity correction in
[evidence/clawsta-77/p3-r1/](../evidence/clawsta-77/p3-r1/README.md).

- `https://192.168.3.49:13443` is the only browser origin. Caddy sends
  `/api/*`, `/auth/*`, `/ws`, `/uploads/*`, `/v1/*` and the health routes to
  `127.0.0.1:18081` and everything else to `127.0.0.1:13002`.
- The Next.js server moves to loopback `127.0.0.1:13002` (`-Action Configure`
  `-WebPort 13002 -WebHost 127.0.0.1`); the API and PostgreSQL stay loopback.
- `http://192.168.3.49:13001` answers GET/HEAD with `308` to the HTTPS URL and
  refuses other methods with `405`; the new firewall rule
  `UWorkers-CLAWSTA80-HTTPS-13443` admits only `192.168.3.0/24`.
- `PublicOrigin`, `FRONTEND_ORIGIN`, `MULTICA_APP_URL` and
  `CORS_ALLOWED_ORIGINS` must use the HTTPS origin; session cookies then carry
  `Secure` as well.
- The proxy clears client-supplied forwarding headers and sets
  `X-Forwarded-For` to the real peer; keep `RATE_LIMIT_TRUSTED_PROXIES` and
  `MULTICA_TRUSTED_PROXIES` limited to `127.0.0.1/32,::1/128`, never the LAN
  subnet.
- TLS uses Caddy's local CA with an IP SAN. Only the public root certificate
  (`https\public\root-ca.crt`) leaves the host; the CA private key stays in the
  ACL-restricted `https\data`. The host and each client import the root once
  (machine store on the host, user store on clients). Removing it is
  `certutil -user -delstore Root <sha1-thumbprint>`.
- `caddy-windows.ps1` owns start/stop/status/trust and registers the ONSTART
  SYSTEM task `UWorkers-CLAWSTA80-HTTPS`, so the proxy survives the SSH session
  or agent that started it; the Caddy admin API is disabled. The task runs with
  no execution time limit (`PT0S`), restarts on failure (999 times, one-minute
  interval), starts when available, ignores battery conditions and allows one
  instance; every `Start` re-applies that definition in place even while the
  proxy is healthy, and `Stop` removes only this deployment's own process by its
  exact managed executable path. P3 verified a controlled stop/start with
  normal certificate validation.
- DB/API/web upgrades stay with `runtime\native-windows.ps1`. Roll back by
  stopping the HTTPS entry and restoring the previous release, `WebPort 13001`,
  `WebHost 0.0.0.0` and the HTTP `PublicOrigin` from the pre-change backup.

## Identity, first use and the private login entry

The local login entry lives at `native\Set-UWorkers-Login.cmd` with the desktop
shortcut "设置 U Workers 登录". It is a one-time host entry that opens the web
registration page; it never asks for, reads, prints or stores a password.

It reads `native\local-login-identity.json` and `deployment.json`, then
verifies that the pinned identity's normalized (trim + lowercase, UTF-8)
SHA-256 matches the pinned fingerprint **and** that the database row for that
exact user id still carries exactly that address; requires the deployment's
`PublicOrigin` to be HTTPS; and checks whether a credential exists. With no
credential it runs `local-auth.exe bootstrap-host`, which mints a 15-minute
single-use registration invitation and opens the private
`https://<host>/register#token=…` page in a private browser window — the secret
lives only in the tool's memory and in that address bar, never in a terminal, a
log, a file or a screenshot. With an existing credential it only opens
`/login`: the entry never resets or overwrites a password. `-Preflight` runs the
same checks and stops before any invitation is minted or any browser opens.

Once the person sets their password on that page, every later action happens in
the web UI: sign in, invite or remove users, change the password, and flip the
persistent instance open-registration switch under Settings → 用户与注册
(instance administrators only). The desktop entry is not needed again.

A new identity is provisioned once by the host-local admin task
`deploy/windows-https/create-local-identity.ps1` (idempotent, transactional,
reuses an existing row without overwriting it, optionally writes the pinned
identity file); the person then completes the normal first-use onboarding in
the browser to create their own workspace. The app has no public signup or back
door. Pin only the identity the requester explicitly supplied — compare the
normalized (trim + lowercase, UTF-8) SHA-256 of the email with the supplied
fingerprint, and never infer it from an older row or the runtime owner's
profile; P3-R1 records a correction for exactly that mistake, keeping the
mis-sourced identity untouched and uncredentialed.

## Operations and recovery

### External reporting policy

This local fork disables PostHog product/error reporting, signup-attribution cookies, and daily self-host usage snapshots in code. `server/internal/privacy/reporting.go` and `packages/core/analytics/privacy.ts` are build policies, not environment switches: setting a PostHog key or clearing `DO_NOT_TRACK` cannot enable these reporting paths. The public config keeps its existing JSON fields empty, and web/desktop do not mount the source-attribution questionnaire.

Native startup also forces `DO_NOT_TRACK=1`, `ANALYTICS_DISABLED=true`, empty `POSTHOG_API_KEY`/`POSTHOG_HOST`, and `NEXT_TELEMETRY_DISABLED=1` after imported settings. Production frontend builds set `NEXT_TELEMETRY_DISABLED=1` and restore the caller's environment afterwards. Enabling reporting requires an explicit source change, rebuild, and deployment; do not remove these gates during upstream merges.

The same shared privacy module disables optional background GitHub star/release requests. The runtime's official self-update controls are hidden; build this fork's CLI with `scripts/build-local-cli.ps1`. Directly opening a documentation/download link remains an explicit user action.

`/health/realtime` requires a configured `REALTIME_METRICS_TOKEN` and its bearer credential. Without a token the route returns 404 even from loopback, since the LAN TCP relay cannot preserve the caller's address or add forwarding headers. General readiness and authenticated data routes remain available.

These controls do not disable local logs/metrics, configured model providers, or integrations deliberately enabled by the operator. They are not a machine-wide network firewall. Existing LAN access and DSH/ClawStation tunnels have their own access controls and lifecycle.

```powershell
.\scripts\native-windows.ps1 -Action Status -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Stop -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Start -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action RemoveStartup -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action InstallTask -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action RemoveTask -Root D:\MulticaData\native
```

Stop verifies each child by PID, executable, and process start time. It stops the application processes, then shuts down this PostgreSQL cluster in `fast` mode. Schedule stops/upgrades when tasks are idle; the application children are terminated by their supervisor.

For a consistent native backup, stop application processes while leaving PostgreSQL available, export the database and uploads, then start again:

```powershell
.\scripts\native-windows.ps1 -Action Stop -KeepDatabase -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Backup -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Start -Root D:\MulticaData\native
```

For an upgrade, take this backup, stop the deployment, build a new release, and start. The migrator runs before the API. Restore overwrites no existing database. Keep tested backups and previous releases.

Before the native deployment accepts writes, rollback can restart the preserved Docker deployment. After native writes begin, export the native database and uploads back to the recovery deployment first; starting the old database snapshot alone would lose those writes.

### Local password login (no mail service)

A native deployment with no outbound mail service can sign users in with
account/password credentials that the host administrator provisions locally.
The credential attaches to an existing user; it does not create users, grant
workspace membership, or change any existing profile or membership row.

To enable it, add the switches to `BackendEnvironment` in `deployment.json` (or
set them in the environment), then restart the deployment:

```json
"AUTH_LOCAL_PASSWORD_ENABLED": "true",
"AUTH_EMAIL_CODE_ENABLED":     "false"
```

`AUTH_LOCAL_PASSWORD_ENABLED` defaults to off. `AUTH_EMAIL_CODE_ENABLED`
defaults to on; setting it to `false` disables `/auth/send-code` and
`/auth/verify-code` server-side (403) rather than only hiding the form. On a
fresh deployment, provision password credentials FIRST: the email-code flow is
the only browser path that creates user rows, and a credential needs an existing
user. `/api/config` advertises both switches, so the login page shows the account
form and drops the unavailable email flow without a web rebuild.

Provision credentials with the `local-auth.exe` that `Build` writes into the
release directory. The wrapper finds the current release from `deployment.json`
and passes the connection configuration; it never prints the connection string:

```powershell
# Create a credential for an existing user (prompts twice, no echo)
D:\UWorkers\clawsta80\repo\scripts\local-auth.ps1 -Root D:\UWorkers\clawsta80\native `
  create --user-email you@example.com --username you

# Change the password; the previous one stops working immediately
D:\UWorkers\clawsta80\repo\scripts\local-auth.ps1 -Root D:\UWorkers\clawsta80\native `
  reset --username you

# Remove the credential. The user and other login methods are untouched.
D:\UWorkers\clawsta80\repo\scripts\local-auth.ps1 -Root D:\UWorkers\clawsta80\native `
  delete --username you

# Show which usernames exist and which user each one signs in as
D:\UWorkers\clawsta80\repo\scripts\local-auth.ps1 -Root D:\UWorkers\clawsta80\native list
```

Run the tool on the host that owns the database. It refuses to read a password
from a non-interactive stdin unless `--password-stdin` is passed explicitly, and
never accepts a password as a command-line argument. Passwords are at least 12
characters and at most 72 UTF-8 bytes, hashed with bcrypt (cost 12); the tool
never writes, prints or logs the plaintext, and a reset never frees the old
username.

Password login is rate limited in-process, so it works without Redis: five
attempts per account and twenty per source IP each minute, charged before the
bcrypt comparison, answering 429 with `Retry-After`. Behind a reverse proxy set
`RATE_LIMIT_TRUSTED_PROXIES` to the proxy CIDRs, otherwise every client shares
the proxy address and `X-Forwarded-For` is ignored by design.

The current LAN deployment serves the web UI over HTTP. That does not protect a
password in transit: terminate TLS at a reverse proxy (and keep the API and
database on loopback) before enabling password login beyond a trusted LAN. The
web and desktop clients share the same login page; this feature does not relax
certificate validation.

Do not record account names or passwords in this document, a task report or a
screenshot. The interactive prompt is the only expected place a password is
typed, and the tool prints passwords never.
### Outbound network acceptance

A native deployment can be observed for unsolicited outbound traffic without a
machine-wide firewall: start `scripts/e2e/recording-proxy.mjs`, put
`HTTP(S)_PROXY` at the recorder (with `NO_PROXY` covering loopback) into the
deployment's `BackendEnvironment`/`FrontendEnvironment`, and point the browser
context at the same recorder. The recorder records destination metadata and
refuses every non-loopback request with 502, so an attempted request fails
instead of being silently swallowed.

The full isolated procedure, exact Playwright specs, sanitized evidence format
and observed results are in
[docs/outbound-data-verification.md](outbound-data-verification.md). Keep it on
its own root, cluster, database, ports and generated credentials; never reuse a
production deployment or database. The same document records the boundary:
same-host Windows verification and C1 cross-machine LAN login/realtime are
covered. Company-network and HTTPS access remain untested; desktop request
observation is tracked separately in D2.

When launching PowerShell helpers for that deployment, do not keep both
`NO_PROXY` and `no_proxy` (or the same spelling pair for `HTTP_PROXY`,
`HTTPS_PROXY`) in the process environment: Windows environment names are
case-insensitive and .NET cannot create a child process from that duplicated
environment block.

Run the script tests with `powershell -NoProfile -File scripts/native-windows.test.ps1`. They use temporary files and fake child processes, never PostgreSQL, Docker, or user-installed agent CLIs.
