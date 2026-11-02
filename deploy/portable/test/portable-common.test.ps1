# Unit tests for the portable package scripts. Windows PowerShell 5.1 compatible.
# Run: powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\portable-common.test.ps1
$ErrorActionPreference = 'Stop'

$portableRoot = Split-Path -Parent $PSScriptRoot

function Assert-PortableTest {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw ('FAILED: ' + $Message) }
}

# Every package script must parse before anything is executed on a fresh machine.
$scriptFiles = @()
$scriptFiles += @(Get-ChildItem -LiteralPath $portableRoot -Filter *.ps1)
$scriptFiles += @(Get-ChildItem -LiteralPath (Join-Path $portableRoot 'package') -Filter *.ps1)
foreach ($file in $scriptFiles) {
    $parseErrors = $null
    [Management.Automation.Language.Parser]::ParseFile($file.FullName, [ref]$null, [ref]$parseErrors) | Out-Null
    if ($parseErrors) { throw ('Parse error in ' + $file.Name + ': ' + (($parseErrors | ForEach-Object { $_.Message }) -join '; ')) }
}

$common = Join-Path $portableRoot 'portable-common.ps1'
. $common

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-portable-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
try {
    # --- JSON helpers -----------------------------------------------------
    $jsonPath = Join-Path $testRoot 'state\value.json'
    $unicode = [string][char]0x4E2D + [char]0x6587
    Write-PortableJson -Path $jsonPath -Value @{ value = $unicode; number = 7 }
    $loaded = Read-PortableJson -Path $jsonPath
    Assert-PortableTest ($loaded.value -eq $unicode) 'Write-PortableJson must keep UTF-8 text readable through Read-PortableJson.'
    Assert-PortableTest ($loaded.number -eq 7) 'Write-PortableJson must round-trip numbers.'
    Assert-PortableTest ($null -eq (Read-PortableJson -Path (Join-Path $testRoot 'missing.json'))) 'A missing JSON file must read as null.'
    $missingThrew = $false
    try { Get-PortableDeployment -Root $testRoot | Out-Null } catch { $missingThrew = $true }
    Assert-PortableTest $missingThrew 'Get-PortableDeployment must refuse a folder that was never installed.'

    # --- manifest / version ----------------------------------------------
    Write-PortableJson -Path (Join-Path $testRoot 'manifest.json') -Value @{ name = 'UWorkers-Portable'; version = 'v1.2.3'; commit = 'abc1234' }
    Assert-PortableTest ((Get-PortableVersion -Root $testRoot) -eq 'v1.2.3') 'Get-PortableVersion must read manifest.json.'
    $missingFileThrew = $false
    try { Get-PortableFile -Root $testRoot -RelativePath 'payload\none.bin' | Out-Null } catch { $missingFileThrew = $true }
    Assert-PortableTest $missingFileThrew 'Get-PortableFile must name a missing package file instead of returning a bad path.'

    # --- Caddyfile generation --------------------------------------------
    $caddyfile = Get-PortableCaddyfile -Root 'D:\Apps\U Workers Portable' -PublicHost '192.168.3.162' -HttpsPort 13443 -ApiPort 18080 -WebPort 13000
    Assert-PortableTest ($caddyfile -match 'admin off') 'The Caddyfile must disable the admin API.'
    Assert-PortableTest ($caddyfile -match 'storage file_system "D:/Apps/U Workers Portable/https/data"') 'The private CA storage must stay inside the package and use forward slashes.'
    Assert-PortableTest ($caddyfile -match 'https://192\.168\.3\.162:13443') 'The LAN address must be in the certificate site addresses.'
    Assert-PortableTest ($caddyfile -match 'https://localhost:13443') 'localhost must be in the certificate site addresses.'
    Assert-PortableTest ($caddyfile -match 'tls internal') 'The HTTPS entry must use the package-local CA.'
    Assert-PortableTest ($caddyfile -match 'reverse_proxy @api 127\.0\.0\.1:18080') 'API traffic must be proxied to the loopback API port.'
    Assert-PortableTest ($caddyfile -match 'reverse_proxy 127\.0\.0\.1:13000') 'Everything else must be proxied to the loopback web port.'
    $loopbackOnly = Get-PortableCaddyfile -Root $testRoot -PublicHost '127.0.0.1' -HttpsPort 13443 -ApiPort 18080 -WebPort 13000
    Assert-PortableTest (([regex]::Matches($loopbackOnly, 'https://127\.0\.0\.1:13443')).Count -eq 1) 'A loopback public host must not duplicate a site address.'

    # --- ports ------------------------------------------------------------
    $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $probePort = ([Net.IPEndPoint]$listener.LocalEndpoint).Port
    Assert-PortableTest (-not (Test-PortablePortFree $probePort)) 'A bound port must not be reported free.'
    Assert-PortableTest (Test-PortablePortListening $probePort) 'A bound loopback port must be reported listening.'
    $listener.Stop()
    Assert-PortableTest (Test-PortablePortFree $probePort) 'A released port must be reported free again.'

    # --- process records --------------------------------------------------
    $expectedPath = [IO.Path]::GetFullPath((Get-Process -Id $PID).Path)
    $record = Get-PortableProcessRecord -ProcessId $PID -ExpectedPath $expectedPath
    Assert-PortableTest (Test-PortableProcessRecord $record) 'A process record must verify by pid, executable and start time.'
    $stale = [pscustomobject]@{ Pid = $PID; StartedAt = '2000-01-01T00:00:00.0000000Z'; Executable = $expectedPath }
    Assert-PortableTest (-not (Test-PortableProcessRecord $stale)) 'A reused pid with a different start time must be rejected.'

    # --- argument quoting -------------------------------------------------
    Assert-PortableTest ((ConvertTo-PortableArgumentLine @('validate', '--config', 'C:\a b\Caddyfile')) -eq 'validate --config "C:\a b\Caddyfile"') 'Arguments with spaces must be quoted.'
    Assert-PortableTest ((ConvertTo-PortableArgumentLine @('a"b')) -eq '"a""b"') 'Embedded quotes must be doubled for Start-Process.'
    $newlineRejected = $false
    try { ConvertTo-PortableArgumentLine @('bad' + [char]10 + 'value') | Out-Null } catch { $newlineRejected = $true }
    Assert-PortableTest $newlineRejected 'Arguments containing newlines must be refused.'

    # --- addresses / origins ---------------------------------------------
    Assert-PortableTest ((Get-PortablePublicHost -Override '10.1.2.3') -eq '10.1.2.3') 'An explicit public host must win.'
    $detected = Get-PortablePublicHost
    $parsed = $null
    Assert-PortableTest ([Net.IPAddress]::TryParse($detected, [ref]$parsed)) 'Auto-detected public host must be an IP address.'
    Assert-PortableTest (-not [Net.IPAddress]::IsLoopback($parsed)) 'Auto-detected public host must not be loopback when a LAN address exists.'
    $origins = Get-PortableCorsOrigins -PublicHost '192.168.3.162' -HttpsPort 13443
    Assert-PortableTest ($origins -eq 'https://192.168.3.162:13443,https://localhost:13443,https://127.0.0.1:13443') 'CORS origins must list the exact https origins.'

    # --- report / config helpers -----------------------------------------
    New-Item -ItemType Directory -Path (Join-Path $testRoot 'data') -Force | Out-Null
    Write-PortableJson -Path (Join-Path $testRoot 'data\install-report.json') -Value @{ lanHost = '192.168.3.99'; httpsPort = 14443; profile = 'portable-test' }
    Assert-PortableTest ((Get-PortableLanHostFromConfig -Root $testRoot) -eq '192.168.3.99') 'The LAN host must come from the install report.'
    Assert-PortableTest ((Get-PortableHttpsPortFromConfig -Root $testRoot) -eq 14443) 'The HTTPS port must come from the install report.'
    Assert-PortableTest (($null -ne (Get-PortableProfileConfigRoots))) 'Profile config roots must always be a list.'

    # --- daemon status and start ------------------------------------------
    # `daemon status --output json` exits 0 for a stopped daemon too, so the
    # verdict has to come from the reported status field. Trusting the exit code
    # is what made Start.cmd print "Local agent daemon: running" while no daemon
    # was running, and skipped the start that would have brought it up.
    $script:cliCalls = New-Object System.Collections.Generic.List[string]
    $script:cliStatusExit = 0
    $script:cliStatusOutput = '{"status":"stopped"}'
    function Invoke-PortableCli {
        param([string]$Root, [string[]]$Arguments)
        $script:cliCalls.Add(($Arguments -join ' '))
        if (($Arguments -join ' ') -match 'daemon start') { $script:cliStatusOutput = '{"status":"running","pid":4242}' }
        return [pscustomobject]@{ ExitCode = $script:cliStatusExit; Output = $script:cliStatusOutput }
    }
    $stopped = Get-PortableDaemonStatus -Root $testRoot -Profile 'acceptance'
    Assert-PortableTest (-not $stopped.Running) 'A status document that says stopped must not be reported as running.'
    $started = Start-PortableDaemon -Root $testRoot -Profile 'acceptance'
    Assert-PortableTest ([bool]$started.Running) 'Starting a stopped daemon must be verified by the next status read.'
    Assert-PortableTest (@($script:cliCalls | Where-Object { $_ -match 'daemon start' }).Count -eq 1) 'A daemon reported stopped must actually be started.'
    $script:cliStatusOutput = '{"status":"starting","pid":4242}'
    Assert-PortableTest ([bool](Get-PortableDaemonStatus -Root $testRoot -Profile 'acceptance').Running) 'A starting daemon must not be started a second time.'
    $script:cliStatusExit = 1
    $script:cliStatusOutput = 'unknown profile'
    Assert-PortableTest (-not (Get-PortableDaemonStatus -Root $testRoot -Profile 'acceptance').Running) 'A failed status command must not be reported as running.'
    $script:cliStatusExit = 0
    $script:cliStatusOutput = ''
    Assert-PortableTest (-not (Get-PortableDaemonStatus -Root $testRoot -Profile 'acceptance').Running) 'Unparseable status output must not be reported as running.'

    # --- password generator ----------------------------------------------
    $password = New-PortablePassword -Length 24
    Assert-PortableTest ($password.Length -eq 24) 'Generated passwords must honour the requested length.'
    Assert-PortableTest ($password -match '^[A-Za-z0-9]+$') 'Generated passwords must be URL/CLI safe.'
    Assert-PortableTest ($password -ne (New-PortablePassword -Length 24)) 'Generated passwords must differ between runs.'

    Write-Host 'portable-common tests passed.'
} finally {
    Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
