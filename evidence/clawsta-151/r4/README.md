# CLAWSTA-151 R4b 真实渲染截图（集成卡 CLAWSTA-159）

本目录是任务书「R4b 前端」验收段要求的亮/暗两套真实渲染截图，由 `e2e-visual/clawsta-151-r4.spec.ts` 在隔离 worktree 环境里跑 Chromium 生成。图里所有卡片都是本 spec 现场造的演示数据，没有连真实部署或真实数据库。

## 截图清单（每种 1280×900，light / dark 各一张，共 20 张）

| 截图 | 验收项 |
| --- | --- |
| `awaiting-three-groups-{light,dark}.png` | 三组清单：回复 2 / 推进 1 / 审批 1，组标题带数量，底部「另有 2 张卡在等人，但没标明等谁」 |
| `popover-reply-{light,dark}.png` | 回复弹出层（一句话输入、接收人提示） |
| `popover-done-{light,dark}.png` | 推进「已完成」弹出层（可留空、默认句） |
| `unmarked-expanded-{light,dark}.png` | 「未标明」就地展开，其中一张带机器码白话原因（`invalid_review_mark` → 「这张卡的审核标记不完整，没法交给人处理。」） |
| `sync-toggle-owner-{light,dark}.png` | 同步开关文字一：`同步给主卡负责人 · 协调负责人`（主卡负责人可达） |
| `sync-toggle-recipient-{light,dark}.png` | 同步开关文字二：`同步给 老陈`（评论人叫不醒主卡负责人，改投主卡派活人） |
| `sync-toggle-locked-{light,dark}.png` | 同步开关锁定态：普通路由已覆盖主卡负责人（小组组长就是子卡负责人），勾选且不可改；勾选框置灰，文字后面写明「本来就会通知到」 |
| `transfer-prompt-{light,dark}.png` | 转派提示：「老陈 无法叫醒 协调负责人」+ 具体原因白话 + 建子卡/仍然转派/取消 |
| `inbox-parent-line-{light,dark}.png` | 收件箱子卡通知灰字行「属于 CLAWSTA-320 · 发布检查项」 |
| `board-blocked-labels-{light,dark}.png` | 看板「已阻塞」列的标签：`等你回复`、`等你推进`、`等 林工`、`等 老陈` |

`screenshots.json` 记录每张图的字节数与 SHA-256。

## 环境与重跑

环境用仓库自带的 worktree 隔离方案：`scripts/dev-env.sh up` 生成 `.env.worktree`（本机端口 18637/13557、数据库 `multica_uworkers_nice_557`，已跑完全部迁移），截图只用这套隔离数据库。

本机没有 GNU make 和 lsof，`dev-env.sh` 的组件启动与端口归属检查在这台 Windows 上会失败（MSYS 看不到原生 Windows 进程树），所以 `.env.worktree`、数据库和迁移仍由 `scripts/dev-env.sh` 创建，API/Web 用同一份 `.env.worktree` 的变量按 Makefile 目标里的命令手工起停，截图完成后按记录的 PID 停止。数据库名、端口、迁移结果与脚本方案一致。

重跑步骤：

1. 起隔离 API 与 Web（沿用 `.env.worktree`），确保 `NEXT_PUBLIC_API_URL`、`FRONTEND_PORT` 生效。
2. 在仓库根执行：

```sh
CLAWSTA151_R4=1 PLAYWRIGHT_BASE_URL=http://localhost:<前端端口> \
  pnpm exec playwright test -c playwright.visual.config.ts clawsta-151-r4.spec.ts
```

spec 会拒绝共享数据库：要求 `DATABASE_URL` 指向隔离库，且显式设置 `CLAWSTA151_R4=1`。演示工作区 slug 为 `uworkers-r4`，每次运行前清空重建。

## 演示数据

独立工作区 `uworkers-r4`（前缀 CLAWSTA）里有：

- 等我处理：301（回复，等 2 天）、306（回复，等 20 小时）、302（推进，等 9 小时）、303（审批，等 1 小时）。
- 未标明：304（没有标记）、305（审核标记不完整）。
- 看板阻塞列：301/302/306（等我）、307（等 林工）、308（等 老陈）、304/305（无可路由标记）。
- 子卡 321/323/325 各挂一张主卡，分别演示同步开关的可达、降级和锁定态。
- 卡 330 由老陈派活，当前成员把负责人从成员改成「协调负责人」时走转派预检。
- 收件箱里有一张子卡通知，用来渲染父卡灰字行。

演示账号 `clawsta151-r4@example.test`（开发验证码 888888，只存在于隔离库）。

## 出图时一并修掉的集成问题

- `review-badge.tsx` 原来自己读 `issue.metadata.review_by`，改为统一用 core 的 `awaitingKind` / `isAwaitingMe`，并对缺失 metadata 的旧快照容错，避免任一卡片行因半截数据整页崩掉。
- 同步开关的锁定态在真实后端里走不到：空评论的触发预览没有算普通路由（issue 负责人的兜底路由），`already_routed` 永远是 false。已在 `PreviewCommentTriggers` 的空内容分支补上该路由，`TestPreviewSyncLocksWhenRoutingAlreadyReachesSquadLeader` 覆盖。
- 「未标明」查询里，组合非法的审核标记（blocked + user 且缺 `review_kind`）因为三值逻辑得到 NULL 而被漏掉，导致这类卡既不在清单也不在未标明里。已在 `awaitingUnmarkedPredicate` 里 `COALESCE` 成 FALSE，`TestAwaitingFilters` 增加用例覆盖。

## 附注

- 截图里的「创建者 / 负责人」名字、编号、时间为演示数据；真实卡片未被读取或改写。
- 截图使用 zh-Hans；五种语言键集合由 `packages/views/locales/*/issues.json` 的测试与人工核对保持一致。
- 卡片动作句、标题均为为演示编写的短句，不包含任何凭据或真实业务内容。
