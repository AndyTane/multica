#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference = 'Stop'
$template = Split-Path -Parent $PSScriptRoot
$work = Join-Path ([IO.Path]::GetTempPath()) ('client settings 中文 '+[guid]::NewGuid().ToString('N'))
$old = Join-Path $work 'old kit'; $kit = Join-Path $work 'new kit'
New-Item -ItemType Directory -Path $old,$kit | Out-Null
Get-ChildItem -LiteralPath $template -File | Copy-Item -Destination $kit
Copy-Item (Join-Path $PSScriptRoot 'public-test-ca.crt') (Join-Path $old 'root.crt')
& "$env:WINDIR/Microsoft.NET/Framework64/v4.0.30319/csc.exe" /nologo /target:exe ('/out:'+(Join-Path $kit 'u-workers.exe')) (Join-Path $PSScriptRoot 'fake-client.cs') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Fake compile failed' }
$checks = New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok,[string]$Label) { if (-not $Ok) { throw $Label }; $checks.Add($Label) }
function Fail([scriptblock]$Run,[string]$Label) { $failed=$false; try { & $Run | Out-Null } catch { $failed=$true }; Check $failed $Label }
function Settings([string]$Directory,[string]$Server='https://server.invalid:13443',[string]$Ca='',[string]$Profile='office') {
    [IO.File]::WriteAllText((Join-Path $Directory 'connection.json'),(@{profile=$Profile;serverUrl=$Server;rootCa=$Ca}|ConvertTo-Json),(New-Object Text.UTF8Encoding($false)))
}
function Hash([string]$Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash }
function Calls { return @(Get-Content (Join-Path $kit 'calls.txt') -ErrorAction SilentlyContinue) }
$global:ClientTestAnswers = New-Object 'System.Collections.Generic.Queue[string]'
function Read-Host([string]$Prompt) {
    if ($global:ClientTestAnswers.Count -eq 0) { throw ('Unexpected prompt: '+$Prompt) }
    return $global:ClientTestAnswers.Dequeue()
}
$saved=@{}; foreach ($name in @('KIT_TEST_ROOT','KIT_SERVER_FAIL','UWORKERS_NO_PAUSE')) { $saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
try {
    $env:KIT_TEST_ROOT=$kit; $env:KIT_SERVER_FAIL=''; $env:UWORKERS_NO_PAUSE='1'
    $client=Join-Path $kit 'client.ps1'
    Fail { & $client -Action Connect -NonInteractive } 'noninteractive first connect requires server without prompting'
    Check ((Calls).Count -eq 0) 'missing first connection never calls CLI'
    foreach ($answer in @('https://server.invalid:13443',(Join-Path $old 'root.crt'),'office')) { $global:ClientTestAnswers.Enqueue($answer) }
    & $client -Action Connect -Interactive
    $first=Get-Content (Join-Path $kit 'connection.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    Check ($first.profile -eq 'office' -and $first.serverUrl -eq 'https://server.invalid:13443' -and $global:ClientTestAnswers.Count -eq 0) 'first connection collects and stores three settings'
    Check (@(Calls | Where-Object { $_ -like '--profile|office|connect|*|--root-ca|*' }).Count -eq 1) 'validated first input passed to connect'
    $prior=Hash (Join-Path $kit 'connection.json')
    Fail { & $client -Action Connect -ServerUrl https://other.invalid -NonInteractive } 'personal settings require explicit replacement'
    $global:ClientTestAnswers.Enqueue('N'); & $client -Action Connect -ServerUrl https://other.invalid -Interactive
    Check ((Hash (Join-Path $kit 'connection.json')) -eq $prior) 'cancelled address change leaves settings unchanged'
    foreach ($url in @('http://server.invalid','https://name:secret@server.invalid','https://server.invalid/?token=secret','https://server.invalid/path')) {
        Fail { & $client -Action Connect -ServerUrl $url -ReplaceSettings } ('unsafe URL rejected: '+$url)
    }
    Check ((Hash (Join-Path $kit 'connection.json')) -eq $prior) 'invalid input preserves existing settings'
    Settings $old 'https://server.invalid:13443' 'root.crt' 'old-profile'
    $oldHash=Hash (Join-Path $old 'connection.json')
    [IO.File]::WriteAllText((Join-Path $old '.env'),'FAKE_PRIVATE_DO_NOT_COPY')
    [IO.File]::WriteAllText((Join-Path $old 'credentials.json'),'FAKE_LOGIN_DO_NOT_COPY')
    Fail { & $client -Action Import -SourceDirectory $old } 'import protects personalised destination'
    $global:ClientTestAnswers.Enqueue('N'); & $client -Action Import -SourceDirectory $old -Interactive
    Check ((Hash (Join-Path $kit 'connection.json')) -eq $prior) 'cancelled import preserves configuration'
    $callCount=(Calls).Count
    & $client -Action Import -SourceDirectory $old -ReplaceSettings
    $imported=Get-Content (Join-Path $kit 'connection.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    Check ($imported.profile -eq 'old-profile' -and $imported.rootCa.StartsWith('certificates/public-')) 'relative public certificate and original profile migrated'
    Check ((Hash (Join-Path $kit $imported.rootCa)) -eq (Hash (Join-Path $old 'root.crt'))) 'copied public certificate has identical bytes'
    Check ((Hash (Join-Path $old 'connection.json')) -eq $oldHash -and (Calls).Count -eq $callCount) 'import leaves old kit and daemon untouched'
    Check (-not (Test-Path (Join-Path $kit '.env')) -and -not (Test-Path (Join-Path $kit 'credentials.json'))) 'login and env files never copied'
    $prior=Hash (Join-Path $kit 'connection.json')
    [IO.File]::WriteAllText((Join-Path $old 'private.pem'),([IO.File]::ReadAllText((Join-Path $old 'root.crt'))+"`n-----BEGIN PRIVATE KEY-----`nFAKE_ONLY`n-----END PRIVATE KEY-----"))
    Settings $old 'https://server.invalid' 'private.pem'
    Fail { & $client -Action Import -SourceDirectory $old -ReplaceSettings } 'certificate containing private key rejected'
    Settings $old 'https://server.invalid' '../outside.crt'
    Fail { & $client -Action Import -SourceDirectory $old -ReplaceSettings } 'relative certificate escape rejected'
    $link=Join-Path $old 'linked'
    New-Item -ItemType Junction -Path $link -Target $kit | Out-Null
    Settings $old 'https://server.invalid' 'linked/root.crt'
    Fail { & $client -Action Import -SourceDirectory $old -ReplaceSettings } 'certificate directory junction rejected'
    [IO.Directory]::Delete($link)
    Settings $old 'https://server.invalid' '' '../bad'
    Fail { & $client -Action Import -SourceDirectory $old -ReplaceSettings } 'invalid imported profile rejected'
    [IO.File]::WriteAllText((Join-Path $old 'connection.json'),'{"profile":"office","serverUrl":"https://server.invalid","rootCa":"","token":"FAKE_PRIVATE"}')
    Fail { & $client -Action Import -SourceDirectory $old -ReplaceSettings } 'unknown settings field rejected'
    Check ((Hash (Join-Path $kit 'connection.json')) -eq $prior) 'all failed imports preserve destination settings'
    [IO.File]::WriteAllText((Join-Path $old 'other.crt'),([IO.File]::ReadAllText((Join-Path $old 'root.crt'))+"`r`n"))
    Settings $old 'https://server.invalid' 'other.crt'
    $certificateCount=@(Get-ChildItem (Join-Path $kit 'certificates') -File).Count
    $configPath=Join-Path $kit 'connection.json'
    [IO.File]::SetAttributes($configPath,[IO.FileAttributes]::ReadOnly)
    try { Fail { & $client -Action Import -SourceDirectory $old -ReplaceSettings } 'atomic config replacement failure reported' }
    finally { [IO.File]::SetAttributes($configPath,[IO.FileAttributes]::Normal) }
    Check ((Hash $configPath) -eq $prior -and @(Get-ChildItem (Join-Path $kit 'certificates') -File).Count -eq $certificateCount) 'failed atomic write preserves settings and removes new certificate'
    Settings $old 'https://server.invalid' (Join-Path $old 'root.crt') 'absolute-profile'
    & $client -Action Import -SourceDirectory $old -ReplaceSettings
    $absolute=Get-Content (Join-Path $kit 'connection.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    Check ($absolute.rootCa -eq (Join-Path $old 'root.crt')) 'valid absolute public certificate reference preserved'
    [IO.File]::WriteAllText((Join-Path $kit 'state.txt'),'stopped')
    $log=(& $client -Action Status 6>&1 | Out-String)
    Check ($log.Contains('尚未连接') -and $log.Contains('Reconnect.cmd')) 'stopped is a normal Chinese state'
    $env:KIT_SERVER_FAIL='1'; [IO.File]::WriteAllText((Join-Path $kit 'state.txt'),'starting')
    $log=(& $client -Action Status 6>&1 | Out-String)
    Check ($log.Contains('连接中') -and -not $log.Contains('服务器访问已验证')) 'starting does not claim server access'
    [IO.File]::WriteAllText((Join-Path $kit 'state.txt'),'running')
    Fail { & $client -Action Status } 'running with server failure never reports success'
    $env:KIT_SERVER_FAIL=''
    Settings $kit 'https://server.invalid' 'no-longer-present.crt' 'absolute-profile'
    [IO.File]::WriteAllText((Join-Path $kit 'state.txt'),'stopped')
    $reconnectLog=(& $client -Action Reconnect 6>&1 | Out-String)
    Check ($reconnectLog.Contains('服务器访问已验证')) 'reconnect reuses CLI certificate when original public input file has moved'
    Settings $kit '' '' 'preserved-profile'
    $global:ClientTestAnswers.Enqueue('https://server.invalid')
    $global:ClientTestAnswers.Enqueue('')
    $global:ClientTestAnswers.Enqueue('Y')
    & $client -Action Connect -Interactive
    Check ((Get-Content (Join-Path $kit 'connection.json') -Raw -Encoding UTF8 | ConvertFrom-Json).profile -eq 'preserved-profile' -and $global:ClientTestAnswers.Count -eq 0) 'first input preserves an existing nonempty profile'
    # Exercise the Windows 5.1 launcher through cmd, preserving its real exit code.
    $start=New-Object Diagnostics.ProcessStartInfo
    $start.FileName=$env:ComSpec; $start.UseShellExecute=$false; $start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
    $start.Arguments='/d /c ""'+(Join-Path $kit 'Connect.cmd')+'" -NonInteractive -ServerUrl http://bad.invalid -ReplaceSettings"'
    $proc=New-Object Diagnostics.Process; $proc.StartInfo=$start; $null=$proc.Start()
    $out=$proc.StandardOutput.ReadToEndAsync(); $err=$proc.StandardError.ReadToEndAsync(); $proc.WaitForExit()
    $null=$out.GetAwaiter().GetResult(); $null=$err.GetAwaiter().GetResult()
    Check ($proc.ExitCode -ne 0) 'cmd preserves failing PowerShell exit code in Unicode path'
    $proc.Dispose()
    $result=@{powershell=$PSVersionTable.PSVersion.ToString();passed=$checks.Count;checks=@($checks);transport='fake CLI and public test certificate; no services or account changes'}
    if ($EvidencePath) { [IO.File]::WriteAllText($EvidencePath,($result|ConvertTo-Json -Depth 5),(New-Object Text.UTF8Encoding($false))) }
    $result|ConvertTo-Json -Depth 5
} finally { foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') } }
