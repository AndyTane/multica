# CLAWSTA-80 / C1 cross-machine deployment evidence (sanitized)

First production-native deployment of the Go API, Next.js production web and a
fresh PostgreSQL 17 cluster on a second Windows machine (CHUN-I5-12400F-32G),
reached from the client PC over the LAN. Only destination/result metadata is
recorded here: no credentials, login codes, cookies, tokens or production
traffic. Issue titles and IDs are synthetic test data created by the run.

- `01-browser-acceptance.json` — single Playwright run from the client PC
  against `http://192.168.3.49:13001`: real email-code login through the UI,
  issue create/read, attachment upload/download SHA-256 comparison, and a second
  browser session editing the issue while the first one proves it received the
  `issue:updated` business event and re-rendered the new title.
- `02-cross-machine-access.json` — a fresh SSH read plus LAN HTTP checks after
  the SSH session that started the deployment had exited.
- `03-persistence-lifecycle.json` — toolchain versions, listener isolation,
  scheduled-task description, LAN firewall rule contract, release junction
  portability check, supervisor restart proof (child killed and replaced) and a
  documented Stop/Start lifecycle run.
- `04-network-reachability.json` — outbound reachability measured on the
  deployment host before installing the toolchain.
- Browser screenshots from the same run are attached to the CLAWSTA-81 issue
  comment (kept out of the repository, like the earlier raw Playwright output).

Reproduce the browser half from any authorized client that can reach the web
port (Linux/macOS use `npx playwright`, Windows uses the bundled Chromium):

```text
1. Open http://<deploy-host>:<web-port>/login, request a code for the test user.
2. Read the code from the deployment's verification_code table, complete login.
3. Create an issue in the UI, upload an attachment, download it, compare SHA-256.
4. In a second session, update the issue title/status; confirm the first session
   receives `issue:updated` and shows the new title.
```

Boundary: this run covers the Windows host, the LAN path, one client machine and
a synthetic test workspace. Company-network reachability, a second client OS and
the desktop package are out of scope (see docs/deployment-environment-issues.md).
