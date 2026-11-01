# Package source-consistency check: prove that an isolated build of the portable
# package corresponds to one exact Git revision, that the bundled source
# snapshot is that revision's archive, and that the installer can be rebuilt
# offline from the bundled toolchain and source. Windows PowerShell 5.1
# compatible.
# Run: powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\verify-package-source.ps1 -Package <package folder> [-ExpectedCommit <sha>]
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Package,
    [string]$Repo = '',
    [string]$ExpectedCommit = '',
    [string]$OutputRoot = ''
)

$ErrorActionPreference = 'Stop'
$portableRoot = Split-Path -Parent $PSScriptRoot
$repoRoot = Split-Path -Parent (Split-Path -Parent $portableRoot)
if ($Repo) { $repoRoot = $Repo }
$repoRoot = [IO.Path]::GetFullPath($repoRoot)
$Package = [IO.Path]::GetFullPath($Package)
if (-not $OutputRoot) { $OutputRoot = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-source-check-' + [guid]::NewGuid().ToString('N')) }
$OutputRoot = [IO.Path]::GetFullPath($OutputRoot)

$checks = 0
$failed = 0
function Add-Check {
    param([string]$Name, [bool]$Ok, [string]$Detail = '')
    $script:checks++
    if ($Ok) { Write-Host ('  [PASS] ' + $Name + $(if ($Detail) { '  ' + $Detail } else { '' })) }
    else { $script:failed++; Write-Host ('  [FAIL] ' + $Name + '  ' + $Detail) -ForegroundColor Red }
}

function Test-CommitPrefix {
    # A recorded build commit may be abbreviated; compare two commit ids only when
    # one is a prefix of the other.
    param([string]$A, [string]$B)
    if (-not $A -or -not $B) { return $false }
    $a = $A.Trim().ToLowerInvariant()
    $b = $B.Trim().ToLowerInvariant()
    if ($a.Length -lt 7 -or $b.Length -lt 7) { return $false }
    return ($a.StartsWith($b) -or $b.StartsWith($a))
}

$temp = Join-Path $OutputRoot 'tmp'
New-Item -ItemType Directory -Path $temp -Force | Out-Null
try {
    Write-Host '==> Checking the build record against the checked-out revision' -ForegroundColor Cyan
    $manifestPath = Join-Path $Package 'manifest.json'
    Add-Check 'the package has a manifest' (Test-Path -LiteralPath $manifestPath)
    $manifest = Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
    $actualHead = (& git -C $repoRoot rev-parse HEAD).Trim()
    if (-not $ExpectedCommit) { $ExpectedCommit = $actualHead }
    $resolvedExpected = (& git -C $repoRoot rev-parse --verify ($ExpectedCommit + '^{commit}') 2>$null)
    if ($resolvedExpected) { $ExpectedCommit = ([string]$resolvedExpected).Trim() }
    Add-Check 'the package manifest names an explicit build commit' ([bool][string]$manifest.commit) ([string]$manifest.commit)
    Add-Check 'the package build commit is the expected revision' (Test-CommitPrefix ([string]$manifest.commit) $ExpectedCommit) ([string]$manifest.commit + ' vs ' + $ExpectedCommit)
    Add-Check 'the checked-out revision is still the expected revision' (Test-CommitPrefix $actualHead $ExpectedCommit) ($actualHead + ' vs ' + $ExpectedCommit)
    $trackedChanges = @(& git -C $repoRoot status --porcelain --untracked-files=no | Where-Object { $_ -notmatch 'apps/web/next-env\.d\.ts' })
    Add-Check 'no tracked source file is modified beyond the generated web type file' ($trackedChanges.Count -eq 0) (($trackedChanges | Select-Object -First 3) -join '; ')
    Add-Check 'the manifest carries the package version and build-kit flag' ([bool][string]$manifest.version) ([string]$manifest.version + ' buildKit=' + [string]$manifest.buildKit)

    Write-Host '==> Checking that payload/source/source.zip is the archive of that revision' -ForegroundColor Cyan
    $sourceZip = Join-Path $Package 'payload\source\source.zip'
    Add-Check 'the package bundles a source snapshot' (Test-Path -LiteralPath $sourceZip)
    $archivePath = Join-Path $temp 'expected-source.zip'
    & git -C $repoRoot archive --format=zip -o $archivePath $ExpectedCommit
    if ($LASTEXITCODE -ne 0) { throw 'git archive failed.' }
    $expectedHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash
    $actualHash = (Get-FileHash -LiteralPath $sourceZip -Algorithm SHA256).Hash
    Add-Check 'the bundled source snapshot is byte-identical to the revision archive' ($expectedHash -eq $actualHash) ($actualHash)

    Write-Host '==> Rebuilding the installer offline from the bundled toolchain and source' -ForegroundColor Cyan
    $installerDirectory = Join-Path $temp 'installer'
    New-Item -ItemType Directory -Path $installerDirectory -Force | Out-Null
    $savedEnvironment = @{}
    foreach ($name in @('HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'NO_PROXY')) { $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
    $deadProxy = 'http://127.0.0.1:9'
    $env:HTTP_PROXY = $deadProxy
    $env:HTTPS_PROXY = $deadProxy
    $env:ALL_PROXY = $deadProxy
    $env:NO_PROXY = 'localhost,127.0.0.1'
    try {
        $builder = Join-Path $Package 'build-installer.ps1'
        Add-Check 'the package bundles the offline installer builder' (Test-Path -LiteralPath $builder)
        & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $builder -OutputDir $installerDirectory
        $buildExit = $LASTEXITCODE
    } finally {
        foreach ($name in $savedEnvironment.Keys) {
            $value = $savedEnvironment[$name]
            if ($null -eq $value -or [string]$value -eq '') { Remove-Item ('Env:' + $name) -ErrorAction SilentlyContinue } else { [Environment]::SetEnvironmentVariable($name, [string]$value, 'Process') }
        }
    }
    Add-Check 'the installer rebuild exits successfully with the network disabled' ($buildExit -eq 0) ('exit ' + $buildExit)
    $rebuilt = @(Get-ChildItem -LiteralPath $installerDirectory -Filter 'u-workers-setup-*.exe' -File -ErrorAction SilentlyContinue)
    Add-Check 'the offline rebuild produced an installer' ($rebuilt.Count -eq 1) (($rebuilt | ForEach-Object { $_.Name }) -join ',')
    if ($rebuilt.Count -eq 1) {
        $rebuiltVersion = ((& $rebuilt[0].FullName --version) | Out-String).Trim()
        Add-Check 'the rebuilt installer reports the package version' ($rebuiltVersion -match [regex]::Escape([string]$manifest.version)) ($rebuiltVersion)
    }
    $payloadCli = Join-Path $Package 'payload\bin\u-workers.exe'
    Add-Check 'the package ships the CLI binary' (Test-Path -LiteralPath $payloadCli)
    if (Test-Path -LiteralPath $payloadCli) {
        $cliVersion = ((& $payloadCli --version) | Out-String).Trim()
        Add-Check 'the packaged CLI reports the package version' ($cliVersion -match [regex]::Escape([string]$manifest.version)) ($cliVersion)
    }

    Write-Host ''
    if ($failed -eq 0) {
        Write-Host ('PACKAGE SOURCE CONSISTENCY PASSED (' + $checks + ' checks)') -ForegroundColor Green
        Write-Host ('  build commit:   ' + [string]$manifest.commit)
        Write-Host ('  source.zip sha: ' + $actualHash)
        exit 0
    }
    Write-Host ('PACKAGE SOURCE CONSISTENCY FAILED (' + $failed + ' failed of ' + $checks + ')') -ForegroundColor Red
    exit 1
} finally {
    Remove-Item -LiteralPath $OutputRoot -Recurse -Force -ErrorAction SilentlyContinue
}
