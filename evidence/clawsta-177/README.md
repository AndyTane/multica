CLAWSTA-177 将审批行的“打回”改为“审批意见”，确认按钮使用“发送”。通过仍是一键完成加 5 秒撤销；审批意见仍先发顶层评论，再按评论返回的 revision 写回 in_progress。

## 真实页面证据

- 基线：`587ef1580273baa20e70ebd4f1498ba9b0a7b38c`。
- `feedback-popover-light.png`、`feedback-popover-dark.png`：审批意见弹层、接收人和发送按钮。
- `footer-light.png`、`footer-dark.png`：回复、推进、审批三组和底部脚注，最后一行无下边框，脚注保留一条上边框。
- 1280 × 900，实际 Edge / Next / API / PostgreSQL 渲染；没有拦截接口或用静态页面代替。
- `screenshots.json` 记录各图尺寸和 SHA-256。八张截图在任务评论中作为附件交付（本仓四张）。

## 复现

沿用 CLAWSTA-159 的独立 worktree 环境：先使用 `scripts/init-worktree-env.sh` 和 `scripts/dev-env.sh` 准备隔离数据库并迁移，再按 Makefile 启动独立 API/Web。Windows 缺少 make 时可直接运行构建后的服务端和 Next dev。此次 PostgreSQL 只监听 127.0.0.1:25432；脚本强制检查这个端口和 multica_ 数据库前缀。演示数据在独立用户/工作区生成，卡片和智能体在 finally 中清理。

`pnpm exec playwright test -c playwright.feedback.config.ts`

脚本检查三组最后一行实际计算样式均为 border-bottom-width: 0px，组内前一行仍为 1px，脚注 border-top-width: 1px；同时检查中文输入框名称、占位、必填和发送按钮状态。所有隔离服务已停止，现有本机服务未替换。

## 代码检查

`pnpm typecheck`；`pnpm --dir packages/views exec vitest run --maxWorkers=4 --reporter=dot`；对改动组件运行 ESLint；`go test ./internal/daemon/execenv -run '^TestInjectRuntimeConfig' -count=1`。完整测试结果与基线对照见任务评论及 PR。
