# 公司电脑使用完整 {{VERSION}} 包

## 下载入口

本包 `download-tools/Update.cmd` 可用于后续升级：读取原安装配置、下载校验，再确认执行。v0.2.5 用户第一次使用时需先取得并校验外置 `UWorkers-Fetch-bootstrap.zip`；旧安装没有该入口。遇到完整包，入口仅下载校验并指向本说明，不调用差量 Apply。原权限保留，正常升级无需重复授予管理员权限。


这次下载 `full.zip`。它包含完整程序和运行依赖，不要求公司旧版文件与家里 v0.2.1 的逐文件 hash 一致。旧版具体报错仍请保留，便于确认差异来源。

本包从 {{VERSION}} tag 对应的干净提交 {{COMMIT}} 重新构建；FULL-FILES.json 记录全部程序、依赖及辅助文件的 SHA-256。请保留完整原包作为以后差量发布的基线，不在安装目录上重新打包。包含 Node、PostgreSQL、Caddy、opencode 和预编译安装器；不含离线源码编译工具链。包内不含任何人的数据库、账号、密钥、配置或附件。

## 给公司电脑的 agent

1. 解压到旧安装目录之外的新文件夹。先读本说明，核对 `FULL-FILES.json` 中全部文件的大小与 SHA-256。不要运行之前差分包的 Apply；不要改旧 manifest 或跳过差分 hash 检查。
2. 找到真正的旧安装根目录：里面应有 `deployment.json`、`data/install-report.json` 和现有业务数据。目录名字中的 v0.2.1 不能代替实际版本判断。只读取版本、Profile、原 HTTPS 地址和端口等必要字段，不读取 `.env`、不输出整份配置。
3. 核对旧 PostgreSQL 的主版本与本包一致，并确认旧数据库可正常启动、备份。完整包的升级流程保留旧 PostgreSQL 数据与已安装的 PostgreSQL 程序，不进行数据库大版本升级。还须将 FULL-FILES.json 中 payload/runtime/pgsql/ 下的文件与现有 runtime/pgsql/ 逐个核验；这些保留文件必须一致才能作为下一版差分基线。若不一致，先反馈具体文件，单独安排 PostgreSQL 运行时升级，不能覆盖数据库目录来修复。
4. 等该安装的任务全部空闲，停止该安装专属的 daemon。不要操作其他 Multica/U Workers 安装。升级前记录原页面、账号、项目、任务和附件样本。
5. 在本机保存原程序文件与 `manifest.json`、`deployment.json` 的回滚副本。范围包括本安装的根目录脚本、`payload/release`、`payload/bin`、`payload/installer`、`payload/scripts`、`payload/source`、Caddy/opencode 和运行中的 Node。保留原 staged releases；不要清理已有 backups。备份留在公司电脑，不上传。

## 使用完整包升级

这是**完整包专用**的 `-Source` 路径，之前的差分 ZIP 不能这样用。

下面命令从旧安装目录运行其原有升级器；执行前公司 agent 应确认该升级器具备数据库备份失败即中止、完整 Source 覆盖和下列参数。若旧脚本不具备这些能力，停止并反馈，不直接覆盖替换脚本。

```powershell
$root = 'D:\03_DevProjects\sandboxes\UWorkers-Portable-v0.2.1' # 改成实际已安装根目录
$full = 'D:\Transfer\UWorkers-Portable-{{VERSION}}-full' # 改成解压后的完整包目录
$profile = (Get-Content -Raw -Encoding UTF8 "$root\data\install-report.json" | ConvertFrom-Json).profile
if (-not $profile) { throw '先确认本安装的 CLI profile' }
$env:PATH = "$env:SystemRoot\System32;" + $env:PATH
$shell = (Get-Process -Id $PID).Path
& $shell -NoProfile -ExecutionPolicy Bypass -File "$root\upgrade.ps1" `
  -Source $full -Profile $profile -NoLanAccess -SkipDaemon -SkipBrowserTrust -NoStartup -NoBrowser
if ($LASTEXITCODE -ne 0) { throw '升级失败，保留日志和备份，先定位，不要重装' }
```

升级器会先备份数据库和上传文件，再更新程序、Stage 和启动。它不要求旧程序与差分基线 hash 一致。这里显式关闭防火墙、证书信任和自启动修改，并暂时只监听本机；不是完成公司局域网验收。

公司 agent 随后须核对升级后的原 PublicOrigin 与端口，沿用本安装原 Caddy 和 HTTPS 配置恢复原有监听，不新增防火墙规则或证书信任。可复用包内 `portable-common.ps1` 的安装专属 Caddy 生命周期，但先核实 PID/路径归属；不能按进程名停止服务。若现有网络策略不允许原地址，反馈管理员处理。

## 验收与失败恢复

- 确认 CLI/API 为 {{VERSION}}、commit `{{COMMIT}}`，`/readyz` 成功，原 HTTPS 地址不变。
- 原账号登录，核对原工作区、项目、任务、附件；核对 `data/upgrade-report.json` 的升级前后数量，并从另一台公司电脑访问验证。
- 保留原账号权限。若需要补授系统管理员，由公司 agent 确认本实例的目标账号后单独处理，不对所有用户授权。
- 验收后再启动原 profile 的 daemon，不重新 connect、不重新 Setup、不新建数据库。
- 用 FULL-FILES.json 核验升级后的全部程序基线：忽略 UPGRADE.md 和 download-tools/ 辅助文件，payload/runtime/node.exe 映射到安装内 runtime/node/node.exe，payload/runtime/pgsql/ 映射到 runtime/pgsql/，其他程序路径原样核对；另核对 runtime/native-windows.ps1 与 payload/scripts/native-windows.ps1 相同。存在差异时保留具体路径与 hash 反馈，不宣布基线已对齐，不私下补拷或修改 manifest。
- 失败时停止本安装相关服务，保留日志、数据库/上传备份和旧程序副本。恢复程序副本和原 deployment 指针；若需要恢复数据库状态，恢复到新数据库后核对，不覆盖/删除旧库或附件。不要直接重复执行 Setup。

公司 agent 请回传：失败原文或成功的版本/readyz、业务数据及局域网访问结果。不要回传凭据或完整配置。

## 下载工具

`download-tools/` 内附 Fetch、Apply、共享模块、官方 ossutil 2.4.1 和使用说明，供后续下载使用。本次完整包已经带齐程序，不需要它再次拉取差分包。更新后的 Fetch 能识别完整包并下载、校验和解压。运行 `download-tools/Fetch-Upgrade.ps1 -Version latest -Ossutil ./download-tools/ossutil.exe`，不要附加差分专用的 -Apply 或 -CheckOnly；升级按本说明执行。旧版 Fetch 不认识完整包清单，请先从 OSS 控制台下载 full.zip 获取新版工具。
