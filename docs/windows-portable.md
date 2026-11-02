# 便携一键部署包（Windows，免管理员）

一句话：把一个文件夹拷到 Windows 电脑上，双击 `Setup.cmd`，这台电脑就是一台完整的 U Workers 服务器——服务端、网页、数据库、局域网 HTTPS、本机 opencode 运行时全在里面。不需要管理员权限，不需要联网装任何东西，所有数据都留在这个文件夹里。

这是目前推荐的部署方式。旧的按步骤手工部署方式仍然有效，见 [windows-native.md](windows-native.md)。

## 包里有什么

| 目录 / 文件 | 是什么 |
| --- | --- |
| `Setup.cmd` | 唯一的入口：装好并启动全部东西 |
| `Start.cmd` / `Stop.cmd` / `Status.cmd` | 日常启动、停止、看状态 |
| `Upgrade.cmd` | 升级（先自动备份，再换新版本，数据不动） |
| `Uninstall.cmd` | 卸载（默认保留数据；`-PurgeData` 才删数据） |
| `root-ca.crt` | 装好之后生成，给第二台电脑用（见下文） |
| `payload\` | 预编译好的服务端、网页、CLI、安装包、依赖运行时（Node、PostgreSQL、Caddy、opencode） |
| `tools\` | Go 工具链和依赖缓存，用来在这台机器上离线重新打安装包 |
| `data\` | 数据库、上传文件、日志、备份、安装报告 |

整个文件夹就是这台服务器的“整机”。移动位置 = 换安装路径；拷到新机器 = 全新部署。

## 场景一：公司内网电脑，全新部署（无管理员、无外网）

1. 把 `UWorkers-Portable-v0.2.x` 文件夹整个拷到电脑上一个可写的位置，例如 `C:\Users\<你>\U Workers Portable`。
2. 双击 `Setup.cmd`。第一次要等一两分钟（建数据库、跑迁移、签证书、装环境）。
3. 结束时浏览器会自动打开登录页。用户名是 `admin`，密码在 `data\local-login.txt` 里（安装时生成，只存本机）。
4. 登录后就看到看板。看板右上角「运行时」里应该已经有本机的 opencode 运行时。

Setup 会顺手做这些事，全都不需要管理员：

- 在文件夹里建 PostgreSQL 集群和数据库，账号密码只写在受保护的 `deployment.json` 里；
- 起 Go API（只监听 `127.0.0.1:18080`）和网页（`127.0.0.1:13000`）；
- 起 Caddy 做 HTTPS（本机 `https://localhost:13443`；局域网 `https://<本机局域网IP>:13443` 只在防火墙规则覆盖时才开），自动生成私有根证书并导出 `root-ca.crt`；
- 建一个本机管理员账号和对应的看板空间，生成一个只给本机守护进程用的令牌；
- 用包里的 opencode 启动本机守护进程并连回自己，看板上出现 opencode 运行时；
- 在当前用户的「启动」文件夹里放一个隐藏启动项：以后开机登录自动拉起服务（不写系统服务，不需要管理员）。

如果 Windows 弹「安全警告：是否信任此证书」，点「是」；不想点就关掉，用 `Setup.cmd -SkipBrowserTrust` 跳过——CLI 和守护进程照样工作，只是浏览器会提示一次证书不受信任。以后想补装：双击 `Trust-Browser.cmd`。

## 场景二：台式机（已有 CLAWSTA-81 的数据）换成便携包

旧的部署目录（例如 `D:\UWorkers\clawsta80\native`）里已经有数据库和上传文件。接管它分三步：

1. 把便携包文件夹拷到台式机上，例如 `D:\UWorkers\Portable`。
2. 用普通用户（不是管理员）打开 PowerShell，执行：

`@powershell
cd D:\UWorkers\Portable
.\Setup.cmd -AdoptRoot D:\UWorkers\clawsta80\native
`@

   接管过程会：
   - 先调用旧部署自己的备份功能，把数据库 dump、上传文件、配置和校验和写进旧目录的 `backups\`；
   - 停掉旧部署（只停旧部署记录的进程，不碰别的服务）；
   - 把旧数据库集群和上传文件复制到便携包里，保留原来的账号、空间、任务数据，也保留原 JWT 密钥（已登录的会话不失效）；
   - 换上新版本、跑数据库迁移、起新服务、重新签 HTTPS 证书；
   - 用原来的空间所有者身份生成本机守护进程令牌，把本机 opencode 运行时接回来。

3. 结束后用原来的账号密码登录（账号没变）。旧目录原样保留，确认没问题后再自行归档。

想指定别的账号做本机守护进程，加 `-OperatorEmail <账号邮箱>`。

## 场景三：第二台电脑（笔记本）接入

服务端装好后：

1. 在服务器文件夹里找到 `root-ca.crt`（Setup 生成的公共根证书）。
2. 找到安装包 `payload\installer\u-workers-setup-v0.2.x-windows-amd64.exe`，和 `root-ca.crt` 一起拷到笔记本（U 盘、共享目录、IM 都行）。
3. 笔记本上双击安装包（装到当前用户目录，不需要管理员）。
4. 打开命令行执行一条命令：

`@powershell
u-workers connect --server-url https://192.168.3.49:13443 --root-ca .\root-ca.crt
`@

   把 IP 换成服务器实际的局域网地址（Setup 结束时会打印）。这条命令会验证证书、信任根证书、登录、启动守护进程，并在看板登记这台笔记本的运行时。浏览器第一次打开看板会弹一次信任确认，点「是」即可。

更细的排错见 [windows-second-machine.md](windows-second-machine.md)。

## 升级

1. 把新版本便携包拷到机器上（可以先解压到旁边目录）。
2. 在旧包目录里执行：

`@powershell
.\Upgrade.cmd -Source D:\新建的\UWorkers-Portable-v0.2.4
`@

   或者把新包内容覆盖到旧包目录后直接双击 `Upgrade.cmd`。

升级顺序是固定的：先把数据库和上传文件备份到 `backups\`（备份失败就中止，什么都不换），再停服务、换新版本、启动、跑迁移，最后核对升级前后数据量没有减少。升级不会覆盖 PostgreSQL 二进制；跨 PostgreSQL 大版本迁移是另一件事，不在这一个包的范围里。

## 局域网入口和 Windows 防火墙（为什么不再弹框）

桌面以前每次 Caddy 启动都会弹「Windows Defender 防火墙已阻止此程序的部分功能」。原因有两个，现在都改掉了：

- Caddy 默认监听 `0.0.0.0` / `::`（所有网卡），而不是只监听需要的地址；
- Caddy 默认开 HTTP/3，同一端口上多开了一个 **UDP 13443**。防火墙规则只放行 TCP 13443，UDP 这个套接字没有任何规则覆盖，于是 Windows 每次启动都问一次——普通用户又点不了那个框。

现在的行为：

| 情况 | 行为 |
| --- | --- |
| Caddyfile | 写死 `bind 127.0.0.1`（只在有防火墙规则时才追加局域网 IP），不再有通配监听 |
| HTTP/3 | 关掉（`protocols h1 h2`），13443 只剩唯一一个 TCP 监听 |
| 有管理员权限 | `Setup.cmd` / `Upgrade.cmd` 自动加一条**按程序**的入站规则：显示名 `UWorkers-Portable-HTTPS-<端口>`，程序是包内 `payload\runtime\caddy\caddy.exe`，只放行本网段 |
| 没有管理员权限，且没有现成规则 | 不加规则、不弹框：Caddy 只监听 127.0.0.1，安装输出直接写 `LAN access is off: ... Run Setup.cmd once as an administrator to add the rule` |
| 开机自启（`Start.cmd`） | 不写防火墙，只读规则判断能不能开局域网——登录时也不会弹框 |
| `Setup.cmd -NoLanAccess` | 强制只监听本机，即使有管理员权限 |

规则按程序（固定的 `caddy.exe` 路径）而不是按端口写，所以同一个文件夹原地升级不会改变程序身份，也就不会重新弹框。装好后 `Status.cmd` 的 `LAN entry` 一行会写 `on`（附规则名）或 `off`。

需要局域网访问、但机器上没有管理员时：让 IT 加一条入站规则（TCP 13443，程序指向包内 `payload\runtime\caddy\caddy.exe`，只放行本网段），或者用管理员身份跑一次 `Setup.cmd`。没有规则时不要手动把 Caddy 改成监听 0.0.0.0——那正是弹框的来源。

## 停止、启动、卸载

`@powershell
.\Stop.cmd        # 停止服务；数据保留
.\Start.cmd       # 手动启动（登录时本来就会自动启动）
.\Status.cmd      # 看状态：端口、证书、守护进程、运行时

.\Uninstall.cmd              # 停止服务、删开机启动项、去掉浏览器信任、清掉本包 CLI 配置；数据保留
.\Uninstall.cmd -PurgeData   # 再加：删数据库、上传文件和备份（不可恢复）
`@

卸载不会删除这个文件夹本身，删文件夹请自己来（先确认数据不要了）。

## 证书和浏览器

- 私有根证书由 Caddy 在包内 `https\data` 里生成，公钥导出为 `root-ca.crt`。
- 装根证书到当前用户的证书库会弹一次 Windows 安全警告，这是系统行为，必须手点。不点就跳过，只影响浏览器提示。
- 服务器局域网 IP 变了（换网络、DHCP）：重新跑一次 `Setup.cmd`（检测到已安装会走“修证书 + 启动”路径）或 `Upgrade.cmd` 重新签证书；第二台机器重跑 `connect`。
- 证书只签给 `localhost`、`127.0.0.1` 和安装时检测到的那个局域网 IP。用主机名访问会不匹配。
- 换了网段（例如 192.168.3.x → 192.168.5.x）时，旧的防火墙规则只放行旧网段，局域网入口会被防火墙挡住：有管理员权限重跑一次 `Setup.cmd` 会按新网段重建规则；没有权限就保持本机访问，别手动放开。

## 生成这个包（在台式机上）

打包需要开发工具（只在打包机上需要，目标机器不需要）：Go、Node.js、pnpm（仓库依赖已安装）、一份 PostgreSQL Windows 二进制目录、`caddy.exe`（官方 Caddy 编译版）、`opencode.exe`。

`@powershell
# 在仓库根目录，按需要替换路径
pwsh -File deploy\portable\build-package.ps1 -Version v0.2.4 -PostgresDirectory D:\UWorkers\clawsta80\native\runtime\pgsql -CaddyExe D:\Tools\caddy\caddy.exe -OpenCodeExe C:\Users\<你>\AppData\Roaming\npm\node_modules\opencode-ai\bin\opencode.exe -OutputRoot D:\UWorkers\packages
`@

产物：`D:\UWorkers\packages\UWorkers-Portable-v0.2.4` 文件夹和同名 `.zip`。`-SkipBuildKit` 可以不带 Go 工具链/源码（包更小，但目标机器不能离线重打安装包）。

### 在包里离线重打安装包

目标机器上双击 `Build-Installer.cmd`，或者在包里执行 `.\Build-Installer.cmd`。它使用 `tools\go`、`tools\gomodcache` 和 `payload\source\source.zip`，全程断网也能完成，几十秒到几分钟，产物在 `dist\`。

### 验收（开发/发布前跑）

`@powershell
# 单元测试
powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\portable-common.test.ps1

# 完整验收：干净目录 + 断网 + 全新安装 + 运行时上线 + 离线重打安装包 + 升级演练 + 模拟第二台机器接入
pwsh -File deploy\portable\test\acceptance.ps1 -Package D:\UWorkers\packages\UWorkers-Portable-v0.2.4 -WorkRoot D:\UWorkers\acceptance
`@

验收脚本在一台机器的独立目录里模拟两台电脑（两个互不影响的 CLI profile），结束后会停服务、删启动项、清 CLI 配置、删测试目录，不会把测试服务留在机器上。

## 已知限制

- **安装目录尽量短**：Web 依赖里有一些很深的路径，Windows 仍有 260 字符的路径上限。打包时已经把依赖目录名压成短哈希，正常路径（例如 `C:\UWorkers\Portable`）没问题；如果 Setup 报“release 未完整复制”，换一个更短的目录再装。
- **需要 Windows 10 1803 或更新的 Windows**：脚本用的是 Windows 自带的 PowerShell 5.1 和 curl.exe（安装时不联网、不装任何东西）。Windows 7/8 和精简版系统可能没有 curl，Setup 会在 HTTPS 校验那一步明确报错。
- **没有代码签名**：安装包和 `u-workers.exe` 是自签的，Windows SmartScreen 会提示“未知发布者”，点“仍要运行”。
- **浏览器信任必须手点一次**：Windows 对根证书安装弹的安全警告无法用脚本静默通过；可以跳过，CLI 和守护进程不受影响。
- **局域网地址写死在证书里**：换网络要重签（重跑 Setup 即可）。
- **PostgreSQL 不跨大版本升级**：升级包只换应用，不换数据库大版本。
- **干净安装会建一个本机账号**：`admin`，密码在 `data\local-login.txt`。多用户继续走看板原有的邀请流程。
- **安装目录要可写**：别放在 `C:\Program Files` 这类普通用户不能写的地方。
