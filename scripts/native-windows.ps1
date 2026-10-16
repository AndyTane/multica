[CmdletBinding()]
param(
    [ValidateSet('Prepare', 'Build', 'Backup', 'Restore', 'Start', 'Stop', 'Status', 'Run', 'InstallStartup', 'RemoveStartup', 'Configure')]
    [string]$Action = 'Status',
    [string]$Root = (Join-Path $env:LOCALAPPDATA 'Multica\native'),
    [string]$BackupPath,
    [ValidateSet('Docker', 'Native')][string]$BackupSource = 'Native',
    [string]$Database,
    [int]$ApiPort,
    [int]$WebPort,
    [switch]$KeepDatabase
)

$ErrorActionPreference = 'Stop'
$script:RepoRoot = Split-Path $PSScriptRoot -Parent
$script:ScriptFile = $PSCommandPath

function Write-NativeJson {
    param([string]$Path, $Value)
    $text = $Value | ConvertTo-Json -Depth 12
    [IO.File]::WriteAllText($Path + '.tmp', $text, [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath ($Path + '.tmp') -Destination $Path -Force
}

function Read-NativeConfig {
    param([string]$Directory)
    Get-Content -LiteralPath (Join-Path $Directory 'deployment.json') -Raw -Encoding UTF8 | ConvertFrom-Json
}

function Get-NativeEnvironment {
    param($Entries)
    $result = @{}
    foreach ($entry in $Entries) {
        $parts = $entry -split '=', 2
        if ($parts[0] -notin @('PATH', 'HOSTNAME', 'HOME', 'LANG', 'NODE_VERSION', 'YARN_VERSION')) {
            $result[$parts[0]] = $parts[1]
        }
    }
    return $result
}

function ConvertTo-NativeArguments {
    param([string[]]$Values)
    $quoted = foreach ($value in $Values) {
        if ($value -match '[\r\n\x00]') { throw 'Invalid newline in process argument.' }
        '"' + ([regex]::Replace([regex]::Replace($value, '(\\*)"', '$1$1\"'), '(\\+)$', '$1$1')) + '"'
    }
    return $quoted -join ' '
}

function Invoke-NativeCommand {
    param([string]$Executable, [string[]]$Arguments)
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Command failed: $([IO.Path]::GetFileName($Executable)) (exit $LASTEXITCODE)." }
}

function Copy-NativeDirectory {
    param([string]$Source, [string]$Destination)
    & robocopy.exe $Source $Destination /E /SJ /SL /R:1 /W:1 /MT:4 /NP /NFL /NDL /NJH /NJS | Out-Null
    $copyExit = $LASTEXITCODE
    if ($copyExit -ge 8) { throw "Directory copy failed (robocopy exit $copyExit)." }
    $global:LASTEXITCODE = 0
}

function Get-NativeProcessRecord {
    param($Process, [string]$ExpectedExecutable)
    $Process.Refresh()
    $executablePath = if ($ExpectedExecutable) { [IO.Path]::GetFullPath($ExpectedExecutable) } else { $Process.Path }
    [pscustomobject]@{ Pid = $Process.Id; StartedAt = $Process.StartTime.ToUniversalTime().ToString('o'); Executable = $executablePath }
}

function Test-NativeProcessRecord {
    param($Record)
    if (-not $Record) { return $false }
    $process = Get-Process -Id $Record.Pid -ErrorAction SilentlyContinue
    if (-not $process) { return $false }
    try {
        return ($process.Path -eq $Record.Executable -and
            [math]::Abs(($process.StartTime.ToUniversalTime() - ([datetime]$Record.StartedAt).ToUniversalTime()).TotalMilliseconds) -lt 100)
    } catch { return $false }
}

function Stop-NativeOwnedProcess {
    param($Record)
    if (Test-NativeProcessRecord $Record) {
        $process = Get-Process -Id $Record.Pid -ErrorAction SilentlyContinue
        Stop-Process -Id $Record.Pid -ErrorAction SilentlyContinue
        if ($process) { $null = $process.WaitForExit(5000) }
    }
}

function Start-NativeChild {
    param([string]$Executable, [string[]]$Arguments, [string]$WorkingDirectory, [hashtable]$Environment, [string]$LogPath)
    $saved = @{}
    try {
        foreach ($key in $Environment.Keys) {
            $saved[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
            [Environment]::SetEnvironmentVariable($key, [string]$Environment[$key], 'Process')
        }
        $options = @{
            FilePath = $Executable; WorkingDirectory = $WorkingDirectory; WindowStyle = 'Hidden'; PassThru = $true
            RedirectStandardOutput = $LogPath; RedirectStandardError = $LogPath + '.err'
        }
        if ($Arguments.Count -gt 0) { $options.ArgumentList = ConvertTo-NativeArguments $Arguments }
        $process = Start-Process @options
        return Get-NativeProcessRecord $process $Executable
    } finally {
        foreach ($key in $saved.Keys) {
            $original = if ($null -eq $saved[$key]) { [NullString]::Value } else { $saved[$key] }
            [Environment]::SetEnvironmentVariable($key, $original, 'Process')
        }
    }
}

function Get-PostgresBin {
    param([string]$Directory)
    Join-Path $Directory 'runtime\pgsql\bin'
}

function Get-DatabaseUrl {
    param($Config, [string]$Name)
    if (-not $Name) { $Name = $Config.Database }
    'postgresql://{0}:{1}@127.0.0.1:{2}/{3}?sslmode=disable' -f
        [uri]::EscapeDataString($Config.DatabaseUser), [uri]::EscapeDataString($Config.DatabasePassword), $Config.DatabasePort, [uri]::EscapeDataString($Name)
}

function Invoke-NativeSql {
    param($Config, [string]$Sql, [string]$Name = 'postgres')
    $savedPassword = $env:PGPASSWORD
    try {
        $env:PGPASSWORD = $Config.DatabasePassword
        Invoke-NativeCommand (Join-Path (Get-PostgresBin $Config.Root) 'psql.exe') @('-h', '127.0.0.1', '-p', [string]$Config.DatabasePort, '-U', $Config.DatabaseUser, '-d', $Name, '-X', '-A', '-t', '-v', 'ON_ERROR_STOP=1', '-c', $Sql)
    } finally { $env:PGPASSWORD = $savedPassword }
}

function Start-NativePostgres {
    param($Config)
    $pgBin = Get-PostgresBin $Config.Root
    $pgData = Join-Path $Config.Root 'data\postgres'
    if (-not (Test-Path -LiteralPath (Join-Path $pgData 'PG_VERSION'))) {
        $passwordFile = Join-Path $Config.Root 'state\initdb-password'
        try {
            [IO.File]::WriteAllText($passwordFile, $Config.DatabasePassword, [Text.UTF8Encoding]::new($false))
            Invoke-NativeCommand (Join-Path $pgBin 'initdb.exe') @('-D', $pgData, '-U', $Config.DatabaseUser, '--pwfile', $passwordFile, '--auth-host=scram-sha-256', '--auth-local=scram-sha-256', '--encoding=UTF8', '--locale=C', '--locale-provider=icu', '--icu-locale=en-US', '--no-instructions')
            $settings = "`nlisten_addresses = '127.0.0.1'`nport = $($Config.DatabasePort)`nshared_buffers = '64MB'`nmax_connections = 40`nwork_mem = '4MB'`njit = off`n"
            [IO.File]::AppendAllText((Join-Path $pgData 'postgresql.conf'), $settings, [Text.UTF8Encoding]::new($false))
        } finally { Remove-Item -LiteralPath $passwordFile -Force -ErrorAction SilentlyContinue }
    }
    & (Join-Path $pgBin 'pg_ctl.exe') status -D $pgData *> $null
    if ($LASTEXITCODE -ne 0) {
        Invoke-NativeCommand (Join-Path $pgBin 'pg_ctl.exe') @('start', '-D', $pgData, '-l', (Join-Path $Config.Root 'logs\postgres.log'), '-w', '-t', '60')
    }
    $identity = @(Invoke-NativeSql $Config 'SELECT current_setting(''data_directory'');')
    if ([IO.Path]::GetFullPath([string]$identity[-1]) -ne [IO.Path]::GetFullPath($pgData)) { throw 'PostgreSQL identity does not match this deployment.' }
}

function Initialize-NativeDeployment {
    param([string]$Directory)
    $Directory = [IO.Path]::GetFullPath($Directory)
    New-Item -ItemType Directory -Path $Directory -Force | Out-Null
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    Invoke-NativeCommand 'icacls.exe' @($Directory, '/inheritance:r', '/grant:r', ('*' + $sid + ':(OI)(CI)F'), '*S-1-5-18:(OI)(CI)F') | Out-Null
    foreach ($child in @('downloads', 'runtime', 'releases', 'backups', 'logs', 'state', 'data', 'uploads')) {
        New-Item -ItemType Directory -Path (Join-Path $Directory $child) -Force | Out-Null
    }
    $configFile = Join-Path $Directory 'deployment.json'
    if (Test-Path -LiteralPath $configFile) { return }
    $backend = @(docker inspect multica-backend-1 | ConvertFrom-Json)[0]
    if ($LASTEXITCODE -ne 0) { throw 'The existing Multica backend container was not found.' }
    $frontend = @(docker inspect multica-frontend-1 | ConvertFrom-Json)[0]
    $postgres = @(docker inspect multica-postgres-1 | ConvertFrom-Json)[0]
    $pgEnv = Get-NativeEnvironment $postgres.Config.Env
    $config = [ordered]@{
        Root = $Directory; Repo = $script:RepoRoot; DatabaseUser = $pgEnv.POSTGRES_USER; DatabasePassword = $pgEnv.POSTGRES_PASSWORD
        SourceDatabase = $pgEnv.POSTGRES_DB; Database = 'multica_native_rehearsal'; DatabasePort = 15432; ApiPort = 18080; WebPort = 13000
        Node = (Get-Command node.exe).Source; Release = ''; BackendEnvironment = (Get-NativeEnvironment $backend.Config.Env)
        FrontendEnvironment = (Get-NativeEnvironment $frontend.Config.Env); CreatedAt = (Get-Date).ToUniversalTime().ToString('o')
    }
    Write-NativeJson $configFile $config
    Copy-Item -LiteralPath $script:ScriptFile -Destination (Join-Path $Directory 'runtime\native-windows.ps1') -Force
    Write-Host 'Native configuration imported into the protected deployment directory.'
}

function Build-NativeDeployment {
    param($Config)
    $version = (git -C $Config.Repo describe --tags --match 'v[0-9]*' --always --dirty).Trim()
    if ($version -notmatch '^v\d+\.\d+\.\d+') { throw 'Tagged source version required.' }
    $commit = (git -C $Config.Repo rev-parse --short HEAD).Trim()
    $release = Join-Path $Config.Root ('releases\' + $version + '-' + (Get-Date -Format 'yyyyMMddHHmmss'))
    New-Item -ItemType Directory -Path $release -Force | Out-Null
    Push-Location -LiteralPath (Join-Path $Config.Repo 'server')
    $savedGo = $env:GOMAXPROCS; $savedCgo = $env:CGO_ENABLED
    try {
        $env:GOMAXPROCS = '2'; $env:CGO_ENABLED = '0'
        Invoke-NativeCommand 'go.exe' @('build', '-p', '2', '-ldflags', "-s -w -X main.version=$version -X main.commit=$commit", '-o', (Join-Path $release 'server.exe'), './cmd/server')
        Invoke-NativeCommand 'go.exe' @('build', '-p', '2', '-ldflags', '-s -w', '-o', (Join-Path $release 'migrate.exe'), './cmd/migrate')
    } finally { $env:GOMAXPROCS = $savedGo; $env:CGO_ENABLED = $savedCgo; Pop-Location }
    Copy-Item -LiteralPath (Join-Path $Config.Repo 'server\migrations') -Destination (Join-Path $release 'migrations') -Recurse
    foreach ($notice in @('LICENSE', 'NOTICE')) { Copy-Item -LiteralPath (Join-Path $Config.Repo $notice) -Destination (Join-Path $release $notice) }
    Push-Location -LiteralPath $Config.Repo
    $savedStandalone = $env:STANDALONE; $savedNodeOptions = $env:NODE_OPTIONS; $savedVersion = $env:NEXT_PUBLIC_APP_VERSION; $savedBuildCpus = $env:MULTICA_BUILD_CPUS
    try {
        $env:STANDALONE = 'true'; $env:NODE_OPTIONS = '--max-old-space-size=2048'; $env:NEXT_PUBLIC_APP_VERSION = $version
        $env:MULTICA_BUILD_CPUS = '2'
        & pnpm.cmd --filter '@multica/web' build
        if ($LASTEXITCODE -ne 0) { throw 'Native frontend build failed.' }
    } finally { $env:STANDALONE = $savedStandalone; $env:NODE_OPTIONS = $savedNodeOptions; $env:NEXT_PUBLIC_APP_VERSION = $savedVersion; $env:MULTICA_BUILD_CPUS = $savedBuildCpus; Pop-Location }
    $webOutput = Join-Path $Config.Repo 'apps\web\.next'
    Copy-NativeDirectory (Join-Path $webOutput 'standalone') (Join-Path $release 'web')
    $appDirectory = Join-Path $release 'web\apps\web'
    New-Item -ItemType Directory -Path (Join-Path $appDirectory '.next') -Force | Out-Null
    Copy-NativeDirectory (Join-Path $webOutput 'static') (Join-Path $appDirectory '.next\static')
    Copy-NativeDirectory (Join-Path $Config.Repo 'apps\web\public') (Join-Path $appDirectory 'public')
    $Config.Release = $release
    Write-NativeJson (Join-Path $Config.Root 'deployment.json') $Config
    Copy-Item -LiteralPath $script:ScriptFile -Destination (Join-Path $Config.Root 'runtime\native-windows.ps1') -Force
    Write-Host "Native release built: $version ($commit)."
}

function Backup-DockerDeployment {
    param($Config)
    $name = Get-Date -Format 'yyyyMMdd-HHmmss'
    $destination = Join-Path $Config.Root ('backups\' + $name)
    New-Item -ItemType Directory -Path $destination | Out-Null
    $remoteDump = '/tmp/multica-native-' + $name + '.dump'
    Invoke-NativeCommand 'docker.exe' @('exec', 'multica-postgres-1', 'pg_dump', '-U', $Config.DatabaseUser, '-d', $Config.SourceDatabase, '-Fc', '--no-owner', '--no-acl', '-f', $remoteDump)
    Invoke-NativeCommand 'docker.exe' @('cp', ('multica-postgres-1:' + $remoteDump), (Join-Path $destination 'database.dump'))
    $uploadSnapshot = Join-Path $destination 'uploads'
    New-Item -ItemType Directory -Path $uploadSnapshot | Out-Null
    Invoke-NativeCommand 'docker.exe' @('cp', 'multica-backend-1:/app/data/uploads/.', $uploadSnapshot)
    Invoke-NativeCommand 'tar.exe' @('-czf', (Join-Path $destination 'uploads.tar.gz'), '-C', $uploadSnapshot, '.')
    Copy-Item -LiteralPath (Join-Path $Config.Root 'deployment.json') -Destination (Join-Path $destination 'deployment.json')
    Invoke-NativeCommand 'docker.exe' @('exec', 'multica-postgres-1', 'rm', '-f', $remoteDump)
    Get-ChildItem -LiteralPath $destination -File | Get-FileHash -Algorithm SHA256 | Select-Object @{n='File';e={[IO.Path]::GetFileName($_.Path)}}, Hash | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $destination 'sha256.json') -Encoding UTF8
    Write-Host "Backup complete: $destination"
}

function Backup-NativeDeployment {
    param($Config)
    $destination = Join-Path $Config.Root ('backups\native-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
    New-Item -ItemType Directory -Path $destination | Out-Null
    $savedPassword = $env:PGPASSWORD
    try {
        $env:PGPASSWORD = $Config.DatabasePassword
        Invoke-NativeCommand (Join-Path (Get-PostgresBin $Config.Root) 'pg_dump.exe') @('-h', '127.0.0.1', '-p', [string]$Config.DatabasePort, '-U', $Config.DatabaseUser, '-d', $Config.Database, '-Fc', '--no-owner', '--no-acl', '--no-password', '-f', (Join-Path $destination 'database.dump'))
    } finally { $env:PGPASSWORD = $savedPassword }
    Invoke-NativeCommand 'tar.exe' @('-czf', (Join-Path $destination 'uploads.tar.gz'), '-C', (Join-Path $Config.Root 'uploads'), '.')
    Copy-Item -LiteralPath (Join-Path $Config.Root 'deployment.json') -Destination (Join-Path $destination 'deployment.json')
    Get-ChildItem -LiteralPath $destination -File | Get-FileHash -Algorithm SHA256 | Select-Object @{n='File';e={[IO.Path]::GetFileName($_.Path)}}, Hash | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $destination 'sha256.json') -Encoding UTF8
    Write-Host "Native backup complete: $destination"
}

function Restore-NativeDeployment {
    param($Config, [string]$Source, [string]$Name)
    if (-not $Name -or $Name -notmatch '^[a-z][a-z0-9_]{0,62}$') { throw 'A simple explicit database name is required for restore.' }
    $statePath = Join-Path $Config.Root 'state\processes.json'
    if (Test-Path -LiteralPath $statePath) {
        $state = Get-Content -LiteralPath $statePath -Raw -Encoding UTF8 | ConvertFrom-Json
        if (Test-NativeProcessRecord $state.Supervisor) { throw 'Stop the native deployment before restoring database files and uploads.' }
    }
    if (-not (Test-Path -LiteralPath (Join-Path $Source 'database.dump'))) { throw 'Database backup not found.' }
    $checksums = Get-Content -LiteralPath (Join-Path $Source 'sha256.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    foreach ($entry in $checksums) {
        if ($entry.File -notin @('database.dump', 'uploads.tar.gz', 'deployment.json')) { throw 'Unexpected file in backup checksum manifest.' }
        if ((Get-FileHash -LiteralPath (Join-Path $Source $entry.File) -Algorithm SHA256).Hash -ne $entry.Hash) { throw "Backup checksum mismatch: $($entry.File)." }
    }
    Start-NativePostgres $Config
    $existing = @(Invoke-NativeSql $Config ("SELECT datname FROM pg_database WHERE datname='$Name';"))
    if ($existing.Count -gt 0 -and [string]$existing[-1]) { throw 'Restore refuses to overwrite an existing database.' }
    Invoke-NativeSql $Config ('CREATE DATABASE "' + $Name + '" TEMPLATE template0 LOCALE_PROVIDER icu ICU_LOCALE ''en-US'' LOCALE ''C'' ENCODING ''UTF8'';') | Out-Null
    $savedPassword = $env:PGPASSWORD
    try {
        $env:PGPASSWORD = $Config.DatabasePassword
        Invoke-NativeCommand (Join-Path (Get-PostgresBin $Config.Root) 'pg_restore.exe') @('-h', '127.0.0.1', '-p', [string]$Config.DatabasePort, '-U', $Config.DatabaseUser, '-d', $Name, '--no-owner', '--no-acl', '--exit-on-error', (Join-Path $Source 'database.dump'))
    } finally { $env:PGPASSWORD = $savedPassword }
    Invoke-NativeSql $Config 'ANALYZE;' $Name | Out-Null
    Invoke-NativeCommand 'tar.exe' @('-xzf', (Join-Path $Source 'uploads.tar.gz'), '-C', (Join-Path $Config.Root 'uploads'))
    Write-Host "Database restored and analyzed: $Name."
}

function Get-BackendEnvironment {
    param($Config)
    $environment = @{}
    foreach ($property in $Config.BackendEnvironment.PSObject.Properties) { $environment[$property.Name] = [string]$property.Value }
    $environment.HOST = '127.0.0.1'; $environment.PORT = [string]$Config.ApiPort
    $environment.DATABASE_URL = Get-DatabaseUrl $Config
    $environment.DATABASE_MAX_CONNS = '10'; $environment.DATABASE_MIN_CONNS = '2'
    $environment.LOCAL_UPLOAD_DIR = Join-Path $Config.Root 'uploads'
    $environment.FRONTEND_ORIGIN = 'http://localhost:' + $Config.WebPort
    $environment.MULTICA_APP_URL = $environment.FRONTEND_ORIGIN
    $environment.MULTICA_DAEMON_SERVER_URL = 'http://localhost:' + $Config.ApiPort
    return $environment
}

function Get-FrontendEnvironment {
    param($Config)
    $environment = @{ NODE_ENV = 'production'; HOSTNAME = '127.0.0.1'; PORT = [string]$Config.WebPort; REMOTE_API_URL = ('http://127.0.0.1:' + $Config.ApiPort); NEXT_TELEMETRY_DISABLED = '1' }
    foreach ($property in $Config.FrontendEnvironment.PSObject.Properties) {
        if ($property.Name -notin @('HOSTNAME', 'PORT', 'REMOTE_API_URL', 'NODE_ENV')) { $environment[$property.Name] = [string]$property.Value }
    }
    if (-not $environment.NEXT_PUBLIC_WS_URL) { $environment.NEXT_PUBLIC_WS_URL = 'ws://localhost:' + $Config.ApiPort + '/ws' }
    return $environment
}

function Invoke-NativeSupervisor {
    param($Config)
    $lockFile = Join-Path $Config.Root 'state\supervisor.lock'
    try { $lock = [IO.File]::Open($lockFile, 'OpenOrCreate', 'ReadWrite', 'None') } catch { return }
    $stateFile = Join-Path $Config.Root 'state\processes.json'
    $stopFile = Join-Path $Config.Root 'state\stop'
    $children = @{}
    try {
        if (Test-Path -LiteralPath $stopFile) { Remove-Item -LiteralPath $stopFile -Force }
        Start-NativePostgres $Config
        $backendEnvironment = Get-BackendEnvironment $Config
        $savedDb = $env:DATABASE_URL
        try {
            $env:DATABASE_URL = $backendEnvironment.DATABASE_URL
            Push-Location -LiteralPath $Config.Release
            try { Invoke-NativeCommand (Join-Path $Config.Release 'migrate.exe') @('up') } finally { Pop-Location }
        } finally { $env:DATABASE_URL = $savedDb }
        while (-not (Test-Path -LiteralPath $stopFile)) {
            $postmaster = Join-Path $Config.Root 'data\postgres\postmaster.pid'
            $postgresProcess = if (Test-Path -LiteralPath $postmaster) { Get-Process -Id ([int](Get-Content -LiteralPath $postmaster -TotalCount 1)) -ErrorAction SilentlyContinue } else { $null }
            if (-not $postgresProcess) { Start-NativePostgres $Config }
            if (-not (Test-NativeProcessRecord $children.Backend)) {
                $stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
                $children.Backend = Start-NativeChild (Join-Path $Config.Release 'server.exe') @() $Config.Release $backendEnvironment (Join-Path $Config.Root "logs\backend-$stamp.log")
            }
            if (-not (Test-NativeProcessRecord $children.Frontend)) {
                $stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
                $frontendEnvironment = Get-FrontendEnvironment $Config
                $children.Frontend = Start-NativeChild $Config.Node @((Join-Path $Config.Release 'web\apps\web\server.js')) (Join-Path $Config.Release 'web') $frontendEnvironment (Join-Path $Config.Root "logs\frontend-$stamp.log")
            }
            Write-NativeJson $stateFile ([ordered]@{ Supervisor = (Get-NativeProcessRecord (Get-Process -Id $PID)); Backend = $children.Backend; Frontend = $children.Frontend; UpdatedAt = (Get-Date).ToUniversalTime().ToString('o') })
            Start-Sleep -Seconds 5
        }
    } finally {
        foreach ($child in $children.Values) { Stop-NativeOwnedProcess $child }
        if (Test-Path -LiteralPath $stateFile) { Remove-Item -LiteralPath $stateFile -Force }
        $lock.Dispose()
    }
}

function Stop-NativeDeployment {
    param($Config, [bool]$KeepDatabaseRunning = $false)
    $stopPath = Join-Path $Config.Root 'state\stop'
    [IO.File]::WriteAllText($stopPath, 'stop')
    $stateFile = Join-Path $Config.Root 'state\processes.json'
    if (Test-Path -LiteralPath $stateFile) {
        $state = Get-Content -LiteralPath $stateFile -Raw -Encoding UTF8 | ConvertFrom-Json
        if (-not (Test-NativeProcessRecord $state.Supervisor)) {
            Stop-NativeOwnedProcess $state.Backend
            Stop-NativeOwnedProcess $state.Frontend
            Remove-Item -LiteralPath $stateFile -Force
        }
    }
    $deadline = (Get-Date).AddSeconds(15)
    while ((Test-Path -LiteralPath $stateFile) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 300 }
    if (Test-Path -LiteralPath $stateFile) { throw 'Supervisor did not stop; refusing to signal unverified processes.' }
    if ($KeepDatabaseRunning) { return }
    $pgData = Join-Path $Config.Root 'data\postgres'
    & (Join-Path (Get-PostgresBin $Config.Root) 'pg_ctl.exe') status -D $pgData *> $null
    if ($LASTEXITCODE -eq 0) { Invoke-NativeCommand (Join-Path (Get-PostgresBin $Config.Root) 'pg_ctl.exe') @('stop', '-D', $pgData, '-m', 'fast', '-w', '-t', '60') }
}

function Start-NativeDeployment {
    param($Config)
    $stateFile = Join-Path $Config.Root 'state\processes.json'
    if (Test-Path -LiteralPath $stateFile) {
        $state = Get-Content -LiteralPath $stateFile -Raw -Encoding UTF8 | ConvertFrom-Json
        if (Test-NativeProcessRecord $state.Supervisor) { Write-Host 'Native supervisor is already running.'; return }
    }
    if (-not (Test-Path -LiteralPath (Join-Path $Config.Release 'server.exe'))) { throw 'Build a native release first.' }
    foreach ($port in @($Config.ApiPort, $Config.WebPort)) {
        $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $port)
        try { $listener.Start() } catch { throw "Port $port is already occupied; refusing to start over another deployment." } finally { $listener.Stop() }
    }
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $runner = Join-Path $Config.Root 'runtime\native-windows.ps1'
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $null = Start-NativeChild $shell @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $runner, '-Action', 'Run', '-Root', $Config.Root) $Config.Root @{} (Join-Path $Config.Root "logs\supervisor-$stamp.log")
    $deadline = (Get-Date).AddSeconds(90)
    do {
        try {
            $ready = Invoke-RestMethod -Uri ('http://127.0.0.1:' + $Config.ApiPort + '/readyz') -TimeoutSec 3
            $health = Invoke-RestMethod -Uri ('http://127.0.0.1:' + $Config.ApiPort + '/health') -TimeoutSec 3
            $web = Invoke-WebRequest -Uri ('http://127.0.0.1:' + $Config.WebPort + '/') -UseBasicParsing -TimeoutSec 3
            $state = Get-Content -LiteralPath $stateFile -Raw -Encoding UTF8 | ConvertFrom-Json
            if ($ready.status -eq 'ok' -and $web.StatusCode -eq 200 -and $health.pid -eq $state.Backend.Pid -and (Test-NativeProcessRecord $state.Backend) -and (Test-NativeProcessRecord $state.Frontend)) { Write-Host "Native deployment ready: http://localhost:$($Config.WebPort)"; return }
        } catch { }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw 'Native readiness failed. Inspect the protected logs directory.'
}

function Set-NativeStartup {
    param($Config, [bool]$Enabled)
    $startupFile = Join-Path ([Environment]::GetFolderPath('Startup')) 'Multica Native.vbs'
    if (-not $Enabled) { Remove-Item -LiteralPath $startupFile -Force -ErrorAction SilentlyContinue; return }
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $command = ConvertTo-NativeArguments @($shell, '-NoProfile', '-ExecutionPolicy', 'Bypass', '-WindowStyle', 'Hidden', '-File', (Join-Path $Config.Root 'runtime\native-windows.ps1'), '-Action', 'Run', '-Root', $Config.Root)
    $vbs = 'CreateObject("WScript.Shell").Run "' + $command.Replace('"', '""') + '", 0, False' + "`r`n"
    [IO.File]::WriteAllText($startupFile, $vbs, [Text.UnicodeEncoding]::new($false, $true))
    Write-Host 'Native deployment starts when this Windows user signs in.'
}

if ($MyInvocation.InvocationName -ne '.') {
    if ($Action -eq 'Prepare') { Initialize-NativeDeployment $Root; return }
    $config = Read-NativeConfig $Root
    switch ($Action) {
        'Build' { Build-NativeDeployment $config }
        'Backup' { if ($BackupSource -eq 'Docker') { Backup-DockerDeployment $config } else { Backup-NativeDeployment $config } }
        'Restore' { Restore-NativeDeployment $config $BackupPath $Database }
        'Start' { Start-NativeDeployment $config }
        'Stop' { Stop-NativeDeployment $config $KeepDatabase.IsPresent }
        'Run' { Invoke-NativeSupervisor $config }
        'InstallStartup' { Set-NativeStartup $config $true }
        'RemoveStartup' { Set-NativeStartup $config $false }
        'Configure' {
            if (Test-Path -LiteralPath (Join-Path $Root 'state\processes.json')) { throw 'Stop the native deployment before changing configuration.' }
            if ($ApiPort) { $config.ApiPort = $ApiPort }
            if ($WebPort) { $config.WebPort = $WebPort }
            if ($Database) { $config.Database = $Database }
            Write-NativeJson (Join-Path $Root 'deployment.json') $config
        }
        'Status' {
            $stateFile = Join-Path $Root 'state\processes.json'
            $state = if (Test-Path -LiteralPath $stateFile) { Get-Content -LiteralPath $stateFile -Raw -Encoding UTF8 | ConvertFrom-Json } else { $null }
            [pscustomobject]@{ Running = ($state -and (Test-NativeProcessRecord $state.Supervisor)); ApiPort = $config.ApiPort; WebPort = $config.WebPort; DatabasePort = $config.DatabasePort; Database = $config.Database; Release = $config.Release } | ConvertTo-Json
        }
    }
}
