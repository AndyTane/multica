## CLAWSTA-80 桌面验收任务书（批次 D1）

负责人：前端负责人。执行：DSH。验收方：后端架构师（Task 8 整体）。

### 目标

产出一个可运行、身份正确的 Windows 隔离桌面产物，并在隔离环境里完成：打包版启动 → 读取隔离配置 → 登录隔离部署 → 观察启动/登录/浏览/静置期间的全部外发请求。替换 `e2e/u-workers-brand.spec.ts` 里的 `test.skip` 为真实用例。

基线：`6b213959c`（PR #3 head）。在分支 `agent/agent/079e5e4a0861` 上提交，PR 目标分支为 PR #3 的 `agent/agent/30af6183fb96`，不合并、不推 main。

### 已确定的技术决策（不需要再讨论）

1. **winCodeSign 打包失败**：不改仓库、不开系统开发者模式。用机器本地缓存绕过：在 `%LOCALAPPDATA%\electron-builder\Cache\winCodeSign\` 下，把 electron-builder 下载的 `winCodeSign-2.6.0.7z` 用 electron-builder 自带的 7za 解压为 `winCodeSign-2.6.0` 目录（darwin 下的 `libcrypto.dylib`/`libssl.dylib` 符号链接报错可忽略，Windows 构建不用）。先删除缓存里失败残留的数字目录（`197572275` 等）及其 `.7z`。然后重新跑 `pnpm --filter @multica/desktop package -- --win --dir`，**整条命令必须 exit 0**；rcedit 才会写入版本信息。把该绕过步骤写进 `docs/windows-native.md` 的桌面打包段。
2. **配置路径统一（产品修复，前端已拍板）**：`apps/desktop/src/main/runtime-config-loader.ts` 的 `desktopConfigPath()` 改为 `join(homedir(), ".u-workers", "desktop.json")`（`os.homedir()`），与 `daemon-manager.ts` 的 `desktop_prefs.json`、`daemon-profile.ts` 的 profile 目录同源。原因：Electron 的 `app.getPath("home")` 在 Windows 上走系统 API、忽略 `USERPROFILE`，而 `os.homedir()` 读 `USERPROFILE`；同一个 `~/.u-workers` 目录两种解析会在重定向用户目录时分裂。同步更新 `runtime-config-loader.test.ts`（mock `os.homedir` 而不是 `app.getPath`），并改 `docs/desktop-identity.md` 一句话说明。不新增任何环境变量开关。
3. **隔离方式**：启动打包版时设 `USERPROFILE`/`HOME` 为验收根下的 `home\`，因此 `desktop.json`、`desktop_prefs.json`、`.multica\profiles\u-workers-desktop-*` 全部落在隔离目录。`userData` 由代码固定为 `%APPDATA%\U Workers`（Chromium 不认 `APPDATA` 环境变量）：运行前确认该目录不存在（当前确实不存在），若存在立即停止并报告；运行后删除。真实 `C:\Users\HuaWei\.multica` 和现有部署（15432/8080/3000）一律不读不写。
4. **隔离部署**：沿用 PR #3 的做法另起一套验收部署（`scripts/native-windows.ps1 -Action Build`、新 initdb 集群、随机密钥、`acceptance-root.json` 标记、录制代理 `scripts/e2e/recording-proxy.mjs`），验收根放仓库外，结束后清理。`desktop.json` 写隔离 API/WS/App 地址（`schemaVersion: 1`）。
5. **请求观察（三路叠加，缺一不算通过）**：
   - Chromium 网络：启动参数 `--proxy-server=http://127.0.0.1:<recorder>` 和 `--proxy-bypass-list=<-loopback>`（只让回环直连）；同时用 Playwright 的 `page.on("request")` 与 context 级请求事件记录渲染进程请求。
   - 主进程与子进程：环境变量 `HTTP_PROXY`/`HTTPS_PROXY` 指向录制代理，`NO_PROXY=localhost,127.0.0.1,::1`，`NODE_USE_ENV_PROXY=1`。桌面自动拉起的 daemon（bundled `multica` CLI，autoStart 默认开）也受此约束，算在观察范围内。
   - 系统层兜底：在整个观察窗口里每秒采样一次 `Get-NetTCPConnection`，筛出打包版进程树（按启动 PID 递归子进程）所有远端地址非回环的连接。任何绕过代理的外连都必须在这里出现为 0。
6. **启动方式**：先试 Playwright `_electron.launch({ executablePath: "<win-unpacked>\\U Workers.exe", env, args })`；环境变量必须显式构造（只带系统必需项 + 上面列出的隔离/代理变量），不要整份转发测试进程环境。若仍超时，按 systematic-debugging 先查原因（单实例锁、配置错误页、调试端口），备选为直接 spawn exe 加 `--remote-debugging-port=<随机>` 再 `chromium.connectOverCDP`。不要靠加长超时掩盖。

### 范围内要交付

- 新用例（替换 `test.skip`）：仅在设置了 `E2E_DESKTOP_EXE` 且验收根标记有效时运行，否则 skip 并给出原因。断言：
  - exe 文件版本信息 `ProductName`、`FileDescription` 为 `U Workers`，不含 `Electron`；
  - `app.getName()` 为 `U Workers`，窗口标题含 `U Workers`；
  - 读取到隔离 `desktop.json`，未出现配置错误页；
  - 用隔离账号登录，打开 Issues、Inbox、Agents，静置 60 秒；
  - 三路观察中非回环外发均为 0；任何一条非零都失败并列出目标主机。
- 真实渲染截图：登录页、登录后主界面（亮色），各一张，放 `evidence/clawsta-80/desktop/`。窗口标题栏与任务栏图标需在截图中可见（用整窗截图，不只截 webContents）。
- 证据 JSON：`evidence/clawsta-80/desktop/run-summary.json`，包含命令、exit code、exe 版本信息、三路观察计数、目标主机清单（脱敏后）、启动 PID 树、`%APPDATA%\U Workers` 运行前后存在性。
- 更新 `docs/outbound-data-verification.md` 的 Desktop 一节：写实际结果，删掉「outstanding」里已完成的部分。

### 不在本批

- NSIS 安装、OS 级安装后的开始菜单/图标关联：不做，在文档里保留为未验证。
- 托盘：源码没有实现，维持现有表述。
- 不重开 PR #2 前端已通过事项；不改 Web 端代码。

### 约束

- 只杀自己启动的 PID；禁止按名字结束 `multica`/`multica.exe`（工作区 daemon 同名）。
- 不提交凭据、`deployment.json`、`.env`、数据库文件；只按文件名 `git add`。
- 不引入新依赖。

### 验收命令（全部贴结果）

- `pnpm --filter @multica/desktop package -- --win --dir` → exit 0
- `pnpm --filter @multica/desktop test`、`pnpm --filter @multica/desktop typecheck`
- `pnpm exec playwright test e2e/u-workers-brand.spec.ts --project=chromium --reporter=line`（带 `E2E_DESKTOP_EXE` 与验收根）→ 桌面用例通过，非跳过
- 清理确认：验收根、`%APPDATA%\U Workers`、临时 tag、自己启动的进程均已移除

### 回报

在 CLAWSTA-80 回复并 @ 前端负责人：base..head、提交列表、上面命令结果、截图与 JSON 路径、未完成项及原因。卡住超过一次尝试的步骤，写清实际报错再回报，不要扩大范围自行改方案。
