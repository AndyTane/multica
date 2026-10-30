$ErrorActionPreference = "Stop"
$source = Get-Content -Raw -Encoding UTF8 (Join-Path $PSScriptRoot "install.ps1")
$source = $source.Substring(0, $source.IndexOf("# Entry point"))
$env:MULTICA_INSTALL_DIR = ""
$env:MULTICA_BIN_DIR = ""
Invoke-Expression $source
if ($InstallDir -ne (Join-Path $env:USERPROFILE ".u-workers\server")) { throw "shared server directory" }
if ($CliBinary -ne (Join-Path $env:USERPROFILE ".u-workers\bin\multica.exe")) { throw "shared binary directory" }

$script:dockerCalls = @()
$script:fakeImages = @("u-workers-backend:dev", "u-workers-web:dev", "postgres:16")
function docker {
    $global:LASTEXITCODE = 0
    if ($args -contains "config") { return $script:fakeImages }
    $script:dockerCalls += ($args -join " ")
}
Pull-SelfHostImages
if ($script:dockerCalls.Count -ne 1 -or $script:dockerCalls[0] -notmatch 'build backend frontend$') { throw "local images must build" }

$script:dockerCalls = @()
$script:fakeImages = @("u-workers-backend:dev", "registry.example/web:chosen")
Pull-SelfHostImages
if ($script:dockerCalls.Count -ne 1 -or $script:dockerCalls[0] -notmatch 'build backend$') { throw "custom frontend image must be preserved" }

$script:dockerCalls = @()
$script:fakeImages = @("registry.example/backend:chosen", "registry.example/web:chosen")
Pull-SelfHostImages
if ($script:dockerCalls.Count -ne 1 -or $script:dockerCalls[0] -notmatch ' pull$') { throw "configured image pull missing" }

# An unrelated executable on PATH must not participate in installation checks.
$script:installed = $false
function multica { throw "original CLI must not execute" }
function Test-Path { return $script:installed }
function Install-CliBinary { $script:installed = $true }
Install-Cli
if (-not $script:installed) { throw "target CLI was not installed" }

# A failed release lookup must not report an existing installation as current.
function Get-InstalledCliVersion { return "1.0.0" }
function Get-LatestVersion { return $null }
function Write-Fail { param([string]$Message) throw $Message }
$failed = $false
try { Install-Cli } catch { $failed = $_.Exception.Message -like "*configured release source*" }
if (-not $failed) { throw "failed release lookup was reported as up to date" }
Write-Host "install source tests passed"
