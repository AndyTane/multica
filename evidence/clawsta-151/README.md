# CLAWSTA-151 R1 验收记录

后续 R2 的最终接收人校验、Windows 测试设施修复、v0.2.16 全量对比和补充截图见 [R2 验收记录](r2/README.md)。本文件保留 R1 当时的执行结果。

本批实现任务书 A/B/C：Web、桌面入口、审核清单、卡片/列表标签、审核字段与状态时间、原子撤销、失败重试、运行说明和测试。版本保持 v0.2.16，发布由后续批次处理。

## 真实数据与截图

`review-cards.json` 是通过 Multica CLI 读取的真实卡片摘要和最新状态活动。取数时共有 24 张待审核；演示按任务书保留 23 张，排除 CLAWSTA-150，143/149 标为等用户，148 标为等专业验收，其余 20 张不标。23 张的时间均来自同任务、同工作区的最新匹配状态活动，本次没有用更新时间估算的卡。

样本只导入独立的 `clawsta151_r1` 测试数据库；卡片编号、标题、负责人名称和审核进入时间保留，测试用户与智能体 ID 在隔离库重新生成。原工作区没有写入这些标记。

`e2e-visual/awaiting-you.spec.ts` 通过真实 API、数据库与 Chromium 渲染并验证：侧栏为 2、清单两行且 143 在前、通过后为 1、撤销后为 2、看板审核列仍为 23 并显示三种外观。亮/暗各有 1280 与 1024 清单、打回框、通过 toast、看板，共 10 张 PNG；已逐张目视检查。`screenshots.json` 记录尺寸与 SHA-256，图片作为 CLAWSTA-151 交付评论的附件提供。

重跑截图需要独立数据库名包含 `clawsta151_r1`，并为测试进程设置 `DATABASE_URL`、`NEXT_PUBLIC_API_URL`、`PLAYWRIGHT_BASE_URL`。启动对应 API/Web 后，在仓库根执行：

```sh
pnpm exec playwright test -c playwright.visual.config.ts awaiting-you.spec.ts
```

## 已验证的行为

- `status_changed_at` 创建时等于创建时间；同状态、标题和元数据修改不重置；匹配历史、无历史、最新不匹配、重复状态记录、缺失 from 的回填及 down 迁移均验证。
- 普通更新、批量、任务共用状态写入与 PR 合并事件入口离开审核时清四键，保留其他元数据；迟到的单键审核写入返回 409。
- 中文和 emoji 共 120 个码点通过，121 个拒绝；空白、非法枚举/类型、错误 UUID、跨工作区用户/智能体拒绝且不改数据。字段错误带键名。
- 撤销与状态一起提交，递增一次 revision，冲突不部分恢复；其他元数据及超过 JavaScript 精确整数范围的数字保持原值。
- 前端覆盖完整分页后过滤计数、稳定排序、三种标签及旧卡原样、通过失败/撤销冲突、打回意见保留、只重试状态及并发冲突提示。

## 实际检查

- `pnpm typecheck`：10 个任务通过；`pnpm lint`：7 个任务通过，有仓库原有 warnings。
- `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`：成功，随后生成目录 diff 为零。
- `go test ./internal/handler -run 'TestReview|TestIssueMetadata|TestIssueTable|TestIssueToMap' -count=1`：通过。最终新增字段错误用例另以 `TestReview|TestIssueToMap` 重跑通过。
- `go test -p 1 ./internal/handler ./internal/service ./internal/daemon/execenv`：service 全套通过；handler 在原有 `TestGetConfigExposesFrontendFeatureFlags` 的空 Queries 调用崩溃；execenv 有原有 Windows HOME 路径与并发 rename 失败。相关失败实现/测试文件相对基线未改。
- `go test ./internal/daemon/execenv -run TestInjectRuntimeConfig -count=1`：通过；内置技能专项也通过。
- `go test ./cmd/multica -run TestValidIssueFieldsMatchListEndpointShape -count=1`：通过，CLI 可选择 `status_changed_at` 字段。
- 上述 Playwright 截图用例：最新编译的 API 与当前前端执行 1 项通过（6.4 分钟），10 张截图；新审核交互 4 项、标签/旧卡 20 项、导航 57 项专项通过。
- 全量前端使用 `node ./node_modules/turbo/bin/turbo test --filter=!@multica/mobile --concurrency=1 -- --maxWorkers=8`：core 175 文件 / 2301 项、docs 3 文件 / 16 项通过；views 476 文件 / 6000 项通过，4 文件 / 5 项失败（原有页面的超时及输入/渲染时序断言），命令退出 1。
- 对上述 4 文件执行 `pnpm --filter @multica/views exec vitest run common/avatar-upload-control.test.tsx auth/registration-page.test.tsx modals/create-project.test.tsx settings/components/mcp-tab.test.tsx --maxWorkers=1 --testTimeout=30000 --hookTimeout=30000`：4 文件 / 58 项全通过。没有修改这些页面来规避失败。
- 因 turbo 在 views 失败后停止，单独执行 `pnpm --filter @multica/web exec vitest run --maxWorkers=4 --testTimeout=30000 --hookTimeout=30000`：25 文件 / 322 项通过，加入页一项标题时序断言失败；随后 `pnpm --filter @multica/web exec vitest run app/join/page.test.tsx --maxWorkers=1 --testTimeout=30000 --hookTimeout=30000`：16 项全通过。
- `pnpm --filter @multica/desktop exec vitest run --maxWorkers=4 --testTimeout=30000 --hookTimeout=30000`：62 文件 / 657 项通过；既有 `scripts/package.test.mjs` 在加载时报 `SyntaxError: Invalid or unexpected token`，该文件及被导入的 `package.mjs` 本批未改，两个文件的 `node --check` 均通过。本轮没有宣称全量前端命令退出 0。

本机没有 `make`，直接 `make test` 报命令不存在。已改用仓库 `scripts/test-go.sh` 和带 agent CLI guard 的 Go 命令执行；全 Go 脚本实际执行过，但全量未绿，除上述失败外还有原有迁移索引清理断言、运行目录标记及 Windows 文件权限相关失败，不能把本批记录当作全 Go 通过。

## 撤销边界

撤销恢复状态和四个审核键，重新进入审核的时间取服务端当前时间。done 已停用的 wakeup 或外部副作用不随撤销恢复，沿用任务书 B4。
