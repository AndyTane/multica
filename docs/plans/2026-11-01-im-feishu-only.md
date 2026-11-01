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
