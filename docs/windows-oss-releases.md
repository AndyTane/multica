# Windows OSS 发布与升级

## 升级入口（v0.2.6 起）

v0.2.5 用户首次使用新版入口，先取得随本次交付的 `UWorkers-Fetch-bootstrap.zip`，核对交付的 SHA-256，解压到安装目录之外。旧安装和 v0.2.3 下载工具中没有 `Update.cmd`。将已有 `ossutil.exe`（2.x）放到同目录，或按提示选择其路径；凭据仍仅使用原环境变量。

双击 `Update.cmd`，选原安装目录。它读取原地址、profile 和监听范围，下载、校验并只读检查；差量起点不匹配会停止，完整包会转到包内说明。任务空闲后用原安装 `Stop.cmd` 停止，再运行入口并输入 Y 升级。取消不启停服务、不修改安装。成功后检查登录、项目、任务、旧附件与另一台电脑访问，再启动原守护进程。失败保留显示的下载和备份目录，按 `UPGRADE.md` 恢复。

后续新服务端包根目录与完整包 `download-tools` 都携带同版本入口；差量计划包含对应文件和 SHA-256，保留旧 full/delta/client 清单兼容。客户端换新版：新包 `Import.cmd` 选旧包，任务空闲后旧包 `Stop.cmd`、新包 `Reconnect.cmd`；首次使用 `Connect.cmd` 按中文提示填写，无需编辑 JSON。

以下底层命令继续供维护人员使用。正式发布仍由负责人升新版本、从最新 main 打 tag、完成服务端包与 Client-Kit 同号上传和校验，最后切换 stable。


家里使用 PowerShell 7 构建/发布，公司使用 Windows PowerShell 5.1 或 PowerShell 7 拉取/升级。沿用现有 portable 安装与 119 的清单式差分流程。

## 首次准备

发布使用上海 cn-shanghai 的 tyx-swrelease 私有桶，阻止公共访问。tyxlife-oss 与发布无关，不使用。家里和公司使用独立 RAM 用户：swrelease-publisher 负责上传，swrelease-reader 负责公司下载。工具只需要写 releases/* 与 latest，以及下载端的 GetObject；Tane 已决定本任务不再调整账号现有权限。发布者无需 GetObject、ListObjects 或删除权限。版本对象由上传请求 `--forbid-overwrite=true` 保证不覆盖；latest 最后更新。已有 40GB 标准 LRS 资源包抵扣存储，公网下载另计。

控制台配置位置及最小权限 JSON 见 CLAWSTA-121 首批操作清单。两端由用户本人通过 Windows“编辑帐户的环境变量”填写：

| 变量 | 值 |
| --- | --- |
| `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` | 各端自己的 RAM AccessKey |
| `OSS_REGION` | `cn-shanghai` |
| `OSS_ENDPOINT` | `https://oss-cn-shanghai.aliyuncs.com`，仅官方外网 HTTPS |
| `UWORKERS_OSS_BUCKET` | `tyx-swrelease` |

保存后新开终端。不要把密钥贴进命令、仓库或工单；无需运行 `ossutil config`。脚本不读 `.env`。现有后台 agent 不一定继承新变量，升级从独立终端执行，不重启工作区 daemon 来刷新变量。

用户显式下载[官方 ossutil 2](https://help.aliyun.com/zh/oss/developer-reference/ossutil-overview/)，按该页 SHA-256 核验，再解压到自选工具目录。已核对 Windows amd64 2.4.1 ZIP：

```text
https://gosspublic.alicdn.com/ossutil/v2/2.4.1/ossutil-2.4.1-windows-amd64.zip
SHA256 06ebec1680ce68b1807393e65a1e3f5f99c3b69c9b47c40578a80187da4462c3
```

```powershell
Get-FileHash .\ossutil-2.4.1-windows-amd64.zip -Algorithm SHA256
# 人工确认与官网一致后才解压和运行
Expand-Archive .\ossutil-2.4.1-windows-amd64.zip C:\Tools\ossutil
```

脚本不会安装工具、后台检查更新、写入工具凭据配置或自动下载运行时。上传使用[PutObject](https://help.aliyun.com/zh/oss/developer-reference/put-object)，传输使用 ossutil 默认 CRC64 校验；包身份另外使用 SHA-256，ETag 不作为 SHA-256。

## 公司首次取得 Fetch

家里或维护端执行：

```powershell
.\deploy\portable\build-fetch-bootstrap.ps1 -OutputDirectory C:\Transfer\fetch-r1
```

将输出的 `UWorkers-Fetch-bootstrap.zip` 通过工单附件或公司允许的文件传输方式交给公司；将 ZIP SHA-256 一并交付。公司核验外层 ZIP 后解压，内含 Fetch、Apply、共享模块、说明和内部 `CHECKSUMS.sha256`，无需访问 GitHub。用户另外从官网获取并核验 ossutil；工具不放进仓库或此脚本包。

## 版本与完整包基线

代码变化或升级都必须升版本。合并已验收修复后，在 main 的同一提交上打新 tag；release.json 记录 tag 对应的完整 commit。禁止旧版本号重编、替换已上传对象或手工同步程序文件。

v0.2.3 用完整包重新对齐公司基线。维护端从干净且已打 tag 的源码调用：

```powershell
.\deploy\portable\publish-release.ps1 -Full -Version v0.2.3 -SourceDirectory C:\Source\UWorkers-v023 -OutputDirectory C:\Releases\v023 -Ossutil C:\Tools\ossutil\ossutil.exe -BuildParameters @{PostgresDirectory='C:\Runtime\pgsql'; CaddyExe='C:\Tools\caddy.exe'; OpenCodeExe='C:\Tools\opencode.exe'}
```

完整包会从源码重建，压缩为 `releases/v0.2.3/full.zip`，内含完整程序、依赖、UPGRADE.md、FULL-FILES.json 及 download-tools（含 ossutil）。上传前重新解压逐文件核验；上传后按成功响应校验，最后更新 latest。发布者不做远端读回。

公司首次从 OSS 控制台下载 full.zip，解压到安装目录之外的新目录，按包内 UPGRADE.md 备份并升级。后续可用包内新版 Fetch 拉取 latest。schemaVersion=2/packageType=full 不进入差分 Apply/CheckOnly；旧版 Fetch 会拒绝新清单，须从完整包取得新版工具。

保留这份原始完整包作为下一版差分的 BaselineDirectory，不能从已运行的安装目录制作基线。公司升级后按包内说明确认版本、服务就绪、原业务数据和局域网访问。

## 家里差量发布

每次使用一个新的输出目录。正常发布从已检出到指定 tag/commit 的干净源码构建，显式传入原 portable **未安装整包**作差分基线：

```powershell
& .\deploy\portable\publish-release.ps1 -Version v0.2.5 -SourceRef v0.2.5 -SourceDirectory C:\Source\UWorkers-v025 -BaselineDirectory C:\Packages\UWorkers-Portable-v0.2.3 -OutputDirectory C:\Releases\v025 -Ossutil C:\Tools\ossutil\ossutil.exe -BuildParameters @{PostgresDirectory='C:\Runtime\pgsql'; CaddyExe='C:\Tools\caddy.exe'; OpenCodeExe='C:\Tools\opencode.exe'}
```

在 PowerShell 7 终端内进入仓库并执行。构建参数沿用 `build-package.ps1` 的工具路径；不允许 `ReusePayloadFrom`、Force 或版本覆盖。构建脚本不负责切换源码分支。Node/PostgreSQL 依赖改变需要单独的运行时升级，当前差分入口会拒绝。

119 既有实包首发复用方式（不会重新编译）：

```powershell
& .\deploy\portable\publish-release.ps1 -Version v0.2.2 -UpgradeDirectory D:\UWorkers\packages\UWorkers-Portable-v0.2.2-upgrade -OutputDirectory C:\Releases\v022-prepared -PrepareOnly
# 凭据就绪且负责人收到 Tane 确认后，在新输出目录执行同样命令并去掉 -PrepareOnly，增加 -Ossutil 工具路径。
```

`-PrepareOnly` 无需凭据，不执行 OSS 请求，完整生成 ZIP、版本清单和 latest。只打包 write 清单与经过审阅的升级辅助文件；部署配置、用户数据、未知额外文件不被递归打包。写清单中的敏感路径或链接会直接拒绝。

固定布局：

```text
releases/<version>/upgrade.zip
releases/<version>/UPGRADE.md
releases/<version>/client-kit.zip
releases/<version>/release.json
channels/stable/latest.json
```

单个对象最多 5 GiB。先上传 ZIP 和说明，成功后上传版本清单，最后写 latest。`publish-result.json` 记录成功对象，不记录凭据。发生部分失败时保留输出目录；不要覆盖/删除已有版本对象。检查失败原因后以新版本重新构建发布。若只有 latest 更新失败，先由维护者确认该版本所有对象已完整存在，再安排显式恢复指针；脚本不会通过扩大发布者权限或覆盖版本对象自动续发。

## 公司拉取与差量升级

先只下载、校验，不碰安装：

```powershell
.\Fetch-Upgrade.ps1 -Version latest -DestinationDirectory C:\Transfer\v022-check -Ossutil C:\Tools\ossutil\ossutil.exe
# 固定版本：将 latest 换为 v0.2.2。
```

以下 -CheckOnly/-Apply 仅适用于差分清单；完整包按 UPGRADE.md 操作。明确填写现有安装的 Root、Profile 和 HTTPS 地址，先检查：

```powershell
.\Fetch-Upgrade.ps1 -Version v0.2.2 -DestinationDirectory C:\Transfer\v022-preflight -Ossutil C:\Tools\ossutil\ossutil.exe -CheckOnly -Root C:\UWorkers -Profile office -PublicOrigin https://192.168.1.100:13443
```

任务空闲后，从独立终端停止**本安装**的 daemon。升级命令：

```powershell
.\Fetch-Upgrade.ps1 -Version v0.2.2 -DestinationDirectory C:\Transfer\v022-apply -Ossutil C:\Tools\ossutil\ossutil.exe -Apply -Root C:\UWorkers -Profile office -PublicOrigin https://192.168.1.100:13443
```

暂存目录必须全新且在安装之外。下载中断、清单/hash/基线错误都不会执行升级；修正原因后换新暂存目录重试。未知基线不能强制跳过，改取匹配该 from/commit 的差分或按整包升级流程处理。仅 119 的指定 v0.2.1→v0.2.2 包保留对 `775029717` 的已知基线例外，未变化依赖仍逐文件校验。

Apply 验证全部文件后检查安装 daemon，运行时拒绝升级；不会按进程名停服务。它备份数据库/上传、记录文件日志、替换程序，再调用原 upgrade（不传 Source）。稀疏包绝不能用于 `/MIR`；新版 `upgrade.ps1` 会拒绝带 UPGRADE-FILES 的 Source。公司旧 upgrade 尚无此保护，也应只执行 Fetch/Apply。

失败时保留 `backups/upgrade-<version>-<timestamp>/files.json`、原 deployment.json、数据库/上传备份及旧 releases。按包内 UPGRADE.md 回滚。现有管理员身份保持不变；准确账号确认后再单独 grant-admin，不能给所有用户提权。成功后按 UPGRADE.md 验证版本、就绪、登录、原数据/附件和公司局域网，再启动原安装 daemon。

## Client-Kit（v0.2.5 起）

源码构建发布同时产出 `client-kit.zip`。包内 CLI 与服务端便携包来自同一次构建，`CLIENT-FILES.json` 记录同一版本与完整提交；`release.json` 的 `clientKit` 记录压缩包大小、SHA-256 和路径。发布顺序为服务端包、说明、客户端包、版本清单、latest，任一步失败都停止。客户端包使用空白连接配置，不复制机器的证书或登录。

公司其他电脑可在 OSS 控制台下载 `releases/v0.2.5/client-kit.zip` 并校验清单中的 SHA-256。取得本次新版 Fetch 和共享模块后，也可只下载客户端：

```powershell
.\Fetch-Upgrade.ps1 -ClientKit -Version latest -DestinationDirectory C:\Transfer\client-v025 -Ossutil C:\Tools\ossutil\ossutil.exe
```

`-ClientKit` 只下载并验证，不执行服务端升级或客户端连接；不能与 `-Apply` / `-CheckOnly` 同用。v0.2.3 内的旧 Fetch 没有此参数，可从本次工单提供的 Fetch bootstrap 取得新版。客户端首次连接和双击重连见 `deploy/client-kit/README.md`。已登录的电脑沿用原 profile，机器重启后双击 `Reconnect.cmd`；开机自动连接默认关闭，需要时由用户放置 Windows 启动快捷方式。

## 验证边界

自动化测试使用 fake OSS 验证传输协议和失败路径，使用隔离的 CLI 替身验证客户端生命周期；这些结果不代表真实发布或公司升级已完成。真实上传、压缩包核验和公司实机升级结果按各版本工单分别记录。发布使用已确认的桶和凭据，公司升级从独立终端执行。
