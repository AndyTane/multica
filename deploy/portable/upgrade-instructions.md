# U Workers 升级

双击新版下载工具中的 **Update.cmd**，选择原安装目录。工具读取原服务地址、profile 和访问范围，下载并校验后显示“当前版本 → 目标版本”。等任务空闲，用原安装的 Stop.cmd 停止服务，再运行 Update.cmd，检查通过后输入 Y 升级。回车取消不会停止服务或改动安装文件。

v0.2.5 安装中没有 Update.cmd。第一次升级先取得本次提供的 **UWorkers-Fetch-bootstrap.zip**，按提供的 SHA-256 校验，解压到安装目录之外；将已有的 ossutil 2 放在工具同目录（或按提示选择路径）。OSS 只读凭据继续使用当前 Windows 用户环境变量，不要填入文件。以后新版包自带入口。工具不会后台检查更新或安装依赖。

下载目录显示在窗口中，包含 release.json、升级包和说明。差量包只能从指定起点升级；起点不符时按提示逐版升级。完整包请按解压后的 UPGRADE.md 操作，不能套用差量 Apply。

## 升级后

“程序升级完成，服务已就绪”表示程序文件和 `/readyz` 检查通过。原有权限保留，无需重复授予管理员权限。请检查：原账号登录、项目和任务样本、旧附件、另一台电脑访问。全部通过后再启动原安装的守护进程，保留升级包和备份至验收完成。

## 失败时

保留窗口中显示的下载目录、`backups` 下本次新增的数据库备份和 `upgrade-<版本>-<时间>` 目录。升级没有自动回滚；请先保持安装停止，交由维护人员按以下检查点恢复，不要重复运行 Setup 或覆盖正在使用的数据库。

1. 文件恢复：按本次 `files.json` 的逆序，先把失败的新文件另存，再将备份 `files` 下存在的原文件放回原位置。仅当记录 `existed=false` 时移除新增文件；记录有意图但没有原文件备份时保留原文件。
2. 恢复检查点中的 `deployment.json`，以及存在时的 `install-report.json`（放回 `data/install-report.json`），恢复原版本、Release 路径和监听记录。保留旧 staged release。
3. 需要恢复数据时，使用 `files.json` 的 `databaseBackup` 和旧版 native runner，按其 Restore 参数恢复到一个**新数据库名**。保留失败数据库与附件，不自动覆盖或删除。
4. 重新验证原 HTTPS 地址、登录、项目、任务、附件与另一台电脑访问，再启动原 profile。

尚未生成检查点的检查、下载或备份失败，不代表安装已被替换；先核对新增备份和文件记录。

## 命令行

自动化不会等待输入，缺参会报错。仅检查：

```powershell
.\Update.ps1 -Root 'C:\UWorkers' -CheckOnly
```

确认执行使用 `-ConfirmUpgrade`。缺失安装记录时可补 `-Profile`、`-PublicOrigin` 和 `-LanAccess` / `-NoLanAccess`，已有记录冲突时必须先核对，工具不会替换地址或访问范围。也可继续使用底层入口：

```powershell
.\Apply-Upgrade.ps1 -Root 'C:\UWorkers' -Profile office -PublicOrigin https://192.168.1.100:13443 -CheckOnly
.\Apply-Upgrade.ps1 -Root 'C:\UWorkers' -Profile office -PublicOrigin https://192.168.1.100:13443
```

不要将差量包传给旧 Upgrade.cmd 的 `-Source`，也不要用 robocopy `/MIR` 覆盖安装目录。
