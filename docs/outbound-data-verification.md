# Outbound data verification (Task 8 acceptance)

Status: acceptance run executed 2026-10-30/31 against the integration baseline
`32099218abac46865ab2e7d1cbb5b18532525e92`. Results and the exact boundary of
what was and was not verified are recorded below. Policy:
[docs/outbound-data-policy.md](outbound-data-policy.md). Plan:
[docs/plans/2026-10-30-u-workers-frontend-local-only.md](plans/2026-10-30-u-workers-frontend-local-only.md).

## What is and is not claimed

Verified in this run:

- Startup, email-code login, workspace browsing, switching workspaces and idling
  produce zero unsolicited external requests from the browser, the Next.js
  production server process and the Go API process. Official cloud, analytics,
  telemetry and update destinations are included in that zero.
- Attempts to reach a blocked destination are recorded and fail. The observation
  path is proven live with exact control requests before the acceptance run.
- Authorized capability is not disabled: model streaming/retry code paths,
  configured-integration/storage tests and authorized scheduled/subscriber code
  paths are covered by existing fake-only regressions re-run in this batch.
- A local task/attachment workflow still works while the external network is
  unreachable, and the Linux API actually starts, migrates, serves login/
  workspace/issue APIs and a realtime WebSocket on Linux.

Not claimed:

- Zero model traffic. Model requests are allowed and can include task context.
- A machine-wide network firewall or a "all data stays local" product promise.
  The recorder is scoped to the isolated acceptance deployment.
- Mobile (excluded by the plan), real agent CLI runs, real model accounts, real
  email/message delivery, real integration accounts or real update downloads.
- Cross-machine login/realtime: no second authorized client machine was
  available. The Windows deployment was verified on the same host only.
- Electron request observation: outstanding (see [Desktop and PWA](#desktop-and-pwa)).
  Tray menu: this codebase has no Electron tray implementation at all.

## Isolated environment

Nothing in this run reads or writes the machine's existing deployment, its
database, credentials or environment files. The acceptance deployment is
created outside the checkout:

| Resource | Value |
| --- | --- |
| Deployment root | `C:\Users\HuaWei\uw80-acceptance\native` (outside the repository, removed after the run) |
| PostgreSQL cluster | fresh `initdb` cluster in `<root>\data\postgres` on 127.0.0.1:16432 |
| Database | `uw80_acceptance` |
| Go API | 127.0.0.1:18081 (pid 75828 during the recorded run) |
| Next.js production server | 127.0.0.1:13001 (pid 75888) |
| Recording proxy | 127.0.0.1:19090 (pid 34888) |
| Credentials | random JWT secret and random database password generated for this run, stored only in the isolated root's `deployment.json`, never in the repository or evidence |

The machine already runs a different U Workers deployment (PostgreSQL 15432,
API 8080, web 3000). It was left running and untouched. This run only reused
that deployment's PostgreSQL binaries directory for a new cluster; it never
connected to that deployment's cluster or database and never copied its
`.env` or credentials. A marker file `acceptance-root.json` inside the
isolated root records that it is a generated acceptance environment; the
Playwright spec refuses to run unless the marker exists and unless
`PLAYWRIGHT_BASE_URL`, `NEXT_PUBLIC_API_URL` and `DATABASE_URL` all point at
the ports/database named by that root's `deployment.json`.

## Build products

The acceptance deployment ran real build output, not development servers:

- A temporary local tag `v0.2.0` was created so `scripts/native-windows.ps1`
  could stamp a version, because the repository has no tags and the build path
  requires one. The tag was deleted after the run; no branch was changed and
  nothing was pushed.
- `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/native-windows.ps1 -Action Build -Root <isolated root>`
  produced `server.exe`, `migrate.exe`, migrations and the Next.js standalone
  production server (`STANDALONE=true next build`), version
  `v0.2.0 (32099218a)`.
- The build emitted two benign PostCSS warnings about `::highlight(...)` in
  generated CSS; no TypeScript or build errors were reported.

## Runbook used for the recorded evidence

The evidence was produced with the direct steps below (there is no committed
PowerShell driver; an earlier draft driver was removed because it never ran
end-to-end). Keep every path/port/database separate from any real deployment.

1. Create an isolated root outside the repository with `downloads/`,
   `runtime/`, `releases/`, `logs/`, `state/`, `data/`, `uploads/`, point
   `runtime/pgsql` at an existing PostgreSQL binaries directory (read-only use),
   and write `deployment.json` (root, repo, random database user/password, ports
   `16432/18081/13001`, generated JWT secret, backend/frontend environment maps
   including loopback-bypassing proxy variables) plus `acceptance-root.json`.
2. `scripts/native-windows.ps1 -Action Build -Root <root>` (with a temporary
   local version tag) and then start the cluster/application database once:
   PostgreSQL via `pg_ctl start`, then `psql ... -c 'CREATE DATABASE uw80_acceptance ...'`.
3. Start the recorder:
   `node scripts/e2e/recording-proxy.mjs --port 19090 --log <root>/logs/recording-proxy.jsonl`.
4. Run the Node and Go observer controls with `HTTP(S)_PROXY` pointing at the
   recorder and `NEXT_PUBLIC_API_URL=/NO_PROXY` covering loopback, then write
   `<root>/observer-controls.json` with the recorded/exit-code results and the
   backend/frontend proxy env keys.
5. Start the deployment: `scripts/native-windows.ps1 -Action Start -Root <root>`.
   (Launch shells must not carry both `NO_PROXY` and `no_proxy`: Windows
   environment names are case-insensitive and .NET refuses to create a child
   process from that duplicated environment block.)
6. Export the test environment and run:
   ```
   PLAYWRIGHT_BASE_URL=http://localhost:13001
   NEXT_PUBLIC_API_URL=http://127.0.0.1:18081
   DATABASE_URL=postgresql://<isolated user>:<isolated password>@127.0.0.1:16432/uw80_acceptance?sslmode=disable
   MULTICA_DEV_VERIFICATION_CODE=888888
   E2E_PROXY_URL=http://127.0.0.1:19090
   E2E_PROXY_LOG=<root>/logs/recording-proxy.jsonl
   E2E_PROXY_PID=<recorder pid>
   E2E_SERVER_PID=<api pid>  E2E_WEB_PID=<web pid>
   E2E_ACCEPTANCE_ROOT=<root>
   E2E_OBSERVER_CONTROLS=<root>/observer-controls.json
   E2E_EVIDENCE_DIR=<repo>/evidence/clawsta-80/acceptance
   pnpm exec playwright test e2e/background-network.spec.ts e2e/u-workers-brand.spec.ts --project=chromium --reporter=line
   ```
7. Stop the deployment and recorder by their recorded PIDs, then remove the
   isolated root/cluster. The spec's final test stops the recorder on purpose to
   create the external-network-unreachable scenario; restart it between runs.

## Observation layers

`scripts/e2e/recording-proxy.mjs` is a refusing HTTP proxy used only for this
acceptance run. It records destination metadata and answers every non-loopback
request with 502, closing the connection. Nothing is forwarded, so an attempted
unsolicited request fails instead of being silently swallowed. It records only
the target host (no userinfo, query or path), HTTP method, trigger, validated
observer marker and result; request headers (including cookies and user agents)
and bodies are never written. A log write failure makes the recorder exit
non-zero rather than leaving partial evidence behind.

| Layer | How it is observed | Why it cannot fake a pass |
| --- | --- | --- |
| Browser requests, WebSockets, service workers, navigation | Playwright context/page events plus the context proxy; all stored coordinates are sanitized to scheme+host at capture | The same context sends exactly two deliberate self-test requests (`uw80-browser-selftest.invalid` HTTP and `uw80-browser-selftest-tls.invalid` HTTPS/CONNECT). The test fails unless the recorder saw both exact hosts and both failed in the browser; only those exact origins are then removed from the business observation. |
| Next.js (Node) server process | Deployment environment carries `NODE_USE_ENV_PROXY=1`, `HTTP(S)_PROXY` at the recorder, `NO_PROXY` for loopback | A Node control request with the same environment contract must be recorded and refused before the deployment starts. The deployment's proxy env keys are recorded in `observer-controls.json`. |
| Go API process | Same proxy environment, honored by Go's HTTP proxy support; a Go control binary built from the same checkout is run with the same environment | The Go control must be recorded and refused. (Implementation note: the control binary is built and then executed rather than using `go run`, so the control depends on exactly the same environment block as a normal process.) |
| All service PIDs | A `netstat -ano` TCP-table sample of the API and web PIDs after every online scenario | Independent of proxy configuration: a process that bypassed the proxy would still show a connection. This is a sampling backstop, not exhaustive process-egress proof; the spec fails if the sample cannot be taken, and reports that boundary explicitly. |

Only exact observer destinations are exempted: `uw80-node-control.invalid:80`,
`uw80-go-control.invalid`, `uw80-browser-selftest.invalid` and
`uw80-browser-selftest-tls.invalid:443`. There is no wildcard ".invalid is
fine" rule and no arbitrary marker-header exemption; markers are only written
for the fixed values `node`, `go`, `browser-http`, `browser-tls`.

## Scenarios and results

Command: the Playwright command from the runbook
(`pnpm exec playwright test e2e/background-network.spec.ts e2e/u-workers-brand.spec.ts --project=chromium --reporter=line`).
Result: **10 passed, 1 skipped, exit 0** in 37.9 s. The skipped test is the
packaged-Electron identity test (no launchable packaged artifact; see below).

| Scenario (spec test) | Observation result |
| --- | --- |
| Observation layers live: exact Node/Go controls + browser HTTP/HTTPS self-tests | Recorded and refused as expected; deployment proxy env keys present |
| Cold start on `/login` | 0 proxy violations, 0 external browser requests, TCP sample available, 0 non-loopback TCP connections |
| Email-code login (real UI form, dev verification code, no email provider) | 0 / 0 / available / 0 |
| Browse Issues, Inbox, Agents, Runtimes, Settings | 0 / 0 / available / 0 |
| Switch to a second workspace through the real switcher | 0 / 0 / available / 0 |
| Idle 8 s on a live workspace view | 0 / 0 / available / 0; a loopback WebSocket was connected |
| External network unreachable: local issue create + attachment upload | Issue created through the UI; attachment upload through the local API returned 200; external sentinel unreachable |

Recorder log during the recorded run: 20 entries, all expected observer markers
or startup records, zero unexpected destinations. Observed traffic in the
browser was 2,652 requests over the scenarios, all loopback; zero external
requests, zero external WebSockets, zero external navigations, zero service
workers. Evidence: `evidence/clawsta-80/acceptance/browser-observation.json`,
`offline-scenario.json`, `run-summary.json`, `observer-controls.json`.

Notes:

- The authenticated shell keeps a live WebSocket and cache polling; the spec
  uses bounded settle windows instead of `networkidle` for authenticated
  scenarios, and the recorder still proves the direction of every request.
- A production-build title observation: client-side navigation to Inbox/Agents
  falls back to the branded root document title instead of the per-route title
  in this standalone build. The brand test therefore asserts U Workers identity
  on every route (the root title is branded) and records this pre-existing
  production-only behavior here rather than silently accepting a stale upstream
  brand. It is not introduced by this task and does not change the network
  boundary.

## Authorized capability remains

Fake-only regressions were re-run from `server/` (no real model, agent,
integration account, email recipient or installer):

- `go test ./internal/privacy ./internal/analytics ./pkg/llm -count=1` — the
  `pkg/llm` test runs the real SDK against a fake non-loopback configured model
  URL with a retryable response followed by a successful stream, proving model
  streaming/retry still works and is not part of the zero-unsolicited boundary.
- Handler subscriber/revocation and autopilot dispatch tests (scheduled/
  subscription lifecycle) were run against the isolated acceptance database:
  `go test ./internal/handler -run 'TestCreateAutopilotPersistsMemberSubscribers|TestAutopilotSubscriberSave_LosesToConcurrentRevoke|TestAutopilotDispatchFansOutSubscribersToIssue|TestAutopilotDispatchSkipsInboxWhenNoSubscribers|TestDeleteAutopilotArchivesAndPreservesHistory' -count=1`.
- Unconfigured daemon-profile provisioning never starts:
  `go test ./internal/daemon -run 'TestProvisionDshMulticaProfile_UnconfiguredIsANoOp|TestStartDshProfileProvision_UnconfiguredNeverStarts' -count=1`.
- Deployment startup log shows every optional channel/Composio/VCS integration
  as disabled because no credentials are configured, while the local scheduler
  starts its database-only maintenance jobs; the recorder stayed silent through
  startup.

## Linux actual startup

Environment: Ubuntu 24.04 on WSL2 (`Linux 6.6.87.2-microsoft-standard-WSL2`,
x86_64), 2026-10-31. PostgreSQL 16.15 server binaries were extracted from the
Ubuntu archive into a user-local directory and `initdb` created a brand-new
temporary cluster in `$HOME/uw80-pg/data` on 127.0.0.1:16433 (own data directory,
no shared service, no existing database). Go 1.26.9 was downloaded to
`$HOME/uw80-go` and the API/migrate binaries were built from this checkout.

The Linux API applied migrations against `uw80_linux_acceptance`, started on
127.0.0.1:18082 with its own recording proxy on 127.0.0.1:19091, and:

- `/health` and `/readyz` returned 200;
- email-code login, workspace creation, issue creation and issue read-back
  succeeded through the real HTTP API (dev verification code, no email provider);
- a realtime WebSocket authenticated with the token frame and received
  `{"type":"auth_ack"}`;
- the Go observation-layer control was recorded and refused (WSL's Node 22 does
  not enable env-proxy support for `fetch`, so the Node-layer control is covered
  by the Windows run);
- the Linux Go API produced zero unsolicited recorder entries.

Evidence: `evidence/clawsta-80/linux-acceptance.json`. Linux processes were
stopped by their recorded PIDs (`api 4782`, `recording proxy 4692`); the
PostgreSQL cluster was stopped and its data directory removed during cleanup.

Boundary: the Next.js production server was not rebuilt and served from Linux in
this run. The Windows acceptance deployment covers the production web server;
the Linux run covers the Go API and its actual Linux process, migrations and
realtime path. A built Linux binary alone would not have been treated as
"Linux startup"; this run started and exercised the process.

## Windows deployment and cross-machine access

The isolated Windows deployment was built and supervised with
`scripts/native-windows.ps1` and verified through the production browser
surface on the same host: real login, workspace browsing, workspace switching,
WebSocket realtime and zero unsolicited egress.

Cross-machine login and realtime require a second authorized client machine.
None was available for this run, and this machine's own IP or the WSL instance
is not a substitute for one. The existing deployment's LAN/HTTPS entry points
were left untouched as required. **This remains an external blocker for the
task, not a pass.**

## Desktop and PWA

- `pnpm --filter @multica/desktop build` (electron-vite main/preload/renderer
  bundles) succeeded.
- `pnpm --filter @multica/desktop package -- --win --dir` was attempted once.
  It produced `apps/desktop/dist/win-unpacked/U Workers.exe` and a large
  `app.asar`, but the packaging command failed at electron-builder's winCodeSign
  extraction (`Cannot create symbolic link ... libcrypto.dylib ... client does
  not hold the required privilege`). Because that step failed before version
  metadata rewriting, the produced `U Workers.exe` still reports
  `ProductName: Electron` in its file version info, so the OS-level product
  identity was not accepted as verified.
- Exactly one Playwright `_electron.launch` of that unpacked executable was
  attempted. That historical attempt still forwarded the test process
  environment (`...process.env`); the explicit throwaway
  `APPDATA`/`USERPROFILE`/`HOME` sandbox with an allowlisted environment was
  added to the spec afterwards but the launch was never retried successfully
  (it timed out after 60 s before the sandbox version existed). The sandbox is
  the only supported way to run that test now; no further retries were made
  without a concrete launch fix. Desktop request observation and native
  install/tray/OS-icon verification are therefore **outstanding**, not silently
  skipped as passing; the brand spec skips the packaged-app test when no
  launchable artifact is supplied and states this boundary.
- Tray: `apps/desktop/src` contains no Electron `Tray` implementation in this
  codebase (searched the main-process sources). There is no tray menu surface
  here to brand or observe; that part of the plan's wording does not correspond
  to existing code.
- PWA: the web app manifest (name, short name, start URL, standalone display,
  shortcuts, 192/512/maskable icons), apple-touch metadata, theme color and
  bundled icons are verified against the production server in
  `e2e/u-workers-brand.spec.ts`. A real OS/browser install prompt was not
  exercised in the headless acceptance browser.

## Boundaries and unverified items

1. Cross-machine login/realtime and real LAN/TLS entry points: no second
   authorized client machine available. External blocker, not verified.
2. Electron request observation, native install, OS icon and tray: outstanding
   with the exact packaging/launch results above; no tray implementation exists.
3. Real agent runtimes, real model providers, real MCP/Git/integration accounts,
   real emails/messages and real update downloads: deliberately not used. Fake
   transports and tests cover the code paths.
4. Mobile: excluded by the plan.
5. Linux web frontend production server: not rebuilt on Linux; the Windows
   production server covered the web behavior.
6. Base checkout known failure: `node --test apps/desktop/scripts/package.test.mjs`
   fails while loading `.mjs` files that import Vitest APIs
   (`TypeError: Cannot read properties of undefined (reading 'config')`). This
   failure exists at the integration base, was reproduced in this batch, and is
   not counted as a pass nor as a Task 8 regression.

## Verification commands and results

Run from the repository root unless noted. Results recorded in this batch:

| Command | Result |
| --- | --- |
| `pnpm typecheck` | PASS — 10/10 packages, exit 0 |
| `pnpm lint` | PASS — 7/7 packages, 0 errors (warnings only), exit 0 |
| `pnpm test` | FAIL — `@multica/web` `app/text-contrast.test.ts` case `product UI expresses text hierarchy with tokens, not transparency` timed out (9.5 s) in the full parallel run. The same file passes in isolation (23/23 in 1.8 s) and on the warm-cache pipeline run; this is parallel-load flakiness, not a Task 8 change (this batch changes no product/web source). Recorded, not counted as a green check. |
| `pnpm test:dsh-web` | PASS — 40/40 tests, exit 0 |
| `go test ./internal/privacy ./internal/analytics ./pkg/llm -count=1` (server/) | PASS — reporting disabled gates and fake-transport model streaming/retry |
| `go test ./internal/handler -run '<autopilot subscriber/revocation tests>' -count=1` with `DATABASE_URL` at the isolated acceptance database | PASS — 5 tests/subtests (create subscribers, concurrent revoke, dispatch fan-out, no-subscriber skip, delete/archive) |
| `go test ./internal/daemon -run 'TestProvisionDshMulticaProfile_UnconfiguredIsANoOp|TestStartDshProfileProvision_UnconfiguredNeverStarts' -count=1` | PASS — unconfigured daemon profile never starts |
| `pnpm exec playwright test e2e/background-network.spec.ts e2e/u-workers-brand.spec.ts --project=chromium --reporter=line` | PASS — 10 passed, 1 skipped (packaged Electron artifact), exit 0, 37.9 s |
| Linux actual-startup run (WSL Ubuntu 24.04) | PASS — migrations, `/health`/`/readyz`, login/workspace/issue API, realtime WebSocket `auth_ack`, zero unsolicited recorder entries |
| `node --test apps/desktop/scripts/package.test.mjs` | FAIL — known integration-base loader failure, reproduced; not counted as a pass |

### Independent reviewer evidence

The reviewer independently executed, against the fake-only boundary:

- `go test -p 2 ./internal/daemon -run '^TestRemoteMCPProxy(FiltersToolsAndInjectsCredential|RejectsUnapprovedToolWithoutCallingUpstream|PreservesAcceptedNotificationStatus|RechecksCredentialBeforeUpstreamCall)$' -count=1`
  → PASS (0.792 s): authorized tool filtering/credential injection, unapproved
  tool rejection without upstream calls, notification status and credential
  recheck before an upstream call.
- `go test -p 2 ./internal/integrations/vcs ./pkg/composio -run '^(TestValidateToken|TestValidateTokenUnauthorized|TestExecuteTool_Success|TestRevokeConnection_Success|TestCreateSession_Success)$' -count=1`
  → PASS (vcs 3.659 s, composio 111.738 s). Precise scope: configured fake VCS
  HTTP auth and Composio tool execution/session/revocation. No real Git
  clone/push was observed.
- Recorder harness with synthetic authoritative-looking userinfo/query/path,
  `Authorization` and user-agent headers and an arbitrary observer marker:
  PASS — HTTP and CONNECT both refused with 502 and recorded; sensitive values
  absent from the metadata.
- `go test -p 2 ./internal/privacy ./internal/analytics ./pkg/llm -count=1`:
  PASS (package-level evidence; not deployed process network observation).
- Read-only readiness of an earlier isolated Windows instance: `/readyz` 200,
  unauthenticated `/api/me` 401, `/login` 200; same-host only.
- Linux runtime evidence reviewed: Linux x86-64 ELF API server, migration log
  ends `Done`, server log shows `/readyz`, send-code, verify-code, workspace
  and issue requests returning 200/201. WebSocket application-auth remained
  candidate-script evidence because the HTTP log alone does not prove frames.

The reviewer's full notes are in `reviewer-validation.txt` next to this
checkout (workdir parent), deliberately not committed.

### Executed evidence revision and post-run fixes

The acceptance run above and the committed `browser-observation.json` were
produced by the code revision before these small post-run fixes:

- duplicate per-page request listeners were removed so the request counter is
  exact (the zero-violation direction and every scenario assertion were already
  unaffected; only the request-count metadata was inflated);
- `sanitizeUrl` now keeps scheme + host only for all captured coordinates;
- the browser self-test no longer sends a custom preflight header;
- the unproven PowerShell acceptance driver was removed in favour of the
  runbook above.

Per the reviewer's completion boundary, the removed deployment was not
recreated to re-measure the listener count. The record above states this
plainly; a future run of the same two specs on a fresh isolated deployment
should be treated as the confirming run for the request-count metadata.

## Cleanup

Owned resources for the recorded run (all loopback, all stopped):

| Resource | PID / path | Cleanup |
| --- | --- | --- |
| Windows API | 75828 (127.0.0.1:18081) | `native-windows.ps1 -Action Stop`, then verified port closed |
| Windows Next.js server | 75888 (127.0.0.1:13001) | same |
| Isolated PostgreSQL | 127.0.0.1:16432 | shut down by the native Stop action, port verified closed |
| Recording proxy | 34888 (127.0.0.1:19090) | stopped by the spec's offline scenario / recorded PID; port verified closed |
| Isolated deployment root | `C:\Users\HuaWei\uw80-acceptance` | removed recursively. A literal `NUL` file created by `pg_ctl`'s Windows redirection blocked the first removal and was deleted with the `\\?\` path prefix. |
| Linux WSL cluster | `$HOME/uw80-pg` (127.0.0.1:16433) | stopped with `pg_ctl`; temporary binaries/data/Go/API directories removed |
| Temporary local Git tag | `v0.2.0` | deleted (`git tag -d`); no branch changed, nothing pushed |
| Desktop packaging output | `apps/desktop/dist` | removed after the one packaging attempt |
| DSH background jobs | — | none left running after this batch |

After cleanup the four isolated ports had no listeners; the only remnant was a
short-lived `TIME_WAIT` socket. A later attempt to recreate the deployment to
re-measure the listener-count fix was stopped and its partial root removed, per
the reviewer's boundary; it produced no acceptance result and is not counted.

## Post-run corrections applied

- The attempted PowerShell acceptance driver was removed rather than repaired:
  it never ran end-to-end and its evidence was produced by the direct runbook
  commands above. It is not attributed as a passing driver anywhere.
- The earlier draft prose claiming the `go run` env behavior caused the control
  failure was replaced by the runbook's neutral build-then-run note; the actual
  root cause was the missing environment assignment now documented as a caution
  in the runbook.
- The historical Electron attempt inherited the process environment; the spec's
  current sandbox/allowlist environment is the only supported form and was never
  successfully launched. No Electron retry was made without a concrete fix.
