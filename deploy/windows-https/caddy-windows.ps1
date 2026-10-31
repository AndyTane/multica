# U Workers HTTPS entry lifecycle (Caddy, Windows).
#
# The deployment's own native-windows.ps1 owns PostgreSQL/API/web. This script
# owns only the TLS reverse proxy that fronts them, and it must keep running
# independently of whichever agent or SSH session started it:
#
#   -Action Start    apply the ONSTART task definition (idempotent) and run it
#   -Action Run      the supervisor loop the scheduled task executes
#   -Action Stop     stop the proxy (verified PID + start time)
#   -Action Status   task settings, process, listeners, certificate and upstream checks
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
    $processPath = $null
    try { $processPath = $process.Path } catch { }
    $recordedPath = [string]$state.Caddy.Executable
    if ($processPath -and $recordedPath -and $processPath -ne $recordedPath) { return $null }
    if (-not $processPath -and $recordedPath -and $recordedPath -ne $script:Exe) { return $null }
    try {
        $recorded = [datetime]::Parse([string]$state.Caddy.StartedAt, $null, [Globalization.DateTimeStyles]::RoundtripKind)
        $delta = ($process.StartTime.ToUniversalTime() - $recorded.ToUniversalTime()).TotalSeconds
        if ([math]::Abs($delta) -gt 10) { return $null }
    } catch { }
    return $process
}

function Get-ProcessRecord {
    param($Process, [string]$FallbackExecutable)
    $executable = $null
    $startedAt = $null
    try { $executable = $Process.Path } catch { }
    if (-not $executable) { $executable = $FallbackExecutable }
    try { $startedAt = $Process.StartTime.ToUniversalTime().ToString('o') } catch { $startedAt = (Get-Date).ToUniversalTime().ToString('o') }
    return [ordered]@{
        Pid = $Process.Id
        Executable = $executable
        StartedAt = $startedAt
    }
}

function Get-ManagedCaddyProcesses {
    # The proxy is managed by exact executable path (never by process name), so
    # an orphaned caddy is always attributable to this deployment's https dir.
    $processes = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq $script:Exe })
    $result = @()
    foreach ($process in $processes) {
        $result += Get-Process -Id $process.ProcessId -ErrorAction SilentlyContinue
    }
    return @($result | Where-Object { $_ -ne $null })
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

function New-HttpsTaskDefinition {
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $arguments = '-NoProfile -ExecutionPolicy Bypass -File "' + $PSCommandPath + '" -Action Run -Root "' + $Root + '"'
    $action = New-ScheduledTaskAction -Execute $shell -Argument $arguments
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
    # No execution time limit (0 = unlimited works around the default PT72H that
    # silently kills a daily-use supervisor after three days), restart on
    # failure, and no battery conditions so a laptop host cannot pause the proxy.
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1)
    return [pscustomobject]@{ Action = $action; Trigger = $trigger; Principal = $principal; Settings = $settings }
}

function Get-TaskSettingsSnapshot {
    $task = Get-ScheduledTask -TaskName $script:TaskName -ErrorAction SilentlyContinue
    if (-not $task) { return $null }
    $settings = $task.Settings
    return [ordered]@{
        name = $script:TaskName
        state = [string]$task.State
        execution_time_limit = [string]$settings.ExecutionTimeLimit
        restart_count = [int]$settings.RestartCount
        restart_interval = [string]$settings.RestartInterval
        disallow_start_if_on_batteries = [bool]$settings.DisallowStartIfOnBatteries
        stop_if_going_on_batteries = [bool]$settings.StopIfGoingOnBatteries
        multiple_instances = [string]$settings.MultipleInstances
        principal = [string]$task.Principal.UserId
        logon_type = [string]$task.Principal.LogonType
        run_level = [string]$task.Principal.RunLevel
        triggers = @($task.Triggers | ForEach-Object { $_.CimClass.CimClassName })
    }
}

function Ensure-ScheduledTask {
    $definition = New-HttpsTaskDefinition
    $existing = Get-ScheduledTask -TaskName $script:TaskName -ErrorAction SilentlyContinue
    if ($existing) {
        # Applies the settings to the existing task even while it is running;
        # Set-ScheduledTask leaves the running instance alone, so Start never
        # restarts a healthy supervisor just to update its definition.
        $null = Set-ScheduledTask -TaskName $script:TaskName -Action $definition.Action -Trigger $definition.Trigger -Principal $definition.Principal -Settings $definition.Settings
        Write-LifecycleLog 'scheduled task settings applied to existing task'
    } else {
        $null = Register-ScheduledTask -TaskName $script:TaskName -Action $definition.Action -Trigger $definition.Trigger -Principal $definition.Principal -Settings $definition.Settings -Force
        Write-LifecycleLog ('registered scheduled task ' + $script:TaskName)
    }
    $snapshot = Get-TaskSettingsSnapshot
    if ([string]$snapshot.execution_time_limit -eq 'PT72H') { throw 'scheduled task still carries the default 72-hour execution limit' }
    if ($snapshot.disallow_start_if_on_batteries -or $snapshot.stop_if_going_on_batteries) { throw 'scheduled task still stops on battery conditions' }
    if ([int]$snapshot.restart_count -lt 1) { throw 'scheduled task has no restart-on-failure setting' }
    return $snapshot
}

function Invoke-Run {
    New-Item -ItemType Directory -Path (Split-Path -Parent $script:StateFile) -Force | Out-Null
    if (Test-Path -LiteralPath $script:StopFile) { Remove-Item -LiteralPath $script:StopFile -Force }
    Write-LifecycleLog 'supervisor starting'
    try {
        while (-not (Test-Path -LiteralPath $script:StopFile)) {
            $caddy = Start-Process -FilePath $script:Exe -ArgumentList @('run', '--config', $script:Caddyfile, '--adapter', 'caddyfile') -PassThru -WindowStyle Hidden
            $supervisor = Get-Process -Id $PID -ErrorAction SilentlyContinue
            $record = [ordered]@{
                Supervisor = if ($supervisor) { Get-ProcessRecord $supervisor $PSCommandPath } else { $null }
                Caddy = Get-ProcessRecord $caddy $script:Exe
                UpdatedAt = (Get-Date).ToUniversalTime().ToString('o')
            }
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

function Get-TaskState {
    $task = Get-ScheduledTask -TaskName $script:TaskName -ErrorAction SilentlyContinue
    if (-not $task) { return '' }
    return [string]$task.State
}

function Complete-Stop {
    # Ends the task and clears any proxy process whose exact executable path is
    # this deployment's caddy.exe. The supervisor is normally stopped through
    # the stop file; the pid check below only handles an orphan left behind by
    # an abrupt task termination, never a process matched by name.
    [void](Invoke-Schtasks @('/End', '/TN', $script:TaskName))
    $deadline = (Get-Date).AddSeconds(15)
    while ((Get-Date) -lt $deadline) {
        if (@(Get-ManagedCaddyProcesses).Count -eq 0) { break }
        Start-Sleep -Milliseconds 500
    }
    $remaining = @(Get-ManagedCaddyProcesses)
    foreach ($process in $remaining) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        Write-Output ('stopped orphan caddy pid ' + $process.Id)
    }
    $deadline = (Get-Date).AddSeconds(15)
    while ((Get-Date) -lt $deadline) {
        if (@(Get-ManagedCaddyProcesses).Count -eq 0) { break }
        Start-Sleep -Milliseconds 500
    }
    Remove-Item -LiteralPath $script:StateFile -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $script:StopFile -Force -ErrorAction SilentlyContinue
    Write-Output ('HTTPS entry stopped; caddy_procs=' + @(Get-ManagedCaddyProcesses).Count + ' task_state=' + (Get-TaskState))
}

function Invoke-Stop {
    New-Item -ItemType Directory -Path (Split-Path -Parent $script:StopFile) -Force | Out-Null
    Set-Content -LiteralPath $script:StopFile -Value 'stop' -Encoding ascii
    # Give the supervisor the chance to stop its own child first (2s poll loop).
    $deadline = (Get-Date).AddSeconds(20)
    while ((Get-Date) -lt $deadline) {
        if (@(Get-ManagedCaddyProcesses).Count -eq 0) { break }
        Start-Sleep -Milliseconds 500
    }
    Complete-Stop
}

function Invoke-Start {
    $snapshot = Ensure-ScheduledTask
    $taskState = Get-TaskState
    $managed = @(Get-ManagedCaddyProcesses)
    $healthy = ($taskState -eq 'Running') -and ($managed.Count -ge 1)
    if (-not $healthy) {
        if ($managed.Count -ge 1 -or $taskState -eq 'Running') {
            Write-LifecycleLog ('resetting proxy before start: task_state=' + $taskState + ' caddy_procs=' + $managed.Count)
            New-Item -ItemType Directory -Path (Split-Path -Parent $script:StopFile) -Force | Out-Null
            Set-Content -LiteralPath $script:StopFile -Value 'stop' -Encoding ascii
            Complete-Stop
        }
        Start-ScheduledTask -TaskName $script:TaskName
        Write-LifecycleLog 'scheduled task started'
    } else {
        Write-LifecycleLog 'proxy already running; task settings updated in place'
    }
    if (-not (Wait-Listening $script:ListenPort 60)) {
        throw 'HTTPS listener did not come up within 60 seconds; inspect https\logs\access.log and https\logs\supervisor.log'
    }
    $finalState = Get-TaskState
    $finalManaged = @(Get-ManagedCaddyProcesses)
    Write-Output ('HTTPS entry listening on https://' + $script:ListenAddress + ':' + $script:ListenPort)
    Write-Output ('task_state=' + $finalState + ' caddy_procs=' + $finalManaged.Count)
    Write-Output ('task_settings=' + ($snapshot | ConvertTo-Json -Compress))
    if ($finalState -ne 'Running') { throw ('HTTPS scheduled task is not running after start: ' + $finalState) }
    if ($finalManaged.Count -ne 1) { throw ('exactly one caddy process expected after start, found ' + $finalManaged.Count) }
}

function Get-OwnedSupervisor {
    # The supervisor is our own '-Action Run' process. Verify it by PID, script
    # path in the command line and recorded start time, never by process name.
    if (-not (Test-Path -LiteralPath $script:StateFile)) { return $null }
    $state = Get-Content -LiteralPath $script:StateFile -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $state.Supervisor) { return $null }
    $process = Get-Process -Id ([int]$state.Supervisor.Pid) -ErrorAction SilentlyContinue
    if (-not $process) { return $null }
    $details = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $process.Id) -ErrorAction SilentlyContinue
    if (-not $details) { return $null }
    $commandLine = [string]$details.CommandLine
    if ($commandLine -notlike '*-Action Run*') { return $null }
    if ($commandLine -notlike ('*' + $PSCommandPath + '*')) { return $null }
    try {
        $recorded = [datetime]::Parse([string]$state.Supervisor.StartedAt, $null, [Globalization.DateTimeStyles]::RoundtripKind)
        $delta = ($process.StartTime.ToUniversalTime() - $recorded.ToUniversalTime()).TotalSeconds
        if ([math]::Abs($delta) -gt 10) { return $null }
    } catch { }
    return $process
}

function Invoke-Status {
    $taskSnapshot = Get-TaskSettingsSnapshot
    $taskOk = ($null -ne $taskSnapshot)
    $supervisor = Get-OwnedSupervisor
    $managed = @(Get-ManagedCaddyProcesses)
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
        task_settings = $taskSnapshot
        caddy_running = ($managed.Count -ge 1)
        caddy_pid = if ($managed.Count -ge 1) { $managed[0].Id } else { $null }
        caddy_processes = $managed.Count
        supervisor_pid = if ($supervisor) { $supervisor.Id } else { $null }
        supervisor_verified = [bool]$supervisor
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
