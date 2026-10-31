# Set a local U Workers login (account + password) on this host.
#
# Run it from an interactive desktop window (the shortcut "设置 U Workers 登录"
# does that). It binds a local account/password credential to an EXISTING user of
# the native deployment; it cannot create users and never prints credentials.
#
# The password is typed twice with no echo by local-auth.exe; it is never stored
# in a file, passed on a command line or written to a log.

[CmdletBinding()]
param(
    [string]$Root = ''
)

$ErrorActionPreference = 'Stop'
if (-not $Root) { $Root = Join-Path $PSScriptRoot 'native' }

function Read-Deployment {
    $configPath = Join-Path $Root 'deployment.json'
    if (-not (Test-Path -LiteralPath $configPath)) {
        throw ('找不到部署配置：' + $configPath + '。请把本脚本放在 D:\UWorkers\clawsta80 目录。')
    }
    return (Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json)
}

function Invoke-LocalSql {
    param($Config, [string]$Database, [string]$Query)
    $psql = Join-Path $Root 'runtime\pgsql\bin\psql.exe'
    $saved = $env:PGPASSWORD
    try {
        $env:PGPASSWORD = [string]$Config.DatabasePassword
        # Feed the statement through psql's stdin: Windows PowerShell 5.1 strips
        # embedded double quotes from native command arguments, and these
        # statements reference the reserved table name "user".
        $output = $Query | & $psql -h 127.0.0.1 -p ([int]$Config.DatabasePort) -U $Config.DatabaseUser -d $Database -v ON_ERROR_STOP=1 -tA -f - 2>&1
        if ($LASTEXITCODE -ne 0) { throw ('查询账号失败：' + ($output -join ' ')) }
        return @($output | Where-Object { $_ -ne $null -and [string]$_ -ne '' })
    } finally {
        $env:PGPASSWORD = $saved
    }
}

function Wait-ForEnter {
    Write-Host ''
    Write-Host '按回车关闭此窗口。'
    [void](Read-Host)
}

try {
    $config = Read-Deployment
    $release = [string]$config.Release
    $localAuth = Join-Path $release 'local-auth.exe'
    if (-not (Test-Path -LiteralPath $localAuth)) {
        throw ('当前 release 里没有 local-auth.exe：' + $release + '。请让管理员重新构建部署。')
    }
    $configPath = Join-Path $Root 'deployment.json'

    Write-Host ''
    Write-Host '=========================================='
    Write-Host '  设置 U Workers 登录（本机账号密码）'
    Write-Host '=========================================='
    Write-Host ''

    $users = @(Invoke-LocalSql -Config $config -Database $config.Database -Query ('SELECT id::text || ''|'' || email || ''|'' || name FROM "user" ORDER BY created_at;'))
    if ($users.Count -eq 0) {
        Write-Host '这台部署上还没有任何用户账号，因此暂时没有可以绑定密码的账号。'
        Write-Host '账号创建不在本工具范围内，请联系平台负责人处理后再运行本入口。'
        Wait-ForEnter
        exit 2
    }

    $selected = $null
    if ($users.Count -eq 1) {
        $parts = ([string]$users[0]).Split('|')
        $selected = @{ Id = $parts[0]; Email = $parts[1]; Name = $parts[2] }
        Write-Host ('将为此账号设置登录：' + $selected.Name + ' <' + $selected.Email + '>')
        $answer = Read-Host '确认？(Y/n)'
        if ($answer -and $answer.Trim().ToLowerInvariant() -ne 'y') {
            Write-Host '已取消，未做任何修改。'
            Wait-ForEnter
            exit 0
        }
    } else {
        Write-Host '这台部署上有多个账号，请选择要为哪一个设置登录：'
        $candidates = @()
        for ($i = 0; $i -lt $users.Count; $i++) {
            $parts = ([string]$users[$i]).Split('|')
            $candidates += @{ Id = $parts[0]; Email = $parts[1]; Name = $parts[2] }
            Write-Host ('  [' + ($i + 1) + '] ' + $parts[2] + ' <' + $parts[1] + '>')
        }
        $choice = Read-Host ('输入序号 (1-' + $candidates.Count + ')')
        $index = 0
        if (-not [int]::TryParse([string]$choice, [ref]$index) -or $index -lt 1 -or $index -gt $candidates.Count) {
            Write-Host '序号无效，已取消。'
            Wait-ForEnter
            exit 1
        }
        $selected = $candidates[$index - 1]
    }

    $hasTable = @(Invoke-LocalSql -Config $config -Database $config.Database -Query ("SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='local_password_credential';"))
    if ([string]$hasTable[0] -eq '1') {
        $existing = @(Invoke-LocalSql -Config $config -Database $config.Database -Query ("SELECT username_normalized FROM local_password_credential WHERE user_id = '" + $selected.Id + "';"))
        if ($existing.Count -gt 0) {
            Write-Host ''
            Write-Host ('该账号已设置过本地密码（账号名：' + [string]$existing[0] + '），本入口不会覆盖它。')
            Write-Host '如需重置密码，请联系平台负责人在本机执行 local-auth reset。'
            Wait-ForEnter
            exit 3
        }
    }

    Write-Host ''
    $username = Read-Host '登录账号名（3-64 个字符，字母/数字/._-，不区分大小写）'
    if (-not $username -or -not $username.Trim()) {
        Write-Host '账号名不能为空，已取消。'
        Wait-ForEnter
        exit 1
    }

    Write-Host ''
    Write-Host '接下来输入两次密码（输入时不显示）。密码至少 12 个字符、最多 72 字节。'
    & $localAuth create --user-email $selected.Email --username $username.Trim() --deployment-config $configPath
    $exitCode = $LASTEXITCODE
    Write-Host ''
    if ($exitCode -eq 0) {
        Write-Host '设置完成。现在可以用这个账号密码登录：'
        Write-Host ('  https://192.168.3.49:13443/login')
    } else {
        Write-Host ('设置未完成（local-auth 退出码 ' + $exitCode + '）。上面的错误信息说明了原因；未写入任何密码。')
    }
    Wait-ForEnter
    exit $exitCode
} catch {
    Write-Host ''
    Write-Host ('出错了：' + $_.Exception.Message)
    Wait-ForEnter
    exit 1
}
