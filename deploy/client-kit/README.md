# U Workers 客户端

完整解压后使用，无需 Node、Go 或管理员权限。

| 要做什么 | 双击 |
| --- | --- |
| 首次连接 | **Connect.cmd**：填写管理员提供的 HTTPS 服务地址和公开根证书路径，按浏览器提示登录；公共 CA 可不填证书。原 profile 留空沿用默认登录，命名 profile 填原名称。无需编辑 JSON。 |
| 电脑重启后 | **Reconnect.cmd**：沿用原登录和证书。已经运行或连接中时不重复启动。**Status.cmd** 查看连接状态。 |
| 换新版 | 将新包解压到独立目录，双击新包 **Import.cmd** 选择旧包目录，仅需一次。等任务空闲，用旧包 **Stop.cmd** 停止，再用新包 **Reconnect.cmd** 重连。 |
| 停止 | **Stop.cmd**，先等本机任务空闲。 |

看到“服务器访问已验证”后，到网页的运行环境列表确认本机在线。尚未连接属于正常停止状态；“连接中”请稍后查看；本机已运行但服务器访问未通过时，先检查服务器和网络，再检查原 profile 的登录。登录过期时用 Connect.cmd。所选智能体 CLI 需另外安装并登录。

## OpenCode 程序和配置

首次连接会同时检查 PATH、用户常用安装目录、固定磁盘 Tools/Programs 和指定位置。搜索不访问网络盘，不遍历其他用户目录，不跟随目录链接；每轮最多 600 个目录、3 层、5 秒。唯一安装显示完整路径后继续；多个安装按编号选择，也可输入完整文件或目录路径。npm 的同一套包装入口和原生程序只显示一次。输入错误可重试，R 重新检查，Q 取消，S 跳过 OpenCode 后继续使用其他运行环境。

配置支持 `opencode.json` 和 `opencode.jsonc`。已有 `OPENCODE_CONFIG` 优先用于文件选择，`OPENCODE_CONFIG_DIR`、`XDG_CONFIG_HOME` 继续生效；否则查找用户配置目录、工作目录及祖先项目、程序附近、Kit 和 Documents/Desktop/Downloads。多个候选列出路径供选择，不改写配置。缺少配置时显示当前用户可使用的完整保存路径，放好后按 R 重试，或输入其他配置路径。

程序和配置路径按 profile 保存在原 Kit 的 `opencode-selection.json`，重连和自动升级继续使用。文件只含路径；模型配置和密钥留在原位置。Status.cmd 可查看保存的路径。若原文件移动或损坏，提示修正原路径或重新选择，不静默换安装。运行中的连接不更换程序；先等任务空闲，再 Stop、选择和 Reconnect。

指定工具/项目目录或直接选择文件：

```powershell
.\client.ps1 -Action Connect -Interactive -SearchDirectory 'D:\Tools','D:\MyProjects'
.\client.ps1 -Action Reconnect -OpenCodePath 'D:\Tools\OpenCode\opencode.exe' -OpenCodeConfig 'D:\Models\opencode.jsonc'
.\client.ps1 -Action Reconnect -SkipOpenCode
```

脚本调用默认不等待输入。多个候选或显式选择不可用时，返回完整候选和指定方式；首次未指定 OpenCode 且未找到程序/配置时，说明缺项并让其他运行环境继续连接，不把 OpenCode 报为就绪。双击 Connect/Reconnect 保留窗口和重试提示。选择基础配置不会改变 OpenCode 自身的配置叠加规则，项目配置仍按任务目录生效（[官方配置说明](https://docs.opencode.ai/docs/config/)）。

## 换新版与设置

带自动升级能力的 Kit 只需按上表换一次。此后跟随实际服务端版本，在任务空闲后自动升级；离线电脑下次连接补升。Status.cmd 显示进度与回退原因。可运行 `client.ps1 -Action PauseUpdate` 暂停、`-Action ResumeUpdate` 恢复。保留原目录的 connection.json、证书、kit-install.json、current.json、programs 和 .managed；原登录与模型配置不写入程序包。根目录程序是恢复入口，当前运行版本以 Status.cmd 为准。

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
