#Requires -Version 5.1
<#
.SYNOPSIS
    Stops the U Workers portable server and its local agent daemon.

.DESCRIPTION
    Only processes recorded by this package are signalled: the daemon through
    its own CLI, Caddy and the server supervisor through their verified process
    records. Nothing else on the machine is touched.
#>
[CmdletBinding()]
param(
    [string]$Profile = '',
    [switch]$Quiet
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
. (Join-Path $root 'portable-common.ps1')

if (-not (Test-Path -LiteralPath (Join-Path $root 'deployment.json'))) {
    if (-not $Quiet) { Write-Host 'U Workers is not installed in this folder.' }
    exit 0
}

if (-not $Profile) {
    $report = Read-PortableJson (Join-Path $root 'data\install-report.json')
    if ($report -and $report.profile) { $Profile = [string]$report.profile } else { $Profile = 'portable' }
}
$config = Get-PortableDeployment $root
$errors = @()
try { Stop-PortableDaemon -Root $root -Profile $Profile | Out-Null } catch { $errors += $_.Exception.Message }
try { Stop-PortableCaddy -Root $root | Out-Null } catch { $errors += $_.Exception.Message }
try { Invoke-PortableNativeAction -Root $root -Action 'Stop' | Out-Null } catch { $errors += $_.Exception.Message }
if (-not $Quiet) {
    if ($errors.Count -eq 0) {
        Write-Host 'U Workers is stopped. Data is kept.'
    } else {
        Write-Host ('U Workers stopped with warnings: ' + ($errors -join '; ')) -ForegroundColor Yellow
    }
}
if ($errors.Count -gt 0) { exit 1 }
