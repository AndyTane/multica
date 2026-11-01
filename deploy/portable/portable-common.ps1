# Shared helpers for the U Workers portable package.
#
# Windows PowerShell 5.1 compatible on purpose: the package must run on a
# fresh Windows machine with no developer tools and no administrator rights, so
# every package script targets what Windows 10/11 already ships.
#
# The caller dot-sources this file and passes the package root explicitly, so
# the package can be copied anywhere (including paths with spaces or non-ASCII
# characters).

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:PortableManifestName = 'manifest.json'
$script:PortableDeploymentName = 'deployment.json'
$script:PortableLogStamp = Get-Date -Format 'yyyyMMdd-HHmmss'

function Write-PortableJson {
    param([string]$Path, $Value)
    $directory = Split-Path -Parent $Path
    if ($directory -and -not (Test-Path -LiteralPath $directory)) {
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
    }
    $text = $Value | ConvertTo-Json -Depth 12
    $temporary = $Path + '.tmp'
    [IO.File]::WriteAllText($temporary, $text, (New-Object Text.UTF8Encoding($false)))
    Move-Item -LiteralPath $temporary -Destination $Path -Force
}

function Read-PortableJson {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { return $null }
    return (Get-Content -LiteralPath $Path -Raw -Encoding UTF8 | ConvertFrom-Json)
}

function Get-PortableDeployment {
    param([string]$Root)
    $config = Read-PortableJson (Join-Path $Root $script:PortableDeploymentName)
    if (-not $config) { throw ('This folder is not an installed U Workers portable package yet: ' + (Join-Path $Root $script:PortableDeploymentName) + ' is missing. Run Setup.cmd first.') }
    return $config
}

function Get-PortableManifest {
    param([string]$Root)
    $manifest = Read-PortableJson (Join-Path $Root $script:PortableManifestName)
    if (-not $manifest) { throw ('The package manifest is missing: ' + (Join-Path $Root $script:PortableManifestName)) }
    return $manifest
}

function Get-PortableVersion {
    param([string]$Root)
    return [string](Get-PortableManifest $Root).version
}

function Get-PortableFile {
    param([string]$Root, [string]$RelativePath)
    $path = Join-Path $Root $RelativePath
    if (-not (Test-Path -LiteralPath $path)) { throw ('The package is missing ' + $RelativePath + '. Re-download the package; do not copy single files between versions.') }
    return $path
}

function Get-PortableServerScriptPath {
    param([string]$Root)
    return (Join-Path $Root 'runtime\native-windows.ps1')
}

function Get-PortableNodePath {
    param([string]$Root)
    return (Join-Path $Root 'runtime\node\node.exe')
}

function Get-PortablePostgresBin {
    param([string]$Root)
    return (Join-Path $Root 'runtime\pgsql\bin')
}

function Get-PortableCliPath {
    param([string]$Root)
    return (Get-PortableFile $Root 'payload\bin\u-workers.exe')
}

function Get-PortableCaddyPath {
    param([string]$Root)
    return (Get-PortableFile $Root 'payload\runtime\caddy\caddy.exe')
}

function Get-PortableOpenCodePath {
    param([string]$Root)
    return (Get-PortableFile $Root 'payload\runtime\opencode\opencode.exe')
}

function Get-PortableCaddyfilePath {
    param([string]$Root)
    return (Join-Path $Root 'https\Caddyfile')
}

function Get-PortableCaddyDataPath {
    param([string]$Root)
    return (Join-Path $Root 'https\data')
}

function Get-PortableRootCertificatePath {
    param([string]$Root)
    return (Join-Path $Root 'https\data\pki\authorities\local\root.crt')
}

function Get-PortablePublicRootCAPath {
    param([string]$Root)
    return (Join-Path $Root 'root-ca.crt')
}

function Get-PortableLogPath {
    param([string]$Root, [string]$Name)
    return (Join-Path $Root ('logs\' + $Name + '-' + $script:PortableLogStamp + '.log'))
}

function New-PortableDirectory {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { New-Item -ItemType Directory -Path $Path -Force | Out-Null }
}

function Test-PortablePortFree {
    param([int]$Port)
    if ($Port -lt 1 -or $Port -gt 65535) { return $false }
    $listener = $null
    try {
        $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, $Port)
        $listener.Start()
        return $true
    } catch {
        return $false
    } finally {
        if ($listener) { $listener.Stop() }
    }
}

function Test-PortablePortListening {
    param([int]$Port, [string]$HostName = '127.0.0.1')
    $client = $null
    try {
        $client = New-Object Net.Sockets.TcpClient
        $async = $client.BeginConnect($HostName, $Port, $null, $null)
        if (-not $async.AsyncWaitHandle.WaitOne(1000)) { return $false }
        $client.EndConnect($async)
        return $true
    } catch {
        return $false
    } finally {
        if ($client) { $client.Close() }
    }
}

# Returns the first usable IPv4 address of this machine (LAN address), never a
# loopback, APIPA (169.254.*) or virtual-adapter placeholder.
function Get-PortablePublicHost {
    param([string]$Override)
    if ($Override) { return $Override }
    $candidates = New-Object System.Collections.Generic.List[string]
    $addresses = @()
    try {
        $addresses = @(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction Stop)
    } catch {
        $addresses = @()
    }
    foreach ($address in $addresses) {
        $ip = [string]$address.IPAddress
        if ([string]$address.InterfaceAlias -match 'Loopback|vEthernet|VirtualBox|VMware|Tailscale|ZeroTier|WSL') { continue }
        if ($address.PrefixOrigin -eq 'WellKnown') { continue }
        $candidates.Add($ip)
    }
    if ($candidates.Count -eq 0) {
        try {
            $hostAddresses = [Net.Dns]::GetHostAddresses([Net.Dns]::GetHostName())
            foreach ($address in $hostAddresses) {
                if ($address.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork) { continue }
                $candidates.Add([string]$address)
            }
        } catch {
        }
    }
    foreach ($candidate in $candidates) {
        $parsed = $null
        if (-not [Net.IPAddress]::TryParse($candidate, [ref]$parsed)) { continue }
        if ([Net.IPAddress]::IsLoopback($parsed)) { continue }
        $bytes = $parsed.GetAddressBytes()
        if ($bytes[0] -eq 169 -and $bytes[1] -eq 254) { continue }
        return $candidate
    }
    return '127.0.0.1'
}

function Get-PortableCaddyfile {
    param(
        [string]$Root,
        [string]$PublicHost,
        [int]$HttpsPort,
        [int]$ApiPort,
        [int]$WebPort
    )
    # Caddy accepts forward slashes on Windows and this keeps the file free of
    # escape sequences; the storage path keeps the private CA inside the
    # package so moving the folder moves the trust anchor with it.
    $storage = ((Join-Path $Root 'https\data') -replace '\\', '/')
    $hosts = @('localhost', '127.0.0.1')
    if ($PublicHost -and $PublicHost -ne '127.0.0.1' -and $PublicHost -ne 'localhost') { $hosts += $PublicHost }
    $sites = ($hosts | ForEach-Object { 'https://' + $_ + ':' + $HttpsPort }) -join ', '
    $newLine = [Environment]::NewLine
    # Parenthesise every concatenation inside the array literal: PowerShell
    # otherwise folds the following lines into the same element.
    $lines = @(
        '{',
        '    admin off',
        '    auto_https disable_redirects',
        ('    storage file_system "' + $storage + '"'),
        '}',
        '',
        ($sites + ' {'),
        '    tls internal',
        '    encode gzip',
        '    @api path /api/* /auth/* /ws /uploads/* /v1/* /health* /readyz /metrics',
        ('    reverse_proxy @api 127.0.0.1:' + $ApiPort),
        ('    reverse_proxy 127.0.0.1:' + $WebPort),
        '}',
        ''
    )
    return ($lines -join $newLine)
}

function Get-PortableProcessRecord {
    param([int]$ProcessId, [string]$ExpectedPath)
    $process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
    if (-not $process) { return $null }
    $path = $null
    try { $path = $process.Path } catch { return $null }
    if (-not $path) { return $null }
    if ($ExpectedPath) { $path = [IO.Path]::GetFullPath($ExpectedPath) }
    return [pscustomobject]@{ Pid = $process.Id; StartedAt = $process.StartTime.ToUniversalTime().ToString('o'); Executable = $path }
}

function Test-PortableProcessRecord {
    param($Record)
    if (-not $Record) { return $false }
    $process = Get-Process -Id $Record.Pid -ErrorAction SilentlyContinue
    if (-not $process) { return $false }
    try {
        $path = $process.Path
        return ($path -eq $Record.Executable -and
            [math]::Abs(($process.StartTime.ToUniversalTime() - ([datetime]$Record.StartedAt).ToUniversalTime()).TotalMilliseconds) -lt 100)
    } catch {
        return $false
    }
}

function Stop-PortableProcessRecord {
    param($Record)
    if (-not $Record) { return }
    if (Test-PortableProcessRecord $Record) {
        $process = Get-Process -Id $Record.Pid -ErrorAction SilentlyContinue
        Stop-Process -Id $Record.Pid -ErrorAction SilentlyContinue
        if ($process) { $null = $process.WaitForExit(5000) }
    }
}

function ConvertTo-PortableArgumentLine {
    param([string[]]$Arguments)
    $quoted = foreach ($argument in $Arguments) {
        $value = [string]$argument
        if ($value -match '[\r\n]') { throw 'Refusing to launch a process with a newline in an argument.' }
        if ($value -match '^[^\s"]+$') { $value } else { '"' + ($value -replace '"', '""') + '"' }
    }
    return ($quoted -join ' ')
}

function Start-PortableHiddenProcess {
    param(
        [string]$Executable,
        [string[]]$Arguments,
        [string]$WorkingDirectory,
        [hashtable]$Environment,
        [string]$LogPath,
        [string]$ErrorLogPath
    )
    if (-not (Test-Path -LiteralPath $Executable)) { throw ('Missing executable: ' + $Executable) }
    $saved = @{}
    try {
        if ($Environment) {
            foreach ($key in $Environment.Keys) {
                $saved[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
                [Environment]::SetEnvironmentVariable($key, [string]$Environment[$key], 'Process')
            }
        }
        $options = @{
            FilePath = $Executable
            WorkingDirectory = $WorkingDirectory
            WindowStyle = 'Hidden'
            PassThru = $true
            RedirectStandardOutput = $LogPath
        }
        if ($ErrorLogPath) { $options.RedirectStandardError = $ErrorLogPath }
        if ($Arguments -and $Arguments.Count -gt 0) { $options.ArgumentList = (ConvertTo-PortableArgumentLine $Arguments) }
        $process = Start-Process @options
        return (Get-PortableProcessRecord -ProcessId $process.Id -ExpectedPath $Executable)
    } finally {
        foreach ($key in $saved.Keys) {
            $original = $saved[$key]
            if ($null -eq $original) { $original = [NullString]::Value }
            [Environment]::SetEnvironmentVariable($key, $original, 'Process')
        }
    }
}

function Invoke-PortableCommand {
    # Capture output through files, not a PowerShell pipeline. A command that
    # starts a long-lived child (the native supervisor) keeps inherited pipe
    # handles open, and a pipeline would then block forever after the launcher
    # itself exited. Waiting only for the started process avoids that.
    param([string]$Executable, [string[]]$Arguments, [hashtable]$Environment, [string]$WorkingDirectory)
    $saved = @{}
    $temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-command-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $temporaryDirectory -Force | Out-Null
    $stdoutPath = Join-Path $temporaryDirectory 'stdout.log'
    $stderrPath = Join-Path $temporaryDirectory 'stderr.log'
    try {
        if ($Environment) {
            foreach ($key in $Environment.Keys) {
                $saved[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
                [Environment]::SetEnvironmentVariable($key, [string]$Environment[$key], 'Process')
            }
        }
        $options = @{
            FilePath = $Executable
            RedirectStandardOutput = $stdoutPath
            RedirectStandardError = $stderrPath
            NoNewWindow = $true
            PassThru = $true
        }
        if ($Arguments -and $Arguments.Count -gt 0) { $options.ArgumentList = (ConvertTo-PortableArgumentLine $Arguments) }
        if ($WorkingDirectory) { $options.WorkingDirectory = $WorkingDirectory }
        $process = Start-Process @options
        $process.WaitForExit()
        $output = ''
        foreach ($path in @($stdoutPath, $stderrPath)) {
            if (Test-Path -LiteralPath $path) {
                $output += (Get-Content -LiteralPath $path -Raw -ErrorAction SilentlyContinue)
            }
        }
        return [pscustomobject]@{ ExitCode = $process.ExitCode; Output = [string]$output }
    } finally {
        foreach ($key in $saved.Keys) {
            $original = $saved[$key]
            if ($null -eq $original) { $original = [NullString]::Value }
            [Environment]::SetEnvironmentVariable($key, $original, 'Process')
        }
        Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Invoke-PortableSql {
    param($Root, [string]$Sql, [string]$Database = 'postgres', [string]$FieldSeparator = '|')
    $config = Get-PortableDeployment $Root
    $psql = Join-Path (Get-PortablePostgresBin $Root) 'psql.exe'
    if (-not (Test-Path -LiteralPath $psql)) { throw ('PostgreSQL tools are missing under ' + (Get-PortablePostgresBin $Root)) }
    $env:PGPASSWORD = [string]$config.DatabasePassword
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $psql -h 127.0.0.1 -p ([int]$config.DatabasePort) -U ([string]$config.DatabaseUser) -d $Database -X -A -t -v ON_ERROR_STOP=1 -F $FieldSeparator -c $Sql 2>&1 | Out-String
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
        Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    }
    if ($exitCode -ne 0) { throw ('PostgreSQL command failed: ' + $output.Trim()) }
    return @($output -split "\r?\n" | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ -ne '' })
}

function New-PortablePassword {
    param([int]$Length = 24)
    $alphabet = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'
    $bytes = New-Object byte[] $Length
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    $characters = New-Object char[] $Length
    for ($index = 0; $index -lt $Length; $index++) {
        $characters[$index] = $alphabet[$bytes[$index] % $alphabet.Length]
    }
    return (-join $characters)
}

# HTTPS requests from the package scripts go through curl.exe. It ships with
# Windows 10 1803+ and validates a private CA with --cacert without touching
# the user's certificate store, and it behaves the same from Windows PowerShell
# 5.1 and PowerShell 7 (unlike .NET TLS callbacks, which need a runspace on the
# networking thread only the older runtime has).
function Invoke-PortableWebRequest {
    param(
        [string]$Uri,
        [string]$Method = 'GET',
        [string]$Body,
        [hashtable]$Headers,
        [string]$RootCAPath,
        [int]$TimeoutSec = 30
    )
    $curl = Join-Path $env:SystemRoot 'System32\curl.exe'
    if (-not (Test-Path -LiteralPath $curl)) { $curl = 'curl.exe' }
    $workingDirectory = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-curl-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $workingDirectory -Force | Out-Null
    $bodyFile = Join-Path $workingDirectory 'request-body.txt'
    $responseFile = Join-Path $workingDirectory 'response-body.txt'
    $stderrFile = Join-Path $workingDirectory 'curl-error.txt'
    try {
        $arguments = @('--silent', '--show-error', '--max-time', [string]$TimeoutSec, '--output', $responseFile, '--write-out', '%{http_code}')
        if ($RootCAPath) { $arguments += @('--cacert', $RootCAPath, '--ssl-no-revoke') }
        if ($Method -and $Method -ne 'GET') { $arguments += @('--request', $Method) }
        if ($Body) {
            [IO.File]::WriteAllText($bodyFile, $Body, (New-Object Text.UTF8Encoding($false)))
            $arguments += @('--data-binary', ('@' + $bodyFile))
            $arguments += @('--header', 'Content-Type: application/json; charset=utf-8')
        }
        if ($Headers) {
            foreach ($key in $Headers.Keys) { $arguments += @('--header', ([string]$key + ': ' + [string]$Headers[$key])) }
        }
        $arguments += $Uri
        $statusText = & $curl @arguments 2> $stderrFile
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) {
            $details = ''
            if (Test-Path -LiteralPath $stderrFile) { $details = [string](Get-Content -LiteralPath $stderrFile -Raw -ErrorAction SilentlyContinue) }
            throw ('HTTPS request failed (' + $exitCode + '): ' + $details.Trim())
        }
        $statusCode = 0
        if (-not [int]::TryParse(([string]$statusText).Trim(), [ref]$statusCode)) { throw ('HTTPS request returned no status code: ' + $Uri) }
        $content = ''
        if (Test-Path -LiteralPath $responseFile) { $content = [string](Get-Content -LiteralPath $responseFile -Raw -ErrorAction SilentlyContinue) }
        return [pscustomobject]@{ StatusCode = $statusCode; Content = $content; Headers = @{} }
    } finally {
        Remove-Item -LiteralPath $workingDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function ConvertFrom-PortableResponse {
    param($Response)
    if (-not $Response -or -not $Response.Content) { return $null }
    return ($Response.Content | ConvertFrom-Json)
}

function Wait-PortableCondition {
    param([scriptblock]$Condition, [int]$TimeoutSec, [string]$Description, [int]$IntervalMs = 500)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            if (& $Condition) { return $true }
        } catch {
        }
        Start-Sleep -Milliseconds $IntervalMs
    }
    throw ('Timed out waiting for ' + $Description)
}

function Wait-PortableHttp {
    param([string]$Uri, [string]$RootCAPath, [int]$TimeoutSec = 90, [string]$Description)
    $ok = Wait-PortableCondition -TimeoutSec $TimeoutSec -Description ($Description + ' (' + $Uri + ')') -Condition {
        try {
            $response = Invoke-PortableWebRequest -Uri $Uri -RootCAPath $RootCAPath -TimeoutSec 5
            return ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500)
        } catch {
            return $false
        }
    }
    return $ok
}

function Invoke-PortableTlsHandshake {
    # Caddy issues the leaf certificate while serving the first TLS handshake
    # for a site; a request with the default (untrusted) validation is expected
    # to fail, but it makes Caddy create the certificate and its CA on disk.
    param([string]$Uri)
    try { Invoke-PortableWebRequest -Uri $Uri -TimeoutSec 10 | Out-Null } catch { }
}

function Export-PortableRootCA {
    param([string]$Root, [int]$TimeoutSec = 60)
    $source = Get-PortableRootCertificatePath $Root
    Wait-PortableCondition -TimeoutSec $TimeoutSec -Description 'the generated root CA certificate' -Condition { Test-Path -LiteralPath $source } | Out-Null
    $destination = Get-PortablePublicRootCAPath $Root
    Copy-Item -LiteralPath $source -Destination $destination -Force
    $publicDirectory = Join-Path $Root 'https\public'
    New-PortableDirectory $publicDirectory
    Copy-Item -LiteralPath $source -Destination (Join-Path $publicDirectory 'root-ca.crt') -Force
    $certificate = New-Object Security.Cryptography.X509Certificates.X509Certificate2($source)
    return [pscustomobject]@{
        Path = $destination
        Sha256 = ($certificate.GetCertHashString([Security.Cryptography.HashAlgorithmName]::SHA256))
        Subject = $certificate.Subject
        Thumbprint = $certificate.Thumbprint
    }
}

function Test-PortableRootCAInstalled {
    param([string]$Thumbprint)
    $store = New-Object Security.Cryptography.X509Certificates.X509Store('Root', 'CurrentUser')
    try {
        $store.Open([Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly)
        $matches = @($store.Certificates | Where-Object { $_.Thumbprint -eq $Thumbprint })
        return ($matches.Count -gt 0)
    } finally {
        $store.Close()
    }
}

function Install-PortableBrowserTrust {
    param([string]$Root, [int]$TimeoutSec = 90)
    # A root-store install raises the standard Windows security-warning dialog;
    # there is no documented way to suppress it, so this waits a bounded time
    # for the operator to click 是/Yes and reports failure without touching the
    # deployment. The caller can continue with -SkipBrowserTrust.
    $certificatePath = Get-PortablePublicRootCAPath $Root
    if (-not (Test-Path -LiteralPath $certificatePath)) { throw 'root-ca.crt is missing; run the HTTPS step first.' }
    $certificate = New-Object Security.Cryptography.X509Certificates.X509Certificate2($certificatePath)
    if (Test-PortableRootCAInstalled $certificate.Thumbprint) {
        return [pscustomobject]@{ Trusted = $true; AlreadyInstalled = $true; Thumbprint = $certificate.Thumbprint }
    }
    $process = Start-Process -FilePath 'certutil.exe' -ArgumentList @('-user', '-addstore', 'Root', ('"' + $certificatePath + '"')) -PassThru
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-PortableRootCAInstalled $certificate.Thumbprint) {
            return [pscustomobject]@{ Trusted = $true; AlreadyInstalled = $false; Thumbprint = $certificate.Thumbprint }
        }
        Start-Sleep -Milliseconds 500
    }
    if (-not $process.HasExited) { $process.Kill() }
    return [pscustomobject]@{ Trusted = $false; AlreadyInstalled = $false; Thumbprint = $certificate.Thumbprint }
}

function Remove-PortableBrowserTrust {
    param([string]$Root)
    $certificatePath = Get-PortablePublicRootCAPath $Root
    if (-not (Test-Path -LiteralPath $certificatePath)) { return $false }
    $certificate = New-Object Security.Cryptography.X509Certificates.X509Certificate2($certificatePath)
    if (-not (Test-PortableRootCAInstalled $certificate.Thumbprint)) { return $false }
    & certutil.exe -user -delstore Root $certificate.Thumbprint 2>&1 | Out-Null
    return $true
}

function Get-PortableStartupShortcutPath {
    return (Join-Path ([Environment]::GetFolderPath('Startup')) 'U Workers Portable.vbs')
}

function Install-PortableStartup {
    param([string]$Root)
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $startScript = Join-Path $Root 'start.ps1'
    if (-not (Test-Path -LiteralPath $startScript)) { throw 'start.ps1 is missing from the package.' }
    $startCommand = '"' + $shell + '" -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "' + $startScript + '"'
    $escaped = $startCommand.Replace('"', '""')
    $vbs = 'CreateObject("WScript.Shell").Run "' + $escaped + '", 0, False' + [Environment]::NewLine
    $shortcut = Get-PortableStartupShortcutPath
    [IO.File]::WriteAllText($shortcut, $vbs, (New-Object Text.UnicodeEncoding($false, $true)))
    return $shortcut
}

function Remove-PortableStartup {
    $shortcut = Get-PortableStartupShortcutPath
    if (Test-Path -LiteralPath $shortcut) {
        Remove-Item -LiteralPath $shortcut -Force
        return $true
    }
    return $false
}

function Start-PortableCaddy {
    param([string]$Root, [string]$PublicHost, [int]$HttpsPort, [int]$ApiPort, [int]$WebPort)
    $caddyExe = Get-PortableCaddyPath $Root
    $statePath = Join-Path $Root 'state\caddy.json'
    $existing = Read-PortableJson $statePath
    if ($existing -and (Test-PortableProcessRecord $existing)) { return $existing }
    Remove-Item -LiteralPath $statePath -Force -ErrorAction SilentlyContinue
    if (Test-PortablePortListening $HttpsPort) { throw ('Port ' + $HttpsPort + ' is already in use; another HTTPS listener is running.') }
    New-PortableDirectory (Join-Path $Root 'state')
    New-PortableDirectory (Join-Path $Root 'logs')
    New-PortableDirectory (Join-Path $Root 'https')
    $caddyfile = Get-PortableCaddyfilePath $Root
    [IO.File]::WriteAllText($caddyfile, (Get-PortableCaddyfile -Root $Root -PublicHost $PublicHost -HttpsPort $HttpsPort -ApiPort $ApiPort -WebPort $WebPort), (New-Object Text.UTF8Encoding($false)))
    $validation = Invoke-PortableCommand -Executable $caddyExe -Arguments @('validate', '--config', $caddyfile, '--adapter', 'caddyfile')
    if ($validation.ExitCode -ne 0) { throw ('Caddy rejected the generated HTTPS configuration: ' + $validation.Output.Trim()) }
    $record = Start-PortableHiddenProcess -Executable $caddyExe -Arguments @('run', '--config', $caddyfile, '--adapter', 'caddyfile') -WorkingDirectory $Root -LogPath (Get-PortableLogPath $Root 'caddy') -ErrorLogPath (Get-PortableLogPath $Root 'caddy-error')
    Write-PortableJson $statePath $record
    Wait-PortableCondition -TimeoutSec 60 -Description ('the HTTPS listener on port ' + $HttpsPort) -Condition { Test-PortablePortListening $HttpsPort } | Out-Null
    return $record
}

function Stop-PortableCaddy {
    param([string]$Root)
    $statePath = Join-Path $Root 'state\caddy.json'
    $record = Read-PortableJson $statePath
    if (-not $record) { return $false }
    Stop-PortableProcessRecord $record
    Remove-Item -LiteralPath $statePath -Force -ErrorAction SilentlyContinue
    return $true
}

function Invoke-PortableNativeAction {
    param([string]$Root, [string]$Action, [string[]]$ExtraArguments)
    $script = Get-PortableServerScriptPath $Root
    if (-not (Test-Path -LiteralPath $script)) { throw ('The server script is missing: ' + $script + '. Run Setup.cmd again.') }
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $script, '-Action', $Action, '-Root', $Root)
    if ($ExtraArguments) { $arguments += $ExtraArguments }
    $result = Invoke-PortableCommand -Executable $shell -Arguments $arguments
    if ($result.ExitCode -ne 0) {
        throw ('The server ' + $Action + ' step failed: ' + $result.Output.Trim())
    }
    return $result.Output
}

function Get-PortableCorsOrigins {
    param([string]$PublicHost, [int]$HttpsPort)
    return 'https://' + $PublicHost + ':' + $HttpsPort + ',https://localhost:' + $HttpsPort + ',https://127.0.0.1:' + $HttpsPort
}

function Get-PortableCliEnvironment {
    param([string]$Root)
    # The daemon discovers agent CLIs on PATH. Prepending the bundled opencode
    # directory is what makes an opencode runtime appear without installing
    # anything on the machine, and the bundled CLI directory is how the
    # installer's own commands resolve.
    $bin = Join-Path $Root 'payload\bin'
    $opencode = Join-Path $Root 'payload\runtime\opencode'
    $path = $bin + ';' + $opencode + ';' + [Environment]::GetEnvironmentVariable('PATH', 'Process')
    # A portable install may be prepared from inside another agent's task
    # (for example while the package is being tested): never let that task
    # identity leak into the installed CLI. The CLI refuses human commands such
    # as login/connect inside a daemon-managed task, and it must not inherit
    # another deployment's server URL or token either.
    $environment = @{ PATH = $path }
    foreach ($name in @(
        'MULTICA_AGENT_ID', 'U_WORKERS_AGENT_ID', 'MULTICA_TASK_ID', 'U_WORKERS_TASK_ID',
        'MULTICA_DAEMON_PORT', 'U_WORKERS_DAEMON_PORT', 'MULTICA_TASK_CONFIG_ROOT', 'U_WORKERS_TASK_CONFIG_ROOT',
        'MULTICA_TOKEN', 'U_WORKERS_TOKEN', 'MULTICA_SERVER_URL', 'U_WORKERS_SERVER_URL',
        'MULTICA_APP_URL', 'U_WORKERS_APP_URL', 'MULTICA_DAEMON_PROFILE', 'U_WORKERS_DAEMON_PROFILE'
    )) {
        $environment[$name] = ''
    }
    return $environment
}

function Invoke-PortableCli {
    param([string]$Root, [string[]]$Arguments)
    $cli = Get-PortableCliPath $Root
    $result = Invoke-PortableCommand -Executable $cli -Arguments $Arguments -Environment (Get-PortableCliEnvironment $Root) -WorkingDirectory $Root
    return $result
}

function Write-PortableInstallReport {
    param([string]$Root, $Report)
    $path = Join-Path $Root 'data\install-report.json'
    New-PortableDirectory (Split-Path -Parent $path)
    Write-PortableJson $path $Report
    return $path
}

function Get-PortableDaemonStatus {
    param([string]$Root, [string]$Profile)
    $arguments = @()
    if ($Profile) { $arguments += @('--profile', $Profile) }
    $arguments += @('daemon', 'status', '--output', 'json')
    $result = Invoke-PortableCli -Root $Root -Arguments $arguments
    if ($result.ExitCode -ne 0) {
        return [pscustomobject]@{ Running = $false; Output = ([string]$result.Output).Trim() }
    }
    $parsed = $null
    try { $parsed = (([string]$result.Output).Trim() | ConvertFrom-Json) } catch { $parsed = $null }
    return [pscustomobject]@{ Running = $true; Detail = $parsed; Output = ([string]$result.Output).Trim() }
}

function Start-PortableDaemon {
    param([string]$Root, [string]$Profile)
    $status = Get-PortableDaemonStatus -Root $Root -Profile $Profile
    if ($status.Running) { return $status }
    $arguments = @()
    if ($Profile) { $arguments += @('--profile', $Profile) }
    $arguments += @('daemon', 'start')
    $result = Invoke-PortableCli -Root $Root -Arguments $arguments
    if ($result.ExitCode -ne 0) {
        throw ('Starting the local daemon failed: ' + ([string]$result.Output).Trim())
    }
    return (Get-PortableDaemonStatus -Root $Root -Profile $Profile)
}

function Stop-PortableDaemon {
    param([string]$Root, [string]$Profile)
    $arguments = @()
    if ($Profile) { $arguments += @('--profile', $Profile) }
    $arguments += @('daemon', 'stop')
    $result = Invoke-PortableCli -Root $Root -Arguments $arguments
    return $result
}

function Get-PortableLanHostFromConfig {
    param([string]$Root)
    $report = Read-PortableJson (Join-Path $Root 'data\install-report.json')
    if ($report -and $report.lanHost) { return [string]$report.lanHost }
    $config = Get-PortableDeployment $Root
    if ($config.PublicOrigin) {
        try { return ([Uri][string]$config.PublicOrigin).Host } catch { }
    }
    return '127.0.0.1'
}

function Get-PortableHttpsPortFromConfig {
    param([string]$Root)
    $report = Read-PortableJson (Join-Path $Root 'data\install-report.json')
    if ($report -and $report.httpsPort) { return [int]$report.httpsPort }
    $config = Get-PortableDeployment $Root
    if ($config.PublicOrigin) {
        try { return ([Uri][string]$config.PublicOrigin).Port } catch { }
    }
    return 13443
}

function Get-PortableProfileConfigRoots {
    # The CLI keeps the default profile in ~/.multica when that legacy folder
    # exists, otherwise in ~/.u-workers. Only the package's own profile
    # subdirectory is ever removed by the uninstaller.
    $homeDirectory = [Environment]::GetFolderPath('UserProfile')
    $roots = @()
    foreach ($name in @('.u-workers', '.multica')) {
        $path = Join-Path $homeDirectory $name
        if (Test-Path -LiteralPath $path) { $roots += $path }
    }
    return $roots
}

