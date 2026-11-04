# U Workers 客户端

完整解压后使用，无需 Node、Go 或管理员权限。

| 要做什么 | 双击 |
| --- | --- |
| 首次连接 | **Connect.cmd**：填写管理员提供的 HTTPS 服务地址和公开根证书路径，按浏览器提示登录；公共 CA 可不填证书。原 profile 留空沿用默认登录，命名 profile 填原名称。无需编辑 JSON。 |
| 电脑重启后 | **Reconnect.cmd**：沿用原登录和证书。已经运行或连接中时不重复启动。**Status.cmd** 查看连接状态。 |
| 换新版 | 将新包解压到独立目录，双击新包 **Import.cmd** 选择旧包目录，仅需一次。等任务空闲，用旧包 **Stop.cmd** 停止，再用新包 **Reconnect.cmd** 重连。 |
| 停止 | **Stop.cmd**，先等本机任务空闲。 |

看到“服务器访问已验证”后，到网页的运行环境列表确认本机在线。尚未连接属于正常停止状态；“连接中”请稍后查看；本机已运行但服务器访问未通过时，先检查服务器和网络，再检查原 profile 的登录。登录过期时用 Connect.cmd。所选智能体 CLI 需另外安装并登录。

## 换新版与设置

Import 只读取旧包 `connection.json` 的服务地址、profile 和公开证书，不搬运账号或私钥，不启停后台连接，不修改旧包。相对证书复制到新包 `certificates` 并重写引用；绝对证书路径继续引用原文件，请保留该文件。新包已有设置时会先确认，取消或校验失败保留原设置。

Connect 保留已有非空设置。需要更换地址时，可用下列命令显式指定；现有设置更换须 `-ReplaceSettings`。调用脚本默认不等待输入，只有双击的交互入口会提示填写。示例：

```powershell
.\client.ps1 -Action Connect -ServerUrl https://server.example:13443 -RootCa 'C:\Certificates\root-ca.crt' -Profile office -ReplaceSettings
.\client.ps1 -Action Import -SourceDirectory 'C:\Old Client Kit'
```

仅接受公开 X.509 证书；不要选择私钥或含私钥的容器。首次 Connect 按 CLI 原流程验证证书并为当前 Windows 用户安装信任，Reconnect 不重复登录或安装证书。账号凭据由 CLI 保存在当前 Windows 用户 `.u-workers`（旧安装可能为 `.multica`），不在包中。

## 包与诊断

版本和源码提交见 `CLIENT-FILES.json`，与同次服务端发布一致。下载后按 `release.json` 校验 SHA-256。个性化修改后，原清单不再适用于连接设置和新增证书。`u-workers.exe` 与 `multica.exe` 是同一版本程序，两个文件都保留，旧命令名供内置技能使用。

不要覆盖正在运行的程序。连接失败时保留窗口中的简短提示，不要分享登录文件、令牌或完整配置。脚本不检查更新、不下载新版、不设置开机启动。需要登录 Windows 后自动重连时，将 Reconnect.cmd 的快捷方式放入 `shell:startup`；删除快捷方式即可关闭。
