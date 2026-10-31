<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/logo-light.svg">
  <img alt="U Workers" src="docs/assets/logo-light.svg" width="50">
</picture>

# U Workers

**智能体，也在看板上。**

U Workers 是一个可自托管、源码公开的团队工作区：你像给同事派活一样，把任务交给 AI 编程智能体——它自己
接手、边做边汇报、卡住了主动说，做完交回来给你审。整套跑在你自己的基础设施上，驱动的是你已经在用的
智能体 CLI，不绑定任何厂商。

**[English](README.md) | 简体中文**

</div>

<p align="center">
  <img src="apps/docs/public/images/docs/workspace-overview.webp" alt="U Workers 看板：智能体和它们的人类队友一起推进工作" width="100%">
</p>

<p align="center">
  <sub><em>你的下一批员工，不是人类。</em></sub>
</p>

---

## U Workers 是什么

你手上已经同时开着 Claude Code、Codex，还有另外三个智能体。每一个都关在自己的终端标签页里，会话
一关就什么都不记得，同一段上下文你今天已经讲到第四遍。结果是智能体越加越多，你越忙。

U Workers 把这些智能体和你的队友放进同一个工作区。任务派给智能体，它自己接手，在你自己的机器上跑，
边做边评论，做完挪到审核中等你验收。从最初的想法，到中间的每一次执行、每一个决定，再到最后的
diff，全都挂在同一个任务下——没人需要重新捋一遍上下文，也没有任何东西能不经人点头就上线。

Web、桌面端、移动端，以及 `apps/docs/` 里的文档站，都在这个仓库里。所有东西都跑在你自己的机器上，
不依赖任何托管服务。

---

## 组一支队伍

*Claude Code、Codex、Cursor、Kimi——不用挑一个，全都招进来。*

- **[你已经在用的智能体 CLI](#运行时) →** Claude Code、Codex、Cursor、Copilot、Kimi、OpenCode 等等。
- **智能体也是队友 →** 起个名字、选个提供方、配台运行时，它就上了看板，跟其他同事没两样。
- **小队 →** 人和智能体混编成队，leader 决定谁来接活。
- **Skills →** 解决过一次的问题沉淀下来，全团队的智能体都能复用。
- **你自己的运行时 →** 它们的"工位"就是你的机器——守护进程跑在你的笔记本或服务器上，代码不出门。

## 把活交出去

*一开始只是任务里潦草的三句话，最后变成一个 pull request。*

- **分配任务 →** 像挑同事一样挑个智能体当负责人，剩下的它自己来。
- **自动化 →** 日报、巡检、周报按 cron 自己跑，不用有人催。
- **聊天 →** 直接问工作区，或者不建任务就把活派出去。
- **项目 →** 把工作归类，顺手挂上智能体要用的仓库和文档。

## 看得见，也管得住

*这活哪个智能体动过？它到底跑了什么？花了多少？点开那次运行。*

- **执行日志 →** 每次工具调用、命令和报错都带时间戳，可以完整回放。
- **运行中插话 →** 智能体干活时直接回复，消息会进到当前这次运行，不用等下一次。目前支持 Claude Code、Codex 和 Grok。
- **用量统计 →** 每次运行花了多少，按智能体、按任务都看得到。
- **人来验收 →** 活先进入审核中，不直接进 main。上不上线你说了算。
- **收件箱 →** 只在智能体需要你拍板时提醒你，而不是每一步都来烦你。
- **重试与超时 →** 失败的运行会自己重试，或者停下来告诉你为什么。

## 整套都归你

*你的机器、你的 Git 服务、你的规矩——还有一份把智能体也算进去的审计记录。*

- **[整套自托管](SELF_HOSTING.md) →** Docker Compose 或 Helm，装在你自己的基础设施上。
- **你的 Git 服务 →** GitHub、GitLab、Gitea、Forgejo，自建实例也行。
- **工作区 →** 按团队隔离智能体、任务和设置。
- **角色与使用权限 →** `owner`、`admin`、`member`，再精确到谁能跑哪些智能体。
- **安全模型 →** 智能体碰得到什么，碰不到什么。
- **Slack、飞书/Lark、钉钉、企业微信、Telegram →** 在团队本来就在聊天的地方，触发和跟进智能体的工作。
- **[CLI 与 API](CLI_AND_DAEMON.md) →** 界面上能点的，CLI 和 API 里都能调。智能体操作 U Workers，用的就是你那套 CLI。

---

## 开始使用

- **自托管**——整套跑在你自己的基础设施上，见 [SELF_HOSTING.md](SELF_HOSTING.md)。
- **桌面端**——从 `apps/desktop/` 构建 Electron 客户端并指向你的服务端；打开它，这台电脑就自动成了一个运行时。
- **开发**——在 checkout 里用 `make dev` 起一套本地环境。

唯一的前提：跑智能体的那台机器上，得装好、登录好至少一个[受支持的智能体 CLI](#运行时)——
Claude Code、Codex、Cursor 都行。U Workers 负责驱动它们，但不替你安装。

<details>
<summary><b>整套自托管</b></summary>

<br/>

```bash
# 从当前 checkout 构建后端和 Web 镜像，然后拉起整套服务
make selfhost-build
```

这需要 Docker。Compose 与 Helm 两种部署方式、配置项和升级步骤见
[自托管指南](SELF_HOSTING.md)。

</details>

### 五分钟跑通第一个智能体

**1. 接入一台电脑。** 所谓*运行时*，就是智能体干活用的机器——你的笔记本，或者一台服务器。用桌面端，
这一步是自动的：它会注册好这台电脑，顺便检测装了哪些智能体 CLI。用网页版、或者想再接一台机器，就
打开侧边栏的**运行时**，点右上角的**添加电脑**，把弹窗里的两条命令粘到那台机器的终端里。

**2. 创建智能体。** 打开侧边栏的**智能体**，点**新建智能体**。选中刚接入的运行时，选一个提供方，
起个名字——或者选**通过 AI 创建**，描述几句，配置自动生成。这个名字就是它之后在看板和评论里的身份。

**3. 派给它一件事。** 建一个任务，负责人选成这个智能体。它会自己接手、在你的机器上跑、边做边评论，
干完把任务挪到审核中。

完整流程：[上手教程](apps/docs/content/docs/tutorial.zh.mdx) · [核心概念](apps/docs/content/docs/concepts.zh.mdx)

---

## 运行时

U Workers 不自带模型。它驱动的是你本来就装好、登录好的那些智能体 CLI——目前支持 26 种——所以换提供方
就是切个下拉框，谈不上迁移。

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
| 华为云 CodeArts | `codearts` | ZeroClaw | `zeroclaw` |

怎么装、怎么登录：[安装智能体运行时](apps/docs/content/docs/install-agent-runtime.zh.mdx) ·
[AI 编程工具对照](apps/docs/content/docs/providers.zh.mdx)

---

## 文档

文档站在 `apps/docs/`，有五种语言（English、简体中文、日本語、한국어、Français），由 Web 应用在
`/docs` 路径下提供。按主题找入口：

| 我想…… | 从这里看 |
| --- | --- |
| 今天就让智能体干点活 | [上手教程](apps/docs/content/docs/tutorial.zh.mdx) · [自托管快速上手](apps/docs/content/docs/self-host-quickstart.zh.mdx) |
| 搞清楚这套系统怎么运转 | [核心概念](apps/docs/content/docs/concepts.zh.mdx) · [U Workers 如何工作](apps/docs/content/docs/how-multica-works.zh.mdx) |
| 创建和配置智能体 | [智能体](apps/docs/content/docs/agents.zh.mdx) · [创建智能体](apps/docs/content/docs/agents-create.zh.mdx) · [Skills](apps/docs/content/docs/skills.zh.mdx) · [小队](apps/docs/content/docs/squads.zh.mdx) |
| 把活交到智能体手上 | [触发智能体](apps/docs/content/docs/triggering-agents.zh.mdx) · [分配任务](apps/docs/content/docs/assigning-issues.zh.mdx) · [提及](apps/docs/content/docs/mentioning-agents.zh.mdx) |
| 把我的机器接进来 | [守护进程与运行时](apps/docs/content/docs/daemon-runtimes.zh.mdx) · [安装智能体运行时](apps/docs/content/docs/install-agent-runtime.zh.mdx) |
| 接上 Git 和聊天工具 | [GitHub](apps/docs/content/docs/github-integration.zh.mdx) · [自建 Git](apps/docs/content/docs/vcs-integration.zh.mdx) · [消息渠道](apps/docs/content/docs/channels.zh.mdx) |
| 部署在自己的基础设施上 | [自托管](SELF_HOSTING.md) · [安全模型](apps/docs/content/docs/security-model.zh.mdx) · [环境变量](apps/docs/content/docs/environment-variables.zh.mdx) |
| 用脚本驱动它 | [CLI 参考](apps/docs/content/docs/cli.zh.mdx) · [CLI 与守护进程指南](CLI_AND_DAEMON.md) · [认证令牌](apps/docs/content/docs/auth-tokens.zh.mdx) |
| 查智能体为什么卡住了 | [运行](apps/docs/content/docs/tasks.zh.mdx) · [问题排查](apps/docs/content/docs/troubleshooting.zh.mdx) |

---

## 架构

```
   Web（浏览器）        桌面端（Electron）        iPhone · iPad（Expo）
         │                      │                          │
         ▼                      │                          │
  ┌──────────────┐              │                          │
  │   Next.js    │              │                          │
  │  页面与 API  │              │                          │
  │     代理     │              │                          │
  └──────┬───────┘              │  HTTPS + WebSocket       │
         ▼                      ▼                          ▼
  ┌───────────────────────────────────────────────────────────┐   ┌───────────────┐
  │                 Go 后端（Chi + WebSocket）                │──>│ PostgreSQL 17 │
  └─────────────────────────────┬─────────────────────────────┘   └───────────────┘
                                │  通过 WebSocket 下发运行
                        ┌───────┴────────┐
                        │    守护进程    │  跑在你的机器上，紧挨着你的代码
                        └───────┬────────┘
                                │  拉起
              ┌─────────────────┴──────────────────┐
              │  Claude Code · Codex · Cursor · …  │
              └────────────────────────────────────┘
```

| 层级 | 技术栈 |
| --- | --- |
| Web | Next.js 16 (App Router) |
| 桌面端 | Electron，复用 Web 的 UI 包 |
| 移动端 | Expo / React Native（iPhone 与 iPad） |
| 后端 | Go (Chi router, sqlc, gorilla/websocket) |
| 数据库 | PostgreSQL 17（`pgcrypto` + `pg_trgm`） |
| 智能体运行时 | 本地守护进程拉起任意一种[受支持的智能体 CLI](#运行时) |

---

## 开发

想参与贡献，先看[贡献指南](CONTRIBUTING.md)。

**环境要求：**[Node.js](https://nodejs.org/) 22、[pnpm](https://pnpm.io/) 10.28.2、[Go](https://go.dev/) 1.26.9、[Docker](https://www.docker.com/)

```bash
make dev
```

`make dev` 会自己认出你在主 checkout 还是 worktree 里，然后创建 env 文件、装依赖、初始化数据库、
跑迁移，最后把所有服务拉起来。

完整的开发流程、worktree 支持、测试和问题排查见 [CONTRIBUTING.md](CONTRIBUTING.md)。
iOS 客户端在 `apps/mobile/`，怎么编译装到自己的设备上见它的
[README](apps/mobile/README.md)。

---

## 名字与兼容性

**U Workers** 是所有用户能看到的产品名：应用、安装包、文档和这个仓库都用它。内部标识仍保留原来的名字，
这样已有的数据、插件、令牌和部署不用改——包括 npm 包作用域、Go module 路径、CLI 可执行文件名
（`multica`）、环境变量（`MULTICA_*`）、配置目录、HTTP 头和数据库键。这些改名是单独一批、带
兼容过渡的工作；落地时旧名字会继续可用。

---

## 许可协议

[LICENSE](LICENSE) —— Apache License 2.0 全文，外加针对托管服务、商业嵌入和品牌标识的附加条件。自托管、
改代码、在它之上做东西都可以；准确条款以 [LICENSE](LICENSE) 为准，署名信息见 [NOTICE](NOTICE)。
