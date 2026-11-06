# CLAWSTA-151 R2 验收记录

本批在 PR #48 / `agent/CLAWSTA-151-awaiting` 上追加。程序与测试检验点为 `627b81a382c0386d8c8ff7cb114da5e52d688a9e`；其后仅整理证据和说明。版本保持 v0.2.16，后续由专业验收接续发布。

## 打回与界面

`sendReviewFeedback` 检查最终内容对应的触发预览：智能体匹配 id，小组匹配负责人兜底或带本小组 `squad_id` 的 mention 来源。预览仍无法交到负责人时抛出可识别的错误，页面保留可编辑意见及原状态。`/note` 的记录语义保持，预览请求失败也不发评论。评论成功后保留 comment id/revision，重试只更新状态。

core 的 15 个接收人判定用例覆盖普通意见、显式 @ 别人、note、空/失败预览、其他小组/共享负责人、旧服务端响应和成员负责人；views 的 6 个交互用例覆盖接线、保留意见、通过/撤销、冲突及只重试状态。Go 真实预览验证小组来源、共享负责人去重、note 不派发。它们均纳入最终全量。

第二行标题使用可收缩截断；弹出层移除重复标题，输入框保留「打回意见」名称，五语键同步。

## 同条件全量

同一台 Windows amd64，Node v24.21.0，Go 1.26.9；数据库始终为同一个独立 PostgreSQL `uworkers_clawsta151_r1`，loopback 端口 15651。凭据从隔离部署配置读入子进程环境，证据中移除凭据值。

基线程序固定为已发布 v0.2.16 的 `89cb144113609c944f0d6cb0c8924c4b24be7958`。Go 在同一比较 checkout 切换两个提交执行；base 使用截至 583 的 schema，head 应用 584。两边传入相同的 DATABASE_URL、GOMAXPROCS=4、CGO_ENABLED=0 及下列参数。基线全量使用的初始纯测试补丁保留在 `base/common-fixtures-initial.patch`；程序实现保持发布版。全部设施修复及适用边界见 `fixture-repairs.md`。

```text
node ./node_modules/turbo/bin/turbo typecheck --filter=!@multica/mobile --concurrency=1 --continue=always --force
node ./node_modules/turbo/bin/turbo lint --filter=!@multica/mobile --concurrency=1 --continue=always --force
node ./node_modules/turbo/bin/turbo test --filter=!@multica/mobile --concurrency=1 --continue=always --force -- --maxWorkers=1 --testTimeout=30000 --hookTimeout=30000
go run ./cmd/migrate up
bash scripts/go-test-with-agent-cli-guard.sh -- go -C server test -json -p 2 -parallel 2 -count=1 ./...
```

前三条在根目录，migrate 在 server，最后一条在根目录。它们执行仓库 root scripts 对应的检查，覆盖前端六个非 mobile 测试包及全部 Go 包。本机无 make，Go 使用任务书允许的直接全量命令；本次 CGO 关闭，未运行 race。

| 检查 | base | head |
| --- | --- | --- |
| typecheck | 10/10 任务通过 | 10/10 任务通过 |
| lint | 7/7 任务通过 | 7/7 任务通过 |
| 前端全量 | 751 文件 / 9343 项通过；1 次 worker 启动超时；退出 1 | 755 文件 / 9394 项通过；1 次 worker 启动超时；退出 1 |
| Go 全量 | 95 个包；446 条失败记录 | 95 个包；390 条失败记录；新增失败 0 |

Go 的失败记录包含父/子用例，不能当作独立缺陷数。两条 Go 全量命令退出码均为 1，原因是下表记录的发布版旧失败。`comparison.md` 逐条列出 base/head、原因和处理，`failure-details.json` 保留每项原始诊断；全部 390 条 head 失败均存在于基线。

前端分包、三次补跑和完整覆盖计算见 checks.md / checks.json。全量输出置于 `base/`、`head/`，大文件使用 gzip 无损压缩，SHA-256 与解压后字节数见 `logs-manifest.json`。本目录关闭 Git 文本换行转换，保持跨平台下载后的校验值一致。复现入口为 `run-suite.cjs`，从环境传入 `R2_NATIVE_RUNTIME` 和可选 `R2_REPO`；其隔离库名称/端口校验用于防止误连。

## 失败定位与复跑

完整保留了准备/中断尝试：名单 CRLF 导致 guard 提前退出；旧任务目录标记影响 CLI 凭据解析；中断测试遗留的注册开关；R1 搜索对照扫描少一列及清理批次受历史记录影响。最终正式 Go 输出为 `head/go-complete.log.gz`，源代码 `627b81a38`，上述新增失败已经修复。

两次 4 worker 的 head 前端全量已执行断言通过，但均有 Vitest worker 未在固定 60 秒启动窗口内响应，命令退出 1；首次缺失 create-agent-dialog 的 10 项覆盖。原输出保留为 `head/frontend-worker-start-attempt.log`、`head/frontend-workers4-second-attempt.log`。该文件与 R1 的四个时序文件、Web 加入页分别独立重跑三次，每次 68 + 16 项通过。最终基线和 head 统一改为上述单 worker 命令，各自使用独立的物理 Vite 缓存；没有修改依赖的启动超时或产品逻辑。两份最终前端原输出分别为 `base/frontend-complete.log` 与 `head/frontend-complete.log`。基线此前的桌面脚本加载失败保留在 `base/frontend.log`，修复换行/Node 环境后纳入最终重跑。

| 复跑 | 三次结果 |
| --- | --- |
| 迁移大数据夹具的 90 秒超时 | base 三次通过，判定首次为偶发超时 |
| 不同运行环境连续删除 | base 三次均复现只注册一次，属于稳定旧行为 |
| 搜索对照与源上下文清理 | head 三次全部通过 |
| 首次启动超时文件与 R1 时序文件、Web 加入页 | head 三次分别 68、16 项全部通过 |
| 第二次启动超时的两个 views 文件 | head 三次每次 4 项全部通过（worker-repeat1/2/3） |
| Windows Dim 进程树、运行时探针数量、OpenClaw 并发缓存及最坏预算 | head 三次均复现原有失败，按稳定旧失败列入对比表 |

原有产品行为交回复审：迁移 574/575/579/580 的并发索引清理注册；Windows 归档路径、目录身份/句柄锁及仓库路径；运行环境删除合并窗口；OpenClaw 缓存/预算；Hermes 外部目录/共享链接契约；企业微信打开文件的删除生命周期。协议假进程还需补 Windows 实现。每条对应的用例、平台原因和原始诊断均在对比表中。

## 四张真实渲染

`screenshots/`：1024×900 亮色清单；1280×900 亮/暗打回层；1280×900 note 错误。`screenshots.json` 记录尺寸及 SHA-256。采用 R1 的 23 张真实卡摘要，在独立 API/数据库和 Chromium 中渲染，原工作区未写演示标记。

`CLAWSTA151_REVIEW_ROUND=R2` 的 `e2e-visual/awaiting-you.spec.ts` 实际通过（1 项，2.8 分钟）。note 失败前后的数据库快照一致：状态、revision、评论数、运行队列数均未变化；意见仍可编辑、清单仍有两行。渲染输出另存 `render-r2.log`。

复现截图使用隔离部署对应的 DATABASE_URL、NEXT_PUBLIC_API_URL、PLAYWRIGHT_BASE_URL，并运行：

```text
pnpm exec playwright test -c playwright.visual.config.ts awaiting-you.spec.ts
```

## 收尾

本次交付沿用 PR #48，版本与发布通道保持该批约定。全部本批测试进程已结束，隔离数据库已停止，15651/18651/13651 均无监听；Multica 守护进程保持运行。记录见 cleanup.json。前端失败项与三次补跑对比另见 frontend-comparison.md。

## 最终前端对照的偶发项收口

单 worker 的最终对照中，两边 views 各有一次同源的 Vitest forks worker 启动超时，固定引导窗口为 60 秒；已执行断言全部通过。base 缺少 describe.test.ts 的 16 项，head 缺少 file-card.test.tsx 的 1 项。这两个文件在 base/head 各独立重跑三次，每次 17 项全部通过；补齐完整覆盖为 base 9359 项、head 9395 项。新增断言失败 0、新增失败包 0。两条全量命令均因该框架错误退出 1，原始退出码保留。框架引导超时在当前版本中为内部固定值，仓库未对依赖文件打临时补丁；按 R2 偶发项三次复跑口径记录，后续测试策略由前后端负责人接续调整。
