# 设置 U Workers 登录（仅限已授权的本人账号）。
#
# 双击运行：读取本机部署配置和已绑定的身份，只允许为那一个账号设置密码。
# 输入自定义登录名后，由 local-auth 工具提示两次隐藏密码；入口不读取、
# 不写入、不打印任何密码，已存在凭据时不覆盖。
#
param(
    [string]$Root = '',
    [string]$IdentityFile = ''
)

$ErrorActionPreference = 'Stop'
# The cmd wrapper switches the console to UTF-8; mirror that for redirected runs.
try { [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false) } catch { }
$script:Title = '设置 U Workers 登录'

function Write-Header {
    Write-Output ''
    Write-Output '=========================================='
    Write-Output ('  ' + $script:Title + '（本人账号）')
    Write-Output '=========================================='
    Write-Output ''
}

function Fail([string]$Message, [int]$Code) {
    Write-Output ''
    Write-Output ('无法设置：' + $Message)
    Write-Output '未做任何修改。'
    exit $Code
}

if (-not $Root) { $Root = $PSScriptRoot }
if (-not $IdentityFile) { $IdentityFile = Join-Path $Root 'local-login-identity.json' }
$configPath = Join-Path $Root 'deployment.json'
$tool = $null

Write-Header

if (-not (Test-Path -LiteralPath $configPath)) { Fail '找不到本机部署配置，请确认入口是从部署目录运行的。' 2 }
if (-not (Test-Path -LiteralPath $IdentityFile)) { Fail '找不到本机绑定的身份文件，请联系管理员。' 2 }
$config = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
$identity = Get-Content -LiteralPath $IdentityFile -Raw -Encoding UTF8 | ConvertFrom-Json
$tool = Join-Path ([string]$config.Release) 'local-auth.exe'
if (-not (Test-Path -LiteralPath $tool)) { Fail '当前部署版本缺少本地登录工具，请联系管理员。' 2 }

function Invoke-LocalSql {
    param([string]$Database, [string]$Query)
    $psql = Join-Path $Root 'runtime\pgsql\bin\psql.exe'
    if (-not (Test-Path -LiteralPath $psql)) { throw '找不到本机数据库客户端。' }
    $saved = $env:PGPASSWORD
    try {
        $env:PGPASSWORD = [string]$config.DatabasePassword
        $output = $Query | & $psql -h 127.0.0.1 -p ([int]$config.DatabasePort) -U $config.DatabaseUser -d $Database -v ON_ERROR_STOP=1 -tA -f - 2>&1
        if ($LASTEXITCODE -ne 0) { throw ((($output | Out-String).Trim())) }
        return @($output | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ -ne '' })
    } finally {
        $env:PGPASSWORD = $saved
    }
}

try {
    $rows = @(Invoke-LocalSql -Database $config.Database -Query ('SELECT email || ''|'' || name FROM "user" WHERE id = ''' + [string]$identity.user_id + ''';'))
} catch {
    Fail '无法读取本机账号信息，请联系管理员。' 6
}
if ($rows.Count -ne 1) { Fail "绑定的账号不存在，请联系管理员。" 3 }
$parts = $rows[0].Split('|')
$actualEmail = $parts[0]
$actualName = $parts[1]
if ($actualEmail.Trim().ToLowerInvariant() -ne ([string]$identity.email).Trim().ToLowerInvariant()) {
    Fail '本机登录入口绑定的身份与目标账号不一致，已拒绝。' 3
}

try {
    $existing = @(Invoke-LocalSql -Database $config.Database -Query ('SELECT count(*) FROM local_password_credential WHERE user_id = ''' + [string]$identity.user_id + ''';'))
} catch {
    Fail '无法检查现有登录状态，请联系管理员。' 6
}
if ($existing.Count -ne 1) { Fail '无法检查现有登录状态，请联系管理员。' 6 }
$existingCount = 0
if (-not [int]::TryParse($existing[0], [ref]$existingCount)) { Fail "无法检查现有登录状态，请联系管理员。" 6 }
if ($existingCount -gt 0) {
    Fail '该账号已经设置过登录密码。本入口不会覆盖既有密码；如需重置，请联系管理员处理。' 4
}

Write-Output ('即将为以下账号设置本机登录：' + $actualName)
Write-Output ('邮箱：' + $actualEmail.Substring(0, [Math]::Min(2, $actualEmail.Length)) + '***' + $actualEmail.Substring($actualEmail.IndexOf('@')))
Write-Output ''
Write-Output '登录名将作为网页登录时输入的账号；密码由下一步的隐藏提示输入两次。'
Write-Output ''
$loginName = Read-Host '请输入登录名（字母、数字、点、下划线或连字符，3-64 位）'
if ($null -eq $loginName) { $loginName = '' } else { $loginName = ([string]$loginName).Trim() }
if ([string]::IsNullOrWhiteSpace($loginName)) { Fail '登录名为空，已取消。' 5 }
if ($loginName.Length -lt 3 -or $loginName.Length -gt 64) { Fail '登录名长度需在 3-64 位之间，已取消。' 5 }
if ($loginName -notmatch '^[A-Za-z0-9._-]+$') { Fail '登录名只能包含字母、数字、点、下划线和连字符，已取消。' 5 }

$confirm = Read-Host ('确认使用登录名 “' + $loginName + '”？接下来会提示输入两次隐藏密码 (Y/n)')
if ($null -eq $confirm) { $confirm = '' }
if (([string]$confirm).Trim().ToLowerInvariant() -in @('n', 'no')) { Write-Output '已取消，未做任何修改。'; exit 0 }

Write-Output ''

& $tool create --user-email $actualEmail --username $loginName --deployment-config $configPath
$exitCode = $LASTEXITCODE

if ($exitCode -ne 0) {
    Write-Output ''
    Write-Output ('设置失败（退出码 ' + $exitCode + '）。请按上面的提示重试，或联系管理员。')
    exit 5
}

Write-Output ''
Write-Output '设置完成。'
$origin = [string]$config.PublicOrigin
if ($origin) {
    $loginUrl = $origin.TrimEnd('/') + '/login'
    Write-Output ('正在打开登录页：' + $loginUrl)
    Write-Output '在浏览器里用刚才的登录名和密码登录即可。'
    try { Start-Process $loginUrl } catch { }
}
exit 0
