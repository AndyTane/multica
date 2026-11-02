# CLAWSTA-104 第二台机器接入：Windows 安装包一键启动守护进程（含证书信任）

日期：2026-11-01
范围：本任务书。交付物是「另一台 Windows 机器不用编译：装一个安装包，一条命令连上台式机服务端并启动守护进程，局域网自签证书不报错」，外加三步入门文档与已有下载/安装入口的更新。
权威文档：`docs/windows-second-machine.md`（README 里各语言各有一段三步摘要）。

## 交付内容

| 层 | 文件 | 作用 |
| --- | --- | --- |
| 单文件安装包 | `server/cmd/u-workers-setup/` | Windows 单文件 exe：内嵌当前版本 CLI，按当前用户安装 `u-workers.exe` + `multica.exe` 到 `%LOCALAPPDATA%\U Workers\bin`，维护用户 PATH 与「添加/删除程序」条目，无需管理员权限；支持 `--install-dir/--no-path/--no-connect/--silent/--uninstall/--version`，并把 `--server-url/--root-ca/--token/--app-url/--skip-browser-trust` 透传给 connect |
| 打包 | `scripts/build-windows-installer.ps1` | PS7 脚本：编译 CLI、stage 到 payload、编译安装包，产物 `dist/u-workers-setup-<version>-windows-amd64.exe`（约 21 MB），总是清理 payload |
| 一条命令 | `server/cmd/multica/cmd_connect.go` | `u-workers connect`：验证服务端证书确由给定根证书签发 → 保存信任锚 → 浏览器信任 → 保存地址 → 登录（浏览器或 `--token`）→ 启动/重启守护进程 → 等运行时注册上线 |
| 证书信任 | `server/internal/tlsx/` | `U_WORKERS_ROOT_CA_FILE`（旧名桥接可用）把内网根证书追加到系统信任池：CLI API、daemon API、skill bundle 下载、WebSocket 拨号全部复用；**从不关闭校验**，只增加一个信任锚 |
| 证书信任（浏览器） | `server/internal/wintrust/` | 把公开根证书装进当前用户 Root 存储（Windows 会弹一次系统安全警告，需要点「是」）；幂等，已存在不重复写 |
| CLI/daemon 接线 | `server/internal/cli/client.go`、`stall.go`、`server/internal/daemon/client.go`、`wakeup.go`、`config.go`、`cmd/multica/main.go`、`cmd_config.go` | 所有出站 HTTPS 客户端共用 tlsx 信任池；`config.json` 新增 `root_ca_file`，`config set root_ca_file <path>` 可写入/清空；daemon 启动时从 profile 配置读回 CA |
| 文档 | `docs/windows-second-machine.md`、`README.md`、`README.zh.md` | 3 步接入：导出根证书+安装包 → 双击安装 → 一条 `u-workers connect`；含排错与卸载 |
| 界面入口 | `packages/views/runtimes/components/connect-remote-dialog.tsx`、`packages/views/onboarding/steps/cli-install-instructions.tsx` | 已有的「添加电脑 / 安装 CLI」位置改为新的一条命令 `u-workers connect ...`，token 兜底流程同步换名 |
| 端到端验证 | `server/e2e/secondmachine/`（`-tags secondmachine_e2e`） | 干净用户目录 + 假 HTTPS 服务端（私有 CA）：安装安装包 → 一条命令 → 守护进程上线 → 服务端收到运行时注册；另有「不给根证书必须失败且不写配置」用例 |

## 怎么用（3 步）

1. 台式机：`pwsh -File deploy\windows-https\caddy-windows.ps1 -Action ExportRoot -Root <deployment>` 导出 `root-ca.crt`；`pwsh -File scripts\build-windows-installer.ps1` 生成安装包。两个文件拷到新机器。
2. 新机器：双击安装包（或命令行跑一次）。
3. 新机器：`u-workers connect --server-url https://192.168.3.49:13443 --root-ca .\root-ca.crt`；Windows 弹一次安全警告点「是」，浏览器完成登录，守护进程上线。

## 验证结果（本地 Windows / Go 1.26.9 / pnpm 10.28.2）

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| Go 全量编译 | `cd server && go build ./...` | 通过 |
| tlsx 单测 | `go test ./internal/tlsx/` | 通过（含「配置的 CA 能通、清掉后必须校验失败、无关 CA 必须失败、TLSConfig 不得关闭校验」） |
| wintrust 单测 | `go test ./internal/wintrust/` | 通过（10 用例，不触碰真实证书库） |
| CLI 命令测试 | `go test ./cmd/multica/` | 通过（`TestConnect*` 用私有 CA 的 httptest TLS 服务端覆盖：成功路径写配置/存 CA、无关 CA 在写盘前失败、纯 HTTP 必须要求 `--app-url`） |
| 安装包单测 | `go test ./cmd/u-workers-setup/` | 通过（payload/PATH/安装卸载/参数透传，fstest 假 payload） |
| 安装包构建 | `pwsh -NoProfile -File scripts/build-windows-installer.ps1 -Version 0.0.0-e2e` | 通过，`dist/u-workers-setup-0.0.0-e2e-windows-amd64.exe` 21,572,608 字节 |
| 安装/卸载冒烟 | `u-workers-setup.exe --install-dir <tmp> --no-path --no-connect`，再 `--uninstall` | 通过：两个 exe 都装上并能 `version`；卸载移除文件与注册表项，重复卸载退出 0 |
| 端到端 | `U_WORKERS_E2E_SETUP_EXE=<exe> go test -tags secondmachine_e2e ./e2e/secondmachine -v` | 2 个用例通过：干净 HOME 下装包 → connect → 假服务端收到 `codex` 运行时注册、daemon health `running` 且 runtimes 数与注册一致、`auth status` 复用持久化的根证书；无 CA 时 connect 失败且不写配置 |
| 前端测试 | `pnpm --filter @multica/views exec vitest run runtimes/components/connect-remote-dialog.test.tsx onboarding/steps/cli-install-instructions.test.tsx` | 2 文件 / 5 用例通过 |
| 前端类型检查 | `pnpm --filter @multica/views typecheck` | 通过 |
| Go 格式 | `gofmt -l`（本次改动的 Go 文件） / `git diff --check` | 无输出 |

`go test ./cmd/multica` 与 `go test ./internal/daemon -run TestLoadConfig\|TestConfig_\|...` 的完整运行里，各有 1 个（cmd）和 5 个（daemon）失败用例；已在 `origin/main` 的干净导出上逐个复跑，失败集合完全一致，都是这台机器环境导致的既有失败（Windows chmod 语义、模型 macOS app bundle 路径、路径 lint 断言），与本次改动无关。

真实台式机服务端没有连：连上去会在生产库里新增 runtime 行，属于任务书禁止的「动原有部署和数据库」。端到端用同形状的假 HTTPS 服务端（私有 CA + `register` 接口）验证，符合「回归只用假服务」的约束。

## 没做 / 留给后续

1. **公开下载入口**：本仓库没有已发布的 release（GitHub Actions 在本 fork 不可用），所以 Web/桌面端的「添加电脑」目前仍显示安装命令，没有一个可点的安装包链接。发布授权后需要：把安装包接进 release 资产（`.goreleaser.yml` 增加 installer build/asset）、或由部署把 `u-workers-setup.exe` 放到静态下载路径，再把入口指过去。
2. **安装包签名**：没有代码签名证书，Windows SmartScreen 会提示「未知发布者」。企业内网可接受；要消掉提示需要购买签名证书并在构建脚本里加签，属发布流程。
3. **浏览器信任需要一次点击**：Windows 对「装根证书」强制弹安全警告，没有受支持的静默 API（`deploy/windows-https/import-client-root.ps1` 也是自动化点这个框）。当前实现打印提示并在 3 分钟后超时退出（不影响 CLI/daemon 的信任）；`--skip-browser-trust` 可跳过。
4. **macOS/Linux 客户端**：本任务只要 Windows；connect 与 tlsx 的代码本身跨平台，非 Windows 的浏览器信任步骤是 no-op。
5. **老命令行示例**：文档站 `apps/docs` 五语言正文与内置技能里仍是 `multica …` 示例（批次 C/D 记录为独立收口批次），本次没有全量替换。
6. **安装包版本号**：本地构建默认 `git describe`（当前 `d572cfea7-dirty`）；正式版本应在 tag 上构建，`-Version` 只影响 CLI/安装包版本字符串与「添加/删除程序」显示。

## 边界

- 没有动部署、数据库、`.env`；没有读任何凭据。
- 没有合并、没有推送主干、没有发布 release（等待 Tane 授权）。
- 安装包只写当前用户范围：`%LOCALAPPDATA%\U Workers\bin`、HKCU PATH、HKCU 卸载项；不需要管理员权限。