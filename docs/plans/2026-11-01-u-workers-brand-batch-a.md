# CLAWSTA-85 批次 A：品牌文案与内置智能体改名

日期：2026-11-01
范围：CLAWSTA-85「批次 A（先做，低风险）」——只做界面文案、引导/问候、桌面与移动端显示名、
README 与文档站，以及内置智能体 Mika → Siman 的幂等迁移。批次 B（人设/技能提示词、kickoff 文本）
和批次 C（CLI 命令、下载包、环境变量、配置目录的兼容改名）本轮不做。

## 改了什么

| 类别 | 内容 | 规模 |
| --- | --- | --- |
| 五语言界面文案 | `packages/views/locales/*` 的 onboarding / runtimes 文案，Mika → Siman（韩语助词按发音改写） | 5 语言 × 2 文件，共 125 处 |
| 引导与开场白 | 引导对话标题、install-runtime 引导任务标题与正文（英/中/韩/日）；服务端欢迎语里的 Multica → U Workers | 3 个模板文件 + 服务端 4 语言欢迎语 |
| 内置智能体显示名 | 新建工作区的默认显示名改为 Siman；四语言 agent 简介文案同步改名 | `MikaDefaultName` + 4 条简介 |
| 用户可见提示 | 删除运行时冲突时「内置智能体不可归档」的提示、启动引导失败的 API 错误、系统消息作者名 | 4 处 |
| 桌面显示名 | 已是 U Workers（PR #2 完成），本轮无改动 | — |
| 移动端显示名 | `apps/mobile/app.config.ts` 的 App 名称 → U Workers / U Workers (Staging) / U Workers (Dev)；bundle id、slug、scheme 不动 | 1 文件 + 测试 |
| README | `README.md` / `README.zh.md` 重写品牌区、去掉上游徽章与外部站点链接、改为自托管入口、内部标识兼容说明 | 2 文件 |
| 文档站外壳 | 站点标题、metadata、首页 hero 文案、架构图、社区入口（只保留本仓库 GitHub）、docs 导航标题、Logo 资源（按产品 UI 现有 mark 导出 SVG） | 6 文件 + 2 资源 |
| 文档站正文 | 五种语言 210 个页面的正文品牌名 Multica → U Workers（保留 `multica` CLI 命令、`MULTICA_*` 环境变量、包名、域名、代码块与配置片段） | 210 文件，约 1.6 千处 |
| 仓库级文档 | SELF_HOSTING* / CLI_* / CONTRIBUTING / VISION*（含中文版）正文品牌名；重写 VISION 的「名字由来」一节；删除已不存在的「切换到 Multica Cloud」小节 | 10 文件 |
| 数据迁移 | `583_builtin_agent_display_name`：把 system_key='mika' 且仍叫 Mika 的内置智能体显示名改为 Siman，并把产品写入的四语言简介同步改名；幂等，owner 自定义的名字/简介不动 | 2 个 SQL + 1 个集成测试 |

## 迁移与验证

- `server/migrations/583_builtin_agent_display_name.up.sql`：一条 UPDATE，`WHERE system_key = 'mika' AND (name = 'Mika' OR description IN (产品原文四语言))`；
  只改产品写下的值，第二次执行匹配 0 行，天然幂等；`updated_at` 只在行真正变化时更新。
- 简介是逐语言精确双向映射，不是把名字替换进去：韩语助词随名字收音变化（Mika가 → Siman이），
  写死的 `REPLACE(..., 'Mika', 'Siman')` 会得到 `Siman가`。up 枚举四种语言旧串→新串，down 枚举新串→旧串。
- down 迁移把仍是 Siman 的名字和产品原文简介还原为 Mika；owner 改名/改简介不回滚。
- `server/internal/migrations/builtin_agent_display_name_migration_test.go`：隔离 schema 内建最小 agent 表，覆盖
  「四语言产品默认行（含韩语内置行）」「owner 已改名但简介是产品原文」「普通智能体带 Mika 字样」
  「owner 改过简介」「其它 system_key」「up 之后新建的韩语行」八种行；up 连跑两遍验证第二次不改任何行和
  `updated_at`，再跑 down 验证四语言还原和 owner 自定义字段保护。
- 首次交付时本机没有 PostgreSQL，该集成测试是 skip；真实数据库结果见「CLAWSTA-88 修复记录」。

## 验证结果

| 检查 | 结果 |
| --- | --- |
| `pnpm install --frozen-lockfile` | 通过 |
| `pnpm typecheck`（不含 mobile） | 10/10 任务通过 |
| `pnpm --filter @multica/core test` | 173 文件 / 2224 用例通过 |
| `pnpm --filter @multica/web test` | 25 文件 / 245 用例通过 |
| `pnpm --filter @multica/docs test` | 3 文件 / 16 用例通过 |
| `pnpm --filter @multica/ui-lab test` | 8 文件 / 46 用例通过 |
| `@multica/views` 本次改动相关测试（onboarding-local-flow、welcome-after-onboarding、step-runtime-connect、chat-message-list、locales/parity） | 5 文件 / 257 用例通过 |
| `pnpm --filter @multica/mobile typecheck` | 通过 |
| `pnpm --filter @multica/mobile test` | 36 文件 / 243 用例 + ios-run 断言通过 |
| `pnpm --filter @multica/docs build` | 通过（五语言 217 个静态页面全部渲染，验证 MDX 未被文案替换破坏） |
| `pnpm lint` | 0 error（仅存量 warning） |
| `cd server && go build ./...` | 通过 |
| `go test ./internal/handler ./internal/migrations` | 退出 0 但本机无数据库：handler 包整体跳过、迁移集成测试 SKIP（真实数据库结果见「CLAWSTA-88 修复记录」） |
| `git diff --check` | 通过 |

已知与本轮无关的失败（在 `origin/main` 的干净 worktree 上复现同样失败，属于本机 Windows 检出环境问题或存量问题）：

- `server/internal/service`：内置技能 SKILL.md 的前置 frontmatter 检查在 CRLF 工作区下失败（main 上同样失败）。
- `server/cmd/migrate` TestEveryConcurrentUpBuildHasCleanup：574/575/579/580 四个存量的并发索引迁移没有 up cleanup（main 上同样失败）。
- `@multica/views` / `@multica/web` 全量并发跑测试时偶发与本改动无关的对话框/定时器类用例超时；
  单独跑对应包或相关文件全部通过。

## 没做 / 需要决策的部分（批次 A 明确排除或超出本轮范围）

1. **批次 B**：`builtin_agents/mika/INSTRUCTIONS.md`、`builtin_skills/*` 提示词、onboarding kickoff 文本里仍有 Mika/Multica；
   技能 ID 保留。这批需要提示词快照测试与行为回归。
2. **批次 C / 兼容标识**：`multica` CLI 命令、`MULTICA_*` 环境变量、`@multica/*` 包名、Go module、
   `~/.multica` 配置目录、`multica://` 协议、HTTP 头（如 `X-Multica-Next-Before`）、token 前缀、mobile bundle id 全部未动；
   README 与文档站里的命令/变量示例因此仍显示旧标识，等批次 C 双读过渡后再统一。
3. **上游云服务文案**：文档站与仓库文档里仍有 79 处 `Multica Cloud`（认证令牌表、channels 环境变量说明、
   CLI 登录、Desktop 默认云地址等）和 `multica.ai` 链接。这个仓库是纯自托管，没有对应产品；改名成
   「U Workers Cloud」会凭空造出一个不存在的服务，所以本轮保留原文，建议在批次 C 里连同 cloud-quickstart 页面一起删改。
4. **后端其它用户可见文案**：CLI 报错文案（`server/internal/cli/errors.go`）、daemon 运行时错误原因、
   Slack/Lark/钉钉/企业微信/Telegram 集成回复、邀请邮件正文、旧版桌面引导 shim 的 “Multica Helper” 文案仍用旧名；
   这些不在批次 A 枚举的类别里，留给批次 B/C 或单独收口。另：`server/internal/handler/agent.go` 归档内置智能体时的
   API 报错仍写 `built into Multica`，与运行时冲突提示同源，未纳入本批两项修复，等下一轮收口。
5. **截图与图片资产**：README 首图、文档站截图仍是上游界面截图（能看到旧看板名与 Discord 入口），
   需要一次真实环境重新截图；本轮只替换了 logo SVG（按 `packages/ui/components/common/product-mark.tsx` 的现有 mark 导出）。
6. **文档站部署配置**：`apps/docs/lib/site.ts` 的 canonical domain 仍是 `https://www.multica.ai`，
   属于部署/SEO 配置，等部署批次给正式域名后替换。
7. **根文档里引用上游输出/安装入口的几处**：`SELF_HOSTING_AI.md` 引用的服务启动输出、`CLI_INSTALL.md` 里指向上游仓库的
   安装说明、`CLI_AND_DAEMON.md` 的 Cloud 说明，属于批次 C 的 CLI/部署范围，未改。

## CLAWSTA-88 修复记录（评审后，PR #10 同分支）

评审 CLAWSTA-87 指出两处阻塞，本批在同一分支 `agent/dsh/1426ab2bb825` 修复，并拉起隔离 PostgreSQL 实测：

- **迁移 583 韩语简介回滚**：up 原来用 `REPLACE(description, 'Mika', 'Siman')`，韩语产出 `Siman가`，
  与新建行的 `Siman이` 不一致，down 也匹配不到。改为四语言精确双向映射（up 旧串→新串，down 新串→旧串）。
  新测试覆盖四语言内置行、up 重放时 `updated_at` 不变、down 保留 owner 改名/自定义简介；
  用旧 SQL 跑新测试失败在 `Siman가`，换新 SQL 通过。
- **运行时冲突标签**：`server/internal/handler/runtime_blocking_agents.go` 的 `blockingAgentLabel`
  仍返回 `built into Multica`，改为 `built into U Workers`；新增
  `TestBlockingAgentLabelMarksEveryProductOwnedClass`，固定 Mika 与其它 system 两类标签。

隔离 PostgreSQL（Docker `postgres:18.6-alpine`，全量迁移 up 后执行）：

| 检查 | 结果 |
| --- | --- |
| `go test ./internal/migrations`（DATABASE_URL 指向隔离库） | `ok github.com/multica-ai/multica/server/internal/migrations 2.570s`，无 SKIP |
| 相关 handler 用例（冲突标签、Mika 补救、类映射等 6 个） | 全部 PASS |
| `go test ./internal/handler`（全包） | 跑完 124.9s；两个 config 用例的 panic 在 `origin/main` 干净 worktree 同样复现（`&Handler{}` 未注入 Queries），另两个失败是 Windows 路径语义问题，均与本批无关 |
