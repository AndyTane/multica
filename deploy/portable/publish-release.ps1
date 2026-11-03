#Requires -Version 7.0
[CmdletBinding(DefaultParameterSetName='Build')]
param(
    [Parameter(Mandatory=$true)][string]$Version,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [Parameter(Mandatory=$true,ParameterSetName='Existing')][string]$UpgradeDirectory,
    [Parameter(Mandatory=$true,ParameterSetName='Build')][string]$SourceDirectory,
    [Parameter(ParameterSetName='Build')][string]$BaselineDirectory,
    [Parameter(ParameterSetName='Build')][switch]$Full,
    [Parameter(ParameterSetName='Build')][string]$SourceRef,
    [Parameter(ParameterSetName='Build')][hashtable]$BuildParameters = @{},
    [string]$Ossutil = 'ossutil.exe',
    [switch]$PrepareOnly
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'oss-release-common.ps1')
Assert-ReleaseVersion $Version
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw 'Use a fresh release output directory; prepared artifacts are never overwritten.' }
Resolve-ReleasePath (Split-Path -Parent $output) (Split-Path -Leaf $output) | Out-Null
if ($UpgradeDirectory) { Assert-ReleaseSeparateDirectory $output $UpgradeDirectory }
if ($SourceDirectory) {
    if (-not $Full -and -not $BaselineDirectory) { throw 'Differential builds require -BaselineDirectory; use -Full to establish a new baseline.' }
    Assert-ReleaseSeparateDirectory $output $SourceDirectory
    if (-not $SourceRef) { $SourceRef = $Version }
    $head = (& git -C $SourceDirectory rev-parse HEAD | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve source checkout.' }
    $selected = (& git -C $SourceDirectory rev-parse ($SourceRef+'^{commit}') | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $selected -ne $head) { throw 'SourceDirectory must be checked out at SourceRef (default: Version tag).' }
    $tagCommit = (& git -C $SourceDirectory rev-parse ('refs/tags/'+$Version+'^{commit}') | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $tagCommit -ne $head) { throw 'Version tag must identify the clean source commit; do not rebuild an old version from changed code.' }
    $dirty = & git -C $SourceDirectory status --porcelain --untracked-files=normal
    if ($LASTEXITCODE -ne 0 -or $dirty) { throw 'Release source must be clean.' }
    foreach ($key in $BuildParameters.Keys) {
        if ($key -notin @('PostgresDirectory','PgArchive','PgArchiveSha256','NodeExe','GoExecutable','PnpmCmd','GoModCache','CaddyExe','OpenCodeExe')) { throw 'Unsupported build parameter; payload reuse and version overrides are prohibited.' }
    }
}
if (-not $PrepareOnly) { $config = Get-ReleaseOssConfig; Assert-ReleaseOssutil $Ossutil }
New-Item -ItemType Directory -Path $output | Out-Null
if ($SourceDirectory) {
    & (Join-Path $SourceDirectory 'deploy/portable/build-package.ps1') -Version $Version -OutputRoot (Join-Path $output 'build') -SkipBuildKit -NoZip @BuildParameters | Out-Host
    if ($Full) {
        $fullOutput = Join-Path $output 'full'
        & (Join-Path $SourceDirectory 'deploy/portable/build-full-release.ps1') -PackageDirectory (Join-Path $output ('build/UWorkers-Portable-'+$Version)) -OutputDirectory $fullOutput -Version $Version -Commit $head -Ossutil $Ossutil | Out-Host
        $output = $fullOutput
    } else {
    $UpgradeDirectory = & (Join-Path $PSScriptRoot 'build-upgrade.ps1') -PackageDirectory (Join-Path $output ('build/UWorkers-Portable-'+$Version)) -BaselineDirectory $BaselineDirectory -OutputDirectory (Join-Path $output 'delta') -Commit $head
    }
}
if (-not $Full) {
$plan = Get-ReleasePlan $UpgradeDirectory
if ($plan.to -cne $Version) { throw 'Requested version and upgrade plan disagree.' }
Assert-ReleaseDirectory $UpgradeDirectory $plan
$stage = Join-Path $output 'archive'
New-Item -ItemType Directory -Path $stage | Out-Null
# Copy ONLY the checked program write set, never recursively zip a deployment.
foreach ($entry in $plan.write) {
    $to = Resolve-ReleasePath $stage $entry.path
    New-Item -ItemType Directory -Path (Split-Path -Parent $to) -Force | Out-Null
    Copy-Item -LiteralPath (Resolve-ReleasePath $UpgradeDirectory $entry.path) -Destination $to
}
Write-ReleaseJson (Join-Path $stage 'UPGRADE-FILES.json') $plan
foreach ($name in @('Apply-Upgrade.ps1','oss-release-common.ps1')) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $stage $name) }
$instructions = Join-Path $stage 'UPGRADE.md'
$originalInstructions = Resolve-ReleasePath $UpgradeDirectory 'UPGRADE.md'
if (Test-Path -LiteralPath $originalInstructions) { Copy-Item -LiteralPath $originalInstructions -Destination $instructions } else { Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'upgrade-instructions.md') -Destination $instructions }
if ($plan.from -eq 'v0.2.1' -and $plan.to -eq 'v0.2.2' -and $plan.commit -eq '3e62bb6ebb9393210a7ed44fbad021611e9bc7ff') {
    $text = [IO.File]::ReadAllText($instructions,[Text.Encoding]::UTF8).Replace('upgrade-v022-','upgrade-v0.2.2-')
    [IO.File]::WriteAllText($instructions,$text,(New-Object Text.UTF8Encoding($false)))
}
Add-Content -LiteralPath $instructions -Encoding UTF8 -Value "`r`nRelease channel: $($plan.from) -> $($plan.to); commit $($plan.commit). Use the bundled Apply script; rollback checkpoints are backups/upgrade-<version>-<timestamp>."
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = Join-Path $output 'upgrade.zip'
[IO.Compression.ZipFile]::CreateFromDirectory($stage,$archive,[IO.Compression.CompressionLevel]::Optimal,$false)
if ((Get-Item -LiteralPath $archive).Length -gt $script:ReleaseMaxObjectBytes) { throw 'Release ZIP exceeds the 5 GiB single-object upload limit.' }
Copy-Item -LiteralPath $instructions -Destination (Join-Path $output 'UPGRADE.md')
function Get-ObjectRecord([string]$Name) {
    $file = Join-Path $output $Name
    return @{key=('releases/'+$Version+'/'+$Name);bytes=(Get-Item -LiteralPath $file).Length;sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()}
}
$release = @{schemaVersion=1;from=$plan.from;to=$plan.to;commit=$plan.commit;baselineCommit=$plan.baselineCommit;platform='windows-amd64';package=(Get-ObjectRecord 'upgrade.zip');instructions=(Get-ObjectRecord 'UPGRADE.md')}
Assert-ReleaseManifest $release $Version
Write-ReleaseJson (Join-Path $output 'release.json') $release
$releaseRecord = Get-ObjectRecord 'release.json'
Write-ReleaseJson (Join-Path $output 'latest.json') @{schemaVersion=1;version=$Version;release=$releaseRecord}
}
$release = Read-ReleaseJson (Join-Path $output 'release.json')
$result = @{version=$Version;commit=$release.commit;prepared=$true;published=$false;sourceMode=$(if ($Full) {'source-build-full'} elseif ($SourceDirectory) {'source-build'} else {'existing-delta'});completedObjects=@()}
$resultPath = Join-Path $output 'publish-result.json'
Write-ReleaseJson $resultPath $result
if ($PrepareOnly) { Write-Host ('Prepared '+$Version+'; no OSS requests were made.'); return $output }
try {
    $packageName = if ($Full) {'full.zip'} else {'upgrade.zip'}
    foreach ($name in @($packageName,'UPGRADE.md','release.json','latest.json')) {
        $key = if ($name -eq 'latest.json') {'channels/stable/latest.json'} else {'releases/'+$Version+'/'+$name}
        Invoke-ReleaseObject $Ossutil $config put $key (Join-Path $output $name) -Mutable:($name -eq 'latest.json')
        $result.completedObjects += $key
        Write-ReleaseJson $resultPath $result
    }
    $result.published = $true; Write-ReleaseJson $resultPath $result
} catch {
    Write-Host 'Publish stopped. See publish-result.json. Do not overwrite or delete version objects; use a new version after inspecting the partial release. latest is updated only after all immutable objects succeed.'
    throw
}
return $output
