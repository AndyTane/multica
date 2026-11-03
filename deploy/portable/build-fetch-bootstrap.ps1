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
foreach ($name in @('Fetch-Upgrade.ps1','Apply-Upgrade.ps1','oss-release-common.ps1')) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $stage $name) }
Copy-Item -LiteralPath (Join-Path $PSScriptRoot '../../docs/windows-oss-releases.md') -Destination (Join-Path $stage 'README.md')
$sums = @(Get-ChildItem -LiteralPath $stage -File | Sort-Object Name | ForEach-Object { (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()+'  '+$_.Name })
[IO.File]::WriteAllLines((Join-Path $stage 'CHECKSUMS.sha256'),$sums,(New-Object Text.UTF8Encoding($false)))
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = Join-Path $output 'UWorkers-Fetch-bootstrap.zip'
[IO.Compression.ZipFile]::CreateFromDirectory($stage,$zip,[IO.Compression.CompressionLevel]::Optimal,$true)
Write-Host ('Bootstrap ZIP SHA-256: '+(Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant())
return $zip
