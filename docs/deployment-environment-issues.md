# Deployment environment issues

Problems found while deploying the native Windows stack to a clean second
machine, with the fix that landed and how it was re-tested. The deployment was
CLAWSTA-80 / C1: repository baseline `6b213959c5b5a4bb302fb6eb7c286c285ea014cd`
on the target host CHUN-I5-12400F-32G (Windows 11, LAN 192.168.3.49), observed
from the client PC over the LAN. Evidence: `evidence/clawsta-80/cross-machine/`.

## Verified facts on this machine (2026-10-30)

| Item | Value |
| --- | --- |
| Windows / PowerShell | Windows 11 26300, PowerShell 5.1.26100 |
| Node / pnpm | Node v24.21.0 (present), pnpm 10.28.2 (installed task-local) |
| Go | go1.26.9 windows/amd64 (task-local copy) |
| PostgreSQL | 17.11 Windows binaries (EDB archive, SHA-256 verified) |
| Pinned npm install | filtered `@multica/web...` frozen install, 5m54s |
| Release | `v0.2.0-20261031030941`, 44.7 MB `server.exe`, Next 16.3.8 standalone |
| Listeners | API `127.0.0.1:18081`, PG `127.0.0.1:16432`, web `0.0.0.0:13001` |
| Database | `multica_clawsta80`, 598 migrations, 125 public tables |
| Persistence | Scheduled task `MulticaNative-D-UWorkers-clawsta80-native`, SYSTEM, boot trigger, hidden |
| Firewall | `UWorkers-CLAWSTA80-Web-13001` inbound TCP 13001 from 192.168.3.162 and 100.102.214.78 only |

## Environment issues and fixes

### 1. `Prepare` could only import an existing Docker deployment

- Symptom: on a machine with no `multica-*` containers, `Prepare` failed at
  `docker inspect multica-backend-1`; the only documented first-run path assumed
  Docker as the settings source.
- Root cause: first-run preparation hard-coded container import.
- Fix: `Prepare -Clean` generates its own database/JWT credentials, takes
  `-PublicOrigin`, `-CorsOrigins`, `-WebHost`, `-Version` and the toolchain paths,
  writes an ACL-protected `deployment.json`, and refuses to overwrite one. The
  Docker-import path is unchanged for machines that have it.
- Retest: `Prepare -Clean` on the empty host, then `Build` and `Start`; sections
  2-8 below were found by that run. `scripts/native-windows.test.ps1` covers the
  clean configuration, generated secrets, CORS list and overwrite refusal.

### 2. Go module proxy unreachable; direct VCS fetch reset

- Symptom: `Test-NetConnection` reached `github.com:443`, but
  `go mod download` failed with `Recv failure: Connection was reset` against
  `github.com/aws/aws-sdk-go-v2`; `proxy.golang.org:443` and `sum.golang.org:443`
  were unreachable (TCP closed).
- Root cause: this network blocks the Go module proxy/checksum service and resets
  some Git-over-HTTPS fetches; TCP connect alone did not predict it.
- Fix: populate the module cache on a machine that can reach it
  (`go build` with `GOPROXY=off` succeeds there) and transfer it as a verified
  offline archive. `Prepare -Clean -GoModCache <dir>` makes `Build` run with
  `GOMODCACHE=<dir>`, `GOPROXY=off`, `GOSUMDB=off`; `go.sum` still verifies
  module content. No mirror, TLS weakening or global proxy change was used.
- Retest: `server.exe` and `migrate.exe` built offline on the target host
  (44.7 MB / 10.9 MB), API serves `/readyz` with `migrations: ok`.

### 3. pnpm absent and version-sensitive

- Symptom: no pnpm on the host; `packageManager` pins `pnpm@10.28.2`.
- Root cause: Node is installed system-wide but the package manager is not.
- Fix: copy the exact 10.28.2 pnpm package directory into the task root and run
  it as `node <root>\tools\pnpm\bin\pnpm.cjs`; `Prepare -Clean -PnpmCmd` records
  that path so `Build` never depends on a global install or corepack download.
- Retest: `pnpm --version` = 10.28.2; frozen filtered install completed.

### 4. PostgreSQL binaries are not part of the source tree

- Symptom: no PostgreSQL on the host; `runtime\pgsql` missing would fail at
  first `initdb`.
- Root cause: clean install expects the operator to fetch the EDB Windows binary
  archive, and a wrong/partial archive is hard to detect afterwards.
- Fix: `-PgArchive` + `-PgArchiveSha256`; the script verifies the archive SHA-256
  before extraction, extracts only when `runtime\pgsql\bin\psql.exe` is absent,
  and never replaces an existing `runtime\pgsql`.
- Retest: archive SHA-256 `6EABDF00...CFEA3` verified; `initdb`/`pg_ctl` ran;
  `SELECT version()` = PostgreSQL 17.11, 598 migrations applied.

### 5. A PR-branch checkout has no reachable version tag

- Symptom: `Build` required `git describe --tags --match 'v[0-9]*'` to match
  `v1.2.3`; the branch contains no reachable `v*` tag, so the build refused.
- Root cause: version stamping assumed a release tag; version-gated features
  still need a real version string.
- Fix: `Prepare -Clean -Version vX.Y.Z` records the release version; `Build` uses
  it when the checkout has no tag, and stamps `unknown` as the commit instead of
  failing when there is no Git history.
- Retest: release `v0.2.0-...` built from the transferred source archive;
  `/health` reports the stamped version source.

### 6. A fresh cluster has no application database

- Symptom: first `Start` on a fresh cluster left migrations failing because
  database `multica_clawsta80` did not exist; only the restore path created a
  database.
- Root cause: the supervisor ran migrations immediately after `initdb`.
- Fix: the supervisor now ensures the configured application database exists
  before migrations (name validated, `CREATE DATABASE ... TEMPLATE template0`).
- Retest: fresh cluster start applied 598 migrations; `/readyz` reports db and
  migrations ok. The restore path still refuses to overwrite a database.

### 7. Web listener was loopback-only

- Symptom: local `Invoke-WebRequest http://192.168.3.49:13001/login` worked only
  on the host; a browser on the client PC timed out.
- Root cause: the frontend environment hard-coded `HOSTNAME=127.0.0.1`.
- Fix: `-WebHost` on `Prepare -Clean` (and `Configure`) selects the listener
  interface; loopback stays the default. `REMOTE_API_URL` still points at
  `127.0.0.1:<api>`, so only the Next server talks to the API and `/ws` goes
  through the same rewrite.
- Retest: web listens `0.0.0.0:13001`; LAN browser login, issue, attachment and
  WebSocket acceptance all passed; API `18081` and PG `16432` remain closed from
  the LAN (verified by direct TCP attempts from the client).

### 8. Lifecycle did not survive a reboot without an interactive login

- Symptom: `Start` launched the supervisor as a child of the SSH session and
  `InstallStartup` only runs at interactive user logon, which is unsuitable for
  an unattended host.
- Root cause: no machine-level persistence; a closed session or reboot lost the
  deployment until someone logged in.
- Fix: `InstallTask` registers the hidden supervisor as a scheduled task
  (SYSTEM, boot trigger, hidden, restart-on-failure); `Start` runs the task when
  it exists, otherwise falls back to the previous child launch. `RemoveTask`
  deletes it. `Status` reports the task state plus each process PID and start
  time.
- Retest: started from the task, read back from a new SSH connection after the
  original one exited; killing the verified backend child had it replaced in
  2.6 s while supervisor/frontend stayed up; `Stop` cleared all listeners in
  4.5 s and `Start` was ready again in 1.6 s.

### 9. Standalone Windows build references the local checkout through junctions

- Symptom: the release tree contains 47 directory junctions that point into the
  build machine's pnpm store (`<root>\repo\node_modules\.pnpm\...`), because
  Next standalone traces dependencies rather than copying them.
- Root cause: Next.js standalone output on Windows uses junctions for traced
  dependencies.
- Fix: build on the deployment machine and keep the checkout in place. The
  release is not portable between machines as a directory: relocating it needs a
  rebuild on the target machine. The portability check that is meaningful — "no
  junction points at another machine's path" — is recorded in
  `03-persistence-lifecycle.json` (`foreign_target_count: 0`).
- Retest: every junction target starts with the deployment root; the LAN web
  acceptance ran against that release.

### 10. npm registry connections reset during install

- Symptom: the first frozen install ended with
  `ECONNRESET ... registry.npmjs.org/ccount` after ~1320 packages, and one
  tarball download averaged 22 KiB/s.
- Root cause: transient network instability to the public registry on this host.
- Fix: retry the same frozen install; pnpm reuses its content-addressable store,
  so a retry resumes instead of restarting. Retries use the official registry;
  no mirror was introduced.
- Retest: retry completed in 5m54s, no lockfile change.

### 11. Windows OpenSSH makes ad-hoc PowerShell administration awkward

- Symptom: `ssh <host> <powershell one-liner>` was unreliable: `$` expanded by
  the calling shell, and a large `-EncodedCommand` failed with "command line too
  long".
- Root cause: the remote default shell is `cmd.exe`, and the command line size
  limit is inherited from the interactive session.
- Fix: upload `.ps1` files and run `powershell -NoProfile -ExecutionPolicy Bypass
  -File <path>`; keep secrets and results in files/structured output instead of
  command lines. This is an operator practice, not a server change.
- Retest: every step in this deployment used uploaded scripts; JSON evidence was
  collected the same way.

### 12. Single-origin preparation and imported-config LAN updates

- Symptom: reviewer regression cases reproduced `PublicOrigin = "h"` when
  `Prepare -Clean` received only one origin (including the localhost default).
  `Configure -WebHost` on a Docker-imported configuration threw because that
  JSON object did not yet have the new property.
- Root cause: PowerShell unwraps a one-element function result to a string;
  indexing it selects a character. Assigning a missing PSCustomObject property
  also fails instead of creating it.
- Fix: collect the origin result with `@(...)`, and add/replace the optional
  LAN properties with `Add-Member -Force`.
- Retest: real temporary configuration writes for explicit single-origin,
  default-origin and imported-config cases now pass in
  `scripts/native-windows.test.ps1`. The existing C1 instance used two origins
  and was unaffected. Its browser acceptance was not repeated.

## Not verified here / company network checklist

This network was a home LAN (192.168.3.0/24) plus Tailscale; the deployment
itself was validated only over the LAN path. A company network needs its own
run, and these are the likely failure points, in the order this run hit them:

1. Outbound filtering: TLS-inspecting proxies and allowlists decide whether Go
   modules (`proxy.golang.org`, `sum.golang.org`, VCS hosts) and `registry.npmjs.org`
   work. Measure, then decide between a proxy configuration for the build
   account and a transferred module cache / verified offline packages.
2. Package-manager policy: some fleets block corepack or global npm installs;
   keep the task-local pinned pnpm and Node on the machine image.
3. Elevated-token restrictions: PostgreSQL 17 ran under both the elevated
   Administrator SSH token and SYSTEM here; locked-down UAC or a managed service
   account can change that, so probe `initdb` once before promising a deployment.
4. Scheduled tasks and firewall rules can be group-policy managed; capture the
   pre-existing values and keep the batch-specific rule name so removal is exact.
5. Address space: reserve the web/API/database ports in the site IPAM before the
   run; the loopback-only API/PG design only needs one LAN-reachable port.
6. Time sync and TLS trust for client browsers: the LAN entry is plain HTTP in
   this run; a company HTTPS front end changes CORS/WS settings (`NEXT_PUBLIC_WS_URL`
   stays empty, add the HTTPS origin to `CORS_ALLOWED_ORIGINS`).
