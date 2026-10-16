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
        BackendEnvironment = [pscustomobject]@{ JWT_SECRET = 'test-secret'; PORT = '8080'; DATABASE_URL = 'old'; FRONTEND_ORIGIN = 'old' }
        FrontendEnvironment = [pscustomobject]@{ REMOTE_API_URL = 'http://backend:8080'; NEXT_PUBLIC_WS_URL = '' }
    }
    $backendEnv = Get-BackendEnvironment $config
    Assert-NativeTest ($backendEnv.JWT_SECRET -eq 'test-secret') 'Existing application secrets must survive migration.'
    Assert-NativeTest ($backendEnv.HOST -eq '127.0.0.1') 'The native API must bind to loopback.'
    Assert-NativeTest ($backendEnv.FRONTEND_ORIGIN -eq 'http://localhost:13000') 'The rehearsal frontend origin must use its isolated port.'
    Assert-NativeTest ($backendEnv.MULTICA_DAEMON_SERVER_URL -eq 'http://localhost:18080') 'Daemon setup must point to the API listener with native WebSocket support.'
    Assert-NativeTest ($backendEnv.DATABASE_URL -like '*a%40b%3Ac%2F%23%3F@127.0.0.1:15432/example*') 'Database credentials must be URI escaped.'
    $frontendEnv = Get-FrontendEnvironment $config
    Assert-NativeTest ($frontendEnv.REMOTE_API_URL -eq 'http://127.0.0.1:18080') 'Frontend API proxy must target the native backend.'
    Assert-NativeTest ($frontendEnv.NEXT_PUBLIC_WS_URL -eq 'ws://localhost:18080/ws') 'Browser realtime must target the native API rather than a different running deployment.'

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
    Write-Host 'PASS: native Windows configuration, environment isolation, process ownership, and state tests.'
} finally {
    $resolvedRoot = [IO.Path]::GetFullPath($testRoot)
    $expectedParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolvedRoot.StartsWith($expectedParent, [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolvedRoot) -notlike 'multica-native-test-*') { throw 'Unsafe test cleanup path.' }
    Remove-Item -LiteralPath $resolvedRoot -Recurse -Force
}
