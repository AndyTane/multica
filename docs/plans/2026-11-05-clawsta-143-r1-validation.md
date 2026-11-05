# CLAWSTA-143 R1 验证记录

候选从 main `03f57f4cf5292ef4c190a1ef96eccda50d344ab5` 开始，应用清单升为 `0.2.14`。维护者任务书原件与本分支副本 SHA-256 均为 `70FBCDD321AC2079805686A9540B876D04637B7E0C73F9D250829194105CED14`。

本轮交付代码候选。专业验收、正式发布及公司现场验收由后续流程接续；本轮没有合并、打 tag、上传 OSS、切 stable、替换现场程序或停止真实任务。

## 已通过的专项检查

| 范围 | 实际验证 | 证据文件 |
| --- | --- | --- |
| 服务端保存与权限 | 保存、重新读取、继承、清除覆盖、零值、非法类型/小数/溢出、无关 JSON 键、旧客户端更新、任务 token、无权成员、不同工作区 | `target.log`，`server/internal/handler/execution_timeouts_test.go` |
| 领取与快照 | 单/批领取、智能体优先、工作区默认、无策略兼容、旧客户端升级状态、同一智能体连续领取读取新值、旧快照不变 | `target.log`，`TestExecutionTimeoutClaimRefresh` |
| 客户端计时 | 本机短窗口被显式服务端策略替代；越过旧窗口完成、到新窗口终止、整轮硬上限、工具开始/结束、持续消息重置、并发不串值、第二轮刷新 | `target.log`，`server/internal/daemon/execution_timeouts_test.go` |
| 引擎与兼容 | OpenCode 子进程收到 `5400000` 毫秒工具默认值；其他 provider 不映射该字段；Codex semantic/first-response 采用服务端预算；HTTP/批领取畸形策略拒绝，缺失完整策略不会降级 | `target.log` |
| 界面 | 18 项分文件通过：编辑器 7、工作区入口 7、智能体入口 4；覆盖保存、继承、错误、只读、来源、扩展显隐、缓存刷新、旧服务端与未绑定运行环境 | `ui-final-capabilities.log`、`workspace-capabilities.log`、`editor-final.log` |
| 共享解析 | Core 全量正常 worker 下 175 文件、2290 项通过；最新策略及服务端能力边界 3 项通过；模型轮询/上传等 27 项定向检查通过 | `frontend-forks.log`、`core-capabilities.log`、`core-authoritative.log` |
| 类型 | 根 `pnpm typecheck`：10 项任务通过；追加兼容性后的共享 views 类型检查通过 | `typecheck.log`、`views-typecheck-capabilities.log` |
| Lint | 根 `pnpm lint`：7 项任务通过；本批文件追加检查通过；既有文件有 warning | `lint.log`、`lint-capabilities.log` |
| SQL | `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` 成功；仅两份相关生成文件有内容变化 | `server/pkg/db/generated/agent.sql.go`、`workspace.sql.go` |
| 视觉 | 6 张实际共享组件截图；浏览器无运行错误、无横向溢出；已核对桌面和 390 像素窄屏 | `ui-*.png`、`ui-capture.json` |

截图使用生产组件、翻译和样式，在隔离预览中传入模拟设置；服务端读写与领取链路由隔离数据库测试验证。截图没有操作在线账号、现场服务或真实任务。

## 整仓检查及环境边界

- 原生 Windows 没有 `make`/GCC，`CGO_ENABLED=0`。执行了原生等价的 `go test -p 2 ./...`；未声称 `make test --race` 通过。
- 整仓 Go 没有全绿：现有 CLI task marker、Windows 文件模式/符号链接/路径/Job Object，以及受负载影响的既有子进程超时等用例失败。handler 整仓失败项为 `TestStableIDSuffixMatchesDaemon` 和 `TestGetConfigExposesFrontendFeatureFlags`，R1 专项通过。完整输出保留在 `go-full.log`。
- 最初的 Vitest threads worker 启动超时；VM worker 下有跨计时器相关失败。正常 forks worker 重跑的 Core 全量为 **175 文件、2290 项通过**，受影响的模型轮询与上传专项也全部通过。其余前端整仓运行结果在交付评论和证据包补齐。
- 服务端以只读 `execution_timeouts_supported` 标记真实支持能力；客户端以 `execution-timeouts-v1` 标记执行能力。新界面连接旧服务端时禁止写入此组配置，未绑定运行环境时不误报客户端版本过旧。
- 未执行真实付费模型调用。秒级临时 OpenCode/Codex 子进程测试验证的是平台计时与协议，不等同于真实四十分钟业务编译。

数据库使用仓库脚本生成的 `.env.worktree`，连接端口 `25433` 的独立 PostgreSQL 数据目录。末轮一次补测漏载环境，在初始化检查阶段退出；已改回带隔离断言的统一测试入口，重新通过。原现场数据库与常驻 daemon 未参与测试。

## 交付证据

上述日志、截图、隔离截图脚本和汇总随 CLAWSTA-143 最终交付评论提供附件；代码位置均为仓库相对路径。截图脚本是验证材料，不进入产品入口。正式发布前须由协调侧重新检查最新 main、未用版本号及既有同号服务端/Client-Kit 发布流程。
