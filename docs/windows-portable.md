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

## 场景四：内网同事免邮件加入工作区

便携包默认开启账号密码登录、关闭邮件验证码登录，没有配置 SMTP / Resend，设置页也没有发信配置入口。在 "成员" 中输入邮箱后，邀请会先保存，再尝试发邮件；当前 "已发送邀请" 提示不代表邮件已经送达。如果 "设置 → 成员 → 待接受邀请" 中已有记录，不必重复邀请。

内网使用两种链接完成加入，不需要邮件验证码，也不需要开启 "开放注册"：

| 链接 | 谁生成 | 用途 |
| --- | --- | --- |
| 账号邀请链接 | **实例管理员**，"设置 → 用户与注册" | 同事自行设置登录名和密码；单次使用，默认 24 小时有效。不会自动加入工作区。 |
| 工作区分享链接 | 该工作区的 `owner` / `admin`，"设置 → 成员 → 分享链接" | 已登录的同事加入指定工作区；可选成员角色和有效期，默认 7 天。不会创建可登录账号。 |

**先确认实例管理员。** 这是管理整台服务账号的权限，与某个工作区的 `owner` / `admin` 不同。已发布的 v0.2.1 不会自动给默认 `admin` 授予这项权限；看不到 "用户与注册" 时，按下一节在部署主机初始化。工作区分享链接不能代替账号邀请。

本 PR 修复后，干净安装会仅为**本次新建的操作员**初始化实例管理员。已存在的同名 / 同邮箱账号、重跑 Setup、升级及接管部署都保留原权限；之前撤销的实例权限不会自动恢复。若初次授权中断，后续恢复也走下面的显式操作。此权限与成员页的 "分享链接" 标签无关：工作区 `owner` / `admin` 应能看到该标签。用户已找到标签并成功让同事加入，原标签缺失事项已关闭。

### 已安装 v0.2.1：在部署主机初始化实例管理员

由主机负责人决定把默认操作员设为实例管理员，在**安装目录**打开普通 PowerShell。服务须已通过 `Start.cmd` 启动。下面只使用包内工具，不编译、不手写 SQL，不改密码或注册开关。

先读取安装时记录的身份，检查包内工具；只显示身份字段，不显示整个配置或密码文件。把第一行路径换成实际安装位置：

```powershell
$packageRoot = (Resolve-Path 'D:\UWorkers\Portable').Path
$configPath = Join-Path $packageRoot 'deployment.json'
$config = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
$identity = Get-Content -LiteralPath (Join-Path $packageRoot 'data\local-login.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$tool = Join-Path ([string]$config.Release) 'local-auth.exe'
if (-not (Test-Path -LiteralPath $tool -PathType Leaf)) { throw '找不到当前部署的 local-auth.exe，请检查安装目录。' }
$operatorId = [guid]::Parse([string]$identity.user_id).ToString()
[pscustomobject]@{ Username = $identity.username; Email = $identity.email; UserId = $operatorId } | Format-List
```

确认这是要授权的操作员。下面用本机保存的登录信息读取当前账号，与记录的 UUID、邮箱核对后才授权；密码和临时会话只在内存使用。如果密码已改、安装身份记录缺失或核对失败，停止此步骤，由主机负责人核实账号，不能改用猜测的 UUID。`DATABASE_URL` 仅在当前代码块内清空，避免工具连接到其他数据库，结束时还原：

```powershell
$previousDatabaseUrl = [Environment]::GetEnvironmentVariable('DATABASE_URL', 'Process')
try {
    [Environment]::SetEnvironmentVariable('DATABASE_URL', $null, 'Process')
    & $tool list --deployment-config $configPath
    if ($LASTEXITCODE -ne 0) { throw '账号列表读取失败，未授权。' }
    $credentialLines = @(Get-Content -LiteralPath (Join-Path $packageRoot 'data\local-login.txt') -Encoding UTF8)
    if ($credentialLines.Count -lt 2 -or $credentialLines[0].Trim() -ne $identity.username) { throw '本机登录记录不一致，未授权。' }
    $localApi = 'http://127.0.0.1:' + [int]$config.ApiPort
    $body = @{ username = $credentialLines[0].Trim(); password = $credentialLines[1].Trim() } | ConvertTo-Json -Compress
    $session = Invoke-RestMethod -Uri ($localApi + '/auth/password') -Method Post -ContentType 'application/json' -Body $body
    $current = Invoke-RestMethod -Uri ($localApi + '/api/me') -Headers @{ Authorization = ('Bearer ' + $session.token) }
    if ($current.id -ne $operatorId -or $current.email -ne $identity.email) { throw '当前登录身份与安装记录不一致，未授权。' }
    & $tool grant-admin --user-id $operatorId --deployment-config $configPath
    if ($LASTEXITCODE -ne 0) { throw '实例管理员初始化失败，请保留错误信息排查。' }
} finally {
    $credentialLines = $null; $body = $null; $session = $null
    [Environment]::SetEnvironmentVariable('DATABASE_URL', $previousDatabaseUrl, 'Process')
}
```

成功时工具显示 `Instance administrator granted for ...`，重复显式执行显示 `already granted`，不会额外创建账号。退出网页账号并重新登录，打开 "设置 → 用户与注册"，再执行下文。此步骤是负责人主动授权；普通 Setup 重跑和升级不会执行它。不要使用固定操作者身份的 `bootstrap-host` 替代核对。

### 管理员准备两种链接

1. 先用公司电脑能访问的局域网 HTTPS 地址打开看板，例如 `https://192.168.3.49:13443`。参考上一节处理根证书及局域网入口。**旧版 v0.2.1 按浏览器当前入口复制链接**，管理员也应通过局域网入口登录后复制。
2. 实例管理员打开 "设置 → 用户与注册"，在 "添加用户" 填同事的显示名和邮箱，点击 "添加用户并创建邀请"，再点击 "复制邀请链接"，单独发给该同事。如果账号已存在，不重复添加：尚未激活的账号可在 "更多操作 → 重新生成邀请" 中取得新链接；已激活的账号直接使用原登录名和密码。
3. 工作区管理员打开目标工作区的 "设置 → 成员 → 分享链接 → 创建分享链接"，角色选 "成员"，按需选择有效期，点击 "生成"，再点击 "复制链接"。把这个工作区分享链接也发给同事。**"分享链接" 是成员页中的独立标签，不在只填写邮箱的 "邀请成员" 弹窗内；先关闭弹窗再切换标签。**

旧版从本机入口复制出 `localhost` / `127.0.0.1` 时，改用同事能访问的局域网入口登录，到 "成员 → 分享链接" 复制**已有链接**即可，无须重新生成。手动修正也可以，但只能替换为**同一套部署**的协议、主机和端口，后面的 `/join?code=...` 保持原样；已经加入的同事无须再次邀请。账号邀请链接同样保留完整路径和 `#token=...`，仅发给对应同事；工作区分享链接在有效期内可供拿到链接的已登录账号加入。

本 PR 修复后，工作区分享及账号邀请链接优先采用部署配置中的有效、非回环前端入口：后端运行时读取 `MULTICA_APP_URL`，无有效值时再读取 `FRONTEND_ORIGIN`，通过 `/api/config` 的 `public_app_url` 传给网页。只接受无账号密码、无路径/query/hash 的 HTTP(S) origin，保留协议和端口。未配置、无效、仅有回环地址、配置尚未加载或连接旧服务端时，仍按浏览器当前入口复制；桌面端继续使用原有运行配置。两种复制入口共用这一地址选择，不同入口的登录会话独立。此修复不会自动识别局域网地址，也不保证其他电脑的网络或证书已就绪，发送前仍应确认入口可访问。

### 同事设置密码并加入

1. 在自己的浏览器打开**账号邀请链接**，填写 "显示名"、"登录名"、"密码"、"确认密码"，点击 "设置密码并进入"。密码至少 12 个字符，以后用登录名和密码登录。
2. 新账号设置完成后会进入上手引导。此时在**同一浏览器**打开管理员发来的**工作区分享链接**，无需先创建个人工作区，也无需连接自己的运行时。
3. 加入页目前显示英文。核对工作区名称和 `Member` 角色后，点击 **Join Workspace**。如果显示 **Log In to Join**，先登录，再重新打开工作区分享链接。本 PR 修复后会先同步账号和工作区再进入看板；若显示成员关系已保存但同步失败，点击 **Retry sync**，不会重复加入。**仍使用 v0.2.1 时，如果加入后又回到上手引导，刷新当前页即可进入刚加入的工作区**，不必再创建工作区。
4. 进入目标工作区的任务看板后刷新一次，确认仍可访问。只持有工作区分享链接、尚无本实例账号的人，需要先取得账号邀请，不能越过关闭的注册。

账号邀请链接用过一次即失效；未使用但过期时，请实例管理员重新生成。仅输入相同邮箱不会证明邮箱所有权，也不能把 "用相同邮箱注册后自动接受邮箱邀请" 当作通用加入方式。

### 清理旧邮箱邀请与撤销链接

本次实测中，实例管理员添加的账号在数据库中标记为 `email_verified=true`；同事经分享链接加入后，原邮箱邀请自动变为 `accepted`，不再显示在 "待接受邀请"，无需手动撤销。这里没有收取邮件验证码，标记来自管理员创建账号的流程。

如果仍有旧邮箱邀请需要作废，在 "设置 → 成员 → 待接受邀请" 点击该行右侧的叉号（按钮名称为 "撤销发给该邮箱的邀请"），再点 "确认"。未使用的邮箱邀请会被删除；如果同事已加入，先在 "成员" 列表核对他的账号。未验证的自助注册账号可能保留旧 pending 记录；不要为了清理它而开启 "开放注册"。

三种撤销分别生效：

- "用户与注册 → 更多操作 → 撤销邀请"：撤销尚未使用的**账号设密邀请**，不用于重置已激活账号的密码。
- "成员 → 待接受邀请" 中撤销：作废该条**邮箱邀请**，不会移除已经通过分享链接加入的成员。
- "成员 → 分享链接 → 撤销链接"：阻止其他账号继续使用该**工作区分享链接**，已加入成员仍保留权限。需要取消某人的工作区访问时，在成员列表执行 "从工作区移除"。

原版 `775029717` 的浏览器步骤和持久化结果见 [R1 验证记录](../evidence/clawsta-118/r1/verification.md)；本 PR 的安装授权、v0.2.1 主机命令及加入修复见 [R2 验证记录](../evidence/clawsta-118/r2/verification.md)；双入口复制及加入见 [R3 验证记录](../evidence/clawsta-118/r3/verification.md)。公司部署的实际邀请记录需要在该部署中另行确认。

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
- 服务器局域网 IP 变了（换网络、DHCP）：重新跑一次 `Setup.cmd`（检测到已安装会走“修证书 + 启动”路径）或 `Upgrade.cmd` 重新签证书；第二台机器重跑 `connect`。IP 变了但还没重签之前，`Start.cmd` 会退回只监听本机（不会因为绑一个已不存在的地址而起不来），输出里会说明原因。
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
# 单元测试：两个 shell 都要跑。开机自启走系统自带的 5.1，而 5.1 和 7 的默认
# 文本编码不同；脚本必须两边结果一致。
powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\portable-common.test.ps1
pwsh -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\portable-common.test.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\account-initialization.test.ps1
pwsh -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\account-initialization.test.ps1

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
- **干净安装会建一个本机账号**：`admin`，密码在 `data\local-login.txt`。多用户使用上文的账号邀请和工作区分享链接，先确认实例管理员权限。
- **安装目录要可写**：别放在 `C:\Program Files` 这类普通用户不能写的地方。
