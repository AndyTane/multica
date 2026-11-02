#Requires -Version 5.1
<#
.SYNOPSIS
    Removes the U Workers portable server from this Windows user.

.DESCRIPTION
    Stops the server and local daemon, removes the start-on-login item, removes
    the browser trust for the package root CA, and forgets this package's CLI
    profile. Data is kept by default so a reinstall can reuse it; pass
    -PurgeData to delete the database, uploads and backups (irreversible).

    The profile name is validated before anything is stopped or deleted, and
    only profile directories this package marked as its own are removed. The
    package folder itself is left in place; delete it afterwards if you do not
    want to reinstall.
#>
[CmdletBinding()]
param(
    [string]$Profile = '',
    [switch]$PurgeData,
    [switch]$Force,
    # Optional config-root override: the CLI's actual root is the first entry.
    # Used by tests to run the uninstall control flow against a temporary HOME;
    # production callers leave it empty.
    [string[]]$ConfigRoots
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
. (Join-Path $root 'portable-common.ps1')

if (-not (Test-Path -LiteralPath (Join-Path $root 'deployment.json'))) {
    Write-Host 'U Workers is not installed in this folder.'
    exit 0
}
if (-not $Profile) {
    $report = Read-PortableJson (Join-Path $root 'data\install-report.json')
    if ($report -and $report.profile) { $Profile = [string]$report.profile } else { $Profile = 'portable' }
}

# Validation and ownership lookup run before the first stop or delete: an
# unexpected profile name must fail without stopping the server or touching any
# profile directory. "." and ".." used to escape profiles\ and could delete the
# whole config root.
Assert-PortableProfileName $Profile | Out-Null
$ownershipArguments = @{ Profile = $Profile; PackageRoot = $root }
if ($ConfigRoots) { $ownershipArguments.ConfigRoots = $ConfigRoots }
$ownedProfiles = @(Get-PortableOwnedProfileDirectories @ownershipArguments)

if (-not $Force) {
    $answer = Read-Host ('Stop U Workers, remove its start-on-login item and browser trust' + $(if ($PurgeData) { ', and DELETE its data' } else { ' (data is kept)' }) + '? [y/N]')
    if ($answer -notmatch '^[yY]') { Write-Host 'Cancelled.'; exit 0 }
}

$warnings = @()
try { Stop-PortableDaemon -Root $root -Profile $Profile | Out-Null } catch { $warnings += $_.Exception.Message }
try { Stop-PortableCaddy -Root $root | Out-Null } catch { $warnings += $_.Exception.Message }
try { Invoke-PortableNativeAction -Root $root -Action 'Stop' | Out-Null } catch { $warnings += $_.Exception.Message }

if (Remove-PortableStartup) { Write-Host 'Removed the start-on-login item.' }
if (Remove-PortableBrowserTrust -Root $root) { Write-Host 'Removed the root CA from this user''s certificate store.' }

# Only this package's own marked profile directory is removed. Unmarked
# directories, including a same-named profile of another product, are left alone.
foreach ($profileDirectory in $ownedProfiles) {
    if (Test-Path -LiteralPath $profileDirectory) {
        Remove-Item -LiteralPath $profileDirectory -Recurse -Force
        Write-Host ('Removed CLI profile: ' + $profileDirectory)
    }
}
if ($ownedProfiles.Count -eq 0) {
    Write-Host ('No CLI profile marked as owned by this package was found for ' + $Profile + '; other profiles were left alone.') -ForegroundColor Yellow
}

if ($PurgeData) {
    $targets = @('data', 'uploads', 'backups', 'releases', 'state', 'logs', 'https', 'runtime')
    $rootPath = [IO.Path]::GetFullPath($root).TrimEnd('\')
    foreach ($relative in $targets) {
        $target = [IO.Path]::GetFullPath((Join-Path $root $relative))
        if (-not $target.StartsWith($rootPath + '\', [StringComparison]::OrdinalIgnoreCase)) {
            throw ('Refusing to delete a path outside the package: ' + $target)
        }
        if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Recurse -Force }
    }
    foreach ($file in @('deployment.json', 'root-ca.crt')) {
        $target = Join-Path $root $file
        if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Force }
    }
    Write-Host 'Deleted the database, uploads, backups and generated files.' -ForegroundColor Yellow
} else {
    Write-Host 'Data kept (database, uploads, backups). Use -PurgeData to delete it.'
}

if ($warnings.Count -gt 0) {
    Write-Host ('Warnings: ' + ($warnings -join '; ')) -ForegroundColor Yellow
}
Write-Host 'U Workers is uninstalled. Delete this folder to remove the package files.'
