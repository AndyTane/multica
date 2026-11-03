#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$PackageDirectory,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [Parameter(Mandatory=$true)][string]$Version,
    [Parameter(Mandatory=$true)][string]$Commit
)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'oss-release-common.ps1')
Assert-ReleaseVersion $Version
if ($Commit -cnotmatch '^[0-9a-f]{40}$') { throw 'Client kit requires the complete source commit.' }
Assert-ReleaseSeparateDirectory $OutputDirectory $PackageDirectory
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Use a fresh client kit output directory.' }
if (Test-Path -LiteralPath (Join-Path $PackageDirectory 'deployment.json')) { throw 'Use an uninstalled release package.' }
$manifest=Read-ReleaseJson (Resolve-ReleasePath $PackageDirectory 'manifest.json')
if ($manifest.version -cne $Version -or ([string]$manifest.commit).Length -lt 9 -or -not $Commit.StartsWith([string]$manifest.commit)) { throw 'Client and server source identities differ.' }
$stage=Join-Path $OutputDirectory 'client-kit'
New-Item -ItemType Directory -Path $stage -Force | Out-Null
$template=Join-Path $PSScriptRoot '../client-kit'
foreach ($name in $script:ClientKitFiles) {
    $source=if ($name -in @('u-workers.exe','multica.exe')) { Resolve-ReleasePath $PackageDirectory ('payload/bin/'+$name) } else { Resolve-ReleasePath $template $name }
    Copy-Item -LiteralPath $source -Destination (Join-Path $stage $name)
}
$files=@(foreach ($file in Get-ReleaseFiles $stage) {
    @{path=$file.Name;bytes=$file.Length;sha256=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
})
Write-ReleaseJson (Join-Path $stage 'CLIENT-FILES.json') @{schemaVersion=2;version=$Version;commit=$Commit;platform='windows-amd64';files=$files}
Assert-ClientKitDirectory $stage $Version $Commit
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive=Join-Path $OutputDirectory 'client-kit.zip'
[IO.Compression.ZipFile]::CreateFromDirectory($stage,$archive,[IO.Compression.CompressionLevel]::Optimal,$false)
$verified=Join-Path $OutputDirectory 'verified'
Expand-ReleaseArchive $archive $verified
Assert-ClientKitDirectory $verified $Version $Commit
return $archive
