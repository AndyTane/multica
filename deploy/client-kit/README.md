# UWorkers-Client-Kit

用于把另一台 Windows 电脑的智能体接入已有的 U Workers 服务端。完整解压后使用，无需 Node、Go 或管理员权限；所选智能体 CLI 需要另外安装并登录。

## 机器重启后

双击 **Reconnect.cmd**。它复用原来的登录和证书，启动原 profile 的守护进程；已经运行时直接检查，不打断任务。先确认服务端电脑已启动、两台电脑网络可达。成功时显示 `Server access verified`，再到网页的运行时列表确认本机在线。

**Status.cmd** 查看状态和服务端访问；**Stop.cmd** 停止此 profile 的守护进程（先等任务空闲）。脚本窗口保留结果，失败时不要当作已连接。账号过期运行 Connect.cmd，网络故障恢复后重新运行 Reconnect.cmd。

## 第一次连接

编辑 `connection.json`：`serverUrl` 填公司服务端 HTTPS 地址；内网自签证书的 `rootCa` 填管理员提供的**公开根证书**文件路径。两个字段留空时，Connect.cmd 会按 CLI 现有配置或提示操作。不要放入 CA 私钥。

`profile` 留空使用原默认 profile；旧客户端使用命名 profile 时，填原来的名字。不要随意换 profile，否则会变成另一套登录和守护进程。双击 **Connect.cmd**，按浏览器提示登录。此操作会校验服务器证书并为当前 Windows 用户信任管理员提供的证书；重连不重复登录和安装证书。

账号凭据仍由 CLI 保存在当前 Windows 用户的 `.u-workers`（已有旧安装可能使用 `.multica`），不保存在本包中。发布包不含服务端地址、个人账号或公司证书。

## 更新与自启动

新包解压到独立目录，保留旧 `connection.json` 设置；CLI profile 的登录无需搬运。等任务空闲，用旧包 Stop.cmd 停止原守护进程，再在新目录用 Reconnect.cmd 启动新版本。不要覆盖正在运行的 exe。

建议先保留手动重连。需要登录 Windows 后自动连接时，确认连接成功并把包放到固定目录，然后在 `shell:startup` 放一个指向 Reconnect.cmd 的快捷方式。删除该快捷方式即可关闭；这不等于开机未登录时运行，也不负责启动远端服务端。脚本不检查或下载更新。

包版本和源码提交见 `CLIENT-FILES.json`，与同次服务端发布一致。原始压缩包下载后先用 release.json 的 SHA-256 校验；个性化修改 connection.json 后，原包文件清单不再适用于修改后的这一文件。
