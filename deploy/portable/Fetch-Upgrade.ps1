#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version = 'latest',
    [string]$DestinationDirectory = '',
    [string]$Ossutil = 'ossutil.exe',
    [switch]$ClientKit,
    [switch]$Apply,
    [switch]$CheckOnly,
    [string]$Root,
    [string]$Profile,
    [string]$PublicOrigin,
    [switch]$NoLanAccess
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'oss-release-common.ps1')
if ($Version -ne 'latest') { Assert-ReleaseVersion $Version }
if ($ClientKit -and ($Apply -or $CheckOnly)) { throw 'ClientKit downloads do not apply server upgrades.' }
if (($Apply -or $CheckOnly) -and (-not $Root -or -not $Profile -or -not $PublicOrigin)) { throw 'Pass -Root, -Profile and -PublicOrigin with -Apply or -CheckOnly.' }
if (-not $DestinationDirectory) { $DestinationDirectory = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-upgrade-'+[guid]::NewGuid().ToString('N')) }
$destination = [IO.Path]::GetFullPath($DestinationDirectory)
if ($Root) { Assert-ReleaseSeparateDirectory $destination $Root }
if (Test-Path -LiteralPath $destination) { throw 'Use a new staging directory outside the installation.' }
Resolve-ReleasePath (Split-Path -Parent $destination) (Split-Path -Leaf $destination) | Out-Null
$config = Get-ReleaseOssConfig
Assert-ReleaseOssutil $Ossutil
New-Item -ItemType Directory -Path $destination | Out-Null
$releasePath = Join-Path $destination 'release.json'
$latest = $null
if ($Version -eq 'latest') {
    $latestPath = Join-Path $destination 'latest.json'
    Invoke-ReleaseObject $Ossutil $config get 'channels/stable/latest.json' $latestPath
    $latest = Read-ReleaseJson $latestPath
    Assert-ReleaseVersion ([string]$latest.version)
    $Version = [string]$latest.version
    if ($latest.schemaVersion -ne 1 -or $latest.release.key -cne ('releases/'+$Version+'/release.json') -or $null -eq $latest.release.bytes -or $latest.release.bytes -is [string] -or [long]$latest.release.bytes -le 0 -or [long]$latest.release.bytes -gt 8MB) { throw 'Invalid latest pointer.' }
    Assert-ReleaseHash ([string]$latest.release.sha256)
}
Invoke-ReleaseObject $Ossutil $config get ('releases/'+$Version+'/release.json') $releasePath
if ($latest) { Assert-ReleaseFile $releasePath $latest.release.sha256 ([long]$latest.release.bytes) }
$release = Read-ReleaseJson $releasePath
Assert-ReleaseManifest $release $Version
if ($ClientKit) {
    if (-not $release.clientKit) { throw 'This release has no client kit. Select v0.2.5 or a newer release.' }
    $archive=Join-Path $destination 'client-kit.zip'
    Invoke-ReleaseObject $Ossutil $config get $release.clientKit.key $archive
    Assert-ReleaseFile $archive $release.clientKit.sha256 ([long]$release.clientKit.bytes)
    $unpacked=Join-Path $destination 'client-kit'
    Expand-ReleaseArchive $archive $unpacked
    Assert-ClientKitDirectory $unpacked $Version $release.commit
    Write-ReleaseJson (Join-Path $destination 'fetch-result.json') @{version=$Version;commit=$release.commit;packageType='client-kit';sha256=$release.clientKit.sha256;verified=$true;applied=$false}
    Write-Host ('客户端包 '+$Version+' 已校验。换新版先用 Import.cmd 导入旧设置；首次使用双击 Connect.cmd。')
    return $unpacked
}
$archive = Join-Path $destination 'upgrade.zip'
Invoke-ReleaseObject $Ossutil $config get $release.package.key $archive
Assert-ReleaseFile $archive $release.package.sha256 ([long]$release.package.bytes)
$instructions = Join-Path $destination 'UPGRADE.md'
Invoke-ReleaseObject $Ossutil $config get $release.instructions.key $instructions
Assert-ReleaseFile $instructions $release.instructions.sha256 ([long]$release.instructions.bytes)
$unpacked = Join-Path $destination 'upgrade'
Expand-ReleaseArchive $archive $unpacked
if ($release.schemaVersion -eq 2 -and $release.packageType -eq 'full') {
    Assert-FullReleaseDirectory $unpacked $Version $release.commit
    Assert-ReleaseFile (Join-Path $unpacked 'UPGRADE.md') $release.instructions.sha256 ([long]$release.instructions.bytes)
    Write-ReleaseJson (Join-Path $destination 'fetch-result.json') @{version=$Version;packageType='full';sha256=$release.package.sha256;verified=$true;applied=$false}
    if ($Apply -or $CheckOnly) { throw 'Full package verified. Full upgrades require the database and backup checks in the extracted UPGRADE.md; do not run the differential Apply entry.' }
    Write-Host ('Verified full package '+$Version+'. Follow upgrade/UPGRADE.md to back up and upgrade the existing installation.')
    return $unpacked
}
$plan = Get-ReleasePlan $unpacked
foreach ($name in @('from','to','commit','baselineCommit')) {
    if ($plan.$name -cne $release.$name) { throw 'Archive plan and release manifest disagree.' }
}
Assert-ReleaseDirectory $unpacked $plan
Assert-ReleaseFile (Join-Path $unpacked 'UPGRADE.md') $release.instructions.sha256 ([long]$release.instructions.bytes)
$allowed = @{}; foreach ($entry in $plan.write) { $allowed[[string]$entry.path] = $true }
foreach ($name in @('UPGRADE-FILES.json','Apply-Upgrade.ps1','oss-release-common.ps1','UPGRADE.md')) { $allowed[$name] = $true }
foreach ($file in Get-ReleaseFiles $unpacked) {
    $relative = $file.FullName.Substring($unpacked.Length+1).Replace('\','/')
    if (-not $allowed.ContainsKey($relative)) { throw 'Archive contains a file outside the upgrade plan.' }
}
foreach ($name in @('Apply-Upgrade.ps1','oss-release-common.ps1')) {
    if (-not (Test-Path -LiteralPath (Join-Path $unpacked $name) -PathType Leaf)) { throw 'Archive is missing its upgrade helper.' }
}
Write-ReleaseJson (Join-Path $destination 'fetch-result.json') @{version=$Version;sha256=$release.package.sha256;verified=$true;applied=$false}
if ($Apply -or $CheckOnly) {
    $clientArgs=@{}
    if ($Apply -and $release.clientKit) {
        $clientStage=$destination+'-client'
        $null=& $PSCommandPath -Version $Version -ClientKit -Ossutil $Ossutil -DestinationDirectory $clientStage
        $clientArgs.ClientReleaseDirectory=$clientStage
    }
    # Use the reviewed bootstrap entry, not a command/path supplied in JSON.
    & (Join-Path $PSScriptRoot 'Apply-Upgrade.ps1') -Root $Root -Profile $Profile -PublicOrigin $PublicOrigin -NoLanAccess:$NoLanAccess -CheckOnly:$CheckOnly -UpgradeDirectory $unpacked @clientArgs
    Write-ReleaseJson (Join-Path $destination 'fetch-result.json') @{version=$Version;sha256=$release.package.sha256;verified=$true;applied=[bool](-not $CheckOnly);checked=[bool]$CheckOnly}
}
Write-Host ('升级包 '+$Version+' 已校验，文件与说明在下载目录。原有权限保留。')
return $unpacked
