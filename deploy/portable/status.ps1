#Requires -Version 5.1
<#
.SYNOPSIS
    Shows what the installed U Workers portable server is doing.
#>
[CmdletBinding()]
param([string]$Profile = '')

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
. (Join-Path $root 'portable-common.ps1')

if (-not (Test-Path -LiteralPath (Join-Path $root 'deployment.json'))) {
    Write-Host 'U Workers is not installed in this folder. Run Setup.cmd first.'
    exit 1
}
if (-not $Profile) {
    $report = Read-PortableJson (Join-Path $root 'data\install-report.json')
    if ($report -and $report.profile) { $Profile = [string]$report.profile } else { $Profile = 'portable' }
}
$config = Get-PortableDeployment $root
$hostName = Get-PortableLanHostFromConfig $root
$httpsPort = Get-PortableHttpsPortFromConfig $root

$serverStatus = $null
try { $serverStatus = (Invoke-PortableNativeAction -Root $root -Action 'Status' | Out-String).Trim() } catch { $serverStatus = '' }
$caddyRecord = Read-PortableJson (Join-Path $root 'state\caddy.json')
$caddyRunning = [bool]($caddyRecord -and (Test-PortableProcessRecord $caddyRecord))
$daemon = Get-PortableDaemonStatus -Root $root -Profile $Profile
$apiReady = $false
try {
    $response = Invoke-PortableWebRequest -Uri ('https://localhost:' + $httpsPort + '/readyz') -RootCAPath (Get-PortablePublicRootCAPath $root) -TimeoutSec 5
    $apiReady = ($response.StatusCode -eq 200)
} catch { }

Write-Host ''
Write-Host 'U Workers portable server' -ForegroundColor Green
Write-Host ('  version:        ' + [string]$config.Version)
Write-Host ('  board:          https://' + $hostName + ':' + $httpsPort + '/')
Write-Host ('  API ready:      ' + $apiReady)
Write-Host ('  HTTPS proxy:    ' + $caddyRunning)
Write-Host ('  local daemon:   ' + $daemon.Running + ' (profile ' + $Profile + ')')
Write-Host ('  root CA:        ' + (Get-PortablePublicRootCAPath $root))
if ($serverStatus) { Write-Host ('  server:         ' + $serverStatus) }
$report = Read-PortableJson (Join-Path $root 'data\install-report.json')
if ($report -and $report.runtimes) {
    $names = @($report.runtimes | ForEach-Object { [string]$_.name + ' (' + [string]$_.provider + ')' })
    Write-Host ('  runtimes seen:  ' + ($names -join ', '))
}
Write-Host ''
