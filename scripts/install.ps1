# U Workers installer for Windows - one command to get started.
#
# Install CLI from your explicitly configured release source
#   powershell -File scripts/install.ps1
#
# Self-host: starts a local U Workers server + installs CLI + configures
#   $env:MULTICA_MODE="local"; powershell -File scripts/install.ps1
#

$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
$RepoUrl       = $env:MULTICA_REPO_URL
$RepoWebUrl    = $env:MULTICA_REPO_WEB_URL
$ReleaseApiUrl = $env:MULTICA_RELEASE_API_BASE_URL
$BinDir = if ($env:MULTICA_BIN_DIR) { $env:MULTICA_BIN_DIR } else { Join-Path $env:USERPROFILE ".u-workers\bin" }
$CliBinary = Join-Path $BinDir "multica.exe"
$DefaultInstallDir = Join-Path $env:USERPROFILE ".u-workers\server"
$InstallDir    = if ($env:MULTICA_INSTALL_DIR) { $env:MULTICA_INSTALL_DIR } else { $DefaultInstallDir }

# Host ports Compose reported after `up -d`; set by Setup-Server and reused by
# the summary so the health check and the printed URLs cannot diverge.
$script:SelfHostBackendPort  = $null
$script:SelfHostFrontendPort = $null

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
function Write-Info  { param([string]$Msg) Write-Host "==> $Msg" -ForegroundColor Cyan }
function Write-Ok    { param([string]$Msg) Write-Host "[OK] $Msg" -ForegroundColor Green }
function Write-Warn  { param([string]$Msg) Write-Warning $Msg }
function Write-Fail  { param([string]$Msg) Write-Host "[ERROR] $Msg" -ForegroundColor Red; exit 1 }

function Test-CommandExists {
    param([string]$Name)
    $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function New-RandomHex {
    param([int]$ByteCount)

    $bytes = New-Object byte[] $ByteCount
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    } finally {
        $rng.Dispose()
    }
    return -join ($bytes | ForEach-Object { "{0:x2}" -f $_ })
}

# Host port Docker Compose actually published for a service.
#
# This is the only authority. Compose's interpolation gives the calling process
# environment precedence over .env, so an ambient PORT / BACKEND_PORT / API_PORT
# / SERVER_PORT / FRONTEND_PORT moves the published port without touching the
# file. Re-deriving the port from .env alone made the installer probe and print
# a port the stack was never published on (#6145). Must be called from the
# installation directory, after `up -d`.
function Get-ComposePublishedPort {
    param(
        [Parameter(Mandatory = $true)][string]$Service,
        [Parameter(Mandatory = $true)][int]$ContainerPort
    )

    $output = $null
    try {
        $output = docker compose -f docker-compose.selfhost.yml port $Service $ContainerPort 2>$null
    } catch {
        return $null
    }
    if ($LASTEXITCODE -ne 0) {
        return $null
    }

    $line = @($output | Where-Object { $_ }) | Select-Object -Last 1
    if (-not $line) {
        return $null
    }

    $published = ($line -split ":")[-1].Trim()
    if ($published -notmatch '^[0-9]+$') {
        return $null
    }
    return $published
}

function Get-LatestVersion {
    if (-not $ReleaseApiUrl) { return $null }
    try {
        $release = Invoke-RestMethod -Uri "$ReleaseApiUrl/latest" -ErrorAction Stop
        return $release.tag_name
    } catch {
        return $null
    }
}

function Get-SelfHostRef {
    if ($env:MULTICA_SELFHOST_REF) {
        return $env:MULTICA_SELFHOST_REF
    }

    $latest = Get-LatestVersion
    if ($latest) {
        return $latest
    }

    return "main"
}

function Checkout-ServerRef {
    param([string]$Ref)

    if ($Ref -eq "main") {
        git fetch origin main --depth 1 2>$null
        git checkout --force main 2>$null
        git reset --hard origin/main 2>$null
        return
    }

    git fetch origin --tags --force 2>$null
    $tagRef = "refs/tags/$Ref"
    git show-ref --verify --quiet $tagRef 2>$null
    if ($LASTEXITCODE -eq 0) {
        git checkout --force $Ref 2>$null
        return
    }

    git fetch origin $Ref --depth 1 2>$null
    git checkout --force $Ref 2>$null
}

function Pull-SelfHostImages {
    $images = @(docker compose -f docker-compose.selfhost.yml config --images)
    if ($LASTEXITCODE -ne 0 -or $images.Count -eq 0) {
        Write-Fail "Could not resolve your server image configuration."
    }
    $buildServices = @()
    if ($images -contains "u-workers-backend:dev") { $buildServices += "backend" }
    if ($images -contains "u-workers-web:dev") { $buildServices += "frontend" }
    if ($buildServices.Count -gt 0) {
        Write-Info "Building local server images from your selected source..."
        docker compose -f docker-compose.selfhost.yml -f docker-compose.selfhost.build.yml build @buildServices
        if ($LASTEXITCODE -ne 0) { Write-Fail "Could not build local server images." }
        return
    }
    docker compose -f docker-compose.selfhost.yml pull
    if ($LASTEXITCODE -ne 0) { Write-Fail "Could not pull your configured server images." }
}

function Convert-ToCliArch {
    param([object]$Value)

    if ($null -eq $Value) {
        return $null
    }

    $normalized = "$Value".Trim().ToUpperInvariant()
    switch ($normalized) {
        "9"      { return "amd64" }
        "AMD64"  { return "amd64" }
        "X64"    { return "amd64" }
        "X86_64" { return "amd64" }
        "12"     { return "arm64" }
        "ARM64"  { return "arm64" }
        "AARCH64" { return "arm64" }
        default  { return $null }
    }
}

function Get-WindowsCliArch {
    $signals = @()
    $nativeArchSignalFound = $false

    # Prefer the native processor architecture over the current PowerShell
    # process architecture. This keeps Windows on ARM from being misdetected
    # when PowerShell is running through x64/x86 emulation.
    try {
        if (Get-Command Get-CimInstance -ErrorAction SilentlyContinue) {
            $processorArch = Get-CimInstance -ClassName Win32_Processor -ErrorAction Stop |
                Select-Object -First 1 -ExpandProperty Architecture
            $signals += [pscustomobject]@{ Source = "Win32_Processor.Architecture"; Value = $processorArch }
            $nativeArchSignalFound = $true
        }
    } catch {}

    try {
        if (-not $nativeArchSignalFound -and (Get-Command Get-WmiObject -ErrorAction SilentlyContinue)) {
            $processorArch = Get-WmiObject -Class Win32_Processor -ErrorAction Stop |
                Select-Object -First 1 -ExpandProperty Architecture
            $signals += [pscustomobject]@{ Source = "Win32_Processor.Architecture"; Value = $processorArch }
            $nativeArchSignalFound = $true
        }
    } catch {}

    try {
        $signals += [pscustomobject]@{
            Source = "RuntimeInformation.OSArchitecture"
            Value = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
        }
    } catch {}

    $signals += [pscustomobject]@{ Source = "PROCESSOR_ARCHITEW6432"; Value = $env:PROCESSOR_ARCHITEW6432 }
    $signals += [pscustomobject]@{ Source = "PROCESSOR_ARCHITECTURE"; Value = $env:PROCESSOR_ARCHITECTURE }

    foreach ($signal in $signals) {
        $arch = Convert-ToCliArch $signal.Value
        if ($arch) {
            return $arch
        }
    }

    $details = ($signals |
        Where-Object { $null -ne $_.Value -and "$($_.Value)".Trim() -ne "" } |
        ForEach-Object { "$($_.Source)=$($_.Value)" }) -join ", "
    if (-not $details) {
        $details = "no architecture signals available"
    }

    Write-Fail "Unsupported Windows architecture ($details). Only x64 and ARM64 are supported."
}

function Get-InstalledCliVersion {
    try {
        $firstLine = & $CliBinary version 2>$null | Select-Object -First 1
        if ("$firstLine" -match '\b(v?\d+(?:\.\d+)+)\b') {
            $version = $Matches[1]
            if ($version -notlike 'v*') {
                $version = "v$version"
            }
            return $version
        }
    } catch {}

    return $null
}

# ---------------------------------------------------------------------------
# CLI Installation
# ---------------------------------------------------------------------------
function Install-CliBinary {
    if (-not $ReleaseApiUrl -or -not $RepoWebUrl) {
        Write-Fail "Set MULTICA_RELEASE_API_BASE_URL and MULTICA_REPO_WEB_URL to your release source."
    }
    Write-Info "Installing U Workers CLI from GitHub Releases..."

    if (-not [Environment]::Is64BitOperatingSystem) {
        Write-Fail "U Workers requires a 64-bit Windows installation."
    }

    $arch = Get-WindowsCliArch

    $latest = Get-LatestVersion
    if (-not $latest) {
        Write-Fail "Could not determine latest release. Check your network connection."
    }

    $version = $latest.TrimStart('v')
    $url = "$RepoWebUrl/releases/download/$latest/multica-cli-$version-windows-$arch.zip"
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("u-workers-install-" + [guid]::NewGuid().ToString("N"))

    New-Item -ItemType Directory -Path $tmpDir | Out-Null

    Write-Info "Downloading $url ..."
    try {
        Invoke-WebRequest -Uri $url -OutFile (Join-Path $tmpDir "multica.zip") -UseBasicParsing
    } catch {
        Remove-Item $tmpDir -Recurse -Force
        Write-Fail "Failed to download CLI binary: $_"
    }

    # Verify SHA256 checksum
    $checksumUrl = "$RepoWebUrl/releases/download/$latest/checksums.txt"
    try {
        $checksums = Invoke-WebRequest -Uri $checksumUrl -UseBasicParsing -ErrorAction Stop
        $checksumContent = if ($checksums.Content -is [byte[]]) {
            [System.Text.Encoding]::UTF8.GetString($checksums.Content)
        } else {
            [string]$checksums.Content
        }
        $zipFile = Join-Path $tmpDir "multica.zip"
        $actualHash = (Get-FileHash -Path $zipFile -Algorithm SHA256).Hash.ToLower()
        $releaseAsset = "multica-cli-$version-windows-$arch.zip"
        $legacyAsset = "multica_windows_$arch.zip"
        $expectedLine = ($checksumContent -split "`r?`n") |
            Where-Object {
                $_ -match [regex]::Escape($releaseAsset) -or
                $_ -match [regex]::Escape($legacyAsset)
            } |
            Select-Object -First 1
        if ($expectedLine) {
            $expectedHash = ($expectedLine -split "\s+")[0].ToLower()
            if ($actualHash -ne $expectedHash) {
                Remove-Item $tmpDir -Recurse -Force
                Write-Fail "Checksum verification failed. Expected: $expectedHash, Got: $actualHash"
            }
            Write-Ok "Checksum verified"
        } else {
            Write-Warn "Could not find checksum entry for $releaseAsset - skipping verification."
        }
    } catch {
        Write-Warn "Could not download checksums.txt - skipping verification."
    }

    Expand-Archive -Path (Join-Path $tmpDir "multica.zip") -DestinationPath $tmpDir -Force

    if (-not (Test-Path $binDir)) {
        New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    }

    $exeSrc = Join-Path $tmpDir "multica.exe"
    if (-not (Test-Path $exeSrc)) {
        $exeSrc = Get-ChildItem -Path $tmpDir -Filter "multica.exe" -Recurse | Select-Object -First 1 -ExpandProperty FullName
    }
    if (-not $exeSrc -or -not (Test-Path $exeSrc)) {
        Remove-Item $tmpDir -Recurse -Force
        Write-Fail "multica.exe not found in downloaded archive."
    }

    Copy-Item $exeSrc (Join-Path $binDir "multica.exe") -Force
    Remove-Item $tmpDir -Recurse -Force

    Write-Ok "U Workers CLI installed to $binDir\multica.exe"
}

function Install-Cli {
    if (Test-Path -LiteralPath $CliBinary -PathType Leaf) {
        $currentVer = Get-InstalledCliVersion
        $latestVer = Get-LatestVersion
        if (-not $latestVer) { Write-Fail "Could not check your configured release source." }

        $currentCmp = if ($currentVer) { $currentVer -replace '^v','' } else { $null }
        $latestCmp = if ($latestVer) { $latestVer -replace '^v','' } else { $null }

        $isUpToDate = $false
        if (-not $isUpToDate) {
            try {
                $isUpToDate = $currentCmp -and $latestCmp -and ([System.Version]$currentCmp -ge [System.Version]$latestCmp)
            } catch {
                $isUpToDate = $currentCmp -and $latestCmp -and ($currentCmp -eq $latestCmp)
            }
        }

        if ($isUpToDate) {
            Write-Ok "U Workers CLI is up to date ($currentVer)"
            return
        }

        Write-Info "U Workers CLI $currentVer installed, latest is $latestVer - upgrading..."
        Install-CliBinary

        $newVer = Get-InstalledCliVersion
        Write-Ok "U Workers CLI upgraded ($currentVer -> $newVer)"
        return
    }

    Install-CliBinary

    if (-not (Test-Path -LiteralPath $CliBinary -PathType Leaf)) {
        Write-Fail "CLI binary was not created at the configured target directory."
    }
}

# ---------------------------------------------------------------------------
# Docker check
# ---------------------------------------------------------------------------
function Test-Docker {
    if (-not (Test-CommandExists "docker")) {
        Write-Fail @"
Docker is not installed. U Workers self-hosting requires Docker and Docker Compose.

Install Docker Desktop for Windows:
  https://docs.docker.com/desktop/install/windows-install/

After installing Docker, re-run this script with `$env:MULTICA_MODE="local"`.
"@
    }

    try {
        docker info 2>$null | Out-Null
    } catch {
        Write-Fail "Docker is installed but not running. Please start Docker Desktop and re-run this script."
    }

    Write-Ok "Docker is available"
}

# ---------------------------------------------------------------------------
# Server setup (self-host / local)
# ---------------------------------------------------------------------------
function Install-Server {
    if (-not $RepoUrl) { Write-Fail "Set MULTICA_REPO_URL to your source repository." }
    Write-Info "Setting up U Workers server..."
    $serverRef = Get-SelfHostRef
    Write-Info "Using self-host assets from $serverRef..."

    if (Test-Path (Join-Path $InstallDir ".git")) {
        Write-Info "Updating existing installation at $InstallDir..."
        Write-Warn "Any local changes in $InstallDir will be overwritten."
    } else {
        Write-Info "Cloning U Workers repository..."
        if (-not (Test-CommandExists "git")) {
            Write-Fail "Git is not installed. Please install git and re-run."
        }
        if (Test-Path $InstallDir) {
            Write-Warn "Removing incomplete installation at $InstallDir..."
            Remove-Item $InstallDir -Recurse -Force
        }
        $parentDir = Split-Path $InstallDir -Parent
        if (-not (Test-Path $parentDir)) {
            New-Item -ItemType Directory -Path $parentDir -Force | Out-Null
        }
        git clone --depth 1 $RepoUrl $InstallDir
    }

    Push-Location $InstallDir
    Checkout-ServerRef $serverRef
    Write-Ok "Repository ready at $InstallDir ($serverRef)"

    if (-not (Test-Path ".env")) {
        Write-Info "Creating .env with random secrets..."
        Copy-Item ".env.example" ".env"
        $jwt = New-RandomHex 32
        $pgpass = New-RandomHex 24
        $content = Get-Content ".env"
        $content = $content -replace '^JWT_SECRET=.*', "JWT_SECRET=$jwt"
        $content = $content -replace '^POSTGRES_PASSWORD=.*', "POSTGRES_PASSWORD=$pgpass"
        $content = $content -replace '^(DATABASE_URL=postgres://[^:]+:)[^@]*(@.*)', "`${1}$pgpass`${2}"
        $content | Set-Content ".env"
        Write-Ok "Generated .env with random JWT_SECRET and POSTGRES_PASSWORD"
    } else {
        Write-Ok "Using existing .env"
    }

    Write-Info "Pulling images configured by your selected server source..."
    Pull-SelfHostImages
    Write-Info "Starting U Workers services (this may take a few minutes on first run)..."
    docker compose -f docker-compose.selfhost.yml up -d

    # Read the ports Compose actually published, once, and reuse them for both
    # the health check and the summary so the two can never disagree.
    $script:SelfHostBackendPort = Get-ComposePublishedPort -Service "backend" -ContainerPort 8080
    if (-not $script:SelfHostBackendPort) {
        Write-Fail "Started the stack but could not read the backend host port from Docker Compose.`n  Check it with: cd $InstallDir; docker compose -f docker-compose.selfhost.yml ps"
    }
    $script:SelfHostFrontendPort = Get-ComposePublishedPort -Service "frontend" -ContainerPort 3000
    if (-not $script:SelfHostFrontendPort) {
        Write-Fail "Started the stack but could not read the frontend host port from Docker Compose.`n  Check it with: cd $InstallDir; docker compose -f docker-compose.selfhost.yml ps"
    }

    Write-Info "Waiting for backend to be ready..."
    $ready = $false
    for ($i = 1; $i -le 45; $i++) {
        try {
            $null = Invoke-WebRequest -Uri "http://localhost:$($script:SelfHostBackendPort)/health" -UseBasicParsing -TimeoutSec 2
            $ready = $true
            break
        } catch {
            Start-Sleep -Seconds 2
        }
    }

    if ($ready) {
        Write-Ok "U Workers server is running"
    } else {
        Write-Warn "Server is still starting. Check logs with:"
        Write-Host "  cd $InstallDir; docker compose -f docker-compose.selfhost.yml logs"
    }

    Pop-Location
}


# ---------------------------------------------------------------------------
# Main: Default mode (cloud)
# ---------------------------------------------------------------------------
function Start-DefaultInstall {
    Write-Host ""
    Write-Host "  U Workers - Installer" -ForegroundColor White
    Write-Host ""

    Install-Cli

    Write-Host ""
    Write-Host "  ============================================" -ForegroundColor Green
    Write-Host "  [OK] U Workers CLI is ready!" -ForegroundColor Green
    Write-Host "  ============================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Next: configure your environment"
    Write-Host ""
    Write-Host "     & `"$CliBinary`" --profile u-workers setup self-host --server-url <YOUR_SERVER> --app-url <YOUR_APP>"
    Write-Host ""
    Write-Host "  Self-hosting? Install the server first:"
    Write-Host '     $env:MULTICA_MODE="with-server"; powershell -File scripts/install.ps1'
    Write-Host ""
}

# ---------------------------------------------------------------------------
# Main: Local mode (self-host)
# ---------------------------------------------------------------------------
function Start-LocalInstall {
    Write-Host ""
    Write-Host "  U Workers - Self-Host Installer" -ForegroundColor White
    Write-Host "  Provisioning server infrastructure + installing CLI"
    Write-Host ""

    Test-Docker
    Install-Server
    Install-Cli

    Write-Host ""
    Write-Host "  ============================================" -ForegroundColor Green
    Write-Host "  [OK] U Workers server is running and CLI is ready!" -ForegroundColor Green
    Write-Host "  ============================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Frontend:  http://localhost:$($script:SelfHostFrontendPort)"
    Write-Host "  Backend:   http://localhost:$($script:SelfHostBackendPort)"
    Write-Host "  Server at: $InstallDir"
    Write-Host ""
    Write-Host "  Next: configure your CLI to connect"
    Write-Host ""
    Write-Host "     & `"$CliBinary`" --profile u-workers setup self-host --server-url http://localhost:$($script:SelfHostBackendPort) --app-url http://localhost:$($script:SelfHostFrontendPort)"
    Write-Host ""
    Write-Host "  Login: configure RESEND_API_KEY in .env for email codes,"
    Write-Host "  or read the generated code from backend logs when Resend is unset."
    Write-Host ""
    Write-Host "  To stop all services:"
    Write-Host '     $env:MULTICA_MODE="stop"; powershell -File scripts/install.ps1'
    Write-Host ""
}

# ---------------------------------------------------------------------------
# Stop: shut down a self-hosted installation
# ---------------------------------------------------------------------------
function Start-Stop {
    Write-Host ""
    Write-Info "Stopping U Workers services..."

    if (Test-Path $InstallDir) {
        Push-Location $InstallDir
        if (Test-Path "docker-compose.selfhost.yml") {
            docker compose -f docker-compose.selfhost.yml down
            Write-Ok "Docker services stopped"
        } else {
            Write-Warn "No docker-compose.selfhost.yml found at $InstallDir"
        }
        Pop-Location
    } else {
        Write-Warn "No U Workers installation found at $InstallDir"
    }

    if (Test-Path -LiteralPath $CliBinary -PathType Leaf) {
        try {
            & $CliBinary --profile u-workers daemon stop 2>$null
            Write-Ok "Daemon stopped"
        } catch {}
    }

    Write-Host ""
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
$mode = if ($env:MULTICA_MODE) { $env:MULTICA_MODE.ToLower() } else { "default" }

if ($mode -ne "stop") {
    if (-not $ReleaseApiUrl -or -not $RepoWebUrl) {
        Write-Fail "Set MULTICA_RELEASE_API_BASE_URL (releases endpoint) and MULTICA_REPO_WEB_URL (repository web URL) before installing."
    }
    if (($mode -eq "with-server" -or $mode -eq "local") -and -not $RepoUrl) {
        Write-Fail "Set MULTICA_REPO_URL to your source repository before provisioning."
    }
}

switch ($mode) {
    "with-server" { Start-LocalInstall }
    "local"       { Start-LocalInstall }  # backwards compat alias
    "stop"        { Start-Stop }
    default       { Start-DefaultInstall }
}
