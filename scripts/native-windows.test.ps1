$ErrorActionPreference = 'Stop'
$sourcePath = Join-Path $PSScriptRoot 'native-windows.ps1'
$parseErrors = $null
[Management.Automation.Language.Parser]::ParseFile($sourcePath, [ref]$null, [ref]$parseErrors) | Out-Null
if ($parseErrors) { throw ($parseErrors.Message -join '; ') }

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('multica-native-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
. $sourcePath -Root $testRoot

function Assert-NativeTest {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

try {
    Assert-NativeTest (-not (Test-Path -LiteralPath (Join-Path $testRoot 'deployment.json'))) 'Importing the script must not initialize a deployment.'
    $environment = Get-NativeEnvironment @('PATH=/linux/bin', 'HOSTNAME=container', 'JWT_SECRET=test=secret', 'EMPTY=')
    Assert-NativeTest (-not $environment.ContainsKey('PATH')) 'Container PATH must not leak into Windows child processes.'
    Assert-NativeTest ($environment.JWT_SECRET -eq 'test=secret') 'Environment values must retain equals signs.'

    $config = [pscustomobject]@{
        Root = $testRoot; Database = 'example'; DatabaseUser = 'example'; DatabasePassword = 'a@b:c/#?'
        DatabasePort = 15432; ApiPort = 18080; WebPort = 13000
        BackendEnvironment = [pscustomobject]@{ JWT_SECRET = 'test-secret'; PORT = '8080'; DATABASE_URL = 'old'; FRONTEND_ORIGIN = 'old'; DO_NOT_TRACK = '0'; ANALYTICS_DISABLED = 'false'; POSTHOG_API_KEY = 'test-key'; POSTHOG_HOST = 'https://collector.invalid' }
        FrontendEnvironment = [pscustomobject]@{ REMOTE_API_URL = 'http://backend:8080'; NEXT_PUBLIC_WS_URL = ''; NEXT_TELEMETRY_DISABLED = '0' }
    }
    $backendEnv = Get-BackendEnvironment $config
    Assert-NativeTest ($backendEnv.JWT_SECRET -eq 'test-secret') 'Existing application secrets must survive migration.'
    Assert-NativeTest ($backendEnv.HOST -eq '127.0.0.1') 'The native API must bind to loopback.'
    Assert-NativeTest ($backendEnv.FRONTEND_ORIGIN -eq 'http://localhost:13000') 'The rehearsal frontend origin must use its isolated port.'
    Assert-NativeTest ($backendEnv.MULTICA_DAEMON_SERVER_URL -eq 'http://localhost:18080') 'Daemon setup must point to the API listener with native WebSocket support.'
    Assert-NativeTest ($backendEnv.DATABASE_URL -like '*a%40b%3Ac%2F%23%3F@127.0.0.1:15432/example*') 'Database credentials must be URI escaped.'
    Assert-NativeTest ($backendEnv.DO_NOT_TRACK -eq '1') 'Imported settings must not enable daily self-host snapshots.'
    Assert-NativeTest ($backendEnv.ANALYTICS_DISABLED -eq 'true') 'Imported settings must not enable analytics.'
    Assert-NativeTest (-not $backendEnv.POSTHOG_API_KEY -and -not $backendEnv.POSTHOG_HOST) 'Native startup must not expose an analytics project to clients.'
    $frontendEnv = Get-FrontendEnvironment $config
    Assert-NativeTest ($frontendEnv.REMOTE_API_URL -eq 'http://127.0.0.1:18080') 'Frontend API proxy must target the native backend.'
    Assert-NativeTest ($frontendEnv.NEXT_PUBLIC_WS_URL -eq '') 'Browser realtime must derive its protocol and host from the page for LAN and HTTPS access.'
    $config.FrontendEnvironment.NEXT_PUBLIC_WS_URL = 'wss://example.invalid/ws'
    $explicitFrontendEnv = Get-FrontendEnvironment $config
    Assert-NativeTest ($explicitFrontendEnv.NEXT_PUBLIC_WS_URL -eq 'wss://example.invalid/ws') 'An explicitly configured WebSocket endpoint must be preserved.'
    $config.FrontendEnvironment.NEXT_PUBLIC_WS_URL = ''
    Assert-NativeTest ($frontendEnv.NEXT_TELEMETRY_DISABLED -eq '1') 'Imported frontend settings must not re-enable Next.js telemetry.'

    $savedEnv = [Environment]::GetEnvironmentVariable('MULTICA_NATIVE_TEST_MARKER', 'Process')
    $fakeScript = Join-Path $testRoot 'fake child.ps1'
    [IO.File]::WriteAllText($fakeScript, '[Console]::WriteLine($env:MULTICA_NATIVE_TEST_MARKER); Start-Sleep -Seconds 20')
    $shell = (Get-Process -Id $PID).Path
    $record = Start-NativeChild $shell @('-NoProfile', '-File', $fakeScript) $testRoot @{ MULTICA_NATIVE_TEST_MARKER = 'child-only' } (Join-Path $testRoot 'child.log')
    try {
        Assert-NativeTest (Test-NativeProcessRecord $record) 'The started child must be recognized by executable, PID, and start time.'
        $reloadedRecord = $record | ConvertTo-Json | ConvertFrom-Json
        Assert-NativeTest (Test-NativeProcessRecord $reloadedRecord) 'JSON date conversion must retain process start-time precision across PowerShell versions.'
        Assert-NativeTest ([Environment]::GetEnvironmentVariable('MULTICA_NATIVE_TEST_MARKER', 'Process') -eq $savedEnv) 'Launching a child must restore the parent environment.'
        $foreign = [pscustomobject]@{ Pid = $record.Pid; Executable = $record.Executable; StartedAt = '2000-01-01T00:00:00Z' }
        Assert-NativeTest (-not (Test-NativeProcessRecord $foreign)) 'A reused PID with a different start time must be rejected.'
        Stop-NativeOwnedProcess $foreign
        Assert-NativeTest (Test-NativeProcessRecord $record) 'Stopping an unverified record must leave the live process untouched.'
        $deadline = (Get-Date).AddSeconds(10)
        do { Start-Sleep -Milliseconds 200; $output = Get-Content -LiteralPath (Join-Path $testRoot 'child.log') -Raw -ErrorAction SilentlyContinue } while (-not $output -and (Get-Date) -lt $deadline)
        Assert-NativeTest ($output.Trim() -eq 'child-only') 'A path containing spaces and isolated child environment must work.'
    } finally { Stop-NativeOwnedProcess $record }

    $jsonFile = Join-Path $testRoot 'state.json'
    Write-NativeJson $jsonFile @{ value = 'first' }
    Write-NativeJson $jsonFile @{ value = 'second' }
    Assert-NativeTest ((Get-Content -LiteralPath $jsonFile -Raw | ConvertFrom-Json).value -eq 'second') 'Atomic state writes must replace the previous value.'
    $unicodeValue = [string][char]0x4E2D + [char]0x6587
    Write-NativeJson (Join-Path $testRoot 'deployment.json') @{ value = $unicodeValue }
    Assert-NativeTest ((Read-NativeConfig $testRoot).value -eq $unicodeValue) 'Configuration must retain UTF-8 text in Windows PowerShell 5.1.'
    $rejected = $false
    try { Restore-NativeDeployment $config $testRoot 'bad-name; DROP DATABASE example' } catch { $rejected = $true }
    Assert-NativeTest $rejected 'Restore must reject unsafe database names before connecting to PostgreSQL.'
    New-Item -ItemType Directory -Path (Join-Path $testRoot 'state') -Force | Out-Null
    $liveState = Join-Path $testRoot 'state\processes.json'
    Write-NativeJson $liveState @{ Supervisor = (Get-NativeProcessRecord (Get-Process -Id $PID)) }
    $rejected = $false
    try { Restore-NativeDeployment $config $testRoot 'valid_restore_name' } catch { $rejected = $_.Exception.Message -like 'Stop the native deployment*' }
    Assert-NativeTest $rejected 'Restore must not overwrite uploads used by a running native deployment.'
    Remove-Item -LiteralPath $liveState -Force
    $traceSource = Join-Path $testRoot 'trace'
    $dependency = Join-Path $testRoot 'dependency'
    New-Item -ItemType Directory -Path $traceSource, $dependency | Out-Null
    [IO.File]::WriteAllText((Join-Path $dependency 'index.js'), 'module.exports = true;')
    New-Item -ItemType Junction -Path (Join-Path $traceSource 'dependency') -Target $dependency | Out-Null
    Copy-NativeDirectory $traceSource (Join-Path $testRoot 'trace-copy')
    $copiedLink = Get-Item -LiteralPath (Join-Path $testRoot 'trace-copy\dependency') -Force
    Assert-NativeTest ($copiedLink.LinkType -eq 'Junction') 'Production copying must retain dependency junctions instead of expanding them.'
    Assert-NativeTest (Test-Path -LiteralPath (Join-Path $testRoot 'trace-copy\dependency\index.js')) 'Copied dependency junctions must remain usable.'
    $releaseDirectory = Join-Path $testRoot 'release'
    New-Item -ItemType Directory -Path $releaseDirectory | Out-Null
    [IO.File]::WriteAllText((Join-Path $releaseDirectory 'server.exe'), '')
    $config | Add-Member -NotePropertyName Release -NotePropertyValue $releaseDirectory
    New-Item -ItemType Directory -Path (Join-Path $testRoot 'state') -Force | Out-Null
    $occupiedPort = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    try {
        $occupiedPort.Start()
        $config.ApiPort = $occupiedPort.LocalEndpoint.Port
        $rejected = $false
        try { Start-NativeDeployment $config } catch { $rejected = $_.Exception.Message -like '*already occupied*' }
        Assert-NativeTest $rejected 'Start must refuse a port owned by another application before launching children.'
    } finally { $occupiedPort.Stop() }
    # Clean deployment preparation, scheduled-task persistence, and tagless build stamping.
    $cleanArchive = Join-Path $testRoot 'postgres-archive.zip'
    $archiveSource = Join-Path $testRoot 'archive-source'
    New-Item -ItemType Directory -Path (Join-Path $archiveSource 'pgsql\bin') -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $archiveSource 'pgsql\bin\psql.exe'), '')
    Compress-Archive -Path (Join-Path $archiveSource 'pgsql') -DestinationPath $cleanArchive
    $archiveHash = (Get-FileHash -LiteralPath $cleanArchive -Algorithm SHA256).Hash
    $cleanRoot = Join-Path $testRoot 'clean-deploy'
    Initialize-NativeCleanDeployment -Directory $cleanRoot -Database 'multica_test' -DatabasePort 16432 -ApiPort 18081 -WebPort 13001 -PublicOrigin 'http://192.168.3.49:13001' -CorsOrigins 'http://100.73.70.87:13001' -PgArchive $cleanArchive -PgArchiveSha256 $archiveHash -Version 'v0.2.0' -NodeExe $shell
    $cleanConfig = Read-NativeConfig $cleanRoot
    Assert-NativeTest ($cleanConfig.PublicOrigin -eq 'http://192.168.3.49:13001') 'A clean deployment must record its public browser origin.'
    Assert-NativeTest ($cleanConfig.BackendEnvironment.APP_ENV -eq 'production') 'A clean deployment must run the production application environment.'
    Assert-NativeTest ($cleanConfig.BackendEnvironment.JWT_SECRET.Length -eq 64) 'A clean deployment must generate its own JWT secret.'
    Assert-NativeTest ($cleanConfig.DatabasePassword.Length -eq 48) 'A clean deployment must generate its own database password.'
    Assert-NativeTest ($cleanConfig.DatabasePort -eq 16432 -and $cleanConfig.ApiPort -eq 18081 -and $cleanConfig.WebPort -eq 13001) 'Clean deployment ports must be honored.'
    Assert-NativeTest ($cleanConfig.Version -eq 'v0.2.0') 'A clean deployment must record its explicit release version.'
    Assert-NativeTest ($cleanConfig.BackendEnvironment.CORS_ALLOWED_ORIGINS -eq 'http://192.168.3.49:13001,http://100.73.70.87:13001') 'CORS must list exactly the configured browser origins.'
    Assert-NativeTest (Test-Path -LiteralPath (Join-Path $cleanRoot 'runtime\pgsql\bin\psql.exe')) 'A verified PostgreSQL archive must be extracted into the deployment runtime.'
    Assert-NativeTest (Test-Path -LiteralPath (Join-Path $cleanRoot 'runtime\native-windows.ps1')) 'Prepare must stage the lifecycle runner inside the deployment.'
    $cleanBackendEnv = Get-BackendEnvironment $cleanConfig
    Assert-NativeTest ($cleanBackendEnv.FRONTEND_ORIGIN -eq 'http://192.168.3.49:13001') 'The LAN origin must replace the loopback frontend origin.'
    Assert-NativeTest ($cleanBackendEnv.MULTICA_APP_URL -eq 'http://192.168.3.49:13001') 'Application links must use the LAN origin.'
    Assert-NativeTest ($cleanBackendEnv.CORS_ALLOWED_ORIGINS -eq 'http://192.168.3.49:13001,http://100.73.70.87:13001') 'Backend CORS must preserve the configured browser origins.'
    $rejected = $false
    try { Initialize-NativeCleanDeployment -Directory $cleanRoot -Database 'multica_test' -DatabasePort 16432 -ApiPort 18081 -WebPort 13001 -NodeExe $shell } catch { $rejected = $_.Exception.Message -like '*already configured*' }
    Assert-NativeTest $rejected 'Clean prepare must refuse to overwrite an existing deployment and its credentials.'
    $rejected = $false
    try { Initialize-NativeCleanDeployment -Directory (Join-Path $testRoot 'clean-bad-archive') -Database 'multica_bad' -DatabasePort 16433 -ApiPort 18082 -WebPort 13002 -PgArchive $cleanArchive -PgArchiveSha256 ('0' * 64) -NodeExe $shell } catch { $rejected = $_.Exception.Message -like '*SHA-256*' }
    Assert-NativeTest $rejected 'Clean prepare must reject a PostgreSQL archive that fails checksum verification.'
    $rejected = $false
    try { Resolve-NativeBrowserOrigins -PublicOrigin 'http://example.test/app' -CorsOrigins '' -WebPort 13000 } catch { $rejected = $true }
    Assert-NativeTest $rejected 'Browser origins must be origins, not application paths.'

    Assert-NativeTest ($cleanConfig.WebHost -eq '127.0.0.1') 'A clean deployment must default the web listener to loopback.'
    $lanConfig = $cleanConfig.PSObject.Copy()
    $lanConfig.WebHost = '0.0.0.0'
    $lanFrontendEnv = Get-FrontendEnvironment $lanConfig
    Assert-NativeTest ($lanFrontendEnv.HOSTNAME -eq '0.0.0.0') 'A LAN deployment must bind the web listener to the configured interface.'
    Assert-NativeTest ($lanFrontendEnv.REMOTE_API_URL -eq 'http://127.0.0.1:18081') 'Even a LAN web listener must reach the API over loopback.'

    $rejected = $false
    try { Initialize-NativeDatabase ([pscustomobject]@{ Database = 'bad-name; DROP DATABASE x'; DatabaseUser = 'example'; DatabasePassword = 'x'; DatabasePort = 1; Root = $testRoot }) } catch { $rejected = $_.Exception.Message -like '*lowercase database name*' }
    Assert-NativeTest $rejected 'The supervisor must reject an unsafe application database name before contacting PostgreSQL.'

    $xmlText = Get-NativeScheduledTaskXml -RunnerPath 'C:\Deploy & Test\runtime\native-windows.ps1' -Root 'C:\Deploy & Test'
    $taskXml = [xml]$xmlText
    Assert-NativeTest ($taskXml.Task.Principals.Principal.UserId -eq 'S-1-5-18') 'The persistence task must run as SYSTEM so it survives SSH sessions and reboots.'
    Assert-NativeTest ($taskXml.Task.Settings.Hidden -eq 'true') 'The persistence task must stay hidden.'
    Assert-NativeTest ($taskXml.Task.Triggers.BootTrigger.Enabled -eq 'true') 'The persistence task must start at boot.'
    Assert-NativeTest ($taskXml.Task.Actions.Exec.Arguments -like '*C:\Deploy & Test\runtime\native-windows.ps1*') 'Task arguments must survive XML escaping.'
    Assert-NativeTest ((Get-NativeScheduledTaskName ([pscustomobject]@{ Root = 'D:\UWorkers\clawsta80\native' })) -eq 'MulticaNative-D-UWorkers-clawsta80-native') 'The scheduled task name must be derived from the deployment root.'
    Assert-NativeTest (-not (Test-NativeScheduledTask 'MulticaNative-definitely-missing-task')) 'An unregistered task must be reported as missing rather than crashing the lifecycle.'

    foreach ($originCase in @(
        @{ Name = 'single-origin'; Input = 'http://192.168.3.49:13001'; Expected = 'http://192.168.3.49:13001' },
        @{ Name = 'default-origin'; Input = ''; Expected = 'http://localhost:13001' }
    )) {
        $originRoot = Join-Path $testRoot $originCase.Name
        Initialize-NativeCleanDeployment -Directory $originRoot -PublicOrigin $originCase.Input -WebPort 13001 -PgArchive $cleanArchive -PgArchiveSha256 $archiveHash -NodeExe $shell
        $originConfig = Read-NativeConfig $originRoot
        Assert-NativeTest ($originConfig.PublicOrigin -eq $originCase.Expected) 'A single/default origin must remain a full URL, not its first character.'
        $originEnvironment = Get-BackendEnvironment $originConfig
        Assert-NativeTest ($originEnvironment.FRONTEND_ORIGIN -eq $originCase.Expected -and $originEnvironment.MULTICA_APP_URL -eq $originCase.Expected) 'Single-origin application URLs must survive configuration and environment generation.'
        Assert-NativeTest ($originEnvironment.CORS_ALLOWED_ORIGINS -eq $originCase.Expected) 'Single-origin CORS must retain the complete origin.'
    }

    $importedRoot = Join-Path $testRoot 'imported-config'
    New-Item -ItemType Directory -Path $importedRoot | Out-Null
    Write-NativeJson (Join-Path $importedRoot 'deployment.json') @{ Root = $importedRoot; ApiPort = 18080; WebPort = 13000; BackendEnvironment = @{} }
    & $sourcePath -Action Configure -Root $importedRoot -WebHost '0.0.0.0' -PublicOrigin 'http://192.168.3.49:13000' -CorsOrigins 'http://192.168.3.49:13000'
    $importedConfig = Read-NativeConfig $importedRoot
    Assert-NativeTest ($importedConfig.WebHost -eq '0.0.0.0' -and $importedConfig.PublicOrigin -eq 'http://192.168.3.49:13000' -and $importedConfig.BackendEnvironment.CORS_ALLOWED_ORIGINS -eq 'http://192.168.3.49:13000') 'Configure must add LAN settings to an existing Docker-imported configuration.'

    $versionConfig = [pscustomobject]@{ Repo = (Join-Path $testRoot 'no-git-checkout'); Version = 'v0.3.1'; Commit = '' }
    Assert-NativeTest ((Resolve-NativeBuildVersion $versionConfig) -eq 'v0.3.1') 'An explicit version must let a clean machine build from source without a Git tag.'
    Assert-NativeTest ((Resolve-NativeBuildCommit $versionConfig) -eq 'unknown') 'A source tree without Git history must stamp an unknown commit instead of failing.'
    $rejected = $false
    try { Resolve-NativeBuildVersion ([pscustomobject]@{ Repo = (Join-Path $testRoot 'no-git-checkout'); Version = 'dev'; Commit = '' }) } catch { $rejected = $_.Exception.Message -like '*v1.2.3*' }
    Assert-NativeTest $rejected 'An invalid release version must be rejected before the build starts.'
    $rejected = $false
    try { Resolve-NativeBuildVersion ([pscustomobject]@{ Repo = (Join-Path $testRoot 'no-git-checkout'); Version = ''; Commit = '' }) } catch { $rejected = $_.Exception.Message -like '*Tagged source version required*' }
    Assert-NativeTest $rejected 'A tagless checkout without an explicit version must fail with an actionable error.'

    # Prebuilt release staging: the portable package ships binaries built on a
    # build machine and must never need Go or Node on the target machine.
    $stageRoot = Join-Path $testRoot 'stage-deploy'
    Initialize-NativeCleanDeployment -Directory $stageRoot -Database 'multica_stage' -DatabasePort 16434 -ApiPort 18083 -WebPort 13003 -Version 'v0.2.0' -PgArchive $cleanArchive -PgArchiveSha256 $archiveHash -NodeExe $shell
    $stageConfig = Read-NativeConfig $stageRoot
    $stageSource = Join-Path $testRoot 'prebuilt-release'
    New-Item -ItemType Directory -Path (Join-Path $stageSource 'web\apps\web'), (Join-Path $stageSource 'migrations') -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $stageSource 'server.exe'), 'server')
    [IO.File]::WriteAllText((Join-Path $stageSource 'web\apps\web\server.js'), 'web')
    [IO.File]::WriteAllText((Join-Path $stageSource 'migrations\001_init.up.sql'), 'select 1;')
    Stage-NativeDeployment $stageConfig $stageSource
    $stagedConfig = Read-NativeConfig $stageRoot
    Assert-NativeTest ($stagedConfig.Release -and (Test-Path -LiteralPath (Join-Path $stagedConfig.Release 'server.exe'))) 'Stage must publish a prebuilt release and record it in deployment.json.'
    Assert-NativeTest (Test-Path -LiteralPath (Join-Path $stagedConfig.Release 'web\apps\web\server.js')) 'Stage must copy the standalone web app.'
    Assert-NativeTest (Test-Path -LiteralPath (Join-Path $stagedConfig.Release 'migrations\001_init.up.sql')) 'Stage must copy the migrations the supervisor runs.'
    Assert-NativeTest ((Get-Content -LiteralPath (Join-Path $stagedConfig.Release 'server.exe') -Raw).Trim() -eq 'server') 'Stage must copy the prebuilt server binary unchanged.'
    $rejected = $false
    try { Stage-NativeDeployment $stagedConfig (Join-Path $testRoot 'missing-release') } catch { $rejected = $_.Exception.Message -like '*server.exe*' }
    Assert-NativeTest $rejected 'Stage must refuse a directory that is not a prebuilt release.'

    Write-Host 'PASS: native Windows configuration, environment isolation, process ownership, state, clean-deploy, prebuilt staging, persistence-task, and build-stamp tests.'
} finally {
    $resolvedRoot = [IO.Path]::GetFullPath($testRoot)
    $expectedParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolvedRoot.StartsWith($expectedParent, [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolvedRoot) -notlike 'multica-native-test-*') { throw 'Unsafe test cleanup path.' }
    Remove-Item -LiteralPath $resolvedRoot -Recurse -Force
}
