# 设置 U Workers 登录（本人账号的网页初设入口）。
#
# 双击运行：读取本机部署配置和已绑定的身份，核对身份指纹后，直接在浏览器里
# 完成设置。密码只由本人在网页里输入，入口不询问、不读取、不打印、不保存任何
# 密码，也不打印邀请链接里的秘密。
#
#   尚无密码：由 local-auth.exe bootstrap-host 生成 15 分钟单次邀请，并私下打开
#             受信任的 HTTPS 注册页；邀请只存在于进程内存和浏览器地址栏。
#   已有密码：只打开登录页，绝不重置或覆盖既有密码。
#
# 预检/排错用 -Preflight：只核对身份、发布版本、配置和凭据状态，不生成邀请、
# 不打开浏览器。日常登录、发邀请、增删用户和改密码都在网页里完成。
#
# 退出码：0 完成 / 2 配置或发布缺失 / 3 身份不符 / 4 环境不满足 /
#         5 工具失败 / 6 数据库不可读
[CmdletBinding()]
param(
    [string]$Root = '',
    [string]$IdentityFile = '',
    [switch]$Preflight
)

$ErrorActionPreference = 'Stop'
# The cmd wrapper switches the console to UTF-8; mirror that for redirected runs.
try { [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false) } catch { }
$script:Title = '设置 U Workers 登录'

# The one identity this entry may provision. SHA-256 of the normalized (trimmed,
# lowercased, UTF-8) address; never inferred from a display name, an older
# verification row, the first account in the database or the runtime owner.
$script:PinnedEmailSha256 = '5983f9551dc3f2370d8f04a7291d1d03e1c7fb96eac1b5112791572728122788'

function Write-Header {
    Write-Output ''
    Write-Output '=========================================='
    Write-Output ('  ' + $script:Title + '（本人账号）')
    Write-Output '=========================================='
    Write-Output ''
}

function Fail([string]$Message, [int]$Code) {
    Write-Output ''
    Write-Output ('无法继续：' + $Message)
    Write-Output '未做任何修改。'
    exit $Code
}

function Get-EmailSha256([string]$Email) {
    $normalized = $Email.Trim().ToLowerInvariant()
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        $bytes = $sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($normalized))
    } finally { $sha.Dispose() }
    return (($bytes | ForEach-Object { $_.ToString('x2') }) -join '')
}

function Invoke-LocalSql {
    param([string]$Database, [string]$Query)
    $psql = Join-Path $Root 'runtime\pgsql\bin\psql.exe'
    if (-not (Test-Path -LiteralPath $psql)) { throw '找不到本机数据库客户端。' }
    $saved = $env:PGPASSWORD
    try {
        $env:PGPASSWORD = [string]$config.DatabasePassword
        $previous = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        try {
            $output = $Query | & $psql -h 127.0.0.1 -p ([int]$config.DatabasePort) -U $config.DatabaseUser -d $Database -v ON_ERROR_STOP=1 -tA -f - 2>&1
        } finally { $ErrorActionPreference = $previous }
        if ($LASTEXITCODE -ne 0) { throw ((($output | Out-String).Trim())) }
        return @($output | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ -ne '' })
    } finally {
        $env:PGPASSWORD = $saved
    }
}

if (-not $Root) { $Root = $PSScriptRoot }
if (-not $IdentityFile) { $IdentityFile = Join-Path $Root 'local-login-identity.json' }
$configPath = Join-Path $Root 'deployment.json'

Write-Header

if (-not (Test-Path -LiteralPath $configPath)) { Fail '找不到本机部署配置，请确认入口是从部署目录运行的。' 2 }
if (-not (Test-Path -LiteralPath $IdentityFile)) { Fail '找不到本机绑定的身份文件，请联系管理员。' 2 }
$config = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
$identity = Get-Content -LiteralPath $IdentityFile -Raw -Encoding UTF8 | ConvertFrom-Json

$userID = ([string]$identity.user_id).Trim()
if (-not $userID -or $userID -notmatch '^[0-9a-fA-F-]{36}$') { Fail '本机身份文件缺少有效的账号标识，请联系管理员。' 3 }
$identityEmail = ([string]$identity.email).Trim()
if (-not $identityEmail -or $identityEmail -notmatch '^[^@\s]+@[^@\s]+$') { Fail '本机身份文件缺少有效的邮箱，请联系管理员。' 3 }

# 1. The bound identity must be the one this entry is allowed to provision.
$pinnedHash = Get-EmailSha256 $identityEmail
if ($pinnedHash -ne $script:PinnedEmailSha256) {
    Fail '本机登录入口绑定的身份不是被授权的账号，已拒绝。' 3
}

# 2. The current release and its entry tool must be present.
$tool = Join-Path ([string]$config.Release) 'local-auth.exe'
if (-not (Test-Path -LiteralPath $tool)) { Fail '当前部署版本缺少本地登录工具，请先完成部署或联系管理员。' 2 }

# 3. The browser entry must be the deployment's trusted HTTPS origin.
$origin = ([string]$config.PublicOrigin).Trim().TrimEnd('/')
$parsedOrigin = $null
if (-not $origin -or -not [Uri]::TryCreate($origin, [UriKind]::Absolute, [ref]$parsedOrigin)) {
    Fail '本机部署配置缺少有效的对外地址，请联系管理员。' 4
}
if ($parsedOrigin.Scheme -ne 'https') {
    Fail '本机部署的对外地址不是 HTTPS，已拒绝打开网页设置页。' 4
}
$origin = $parsedOrigin.GetLeftPart([UriPartial]::Authority)

# 4. The pinned account must still exist with exactly that address.
try {
    $rows = @(Invoke-LocalSql -Database $config.Database -Query ('SELECT id::text || ''|'' || email FROM "user" WHERE id = ''' + $userID + '''::uuid;'))
} catch {
    Fail '无法读取本机账号信息，请联系管理员。' 6
}
if ($rows.Count -ne 1) { Fail '绑定的账号不存在，请联系管理员。' 3 }
$parts = ([string]$rows[0]).Split('|')
$actualID = $parts[0].Trim()
$actualEmail = $parts[1].Trim()
if ($actualID -ne $userID) { Fail '本机登录入口绑定的身份与目标账号不一致，已拒绝。' 3 }
if ($actualEmail.Trim().ToLowerInvariant() -ne $identityEmail.ToLowerInvariant()) {
    Fail '本机登录入口绑定的身份与目标账号不一致，已拒绝。' 3
}
if ((Get-EmailSha256 $actualEmail) -ne $script:PinnedEmailSha256) {
    Fail '目标账号的邮箱与被授权的身份不一致，已拒绝。' 3
}

# 5. Credential state decides what happens next; it is never overwritten.
try {
    $existing = @(Invoke-LocalSql -Database $config.Database -Query ('SELECT count(*) FROM local_password_credential WHERE user_id = ''' + $userID + '''::uuid;'))
} catch {
    Fail '无法检查现有登录状态，请联系管理员。' 6
}
if ($existing.Count -ne 1) { Fail '无法检查现有登录状态，请联系管理员。' 6 }
$existingCount = 0
if (-not [int]::TryParse(([string]$existing[0]), [ref]$existingCount)) { Fail '无法检查现有登录状态，请联系管理员。' 6 }

$state = if ($existingCount -gt 0) { '已设置密码' } else { '尚未设置密码' }
Write-Output ('账号：' + ([string]$identity.display_name))
Write-Output ('邮箱：' + $actualEmail.Substring(0, [Math]::Min(2, $actualEmail.Length)) + '***' + $actualEmail.Substring($actualEmail.IndexOf('@')))
Write-Output ('状态：' + $state)
Write-Output ('版本：' + ([string]$config.Version) + ' (' + ([string]$config.Commit) + ')')
Write-Output ''

if ($Preflight) {
    Write-Output '预检通过：身份、版本与配置一致。未生成邀请，未打开浏览器。'
    exit 0
}

if ($existingCount -gt 0) {
    Write-Output '该账号已经有密码。正在打开登录页，本入口不会重置或覆盖既有密码。'
    try {
        Start-Process ($origin + '/login') | Out-Null
    } catch {
        Fail '无法打开登录页，请手动在受信任的浏览器里访问本机地址。' 5
    }
    Write-Output '请在浏览器里用既有登录名和密码登录。忘记密码请由管理员处理。'
    exit 0
}

Write-Output '接下来会在浏览器里打开受信任的 HTTPS 设置页，请你本人在网页里设置登录名和密码。'
Write-Output '邀请 15 分钟内有效、只能使用一次；链接只出现在浏览器地址栏，本程序不打印、不保存。'
Write-Output ''
$confirm = Read-Host '现在打开网页设置页？(Y/n)'
if ($null -eq $confirm) { $confirm = '' }
if (([string]$confirm).Trim().ToLowerInvariant() -in @('n', 'no')) {
    Write-Output '已取消，未做任何修改。'
    exit 0
}

Write-Output ''
# bootstrap-host mints the invitation and opens the private setup page itself: the
# secret goes from the tool's memory into the browser's address bar and is never
# printed here. Its own output reports only the invitation id and expiry.
& $tool bootstrap-host --user-id $userID --minutes 15 --deployment-config $configPath
$exitCode = $LASTEXITCODE

if ($exitCode -ne 0) {
    Write-Output ''
    Write-Output ('无法打开网页设置页（退出码 ' + $exitCode + '）。未生成可用的邀请时请重试或联系管理员。')
    exit 5
}

Write-Output ''
Write-Output '设置页已打开：请在网页里填写登录名和密码。完成后回到网页登录即可。'
Write-Output '之后日常登录、发邀请、增删用户和改密码都在网页里完成。'
exit 0
