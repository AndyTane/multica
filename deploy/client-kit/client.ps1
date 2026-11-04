#Requires -Version 5.1
[CmdletBinding()]
param(
    [ValidateSet('Connect','Reconnect','Status','Stop','Import','PauseUpdate','ResumeUpdate')][string]$Action = 'Reconnect',
    [string]$DataDirectory = '',
    [string]$ServerUrl,
    [string]$RootCa,
    [string]$Profile,
    [string]$SourceDirectory,
    [string]$OpenCodePath,
    [string]$OpenCodeConfig,
    [string[]]$SearchDirectory,
    [string]$WorkDirectory,
    [switch]$SkipOpenCode,
    [switch]$ReplaceSettings,
    [switch]$Interactive,
    [switch]$NonInteractive
)
$ErrorActionPreference = 'Stop'
$kitRoot = if ($DataDirectory) { [IO.Path]::GetFullPath($DataDirectory) } else { $PSScriptRoot }
$needsRecovery=$false
if (-not $DataDirectory -and $Action -in @('Connect','Reconnect')) {
    $journalPath=Join-Path $kitRoot '.managed/update.json'
    if (Test-Path -LiteralPath $journalPath) {
        $journal=Get-Content -LiteralPath $journalPath -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($journal.phase -in @('switching','verifying','interrupted')) {
            # The original executable remains available even if the candidate
            # executable is invalid or the machine lost power during switching.
            $needsRecovery=$true
        }
    }
}
if (-not $needsRecovery -and -not $DataDirectory -and (Test-Path -LiteralPath (Join-Path $kitRoot 'current.json'))) {
    $pointer = Get-Content -LiteralPath (Join-Path $kitRoot 'current.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($pointer.program -ne '.') {
        if ($pointer.program -cnotmatch '^programs/v[0-9]+\.[0-9]+\.[0-9]+-[a-f0-9]{12}-[1-3]$') { throw '客户端版本入口无效，请保留 .managed 更新记录。' }
        $next = Join-Path $kitRoot ($pointer.program+'/client.ps1')
        & $next @PSBoundParameters -DataDirectory $kitRoot
        return
    }
}
. (Join-Path $PSScriptRoot 'client-settings.ps1')
$canPrompt = $Interactive -and -not $NonInteractive

# User launchers must not inherit a task's credentials or daemon identity.
$saved = @{}
$runtimeSaved = @{}
foreach ($name in @('OPENCODE_CONFIG','OPENCODE_CONFIG_DIR','XDG_CONFIG_HOME')) { $runtimeSaved[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
foreach ($entry in Get-ChildItem Env:) {
    if ($entry.Name -match '^(MULTICA_|U_WORKERS_)') {
        $saved[$entry.Name] = $entry.Value
        Remove-Item -LiteralPath ('Env:'+$entry.Name)
    }
}
$env:U_WORKERS_KIT_ROOT = $kitRoot
function Quote-Argument([string]$Value) {
    return '"'+([regex]::Replace([regex]::Replace($Value,'(\\*)"','$1$1\"'),'(\\+)$','$1$1'))+'"'
}
function Invoke-Client([string[]]$Arguments,[switch]$Capture) {
    $argv = @(); if ($settings.profile) { $argv += @('--profile',[string]$settings.profile) }; $argv += $Arguments
    if (-not $Capture) {
        & $cli @argv
        if ($LASTEXITCODE -ne 0) { throw ('操作未完成（退出码 '+$LASTEXITCODE+'）。请检查网络和原 profile 的登录，登录过期时双击 Connect.cmd。') }
        return
    }
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName=$cli; $start.WorkingDirectory=$kitRoot; $start.UseShellExecute=$false; $start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
    $start.StandardOutputEncoding=New-Object Text.UTF8Encoding($false)
    $start.StandardErrorEncoding=New-Object Text.UTF8Encoding($false)
    $start.Arguments=(($argv | ForEach-Object { Quote-Argument $_ }) -join ' ')
    $process=New-Object Diagnostics.Process; $process.StartInfo=$start
    try {
        $null=$process.Start()
        $stdout=$process.StandardOutput.ReadToEndAsync(); $stderr=$process.StandardError.ReadToEndAsync()
        $process.WaitForExit(); $raw=$stdout.GetAwaiter().GetResult(); $null=$stderr.GetAwaiter().GetResult()
        if ($process.ExitCode -ne 0) { throw ('无法验证（退出码 '+$process.ExitCode+'）。请检查服务地址与网络，再检查原 profile 的登录；登录过期时双击 Connect.cmd。') }
        try { return ($raw | ConvertFrom-Json -ErrorAction Stop) } catch { throw '无法读取状态，请重新解压完整客户端包后重试。' }
    } finally { $process.Dispose() }
}
function Get-ClientStatus {
    $status=Invoke-Client @('daemon','status','--output','json') -Capture
    if ($status.status -notin @('running','starting','stopped')) { throw '状态无法识别，请核对客户端版本后重试。' }
    if ($status.port_conflict) { throw '端口由另一套 profile 占用，请核对原 profile；不要重复启动。' }
    if ($status.managed_client_update -and $status.managed_client_update.installation_id -and (Test-Path -LiteralPath $markerPath)) {
        $identity=Get-Content -LiteralPath $markerPath -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($status.managed_client_update.installation_id -cne $identity.id) { throw '原 profile 正由另一套客户端使用。请先用原包停止，或为独立安装选择独立 profile。' }
    }
    return $status
}
function Initialize-ClientOpenCode {
    Set-ClientOpenCode -Directory $kitRoot -Profile $settings.profile -CanPrompt $canPrompt -OpenCodePath $OpenCodePath -OpenCodeConfig $OpenCodeConfig -SearchDirectory $SearchDirectory -WorkDirectory $WorkDirectory -SkipOpenCode:$SkipOpenCode
}
try {
    if ($needsRecovery) {
        Start-Process -FilePath (Join-Path $kitRoot 'u-workers.exe') -ArgumentList @('kit-recover','--root',('"'+$kitRoot+'"')) -WorkingDirectory $kitRoot -WindowStyle Hidden | Out-Null
        Write-Host '正在恢复或确认客户端升级，请稍后用 Status.cmd 查看。'
        return
    }
    $settings = Read-ClientSettings $kitRoot
    $markerPath = Join-Path $kitRoot 'kit-install.json'
    if ($settings.serverUrl -and -not (Test-Path -LiteralPath $markerPath)) {
        $marker = [ordered]@{id=[guid]::NewGuid().ToString();profile=[string]$settings.profile;serverUrl=[string]$settings.serverUrl;paused=$false}
        [IO.File]::WriteAllText($markerPath,($marker|ConvertTo-Json),(New-Object Text.UTF8Encoding($false)))
    }
    if ($Action -in @('PauseUpdate','ResumeUpdate')) {
        if (-not (Test-Path -LiteralPath $markerPath)) { throw '请先连接或导入原客户端。' }
        $marker = Get-Content -LiteralPath $markerPath -Raw -Encoding UTF8 | ConvertFrom-Json
        $marker.paused = $Action -eq 'PauseUpdate'
        [IO.File]::WriteAllText($markerPath,($marker|ConvertTo-Json),(New-Object Text.UTF8Encoding($false)))
        Write-Host $(if ($marker.paused) { '自动升级已暂停，当前任务继续。' } else { '自动升级已恢复，下次心跳检查服务端版本。' })
        return
    }
    if ($Action -eq 'Import') {
        if (-not $SourceDirectory) {
            if (-not $canPrompt) { throw '缺少旧包目录，请传 -SourceDirectory，或双击 Import.cmd。' }
            $SourceDirectory = (Read-Host '旧客户端包目录').Trim().Trim('"')
        }
        if (-not $SourceDirectory -or -not (Test-Path -LiteralPath (Join-Path $SourceDirectory 'connection.json') -PathType Leaf)) { throw '旧包目录中找不到 connection.json。' }
        $source = [IO.Path]::GetFullPath($SourceDirectory).TrimEnd('\','/')
        if ($source -eq $kitRoot.TrimEnd('\','/')) { throw '请选择旧包目录，不能导入自身。' }
        $incoming = Read-ClientSettings $source
        $certificate = Assert-ClientSettings $incoming $source
        if (($settings.profile -or $settings.serverUrl -or $settings.rootCa) -and -not $ReplaceSettings) {
            if (-not $canPrompt) { throw '新包已有连接设置，未覆盖。确认更换请传 -ReplaceSettings。' }
            if ((Read-Host '新包已有连接设置，确认替换（Y/N，回车取消）') -ne 'Y') { Write-Host '已取消，原设置保留。'; return }
        }
        Import-ClientSettings $incoming $certificate $kitRoot
        Write-Host '连接设置已导入，仅需操作一次。原登录和旧包保留。' -ForegroundColor Green
        Write-Host '等任务空闲后，用旧包 Stop.cmd 停止，再用新包 Reconnect.cmd 重连。'
        return
    }
    # Lifecycle diagnostics remain available while a candidate exe is invalid.
    $cli=Join-Path $(if ($Action -in @('Status','Stop')) {$kitRoot} else {$PSScriptRoot}) 'u-workers.exe'
    if (-not (Test-Path -LiteralPath $cli -PathType Leaf)) { throw '缺少 u-workers.exe，请完整解压客户端包。' }
    if ($Action -eq 'Connect') {
        $candidate = [pscustomobject]@{profile=$settings.profile;serverUrl=$settings.serverUrl;rootCa=$settings.rootCa}
        foreach ($pair in @(@('ServerUrl','serverUrl'),@('RootCa','rootCa'),@('Profile','profile'))) {
            if ($PSBoundParameters.ContainsKey($pair[0])) { $candidate.($pair[1]) = [string](Get-Variable -Name $pair[0] -ValueOnly) }
        }
        if (-not $candidate.serverUrl -and $canPrompt) {
            $candidate.serverUrl = (Read-Host 'HTTPS 服务地址（由管理员提供）').Trim()
            if (-not $candidate.rootCa -and -not $PSBoundParameters.ContainsKey('RootCa')) { $candidate.rootCa = (Read-Host '公开根证书路径（公共 CA 可回车留空；不要选择私钥）').Trim().Trim('"') }
            if (-not $candidate.profile -and -not $PSBoundParameters.ContainsKey('Profile')) { $candidate.profile = (Read-Host '原 profile 名称（默认配置回车留空，决定沿用哪套登录）').Trim() }
        }
        $certificate = Assert-ClientSettings $candidate $kitRoot -RequireServer
        $changed = $candidate.serverUrl -cne $settings.serverUrl -or $candidate.rootCa -cne $settings.rootCa -or $candidate.profile -cne $settings.profile
        if ($changed -and ($settings.serverUrl -or $settings.profile -or $settings.rootCa) -and -not $ReplaceSettings) {
            if (-not $canPrompt) { throw '已有连接设置，未覆盖。确认更换请传 -ReplaceSettings。' }
            if ((Read-Host '确认更换连接设置（Y/N，回车取消）') -ne 'Y') { Write-Host '已取消，原设置保留。'; return }
        }
        $status=Get-ClientStatus
        if ($status.status -in @('running','starting')) {
            if ($OpenCodePath -or $OpenCodeConfig -or $SkipOpenCode -or $changed) { throw '原连接正在运行。请等任务空闲后用 Stop.cmd 停止，再选择程序、配置或连接设置。' }
        } else {
            $settings=$candidate
            Initialize-ClientOpenCode
        }
        if ($changed) { Save-ClientSettings $candidate $kitRoot }
        if (-not (Test-Path -LiteralPath $markerPath)) {
            $marker = [ordered]@{id=[guid]::NewGuid().ToString();profile=[string]$candidate.profile;serverUrl=[string]$candidate.serverUrl;paused=$false}
            [IO.File]::WriteAllText($markerPath,($marker|ConvertTo-Json),(New-Object Text.UTF8Encoding($false)))
        }
        $marker=Get-Content -LiteralPath $markerPath -Raw -Encoding UTF8 | ConvertFrom-Json
        $marker.profile=[string]$candidate.profile; $marker.serverUrl=[string]$candidate.serverUrl
        [IO.File]::WriteAllText($markerPath,($marker|ConvertTo-Json),(New-Object Text.UTF8Encoding($false)))
        $settings = $candidate
        $argv=@('connect','--server-url',$settings.serverUrl)
        if ($certificate) { $argv+=@('--root-ca',$certificate.Path) }
        Write-Host ('连接：'+$settings.serverUrl)
        Invoke-Client $argv
    } elseif ($Action -eq 'Stop') {
        $journalPath=Join-Path $kitRoot '.managed/update.json'
        if (Test-Path -LiteralPath $journalPath) {
            $journal=Get-Content -LiteralPath $journalPath -Raw -Encoding UTF8 | ConvertFrom-Json
            if ($journal.phase -in @('switching','verifying')) {
                [IO.File]::WriteAllText((Join-Path $kitRoot '.managed/stop.json'),(@{sha256=$journal.offer.sha256}|ConvertTo-Json),(New-Object Text.UTF8Encoding($false)))
            }
        }
        $null = Assert-ClientSettings ([pscustomobject]@{profile=$settings.profile;serverUrl='';rootCa=''}) $PSScriptRoot
        Invoke-Client @('daemon','stop')
        Write-Host '已停止。需要连接时双击 Reconnect.cmd。'
        return
    } elseif ($Action -eq 'Reconnect') {
        $null = Assert-ClientSettings $settings $kitRoot -SkipCertificate
        $status=Get-ClientStatus
        if ($status.status -notin @('running','starting')) {
            Initialize-ClientOpenCode
            Invoke-Client @('daemon','start')
        } elseif ($OpenCodePath -or $OpenCodeConfig -or $SkipOpenCode) {
            throw '原连接正在运行。请等任务空闲后用 Stop.cmd 停止，再选择程序或配置。'
        }
    }
    $null = Assert-ClientSettings $settings $kitRoot -SkipCertificate
    Write-Host ('profile：'+$(if ($settings.profile) {$settings.profile} else {'default'}))
    if ($settings.serverUrl) { Write-Host ('服务：'+$settings.serverUrl) }
    if ($Action -eq 'Status') { Show-ClientOpenCode $kitRoot $settings.profile }
    $inventory = Join-Path $PSScriptRoot 'CLIENT-FILES.json'
    if (Test-Path -LiteralPath $inventory) {
        $identity = Get-Content -LiteralPath $inventory -Raw -Encoding UTF8 | ConvertFrom-Json
        if ([string]$identity.version -match '^v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$') { Write-Host ('版本：'+$identity.version) }
    }
    $status=Get-ClientStatus
    if (-not $status.managed_client_update -and (Test-Path -LiteralPath (Join-Path $kitRoot '.managed/update.json'))) {
        $journal=Get-Content -LiteralPath (Join-Path $kitRoot '.managed/update.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        $status|Add-Member -NotePropertyName managed_client_update -NotePropertyValue ([pscustomobject]@{phase=$journal.phase;reason=$journal.reason}) -Force
    }
    if ($status.managed_client_update -and $status.managed_client_update.phase) {
        $labels=@{waiting_idle='等待任务空闲';downloading='下载并校验';switching='切换并重启';verifying='等待新版重连服务器';completed='升级成功';failed='升级失败，保留原版';rolled_back='启动失败，已恢复旧版';paused='自动升级已暂停';interrupted='已停止，等待重连恢复'}
        $phase=[string]$status.managed_client_update.phase
        Write-Host ('自动升级：'+$(if ($labels.ContainsKey($phase)) {$labels[$phase]} else {$phase}))
        if ($status.managed_client_update.reason) { Write-Host ([string]$status.managed_client_update.reason) }
    }
    if ($status.status -eq 'stopped') { Write-Host '尚未连接，双击 Reconnect.cmd；首次使用请双击 Connect.cmd。'; return }
    if ($status.status -eq 'starting') { Write-Host '连接中，请稍后双击 Status.cmd 查看。'; return }
    Write-Host '本机守护进程已运行，正在验证服务器访问...'
    # Authenticated read distinguishes server access from local process liveness.
    try { $null=Invoke-Client @('workspace','list','--output','json') -Capture }
    catch { throw '本机已运行，但服务器访问未通过。请检查服务地址与网络，再检查原 profile 的登录；登录过期时双击 Connect.cmd。' }
    Write-Host '服务器访问已验证，请在网页运行时列表确认本机在线。' -ForegroundColor Green
    if (@($status.agents).Count -eq 0) { Write-Host '尚未检测到智能体 CLI，请安装并登录后再分配任务。' -ForegroundColor Yellow }
} finally {
    Remove-Item Env:MULTICA_OPENCODE_PATH -ErrorAction SilentlyContinue
    if (-not $saved.ContainsKey('U_WORKERS_KIT_ROOT')) { Remove-Item Env:U_WORKERS_KIT_ROOT -ErrorAction SilentlyContinue }
    foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') }
    foreach ($name in $runtimeSaved.Keys) { [Environment]::SetEnvironmentVariable($name,$runtimeSaved[$name],'Process') }
}
