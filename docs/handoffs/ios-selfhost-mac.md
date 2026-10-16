# Mac Codex handoff: Multica iPhone 客户端

## 目标与交付入口

把本仓库现有 Expo / React Native 客户端编译成独立运行的 iOS Release 版，
签名并安装到用户自己的 iPhone，连接用户已在 Windows 上部署的 Multica。
用户已授权编译、安装和必要的依赖准备。继续完成工作，遇到 Apple ID 登录、
手机信任或开发者模式等必须由用户完成的操作再请用户介入。

- 仓库：<https://github.com/AndyTane/multica>
- 接手分支：`codex/ios-selfhost-handoff`
- 基础分支及提交：`tane/main`，`76fd1003f2f11818d8663584d793666086c2c64b`
- 客户端目录：`apps/mobile/`
- 本文：`docs/handoffs/ios-selfhost-mac.md`
- 构建入口：`apps/mobile/scripts/install-selfhost.command`
- 自建配置模板：`apps/mobile/selfhost-windows.env.example`

当前云端执行环境是 Linux。已完成代码检查、iOS 工程生成和 Hermes JavaScript
导出；尚未执行 Xcode 原生编译、苹果签名、iPhone 安装或真机界面验证，尚无已签名 IPA。
这个分支本身就是源码交付，GitHub 的 Download ZIP 包含全部源码、模板和本文；
依赖、生成的 `ios/`、`node_modules` 和个人 `.env.production.local` 在 Mac 上生成。

先读根目录 `AGENTS.md`、`apps/mobile/AGENTS.md` 和 `apps/mobile/README.md`。

## 拉取源码

在新目录克隆，避免覆盖已有工作的修改：

```bash
git clone --single-branch -b codex/ios-selfhost-handoff https://github.com/AndyTane/multica.git multica-ios
cd multica-ios
git status --short --branch
```

如 Mac 上已有此仓库，先检查工作区，再 fetch 并切到这个分支；有未提交工作时使用
独立 worktree，不要 reset 或覆盖它。不要只拉 `tane/main`，本次 iOS 配置和构建脚本
在交付分支上。

## 已运行的 Windows 服务

以下是 2026-10-16 通过 Desktop Commander MCP 设备 "移动专用机" 查询并配置的结果：

| 项目 | 值 |
| --- | --- |
| MCP 设备 ID | `011dfbc5-e20b-497c-927d-176ed7434dd6` |
| Windows Wi-Fi 网卡 | `WLAN` |
| Wi-Fi IPv4 / 网段 | `192.168.3.162` / `192.168.3.0/24` |
| 手机使用的 API | `http://192.168.3.162:8080` |
| 网页地址 | `http://192.168.3.162:3000` |
| 原 API / 网页监听 | `127.0.0.1:8080` / `127.0.0.1:3000` |
| 运行数据根目录 | `D:\MulticaData\native` |
| Windows 源码目录 | `D:\01_Projects\multica-fork` |
| 运行中的服务发布目录 | `D:\MulticaData\native\releases\v0.6.1-46-g89727acb4-20261015123138` |

已在 `D:\MulticaData\native\mobile-access` 部署独立 Node TCP 转发：

- `lan-proxy.mjs`：对应本分支 `scripts/lan-proxy.mjs`。
- `config.json`：`{ "interface": "WLAN", "ports": [3000, 8080] }`。
- 只绑定 Wi-Fi IPv4，只接受同一 IPv4 网段客户端，转发到对应的本机端口。
- 支持 HTTP 流式响应与 WebSocket，Wi-Fi 地址变化后每 10 秒刷新监听。
- `start.vbs` 检查重复进程并隐藏启动；用户 Startup 目录已有 `Multica Mobile Access.vbs`。
- 现有 Windows Node 入站规则允许该程序，未修改防火墙规则、原部署监听或数据库。
- 配置时原 API PID `796`、网页 PID `42400` 保持运行。PID 只用于当时核对，后续不要据此盲目停止进程。

本机环回及 Wi-Fi 地址的 HTTP 检查均为 200；API 的真实 WebSocket 握手也通过。
检查只从 Windows 发起，实际手机网络、防火墙入站和 Wi-Fi 客户端隔离仍要从 Mac / iPhone 验证。
网页目前验证到页面响应；远端网页登录、CSRF 和浏览器 WebSocket 配置没有验收。
iOS 客户端直接访问 `8080`，使用已有 bearer-token 鉴权。

Windows 服务应保持开机，Mac 和 iPhone 应能路由到此局域网。
若 IP 已变化，查询当前 `WLAN` 地址，更新手机构建配置。不要把地址改成手机上的 `localhost`。
转发会跟随 Windows 地址变化，但已安装的 Release 客户端内置地址仍需更新后重新编译。

## Mac 上的接手步骤

### 1. 检查构建条件

```bash
sw_vers
uname -m
node --version
npm --version
xcodebuild -version
xcrun --sdk iphoneos --show-sdk-path
```

需要完整 Xcode 及 iPhone SDK、Node.js 22 或以上（推荐 24 LTS）。单独安装 Command Line
Tools 不足以编译 iOS App。首次打开 Xcode 应完成组件安装和许可；在 Xcode Settings →
Accounts 登录用户的 Apple ID。凭据和签名材料保留在 Mac 上。

源码采用 Expo SDK 57、React Native 0.86，启用 scene lifecycle，并从源码编译 React Native。
按仓库锁定版本安装；若原生编译提示 Xcode / SDK 版本不满足，升级相应 Apple 工具，
不要通过降级 Expo 或丢弃 scene 配置掩盖问题。

### 2. 创建并检查本机配置

已有 `.env.production.local` 时先检查相关 URL、bundle ID 和 team 设置，保留用户自己的值。
新克隆可运行：

```bash
test -f apps/mobile/.env.production.local || cp apps/mobile/selfhost-windows.env.example apps/mobile/.env.production.local
```

模板中的值是：

```dotenv
EXPO_PUBLIC_API_URL=http://192.168.3.162:8080
EXPO_PUBLIC_WEB_URL=http://192.168.3.162:3000
EXPO_BUNDLE_IDENTIFIER_PROD=com.andytane.multica.personal
```

API URL 是源站地址，不加 `/api`。使用自有 bundle ID，避免尝试用官方
`ai.multica.mobile` 签名。若 Apple 报 bundle ID 已被其他 team 注册，换一个用户可用的唯一值。
用户有多个 team 时，在这个本地配置里指定 `EXPO_APPLE_TEAM_ID`，继续使用现有 config 的机制。

从 Mac 检查实际连接：

```bash
curl --fail --connect-timeout 5 --max-time 10 http://192.168.3.162:8080/readyz
```

若失败，先检查当前 IP、双方网络和无线客户端隔离，再通过 MCP 检查转发进程。
云端曾从 Windows 自测成功不能替代这一步。故障排查不要重建、迁移或覆盖已有数据库。

### 3. 编译并安装 Release 版

用 USB 连接并解锁 iPhone，信任 Mac，启用 iPhone 的开发者模式。
从仓库根目录执行：

```bash
bash apps/mobile/scripts/install-selfhost.command
```

脚本检查 Node / Xcode，缺少本地配置时复制自建模板，然后使用精确版本
`pnpm@10.28.2` 安装移动端及其 workspace 依赖，最后调用既有
`pnpm ios:mobile:device:prod:release`。
它经过 `apps/mobile/scripts/ios-run.sh`，先 Expo prebuild 再 run:ios，
确保包名、图标、权限说明和 HTTP 策略重新应用。wrapper 明确传 `--no-clean`，
因为 Expo SDK 57 默认清理原生目录；此参数保留 CocoaPods 缓存，方便后续重新签名。
不要直接绕过 wrapper 使用旧的生成工程。

等价的依赖安装和构建命令：

```bash
npm exec --yes --package=pnpm@10.28.2 -- pnpm --filter @multica/mobile... install --frozen-lockfile
npm exec --yes --package=pnpm@10.28.2 -- pnpm ios:mobile:device:prod:release
```

保留根目录注册的 `apps/mobile/patches/expo-modules-core@57.0.21.patch`；它修复 scene
窗口下原生图片 / 文档选择器的 presenter。不要使用会忽略根 `pnpm.patchedDependencies`
的 pnpm 11 安装。首次构建下载 CocoaPods 及编译 RN 会较慢。

生成的 `ios/` 在 Mac 本机维护，不提交。正常调试不要反复 clean prebuild。
若实际原生构建失败，修复后记录确切的 Xcode、SDK、CocoaPods 和签名错误及处理。
仅在确实需要时请用户完成本机 Apple 操作。

免费 Apple ID 可用于个人真机签名，通常有效 7 天；到期重新签名安装。
本次目标是可独立启动的设备 Release 版，无需先发布 App Store 或 TestFlight。

### 4. 真机验收并记录结果

在 iPhone 上允许 Multica 使用本地网络。使用原 Multica 账号登录并验证：

1. 显示既有工作区、任务和聊天历史，连接的是 Windows 自建数据。
2. WebSocket 数据刷新和后台 / 前台重连正常，记录真实连接结果。
3. 拔掉 USB，并退出 Mac 上 Metro 后冷启动，应用仍能连接保持开机的 Windows 服务。
4. 若测试图片或文档附件，确认选择器和上传均可完成。

遵守仓库的真实智能体测试门禁，不把启动真实 agent 运行作为默认冒烟验证。
最终向用户说明实际安装结果、bundle ID / team、签名有效期、网络条件和任何未完成验证。
只有 JS 导出、类型检查或 prebuild 成功时，不要宣称 IPA 或真机安装完成。

## 这次代码改动与已完成检查

- `apps/mobile/app.config.ts`：为 HTTP API 的确切主机添加 ATS 例外及本地网络用途说明，
  保持 `NSAllowsArbitraryLoads=false`；HTTPS 构建不添加 HTTP 例外。
- `apps/mobile/app.config.test.ts`：覆盖 HTTP 配置和 HTTPS 默认行为。
- `apps/mobile/scripts/install-selfhost.command`：Mac 真机 Release 安装入口。
- `apps/mobile/scripts/ios-run.sh` 及测试：显式保留原生目录的 prebuild 参数。
- `apps/mobile/selfhost-windows.env.example`：可从 Git 下载的自建配置模板。
- `scripts/lan-proxy.mjs` 与测试：Windows 局域网 TCP 转发的源码及运维说明。

云端已完成：

| 检查 | 结果 |
| --- | --- |
| 移动端 typecheck | 通过 |
| 移动端 lint | 通过 |
| 移动端 Vitest | 36 个文件、245 项测试通过 |
| `scripts/ios-run.test.sh` | 通过 |
| `node --test scripts/lan-proxy.test.mjs` | 4 项测试通过，覆盖 HTTP、WebSocket、网段限制、TCP 半关闭 |
| 本地自建配置下 Expo iOS prebuild | 通过，并核对生成 Info.plist 的主机例外与权限说明 |
| `expo export --platform ios` | 通过，2650 个模块，Hermes 输出包含正确的自建 API / 网页地址 |
| 安装脚本 Bash 语法、精确 pnpm 子进程版本 | 通过 |
| Windows 环回 / Wi-Fi HTTP，API WebSocket 握手 | 通过，原部署继续运行 |

Xcode 原生编译、代码签名、安装、登录和真机界面 / 网络验收由 Mac Codex 接着完成。
