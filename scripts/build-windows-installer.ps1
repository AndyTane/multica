#Requires -Version 7.0
<#
.SYNOPSIS
    Builds the single-file U Workers installer for Windows.
.DESCRIPTION
    Stages the CLI binaries built from server/cmd/multica into the installer
    package payload, builds u-workers-setup.exe with them embedded, and removes
    the staged binaries again so build outputs never land in source control.

    The result is dist/u-workers-setup-<version>-windows-amd64.exe.
.PARAMETER Version
    Version stamped into both the CLI and the installer. Defaults to
    'git describe --tags --always --dirty', falling back to "dev".
.EXAMPLE
    pwsh -NoProfile -File scripts/build-windows-installer.ps1 -Version 1.2.3
#>
[CmdletBinding()]
param(
    [string]$Version,
    [string]$GoExecutable = 'go'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$serverDir = Join-Path $repoRoot 'server'
$distDir = Join-Path $repoRoot 'dist'
$stagingDir = Join-Path $distDir 'installer-payload'
# //go:embed payload/* only reads files inside the package directory, so the
# staged binaries are copied here for the installer build.
$payloadDir = Join-Path $serverDir 'cmd/u-workers-setup/payload'

if (-not (Test-Path -LiteralPath (Join-Path $serverDir 'go.mod'))) {
    throw "Cannot find the Go module under $serverDir."
}

if (-not $Version) {
    $git = Get-Command git -ErrorAction SilentlyContinue
    if ($git) {
        $described = & $git.Source describe --tags --always --dirty 2>$null
        if ($LASTEXITCODE -eq 0 -and $described) {
            $Version = "$described".Trim()
        }
    }
    if (-not $Version) {
        $Version = 'dev'
    }
}

if ($Version -match '[\\/:*?"<>|]') {
    throw "Version '$Version' cannot be part of a file name."
}

$go = Get-Command $GoExecutable -ErrorAction SilentlyContinue
if (-not $go) {
    throw "Cannot find '$GoExecutable'. Install Go 1.26 or pass -GoExecutable <path>."
}

$primaryStaged = Join-Path $stagingDir 'u-workers.exe'
$legacyStaged = Join-Path $stagingDir 'multica.exe'
$payloadPrimary = Join-Path $payloadDir 'u-workers.exe'
$payloadLegacy = Join-Path $payloadDir 'multica.exe'
$installerPath = Join-Path $distDir "u-workers-setup-$Version-windows-amd64.exe"
$linkFlags = "-s -w -X main.version=$Version"

function Invoke-GoBuild {
    param([string[]]$Arguments)
    & $go.Source @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "go build exited with code $LASTEXITCODE."
    }
}

New-Item -ItemType Directory -Force -Path $distDir, $stagingDir, $payloadDir | Out-Null

$previousCgo = $env:CGO_ENABLED
$env:CGO_ENABLED = '0'

try {
    Write-Host "==> Building the CLI from server/cmd/multica (version $Version)"
    Push-Location -LiteralPath $serverDir
    try {
        Invoke-GoBuild @('build', '-trimpath', '-ldflags', $linkFlags, '-o', $primaryStaged, './cmd/multica')
    }
    finally {
        Pop-Location
    }

    Write-Host '==> Copying u-workers.exe to multica.exe and into the installer payload'
    Copy-Item -LiteralPath $primaryStaged -Destination $legacyStaged -Force
    Copy-Item -LiteralPath $primaryStaged -Destination $payloadPrimary -Force
    Copy-Item -LiteralPath $legacyStaged -Destination $payloadLegacy -Force

    Write-Host '==> Building the installer with the payload embedded'
    Push-Location -LiteralPath $serverDir
    try {
        Invoke-GoBuild @('build', '-trimpath', '-ldflags', $linkFlags, '-o', $installerPath, './cmd/u-workers-setup')
    }
    finally {
        Pop-Location
    }
}
finally {
    # The payload is a build input, never a source file: leave only README.md.
    foreach ($path in @($payloadPrimary, $payloadLegacy)) {
        Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
    }
    Remove-Item -LiteralPath $stagingDir -Recurse -Force -ErrorAction SilentlyContinue
    $env:CGO_ENABLED = $previousCgo
}

if (-not (Test-Path -LiteralPath $installerPath)) {
    throw "The installer build produced no file at $installerPath."
}

$installer = Get-Item -LiteralPath $installerPath
Write-Host "Installer: $($installer.FullName)"
Write-Host "Size: $($installer.Length) bytes"
