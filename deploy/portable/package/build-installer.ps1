#Requires -Version 5.1
<#
.SYNOPSIS
    Builds the second-machine installer (u-workers-setup.exe) from the package.

.DESCRIPTION
    The portable package carries its own build inputs, so this works on a
    machine that has never had Go installed and has no internet connection:
    a Go toolchain under tools\go, the module cache under tools\gomodcache, and
    a source snapshot under payload\source\source.zip.

    The result is written to dist\ and is also usable directly from this package
    folder for other Windows machines.

.PARAMETER Version
    Version stamped into the CLI and the installer. Defaults to the package
    version from manifest.json.
#>
[CmdletBinding()]
param(
    [string]$Version = '',
    [string]$OutputDir = ''
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
. (Join-Path $root 'portable-common.ps1')

$manifest = Get-PortableManifest $root
if (-not $Version) { $Version = [string]$manifest.version }
if ($Version -notmatch '^v?\d+\.\d+\.\d+') { throw ('Version must look like v1.2.3: ' + $Version) }

$go = Join-Path $root 'tools\go\bin\go.exe'
$modCache = Join-Path $root 'tools\gomodcache'
$sourceZip = Join-Path $root 'payload\source\source.zip'
if (-not (Test-Path -LiteralPath $go)) { throw ('This package does not include the build tools (tools\go\bin\go.exe). Rebuild the package with the build kit, or use the prebuilt EXE under payload\installer.') }
if (-not (Test-Path -LiteralPath $modCache)) { throw ('The bundled Go module cache is missing: ' + $modCache) }
if (-not (Test-Path -LiteralPath $sourceZip)) { throw ('The bundled source snapshot is missing: ' + $sourceZip) }

$work = Join-Path $root 'data\build'
New-PortableDirectory $work
$sourceDir = Join-Path $work ('source-' + ($Version -replace '[^0-9A-Za-z._-]', '_'))
if (Test-Path -LiteralPath $sourceDir) { Remove-Item -LiteralPath $sourceDir -Recurse -Force }
Write-Host '==> Unpacking the bundled source snapshot'
Expand-Archive -LiteralPath $sourceZip -DestinationPath $sourceDir -Force
$serverDir = Join-Path $sourceDir 'server'
if (-not (Test-Path -LiteralPath (Join-Path $serverDir 'go.mod'))) { throw 'The bundled source snapshot does not contain server/go.mod.' }

$dist = if ($OutputDir) { $OutputDir } else { Join-Path $root 'dist' }
New-PortableDirectory $dist
$stage = Join-Path $work 'installer-payload'
New-PortableDirectory $stage
$goCache = Join-Path $work 'gocache'
New-PortableDirectory $goCache

$saved = @{}
foreach ($name in @('GOROOT', 'GOMODCACHE', 'GOPROXY', 'GOSUMDB', 'GOFLAGS', 'GOTOOLCHAIN', 'CGO_ENABLED', 'GOCACHE', 'GO111MODULE', 'GOWORK')) {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$env:GOROOT = (Join-Path $root 'tools\go')
$env:GOMODCACHE = $modCache
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOFLAGS = '-mod=mod'
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
$env:GO111MODULE = 'on'
$env:GOCACHE = $goCache
Remove-Item Env:GOWORK -ErrorAction SilentlyContinue

function Invoke-PortableGo {
    param([string[]]$Arguments)
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $go @Arguments 2>&1 | Out-String
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    if ($exitCode -ne 0) { throw ('go build failed: ' + $output.Trim()) }
    return $output
}

try {
    $linkFlags = '-s -w -X main.version=' + $Version
    Write-Host ('==> Building the U Workers CLI (' + $Version + ')')
    $primary = Join-Path $stage 'u-workers.exe'
    Invoke-PortableGo @('build', '-trimpath', '-ldflags', $linkFlags, '-o', $primary, './cmd/multica') | Out-Null
    # The installer embeds both command names; build one binary and copy it.
    $payload = Join-Path $serverDir 'cmd\u-workers-setup\payload'
    New-PortableDirectory $payload
    Copy-Item -LiteralPath $primary -Destination (Join-Path $payload 'u-workers.exe') -Force
    Copy-Item -LiteralPath $primary -Destination (Join-Path $payload 'multica.exe') -Force
    Write-Host '==> Building the single-file installer'
    $installer = Join-Path $dist ('u-workers-setup-' + $Version + '-windows-amd64.exe')
    Invoke-PortableGo @('build', '-trimpath', '-ldflags', $linkFlags, '-o', $installer, './cmd/u-workers-setup') | Out-Null
} finally {
    foreach ($name in $saved.Keys) {
        [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process')
    }
    foreach ($name in @('u-workers.exe', 'multica.exe')) {
        Remove-Item -LiteralPath (Join-Path $serverDir ('cmd\u-workers-setup\payload\' + $name)) -Force -ErrorAction SilentlyContinue
    }
}

if (-not (Test-Path -LiteralPath $installer)) { throw 'The build produced no installer file.' }
$item = Get-Item -LiteralPath $installer
Write-Host ('Installer: ' + $item.FullName)
Write-Host ('Size:      ' + [math]::Round($item.Length / 1MB, 1) + ' MB')
