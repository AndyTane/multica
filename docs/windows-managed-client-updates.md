# Windows 客户端自动升级

受管 Client-Kit 跟随**实际安装的服务端版本**。先将带此能力的新包解压到独立目录，用 Import.cmd 导入旧连接设置，等任务空闲后用旧包 Stop.cmd，再用新包 Reconnect.cmd。此后服务端升级成功、同版客户端缓存就绪，在线电脑在任务结束后自动升级；离线电脑下次连接时补升。客户端只访问原来已信任的 HTTPS 地址，不需要 OSS 凭据、GitHub 或额外端口。

## 操作与状态

Status.cmd 显示等待空闲、下载校验、切换重启、等待服务器确认、成功或失败/已回退。新进程完成原服务器的鉴权心跳并确认目标版本后才算成功。现有任务继续完成，切换期间暂停新任务领取。

```powershell
.\client.ps1 -Action PauseUpdate
.\client.ps1 -Action ResumeUpdate
```

暂停不会停止当前任务。损坏包或启动失败保留旧版，针对同一个包最多尝试三次，失败后分别等待 5、10、15 分钟；达到三次后保留诊断，管理员修复发布包或手动换新 Kit。网络中断时，已启动的新进程等待服务器确认，不会把本机在线当成升级成功。电脑异常关闭后，用原入口 Reconnect.cmd 恢复未完成的切换。

每套独立安装使用独立 profile。`kit-install.json` 是该安装的非秘密身份及暂停设置；`current.json` 指向整组有效程序，`programs/` 中每版包含同版脚本、清单和两个相同 CLI 别名。根目录原始程序作为稳定恢复入口保留。连接地址与公开证书留在原 Kit 数据目录，账号、profile 和模型配置继续使用原 Windows 用户存储；发布包不会覆盖它们。不要删除 `.managed/` 更新记录及上一版程序，恢复失败时保留 `helper.log` 交管理员检查。

## 服务端缓存与发布

Update.ps1 在显式升级时，复用 reader 下载同目标版本的 client-kit.zip 和 release.json；Apply 在停止服务前校验并准备安装目录 `data/client-releases/<version>/`。最终 HTTPS readyz、文件及版本检查通过后才写 ready.json。API 运行时只读缓存，不要求 OSS 环境变量或写权限。服务端回滚后，版本/提交不匹配的缓存不会继续下发。

新安装可先 Fetch-Upgrade.ps1 -ClientKit 下载所选版本，然后传 `install.ps1 -ClientReleaseDirectory <包含 release.json 和 client-kit.zip 的下载目录>`。直接 Apply 和完整包 upgrade.ps1 也接受这个参数；不提供缓存时，服务和原客户端仍可使用，自动升级标记为尚未就绪。缓存准备失败会阻止此次带缓存升级，不会跳过校验。

候选版本只用于隔离验证。本功能须负责人验收并走新的正式版本发布后才供公司使用，不能将候选程序手动覆盖正式安装。

## 契约与维护

- HTTP/WS 心跳增加可选 `managed_client_update`：version、commit、platform、sha256、bytes。旧客户端忽略；不改变手动 UpdateStore 的旧状态枚举或短超时。
- `GET /api/daemon/client-package?runtime_id=...&sha256=...` 使用现有 daemon/PAT 鉴权与 workspace/runtime 校验。路径和文件由服务端固定选择，拒绝转发外部 URL。客户端禁止下载重定向并检查长度、哈希、Windows amd64、版本、ZIP 固定文件白名单及清单；禁止自动降级或覆盖开发版本。
- 本机 daemon status/health 增加可选 managed_client_update 诊断（安装 ID、阶段、目标、原因、次数、暂停），不返回命令参数、凭据或恢复日志。没有新增 Web 页面和 Web 状态枚举；以上增量供接口专项评审。
- 这是受管 Windows Kit 的明确授权范围。通用 CLI、Desktop、模型 CLI 不进入此流程，不变更系统信任、防火墙或自启动。

定向检查：`go test ./internal/managedkit`、daemon 的 TestManagedUpdate/AutoUpdate/SelfReload/HandleUpdate/ReportUpdate、隔离数据库下的 TestManagedClientPackageAuthorization，以及 PS5.1/7 的 client/settings/entry/client-cache 回归。真实验证使用独立 API、PostgreSQL、HTTPS、两个 profile 和可控本地协议进程，不调用付费模型。
