#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PackageDirectory,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [Parameter(Mandatory=$true)][string]$Version,
    [Parameter(Mandatory=$true)][string]$Commit,
    [Parameter(Mandatory=$true)][string]$Ossutil
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'oss-release-common.ps1')
Assert-ReleaseVersion $Version
if ($Commit -cnotmatch '^[0-9a-f]{40}$') { throw 'Full release requires the complete source commit.' }
Assert-ReleaseSeparateDirectory $OutputDirectory $PackageDirectory
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Use a fresh full release directory.' }
if (Test-Path -LiteralPath (Join-Path $PackageDirectory 'deployment.json')) { throw 'Do not package an installed deployment.' }
Assert-ReleaseOssutil $Ossutil
$package = [IO.Path]::GetFullPath($PackageDirectory).TrimEnd('\')
$manifest = Read-ReleaseJson (Resolve-ReleasePath $package 'manifest.json')
if ($manifest.version -cne $Version -or ([string]$manifest.commit).Length -lt 9 -or -not $Commit.StartsWith([string]$manifest.commit)) { throw 'Package differs from the release source.' }
$paths = @($script:ReleaseProgramFiles | Where-Object { Test-Path -LiteralPath (Join-Path $package $_) -PathType Leaf })
foreach ($prefix in @('payload/release','payload/bin','payload/runtime','payload/installer','payload/scripts','payload/source')) {
    $folder = Resolve-ReleasePath $package $prefix
    if (Test-Path -LiteralPath $folder) { $paths += @(Get-ReleaseFiles $folder | ForEach-Object { $_.FullName.Substring($package.Length+1).Replace('\','/') }) }
}
# Validate names before reading any file, including hashes.
foreach ($path in $paths) { Assert-ReleaseProgramPath $path; $null = Resolve-ReleasePath $package $path }
$stage = Join-Path $OutputDirectory 'archive'
New-Item -ItemType Directory -Path $stage -Force | Out-Null
foreach ($path in $paths) {
    $target = Resolve-ReleasePath $stage $path
    New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force | Out-Null
    Copy-Item -LiteralPath (Resolve-ReleasePath $package $path) -Destination $target
}
$instructions = [IO.File]::ReadAllText((Join-Path $PSScriptRoot '../../docs/windows-full-package-upgrade.md'),[Text.Encoding]::UTF8).Replace('{{VERSION}}',$Version).Replace('{{COMMIT}}',$Commit)
[IO.File]::WriteAllText((Join-Path $stage 'UPGRADE.md'),$instructions,(New-Object Text.UTF8Encoding($false)))
$bootstrap = Join-Path $stage 'download-tools'
New-Item -ItemType Directory -Path $bootstrap | Out-Null
foreach ($name in $script:UpdateToolFiles) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $bootstrap $name) }
Copy-Item -LiteralPath (Get-Command $Ossutil -ErrorAction Stop).Source -Destination (Join-Path $bootstrap 'ossutil.exe')
$files = @(foreach ($file in Get-ReleaseFiles $stage) {
    @{path=$file.FullName.Substring($stage.Length+1).Replace('\','/');bytes=$file.Length;sha256=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
})
Write-ReleaseJson (Join-Path $stage 'FULL-FILES.json') @{schemaVersion=1;version=$Version;commit=$Commit;files=$files}
Assert-FullReleaseDirectory $stage $Version $Commit
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = Join-Path $OutputDirectory 'full.zip'
[IO.Compression.ZipFile]::CreateFromDirectory($stage,$archive,[IO.Compression.CompressionLevel]::Optimal,$false)
Copy-Item -LiteralPath (Join-Path $stage 'UPGRADE.md') -Destination (Join-Path $OutputDirectory 'UPGRADE.md')
function Record([string]$Name) {
    $file = Join-Path $OutputDirectory $Name
    return @{key=('releases/'+$Version+'/'+$Name);bytes=(Get-Item -LiteralPath $file).Length;sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()}
}
$release = @{schemaVersion=2;packageType='full';tag=$Version;to=$Version;commit=$Commit;platform='windows-amd64';package=(Record 'full.zip');instructions=(Record 'UPGRADE.md')}
Assert-ReleaseManifest $release $Version
Write-ReleaseJson (Join-Path $OutputDirectory 'release.json') $release
Write-ReleaseJson (Join-Path $OutputDirectory 'latest.json') @{schemaVersion=1;version=$Version;release=(Record 'release.json')}
# Validate the compressed bytes as the recipient will before publishing.
$verified = Join-Path $OutputDirectory 'verified'
Expand-ReleaseArchive $archive $verified
Assert-FullReleaseDirectory $verified $Version $Commit
return $OutputDirectory
