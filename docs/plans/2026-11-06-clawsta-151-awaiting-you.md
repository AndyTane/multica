# CLAWSTA-151「等我处理」清单

负责人：前端负责人（方案与前端验收）；后端架构师（B 节契约确认与后端验收）。

## 本批目标（R1）

用户打开 UWorkers 就能看到「等我处理」的清单：只列需要他本人动手的待审核任务，每张一句话说清要做什么，等得最久的排最前，可以直接通过或打回。看板卡片上分清「等你」和「等验收」。

## A. 前端方案（已定）

### A1 入口

- 侧栏个人区 `收件箱` 下方新增一项 **等我处理**，右侧数量沿用收件箱的 `CappedNumberFlow` 样式；数量为 0 时不显示数字。
- 路由 `/<workspace>/awaiting`（web 与 desktop 都接入，图标在 `routeIconForPath` 里登记为 lucide `UserCheck`）。
- 英文 `Waiting on you`，中文 `等我处理`；其余语言先用英文。

### A2 清单页

单列、居中、最大宽度与收件箱一致。每行：

```
[要你做什么的那句话 —— 主文字，text-body font-medium，最多两行]
CLAWSTA-143 · 运行超时界面化…（标题，单行截断） · [头像] 交付方 · 已等 2 天        [打回] [通过]
```

- 主文字是**动作句**，不是标题。标题、编号、交付方、等待时长都放在第二行的次要文字里（`text-caption text-muted-foreground`）。
- 等待时长：`已等 X 分钟/小时/天`，复用现有 `useTimeAgo` 的口径；超过 3 天用现有警示色 token。
- 没写动作句：主文字位置显示「交付方没写要你做什么」（muted），行仍可点开。
- 点行本身打开现有 issue peek；按钮点击不冒泡。
- 排序：`status_changed_at` 升序（等得最久在前）；该字段缺失时退回 `updated_at`。
- 空状态：一个对勾图标 + 「没有等你处理的事」，不加说明文字。
- 键盘：行可 Tab 聚焦、Enter 打开；两个按钮可达，焦点环用现有样式。

### A3 通过 / 打回

- **通过**：`status → done`，不弹确认框；出现 toast「已通过 CLAWSTA-143」带「撤销」（5 秒），撤销恢复 `in_review` 和原 review 元数据。行在服务端成功后移出（不做乐观删除）。
- **打回**：按钮展开 Popover，一个必填文本框（占位「说一下哪里要改」）和「打回」按钮。提交顺序：先发评论（作者为当前用户），再 `status → in_progress`。评论须能唤醒当前负责的智能体——执行前在本仓库确认「成员在智能体负责的 issue 上评论」会不会触发该智能体；不会则评论里带负责人的 mention。失败时保留输入并提示重试。

### A4 看板卡片与列表行

只在状态类别为 `in_review` 时显示：

- `review_by = user` 且是给当前用户的：小标签 **等你**（实心、主题强调色的浅底，`text-micro`），标题下加一行动作句（单行截断，`text-caption text-foreground`）。
- `review_by = user` 但给的是别人：标签 `等 <成员名>`，muted，不显示动作句。
- `review_by = specialist`：标签 **等验收**（描边、muted），悬停提示 `等 <智能体名> 验收`（有 `review_agent` 时）。
- 没有 review 元数据：保持现状，不加任何东西。
- 卡片现有布局、宽度、截断规则不改（B2 已回退的自适应列宽不得带回）。

### A5 「等你」的判定（前端纯函数，放 `packages/core/issues/`）

```
isAwaitingMe(issue, meUserId) =
  issue 状态类别 == in_review
  && metadata.review_by == "user"
  && (metadata.review_user 缺失 || metadata.review_user == meUserId)
```

清单与侧栏数量共用一个查询：现有列表接口 `status_category=in_review` + `metadata={"review_by":"user"}`，再用上式过滤。查询键含 `wsId`，issue 的 WebSocket 更新使其失效。

## B. 数据契约（待后端架构师确认）

复用 `issue.metadata`（已有 CLI `multica issue metadata set`、列表过滤和单键原子写），不加新表：

| 键 | 值 | 说明 |
| --- | --- | --- |
| `review_by` | `"user"` / `"specialist"` | 等用户拍板 / 等专业验收 |
| `review_action` | 字符串，≤ 120 字 | 一句话写清要对方做什么，例如「在公司 G1024152 跑一次 40 分钟编译，没被中途停掉就关」 |
| `review_user` | 成员 user_id，可选 | 指定等哪位用户；缺省表示任一成员 |
| `review_agent` | 智能体 id，可选 | `specialist` 时指明哪位负责人验收 |

需要后端提供：

1. **`status_changed_at`**：issue 响应（列表、详情、WS 事件）新增「进入当前状态的时间」。每次状态变化写入；存量用最近一条 `status_changed` 活动回填，没有则用 `updated_at`。zod schema 设为可选并有回退。
2. **离开 `in_review` 时清掉 `review_*` 四个键**，与状态更新同一事务，避免旧动作句挂在下一轮。
3. **智能体运行说明**（`server/internal/daemon/execenv/runtime_config_sections.go` 里写 `in_review` 的那段）：交付进入 `in_review` 时必须写 `review_by` 和 `review_action`；等用户时写 `review_user`（当前请求用户），等专业验收时写 `review_agent`。
4. `review_action` 超长时后端截断还是拒绝，由后端定；前端按 120 字截断显示。

以上如需调整，后端架构师直接改本节并在 issue 上说明，前端按改后的契约实现。

## C. 验收

- 单元测试：`isAwaitingMe` 判定矩阵；排序（含缺 `status_changed_at`）；通过/打回的成功、失败、撤销；卡片三种标签与无元数据不变。
- 后端：`status_changed_at` 写入与回填、离开 `in_review` 清键的 Go 测试。
- `pnpm typecheck`、`pnpm lint`、`pnpm test`、`make test` 通过。
- **真实数据演示**：在隔离环境导入当前 23 张真实待审核卡（编号、标题、负责人、进入待审核的真实时间），按 CLAWSTA-149 的盘点打标：
  - CLAWSTA-143 → `user`：「在公司 G1024152 装好 v0.2.16，跑一次四十多分钟的编译，中途没被停掉就关」
  - CLAWSTA-149 → `user`：「看完本次汇报，没问题就关」
  - CLAWSTA-148 → `specialist`，`review_agent` = 后端架构师
  - 其余 20 张不打标
  
  截图（亮色、暗色各一套）：侧栏数量为 2、清单页两行按等待时间排序、看板待审核列三种外观、打回 Popover、通过后 toast。外加 1280 和 1024 宽各一张清单页。
- 通过后按版本规则走完整发布（升版本号、差量包、OSS、stable 最后切换）。

## 不在本批

- 移动端（`apps/mobile`）。
- 「等验收」的独立清单页。
- 旧卡的归档清理（由协调负责人另行处理）。
