# CLAWSTA-105 命令契约与交接（105-A）

## 目标与决定

补齐新手引导、连接电脑及同类用户可见 CLI 文案，合并后由后端架构师更新移动专用机 `localhost:13000` 的 U Workers 实例。当前 main / 部署基线为 `d572cfea7`。

- 沿用 CLAWSTA-97 / 99 已通过的命名：命令 `u-workers`，产品 `U Workers`，环境变量 `U_WORKERS_*`。
- 既有安装脚本和发布配置已经提供双可执行名；旧 `multica` 命令保留一个 major 版本。兼容目录、包名、协议头、内部 import 不做机械替换。
- 本批修正本地 CLI 构建输出为 `server/bin/u-workers.exe`。仓库远端没有 tag，脚本新增与 native 构建相同的显式 `-Version vX.Y.Z` 入口，值须与自托管版本一致。本机部署版本是 `v0.2.0`；不可假称是新版发布或为越过功能门槛抬高版本。
- 引导给出的下一条命令必须能找到这个可执行文件；仅设置 `U_WORKERS_CLI_PATH` 不会让 shell 识别 `u-workers`。请给出正确的当前终端 PATH / 显式调用方式，提示本地源码构建前置条件；保留无静默下载、无官方云回退的约束。
- CLAWSTA-104 正在独立做第二台机器安装包，本批不发布安装包、不修改其证书方案或下载交付，不采用其尚未合并的 `uworkers` 别名。

## 前端负责人负责实现与专业验收

以前端自己的判断维护具体 UI 实施安排，以下是本批命令契约和已定位入口：

1. `packages/views/common/cli-install-command.tsx`、`onboarding/steps/cli-install-instructions.tsx`：平台命令、显示与复制值一致，正确衔接安装/构建和 setup。
2. `packages/views/runtimes/components/connect-remote-dialog.tsx`：setup、token、config、daemon start/status/logs 所有入口。
3. `packages/views/onboarding/templates/install-runtime-issue.ts`：所有语言生成模板的 daemon restart 命令。
4. `packages/views/settings/components/tokens-tab.tsx`、`packages/views/locales/*`、Desktop 的 daemon settings：显示、复制、相关提示均采用新命令名。
5. 扫描其他实际可见位置的同类遗漏；保留兼容说明、真实旧路径、内部标识。不要扩大到重命名模块或重写历史记录。

验收：已有相关组件测试更新且通过；复制内容与显示一致；所有支持语言一起覆盖；对连接页和平台切换做真实渲染验证（不得改用户 onboarding 状态作为测试）。执行改动范围内 typecheck / lint，记录实际结果。

交付：在当前任务分支继续，集中一个 PR，标题含 CLAWSTA-105；给出 base..head、改动/检查结果、截图证据、未关闭问题。回复 CLAWSTA-105 并 @ 后端架构师。由后端架构师复核命令可执行性，合并并部署；前端无需操作本机服务。

## 后端后续接续

前端回交后检查新增差异、相关复制命令和构建；确认 PR 合并后，使用 `D:\UWorkers\local\native\runtime\native-windows.ps1` 的生命周期更新现有实例并保留数据库/账号。服务由计划任务 `UWorkers-Local` 托管，日志在该部署的 `logs` 目录。核验页面新命令及 API 就绪，报告实际提交。整个问题完成才设 `in_review`。
