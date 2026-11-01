# CLAWSTA-102：外部 IM 只保留飞书，其余四个渠道用开关关闭

日期：2026-11-01
负责：后端架构师（方案、验收）；执行：DSH
来源：CLAWSTA-85 Tane 的决定——外部 IM 只保留飞书；Slack、Telegram、钉钉、企业微信各用一个开关关掉，
代码不删，打开开关即恢复。

## 方案（已定，执行时不再改）

### 开关形式：沿用现有运行时功能开关 `FF_*`，不用 Go build tag

新增四个开关，默认关闭；飞书不加开关，行为不变。

| 渠道 | flag key | 环境变量 |
| --- | --- | --- |
| Slack | `channel_slack` | `FF_CHANNEL_SLACK=true` |
| Telegram | `channel_telegram` | `FF_CHANNEL_TELEGRAM=true` |
| 钉钉 | `channel_dingtalk` | `FF_CHANNEL_DINGTALK=true` |
| 企业微信 | `channel_wecom` | `FF_CHANNEL_WECOM=true` |

选它而不是 build tag 的理由：

1. 一个开关同时管服务端和前端。`server/internal/featureflags` 的 `frontendPublicFlags` 已经把开关下发给
   web / desktop，build tag 前端看不到，还得再造一套前端开关。
2. 恢复只需改环境变量并重启，不用重新构建二进制和镜像。
3. 仓库已有同样的先例：Composio（`router.go` 里 `ComposioMCPAppsEnabled` 先于密钥判断）。

开关在服务启动时读取（和渠道密钥一样），改完需重启服务。前端拿到的是同一份环境变量的求值结果，重启后两边一致。

### 关闭的语义：等同于「没配该渠道密钥」

服务端每个渠道早已有「`MULTICA_<渠道>_SECRET_KEY` 未设置 → 不接线」的分支，且这条路径是现有的、
受支持的部署形态（handler 返回 `configured:false`，写操作返回 feature disabled，channel supervisor 跳过
没有 Factory 的 channel_type）。开关关闭时直接走这条路径，不新增任何降级逻辑。

## 改动范围

### 1. 服务端

- `server/internal/featureflags/keys.go`：加四个常量和 `ChannelSlackEnabled` 等四个函数（默认 `false`），
  四个 key 加入 `frontendPublicFlags`。
- `server/cmd/server/router.go`：Slack / 钉钉 / 企业微信 / Telegram 四个接线块，在 `secretbox.LoadKey(...)`
  之前先判断开关；关闭时只打一行 `slog.Info("<渠道> integration disabled (feature flag off)")`，其他不动。
  写法照 Composio 那段。飞书（lark）接线块不动。
- `server/cmd/server/main.go` 里企业微信 relay 相关代码不动：密钥未设置时它今天就这样运行，属于已验证形态。
- handler 不改。四个渠道的 handler 已对 nil service 做了处理（List 返回 `configured:false`，写操作返回
  disabled），开关关闭后正是这个状态。执行时只需确认 `chat_history.go` 里 `h.SlackHistory` 为 nil 的分支对
  已有 Slack 会话仍能正常返回（不 panic、不 500）。
- CLI、daemon 不改：`execenv/channel_type.go`、`prompt.go`、`cmd_chat.go` 只是给已存在的会话打渠道标签，
  不提供接入入口；渠道关闭后不会再产生新的该渠道会话。
- `.env.example`：在四个渠道各自的密钥段落旁加一行 `FF_CHANNEL_<渠道>=`，注释写明默认关闭、设为 `true`
  并重启即恢复。

### 2. 前端（web / desktop 共用 `packages/views`，由前端负责人评审）

- `packages/core/feature-flags/keys.ts` 和 `index.ts`：加四个 flag 常量并导出。
- `packages/views/settings/components/channels-tab.tsx`：渠道列表按开关过滤；对应 `useQuery` 在开关关闭时
  `enabled: false`，不再发请求。深链 `?integration=slack` 指向关闭的渠道时回到渠道列表。
- `packages/views/agents/components/tabs/integrations-tab.tsx`、`agent-overview-pane.tsx`：四个渠道的行、
  按钮、状态和查询同样按开关隐藏 / 停发。
- 取开关用现有的 `useFeatureEnabled(KEY, false)`（见 `agent-overview-pane.tsx` 的 Composio 用法）。
- `/slack/bind` 等绑定页不改：服务端兑换接口在关闭时已返回 disabled，页面显示现有错误态即可。
- 移动端没有 IM 设置入口，不改。

### 3. 文档站

- 五种语言的 `apps/docs/content/docs/meta*.json`：从导航中去掉 `slack-bot-integration`、
  `dingtalk-bot-integration`、`telegram-bot-integration`（企业微信没有单独页面）。页面文件保留，开关打开后
  恢复导航即可。
- `channels*.mdx` 与 `environment-variables*.mdx` 和 PR #14（CLAWSTA-96，未合并）重叠。**开工时 PR #14
  若已合并**：基于最新 main，在 `channels*.mdx` 中只保留飞书为默认渠道并写一句「其他渠道默认关闭，见
  环境变量」，在 `environment-variables*.mdx` 加四个 `FF_CHANNEL_*`；**若未合并**：本批不动这两组文件，
  在交付说明里列为合并 #14 后的补丁，避免冲突。

### 4. 存量数据（不迁移、不删除、不撤销）

已接入这四个渠道的工作区，关闭开关后：

- 安装记录、绑定、会话、消息原样保留在数据库，状态不改。
- 服务启动时不再建立这些渠道的连接，不再收消息；由这些渠道发起、仍在运行的任务照常结束，只是不再回发
  到该渠道。
- 界面上不显示这些渠道；已有会话在聊天记录里仍可查看。
- 重新打开开关并重启：沿用原安装记录和原密钥直接恢复连接，不需要重新安装。前提是
  `MULTICA_<渠道>_SECRET_KEY` 未更换。

不做「自动撤销安装」：撤销是不可逆的（要求用户重新安装、重新绑定），违背「打开开关即恢复」。

## 测试

Go：

- `featureflags`：四个开关默认 `false`；`FF_CHANNEL_*=true` 时为 `true`；`EvaluateFrontendPublicFlags`
  含四个 key。
- 已有依赖渠道密钥的接线测试（`dingtalk_reactions_integration_test.go`、`wecom_*_wiring_test.go` 等）
  会因默认关闭而失效：在这些测试里同时打开对应开关，测试意图不变。
- 新增一个接线测试：设置 `MULTICA_SLACK_SECRET_KEY` 但开关关闭 → `GET .../slack/installations` 返回
  `configured:false`；打开开关 → `configured:true`。其余三个渠道用同一张表驱动测试覆盖。

前端：

- `channels-tab`：默认只渲染飞书；打开 `channel_slack` 后出现 Slack；深链到关闭渠道回到列表。
- `integrations-tab`：默认只有飞书一行；打开某渠道后该行出现。
- 用 `configStore.getState().setFeatureFlags(...)` 设置开关（见 `create-agent-dialog.test.tsx`）。

## 验收（本地为准，GitHub Actions 不可用，不等 CI）

1. 默认配置（只设飞书密钥 + 任意其他渠道密钥，不设 `FF_CHANNEL_*`）：启动日志四个渠道为
   `disabled (feature flag off)`；设置页、智能体集成页只看到飞书；四个渠道的 List 接口返回
   `configured:false`；飞书的列表、安装、绑定接口和原来一致，lark 包测试全过。
2. 只设 `FF_CHANNEL_SLACK=true` 并重启：Slack 在设置页和接口恢复，其他三个仍关闭。其余三个各做一次。
3. 检查：`cd server && go build ./... && go vet ./cmd/server/ ./internal/featureflags/ ./internal/handler/`；
   `go test ./internal/featureflags/ ./cmd/server/ ./internal/integrations/lark/...`（含新增和改动的测试）；
   `pnpm typecheck`、`pnpm lint`，以及 `packages/core`、`packages/views` 相关测试。
4. 第 1、2 条的真实运行验证用 `make up` 起本 worktree 的环境，贴启动日志片段和接口返回作为证据
   （密钥值不得出现在日志、评论或文件里）。

## 交付

- 从本分支（基于 main `d53f16b41`）继续，提交按原子化 conventional commits，开 Draft PR 到 `main`。
- 本文件末尾补「交付记录」：base..head、改动文件数、实际跑过的检查与结果、PR #14 是否已合并及文档处理方式。
- 不合并、不推送 main。

## 交付记录（2026-11-01，DSH，批次 1 / 唯一一批）

**PR**：[#15](https://github.com/AndyTane/UWorkers-nice/pull/15)（Draft，`clawsta-102-im-feishu-only` → `main`）
**base..head**：任务基于 `d53f16b41`；PR base 现为 `b578c3197`（#14 在本批进行中合并，已把最新 main 合并进分支，
见文末「交付后更新」）。代码提交到 `3af1ab39d` 为止，head 见 PR 顶部。
**改动文件数**：代码 23 个文件（+779 / −220），另加本任务书（连同任务书共 24 个文件）；4 个代码提交：
`b346ee25e` feat(channels) 开关与路由接线、`667bd913a` test(channels) 测试、`88598ee8a` feat(web) 前端、
`3af1ab39d` docs(channels) 文档站导航。

### 做了什么

- **服务端**：`server/internal/featureflags/keys.go` 新增 `channel_slack` / `channel_telegram` / `channel_dingtalk` /
  `channel_wecom` 四个开关（默认关）并加入 `frontendPublicFlags`；`server/cmd/server/router.go` 在四个渠道
  `secretbox.LoadKey(...)` 之前先判断开关，关闭时只打一行 `<渠道> integration disabled (feature flag off)`，
  其余接线、handler 未改；`.env.example` 四个密钥段落各加一条 `FF_CHANNEL_*` 与说明。飞书没有开关，代码未动。
- **前端**：`packages/core/feature-flags/` 加四个常量并导出；设置页渠道列表、智能体 Integrations 页、
  智能体概览的 Integrations 入口都按开关过滤，关闭的渠道不再发对应请求（`enabled:false`），指向关闭渠道的
  深链回到渠道列表；`packages/views` 对应的三个测试文件覆盖这些行为。
- **文档站**：五种语言 `apps/docs/content/docs/meta*.json` 导航去掉 Slack / 钉钉 / Telegram 三个页面，
  页面文件保留。
- **存量数据**：没有迁移、删除、改状态或撤销任何安装/绑定/会话；关闭只影响服务启动时的接线。
- **CLI / daemon / 移动端**：按方案未改（没有接入入口）。

### 验证

运行方式：本机是 Windows，`scripts/dev-env.sh` 用 `lsof` 与 `ps -o` 证明进程归属，Git Bash 没有这两样，
所以在同机 WSL（Ubuntu 24.04，与应用共享同一个 PostgreSQL 容器）里执行同一份 `Makefile` / 脚本：
`make up C=api ARGS=--ephemeral`。没有推送到任何远端环境。

1. **默认配置**（飞书密钥 + Slack/Telegram/钉钉/企业微信密钥全部设置，不设任何 `FF_CHANNEL_*`）的启动日志：
```text
15:29:24.675 INF lark integration enabled
15:29:24.675 INF slack integration disabled (feature flag off)
15:29:24.675 INF dingtalk integration disabled (feature flag off)
15:29:24.675 INF wecom integration disabled (feature flag off)
15:29:24.675 INF telegram integration disabled (feature flag off)
```
   同一环境里用 dev 登录建了一个工作区，逐个读安装列表接口（`GET /api/workspaces/{id}/<channel>/installations`，只取 `configured` 字段）：
```text
lark      configured=true
slack     configured=false
telegram  configured=false
dingtalk  configured=false
wecom     configured=false
```
2. **只打开一个开关并重启**（每次 `make down C=api && make up C=api`，改的只是 `.env.worktree` 里的 `FF_CHANNEL_*`）：
```text
STATE slack only
  15:29:47.451 INF lark integration enabled
  15:29:47.451 INF slack integration enabled (BYO per-installation socket mode)
  15:29:47.451 INF dingtalk/wecom/telegram integration disabled (feature flag off)
  lark configured=true | slack configured=true | telegram/dingtalk/wecom configured=false

STATE telegram only
  15:30:09.884 INF telegram integration enabled (per-installation long polling)
  其余三个 disabled (feature flag off)；telegram configured=true，其余 false

STATE dingtalk only
  15:30:35.558 INF dingtalk integration enabled (BYO per-installation stream mode)
  其余三个 disabled (feature flag off)；dingtalk configured=true，其余 false

STATE wecom only
  15:31:00.390 INF wecom integration enabled (smart bot, long connection)
  其余三个 disabled (feature flag off)；wecom configured=true，其余 false

STATE all four on
  四个 integration enabled；四个 configured=true
```
   每一种状态下飞书都是 `lark integration enabled` / `lark configured=true`，即飞书行为没有随开关变化。
3. **静态检查与测试**（实际跑过的）：
   - `cd server && go build ./...`：通过（BUILD_OK）
   - `go vet ./cmd/server/ ./internal/featureflags/ ./internal/handler/`：通过（VET_OK）
   - `go test ./internal/featureflags/`：ok 0.008s
   - `go test ./cmd/server/`：ok 7.663s；其中新增的 `TestChannelFeatureFlagsGateConfiguredChannels`（默认关、
     Slack/Telegram/钉钉/企业微信各自单开、四个全开共 6 个子测试）与既有的 DingTalk/WeCom 真实启动路径接线测试
     全部 PASS（`go test -run '...' -v` 输出：6 个 channel 子测试 + 4 个既有接线测试 PASS）
   - `go test ./internal/integrations/lark/`：ok 7.829s
   - `pnpm typecheck`：10/10 成功
   - `pnpm lint`：7/7 成功，0 error
   - `pnpm --filter @multica/views exec vitest run settings/components/channels-tab.test.tsx agents/components/tabs/integrations-tab.test.tsx agents/components/agent-overview-pane.test.tsx`：
     3 个文件 62 个用例全过
   - 未跑：Playwright E2E（本批没有改动端到端流程，任务书也未要求）

### PR #14（CLAWSTA-96）与文档处理

- 开工时 `origin/main` 仍是 `d53f16b41`，PR #14 **未合并**。因此按任务书：
  **没有动** `channels*.mdx` 和 `environment-variables*.mdx`（五种语言都不动），只改了 `meta*.json` 导航，
  避免和 #14 冲突。
- 合并 #14 后的补丁（留给提交 #14 之后的小批）：`channels*.mdx` 只保留飞书为默认渠道并加一句
  「其他渠道默认关闭，见环境变量」；`environment-variables*.mdx` 增加四个 `FF_CHANNEL_*` 的说明。
  另外 `channels*.mdx` 正文里仍有指向已从导航移除的 Slack/钉钉/Telegram 页面的链接，页面文件还在、
  链接仍可用，等 #14 合并后的补丁一并收口。

### 没做 / 不确定

1. **Web / Desktop / 移动端真机点开设置页**没有做：前端部分由单测覆盖，按任务书交前端负责人评审。
2. 文档站只改了五种语言的导航，正文与语言内链接按上面「PR #14 与文档处理」处理。
3. Slack/Telegram/钉钉/企业微信的存量安装、绑定、会话按方案原样保留，没有撤销、没有改状态；关闭期间
   由这些渠道发起、仍在运行的任务照常结束，只是不再回发到该渠道。
4. 验证用的本地 dev 环境（WSL 里的 `uworkers_nice-469`）只有本机可见，数据是临时库；关闭只影响该环境。

### 交付后更新：PR #14 已合并

本批开工时 `origin/main` 是 `d53f16b41`、PR #14 未合并，所以按任务书「未合并」分支处理：不动
`channels*.mdx` 与 `environment-variables*.mdx`，只改导航。交付前复查时发现 #14 已于 15:14 合并到 main
（`b578c3197`），于是把最新 main 合并进本分支（merge commit `0342b935c`），PR base 更新为 `b578c3197`
且保持可合并；合并后在合并结果上重跑了 `go build ./...`、`go vet`、`go test ./internal/featureflags/
./cmd/server/ ./internal/integrations/lark/...` 与 `pnpm typecheck`，全部通过（结果与前面一致）。

**文档补丁的去向**：`channels*.mdx`（只保留飞书为默认渠道，并写一句「其他渠道默认关闭，见环境变量」）与
`environment-variables*.mdx`（加四个 `FF_CHANNEL_*`）仍按任务书作为 #14 合并后的独立补充，没有塞进本批：
它涉及五种语言共 10 个页面，单列一批更容易审翻译和文案。评审若希望并入本 PR，我可以按现有分支直接补一个提交。

## 专业验收与补丁（2026-11-01，后端架构师）

以下为当前交付状态；上面的 DSH 记录保留为候选 `25450f41e` 及合入 main 后 `048117667` 的验证证据。

- 后端评审通过：四个开关先于密钥读取、服务创建和 Factory 注册生效，默认值均为 false，
  前端下发同一组开关；飞书实现无差异。关闭时现有 handler 返回未配置/禁用；Supervisor 跳过
  未注册渠道，不获取其连接租约；SlackHistory 为 nil 时历史接口读取库内聊天记录，线程接口返回
  现有未配置提示。没有数据迁移、撤销或删除。
- 验收发现并关闭一项部署遗漏：Compose 原来未传入四个 `FF_CHANNEL_*`，导致在 `.env` 打开
  开关也无法恢复容器里的渠道。提交 `2b764bc29` 补齐透传，默认 false，并更新旧注释。
- PR #14 已于 2026-11-01 15:14（UTC+8）合并。本分支已合入最新 main `b578c3197`，没有冲突，
  服务端无新增差异；五种语言的 `channels*.mdx`、`environment-variables*.mdx` 后续补丁已完成，
  默认列表只列飞书、说明四个开关和原密钥恢复方式、去掉其他渠道的推荐入口，保留原独立页面。
  **文档补丁不再是待办。**
- 本轮独立验证：`go test ./internal/featureflags/ ./internal/integrations/lark/... -count=1` 通过
  （2.134s / 9.624s）。本轮未配置测试 DB/Redis，依赖它们的用例按既有条件跳过；DB 接线矩阵和
  `make up` 实跑验收采用上面 DSH 同机隔离环境的记录，不把本轮普通 Go 测试充作 DB 验收。
- Compose 实际解析：对 `docker-compose.selfhost.yml` 及叠加 `.build.yml` 两种方式，分别检查
  默认关闭、四个单开、全开，读取 `services.backend.environment` 的四个开关，12/12 通过。
  使用临时空 env 文件和测试占位值，仅输出开关结果；没有启动容器或打印凭据。
- 五种语言文档检查通过：每个开关说明恰好一项，默认表只有飞书，导航与渠道页无其他渠道推荐链接，
  原独立页面仍在，渠道页本地链接可解析；`git diff --check` 通过。未重复前端构建或浏览器检查。

本阶段后端实现、运行证据审查与部署/文档补丁完成，无后端未关闭阻塞。前端负责人接手本批专业评审：
按本任务书「前端」「验收」检查 Web/Desktop 共用设置页、集成页、概览入口、关闭时停发请求和深链，
以及移动端没有遗漏入口。默认只显示飞书，单开各渠道能恢复对应入口；使用 PR #15 的最新 head，
在 issue 回复评审结论与必要证据。前端通过后交协调负责人按现有授权合并，不等 GitHub Actions；
最终完成仍由用户确认。
