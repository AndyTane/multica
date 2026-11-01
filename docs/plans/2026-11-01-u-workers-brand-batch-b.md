# CLAWSTA-89 批次 B：提示词、内置技能与剩余文案

日期：2026-11-01
范围：CLAWSTA-85「批次 B」——内置智能体的人设提示词、内置技能提示词、开场/kickoff 文本，
以及批次 A 留下的面向用户文案。品牌写法统一为 **U Workers**（带空格），内置智能体显示名 Mika → **Siman**。
技能 ID（`multica-onboarding` / `multica-platform`）、system key `mika`、`multica` 命令名、
`MULTICA_*` 环境变量、`.multica` 配置目录、`multica.ai` 域名全部保持不变。

## 改了什么

| 类别 | 内容 | 规模 |
| --- | --- | --- |
| 内置智能体人设提示词 | `builtin_agents/mika/INSTRUCTIONS.md`：产品名、"(Mika)" 身份行；第 21 行的 "Multica CLI" 改成命令名 `multica`，避免提示词里出现旧品牌 | 1 文件，4 处 Multica / 1 处 Mika |
| 内置技能提示词 | `builtin_skills/**`：SKILL.md 的 description 与正文、8 个 references 里的产品名与智能体名；技能目录名、`allowed-tools: Bash(multica *)`、命令示例全部不动 | 9 文件，21 处 Multica / 7 处 Mika |
| Onboarding kickoff | `buildMikaOnboardingKickoff` 的 profile 块："Wants to use Multica to" → U Workers（开场白本身批次 A 已改好，本批复核无残留） | 1 处 |
| 归档报错 | `handler/agent.go` 归档内置智能体的 API 报错 "built into Multica" → U Workers | 1 处 |
| 集成回复与绑定文案 | Slack / Lark(飞书) / 钉钉 / 企业微信 / Telegram 的回复、绑定提示、卡片标题、"已连接到其他工作区/用户"报错；Lark 安装时的 bot 默认名 `<agent> - U Workers` | 5 平台，41 处 |
| 事务邮件 | 邀请邮件标题/正文、验证码邮件标题："on Multica" → "on U Workers" | 4 处 |
| 旧版桌面引导 shim | `onboarding_shim.go`：Multica Helper → U Workers Helper、引导 issue/指令/运行时装机指引中英两套文案 | 1 文件，20 处 |
| 其它用户可见文案 | 运行时删除/自动回收说明、并行模式不支持的报错、VCS CA 信任报错 | 4 文件，8 处 |
| 「Multica Cloud」写法 | 文档站五种语言正文：Multica Cloud → U Workers Cloud；另有 3 处 "Multica's protocol arguments" → U Workers' | 68 文件，共 77 处 |
| 文档站首页标题 | `homeCopy` 的英文/法文/韩文 lead 与强调段之间补空格（"Humans and agents, " + "in one place."） | 1 文件，3 语言 |
| 测试同步 | 提示词契约锚点、邮件标题断言、handler/集成测试文案、Lark bot 名断言、技能 frontmatter 前置检查 | 12 文件，30 处 |

合计：120 个文件，旧品牌出现处 224 处 Multica + 9 处 Mika 全部替换为 U Workers / Siman。
（`git grep Multica` 在该范围内已归零；剩余的旧标识都属于下面「没做」里的命令名、配置目录、
域名、内部 key 与代码注释。）

## 验证结果（本地，GitHub Actions 不可用，按 Tane 授权以本地为准）

| 检查 | 结果 |
| --- | --- |
| `cd server && go build ./...` | 通过 |
| `cd server && go vet ./...` | 通过 |
| `go test ./internal/service` | 通过（含提示词契约/技能模板/邮件用例） |
| `go test ./internal/integrations/...` | dingtalk、lark、slack、telegram 等包通过；wecom 的 6 个 media_stream 用例在本机 Windows 报临时文件占用，在 origin/main 干净 worktree 上同样失败（见下） |
| `go test ./internal/handler`（隔离 PostgreSQL 18.6，全量迁移 up） | 95s 跑完，仅 2 个失败：`TestStableIDSuffixMatchesDaemon`（Windows 路径语义）、`TestGetConfigExposesFrontendFeatureFlags`；两者在 origin/main 干净 worktree 上同样失败 |
| handler 相关用例（onboarding/kickoff/开屏/归档/技能分发/运行时提示/企微错误矩阵） | 全部 PASS（真实数据库，非 skip） |
| `pnpm --filter @multica/docs typecheck` | 通过 |
| `pnpm --filter @multica/docs test` | 3 文件 / 16 用例通过 |
| `pnpm --filter @multica/docs build` | 通过（五语言 217 个页面渲染正常） |
| `git diff --check` | 通过 |

已知与本轮无关的失败（都在 origin/main 干净 worktree 上复现）：`internal/handler` 的 2 个用例
（Windows 路径分隔符、`&Handler{}` 未注入 Queries 的存量 panic）、`internal/integrations/wecom`
的 media_stream 临时文件用例（Windows 占位删除语义）。

本机 Windows checkout 是 CRLF，而内置技能测试检查 SKILL.md 是否以 `---\n` 开头；跑测试前把
`builtin_agents/**` 与 `builtin_skills/**` 的 Markdown 按仓库内容（LF）重新落盘后通过。
CI/Linux 上本来就是 LF，这个差异不进入 diff。

## 智能体行为回归

用产品真实渲染的提示词驱动一个真实 agent runtime（本机 codex CLI，ChatGPT 账号），不接 Multica 平台侧，
只验提示词/技能的人设与行为：

1. 从代码里渲染出 `service.MikaSystemInstructions("Siman")`、英文/中文开场白、
   `buildMikaOnboardingKickoff(...)` 的 kickoff 块，作为 agent 的 `AGENTS.md` 与首轮输入；
   把仓库里的 `multica-onboarding` / `multica-platform` 技能文件放进运行时可发现的技能目录
   （等同于 daemon 把内置技能交付给运行时的位置）。
2. 三个用例，真实模型回复原文：

| 用例 | 输入 | 结果 |
| --- | --- | --- |
| 首次对话 | kickoff + "Hi! I just got here — what can you help me with?" | 没有重复开场白、没有重新自我介绍；直接回答能做什么并邀请一个目标；回复中旧品牌 0 次 |
| 内置技能触发 | kickoff + 引导卡片 "Turn our current goals into a project board." | 按 `multica-onboarding` 的 Board 剧本走：只问一个澄清问题，声明会先给"4–8 个 issue + 负责人 + 完成标准"的预览再确认，没有直接建东西、没有重复开场 |
| 自我介绍/语言 | 中文 kickoff + "你叫什么名字？我们这个产品叫什么？" | 中文回复："我叫 **Siman**，是这里的 Chief of Staff。产品叫 **U Workers**，你当前的工作区叫 **Venus**。" |

说明：真实平台运行时还会在系统提示词里注入逐次运行的 runtime brief（`# Multica Agent Runtime`），
那部分属于平台/CLI 层，不在本批范围（见「没做」）。本回归验证的是 Siman 人设、开场与内置技能本身。

## 没做 / 留给后续决策

1. **逐次运行的 runtime brief 与 daemon prompt**：`internal/daemon/execenv/runtime_config_sections.go`、
   `internal/daemon/prompt.go` 里仍是 "Multica Agent Runtime" / "Multica platform" / "Multica workspace"。
   这是所有运行时的平台层系统提示词（不是 Siman 人设），改它要连带动全部 prompt 快照测试，且与批次 C 的
   CLI/环境变量过渡互相影响，建议和批次 C 或单独一批处理。
2. **CLI 终端文案**：`server/cmd/multica/**`、`server/internal/cli/errors.go` 仍是旧名（例如 "Multica CLI"、
   "Multica server"）。这些文件批次 C 正在改命令名与双读，留给 C 一起收口，避免两个 PR 改同一批文件。
3. **daemon 运行时错误原因**：`internal/daemon/daemon.go`、`internal/service/agent_ready.go`、`pkg/agent/*`
   的运行时原因字符串（如 "the Multica runtime profile is not installed"）同样和批次 C 的 DSH profile 命名
   耦合，未动。
4. **机器人身份/handle**：Slack 的 `@Multica` 与 App manifest、WeCom 的 `Multica Bot` 默认名属于集成配置，
   与批次 C 的配置改名一起处理；Lark 安装表单的 bot 默认名已在代码里改成 `<agent> - U Workers`（只影响新安装）。
5. **`Multica Cloud` 的产品语义**：本批按任务书只统一品牌写法（→ U Workers Cloud）。这个仓库是纯自托管，
   文档里这些段落描述的是上游托管服务；是否整段删除/改写需要产品决策，建议批次 C 或文档批次处理
   （前端负责人在批次 A 评审里也提过同样的建议）。
6. **保留项**：`multica` 命令、`MULTICA_*` 环境变量、`.multica` 配置目录、`%APPDATA%\Multica`、
   `multica.ai` 域名、`@multica/*` 包名、Go module、`multica-onboarding`/`multica-platform` 技能 ID、
   system key `mika`、代码注释与内部标识符（`MikaSystemKey` 等）按任务书全部不动。

## 兼容性说明

- 内置技能目录名与技能 ID 不变，daemon 侧技能解析、bundle ref 与已绑定技能的 agent 不需要迁移。
- 旧版桌面 shim 的 helper agent 显示名从 "Multica Helper" 变为 "U Workers Helper"、引导 issue 标题同步改名；
  该 shim 只服务 pre-v3 桌面的兼容调用，新桌面走 Mika/Siman 的 v3 引导，不受影响。
- 已有工作区里智能体显示名是 Siman（批次 A 迁移 583），本批只改提示词内容，不动数据库。
