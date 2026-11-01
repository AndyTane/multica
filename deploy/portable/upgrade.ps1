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
    [string]$PublicHost,
    [int]$HttpsPort = 0,
    [string]$Profile = '',
    [switch]$SkipBrowserTrust,
    [switch]$SkipDaemon,
    [switch]$NoBrowser,
    [switch]$NoStartup,
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
. (Join-Path $root 'portable-common.ps1')

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
if ($newVersion -eq $installedVersion -and -not $Force) {
    Write-Host ('Already running ' + $installedVersion + '; the package is the same version. Nothing to upgrade.') -ForegroundColor Yellow
    exit 0
}

$hostName = if ($PublicHost) { $PublicHost } else { Get-PortableLanHostFromConfig $root }
$httpsPort = if ($HttpsPort -gt 0) { $HttpsPort } else { Get-PortableHttpsPortFromConfig $root }
$apiPort = [int]$installed.ApiPort
$webPort = [int]$installed.WebPort

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
Start-PortableCaddy -Root $root -PublicHost $hostName -HttpsPort $httpsPort -ApiPort $apiPort -WebPort $webPort | Out-Null
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

$report = [ordered]@{
    upgradedAt = (Get-Date).ToUniversalTime().ToString('o')
    fromVersion = $installedVersion
    toVersion = $newVersion
    backup = $backupDirectory.FullName
    boardUrl = $serverUrl
    profile = $Profile
    dataBefore = $before
    dataAfter = $after
}
$reportPath = Join-Path $root 'data\upgrade-report.json'
Write-PortableJson $reportPath $report

Write-Host ''
Write-Host ('U Workers upgraded to ' + $newVersion + '.') -ForegroundColor Green
Write-Host ('  Board:  ' + $serverUrl + '/')
Write-Host ('  Backup: ' + $backupDirectory.FullName)
Write-Host ('  Report: ' + $reportPath)
if (-not $NoBrowser) {
    try { Start-Process -FilePath ($serverUrl + '/') | Out-Null } catch { }
}
