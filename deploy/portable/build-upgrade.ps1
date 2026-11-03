#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PackageDirectory,
    [Parameter(Mandatory=$true)][string]$BaselineDirectory,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [Parameter(Mandatory=$true)][string]$Commit
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'oss-release-common.ps1')
Assert-ReleaseCommit $Commit
Assert-ReleaseSeparateDirectory $OutputDirectory $PackageDirectory
Assert-ReleaseSeparateDirectory $OutputDirectory $BaselineDirectory
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Use a fresh delta output directory.' }
function Get-ProgramInventory([string]$Directory) {
    if (Test-Path -LiteralPath (Join-Path $Directory 'deployment.json')) { throw 'Pass an uninstalled portable package, not a running installation.' }
    $map = @{}
    $paths = @($script:ReleaseProgramFiles | Where-Object { Test-Path -LiteralPath (Join-Path $Directory $_) -PathType Leaf })
    foreach ($prefix in @('payload/release','payload/bin','payload/runtime','payload/installer','payload/scripts','payload/source')) {
        $folder = Resolve-ReleasePath $Directory $prefix
        if (Test-Path -LiteralPath $folder) {
            $paths += @(Get-ReleaseFiles $folder | ForEach-Object { $_.FullName.Substring([IO.Path]::GetFullPath($Directory).TrimEnd('\').Length+1).Replace('\','/') })
        }
    }
    foreach ($relative in $paths) {
        Assert-ReleaseProgramPath $relative
        $file = Resolve-ReleasePath $Directory $relative
        $map[$relative] = [pscustomobject]@{path=$relative;sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant();bytes=(Get-Item -LiteralPath $file).Length}
    }
    $runner = Resolve-ReleasePath $Directory 'payload/scripts/native-windows.ps1'
    if (Test-Path -LiteralPath $runner) {
        $map['runtime/native-windows.ps1'] = [pscustomobject]@{path='runtime/native-windows.ps1';sha256=(Get-FileHash -LiteralPath $runner -Algorithm SHA256).Hash.ToLowerInvariant();bytes=(Get-Item -LiteralPath $runner).Length}
    }
    return $map
}
$before = Read-ReleaseJson (Resolve-ReleasePath $BaselineDirectory 'manifest.json')
$after = Read-ReleaseJson (Resolve-ReleasePath $PackageDirectory 'manifest.json')
Assert-ReleaseVersion $before.version; Assert-ReleaseVersion $after.version
Assert-ReleaseCommit $before.commit
if (-not $Commit.StartsWith([string]$after.commit) -or ([string]$after.commit).Length -lt 9) { throw 'Built package does not match the selected source commit.' }
$old = Get-ProgramInventory $BaselineDirectory; $new = Get-ProgramInventory $PackageDirectory
$write = @(); $keep = @(); $delete = @()
foreach ($path in @($new.Keys | Sort-Object)) {
    $entry = $new[$path]
    if ($old.ContainsKey($path) -and $entry.sha256 -eq $old[$path].sha256) { $keep += $entry; continue }
    $write += [pscustomobject]@{path=$path;sha256=$entry.sha256;bytes=$entry.bytes;old_sha256=$(if ($old.ContainsKey($path)) {$old[$path].sha256} else {$null});action=$(if ($old.ContainsKey($path)) {'replace'} else {'add'})}
}
foreach ($path in @($old.Keys | Sort-Object)) {
    if (-not $new.ContainsKey($path)) { $delete += $old[$path] }
}
$plan = [pscustomobject]@{from=$before.version;to=$after.version;commit=$Commit;baselineCommit=$before.commit;write=$write;delete=$delete;keep=$keep}
Assert-UpgradePlan $plan
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
foreach ($entry in $write) {
    $destination = Resolve-ReleasePath $OutputDirectory $entry.path
    New-Item -ItemType Directory -Path (Split-Path -Parent $destination) -Force | Out-Null
    $source = if ($entry.path -eq 'runtime/native-windows.ps1') {'payload/scripts/native-windows.ps1'} else {$entry.path}
    Copy-Item -LiteralPath (Resolve-ReleasePath $PackageDirectory $source) -Destination $destination
}
# Installed native runners are refreshed from the same target source script.
# The payload/scripts copy remains covered by the target file inventory.
Write-ReleaseJson (Join-Path $OutputDirectory 'UPGRADE-FILES.json') $plan
return $OutputDirectory
