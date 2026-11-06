# CLAWSTA-151「等我处理」清单

负责人：前端负责人（方案与前端验收）；后端架构师（B 节契约确认与后端验收）。

## R1 投递

- 执行：高级开发执行专家；分支 `agent/CLAWSTA-151-awaiting`，base `89cb14411`（main，v0.2.16）。
- 范围：A 节前端全部 + B 节后端全部 + C 节测试与真实数据演示截图。发版不在 R1，验收通过后单独投递。
- 回收：推到同一分支并开 PR（base `main`，不合并），在 CLAWSTA-151 回复 base..head、测试命令与结果、截图附件，@ 前端负责人。前端验收看真实渲染截图，后端验收由后端架构师按 B6 做。
- 版本号：R1 不改版本号，发版批次再升。

## R3 投递：v0.2.17 发布（P1）与真实数据演示

- 执行：高级开发执行专家。前提：R2 前端、后端复审均通过，未关闭阻塞为 0。授权依据：issue 交付要求「实现后按版本规则走完整发布，升版本号」。
- 测试按 C1 执行：发版前只对新代码跑一次前端和 Go 全量，对照 `evidence/clawsta-151/r2/` 中的 v0.2.16 清单，不再跑 v0.2.16。全量结果更新为 v0.2.17 的基线清单。
- 推送、合并、发布只做下面列出的动作，不夹带其他改动。凭据只从本机用户环境变量读取，不得出现在命令参数、日志、评论或截图里。

### 1. 升版本号并合入

- 在 PR #48 原分支追加一个提交：`package.json`、`apps/web/package.json`、`apps/desktop/package.json` 改为 `0.2.17`。测试夹具和演示数据里提到的 v0.2.16 是数据，不改。
- 用 merge commit 合入 main，保留历史（与 #47 相同），并确认合入后的 main 与 PR head 树一致。
- 在合入后的 main 提交上打 tag `v0.2.17`，并推送 tag。

### 2. 构建与发布

- 用干净检出的 `v0.2.17` 源码，按 `docs/windows-oss-releases.md` 的「家里差量发布」，以 v0.2.16 原始未安装整包为基线，构建差量包 `v0.2.16 → v0.2.17`。使用新的输出目录，不复用载荷，不加 Force。
- 发布四个对象：`releases/v0.2.17/` 下的 `upgrade.zip`、`client-kit.zip`、`UPGRADE.md`、`release.json`。`release.json` 记录 tag 和提交。
- 上传后校验 SHA-256 和 OSS CRC64，并用 Fetch 真实下载两个包、解压核对清单。全部通过后，最后一步才切换 `channels/stable/latest.json`，并读回确认。
- 任何一步失败，都不覆盖、不删除已上传的对象。改用新版本号，并在回复里说明。

### 3. 隔离升级验证

- 在隔离目录用 v0.2.16 原包重建基线，再用官方 `Apply-Upgrade.ps1` 升级到 v0.2.17。
- 核对：程序清单和删除项；样本数据保留；迁移 584 已执行，`status_changed_at` 已回填；服务正常启动，版本显示 v0.2.17。

### 4. 真实数据演示（Tane 验收用）

- 用第 3 步升好的 v0.2.17 隔离实例，按 C 节导入当前真实的待审核卡并打标：143、149 为 `user`，148 为 `specialist`，其余不打标。导入前重新读取当前看板，用实际数量，不照抄 23。
- 截图，亮色和暗色各一套：
  - 侧栏「等我处理」的数量
  - 清单页按等待时间排序
  - 看板待审核列的三种外观
  - 打回弹出层，以及通过后的 toast
  - 另加 1280 宽和 1024 宽清单页各一张
- 截图只用于演示，不写进真实看板数据。

### 5. 落地记录与回收

- 按版本规则第 5 条新开发布落地任务，标题为「v0.2.17 发布落地（CLAWSTA-151）」。在里面写：版本、tag、提交、起止版本、对象 SHA-256、stable 切换时间。
- 台式机 `D:/UWorkers/Portable` 现场升级和移动机自动补升属于 P2，后端验收 P1 证据之后，在落地任务里另行投递。本批不动现场安装。
- 在 CLAWSTA-151 回复并 @ 前端负责人，内容包括：
  - 合并提交、tag、四个对象的 SHA-256
  - 全量测试结果及与清单的对照
  - 隔离升级结果
  - 演示截图（作为附件）
  - 落地任务链接
- 证据放在 `evidence/clawsta-151/r3/`。证据提交在合入之前追加到 PR 分支；如果合入后才产生，就跟着落地任务走，不再改 main。

## R2 投递（R1 验收后的集中修复）

- 执行：高级开发执行专家；同一分支、同一 PR #48，在 `d39ba230e` 之上追加提交，不改写历史。版本号不改。
- 来源：前端验收通过（2 处小瑕疵）；后端 B6 需要修改（1 处打回契约）+ C 节全量测试未收口。
- 回收：在 CLAWSTA-151 回复 `d39ba230e..head`、各项结果与证据路径，@ 前端负责人。后端架构师只复审本批新增差异和下面第 1、3 项。

### 1. 打回必须确认意见真的交回负责人（阻塞，B4）

`sendReviewFeedback` 现在补 @ 后不看第二次预览结果。`/note 请修复编译错误` 两次预览都是 0 个接收人，评论照发，状态照改成 `in_progress`，结果没人接手。

- 以**最终要发送的内容**做的那次预览为准：负责人是智能体时，预览里必须有该智能体；是小组时，必须有 `source` 为 `issue_assignee` 或 `mention_squad_leader` 的条目（后者须来自补的那个小组 @）。负责人是成员时不做此检查。
- 预览执行对象增加可选 `squad_id`，取自本次实际路由的小组；`mention_squad_leader` 必须同时匹配这个 id，不能仅凭同一个负责人或来源字符串认定小组已收到。普通智能体响应省略此字段；旧服务端缺此字段时，仍可确认 `issue_assignee` 兜底，但不猜测小组 @ 的归属。
- 最终预览仍不包含负责人：**不发评论、不改状态**，弹出层保留已填意见，在弹出层内显示原因。文案：
  - 意见以 `/note` 开头：「以 /note 开头的意见只做记录，不会交给 {{name}}」
  - 其他情况：「这条意见到不了 {{name}}，打回没有完成」
- 预览请求失败照旧报错，不发评论。不绕过 note、权限或运行时限制。
- 保留「评论已成功、只重试状态」的行为。
- 测试（`packages/core` 的 helper 测试负责判定矩阵，views 测试只验接线）：普通意见、显式 @ 别人导致兜底被抑制后补 @ 成功、`/note`、补 @ 后最终预览仍为空、评论成功状态失败只重试状态。

### 2. 前端两处小瑕疵

- 清单行第二行：标题去掉固定的 `max-w-[260px]`，改为可收缩截断（`min-w-0` + `truncate`，按需 `flex-1`/`basis`），让「·」紧跟省略号；1024 宽下编号、负责人、已等时间不被挤掉。
- 打回弹出层：去掉与按钮同名的可见标题「打回」，输入框保留可访问名称「打回意见」（`aria-label`），占位提示仍是「说一下哪里要改」。五种语言同步。

### 3. C 节全量测试收口

C 节要求 `pnpm typecheck`、`pnpm lint`、`pnpm test` 和后端全量测试通过。本机没有 `make`，后端用等价命令（`cd server && go test ./...`，按 Makefile 的 `test` 目标补齐环境）。

- 在 head 和 base `89cb14411`（v0.2.16 已发布提交）上用同样的命令、同一台机器、同一隔离数据库各跑一遍全量，失败清单逐条对比。
- **head 不允许有 base 没有的失败。** 新出现的失败一律修。
- base 上就有的失败：逐条写一句原因（Windows 环境、测试自身缺陷、偶发），能在测试设施层面小改修好的就修（例如 desktop `scripts/package.test.mjs` 的语法错误若是测试文件或 Node 版本问题）；涉及产品行为的不在本批改，列入清单交回。
- 偶发失败：单独重跑 3 次，记录结果。
- 证据：两份完整输出放 `evidence/clawsta-151/r2/`，另附一张对比表（失败项 / base / head / 原因 / 处理）。

### 4. 截图

只补本批改动处的真实渲染：清单页 1024 亮色一张（看第二行）、打回弹出层亮暗各一张、`/note` 失败提示一张。

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

- **通过**：`status → done`，不弹确认框；出现 toast「已通过 CLAWSTA-143」带「撤销」（5 秒），撤销按 B4 用 `review_metadata` 原子恢复 `in_review` 和原四键；409 时提示「任务已有变化」并刷新。行在服务端成功后移出（不做乐观删除）。
- **打回**：按钮展开 Popover，一个必填文本框（占位「说一下哪里要改」）和「打回」按钮。提交顺序：先发顶层评论（作者为当前用户，由现有成员评论接手机制交回负责人），再 `status → in_progress`。评论成功而状态失败时显示「意见已发送，状态未更新」，重试只重试状态（细则见 B4）。评论失败时保留输入并提示重试。

### A4 看板卡片与列表行

只在 `issue.status === "in_review"` 时显示：

- `review_by = user` 且是给当前用户的：小标签 **等你**（实心、主题强调色的浅底，`text-micro`），标题下加一行动作句（单行截断，`text-caption text-foreground`）。
- `review_by = user` 但给的是别人：标签 `等 <成员名>`，muted，不显示动作句。
- `review_by = specialist`：标签 **等验收**（描边、muted），悬停提示 `等 <智能体名> 验收`（有 `review_agent` 时）。
- 没有 review 元数据：保持现状，不加任何东西。
- 卡片现有布局、宽度、截断规则不改（B2 已回退的自适应列宽不得带回）。

### A5 「等你」的判定（前端纯函数，放 `packages/core/issues/`）

```
isAwaitingMe(issue, meUserId) =
  issue.status == "in_review"
  && metadata.review_by == "user"
  && (metadata.review_user 缺失 || metadata.review_user == meUserId)
```

清单与侧栏数量共用一个查询：现有列表接口 `status=in_review` + `metadata={"review_by":"user"}`，分页取完后再用上式过滤、计数、排序（不以第一页长度当总数）。查询键含 `wsId` 与当前用户；刷新时机见 B2。

## B. 数据契约（后端已确认，R1 实现依据）

本节基于 `8f4001d5` 的实际代码确认；仅修改契约，功能由前端负责人统一投递实现。与 A 节的数据口径冲突时以本节为准，交互和布局仍按 A 节。

### B1 字段与输入校验

复用 `issue.metadata`，不加新表；原有其他元数据、50 键和 8KB 限制不变。

| 键 | 值 | 说明 |
| --- | --- | --- |
| `review_by` | `"user"` / `"specialist"` | 等用户拍板 / 等专业验收 |
| `review_action` | 非空纯文本，最多 120 个 Unicode 码点 | 去掉首尾空白后计数；Go 用 rune，前端用 `Array.from`，不按字节或 UTF-16 长度计数 |
| `review_user` | 工作区成员的 user_id，可选 | 缺失沿用 A5 的“任一成员可处理”；新交付必须明确当前请求用户，不能把缺失当作默认群发 |
| `review_agent` | 同工作区智能体 id，可选 | `specialist` 时指明验收负责人；不用于表示交付人，交付人沿用 issue 的负责人 |

- **超过 120 个码点拒绝，HTTP 400，不截断保存**；空白、错误类型和非法枚举同样拒绝，返回明确字段错误。拒绝前不修改任何字段。前端只做行数省略，打开任务仍可读完整动作句。
- `review_user` / `review_agent` 验证 UUID 和工作区归属，沿用现有访问权限，不允许跨工作区引用。删除键沿用 DELETE，不用空串或 JSON null 代替。
- 原有无标记/缺动作句的卡仍可读、可流转，不做全库补标，不把新交付规则变成旧卡无法更新的门槛。

### B2 查询与时间

- **纠正 A4/A5 的状态口径**：当前代码的 `status_category` 只有 `unstarted / started / done / closed`，`in_review` 是具体状态键。本批用 `issue.status === "in_review"` 判定，列表请求为 `status=in_review` + `metadata={"review_by":"user"}`；不要请求 `status_category=in_review`，也不要把全部 `started` 当待审核。自定义状态不自动归入本入口。
- 清单和侧栏数量必须使用同一份完整查询结果；沿用现有分页取完匹配卡后再按 `review_user` 过滤、计数及排序，不能把第一页长度当总数。查询键包含工作区和当前用户；`issue:updated`、`issue:metadata_changed`、删除及重连刷新都须使结果更新。
- 新增 `issue.status_changed_at TIMESTAMPTZ NOT NULL DEFAULT now()`，服务端只读输出为 RFC3339。创建时取创建时间；只有实际状态键变化才更新时间，重复设置同状态、评论、标题、负责人和元数据修改均不重置。
- 存量回填：按同工作区、同 issue 查 `activity_log.action = 'status_changed'` 的最新记录（`created_at DESC, id DESC`）；仅当该条 `details->>'to'` 等于当前状态且 `from` 不同，才用其 `created_at`，否则用迁移前 `updated_at`，再兜底 `created_at`。不得倒找更早的一次同名状态来伪造本轮时间。无匹配历史的回退值是估算，演示材料注明来源，不能标为真实状态进入时间。
- 迁移先加可空列、回填，再设置默认值和非空，配套 down；回填不改 `updated_at`、revision 或触发业务通知。列表/详情、表格投影、`service.IssueToMap` 和 WS 中所有 issue 快照一起增加字段，更新 sqlc、共享 TS 类型及 zod 可选字段。旧服务端缺字段时按 A2 回退 `updated_at`；相同时间按 issue id 稳定排序。

### B3 状态写入与清键

采用 issue 表的 `BEFORE UPDATE OF status` 触发器统一维护时间与四键清理：仅在 `OLD.status IS DISTINCT FROM NEW.status` 时更新 `NEW.status_changed_at`；且从 `in_review` 离开时从 **NEW.metadata** 精确删除 `review_by / review_action / review_user / review_agent`。不按 `review_` 前缀删除，不覆盖其他键，不另起 UPDATE。这样普通更新、批量更新、任务状态写入和 PR 自动完成都在原事务中得到相同行为。

原有状态写入负责递增 revision；触发器不额外递增。所有响应和广播取写后快照，避免数据库已清键而浏览器仍显示旧动作。单键写 review 字段时也在行锁内验证当前仍为 `in_review`，离开后并发到达的写入返回 409，不能把旧审核内容写回进行中任务。

### B4 提交审核与撤销的原子写入口

现有 `UpdateIssueRequest` 不支持 metadata，不能假定 A3 的一次状态更新 能恢复四键。扩展现有 `PUT /api/issues/{id}`，增加可选的 `review_metadata` 对象（仅接受上述四键）；提供时须同时传 `status: "in_review"` 和 `expected_revision`。在原有锁定 issue 的事务中验证版本、字段和成员归属，仅替换四个保留键，保留其余元数据，与状态变更作为一次提交并递增一次 revision；失败全部回滚。省略该字段时保持旧 API 行为，不增加任意整包 metadata 覆盖能力。

- **通过**：原有 PUT `{status: "done", expected_revision}`；保存通过前的四键快照和成功响应的 revision。
- **撤销**：5 秒内 PUT `{status: "in_review", review_metadata: <原四键快照>, expected_revision: <通过响应 revision>}`。并发修改返回 409，刷新并提示任务已变化，不自动拿新 revision 覆盖别人。重新进入审核的时间按 B2 取当前时间，不接受客户端回写历史时间。撤销范围是状态和审核字段，不恢复关闭时已经停用的 wakeup 或外部副作用。
- **打回**：沿用 A3 先评论后改状态。代码已确认普通成员的新顶层评论会走 `routeAssigneeFallback` 接到当前负责的智能体/团队，无需例行添加 mention；不要把它发成指向其他智能体的线程回复。接手仍受原有权限、运行时和调度条件约束，离线或无负责人的情况不能显示成“已启动”。
- 评论成功后用响应中的 `issue_revision` 做 `in_progress` PUT 的 `expected_revision`。评论成功而状态失败时保留 comment id，提示“意见已发送，状态未更新”，同次重试只重试状态，不再次评论或重复唤醒；409 时刷新交由用户决定，不能覆盖已运行智能体的新改动。若输入里有 mention/特殊评论语法，使用现有触发预览确认仍会送到交付负责人；必要时明确附上负责人 mention。`in_review → in_progress` 本身不启动运行。

### B5 智能体运行说明

在 `server/internal/daemon/execenv/runtime_config_sections.go` 的交付进入 `in_review` 规则旁增加以下约束，并同步内置 `multica-platform/references/issues.md`：

1. 分清本轮等用户拍板还是等专业验收，写动作句和具体接收人；不因本轮结束就默认等用户。动作句保留真实验收条件，超过 120 个码点须主动缩写，不得悄悄删条件。
2. CLI 沿用 `multica issue metadata set <id> --key <key> --value <value> --type string`。先进入 `in_review`；已有标记重交时先删 `review_by`，清掉另一种接收人键，再依次写 `review_action`、`review_user` 或 `review_agent`，**最后写 `review_by`**，避免先出现一张指向错误用户或仍带旧动作的卡。
3. 每步检查成功，失败停止并说明具体未写入项。由可信的当前请求上下文或任务要求确定成员 user_id；不猜测、不硬编码 Tane 的 id，无法确定时在交付评论明确缺谁的决定，不擅自标成任一成员可处理。
4. `specialist` 的 `review_agent` 表示验收人，不改交付负责人的 assignee。仍按现有正式任务/评论投递规则安排专业验收，元数据写入本身不派活。

### B6 本批后端验收与依据

按 C 节完成代码检查，专项验证以下实际行为：回填匹配/无历史/最新记录不匹配；创建、同状态和非状态编辑时间不变；所有状态写入入口离开审核清四键且保留其他元数据；并发 review 写入不回灌旧键；120/121 个码点（含中文和 emoji）、非法类型和跨工作区目标；原子撤销的成功、冲突及无部分恢复；打回评论成功但状态失败的重试不重复派发。真实 23 卡演示沿用 C 节范围，回填估算单独标注。

决策依据：`server/internal/handler/issue_metadata.go` 已有单键原子写，整包覆盖会丢并发数据；`server/pkg/db/queries/issue.sql` 和 `pr_automation.sql` 有多个独立状态入口，所以用一个行级触发器保证不漏；`server/cmd/server/activity_listeners.go` 的历史字段为 `details.from/to`；`server/internal/issuestatus/issuestatus.go` 区分状态与分类；`server/internal/handler/comment.go` 有成员评论接手与 `issue_revision` 返回，`server/internal/service/issue_trigger.go` 的状态启动仅覆盖离开 backlog。复用这些机制，不新建审核表、派发器或后台任务。

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

### C1 测试分档（R2 之后的批次执行，取代上面「每批 base/head 各跑全量」的做法）

- **日常修复批**：只跑改到的包和对应专项测试，加 `pnpm typecheck`、`pnpm lint`；动到 Go 的只跑受影响包（`go test ./internal/<包>/...`）。不跑 base。
- **发版批**：只对新代码跑一次前端与 Go 全量，不再跑 v0.2.16。
- **旧失败对照**：以 `evidence/clawsta-151/r2/`（`comparison.md`、`frontend-comparison.md`、`checks.json`）为 v0.2.16 基线清单。全量结果与清单逐项对照，清单内的旧失败不再重跑；清单外的新失败才重跑 3 次，稳定失败即为阻塞。
- Vitest worker 启动超时：只补跑未执行的文件一次，并在证据中写明；不整条重跑。
- 每次发版后把新版本的全量结果更新为下一版基线清单。

## 不在本批

- 移动端（`apps/mobile`）。
- 「等验收」的独立清单页。
- 旧卡的归档清理（由协调负责人另行处理）。
