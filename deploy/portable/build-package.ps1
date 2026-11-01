#Requires -Version 7.0
<#
.SYNOPSIS
    Builds the portable U Workers deployment package on a Windows build machine.

.DESCRIPTION
    Produces one folder (and optionally a zip) that a Windows machine without
    administrator rights, developer tools or internet access can run with a
    single script:

      UWorkers-Portable-<version>\
        Setup.cmd                        one-step install (no admin, no downloads)
        payload\release                  prebuilt API, migrations, standalone web
        payload\bin                      u-workers CLI/daemon
        payload\runtime                  node.exe, PostgreSQL, Caddy, opencode
        payload\installer                prebuilt u-workers-setup EXE
        payload\source\source.zip        source snapshot used to rebuild it
        tools\go, tools\gomodcache       go toolchain + module cache (offline build)
        manifest.json                    version, commit, checksums

    Data, logs, backups, the deployment config and the private CA are created by
    Setup.cmd inside the same folder, so the folder itself is the deployment and
    can be moved or upgraded in place.

    Build prerequisites (only on the build machine): Windows PowerShell 7, Go,
    Node.js and pnpm with the repository dependencies installed. Run this from a
    clean commit: the source snapshot is taken with 'git archive HEAD'.

.PARAMETER Version
    Release version used by the native deployment (must look like v1.2.3).

.PARAMETER OutputRoot
    Where the package folder is written. Default: <repo>\dist.

.PARAMETER PostgresDirectory
    An existing PostgreSQL Windows binary directory (the pgsql folder from the
    official EDB archive, or an existing deployment's runtime\pgsql). Either this
    or -PgArchive is required.

.PARAMETER PgArchive
    Official PostgreSQL Windows binaries zip, checksum-verified with
    -PgArchiveSha256. Ignored when -PostgresDirectory is given.

.PARAMETER CaddyExe
    caddy.exe to bundle (official Caddy release binary).

.PARAMETER OpenCodeExe
    opencode.exe to bundle. When omitted the npm-installed opencode is located.

.PARAMETER ReusePayloadFrom
    Copy an existing package's payload instead of rebuilding. Useful to label the
    same binaries as a new version for upgrade rehearsals; a real release must be
    built from source.

.PARAMETER SkipBuildKit
    Do not bundle the Go toolchain, module cache and source snapshot. The package
    still runs a server and ships the prebuilt installer, but cannot rebuild the
    installer offline.

.PARAMETER NoZip
    Do not create the .zip archive next to the folder.

.PARAMETER Force
    Replace an existing package folder of the same version.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [string]$OutputRoot = '',
    [string]$PostgresDirectory,
    [string]$PgArchive,
    [string]$PgArchiveSha256,
    [string]$NodeExe,
    [string]$GoExecutable = 'go',
    [string]$PnpmCmd,
    [string]$GoModCache,
    [string]$CaddyExe,
    [string]$OpenCodeExe,
    [string]$ReusePayloadFrom,
    [switch]$SkipBuildKit,
    [switch]$NoZip,
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
$portableRoot = $PSScriptRoot
$repoRoot = Split-Path -Parent (Split-Path -Parent $portableRoot)
if (-not (Test-Path -LiteralPath (Join-Path $repoRoot 'server\go.mod'))) {
    throw ('This script must live at deploy\portable\ inside the repository checkout: ' + $portableRoot)
}
if ($Version -notmatch '^v\d+\.\d+\.\d+') { throw ('Version must look like v1.2.3: ' + $Version) }
if (-not $OutputRoot) { $OutputRoot = Join-Path $repoRoot 'dist' }

function Write-Step {
    param([string]$Text)
    Write-Host ''
    Write-Host ('==> ' + $Text) -ForegroundColor Cyan
}

function Invoke-BuildCommand {
    param([string]$Executable, [string[]]$Arguments, [string]$WorkingDirectory)
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        if ($WorkingDirectory) { Push-Location -LiteralPath $WorkingDirectory }
        try { $output = & $Executable @Arguments 2>&1 | Out-String } finally { if ($WorkingDirectory) { Pop-Location } }
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    if ($exitCode -ne 0) { throw ($Executable + ' failed: ' + $output.Trim()) }
    return $output
}

function Copy-BuildTree {
    param([string]$Source, [string]$Destination)
    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    # robocopy signals "files copied" with exit code 1..7; only 8+ is a failure.
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & robocopy.exe $Source $Destination /E /SJ /SL /R:2 /W:1 /MT:4 /NP /NFL /NDL /NJH /NJS | Out-Null
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    if ($exitCode -ge 8) { throw ('Copying ' + $Source + ' failed (robocopy exit ' + $exitCode + ').') }
}

function Resolve-RequiredFile {
    param([string]$Path, [string]$Description)
    if (-not (Test-Path -LiteralPath $Path)) { throw ($Description + ' not found: ' + $Path) }
    return (Resolve-Path -LiteralPath $Path).Path
}

function Remove-PortableUnneededPostgresParts {
    # The official PostgreSQL Windows archive also ships pgAdmin 4, StackBuilder,
    # C headers and debug symbols. A deployment only runs the server and tools,
    # so dropping those parts saves several hundred megabytes per package.
    param([string]$PostgresDirectory)
    foreach ($relative in @('pgAdmin 4', 'StackBuilder', 'doc', 'include', 'symbols')) {
        $path = Join-Path $PostgresDirectory $relative
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Recurse -Force }
    }
}

function Find-OpenCodeExecutable {
    if ($OpenCodeExe) { return (Resolve-RequiredFile $OpenCodeExe 'opencode.exe') }
    $command = Get-Command opencode.cmd -ErrorAction SilentlyContinue
    if ($command) {
        $candidate = Join-Path (Split-Path -Parent $command.Source) 'node_modules\opencode-ai\bin\opencode.exe'
        if (Test-Path -LiteralPath $candidate) { return $candidate }
    }
    $command = Get-Command opencode.exe -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    throw 'opencode.exe not found. Pass -OpenCodeExe <path> (the opencode CLI is bundled so a runtime appears on the board).'
}

function Find-CaddyExecutable {
    if ($CaddyExe) { return (Resolve-RequiredFile $CaddyExe 'caddy.exe') }
    $command = Get-Command caddy.exe -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    throw 'caddy.exe not found. Download the official Caddy Windows build and pass -CaddyExe <path>.'
}

$go = (Get-Command $GoExecutable -ErrorAction Stop).Source
if (-not $NodeExe) { $NodeExe = (Get-Command node.exe -ErrorAction Stop).Source }
$NodeExe = Resolve-RequiredFile $NodeExe 'node.exe'
if (-not $PnpmCmd) {
    $pnpm = Get-Command pnpm.cmd -ErrorAction SilentlyContinue
    if (-not $pnpm) { $pnpm = Get-Command pnpm -ErrorAction Stop }
    $PnpmCmd = $pnpm.Source
}
if (-not $GoModCache) {
    $GoModCache = ((& $go env GOMODCACHE) | Out-String).Trim()
}
if (-not (Test-Path -LiteralPath $GoModCache)) { throw ('Go module cache not found: ' + $GoModCache) }
$nodeVersion = ((& $NodeExe --version) | Out-String).Trim()
$caddy = Find-CaddyExecutable
$opencode = Find-OpenCodeExecutable
$git = (Get-Command git.exe -ErrorAction Stop).Source
$commit = ((& $git -C $repoRoot rev-parse --short HEAD) | Out-String).Trim()
$dirty = ((& $git -C $repoRoot status --porcelain) | Out-String).Trim()
if ($dirty) { Write-Host 'Warning: the working tree has uncommitted changes; the bundled source snapshot is HEAD, not the working tree.' -ForegroundColor Yellow }

New-Item -ItemType Directory -Path $OutputRoot -Force | Out-Null
$packageDir = Join-Path $OutputRoot ('UWorkers-Portable-' + $Version)
if (Test-Path -LiteralPath $packageDir) {
    if (-not $Force) { throw ('The package folder already exists: ' + $packageDir + '. Pass -Force to replace it.') }
    Remove-Item -LiteralPath $packageDir -Recurse -Force
}
New-Item -ItemType Directory -Path $packageDir -Force | Out-Null
$work = Join-Path $OutputRoot ('portable-build-' + $Version)
if (Test-Path -LiteralPath $work) { Remove-Item -LiteralPath $work -Recurse -Force }
New-Item -ItemType Directory -Path $work -Force | Out-Null

$payload = Join-Path $packageDir 'payload'
foreach ($relative in @('release', 'bin', 'installer', 'source', 'scripts', 'runtime')) {
    New-Item -ItemType Directory -Path (Join-Path $payload $relative) -Force | Out-Null
}

if ($ReusePayloadFrom) {
    Write-Step ('Reusing the payload of ' + $ReusePayloadFrom)
    $reuse = Resolve-RequiredFile $ReusePayloadFrom 'Reused package'
    foreach ($relative in @('release', 'bin', 'runtime', 'installer', 'source')) {
        $from = Join-Path $reuse ('payload\' + $relative)
        if (Test-Path -LiteralPath $from) { Copy-BuildTree $from (Join-Path $payload $relative) }
    }
} else {
    if (-not $PostgresDirectory -and -not $PgArchive) { throw 'Pass -PostgresDirectory <pgsql dir> or -PgArchive <zip> -PgArchiveSha256 <hash>.' }
    Write-Step 'Preparing a scratch native deployment (configuration only)'
    $buildRoot = Join-Path $work 'deployment'
    New-Item -ItemType Directory -Path $buildRoot -Force | Out-Null
    if ($PostgresDirectory) {
        $postgres = Resolve-RequiredFile $PostgresDirectory 'PostgreSQL directory'
        Copy-BuildTree $postgres (Join-Path $buildRoot 'runtime\pgsql')
    }
    $nativeScript = Join-Path $repoRoot 'scripts\native-windows.ps1'
    $prepareArgs = @('-Action', 'Prepare', '-Clean', '-Root', $buildRoot, '-Version', $Version, '-Database', 'uworkers',
        '-DatabasePort', '16433', '-ApiPort', '18080', '-WebPort', '13000', '-WebHost', '127.0.0.1',
        '-PublicOrigin', 'https://localhost:13443', '-CorsOrigins', 'https://localhost:13443',
        '-NodeExe', $NodeExe, '-GoExe', $go, '-PnpmCmd', $PnpmCmd, '-GoModCache', $GoModCache)
    if ($PgArchive) {
        $prepareArgs += @('-PgArchive', (Resolve-RequiredFile $PgArchive 'PostgreSQL archive'))
        if (-not $PgArchiveSha256) { throw 'Pass -PgArchiveSha256 with -PgArchive.' }
        $prepareArgs += @('-PgArchiveSha256', $PgArchiveSha256)
    }
    Invoke-BuildCommand -Executable (Join-Path $PSHOME 'pwsh.exe') -Arguments (@('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $nativeScript) + $prepareArgs) | Out-Null
    Write-Step 'Building the server, migrations and standalone web app from source'
    Invoke-BuildCommand -Executable (Join-Path $PSHOME 'pwsh.exe') -Arguments @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $nativeScript, '-Action', 'Build', '-Root', $buildRoot) | Write-Host
    $release = Get-ChildItem -LiteralPath (Join-Path $buildRoot 'releases') -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $release) { throw 'The native build produced no release directory.' }
    Write-Step 'Assembling the package payload'
    Copy-BuildTree $release.FullName (Join-Path $payload 'release')
    Copy-BuildTree (Join-Path $buildRoot 'runtime\pgsql') (Join-Path $payload 'runtime\pgsql')
    Write-Step 'Building the u-workers CLI for the package'
    $env:CGO_ENABLED = '0'
    try {
        Invoke-BuildCommand -Executable $go -Arguments @('build', '-trimpath', '-ldflags', ('-s -w -X main.version=' + $Version), '-o', (Join-Path $payload 'bin\u-workers.exe'), './cmd/multica') -WorkingDirectory (Join-Path $repoRoot 'server') | Out-Null
    } finally {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
    Copy-Item -LiteralPath (Join-Path $payload 'bin\u-workers.exe') -Destination (Join-Path $payload 'bin\multica.exe') -Force
    Write-Step 'Building the prebuilt second-machine installer'
    & (Join-Path $repoRoot 'scripts\build-windows-installer.ps1') -Version $Version -GoExecutable $go | Write-Host
    $installer = Get-ChildItem -LiteralPath (Join-Path $repoRoot 'dist') -Filter ('u-workers-setup-' + $Version + '-windows-amd64.exe') | Select-Object -First 1
    if (-not $installer) { throw 'The installer build produced no file.' }
    Copy-Item -LiteralPath $installer.FullName -Destination (Join-Path $payload ('installer\u-workers-setup-' + $Version + '-windows-amd64.exe')) -Force
}

Remove-PortableUnneededPostgresParts (Join-Path $payload 'runtime\pgsql')

if (-not $SkipBuildKit) {
    Write-Step 'Bundling the source snapshot and the offline build kit'
    $sourceZip = Join-Path $payload 'source\source.zip'
    if (-not (Test-Path -LiteralPath $sourceZip)) {
        Invoke-BuildCommand -Executable $git -Arguments @('-C', $repoRoot, 'archive', '--format=zip', ('--output=' + $sourceZip), 'HEAD') | Out-Null
    }
    $goRoot = ((& $go env GOROOT) | Out-String).Trim()
    if (-not (Test-Path -LiteralPath (Join-Path $packageDir 'tools\go\bin\go.exe'))) {
        Write-Host ('  Go toolchain: ' + $goRoot)
        Copy-BuildTree $goRoot (Join-Path $packageDir 'tools\go')
    }
    if (-not (Test-Path -LiteralPath (Join-Path $packageDir 'tools\gomodcache'))) {
        Write-Host ('  Go module cache: ' + $GoModCache)
        Copy-BuildTree $GoModCache (Join-Path $packageDir 'tools\gomodcache')
    }
}

# Runtime pieces that make the package self-contained.
Write-Step 'Adding the bundled runtime (node, Caddy, opencode, server runner)'
Copy-Item -LiteralPath $NodeExe -Destination (Join-Path $payload 'runtime\node.exe') -Force
New-Item -ItemType Directory -Path (Join-Path $payload 'runtime\caddy') -Force | Out-Null
Copy-Item -LiteralPath $caddy -Destination (Join-Path $payload 'runtime\caddy\caddy.exe') -Force
New-Item -ItemType Directory -Path (Join-Path $payload 'runtime\opencode') -Force | Out-Null
Copy-Item -LiteralPath $opencode -Destination (Join-Path $payload 'runtime\opencode\opencode.exe') -Force
New-Item -ItemType Directory -Path (Join-Path $payload 'scripts') -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\native-windows.ps1') -Destination (Join-Path $payload 'scripts\native-windows.ps1') -Force

Write-Step 'Copying the package scripts'
foreach ($name in @('portable-common.ps1', 'install.ps1', 'start.ps1', 'stop.ps1', 'status.ps1', 'uninstall.ps1', 'upgrade.ps1', 'trust-browser.ps1')) {
    Copy-Item -LiteralPath (Join-Path $portableRoot $name) -Destination (Join-Path $packageDir $name) -Force
}
Get-ChildItem -LiteralPath (Join-Path $portableRoot 'package') -File | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $packageDir $_.Name) -Force
}

Write-Step 'Validating the assembled package'
$installerPath = Join-Path $payload ('installer\u-workers-setup-' + $Version + '-windows-amd64.exe')
if (-not (Test-Path -LiteralPath $installerPath)) { throw ('The package is missing the prebuilt installer: ' + $installerPath) }
if (-not $SkipBuildKit) {
    $requiredBuildInputs = @('payload\source\source.zip', 'tools\go\bin\go.exe', 'tools\gomodcache')
    foreach ($relative in $requiredBuildInputs) {
        if (-not (Test-Path -LiteralPath (Join-Path $packageDir $relative))) { throw ('The package is missing the offline build input: ' + $relative) }
    }
}

Write-Step 'Writing the package manifest'
function Get-PortableFileHash {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { return $null }
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
}
$manifest = [ordered]@{
    name = 'UWorkers-Portable'
    version = $Version
    commit = $commit
    builtAt = (Get-Date).ToUniversalTime().ToString('o')
    buildMachine = $env:COMPUTERNAME
    nodeVersion = $nodeVersion
    buildKit = (-not $SkipBuildKit -and -not $ReusePayloadFrom)
    entrypoint = 'Setup.cmd'
    components = [ordered]@{
        server = 'payload\release\server.exe'
        web = 'payload\release\web\apps\web\server.js'
        cli = 'payload\bin\u-workers.exe'
        node = 'payload\runtime\node.exe'
        postgres = 'payload\runtime\pgsql\bin\psql.exe'
        caddy = 'payload\runtime\caddy\caddy.exe'
        opencode = 'payload\runtime\opencode\opencode.exe'
        installer = 'payload\installer\u-workers-setup-' + $Version + '-windows-amd64.exe'
        source = 'payload\source\source.zip'
    }
    sha256 = [ordered]@{
        server = (Get-PortableFileHash (Join-Path $payload 'release\server.exe'))
        cli = (Get-PortableFileHash (Join-Path $payload 'bin\u-workers.exe'))
        node = (Get-PortableFileHash (Join-Path $payload 'runtime\node.exe'))
        caddy = (Get-PortableFileHash (Join-Path $payload 'runtime\caddy\caddy.exe'))
        opencode = (Get-PortableFileHash (Join-Path $payload 'runtime\opencode\opencode.exe'))
        installer = (Get-PortableFileHash (Join-Path $payload ('installer\u-workers-setup-' + $Version + '-windows-amd64.exe')))
    }
}
$manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $packageDir 'manifest.json') -Encoding UTF8

$totalBytes = (Get-ChildItem -LiteralPath $packageDir -Recurse -File | Measure-Object -Property Length -Sum).Sum
$record = [ordered]@{
    package = $packageDir
    version = $Version
    commit = $commit
    totalMegabytes = [math]::Round($totalBytes / 1MB, 1)
    buildKit = [bool]$manifest.buildKit
    zipped = $false
    zip = $null
}

if (-not $NoZip) {
    Write-Step 'Creating the zip archive'
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zipPath = Join-Path $OutputRoot ('UWorkers-Portable-' + $Version + '.zip')
    if (Test-Path -LiteralPath $zipPath) { Remove-Item -LiteralPath $zipPath -Force }
    [IO.Compression.ZipFile]::CreateFromDirectory($packageDir, $zipPath, [IO.Compression.CompressionLevel]::Optimal, $true)
    $record.zipped = $true
    $record.zip = $zipPath
    $record.zipMegabytes = [math]::Round((Get-Item -LiteralPath $zipPath).Length / 1MB, 1)
}

$recordPath = Join-Path $OutputRoot ('UWorkers-Portable-' + $Version + '.build.json')
$record | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $recordPath -Encoding UTF8

Write-Host ''
Write-Host ('Package ready: ' + $packageDir) -ForegroundColor Green
Write-Host ('Size:         ' + $record.totalMegabytes + ' MB')
if ($record.zipped) { Write-Host ('Zip:          ' + $record.zip + ' (' + $record.zipMegabytes + ' MB)') }
Write-Host ('Build record: ' + $recordPath)
