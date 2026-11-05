# Windows 客户端：连接与执行引擎

客户端启动、登录及更新确认与引擎可用性分别判断。缺程序、缺配置、选定文件丢失／不可读／格式错误，或用户跳过 OpenCode 时，客户端仍可真实鉴权连接，显示“已连接；待配置，暂不能接任务”。这时没有可用执行运行时，不领取任务，仍接收后续受管更新。

Status.cmd 与 `u-workers daemon status --output json` 提供同一状态。新增 `connection`（state、verified_at）和 `execution`（state、message、runtime_count、diagnostics）字段；原 status 仍为 starting/running/stopped，running 表示客户端启动完成。diagnostics 只包含路径、是否存在及修复步骤，不包含配置内容或凭据。网络／认证未通过会单独反馈，不能用本机进程存活替代服务器验证。

## 补齐后重新检测

先等任务空闲，Stop.cmd 停止，再运行：

```powershell
.\client.ps1 -Action Reconnect -OpenCodePath 'D:\Tools\OpenCode\opencode.exe' -OpenCodeConfig 'D:\Models\opencode.jsonc'
```

这一步明确解除之前的跳过选择。原 Kit/profile、登录、证书及外部配置文件继续沿用。JSON/JSONC 检查与版本探测不调用模型；就绪表示可以调度，真实模型访问由任务执行反馈。

OpenCode 完整路径以保存的选择、OPENCODE_CONFIG、OPENCODE_CONFIG_DIR、XDG_CONFIG_HOME 和当前 Windows 用户计算，保持 #42 的有界搜索、多候选选择及持久化。原选择无效时不静默改用其他文件。DeepSeek Harness 是另一种选择：

- CLI：DSH_HOME（默认当前用户 `.dsh`）下 `profiles/multica/package.json` 是接入 profile 清单，**不是模型配置**。在 DSH 自身设置中配置服务商、模型和登录，按接入说明安装 multica profile，不生成空清单。
- Web bridge：MULTICA_DSH_WEB_CONFIG 或 `brand.ConfigRoot` 下 `dsh-web.json`。`scripts/configure-dsh-web.ps1` 写入 origin/accessLog、探测已有网页宿主并绑定运行环境。Go bridge launcher 与配置脚本使用同一根目录规则。模型配置仍由 DSH 管理。

## 服务端与旧版本

`POST /api/daemon/client-heartbeat` 使用现有 DaemonAuth 与工作区成员授权，回执绑定 workspace_id/daemon_id。它不创建执行运行时，仅验证连接并传回已激活的受管包。下载接口增加同一工作区授权路径，保留已有 runtime_id 路径。真实程序、Kit/profile、安装标识、版本、目标服务器、包 SHA 与 helper 的鉴权确认规则不变；空闲／暂停和真正失败时回退的保护继续生效。

启动时，已确认不存在或缺配置的内置引擎，仅将同工作区、同 daemon、同 token owner 的旧行置为 offline，保留记录和任务；不触碰其他电脑、其他用户或自定义 profile。自定义执行环境独立判断，一个可用引擎即可恢复接任务。

旧客户端继续使用运行时心跳；HTTP 与 WebSocket 均传升级信息。新客户端连接旧服务端时可通过工作区读取验证连接并保持待配置，但旧服务端没有独立控制心跳，零引擎的升级确认及后续更新要求先升级服务端。404 的工作区拒绝不会当成旧服务端兼容；认证失败也不会生成更新确认。

本轮只交候选。正式合并、升号、发布及台式机／移动机实际落地验收由 R6.2 串行执行。
