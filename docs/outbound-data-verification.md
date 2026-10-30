# Outbound data verification (Task 8 acceptance)

Status: acceptance run executed 2026-10-30/31 against the integration baseline
`32099218abac46865ab2e7d1cbb5b18532525e92`; the packaged Windows desktop
acceptance batch D1 was executed 2026-10-31 on top of `929e40690` (PR #3 head
`6b213959c`, branch `agent/agent/079e5e4a0861`). Results and the exact
boundary of what was and was not verified are recorded below. Policy:
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
- C1 follow-up: a second Windows machine serves a real LAN browser login,
  issue/attachment round-trip and business WebSocket update; its scheduled-task
  supervisor survives the starting SSH session. See the cross-machine section.
- The packaged Windows desktop build (batch D1) boots from an isolated
  `desktop.json`, logs in through its own UI against the isolated deployment,
  browses Issues/Inbox/Agents, idles 60 s and produces zero non-loopback
  traffic in all three observation layers, including the daemon the desktop
  auto-starts from its bundled CLI. The exe carries the `U Workers` product
  identity in its version resources.

Not claimed:

- Zero model traffic. Model requests are allowed and can include task context.
- A machine-wide network firewall or a "all data stays local" product promise.
  The recorder is scoped to the isolated acceptance deployment.
- Mobile (excluded by the plan), real agent CLI runs, real model accounts, real
  email/message delivery, real integration accounts or real update downloads.
- Company-network, HTTPS and Tailscale access: C1 tested the home LAN only.
- NSIS installation and post-install OS icon/start-menu association: not run.
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

### Desktop batch D1 isolated environment

The desktop batch reused the same isolation pattern in a separate, disposable
root:

| Resource | Value |
| --- | --- |
| Deployment root | `C:\Users\HuaWei\uw80-d1\native` (outside the repository, removed after the run) |
| PostgreSQL cluster | fresh `initdb` cluster on 127.0.0.1:16432, database `uw80_d1_acceptance` |
| Go API | 127.0.0.1:18081 |
| Next.js production server | 127.0.0.1:13001 |
| Recording proxy | 127.0.0.1:19090 |
| Desktop profile | redirected `USERPROFILE`/`HOME` at `<root>\home`; `<root>\home\.u-workers\desktop.json` points at the ports above |
| Packaged app | `apps/desktop/dist/win-unpacked/U Workers.exe` built from this checkout |

The machine's existing U Workers deployment (PostgreSQL 15432, API 8080, web
3000) was left running and untouched; this batch only reused its PostgreSQL
binaries for a new cluster. The desktop userData directory resolved under the
redirected profile (`<root>\home\AppData\Roaming\U Workers`) and the real
`%APPDATA%\U Workers` directory was never created. The Playwright spec refuses
to run unless the isolated root marker, the deployment ports and the launch
environment all agree.

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

The desktop batch built the packaged Windows app from the same checkout with
`pnpm --filter @multica/desktop package -- --win --dir`: exit 0, and
`apps/desktop/dist/win-unpacked/U Workers.exe` reports `ProductName: U Workers`
and `FileDescription: U Workers` (FileVersion `0.2.0-dirty`). The packaging
environment needed two fixes that are documented in
[windows-native.md](windows-native.md): a pre-extracted `winCodeSign` toolset
cache (7-Zip cannot create the macOS dylib symlinks on an unprivileged Windows
account) and bundling the `@multica/core` workspace TypeScript into the main
process instead of externalizing it (Node 24 refuses to strip types inside
`node_modules`, which killed the packaged main process at launch).

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
   recorder and `NO_PROXY` covering loopback, then write
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

### Desktop batch D1 runbook

1. Build the isolated deployment described above (`scripts/native-windows.ps1
   -Action Build`) and start it (`-Action Start`), with
   `MULTICA_DEV_VERIFICATION_CODE=888888` in its backend environment so the real
   login UI can be exercised without an email provider.
2. Start the recorder:
   `node scripts/e2e/recording-proxy.mjs --port 19090 --log <root>/logs/recording-proxy.jsonl`.
3. Clear the disposable desktop profile (`<root>\home\AppData\Roaming\U Workers`
   and `<root>\home\.multica`) and confirm `%APPDATA%\U Workers` does not exist.
4. Run the brand spec against the isolated deployment:
   ```
   PLAYWRIGHT_BASE_URL=http://localhost:13001
   NEXT_PUBLIC_API_URL=http://127.0.0.1:18081
   DATABASE_URL=postgresql://<isolated user>:<isolated password>@127.0.0.1:16432/uw80_d1_acceptance?sslmode=disable
   MULTICA_DEV_VERIFICATION_CODE=888888
   E2E_PROXY_URL=http://127.0.0.1:19090
   E2E_PROXY_LOG=<root>/logs/recording-proxy.jsonl
   E2E_ACCEPTANCE_ROOT=<root>
   E2E_DESKTOP_EXE=<repo>/apps/desktop/dist/win-unpacked/U Workers.exe
   E2E_DESKTOP_EVIDENCE_DIR=<repo>/evidence/clawsta-80/desktop
   pnpm exec playwright test e2e/u-workers-brand.spec.ts --project=chromium --reporter=line
   ```
5. The packaged app is launched through Playwright with a constructed
   environment: only system essentials, `USERPROFILE`/`HOME` at the isolated
   home, `HTTP_PROXY`/`HTTPS_PROXY` at the recorder, `NO_PROXY` for loopback and
   `NODE_USE_ENV_PROXY=1`. Its working directory is the isolated home, outside
   this workspace: the bundled CLI refuses `daemon start` when it detects a
   daemon-managed task marker in a parent directory (this agent workspace has
   one), and the daemon must be allowed to auto-start for the observation to
   cover it.
6. The spec writes `evidence/clawsta-80/desktop/login.png`, `main.png` and
   `run-summary.json`, removes the disposable userData directory and stops the
   daemon PIDs it discovered for the isolated profile.
7. Stop the deployment (`-Action Stop`) and the recorder by their recorded
   PIDs, then remove the isolated root.

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

The desktop batch adds a packaged-process variant of the same three-layer
observation, and includes the daemon the desktop starts from its bundled CLI:

| Layer | How it is observed | Why it cannot fake a pass |
| --- | --- | --- |
| Chromium network (packaged app) | The app launches with `--proxy-server=http://127.0.0.1:<recorder>` and an explicit loopback bypass list; Playwright context/page request, websocket, navigation and failed-request events are recorded | Two exact self-test destinations are fetched from the packaged renderer before the window: the recorder must record both and the renderer must receive the recorder's 502 refusal body (HTTPS self-test fails at the tunnel). The desktop renderer runs with `webSecurity: false`, so the refusal is asserted as status + recorder body, not as a rejected promise. |
| Main process and child processes (packaged app, including auto-started daemon) | The constructed main-process environment carries `HTTP(S)_PROXY`, `NO_PROXY` and `NODE_USE_ENV_PROXY=1`; children inherit it. The auto-started bundled CLI daemon is detached, so its exact PIDs are resolved from the isolated profile name and added as extra TCP roots | A Node control process with the same environment contract must be refused and recorded by the recorder; the main-process environment contract is read back from the running app; the daemon PIDs are recorded and sampled. |
| Windows TCP table (packaged app + daemon) | `Get-NetTCPConnection` sampled every second for the packaged process tree plus the detached daemon PIDs | A deliberate `SYN_SENT` to the reserved TEST-NET-1 address `192.0.2.1:80` must appear in the sample first; the spec fails unless the sampler sees it. During the 60 s window, non-loopback connections must be zero. |

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

### Desktop batch D1 results

Command:
`pnpm exec playwright test e2e/u-workers-brand.spec.ts --project=chromium --reporter=line`
against the isolated deployment with `E2E_DESKTOP_EXE` and
`E2E_ACCEPTANCE_ROOT` set. Result: **4 passed, exit 0** in 1.5 minutes (three web
brand tests plus the packaged-desktop acceptance test; the desktop test is no
longer skipped).

| Scenario (desktop test step) | Observation result |
| --- | --- |
| Control liveness (Chromium self-test, Node env-proxy control, system-layer TEST-NET-1 control) | All recorded and refused/observed as expected; main-process env contract present; 2 detached daemon PIDs detected and sampled |
| Boot from isolated `desktop.json` | Login UI shown, no configuration-error page; `app.getName()` is `U Workers`; exe `ProductName`/`FileDescription` are `U Workers`; userData resolved under the isolated profile |
| Real email-code login through the packaged UI | 0 recorder violations / 0 external browser requests / 0 non-loopback TCP samples; 50 requests to the isolated API; loopback WebSocket connected |
| Browse Issues, Inbox and Agents, then idle 60 s | 0 / 0 / 0; 26 TCP samples over 8 packaged processes plus both daemon PIDs, zero non-loopback connections; recorder log empty for the window |

Evidence: `evidence/clawsta-80/desktop/run-summary.json`, `login.png`,
`main.png`. The screenshots are full-desktop captures (DPI-aware) and include
the window title bar and the taskbar icon; unrelated always-on-top desktop
windows from other applications are hidden only for the capture and restored
immediately afterwards.

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

The initial run lacked a second device. Tane subsequently authorized the Chun
desktop, and C1 completed on 2026-10-31: client `192.168.3.162` connected to
`http://192.168.3.49:13001` on CHUN-I5-12400F-32G. The new deployment lives at
`D:\UWorkers\clawsta80`; API `18081` and PostgreSQL `16432` bind only loopback.
The existing deployment was untouched. Source baseline was `6b213959c`, with
C1 deployment changes delivered in `5e54bb54f` (adopted as `27e3ae088`).

- Real browser email-code login: send/verify 200; issue creation 201 and UI
  read-back; attachment upload/download matched SHA-256 (80 bytes).
- A second browser context updated the issue over the LAN (PUT 200, revision
  2 to 3). The first received `issue:updated` with `status_changed=true` and
  updated its UI within 160 ms. This is a business update, not only auth_ack.
- The hidden SYSTEM boot task `MulticaNative-D-UWorkers-clawsta80-native`
  owns the supervisor. A verified backend child recovered in 2.6 s; Stop/Start
  took 4.5/1.6 s. Fresh SSH and LAN GET checks after the initiating SSH exited
  remained healthy. A physical reboot was not performed.
- Backend review independently confirmed the same supervisor/backend/frontend
  process records, task running under SYSTEM, readyz and LAN login/health.
  The deployed runner SHA-256 matched C1 source before the focused preparation
  fixes in `scripts/native-windows.ps1`; those fixes passed local regressions
  and were copied to the target source and lifecycle runner after verifying
  both old/new hashes. They do not change the running two-origin configuration;
  no service restart or browser rerun was required. Review details are in
  `evidence/clawsta-80/cross-machine/05-backend-review.md`.
- All 47 standalone junctions resolve inside the target deployment root. Keep
  its source/dependency directory; another host requires a target-side rebuild.

Evidence: `evidence/clawsta-80/cross-machine/`; screenshots are attached to
CLAWSTA-81. Environment failures and reusable fixes are documented in
[deployment-environment-issues.md](deployment-environment-issues.md). This closes
the second-machine requirement. Company LAN, HTTPS and Tailscale were not tested.
The instance is retained for user review; lifecycle commands are in
[windows-native.md](windows-native.md#lan-access-and-persistence).

## Desktop and PWA

- `pnpm --filter @multica/desktop package -- --win --dir` now exits 0 from this
  checkout. `apps/desktop/dist/win-unpacked/U Workers.exe` reports
  `ProductName: U Workers`, `FileDescription: U Workers`, and the packaged app
  reports `app.getName() === "U Workers"`. The winCodeSign cache bypass and the
  workspace-TypeScript bundling fix are documented in
  [windows-native.md](windows-native.md).
- The packaged app launches through Playwright with a redirected
  `USERPROFILE`/`HOME`, reads the isolated `~/.u-workers/desktop.json`, shows
  no configuration-error page, logs in through the real login UI against the
  isolated deployment, browses Issues/Inbox/Agents and idles for 60 s. Across
  those steps the Chromium proxy layer, the main-process/child environment
  contract (including the auto-started bundled CLI daemon) and the per-second
  Windows TCP sample of the packaged process tree all report zero non-loopback
  traffic; the exe identity, the isolated config path and the login are
  asserted inside the test rather than observed manually. See
  [Desktop batch D1 results](#desktop-batch-d1-results).
- Isolation boundary: the desktop userData directory resolves under the
  redirected profile (`<root>\home\AppData\Roaming\U Workers`), not the real
  `%APPDATA%\U Workers`; the real roaming directory never existed before,
  during or after the run, and no real `~/.multica` desktop profile was
  created. The isolated userData and the auto-started daemon were removed at
  the end of the test.
- Known observation gaps recorded rather than hidden:
  - The task book's literal `--proxy-bypass-list=<-loopback>` removes
    Chromium's implicit loopback bypass, which would route the isolated API and
    WebSocket through the refusing recorder and break login. The launch uses an
    explicit loopback bypass list (`localhost;127.0.0.1;::1`) instead, which
    keeps only loopback direct — the stated intent.
  - The packaged app must run with its working directory outside this agent
    workspace: the bundled CLI refuses `daemon start` when it detects a
    daemon-managed task marker in a parent directory. This is a harness
    requirement, not a product change.
  - The tab system owns `document.title` after login, so the OS window title
    shows the localized route name (for example `Issues`) instead of the
    product name; the login window title is `U Workers`, and the product name
    is carried by the exe resources, `app.getName()` and the in-app brand mark.
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

1. Company-network and HTTPS/Tailscale entry points are unverified. C1 real
   second-machine LAN login, attachment and business realtime passed.
2. Native (NSIS) install and OS-level installed icon/start-menu association:
   not run in this batch. Tray: no implementation exists in this codebase.
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
| `pnpm test` | FAIL — `@multica/web` `app/text-contrast.test.ts` case `product UI expresses text hierarchy with tokens, not transparency` timed out (9.5 s) in the full parallel run. The file passed alone (23/23 in 1.8 s). Load sensitivity is a possible explanation; the full run is still recorded as failed. |
| `pnpm test:dsh-web` | PASS — 40/40 tests, exit 0 |
| `go test ./internal/privacy ./internal/analytics ./pkg/llm -count=1` (server/) | PASS — reporting disabled gates and fake-transport model streaming/retry |
| `go test ./internal/handler -run '<autopilot subscriber/revocation tests>' -count=1` with `DATABASE_URL` at the isolated acceptance database | PASS — 5 tests/subtests (create subscribers, concurrent revoke, dispatch fan-out, no-subscriber skip, delete/archive) |
| `go test ./internal/daemon -run 'TestProvisionDshMulticaProfile_UnconfiguredIsANoOp|TestStartDshProfileProvision_UnconfiguredNeverStarts' -count=1` | PASS — unconfigured daemon profile never starts |
| `pnpm exec playwright test e2e/background-network.spec.ts e2e/u-workers-brand.spec.ts --project=chromium --reporter=line` | PASS — 10 passed, 1 skipped (packaged Electron artifact), exit 0, 37.9 s |
| Linux actual-startup run (WSL Ubuntu 24.04) | PASS — migrations, `/health`/`/readyz`, login/workspace/issue API, realtime WebSocket `auth_ack`, zero unsolicited recorder entries |
| `node --test apps/desktop/scripts/package.test.mjs` | FAIL — known integration-base loader failure, reproduced; not counted as a pass |
| `pnpm --filter @multica/desktop package -- --win --dir` (batch D1) | PASS — exit 0; `U Workers.exe` ProductName/FileDescription `U Workers`, FileVersion `0.2.0-dirty` |
| `pnpm --filter @multica/desktop typecheck` (batch D1) | PASS — exit 0 |
| `pnpm --filter @multica/desktop test` (batch D1) | 644 tests passed; suite `scripts/package.test.mjs` fails to load (`SyntaxError: Invalid or unexpected token`) — the same base-checkout loader family recorded in item 6 above; neither that file nor its config was touched by this batch |
| `pnpm exec playwright test e2e/u-workers-brand.spec.ts --project=chromium --reporter=line` with the isolated desktop harness (batch D1) | PASS — 4 passed, exit 0, 1.5 min; the packaged desktop case ran (not skipped) |

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
- final review made malformed recorder JSON fail the verification, corrected
  IPv6 TCP address parsing and removed the unverified optional Desktop launch.
  These changes received a spec-loading check, not another deployed run.

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

### Desktop batch D1 cleanup

| Resource | PID / path | Cleanup |
| --- | --- | --- |
| Isolated acceptance deployment | `C:\Users\HuaWei\uw80-d1\native` (ports 16432/18081/13001, database `uw80_d1_acceptance`) | `native-windows.ps1 -Action Stop` exited 0; all four ports verified closed afterwards |
| Recording proxy | port 19090 | every run-owned recorder process stopped; port verified closed |
| Packaged app and bundled daemon | run-owned PIDs (the daemon PIDs were resolved per run from the isolated profile, e.g. `[25988, 43104]`) | app closed by Playwright; detached daemon PIDs stopped by exact PID, `daemonCleanup.remainingPids: 0` |
| Isolated desktop userData | `<root>\home\AppData\Roaming\U Workers` | removed by the test (`userDataRemovedAfterRun: true`); the real `%APPDATA%\U Workers` never existed before, during or after the run |
| Isolated acceptance root | `C:\Users\HuaWei\uw80-d1` | removed recursively; the parent directory is absent |
| Temporary local Git tag | `v0.2.0` (native build version stamp) | deleted with `git tag -d`; tag list is empty and nothing was pushed to a base branch |
| Desktop packaging output | `apps/desktop/dist` | retained as local build output only (gitignored); no artifact was published |

## Post-run corrections applied

- The attempted PowerShell acceptance driver was removed rather than repaired:
  it never ran end-to-end and its evidence was produced by the direct runbook
  commands above. It is not attributed as a passing driver anywhere.
- The earlier draft prose claiming the `go run` env behavior caused the control
  failure was replaced by the runbook's neutral build-then-run note; the actual
  root cause was the missing environment assignment now documented as a caution
  in the runbook.
- The historical Electron attempt inherited the process environment. Its
  unverified launch helper was removed; Desktop verification remains pending.
