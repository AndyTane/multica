#Requires -Version 5.1
[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$OutputDirectory)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'oss-release-common.ps1')
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw 'Use a new bootstrap output directory.' }
Resolve-ReleasePath (Split-Path -Parent $output) (Split-Path -Leaf $output) | Out-Null
New-Item -ItemType Directory -Path $output | Out-Null
$stage = Join-Path $output 'UWorkers-Fetch'
New-Item -ItemType Directory -Path $stage | Out-Null
foreach ($name in $script:UpdateToolFiles) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $stage $name) }
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'upgrade-instructions.md') -Destination (Join-Path $stage 'UPGRADE.md')
Copy-Item -LiteralPath (Join-Path $PSScriptRoot '../../docs/windows-oss-releases.md') -Destination (Join-Path $stage 'README.md')
$sums = @(Get-ChildItem -LiteralPath $stage -File | Sort-Object Name | ForEach-Object { (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()+'  '+$_.Name })
[IO.File]::WriteAllLines((Join-Path $stage 'CHECKSUMS.sha256'),$sums,(New-Object Text.UTF8Encoding($false)))
Add-Type -AssemblyName System.IO.Compression.FileSystem
Add-Type -AssemblyName System.IO.Compression
$zip = Join-Path $output 'UWorkers-Fetch-bootstrap.zip'
$archive = [IO.Compression.ZipFile]::Open($zip,[IO.Compression.ZipArchiveMode]::Create)
try {
    # .NET Framework otherwise writes backslashes, which the receiver rejects.
    foreach ($file in Get-ReleaseFiles $stage) {
        $relative = $file.FullName.Substring($stage.Length+1).Replace('\','/')
        $null = [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive,$file.FullName,('UWorkers-Fetch/'+$relative),[IO.Compression.CompressionLevel]::Optimal)
    }
} finally { $archive.Dispose() }
Write-Host ('Bootstrap ZIP SHA-256: '+(Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant())
return $zip
