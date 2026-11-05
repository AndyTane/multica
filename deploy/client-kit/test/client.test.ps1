#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference='Stop'
$template=Split-Path -Parent $PSScriptRoot
$work=Join-Path ([IO.Path]::GetTempPath()) ('client kit '+[char]0x4E2D+[char]0x6587+' '+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
foreach ($name in @('client.ps1','client-settings.ps1','connection.json')) { Copy-Item (Join-Path $template $name) (Join-Path $work $name) }
[IO.File]::WriteAllText((Join-Path $work 'opencode-selection.json'),'{"schemaVersion":1,"selections":[{"profile":"","disabled":true},{"profile":"office","disabled":true}]}')
& "$env:WINDIR/Microsoft.NET/Framework64/v4.0.30319/csc.exe" /nologo /target:exe ('/out:'+(Join-Path $work 'u-workers.exe')) (Join-Path $PSScriptRoot 'fake-client.cs') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Fake compile failed.' }
$checks=New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok,[string]$Label) { if (-not $Ok) { throw $Label }; $checks.Add($Label) }
function Fail([scriptblock]$Run,[string]$Label) { $failed=$false; try { & $Run | Out-Null } catch { $failed=$true }; Check $failed $Label }
function Calls { return @(Get-Content (Join-Path $work 'calls.txt') -ErrorAction SilentlyContinue) }
function Reset([string]$State) {
    [IO.File]::WriteAllText((Join-Path $work 'state.txt'),$State)
    [IO.File]::WriteAllText((Join-Path $work 'calls.txt'),'')
}
$names=@('KIT_TEST_ROOT','KIT_BAD_JSON','KIT_STATUS_FAIL','KIT_START_FAIL','KIT_SERVER_FAIL','MULTICA_TOKEN','U_WORKERS_TASK_ID','MULTICA_DSH_WEB_CONFIG','KIT_EXPECT_DSH_WEB_CONFIG')
$saved=@{}; foreach ($name in $names) { $saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
try {
    $env:KIT_TEST_ROOT=$work; $env:KIT_BAD_JSON=''; $env:KIT_STATUS_FAIL=''; $env:KIT_START_FAIL=''; $env:KIT_SERVER_FAIL=''
    $env:MULTICA_TOKEN='FAKE_PARENT_ONLY'; $env:U_WORKERS_TASK_ID='FAKE_PARENT_ONLY'
    $client=Join-Path $work 'client.ps1'
    Reset 'stopped'; & $client -Action Reconnect
    Check (@(Calls | Where-Object { $_ -eq 'daemon|start' }).Count -eq 1) 'stopped daemon starts exactly once in path with spaces and Unicode'
    Check ($env:MULTICA_TOKEN -eq 'FAKE_PARENT_ONLY' -and $env:U_WORKERS_TASK_ID -eq 'FAKE_PARENT_ONLY') 'parent environment restored and task identity excluded from child'
    Check (@(Calls | Where-Object { $_ -match 'connect|login|restart|stop' }).Count -eq 0) 'reconnect never logs in, restarts or stops'
    foreach ($state in @('running','starting')) {
        Reset $state; & $client -Action Reconnect
        Check (@(Calls | Where-Object { $_ -match 'start$|stop$|connect' }).Count -eq 0) ($state+' daemon left untouched')
    }
    Reset 'stopped'; $env:KIT_START_FAIL='1'
    Fail { & $client -Action Reconnect } 'failed daemon start reported'
    $env:KIT_START_FAIL=''; $env:KIT_STATUS_FAIL='1'
    Fail { & $client -Action Reconnect } 'failed status never treated as stopped'
    Check (@(Calls | Where-Object { $_ -eq 'daemon|start' }).Count -eq 1) 'status failure does not start a daemon'
    $env:KIT_STATUS_FAIL=''; $env:KIT_BAD_JSON='1'
    Fail { & $client -Action Reconnect } 'malformed status rejected'
    $env:KIT_BAD_JSON=''; Reset 'unknown'
    Fail { & $client -Action Reconnect } 'unknown status rejected before daemon start'
    Check (@(Calls | Where-Object { $_ -eq 'daemon|start' }).Count -eq 0) 'unknown status does not start a daemon'
    $env:KIT_SERVER_FAIL='1'; Reset 'running'
    Fail { & $client -Action Reconnect } 'server failure distinct from local daemon liveness'
    Check (@(Calls | Where-Object { $_ -match 'start$|stop$|connect' }).Count -eq 0) 'network failure preserves active daemon'
    $env:KIT_SERVER_FAIL=''
    [IO.File]::WriteAllText((Join-Path $work 'connection.json'),'{"profile":"office","serverUrl":"https://server.invalid:13443","rootCa":""}')
    Reset 'running'; & $client -Action Status
    Check (@(Calls | Where-Object { -not $_.StartsWith('--profile|office|') }).Count -eq 0) 'all commands use configured profile'
    & $client -Action Stop
    Check ((Get-Content (Join-Path $work 'state.txt')) -eq 'stopped') 'explicit stop targets configured profile'
    & $client -Action Connect
    Check (@(Calls | Where-Object { $_ -eq '--profile|office|connect|--server-url|https://server.invalid:13443' }).Count -eq 1) 'first connect uses configured server'
    $env:MULTICA_DSH_WEB_CONFIG=Join-Path $work 'selected-dsh-web.json';$env:KIT_EXPECT_DSH_WEB_CONFIG=$env:MULTICA_DSH_WEB_CONFIG
    Reset 'stopped'; & $client -Action Reconnect
    Check ((Get-Content (Join-Path $work 'runtime-env.txt') -Encoding UTF8)[2] -eq $env:MULTICA_DSH_WEB_CONFIG -and $env:MULTICA_TOKEN -eq 'FAKE_PARENT_ONLY') 'explicit DSH bridge file survives launch while parent task credentials stay excluded'
    [IO.File]::WriteAllText((Join-Path $work 'connection.json'),'{"profile":"../bad","serverUrl":"","rootCa":""}')
    Fail { & $client -Action Reconnect } 'invalid profile rejected before lifecycle command'
    $result=@{powershell=$PSVersionTable.PSVersion.ToString();passed=$checks.Count;checks=@($checks);transport='fake CLI; no agents or network'}
    if ($EvidencePath) { [IO.File]::WriteAllText($EvidencePath,($result|ConvertTo-Json -Depth 5),(New-Object Text.UTF8Encoding($false))) }
    $result | ConvertTo-Json -Depth 5
} finally { foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') } }
