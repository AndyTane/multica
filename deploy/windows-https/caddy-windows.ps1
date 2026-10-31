# U Workers HTTPS entry lifecycle (Caddy, Windows).
#
# The deployment's own native-windows.ps1 owns PostgreSQL/API/web. This script
# owns only the TLS reverse proxy that fronts them, and it must keep running
# independently of whichever agent or SSH session started it:
#
#   -Action Start    register the ONSTART scheduled task (once) and run it now
#   -Action Run      the supervisor loop the scheduled task executes
#   -Action Stop     stop the proxy (verified PID + start time)
#   -Action Status   task, process, listeners, certificate and upstream checks
#   -Action Trust    install the local CA root into this host's machine store
#   -Action Untrust  remove that trust entry
#   -Action ExportRoot  copy the public root certificate out for clients
#
# Nothing here logs request bodies, authorization headers or cookies: Caddy's
# console log format records method, URI, status, size and timing only.

[CmdletBinding()]
param(
    [ValidateSet('Start', 'Run', 'Stop', 'Status', 'Trust', 'Untrust', 'ExportRoot')]
    [string]$Action = 'Status',
    [string]$Root = 'D:\UWorkers\clawsta80\native'
)

$ErrorActionPreference = 'Stop'
$script:Base = Join-Path $Root 'https'
$script:Exe = Join-Path $script:Base 'caddy.exe'
$script:Caddyfile = Join-Path $script:Base 'Caddyfile'
$script:StateFile = Join-Path $script:Base 'state\caddy.json'
$script:StopFile = Join-Path $script:Base 'state\stop'
$script:DataDir = Join-Path $script:Base 'data'
$script:RootCert = Join-Path $script:DataDir 'pki\authorities\local\root.crt'
$script:PublicCert = Join-Path $script:Base 'public\root-ca.crt'
$script:LogDir = Join-Path $script:Base 'logs'
$script:TaskName = 'UWorkers-CLAWSTA80-HTTPS'
$script:Schtasks = Join-Path $env:SystemRoot 'System32\schtasks.exe'

function Invoke-Schtasks {
    param([string[]]$Arguments)
    # schtasks writes expected diagnostics (for example "task not found") to
    # stderr. PowerShell 5.1 with ErrorActionPreference=Stop turns that into a
    # terminating error, so run it with a local Continue and keep the exit code.
    $previous = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $output = & $script:Schtasks @Arguments 2>&1 | Out-String
        return [pscustomobject]@{ ExitCode = $LASTEXITCODE; Output = $output }
    } finally {
        $ErrorActionPreference = $previous
    }
}
$script:ListenAddress = '192.168.3.49'
$script:ListenPort = 13443
$script:RedirectPort = 13001


function Invoke-Caddy {
    param([string[]]$Arguments)
    # Caddy writes structured info logs to stderr; PowerShell 5.1 with
    # ErrorActionPreference=Stop would treat those as terminating errors.
    $previous = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $output = & $script:Exe @Arguments 2>&1 | Out-String
        return [pscustomobject]@{ ExitCode = $LASTEXITCODE; Output = $output }
    } finally {
        $ErrorActionPreference = $previous
    }
}

function Write-LifecycleLog {
    param([string]$Message)
    New-Item -ItemType Directory -Path $script:LogDir -Force | Out-Null
    $line = (Get-Date).ToUniversalTime().ToString('o') + ' ' + $Message
    Add-Content -LiteralPath (Join-Path $script:LogDir 'supervisor.log') -Value $line -Encoding utf8
}

function Get-OwnedProcess {
    # Returns the recorded caddy process only when PID, path and start time all
    # match, so a recycled PID is never mistaken for ours.
    if (-not (Test-Path -LiteralPath $script:StateFile)) { return $null }
    $state = Get-Content -LiteralPath $script:StateFile -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $state.Caddy) { return $null }
    $process = Get-Process -Id ([int]$state.Caddy.Pid) -ErrorAction SilentlyContinue
    if (-not $process) { return $null }
    if ($process.Path -ne [string]$state.Caddy.Executable) { return $null }
    $recorded = [datetime]::Parse([string]$state.Caddy.StartedAt, $null, [Globalization.DateTimeStyles]::RoundtripKind)
    $delta = ($process.StartTime.ToUniversalTime() - $recorded.ToUniversalTime()).TotalSeconds
    if ([math]::Abs($delta) -gt 2) { return $null }
    return $process
}

function Get-ProcessRecord {
    param($Process)
    return [ordered]@{
        Pid = $Process.Id
        Executable = $Process.Path
        StartedAt = $Process.StartTime.ToUniversalTime().ToString('o')
    }
}

function Wait-Listening {
    param([int]$Port, [int]$TimeoutSeconds)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) { return $true }
        } catch { }
        Start-Sleep -Seconds 1
    }
    return $false
}

function Register-ScheduledTaskIfMissing {
    $query = Invoke-Schtasks @('/Query', '/TN', $script:TaskName)
    if ($query.ExitCode -eq 0) { return }
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $arguments = '-NoProfile -ExecutionPolicy Bypass -File "' + $PSCommandPath + '" -Action Run -Root "' + $Root + '"'
    $create = Invoke-Schtasks @('/Create', '/TN', $script:TaskName, '/SC', 'ONSTART', '/RU', 'SYSTEM', '/RL', 'HIGHEST', '/TR', ('"' + $shell + '" ' + $arguments), '/F')
    if ($create.ExitCode -ne 0) { throw ('Failed to register HTTPS scheduled task: ' + $create.Output) }
    Write-LifecycleLog ('registered scheduled task ' + $script:TaskName)
}

function Invoke-Run {
    New-Item -ItemType Directory -Path (Split-Path -Parent $script:StateFile) -Force | Out-Null
    if (Test-Path -LiteralPath $script:StopFile) { Remove-Item -LiteralPath $script:StopFile -Force }
    Write-LifecycleLog 'supervisor starting'
    try {
        while (-not (Test-Path -LiteralPath $script:StopFile)) {
            $caddy = Start-Process -FilePath $script:Exe -ArgumentList @('run', '--config', $script:Caddyfile, '--adapter', 'caddyfile') -PassThru -WindowStyle Hidden
            $record = [ordered]@{ Caddy = (Get-ProcessRecord $caddy); UpdatedAt = (Get-Date).ToUniversalTime().ToString('o') }
            [IO.File]::WriteAllText($script:StateFile, ($record | ConvertTo-Json -Depth 5), [Text.UTF8Encoding]::new($false))
            Write-LifecycleLog ('caddy started pid=' + $caddy.Id)
            while (-not $caddy.HasExited) {
                if (Test-Path -LiteralPath $script:StopFile) {
                    Stop-Process -Id $caddy.Id -Force -ErrorAction SilentlyContinue
                    break
                }
                Start-Sleep -Seconds 2
            }
            $caddy.WaitForExit()
            Remove-Item -LiteralPath $script:StateFile -Force -ErrorAction SilentlyContinue
            if (Test-Path -LiteralPath $script:StopFile) { break }
            Write-LifecycleLog ('caddy exited code=' + $caddy.ExitCode + '; restarting in 3s')
            Start-Sleep -Seconds 3
        }
    } finally {
        Write-LifecycleLog 'supervisor stopped'
        Remove-Item -LiteralPath $script:StopFile -Force -ErrorAction SilentlyContinue
    }
}

function Invoke-Start {
    Register-ScheduledTaskIfMissing
    $run = Invoke-Schtasks @('/Run', '/TN', $script:TaskName)
    if ($run.ExitCode -ne 0) { throw ('Failed to start HTTPS task: ' + $run.Output) }
    if (-not (Wait-Listening $script:ListenPort 60)) {
        throw 'HTTPS listener did not come up within 60 seconds; inspect https\logs\access.log and https\logs\supervisor.log'
    }
    Write-Output ('HTTPS entry listening on https://' + $script:ListenAddress + ':' + $script:ListenPort)
}

function Invoke-Stop {
    New-Item -ItemType Directory -Path (Split-Path -Parent $script:StopFile) -Force | Out-Null
    Set-Content -LiteralPath $script:StopFile -Value 'stop' -Encoding ascii
    [void](Invoke-Schtasks @('/End', '/TN', $script:TaskName))
    $deadline = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $deadline) {
        if (-not (Get-OwnedProcess)) { break }
        Start-Sleep -Seconds 1
    }
    $owned = Get-OwnedProcess
    if ($owned) {
        Stop-Process -Id $owned.Id -Force -ErrorAction SilentlyContinue
        Write-Output ('forced stop of verified caddy pid ' + $owned.Id)
    }
    Remove-Item -LiteralPath $script:StateFile -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $script:StopFile -Force -ErrorAction SilentlyContinue
    Write-Output 'HTTPS entry stopped'
}

function Invoke-Status {
    $task = Invoke-Schtasks @('/Query', '/TN', $script:TaskName, '/FO', 'LIST')
    $taskOk = $task.ExitCode -eq 0
    $taskState = ''
    if ($taskOk) {
        # The task state label is OS-localized; report the query output as-is
        # instead of pattern-matching an English word.
        $taskState = [string]$task.Output.Trim()
    }
    $owned = Get-OwnedProcess
    $listening = $false
    try { $listening = [bool](Get-NetTCPConnection -State Listen -LocalPort $script:ListenPort -ErrorAction SilentlyContinue) } catch { }
    $redirectListening = $false
    try { $redirectListening = [bool](Get-NetTCPConnection -State Listen -LocalPort $script:RedirectPort -ErrorAction SilentlyContinue) } catch { }
    $cert = $null
    if (Test-Path -LiteralPath $script:RootCert) {
        $c = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($script:RootCert)
        $cert = [ordered]@{
            subject = $c.Subject
            sha256 = $c.GetCertHashString([System.Security.Cryptography.HashAlgorithmName]::SHA256)
            not_after = $c.NotAfter.ToUniversalTime().ToString('o')
        }
    }
    [pscustomobject]@{
        task_installed = $taskOk
        task_state = $taskState
        caddy_running = [bool]$owned
        caddy_pid = if ($owned) { $owned.Id } else { $null }
        https_listening = $listening
        http_redirect_listening = $redirectListening
        ca_root = $cert
        root_cert_path = if (Test-Path -LiteralPath $script:PublicCert) { $script:PublicCert } else { $null }
    } | ConvertTo-Json -Depth 5
}

function Invoke-Trust {
    # Caddy's own "trust" command needs its admin API and its certmgr path is not
    # available in this Windows context, so install the public root directly into
    # this host's machine trust store. The private key never leaves https\data.
    if (-not (Test-Path -LiteralPath $script:RootCert)) { throw 'local CA root certificate is missing; start the proxy once first' }
    New-Item -ItemType Directory -Path (Split-Path -Parent $script:PublicCert) -Force | Out-Null
    Copy-Item -LiteralPath $script:RootCert -Destination $script:PublicCert -Force
    $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($script:PublicCert)
    $thumbprint = $cert.Thumbprint
    $installed = @(Get-ChildItem Cert:\LocalMachine\Root | Where-Object { $_.Thumbprint -eq $thumbprint })
    if ($installed.Count -eq 0) {
        $previous = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        try { $output = & certutil.exe -addstore -f Root $script:PublicCert 2>&1 | Out-String } finally { $ErrorActionPreference = $previous }
        if ($LASTEXITCODE -ne 0) { throw ('certutil addstore failed: ' + $output) }
        $installed = @(Get-ChildItem Cert:\LocalMachine\Root | Where-Object { $_.Thumbprint -eq $thumbprint })
    }
    if ($installed.Count -eq 0) { throw 'root certificate was not added to the machine trust store' }
    Write-Output ('machine trust OK subject=' + $cert.Subject + ' sha1=' + $thumbprint + ' sha256=' + $cert.GetCertHashString([System.Security.Cryptography.HashAlgorithmName]::SHA256))
}

function Invoke-Untrust {
    if (Test-Path -LiteralPath $script:PublicCert) {
        $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($script:PublicCert)
        $previous = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        try { & certutil.exe -delstore Root $cert.Thumbprint 2>&1 | Out-String | Write-Output } finally { $ErrorActionPreference = $previous }
    }
    Write-Output 'CA trust entry removed from the machine store'
}

function Invoke-ExportRoot {
    if (-not (Test-Path -LiteralPath $script:RootCert)) { throw 'local CA root certificate is missing; start the proxy once first' }
    New-Item -ItemType Directory -Path (Split-Path -Parent $script:PublicCert) -Force | Out-Null
    Copy-Item -LiteralPath $script:RootCert -Destination $script:PublicCert -Force
    $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($script:PublicCert)
    Write-Output ('exported ' + $script:PublicCert + ' sha256=' + $cert.GetCertHashString([System.Security.Cryptography.HashAlgorithmName]::SHA256))
}

switch ($Action) {
    'Start' { Invoke-Start }
    'Run' { Invoke-Run }
    'Stop' { Invoke-Stop }
    'Status' { Invoke-Status }
    'Trust' { Invoke-Trust }
    'Untrust' { Invoke-Untrust }
    'ExportRoot' { Invoke-ExportRoot }
}
