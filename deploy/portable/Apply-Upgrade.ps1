#Requires -Version 5.1
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Root,
    [Parameter(Mandatory=$true)][string]$PublicOrigin,
    [string]$Profile = '',
    [switch]$NoLanAccess,
    [switch]$LinkPayloadFiles,
    [switch]$CheckOnly,
    [string]$UpgradeDirectory = $PSScriptRoot
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'oss-release-common.ps1')
if ([string]::IsNullOrWhiteSpace($UpgradeDirectory)) { throw 'UpgradeDirectory 不能为空或空白，请指定解压后的差量包目录。' }
# The native lifecycle calls tar.exe. Prefer Windows bsdtar, whose drive-letter
# paths work; Git's GNU tar can interpret D:\... as a remote host.
$env:PATH = (Join-Path $env:SystemRoot 'System32')+';'+$env:PATH
if (-not (Test-Path -LiteralPath (Join-Path $env:SystemRoot 'System32\tar.exe') -PathType Leaf)) { throw 'Windows 自带 System32\tar.exe 缺失，请先修复 Windows 压缩工具。' }
$rootPath = [IO.Path]::GetFullPath($Root).TrimEnd('\')
$sourcePath = [IO.Path]::GetFullPath($UpgradeDirectory).TrimEnd('\')
Assert-ReleaseSeparateDirectory $rootPath $sourcePath
Resolve-ReleasePath $rootPath 'deployment.json' | Out-Null
$uri = [Uri]$PublicOrigin
if ($uri.Scheme -ne 'https' -or $uri.AbsolutePath -ne '/' -or $uri.Query -or $uri.Fragment -or $uri.UserInfo) { throw 'PublicOrigin must be an HTTPS origin with no path, query or credentials.' }
$PublicOrigin = $uri.GetLeftPart([UriPartial]::Authority)
$plan = Get-ReleasePlan $sourcePath
Assert-ReleaseDirectory $sourcePath $plan
$cfgPath = Join-Path $rootPath 'deployment.json'
$cfg = Get-Content -Raw -LiteralPath $cfgPath -Encoding UTF8 | ConvertFrom-Json
if ($cfg.Version -ne $plan.from) { throw ('This delta requires '+$plan.from+'. Download an upgrade matching the installed version; no files were changed.') }
$baseManifest = Get-Content -Raw -LiteralPath (Join-Path $rootPath 'manifest.json') -Encoding UTF8 | ConvertFrom-Json
$baselineCommit = [string]$baseManifest.commit
Assert-ReleaseCommit $baselineCommit
$exactBaseline = $baselineCommit.StartsWith([string]$plan.baselineCommit)
$known119 = $plan.from -eq 'v0.2.1' -and $plan.to -eq 'v0.2.2' -and $plan.commit -eq '3e62bb6ebb9393210a7ed44fbad021611e9bc7ff' -and $plan.baselineCommit -eq 'cfc93fef6' -and $baselineCommit.StartsWith('775029717')
if (-not ($exactBaseline -or $known119) -or $baseManifest.version -ne $plan.from) { throw 'Unrecognised baseline build. Fetch a matching upgrade; no installation files were changed.' }
if (-not $Profile) {
    $report = Get-Content -Raw -LiteralPath (Join-Path $rootPath 'data\install-report.json') -Encoding UTF8 | ConvertFrom-Json
    $Profile = [string]$report.profile
}
if (-not $Profile) { throw 'Pass the installation-specific -Profile.' }

function Resolve-DeltaPath([string]$Parent, [string]$Relative) {
    return (Resolve-ReleasePath $Parent $Relative)
}
function Assert-Hash([string]$Path, [string]$Hash) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw ('Missing file: '+$Path) }
    if ((Get-Item -LiteralPath $Path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw ('Unexpected file link: '+$Path) }
    if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -ne $Hash) { throw ('Checksum mismatch: '+$Path) }
}
function Sync-UpgradeCaddyRecord([string]$Installation, [int]$Port) {
    # Some hosts return no executable path immediately after Start-Process.
    # Once the listener is ready, pin its actual PID/path/start time so Stop.cmd
    # and rollback can still stop precisely this installation's HTTPS process.
    $owners = @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction Stop | Select-Object -ExpandProperty OwningProcess -Unique)
    if ($owners.Count -ne 1) { throw 'Cannot identify the installation HTTPS listener.' }
    $owner = Get-CimInstance Win32_Process -Filter ('ProcessId='+$owners[0])
    $expected = [IO.Path]::GetFullPath((Get-PortableCaddyPath $Installation))
    $config = Get-PortableCaddyfilePath $Installation
    if (-not [string]::Equals([string]$owner.ExecutablePath,$expected,[StringComparison]::OrdinalIgnoreCase) -or ([string]$owner.CommandLine).IndexOf($config,[StringComparison]::OrdinalIgnoreCase) -lt 0) { throw 'HTTPS process ownership does not match this installation.' }
    $record = Get-PortableProcessRecord -ProcessId $owners[0] -ExpectedPath $expected
    if (-not $record -or -not (Test-PortableProcessRecord $record)) { throw 'Cannot verify the HTTPS process lifetime.' }
    Write-PortableJson (Join-Path $Installation 'state\caddy.json') $record
    return $record
}
Write-Host 'Checking the delta and the installed release before changing files...'
$progress=New-ReleaseProgress (@($plan.write).Count+@($plan.delete).Count+@($plan.keep).Count) '校验安装与差量文件'
foreach ($entry in $plan.write) {
    Assert-Hash (Resolve-DeltaPath $sourcePath $entry.path) $entry.sha256
    $installed = Resolve-DeltaPath $rootPath $entry.path
    if ($exactBaseline -and $entry.old_sha256) { Assert-Hash $installed $entry.old_sha256 }
    if ($exactBaseline -and $entry.action -eq 'add' -and (Test-Path -LiteralPath $installed)) { throw 'An added program file already exists. Inspect the baseline before upgrading.' }
    Step-ReleaseProgress $progress
}
foreach ($entry in $plan.delete) {
    $installed = Resolve-DeltaPath $rootPath $entry.path
    if ($exactBaseline -and (Test-Path -LiteralPath $installed)) { Assert-Hash $installed $entry.sha256 }
    Step-ReleaseProgress $progress
}
# Unchanged files must exist; PostgreSQL and Node were moved into runtime by Setup.
foreach ($entry in $plan.keep) {
    $relative = [string]$entry.path
    if ($relative.StartsWith('payload/runtime/pgsql/')) { $relative = $relative.Substring(8) }
    if ($relative -eq 'payload/runtime/node.exe') { $relative = 'runtime/node/node.exe' }
    Assert-Hash (Resolve-DeltaPath $rootPath $relative) $entry.sha256
    Step-ReleaseProgress $progress
}
# Different baseline builds can have different hashed Next.js chunk filenames.
# Remove obsolete generated release files according to the COMPLETE target set,
# never by mirroring the partial delta. User data and old staged releases are
# outside these two immutable payload directories.
$expected = @{}
foreach ($entry in @($plan.write)+@($plan.keep)) { $expected[[string]$entry.path] = $true }
$obsolete = New-Object System.Collections.Generic.List[object]
foreach ($relativeRoot in @('payload/release','payload/installer')) {
    $managed = Resolve-DeltaPath $rootPath $relativeRoot
    foreach ($item in @(Get-ReleaseFiles $managed)) {
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Unexpected junction in immutable payload; stop and inspect.' }
        if ($item.PSIsContainer) { continue }
        $relative = $item.FullName.Substring($rootPath.Length+1).Replace('\','/')
        if (-not $expected.ContainsKey($relative)) {
            Assert-ReleaseProgramPath $relative
            $obsolete.Add([pscustomobject]@{path=$relative;sha256=(Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash})
        }
    }
}
# Source artifacts are removed only when named in the validated delete plan.
# Generated release/installer deletions above already cover those two roots.
foreach ($entry in @($plan.delete | Where-Object { $_.path -like 'payload/source/*' })) {
    $oldSource = Resolve-DeltaPath $rootPath $entry.path
    if (Test-Path -LiteralPath $oldSource) { $obsolete.Add([pscustomobject]@{path=[string]$entry.path;sha256=(Get-FileHash -LiteralPath $oldSource -Algorithm SHA256).Hash}) }
}
$plan.delete = @($obsolete.ToArray())
Write-Host ('Validated build '+$baselineCommit+': '+$plan.write.Count+' files to write, '+$plan.delete.Count+' obsolete generated files to move to backup.')
if ($CheckOnly) { Write-Host 'Read-only preflight passed. No service or installation file was changed.'; return }
. (Join-Path $rootPath 'portable-common.ps1')
$status = Get-PortableDaemonStatus -Root $rootPath -Profile $Profile
Assert-ReleaseDaemonStopped $status
$originalReportPath = Join-Path $rootPath 'data\install-report.json'
$originalReport = if (Test-Path -LiteralPath $originalReportPath) { Read-ReleaseJson $originalReportPath } else { $null }
$finalLanExposed = -not $NoLanAccess
$scopeProperty = if ($originalReport) { $originalReport.PSObject.Properties['lanExposed'] } else { $null }
if (-not $PSBoundParameters.ContainsKey('NoLanAccess') -and $scopeProperty -and $scopeProperty.Value -is [bool]) { $finalLanExposed = $scopeProperty.Value }
if (Test-PortableLoopbackHost $uri.Host) { $finalLanExposed = $false }
$requestedProperty = if ($originalReport) { $originalReport.PSObject.Properties['lanRequestedHost'] } else { $null }
$requestedHost = if ($requestedProperty -and $requestedProperty.Value) { [string]$requestedProperty.Value } elseif ($finalLanExposed) { $uri.Host } else { '127.0.0.1' }
if ($PSBoundParameters.ContainsKey('NoLanAccess')) { $requestedHost = if ($NoLanAccess) { '127.0.0.1' } else { $uri.Host } }
$finalHost = if ($finalLanExposed) { $uri.Host } else { '127.0.0.1' }
$finalOrigin = if ($finalLanExposed -or (Test-PortableLoopbackHost $uri.Host)) { $PublicOrigin } else { 'https://localhost:'+$uri.Port }

# Back up while the old database is available, then stop only this installation.
Invoke-PortableNativeAction -Root $rootPath -Action 'Start' | Out-Null
$previousBackups = @{}
if (Test-Path -LiteralPath (Join-Path $rootPath 'backups')) {
    foreach ($item in Get-ChildItem -LiteralPath (Join-Path $rootPath 'backups') -Directory) { $previousBackups[$item.FullName] = $true }
}
Invoke-PortableNativeAction -Root $rootPath -Action 'Backup' | Out-Null
$databaseBackup = Get-ChildItem -LiteralPath (Join-Path $rootPath 'backups') -Directory | Where-Object { -not $previousBackups.ContainsKey($_.FullName) -and (Test-Path -LiteralPath (Join-Path $_.FullName 'database.dump')) } | Sort-Object LastWriteTime -Descending | Select-Object -First 1
if (-not $databaseBackup) { throw 'Database backup was not created; refusing to replace files.' }
Stop-PortableCaddy -Root $rootPath | Out-Null
Invoke-PortableNativeAction -Root $rootPath -Action 'Stop' | Out-Null
$backup = Join-Path $rootPath ('backups\upgrade-'+$plan.to+'-'+(Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+[guid]::NewGuid().ToString('N').Substring(0,8))
New-Item -ItemType Directory -Path $backup | Out-Null
Copy-Item -LiteralPath $cfgPath -Destination (Join-Path $backup 'deployment.json')
if ($originalReport) { Copy-Item -LiteralPath $originalReportPath -Destination (Join-Path $backup 'install-report.json') }
$journal = [ordered]@{ root=$rootPath; databaseBackup=$databaseBackup.FullName; version=$plan.to; from=$plan.from; profile=$Profile; files=@() }
$journalPath = Join-Path $backup 'files.json'
try {
    foreach ($entry in @($plan.write) + @($plan.delete)) {
        $destination = Resolve-DeltaPath $rootPath $entry.path
        $old = Join-Path $backup ('files\'+$entry.path)
        $existed = Test-Path -LiteralPath $destination
        # Record intent before the move, so an interrupted copy can be rolled back.
        $journal.files += [pscustomobject]@{path=$entry.path; existed=[bool]$existed}
        [IO.File]::WriteAllText($journalPath,($journal|ConvertTo-Json -Depth 8),(New-Object Text.UTF8Encoding($false)))
        if ($existed) {
            New-Item -ItemType Directory -Path (Split-Path -Parent $old) -Force | Out-Null
            Move-Item -LiteralPath $destination -Destination $old
        }
        if ($entry.PSObject.Properties.Name -contains 'action') {
            New-Item -ItemType Directory -Path (Split-Path -Parent $destination) -Force | Out-Null
            $from = Resolve-DeltaPath $sourcePath $entry.path
            if ($LinkPayloadFiles) {
                if ([IO.Path]::GetPathRoot($from) -ne [IO.Path]::GetPathRoot($destination)) { throw 'Hardlinks require the delta and installation on the same NTFS volume.' }
                New-Item -ItemType HardLink -Path $destination -Target $from | Out-Null
            } else {
                Copy-Item -LiteralPath $from -Destination $destination
            }
        }
    }
    $native = Join-Path $rootPath 'runtime\native-windows.ps1'
    $shell = (Get-Process -Id $PID).Path
    & $shell -NoProfile -ExecutionPolicy Bypass -File $native -Action Configure -Root $rootPath -PublicOrigin $finalOrigin -CorsOrigins ($finalOrigin+',https://localhost:'+$uri.Port+',https://127.0.0.1:'+$uri.Port) | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Native configuration failed; see the rollback checkpoint.' }
    # The stock upgrader expects a FULL source tree with -Source. The delta has
    # already been overlaid, so deliberately omit -Source (its /MIR would delete
    # unchanged dependencies from a partial source).
    # Always suppress the stock upgrader's firewall-writing path. Restore the
    # requested listener explicitly below; no firewall/trust/startup API is used.
    $intermediateReportPath = Join-Path $backup 'native-upgrade-report.json'
    & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $rootPath 'upgrade.ps1') -Profile $Profile -NoLanAccess -SkipDaemon -SkipBrowserTrust -NoStartup -NoBrowser -ReportPath $intermediateReportPath
    if ($LASTEXITCODE -ne 0) { throw 'Native upgrade failed; see the rollback checkpoint.' }
    . (Join-Path $rootPath 'portable-common.ps1')
    Sync-UpgradeCaddyRecord -Installation $rootPath -Port $uri.Port | Out-Null
    Stop-PortableCaddy -Root $rootPath | Out-Null
    $now = Get-Content -Raw -LiteralPath $cfgPath -Encoding UTF8 | ConvertFrom-Json
    Start-PortableCaddy -Root $rootPath -PublicHost $finalHost -HttpsPort $uri.Port -ApiPort ([int]$now.ApiPort) -WebPort ([int]$now.WebPort) -LanExposed:$finalLanExposed | Out-Null
    Sync-UpgradeCaddyRecord -Installation $rootPath -Port $uri.Port | Out-Null
    Wait-PortableHttp -Uri ($finalOrigin+'/readyz') -RootCAPath (Get-PortablePublicRootCAPath $rootPath) -TimeoutSec 120 -Description 'the restored HTTPS listener' | Out-Null
    $now = Get-Content -Raw -LiteralPath $cfgPath -Encoding UTF8 | ConvertFrom-Json
    if ($now.Version -ne $plan.to) { throw 'Upgrade did not record the target version.' }
    foreach ($entry in $plan.write) { Assert-Hash (Resolve-DeltaPath $rootPath $entry.path) $entry.sha256 }
    $daemonStartCommand = Get-PortableDaemonStartCommand -Root $rootPath -Profile $Profile
    # Publish final reports only after the listener and all written hashes pass.
    # The inner upgrader writes its temporary report into the checkpoint instead.
    $finalFields = [ordered]@{version=$plan.to;boardUrl=$finalOrigin;httpsPort=$uri.Port;profile=$Profile;lanRequestedHost=$requestedHost;lanHost=$finalHost;lanExposed=[bool]$finalLanExposed;lanMessage='最终 HTTPS 监听已就绪；跨机访问待验收，未修改防火墙。'}
    if (-not $finalLanExposed) { $finalFields.lanMessage='仅本机 HTTPS 监听已就绪，未修改防火墙。' }
    $upgradeReport = Read-ReleaseJson $intermediateReportPath
    if (-not $originalReport) { $originalReport=[pscustomobject]@{} }
    if ($originalReport) {
        foreach ($key in $finalFields.Keys) { $originalReport | Add-Member -NotePropertyName $key -NotePropertyValue $finalFields[$key] -Force }
        if ($NoLanAccess) {
            $originalReport | Add-Member -NotePropertyName lanFirewall -NotePropertyValue 'loopback' -Force
            $originalReport | Add-Member -NotePropertyName lanFirewallRule -NotePropertyValue '' -Force
        }
        Write-PortableJson $originalReportPath $originalReport
    }
    $upgradeReportPath = Join-Path $rootPath 'data\upgrade-report.json'
    foreach ($key in $finalFields.Keys) { $upgradeReport | Add-Member -NotePropertyName $key -NotePropertyValue $finalFields[$key] -Force }
    Write-PortableJson $upgradeReportPath $upgradeReport
    Write-Host ('程序升级完成，原服务地址已就绪。恢复备份：'+$backup)
    Write-Host '原有权限保留。请完成业务验收后再启动原安装的守护进程。'
    Write-Host $daemonStartCommand
} catch {
    Write-Host ('升级未完成，请保留备份：'+$backup)
    Write-Host ('恢复说明：'+(Join-Path $sourcePath 'UPGRADE.md'))
    throw
}
