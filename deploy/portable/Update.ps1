#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Root = '',
    [string]$Version = 'latest',
    [string]$Ossutil = '',
    [string]$DestinationDirectory = '',
    [string]$Profile = '',
    [string]$PublicOrigin = '',
    [switch]$NoLanAccess,
    [switch]$LanAccess,
    [switch]$CheckOnly,
    [switch]$ConfirmUpgrade,
    [switch]$Interactive,
    [switch]$NonInteractive
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'oss-release-common.ps1')
$canPrompt = $Interactive -and -not $NonInteractive
$stage = ''
$installation = ''
$oldBackups = @{}
function Ask-Update([string]$Prompt) {
    if (-not $canPrompt) { throw ('缺少设置：'+$Prompt+'。请传入参数，或双击 Update.cmd。') }
    return (Read-Host $Prompt).Trim().Trim('"')
}
function Get-UpdateOrigin([string]$Value) {
    $uri = $null
    if (-not [Uri]::TryCreate($Value,[UriKind]::Absolute,[ref]$uri) -or $uri.Scheme -ne 'https' -or $uri.AbsolutePath -ne '/' -or $uri.UserInfo -or $uri.Query -or $uri.Fragment) { throw '服务地址须为 HTTPS 地址，不含路径、账号或查询参数。' }
    return $uri.GetLeftPart([UriPartial]::Authority)
}
try {
    if (-not $Root) {
        if (Test-Path -LiteralPath (Join-Path $PSScriptRoot 'deployment.json') -PathType Leaf) { $Root = $PSScriptRoot }
        else { $Root = Ask-Update '原安装目录（例如 C:\UWorkers）' }
    }
    $installation = [IO.Path]::GetFullPath($Root).TrimEnd('\','/')
    $cfg = Read-ReleaseJson (Resolve-ReleasePath $installation 'deployment.json')
    Assert-ReleaseVersion ([string]$cfg.Version)
    $reportPath = Resolve-ReleasePath $installation 'data/install-report.json'
    $report = if (Test-Path -LiteralPath $reportPath -PathType Leaf) { Read-ReleaseJson $reportPath } else { $null }
    if ($PublicOrigin) { $PublicOrigin = Get-UpdateOrigin $PublicOrigin }
    if ($cfg.PublicOrigin) {
        $originalOrigin = Get-UpdateOrigin ([string]$cfg.PublicOrigin)
        if ($PublicOrigin -and $PublicOrigin -ne $originalOrigin) { throw '服务地址与原安装配置冲突。请核对安装目录，升级不会更换服务地址。' }
        $PublicOrigin = $originalOrigin
    }
    if (-not $PublicOrigin) { $PublicOrigin = Get-UpdateOrigin (Ask-Update '原 HTTPS 服务地址（-PublicOrigin）') }
    if ($report.boardUrl -and (Get-UpdateOrigin ([string]$report.boardUrl)) -ne $PublicOrigin) { throw '安装记录与服务地址冲突。请核对该安装的配置后重试。' }
    if ($Profile -and $report.profile -and $Profile -cne $report.profile) { throw '所选 profile 与原安装记录冲突。请使用原 profile。' }
    if (-not $Profile) { $Profile = [string]$report.profile }
    if (-not $Profile) { $Profile = Ask-Update '原安装的 profile（-Profile）' }
    if (-not $Profile -or $Profile -match '[\\/:\x00-\x1f<>"|?*]' -or $Profile -match '^[. ]|[. ]$') { throw 'profile 无效。请填写原安装使用的名称。' }
    if ($LanAccess -and $NoLanAccess) { throw '不能同时指定 LanAccess 和 NoLanAccess。' }
    $lanProperty = if ($report) { $report.PSObject.Properties['lanExposed'] } else { $null }
    if ($lanProperty -and $lanProperty.Value -is [bool]) {
        if (($LanAccess -and -not $lanProperty.Value) -or ($NoLanAccess -and $lanProperty.Value)) { throw '监听设置与原安装记录冲突。升级不会更换访问范围。' }
        $NoLanAccess = -not $lanProperty.Value
    } elseif (-not ($LanAccess -or $NoLanAccess)) {
        $answer = Ask-Update '原安装允许其他电脑访问吗（Y/N；自动化请传 -LanAccess 或 -NoLanAccess）'
        if ($answer -notmatch '^(?i:y|n)$') { throw '请输入 Y 或 N，核对原来的访问范围后重试。' }
        $NoLanAccess = $answer -eq 'n'
    }
    if (-not $Ossutil) {
        $localTool = Join-Path $PSScriptRoot 'ossutil.exe'
        if (Test-Path -LiteralPath $localTool -PathType Leaf) { $Ossutil = $localTool }
        elseif (Get-Command ossutil.exe -ErrorAction SilentlyContinue) { $Ossutil = (Get-Command ossutil.exe).Source }
        else { $Ossutil = Ask-Update '已有 ossutil.exe 的完整路径（-Ossutil）' }
    }
    if (-not $DestinationDirectory) { $DestinationDirectory = Join-Path (Split-Path -Parent $installation) ('UWorkers-Update-'+[guid]::NewGuid().ToString('N')) }
    $stage = [IO.Path]::GetFullPath($DestinationDirectory)
    Assert-ReleaseSeparateDirectory $stage $installation
    $backupRoot = Resolve-ReleasePath $installation 'backups'
    if (Test-Path -LiteralPath $backupRoot) { foreach ($item in Get-ChildItem -LiteralPath $backupRoot -Directory) { $oldBackups[$item.FullName] = $true } }
    Write-Host '正在下载并校验升级包...'
    $payload = & (Join-Path $PSScriptRoot 'Fetch-Upgrade.ps1') -Version $Version -Ossutil $Ossutil -DestinationDirectory $stage -Root $installation 6>$null
    $release = Read-ReleaseJson (Join-Path $stage 'release.json')
    Write-Host ('版本：'+$cfg.Version+' → '+$release.to)
    Write-Host ('服务：'+$PublicOrigin)
    Write-Host ('安装：'+$installation)
    Write-Host ('下载：'+$stage)
    if ($release.to -ceq $cfg.Version) { Write-Host '已是该版本，无需升级。'; return }
    if ([version]($release.to -replace '^v|-.*$','') -le [version]($cfg.Version -replace '^v|-.*$','')) { throw '目标版本不高于当前版本，不能降级。' }
    if ($release.packageType -eq 'full') { Write-Host ('完整包已校验。请按 '+(Join-Path $payload 'UPGRADE.md')+' 备份并升级。'); return }
    if ($release.from -cne $cfg.Version) { throw ('此差量包需要 '+$release.from+'，当前为 '+$cfg.Version+'。请逐版升级，或按完整包说明对齐基线。') }
    $applyArgs = @{Root=$installation;Profile=$Profile;PublicOrigin=$PublicOrigin;UpgradeDirectory=$payload}
    # An existing record retains requested intent separately from current exposure.
    if ($PSBoundParameters.ContainsKey('NoLanAccess') -or $LanAccess -or -not $lanProperty) { $applyArgs.NoLanAccess=[bool]$NoLanAccess }
    & (Join-Path $PSScriptRoot 'Apply-Upgrade.ps1') @applyArgs -CheckOnly | Out-Null
    # Query only the selected installation; an unknown status must not authorize replacement.
    . (Join-Path $installation 'portable-common.ps1')
    $daemon = Get-PortableDaemonStatus -Root $installation -Profile $Profile
    Assert-ReleaseDaemonStopped $daemon
    Write-Host '下载、校验和只读检查完成。'
    if ($CheckOnly) { Write-Host '仅检查，未修改安装。'; return }
    if (-not $ConfirmUpgrade) {
        if (-not $canPrompt) { throw '检查通过。确认升级请传 -ConfirmUpgrade；仅检查请传 -CheckOnly。' }
        if ((Read-Host '现在升级（Y/N，回车取消）') -ne 'Y') { Write-Host '已取消，未修改安装。'; return }
    }
    & (Join-Path $PSScriptRoot 'Apply-Upgrade.ps1') @applyArgs 6>$null | Out-Null
    Write-Host '程序升级完成，服务已就绪。原权限保留。' -ForegroundColor Green
    Write-Host '请检查原登录、项目、任务、旧附件及另一台电脑访问；通过后再启动原守护进程。'
    Write-Host (Get-PortableDaemonStartCommand -Root $installation -Profile $Profile)
} catch {
    $reason = $_.Exception.Message
    if ($reason -notmatch '[\u4e00-\u9fff]') {
        if ($reason -match 'SHA-256|[Cc]hecksum|size mismatch') { $reason = '文件校验失败，请重新下载；未校验的包不能升级。' }
        elseif ($reason -match 'OSS operation|ossutil|environment variable') { $reason = '下载未完成，请检查 ossutil 2、OSS 环境变量、读取权限和网络。' }
        elseif ($reason -match 'baseline|delta requires|Unrecognised') { $reason = '安装基线与升级包不符，请下载匹配的差量包或按完整包说明操作。' }
        elseif ($reason -match 'Windows archive tool|System32\\tar.exe') { $reason = 'Windows 自带 tar.exe 缺失。请先修复 Windows 压缩工具，再重试升级。' }
        else { $reason = '检查或升级未完成。请核对安装目录、原配置和升级包；保留下面的恢复资料。' }
    }
    Write-Host ('升级未完成：'+$reason) -ForegroundColor Yellow
    if ($stage -and (Test-Path -LiteralPath $stage)) {
        Write-Host ('下载与说明：'+$stage)
        Write-Host ('恢复说明：'+(Join-Path $stage 'upgrade/UPGRADE.md'))
    }
    if ($installation -and (Test-Path -LiteralPath (Join-Path $installation 'backups'))) {
        foreach ($item in Get-ChildItem -LiteralPath (Join-Path $installation 'backups') -Directory) {
            if (-not $oldBackups.ContainsKey($item.FullName)) { Write-Host ('保留备份：'+$item.FullName) }
        }
        Write-Host ('备份目录：'+(Join-Path $installation 'backups'))
    }
    throw $reason
}
