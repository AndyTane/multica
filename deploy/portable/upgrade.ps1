#Requires -Version 5.1
<#
.SYNOPSIS
    Upgrades an installed U Workers portable server in place.

.DESCRIPTION
    Backs up the database and uploads first, then stages the prebuilt release
    from the new package and restarts. Existing data, accounts and workspaces are
    preserved; a failed backup aborts before anything is replaced.

    Two ways to use it:
      * Copy the new package contents over the old folder, then run Upgrade.cmd.
      * Run Upgrade.cmd from the old folder with -Source <new package folder>.

    PostgreSQL binaries are never replaced by an overlay upgrade; moving a
    database cluster between PostgreSQL major versions is a separate, explicit
    operation.
#>
[CmdletBinding()]
param(
    [string]$Source,
    # A verified full package can upgrade an older installation without first
    # replacing its upgrader or relying on its historical overlay file list.
    [string]$Root = '',
    [string]$PublicHost,
    [int]$HttpsPort = 0,
    [string]$Profile = '',
    [switch]$SkipBrowserTrust,
    [switch]$SkipDaemon,
    [switch]$NoBrowser,
    [switch]$NoStartup,
    [switch]$NoLanAccess,
    [switch]$Force,
    [string]$ClientReleaseDirectory = '',
    # Apply uses a checkpoint report until its final listener passes validation.
    [string]$ReportPath = ''
)

$ErrorActionPreference = 'Stop'
$externalRoot = [bool]$Root
$root = if ($Root) { [IO.Path]::GetFullPath($Root) } else { $PSScriptRoot }
if ($Source -and (Test-Path -LiteralPath (Join-Path $Source 'UPGRADE-FILES.json'))) {
    throw 'Sparse differential package detected. Run Apply-Upgrade.ps1 from that package; Upgrade -Source requires a complete package.'
}
. (Join-Path $PSScriptRoot 'portable-common.ps1')
if ($externalRoot) {
    if (-not $Source) { throw '-Root requires a verified complete -Source package.' }
    $sourceRoot = [IO.Path]::GetFullPath($Source).TrimEnd('\','/')
    if (-not [string]::Equals($sourceRoot, $PSScriptRoot.TrimEnd('\','/'), [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Run upgrade.ps1 from the full Source package when using -Root.'
    }
    . (Join-Path $PSScriptRoot 'oss-release-common.ps1')
    Assert-ReleaseSeparateDirectory $root $sourceRoot
    $full = Read-ReleaseJson (Join-Path $sourceRoot 'FULL-FILES.json')
    if (-not $full) { throw '-Root requires FULL-FILES.json from the complete release.' }
    Assert-FullReleaseDirectory $sourceRoot $full.version $full.commit
    $null = Get-PortableDeployment $root
    foreach ($entry in @($full.files)) {
        if ($entry.path.StartsWith('payload/runtime/pgsql/', [StringComparison]::Ordinal)) {
            $installedPath = Join-Path $root ('runtime/pgsql/'+$entry.path.Substring('payload/runtime/pgsql/'.Length))
            Assert-ReleaseFile $installedPath $entry.sha256 ([long]$entry.bytes)
        }
    }
}

function Copy-PortableOverlay {
    param([string]$SourceRoot)
    $directories = @('payload\release', 'payload\bin', 'payload\runtime\caddy', 'payload\runtime\opencode', 'payload\scripts', 'payload\installer', 'payload\source', 'tools')
    foreach ($relative in $directories) {
        $from = Join-Path $SourceRoot $relative
        if (-not (Test-Path -LiteralPath $from)) { continue }
        $to = Join-Path $root $relative
        New-PortableDirectory $to
        $copy = Invoke-PortableCommand -Executable 'robocopy.exe' -Arguments @($from, $to, '/MIR', '/SJ', '/SL', '/R:2', '/W:1', '/MT:4', '/NP', '/NFL', '/NDL', '/NJH', '/NJS')
        if ($copy.ExitCode -ge 8) { throw ('Copying ' + $relative + ' from the new package failed (robocopy exit ' + $copy.ExitCode + ').') }
    }
    $files = @('manifest.json', 'install.ps1', 'start.ps1', 'stop.ps1', 'status.ps1', 'uninstall.ps1', 'upgrade.ps1', 'trust-browser.ps1', 'portable-common.ps1', 'build-installer.ps1', 'Setup.cmd', 'Start.cmd', 'Stop.cmd', 'Status.cmd', 'Uninstall.cmd', 'Upgrade.cmd', 'Trust-Browser.cmd', 'README.txt')
    $files += @('Update.cmd', 'Update.ps1', 'Fetch-Upgrade.ps1', 'Apply-Upgrade.ps1', 'oss-release-common.ps1')
    if ($externalRoot) { $files = @($script:ReleaseProgramFiles) }
    foreach ($relative in $files) {
        $from = Join-Path $SourceRoot $relative
        if (-not (Test-Path -LiteralPath $from)) { continue }
        Copy-Item -LiteralPath $from -Destination (Join-Path $root $relative) -Force
    }
    $node = Join-Path $SourceRoot 'payload\runtime\node.exe'
    if (Test-Path -LiteralPath $node) {
        New-PortableDirectory (Join-Path $root 'runtime\node')
        Copy-Item -LiteralPath $node -Destination (Join-Path $root 'runtime\node\node.exe') -Force
    }
    # Stage and subsequent lifecycle commands must use this release's runner.
    $native = Join-Path $SourceRoot 'payload\scripts\native-windows.ps1'
    if (Test-Path -LiteralPath $native) {
        Copy-Item -LiteralPath $native -Destination (Join-Path $root 'runtime\native-windows.ps1') -Force
    }
}

function Get-PortableDataCounts {
    $rows = @(Invoke-PortableSql -Root $root -Sql 'SELECT (SELECT count(*) FROM "user")::text || ''|'' || (SELECT count(*) FROM workspace)::text || ''|'' || (SELECT count(*) FROM issue)::text;' -Database ([string](Get-PortableDeployment $root).Database))
    $line = @($rows | Where-Object { $_ -match '\|' })
    if ($line.Count -eq 0) { return [pscustomobject]@{ Users = 0; Workspaces = 0; Issues = 0 } }
    $parts = ([string]$line[$line.Count - 1]).Split('|')
    return [pscustomobject]@{ Users = [int]$parts[0]; Workspaces = [int]$parts[1]; Issues = [int]$parts[2] }
}

$installed = Get-PortableDeployment $root
$installedVersion = [string]$installed.Version
if (-not $Profile) {
    $report = Read-PortableJson (Join-Path $root 'data\install-report.json')
    if ($report -and $report.profile) { $Profile = [string]$report.profile } else { $Profile = 'portable' }
}

$manifest = if ($Source) { Get-PortableManifest ([IO.Path]::GetFullPath($Source)) } else { Get-PortableManifest $root }
$newVersion = [string]$manifest.version
$clientCache=''
if ($ClientReleaseDirectory) {
    $cacheTools=if ($Source) { [IO.Path]::GetFullPath($Source) } else { $root }
    . (Join-Path $cacheTools 'oss-release-common.ps1')
    $clientRelease=Read-ReleaseJson (Join-Path $ClientReleaseDirectory 'release.json')
    if (-not ([string]$clientRelease.commit).StartsWith([string]$manifest.commit)) { throw 'Client cache source differs from the target server.' }
    $clientCache=Initialize-PortableClientCache $root $ClientReleaseDirectory $newVersion $clientRelease.commit
}
if ($newVersion -eq $installedVersion -and -not $Force) {
    Write-Host ('Already running ' + $installedVersion + '; the package is the same version. Nothing to upgrade.') -ForegroundColor Yellow
    exit 0
}

$httpsPort = if ($HttpsPort -gt 0) { $HttpsPort } else { Get-PortableHttpsPortFromConfig $root }
$apiPort = [int]$installed.ApiPort
$webPort = [int]$installed.WebPort
# Same decision as the installer: keep serving the address the installation was
# set up for, and add the program-scoped firewall rule when this process may.
$requestedLanHost = '127.0.0.1'
if (-not $NoLanAccess) {
    if ($PublicHost) {
        $requestedLanHost = $PublicHost
    } else {
        $configuredHost = Get-PortableLanHostFromConfig $root
        if ($configuredHost -and -not (Test-PortableLoopbackHost $configuredHost)) {
            $requestedLanHost = $configuredHost
        } else {
            $requestedLanHost = Get-PortablePublicHost
        }
    }
}
$lanPlan = Resolve-PortableLanPlan -Root $root -RequestedHost $requestedLanHost -HttpsPort $httpsPort -AllowRuleCreation:(-not $NoLanAccess)
$hostName = [string]$lanPlan.LanHost
Write-Host ([string]$lanPlan.Message)

Write-Host ''
Write-Host ('U Workers upgrade: ' + $installedVersion + ' -> ' + $newVersion) -ForegroundColor Green
Write-Host ''

# The backup needs the database running, so start the old server first.
Write-Host '==> Starting the current version so its data can be dumped'
Invoke-PortableNativeAction -Root $root -Action 'Start' | Out-Null
$before = Get-PortableDataCounts
Write-Host ('  data now: ' + $before.Users + ' users, ' + $before.Workspaces + ' workspaces, ' + $before.Issues + ' issues')

Write-Host '==> Backing up the database and uploads'
$backupOutput = Invoke-PortableNativeAction -Root $root -Action 'Backup'
$backupDirectory = $null
$backupRoot = Join-Path $root 'backups'
if (Test-Path -LiteralPath $backupRoot) {
    $backupDirectory = (Get-ChildItem -LiteralPath $backupRoot -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1)
}
if (-not $backupDirectory -or -not (Test-Path -LiteralPath (Join-Path $backupDirectory.FullName 'database.dump'))) {
    throw ('The upgrade aborted: the pre-upgrade backup could not be verified. ' + ([string]$backupOutput).Trim())
}
Write-Host ('  backup: ' + $backupDirectory.FullName)

Write-Host '==> Stopping the current version'
try { Stop-PortableDaemon -Root $root -Profile $Profile | Out-Null } catch { }
Stop-PortableCaddy -Root $root | Out-Null
Invoke-PortableNativeAction -Root $root -Action 'Stop' | Out-Null

if ($Source) {
    Write-Host '==> Copying the new package payload over the installed files'
    Copy-PortableOverlay ([IO.Path]::GetFullPath($Source))
    $duplicatePostgres = Join-Path $root 'payload\runtime\pgsql'
    if ((Test-Path -LiteralPath (Join-Path $root 'runtime\pgsql\bin\psql.exe')) -and (Test-Path -LiteralPath $duplicatePostgres)) {
        Remove-Item -LiteralPath $duplicatePostgres -Recurse -Force
        Write-Host '  kept the installed PostgreSQL binaries; removed the duplicate copy from the new payload'
    }
}

# Record the new version before staging: the native Stage action stamps the
# release directory from deployment.json, and the install report must agree with
# the package manifest after the upgrade.
$currentConfig = Get-PortableDeployment $root
$currentConfig.Version = $newVersion
Write-PortableJson (Join-Path $root 'deployment.json') $currentConfig

Write-Host '==> Staging the new release'
Invoke-PortableNativeAction -Root $root -Action 'Stage' -ExtraArguments @('-ReleaseSource', (Join-Path $root 'payload\release')) | Out-Null
$stagedRelease = [string](Get-PortableDeployment $root).Release
if ($stagedRelease) { Restore-PortableWebLinks -WebPath (Join-Path $stagedRelease 'web') | Out-Null }

Write-Host '==> Starting the new version (database migrations run automatically)'
Invoke-PortableNativeAction -Root $root -Action 'Start' | Out-Null
Start-PortableCaddy -Root $root -PublicHost $hostName -HttpsPort $httpsPort -ApiPort $apiPort -WebPort $webPort -LanExposed ([bool]$lanPlan.Exposed) | Out-Null
$rootCA = Get-PortablePublicRootCAPath $root
if (-not (Test-Path -LiteralPath $rootCA)) {
    $exported = Export-PortableRootCA $root
    $rootCA = $exported.Path
}
$serverUrl = 'https://' + $hostName + ':' + $httpsPort
Wait-PortableHttp -Uri ($serverUrl + '/readyz') -RootCAPath $rootCA -TimeoutSec 120 -Description 'the upgraded API' | Out-Null

if (-not $SkipDaemon) {
    Write-Host '==> Restarting the local daemon'
    try {
        Start-PortableDaemon -Root $root -Profile $Profile | Out-Null
    } catch {
        Write-Host ('  daemon restart warning: ' + $_.Exception.Message) -ForegroundColor Yellow
    }
}

$after = Get-PortableDataCounts
Write-Host ('  data after upgrade: ' + $after.Users + ' users, ' + $after.Workspaces + ' workspaces, ' + $after.Issues + ' issues')
if ($after.Users -lt $before.Users -or $after.Workspaces -lt $before.Workspaces -or $after.Issues -lt $before.Issues) {
    throw ('The upgrade changed the data unexpectedly. Restore from ' + $backupDirectory.FullName + ' if anything looks wrong.')
}

if (-not $NoStartup) { Install-PortableStartup -Root $root | Out-Null }
if (-not $SkipBrowserTrust) {
    if (-not (Test-PortableRootCAInstalled ((New-Object Security.Cryptography.X509Certificates.X509Certificate2($rootCA)).Thumbprint))) {
        Install-PortableBrowserTrust -Root $root -TimeoutSec 90 | Out-Null
    }
}

if (-not $ReportPath) {
    Update-PortableLanReport -Root $root -Plan $lanPlan | Out-Null
    $installReportPath = Join-Path $root 'data\install-report.json'
    $installReport = Read-PortableJson $installReportPath
    if ($installReport) {
        foreach ($entry in ([ordered]@{version=$newVersion;boardUrl=$serverUrl;httpsPort=$httpsPort;profile=$Profile}).GetEnumerator()) {
            $installReport | Add-Member -NotePropertyName $entry.Key -NotePropertyValue $entry.Value -Force
        }
        Write-PortableJson $installReportPath $installReport
    }
    $ReportPath = Join-Path $root 'data\upgrade-report.json'
}
$report = [ordered]@{
    upgradedAt = (Get-Date).ToUniversalTime().ToString('o')
    fromVersion = $installedVersion
    toVersion = $newVersion
    backup = $backupDirectory.FullName
    boardUrl = $serverUrl
    profile = $Profile
    lanExposed = [bool]$lanPlan.Exposed
    lanHost = $hostName
    lanMessage = [string]$lanPlan.Message
    dataBefore = $before
    dataAfter = $after
}
Write-PortableJson $ReportPath $report
if ($clientCache) { Enable-PortableClientCache $clientCache $newVersion $clientRelease.commit }

Write-Host ''
Write-Host ('U Workers upgraded to ' + $newVersion + '.') -ForegroundColor Green
Write-Host ('  Board:  https://localhost:' + $httpsPort + '/')
if ($lanPlan.Exposed) { Write-Host ('  LAN:    ' + $serverUrl + '/') } else { Write-Host ('  LAN:    off - ' + [string]$lanPlan.Message) }
Write-Host ('  Backup: ' + $backupDirectory.FullName)
Write-Host ('  Report: ' + $ReportPath)
if ($SkipDaemon) { Write-Host (Get-PortableDaemonStartCommand -Root $root -Profile $Profile) }
if (-not $NoBrowser) {
    try { Start-Process -FilePath ($serverUrl + '/') | Out-Null } catch { }
}
