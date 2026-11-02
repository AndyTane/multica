#Requires -Version 5.1
<#
.SYNOPSIS
    Starts the installed U Workers portable server (idempotent).

.DESCRIPTION
    Called by the start-on-login item and by Start.cmd. Safe to run repeatedly:
    an already-running part is left alone. Failures are written to
    logs\start-error-*.log instead of popping a console, because this script
    also runs hidden at sign-in.
#>
[CmdletBinding()]
param(
    [string]$Profile = '',
    [switch]$Quiet
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
. (Join-Path $root 'portable-common.ps1')

try {
    if (-not $Profile) {
        $report = Read-PortableJson (Join-Path $root 'data\install-report.json')
        if ($report -and $report.profile) { $Profile = [string]$report.profile } else { $Profile = 'portable' }
    }
    $config = Get-PortableDeployment $root
    $httpsPort = Get-PortableHttpsPortFromConfig $root
    # Never add a firewall rule here: this script also runs hidden at sign-in, and
    # a silent rule change (or a security prompt) is not something a login task
    # may do. It only reads whether a rule exists, and falls back to loopback.
    $lanPlan = Resolve-PortableLanPlan -Root $root -RequestedHost (Get-PortableRequestedLanHostFromConfig $root) -HttpsPort $httpsPort
    $hostName = [string]$lanPlan.LanHost
    Invoke-PortableNativeAction -Root $root -Action 'Start' | Out-Null
    Start-PortableCaddy -Root $root -PublicHost $hostName -HttpsPort $httpsPort -ApiPort ([int]$config.ApiPort) -WebPort ([int]$config.WebPort) -LanExposed ([bool]$lanPlan.Exposed) | Out-Null
    if (-not (Test-Path -LiteralPath (Get-PortablePublicRootCAPath $root))) {
        Invoke-PortableTlsHandshake -Uri ('https://localhost:' + $httpsPort + '/readyz')
        Export-PortableRootCA $root | Out-Null
    }
    Wait-PortableHttp -Uri ('https://localhost:' + $httpsPort + '/readyz') -RootCAPath (Get-PortablePublicRootCAPath $root) -TimeoutSec 120 -Description 'the API over HTTPS' | Out-Null
    $daemon = Start-PortableDaemon -Root $root -Profile $Profile
    try { Update-PortableLanReport -Root $root -Plan $lanPlan | Out-Null } catch { }
    if (-not $Quiet) {
        Write-Host ('U Workers is running: https://localhost:' + $httpsPort + '/')
        Write-Host ([string]$lanPlan.Message)
        if ($daemon.Running) { Write-Host 'Local agent daemon: running' } else { Write-Host 'Local agent daemon: not running - see the daemon logs under the CLI profile directory.' -ForegroundColor Yellow }
    }
} catch {
    $log = Join-Path $root ('logs\start-error-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.log')
    New-PortableDirectory (Split-Path -Parent $log)
    [IO.File]::WriteAllText($log, $_.Exception.ToString(), (New-Object Text.UTF8Encoding($false)))
    if (-not $Quiet) {
        Write-Host ('U Workers could not start: ' + $_.Exception.Message) -ForegroundColor Red
        Write-Host ('Details: ' + $log)
    }
    exit 1
}
