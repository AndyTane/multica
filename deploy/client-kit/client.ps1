#Requires -Version 5.1
[CmdletBinding()]
param([ValidateSet('Connect','Reconnect','Status','Stop')][string]$Action = 'Reconnect')
$ErrorActionPreference = 'Stop'

# User launchers must not inherit a task's credentials or daemon identity.
$saved = @{}
foreach ($entry in Get-ChildItem Env:) {
    if ($entry.Name -match '^(MULTICA_|U_WORKERS_)') {
        $saved[$entry.Name] = $entry.Value
        Remove-Item -LiteralPath ('Env:'+$entry.Name)
    }
}
function Quote-Argument([string]$Value) {
    return '"'+([regex]::Replace([regex]::Replace($Value,'(\\*)"','$1$1\"'),'(\\+)$','$1$1'))+'"'
}
function Invoke-Client([string[]]$Arguments,[switch]$Capture) {
    $argv = @(); if ($settings.profile) { $argv += @('--profile',[string]$settings.profile) }; $argv += $Arguments
    if (-not $Capture) {
        & $cli @argv
        if ($LASTEXITCODE -ne 0) { throw 'CLI command failed. Follow the message above; use Connect.cmd if sign-in expired.' }
        return
    }
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName=$cli; $start.WorkingDirectory=$PSScriptRoot; $start.UseShellExecute=$false; $start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
    $start.StandardOutputEncoding=New-Object Text.UTF8Encoding($false)
    $start.StandardErrorEncoding=New-Object Text.UTF8Encoding($false)
    $start.Arguments=(($argv | ForEach-Object { Quote-Argument $_ }) -join ' ')
    $process=New-Object Diagnostics.Process; $process.StartInfo=$start
    try {
        $null=$process.Start()
        $stdout=$process.StandardOutput.ReadToEndAsync(); $stderr=$process.StandardError.ReadToEndAsync()
        $process.WaitForExit(); $raw=$stdout.GetAwaiter().GetResult(); $null=$stderr.GetAwaiter().GetResult()
        if ($process.ExitCode -ne 0) { throw ('CLI query failed (exit '+$process.ExitCode+'). Check profile, server access and sign-in; use Connect.cmd for first sign-in.') }
        try { return ($raw | ConvertFrom-Json -ErrorAction Stop) } catch { throw 'CLI returned invalid status JSON.' }
    } finally { $process.Dispose() }
}
function Get-ClientStatus {
    $status=Invoke-Client @('daemon','status','--output','json') -Capture
    if ($status.status -notin @('running','starting','stopped')) { throw 'CLI returned an unknown daemon status.' }
    if ($status.port_conflict) { throw 'Another profile owns this daemon port. Check Status.cmd and the selected profile before reconnecting.' }
    return $status
}
try {
    $cli=Join-Path $PSScriptRoot 'u-workers.exe'
    if (-not (Test-Path -LiteralPath $cli -PathType Leaf)) { throw 'Missing u-workers.exe. Extract the complete client kit first.' }
    $settings=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'connection.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    if ([string]$settings.profile -match '[\\/:\x00-\x1f]' -or [string]$settings.profile -in @('.','..')) { throw 'Invalid profile in connection.json.' }
    if ($Action -eq 'Connect') {
        $argv=@('connect')
        if ($settings.serverUrl) {
            $uri=$null
            if (-not [Uri]::TryCreate([string]$settings.serverUrl,[UriKind]::Absolute,[ref]$uri) -or $uri.Scheme -ne 'https' -or $uri.UserInfo -or $uri.Query -or $uri.Fragment) { throw 'serverUrl must be an HTTPS URL without credentials, query or fragment.' }
            $argv+=@('--server-url',[string]$settings.serverUrl)
        }
        if ($settings.rootCa) {
            $ca=[string]$settings.rootCa
            if (-not [IO.Path]::IsPathRooted($ca)) { $ca=Join-Path $PSScriptRoot $ca }
            if (-not (Test-Path -LiteralPath $ca -PathType Leaf)) { throw 'rootCa file was not found. Obtain the public certificate from the server administrator.' }
            $argv+=@('--root-ca',$ca)
        }
        Invoke-Client $argv
    } elseif ($Action -eq 'Stop') {
        Invoke-Client @('daemon','stop')
        return
    } elseif ($Action -eq 'Reconnect') {
        $status=Get-ClientStatus
        if ($status.status -notin @('running','starting')) {
            Invoke-Client @('daemon','start')
        }
    }
    $status=Get-ClientStatus
    if ($status.status -notin @('running','starting')) { throw 'Daemon is stopped. Run Reconnect.cmd; for first sign-in use Connect.cmd.' }
    Write-Host ('Daemon: '+$status.status+'; profile: '+$(if ($settings.profile) {$settings.profile} else {'default'})+'; PID: '+$status.pid)
    # Authenticated read distinguishes server access from local process liveness.
    $null=Invoke-Client @('workspace','list','--output','json') -Capture
    Write-Host 'Server access verified. Check this machine in the web runtime list.' -ForegroundColor Green
    if (@($status.agents).Count -eq 0) { Write-Host 'No agent CLI detected yet. Install/configure the desired agent CLI before assigning work.' -ForegroundColor Yellow }
} finally {
    foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') }
}
