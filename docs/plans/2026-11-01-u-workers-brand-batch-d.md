# CLAWSTA-95 批次 D：runtime brief 与集成 bot 默认名

日期：2026-11-01
范围：CLAWSTA-85「批次 D」——逐次运行的 runtime brief 与 daemon prompt 文案、Slack / 企业微信
集成的默认 bot 名，以及文档站里跟随默认名一起改的 Slack manifest 与 @ handle（五种语言）。
启动前提是批次 C（PR #12）合并到 main `d0ee4c66`，命令名以 CLAWSTA-99 后端架构师的拍板为准。

品牌写法统一为 **U Workers**（带空格）。本批不动 CLI 终端文案、daemon 运行时原因字符串、
技能 ID、内部 key `mika`、「不做」清单里的标识。

## 新旧名字对照（批次 C 定下，本批照此写 brief）

| 类别 | 旧名（保留可用） | 新名（当前） | 本批在 brief 里怎么写 |
| --- | --- | --- | --- |
| 产品显示名 | Multica | U Workers | 直接写 U Workers |
| CLI 命令 | `multica` | `u-workers` | 两个都写：`u-workers` 是当前命令，`multica` 是同一二进制仍安装的旧名，命令完全一致；示例沿用 `multica`（原因见下） |
| 环境变量前缀 | `MULTICA_*` | `U_WORKERS_*` | 两个前缀都写，并写明两个都设时新名获胜 |
| daemon 注入给任务的变量 | `MULTICA_*` | 未改（本批未动） | 明确写「本版本注入的任务变量仍是 `MULTICA_*` 旧前缀」，和 `taskMulticaEnvironment` 的实际行为一致 |
| 配置目录 | `~/.multica` | `~/.u-workers` | 新装用 `~/.u-workers`，已有 `~/.multica` 的机器继续用旧目录、不自动迁移 |

## 改了什么

| 类别 | 内容 | 规模 |
| --- | --- | --- |
| runtime brief 标题与身份行 | `# Multica Agent Runtime` → `# U Workers Agent Runtime`；coding agent in the Multica platform → in U Workers | `runtime_config_sections.go` 2 处 |
| runtime brief 过渡说明（新增） | 头部新增两段：「CLI names」写清 `u-workers` 与 `multica` 同一二进制、示例为何沿用旧拼写；「Environment and config」写清环境变量双前缀与优先级、本版本任务变量仍是旧前缀、配置目录新旧规则 | 新增 2 段（约 250 tokens） |
| runtime brief 正文品牌 | Background Task Safety、On Behalf Of 凭证行、Session Continuity「stored it」、quick-create「existing issue」、autopilot「assigned issue」、Skills「platform action」、Attachments「resource URLs」、Always Use CLI「platform resources」、Output「can push a file」 | 9 处 |
| daemon 逐次 prompt | `buildPromptBody` / quick-create / comment / chat / autopilot 的 for a Multica workspace（7 处）、chat 的 not the Multica web app / NOT in Multica / Multica stores / Multica has no history reader、回复附件 Multica sends / Multica chat reply、autopilot assigned Multica issue | `prompt.go` 14 处 |
| DSH web bridge | 下发给 DSH 运行的 harness 指令改成 U Workers，并写明替换的可执行名是 `multica`（或新名 `u-workers`）；DSH 会话标题 `Multica · <thread>` → `U Workers · <thread>` | `bridge.mjs` 2 处 |
| 测试与快照 | `Multica Agent Runtime` 断言 11 处、On Behalf Of 凭证文案 1 处、`NOT in Multica` / `Never look in Multica issues` 断言 5 处、brief 夹具 1 处；新增 1 个 brief 过渡契约测试；bridge 测试补 2 条指令断言 + 会话标题断言 | 6 个测试文件 |
| Slack 线程引导 | 两条引导从固定文案改成按安装渲染：用安装记录里的 `bot_user_id` 输出真正的 `<@U…>` mention（Slack 显示该 app 当前名字，老安装 `@Multica`、新安装 `@U Workers`），mention 不放进反引号；没有 `bot_user_id` 的老行退回 `@U Workers` | `slash_command.go`，2 条文案 + 补 2 个测试 |
| 企业微信默认 bot 名 | `Multica Bot` → `U Workers Bot`：安装/群提及夹具、bot 名回填测试、媒体入站夹具，以及三处说明注释；对「含空格的 bot 名」这一被测语义没有影响 | 9 个文件 42 处 |
| 文档站 Slack manifest 与 handle | 五语言 `slack-bot-integration.mdx`：manifest `name`、`bot_user.display_name`、三个 slash command 描述、正文「两处 `Multica`」、4 处 `@Multica` handle；五语言 `channels.mdx`：3 处 `@Multica` handle | 10 个文件 65 处 |

合计：29 个交付文件（28 改 + 1 个新增回归测试；本计划文档另计，PR diff 共 30 个文件），旧品牌文案 136 处、
测试断言 18 处；渲染后的 brief 中 `Multica` 出现 0 次。

## 为什么命令示例仍写 `multica`（本批的关键取舍）

任务书要求「写清新旧命令的用法，以批次 C 定下的命令名为准」。本批把产品名、CLI 名都写进了 brief
头部，但命令示例保持旧拼写，理由是可验证的耦合：

1. 内置技能 `multica-onboarding` / `multica-platform` 的 frontmatter 是 `allowed-tools: Bash(multica *)`。
   brief 若让 agent 改用 `u-workers`，在按 allowed-tools 授权的运行时上会丢技能权限；批次 B 已明确
   技能 ID 与 allowed-tools 不动。
2. 技能 references 里还有 178 处 `multica …` 命令示例，文档站与 CLI_INSTALL.md 的主用名也还没统一
   （`CLI_INSTALL.md` 主用 `u-workers`，文档站正文仍全写 `multica`）。只改 brief 会造成同一产品内
   三种口径。
3. DSH web bridge 的 per-turn 指令让 agent 把命令里的 `multica` 替换成本次运行的 `node <entry>` 包装；
   brief 的示例与它对不上会让 DSH 运行拿不到任务凭据。

所以本批在 brief 头部把两个名字、双前缀、双目录的规则一次讲清（agent 不用猜），命令示例与
allowed-tools 一起留给收口批次，届时统一替换，不会出现「文本说新名、权限只认旧名」的中间态。

## 验证结果（本地，GitHub Actions 不可用，按 Tane 授权以本地为准）

| 检查 | 结果 |
| --- | --- |
| `cd server && go build ./...` | 通过 |
| `go vet ./internal/daemon/... ./internal/dshweb/ ./internal/integrations/...` | 通过 |
| `go test ./internal/daemon/execenv/` 相关用例（brief 矩阵、注入、On Behalf Of、新增过渡契约） | 全部 PASS |
| `go test ./internal/daemon/` 相关用例（prompt / chat / wakeup / comment / autopilot） | 全部 PASS |
| `go test ./internal/daemon/execenv/ ./internal/daemon/ ./internal/integrations/slack/ ./internal/integrations/wecom/ ./internal/service/ ./internal/dshweb/` 全量对照 | 与 origin/main 干净 worktree 的失败集合**完全一致**（51 个既有 Windows 环境失败：缺 agent CLI、home/temp 路径语义、CRLF、临时文件删除语义；没有一个与本次改动相关） |
| `pnpm test:dsh-web` | 40/40 通过 |
| `pnpm --filter @multica/docs test` | 3 文件 / 16 用例通过 |
| `pnpm --filter @multica/docs typecheck` | 通过 |
| `pnpm --filter @multica/docs build` | 通过（五语言 217 个页面） |
| `gofmt -l` / `git diff --check` | 通过 |

handler 包依赖隔离 PostgreSQL，本机没有可用的 PG 实例，未跑；本次没有数据库相关改动，handler
测试里也没有断言被改的 brief/prompt 文案（已用 `git grep` 核对）。

## 智能体真实运行回归

按任务书「至少一次真实运行确认智能体收到的新 brief 能正常使用新旧命令」：

1. 用 `buildMetaSkillContent` 渲染本批的 brief（18,935 字节，kind=issue，含新增头部三段），
   落成 scratch 项目的 `AGENTS.md`。
2. 用本 checkout 的 `server/cmd/multica` 构建同一份二进制，分别以 `multica.exe` 与
   `u-workers.exe` 两个文件名放进 PATH 前缀，验证 brief 里「同一二进制、两个名字」的说法。
3. 用真实 agent runtime：codex-cli 0.162.1，ChatGPT 账号登录，模型 `gpt-6.1-sol`，
   `codex exec --skip-git-repo-check`，prompt 要求它只做两件事：按 brief 的 CLI 段落用新旧两个
   名字各跑一次 version；再用三行回答命令名、前缀优先级、目录规则。

实际执行与回复（原文摘录）：

```text
exec  pwsh -Command 'multica --version'
 succeeded: multica dev (commit: unknown, built: unknown)
exec  pwsh -Command 'u-workers --version'
 succeeded: u-workers dev (commit: unknown, built: unknown)

CLI: new `u-workers`; legacy `multica`; identical commands.
Env vars: new `U_WORKERS_*`; legacy `MULTICA_*`; new wins when both are set.
Config: new installs use `~/.u-workers`; existing `~/.multica` installs keep it unchanged.
```

结论：agent 从 brief 里正确取到新旧命令名，并**实际成功执行了两个名字**，没有再撞上旧品牌或
「某个名字不可用」的困惑；三段规则的回答与 brief 一致。

## 评审后修订（CLAWSTA-100）

后端架构师评审 PR #13 给了 2 处必改，已在本分支修完（本地测试通过，未再跑真实运行，按评审要求）：

1. **brief 的「不能按进程名杀 daemon」规则补上新名字**：原文只写 `multica` / `multica.exe`，新装机器上
   daemon 的进程名是 `u-workers`，agent 执行 `taskkill /IM u-workers.exe` 不会觉得违规。现在这一句同时
   写出 `u-workers`、`multica`、`u-workers.exe`、`multica.exe`，并说明新装与老装的进程名不同；
   `execenv_test.go` 与 `runtime_config_kind_test.go` 的钉住断言同步更新，`runtime_config_brand_test.go`
   新增 `TestBriefBansKillingEitherDaemonExecutableName` 检查该句同时含两个名字。
2. **Slack 线程引导改成按安装渲染 bot 名**：写死 `@U Workers` 对老安装是回退（用户照着输入 @ 不到 bot）。
   两条文案改成函数，从 `inst.Platform` 的安装配置里取 `bot_user_id`（`DecodePublicConfig`，BYO 安装时由
   `auth.test` 写入），输出 `<@BOT_USER_ID>`；为空时退回 `@U Workers`。补了两个测试：有 id 时输出真实
   mention 且不在反引号里、没有 id 时走兜底。

评审同意的第 3 项（命令示例暂留 `multica`，全量换名 + allowed-tools + 技能 references + 文档正文作为
旧名收口前的独立批次）不改。

## 兼容性说明

- Slack：默认 app 名（manifest）只影响新建 app；已有安装的 app 显示名与 @ handle 不变，
  Slack mention 走结构化 `app_mention`，不依赖字面名字，`/issue`、`/new`、`/clear` 照常工作。
  线程引导也不写死名字：它用安装记录里的 `bot_user_id` 渲染成 `<@U…>`，Slack 会显示该 app 当前的
  名字，所以老安装看到 `@Multica`、新安装看到 `@U Workers`；`bot_user_id` 缺失的极老行退回
  `@U Workers`。mention 刻意不放进反引号，否则 Slack 会原样打印 token。
- 企业微信：安装时由管理员在企微控制台给 bot 命名并回填 `bot_display_name`，服务端没有默认名；
  已安装行里存的 name 不动。本批只把仓库内作为「默认名」示例的夹具与注释改成 `U Workers Bot`；
  UI 输入框示例名批次 A 已改为 `U Workers Bot`。
- brief / prompt：只改文案，不改任何命令、参数、状态流转；快照断言与新增契约测试一起锁住新文案。
- daemon 注入的任务变量仍是 `MULTICA_*`（`taskMulticaEnvironment`），brief 已按实际行为写明；
  CLAWSTA-99 的收口前提（移除旧名读取前先把注入变量改新名）不受本批影响。

## 没做 / 留给后续

1. **命令示例全量换名**：brief（约 100 处）、内置技能 references（178 处）、allowed-tools
   `Bash(multica *)`、文档站正文（每语言 50+ 处）要一起换成 `u-workers`，否则会出现文本与权限
   不一致的中间态。建议作为一个收口批次的独立任务，排在旧名移除之前。
2. **文档站 CLI 命令示例仍未用新名**：`cli.mdx`、`daemon-runtimes.mdx`、`troubleshooting.mdx`、
   `self-host-quickstart.mdx` 五语言正文全是 `multica …`，与 `CLI_INSTALL.md` 主用 `u-workers`
   不一致，属文档批次。
3. **DingTalk 夹具**：`dingtalk/inbound_bot_name_test.go` 等仍用 `Multica Bot - Local` 作测试
   bot 名。任务书只点名 Slack 与 WeCom，未动。
4. **营销文案残留**：`apps/web/content/use-cases/*.mdx` 还有 A Multica issue comment、
   Multica Cloud 等旧品牌（批次 A/B 范围外的遗漏），需要产品确认后随文档批次处理。
5. **`%APPDATA%\Multica` 与内部分支记录文案**：`desktop-app` 文档与 worktree branch record
   （`multica: task branch record`）保留，属部署/内部标识，按批次 C 的「不做」清单不动。
6. **旧名移除**：按 CLAWSTA-99 的「本版本发布 ≥2 个 tag」逐项收口。
