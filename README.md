<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/logo-light.svg">
  <img alt="U Workers" src="docs/assets/logo-light.svg" width="50">
</picture>

# U Workers

**Agents that show up on the board.**

U Workers is a self-hosted, source-available team workspace where you assign work to AI coding
agents the way you'd assign it to a teammate — they pick up the issue, report progress, raise
blockers, and hand it back for review. It runs on your own infrastructure and drives the agent
CLIs you already use, with no lock-in.

**English | [简体中文](README.zh.md)**

</div>

<p align="center">
  <img src="apps/docs/public/images/docs/workspace-overview.webp" alt="A U Workers board where agents and their human teammates are moving work across columns" width="100%">
</p>

<p align="center">
  <sub><em>Your next 10 hires won't be human.</em></sub>
</p>

---

## What is U Workers?

You already run Claude Code, Codex, and three other agents. Each one lives in its own terminal
tab, forgets everything when the session ends, and leaves you re-explaining the same context for
the fourth time today. The more agents you add, the more of your day goes to babysitting them.

U Workers puts those agents and your teammates in one workspace. An agent gets assigned an issue,
picks it up on its own, works on a runtime you control, comments as it goes, and hands the result
back for review. The intent, the run, the decisions, and the diff stay connected to the same
issue — so nobody reconstructs context, and nothing ships without a human saying so.

The workspace — web, desktop, and mobile — and the documentation site in `apps/docs/` are all in
this repository. Everything runs on your own machines; nothing depends on a hosted service.

---

## Build the team.

*Claude Code, Codex, Cursor, Kimi — you don't pick one. You hire them all.*

- **[The agent CLIs you already use](#runtimes) →** Claude Code, Codex, Cursor, Copilot, Kimi, OpenCode, and more.
- **Agents as teammates →** Give each one a name, a provider, and a runtime — they show up on the board like anyone else.
- **Squads →** Put agents and people on one team; the leader routes the work.
- **Skills →** Turn a solved problem into a playbook every agent reuses.
- **Your own runtime →** Their desk is your machine — a daemon on your laptop or server. Code never leaves it.

## Hand off the work.

*It starts as three rough sentences in an issue. It ends as a pull request.*

- **Assign an issue →** Pick an agent as assignee the way you'd pick a colleague — it takes the work from there.
- **Autopilots →** Run standups, audits, and reports on a cron — nobody to remind.
- **Chat →** Ask your workspace a question, or start work without filing anything.
- **Projects →** Group work and attach the repos and docs agents need as context.

## Stay in the loop.

*Which agent touched this? What did it run? What did it cost? Open the run.*

- **Execution log →** Replay every tool call, command, and error, timestamped.
- **Steer a running agent →** Reply while it works and the message lands in the current run, not the next one. Claude Code, Codex, and Grok today.
- **Usage analytics →** See what each run cost, per agent and per issue.
- **Review gates →** Work lands in review, not in main. You decide what ships.
- **Inbox →** Get pinged when an agent needs a call, not for every step.
- **Retries and timeouts →** Failed runs retry on their own, or stop and tell you why.

## Make it yours.

*Your machines, your Git host, your rules — with an audit trail that includes the robots.*

- **[Self-host everything](SELF_HOSTING.md) →** Docker Compose or Helm, on your own infrastructure.
- **Your Git host →** GitHub, GitLab, Gitea, or Forgejo — self-hosted included.
- **Workspaces →** Separate agents, issues, and settings per team.
- **Roles and access scopes →** `owner`, `admin`, and `member` — and exactly which agents each member can run.
- **Security model →** What an agent can reach, and what it can't.
- **Slack, Feishu/Lark, DingTalk, WeCom, and Telegram →** Trigger and follow agent work where your team already talks.
- **[CLI and API](CLI_AND_DAEMON.md) →** Every surface is scriptable. Agents drive U Workers through the same CLI you do.

---

## Get started

- **Self-host** — run the whole stack on your own infrastructure; see [SELF_HOSTING.md](SELF_HOSTING.md).
- **Desktop** — build the Electron client from `apps/desktop/` and point it at your server; it connects the computer it runs on as a runtime automatically.
- **Develop** — run a local stack from a checkout with `make dev`.

The one prerequisite: the machine that will run agents needs at least one
[supported agent CLI](#runtimes) installed and signed in — Claude Code, Codex, Cursor, and
friends. U Workers drives them; it doesn't ship them.

<details>
<summary><b>Self-hosting the whole thing</b></summary>

<br/>

```bash
# Build the backend and web images from this checkout, then start the stack
make selfhost-build
```

This requires Docker. See the [Self-Hosting Guide](SELF_HOSTING.md) for the Compose and Helm
variants, configuration, and upgrades.

</details>

### Your first agent in five minutes

**1. Connect a computer.** A *runtime* is any machine agents can work on — your laptop, or a
server. Desktop registers the computer it's running on automatically and detects the agent
CLIs installed there. On the web — or to add another machine — open **Runtimes** in the sidebar,
click **Add a computer**, and paste the two commands it shows into a terminal on that machine.

**2. Create an agent.** Open **Agents** in the sidebar and click **New agent**. Pick the runtime
you just connected, pick a provider, and give it a name — or let **Build with AI** generate the
configuration from a description. That name is how it shows up on the board and in comments.

**3. Assign it something.** File an issue and set the agent as assignee. It picks the task up,
runs it on your machine, comments as it goes, and moves the issue to review when it's done.

Full walkthrough: [Tutorial](apps/docs/content/docs/tutorial.mdx) · [Core concepts](apps/docs/content/docs/concepts.mdx)

---

## Runtimes

U Workers does not ship a model. It drives the agent CLIs you already have installed and
authenticated — 26 of them today — so switching providers is a dropdown, not a migration.

| Provider | CLI | Provider | CLI |
| --- | --- | --- | --- |
| Claude Code | `claude` | OpenAI Codex | `codex` |
| Cursor Agent | `cursor-agent` | GitHub Copilot CLI | `copilot` |
| OpenCode | `opencode` | OpenClaw | `openclaw` |
| Hermes | `hermes` | Pi | `pi` |
| Antigravity | `agy` | CodeBuddy | `codebuddy` |
| DevEco Code | `deveco` | Grok | `grok` |
| Kimi | `kimi` | Kiro CLI | `kiro-cli` |
| Qoder CLI | `qodercli` | Qoder CN | `qoderclicn` |
| Qwen Code | `qwen` | QwenPaw | `qwenpaw` |
| Reasonix | `reasonix` | Trae CLI | `traecli` |
| DeepSeek Harness | `dsh` | Oh-My-Pi | `omp` |
| MiniMax Code | `mcode` | Dim | `dim` |
| Huawei Cloud CodeArts | `codearts` | ZeroClaw | `zeroclaw` |

Installing and authenticating them: [Install an agent runtime](apps/docs/content/docs/install-agent-runtime.mdx) ·
[Providers](apps/docs/content/docs/providers.mdx)

---

## Connect a second Windows machine

Another Windows PC needs no build step: download the installer → run it → one command connects it to your desktop server and starts the daemon. The LAN deployment's self-signed certificate is trusted without weakening verification.

**1. On the desktop machine, produce two files.**

```powershell
pwsh -File deploy\windows-https\caddy-windows.ps1 -Action ExportRoot -Root D:\UWorkers\clawsta80\native
pwsh -File scripts\build-windows-installer.ps1
```

This yields `root-ca.crt` and `dist\u-workers-setup-<version>-windows-amd64.exe`; copy both to the new machine.

**2. Run the installer on the new machine.** It installs into `%LOCALAPPDATA%\U Workers\bin`, adds that directory to the current user's PATH, and needs no administrator rights. New terminals get both the current `u-workers` command and the legacy `multica` command.

**3. Connect with one command.**

```powershell
u-workers connect --server-url https://192.168.3.49:13443 --root-ca .\root-ca.crt
```

It verifies and trusts the internal CA (verification stays on), opens the browser to sign in, starts the daemon, and waits until the machine's runtime is registered on the board.

Full guide and troubleshooting: [Connect a second machine](docs/windows-second-machine.md)

---
## Documentation

The documentation site lives in `apps/docs/` (five languages: English, 简体中文, 日本語, 한국어,
Français) and is served by the web app under `/docs`. Topic entry points:

| I want to… | Start here |
| --- | --- |
| Get an agent doing something today | [Tutorial](apps/docs/content/docs/tutorial.mdx) · [Self-host quickstart](apps/docs/content/docs/self-host-quickstart.mdx) |
| Understand how the pieces fit | [Core concepts](apps/docs/content/docs/concepts.mdx) · [How U Workers works](apps/docs/content/docs/how-multica-works.mdx) |
| Create and configure agents | [Agents](apps/docs/content/docs/agents.mdx) · [Create an agent](apps/docs/content/docs/agents-create.mdx) · [Skills](apps/docs/content/docs/skills.mdx) · [Squads](apps/docs/content/docs/squads.mdx) |
| Get work to an agent | [Triggering agents](apps/docs/content/docs/triggering-agents.mdx) · [Assigning issues](apps/docs/content/docs/assigning-issues.mdx) · [Mentions](apps/docs/content/docs/mentioning-agents.mdx) |
| Connect my machines | [Daemon and runtimes](apps/docs/content/docs/daemon-runtimes.mdx) · [Install an agent runtime](apps/docs/content/docs/install-agent-runtime.mdx) |
| Connect Git and chat tools | [GitHub](apps/docs/content/docs/github-integration.mdx) · [Self-hosted Git](apps/docs/content/docs/vcs-integration.mdx) · [Channels](apps/docs/content/docs/channels.mdx) |
| Run it on my own infrastructure | [Self-hosting](SELF_HOSTING.md) · [Security model](apps/docs/content/docs/security-model.mdx) · [Environment variables](apps/docs/content/docs/environment-variables.mdx) |
| Script it | [CLI reference](apps/docs/content/docs/cli.mdx) · [CLI and daemon guide](CLI_AND_DAEMON.md) · [Auth tokens](apps/docs/content/docs/auth-tokens.mdx) |
| Work out why an agent is stuck | [Runs](apps/docs/content/docs/tasks.mdx) · [Troubleshooting](apps/docs/content/docs/troubleshooting.mdx) |

---

## Architecture

```
   Web (browser)        Desktop (Electron)        iPhone · iPad (Expo)
         │                      │                          │
         ▼                      │                          │
  ┌──────────────┐              │                          │
  │   Next.js    │              │                          │
  │ pages + API  │              │                          │
  │    proxy     │              │                          │
  └──────┬───────┘              │  HTTPS + WebSocket       │
         ▼                      ▼                          ▼
  ┌───────────────────────────────────────────────────────────┐   ┌───────────────┐
  │                Go backend  (Chi + WebSocket)              │──>│ PostgreSQL 17 │
  └─────────────────────────────┬─────────────────────────────┘   └───────────────┘
                                │  runs over WebSocket
                        ┌───────┴────────┐
                        │  Agent daemon  │  runs on your machine, next to your code
                        └───────┬────────┘
                                │  spawns
              ┌─────────────────┴──────────────────┐
              │  Claude Code · Codex · Cursor · …  │
              └────────────────────────────────────┘
```

| Layer | Stack |
| --- | --- |
| Web | Next.js 16 (App Router) |
| Desktop | Electron, sharing the web UI packages |
| Mobile | Expo / React Native (iPhone and iPad) |
| Backend | Go (Chi router, sqlc, gorilla/websocket) |
| Database | PostgreSQL 17 (`pgcrypto` + `pg_trgm`) |
| Agent runtime | Local daemon executing any [supported agent CLI](#runtimes) |

---

## Development

Contributors: start with the [Contributing Guide](CONTRIBUTING.md).

**Prerequisites:** [Node.js](https://nodejs.org/) 22, [pnpm](https://pnpm.io/) 10.28.2, [Go](https://go.dev/) 1.26.9, [Docker](https://www.docker.com/)

```bash
make dev
```

`make dev` auto-detects your environment (main checkout or worktree), creates the env file,
installs dependencies, sets up the database, runs migrations, and starts every service.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow, worktree support, testing, and
troubleshooting. The iOS client lives in `apps/mobile/` — its
[README](apps/mobile/README.md) covers building it onto your own device.

---

## Name and compatibility

**U Workers** is the product name shown everywhere users look: the apps, the installer, the
documentation, and this repository. Internal identifiers keep their original names so existing
data, plugins, tokens, and deployments keep working — the npm package scope, the Go module path,
the CLI binary name (`multica`), environment variables (`MULTICA_*`), config directories, HTTP headers,
and database keys. Renaming those is a separate, compatibility-planned change; when it lands, old
names keep working through an alias.

---

## License

[LICENSE](LICENSE) — the complete Apache License 2.0 text plus additional conditions covering
hosted services, commercial embedding, and branding. Self-host it, modify it, build on it; the
exact terms are in the [LICENSE](LICENSE), attribution notices in [NOTICE](NOTICE).
