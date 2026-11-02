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
    $caddyfile = Get-PortableCaddyfile -Root 'D:\Apps\U Workers Portable' -PublicHost '192.168.3.162' -HttpsPort 13443 -ApiPort 18080 -WebPort 13000 -LanExposed $true
    Assert-PortableTest ($caddyfile -match 'admin off') 'The Caddyfile must disable the admin API.'
    Assert-PortableTest ($caddyfile -match 'storage file_system "D:/Apps/U Workers Portable/https/data"') 'The private CA storage must stay inside the package and use forward slashes.'
    Assert-PortableTest ($caddyfile -match 'https://192\.168\.3\.162:13443') 'The LAN address must be in the certificate site addresses.'
    Assert-PortableTest ($caddyfile -match 'https://localhost:13443') 'localhost must be in the certificate site addresses.'
    Assert-PortableTest ($caddyfile -match 'tls internal') 'The HTTPS entry must use the package-local CA.'
    Assert-PortableTest ($caddyfile -match 'reverse_proxy @api 127\.0\.0\.1:18080') 'API traffic must be proxied to the loopback API port.'
    Assert-PortableTest ($caddyfile -match 'reverse_proxy 127\.0\.0\.1:13000') 'Everything else must be proxied to the loopback web port.'
    # The wildcard listener is what made Windows Firewall offer "allow this
    # program" for every interface and every network profile that came up.
    Assert-PortableTest ($caddyfile -match 'bind 127\.0\.0\.1 192\.168\.3\.162') 'The LAN listener must bind the loopback address and the LAN address explicitly.'
    Assert-PortableTest (-not ($caddyfile -match 'bind 0\.0\.0\.0') -and -not ($caddyfile -match 'bind ::') -and -not ($caddyfile -match '\[::\]')) 'The Caddyfile must never fall back to a wildcard listen address.'
    # HTTP/3 opens a UDP listener on the same port; the TCP firewall rule cannot
    # cover it, so Windows Firewall asked again on every start.
    Assert-PortableTest ($caddyfile -match 'protocols h1 h2') 'HTTP/3 must be disabled so only the TCP listener exists.'
    $loopbackOnly = Get-PortableCaddyfile -Root $testRoot -PublicHost '127.0.0.1' -HttpsPort 13443 -ApiPort 18080 -WebPort 13000 -LanExposed $true
    Assert-PortableTest (([regex]::Matches($loopbackOnly, 'https://127\.0\.0\.1:13443')).Count -eq 1) 'A loopback public host must not duplicate a site address.'
    $lanOff = Get-PortableCaddyfile -Root $testRoot -PublicHost '192.168.3.162' -HttpsPort 13443 -ApiPort 18080 -WebPort 13000 -LanExposed $false
    Assert-PortableTest (-not ($lanOff -match '192\.168\.3\.162')) 'A loopback-only Caddyfile must not name the LAN address at all.'
    Assert-PortableTest ($lanOff -match 'bind 127\.0\.0\.1') 'A loopback-only Caddyfile must bind 127.0.0.1.'

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
    Assert-PortableTest ((Get-PortableCorsOrigins -PublicHost '127.0.0.1' -HttpsPort 13443) -eq 'https://127.0.0.1:13443,https://localhost:13443') 'A loopback-only installation must not list the same CORS origin twice.'
    Assert-PortableTest ((Test-PortableLoopbackHost 'localhost') -and (Test-PortableLoopbackHost '127.0.0.1') -and -not (Test-PortableLoopbackHost '192.168.3.49')) 'Only loopback names may count as this-machine-only addresses.'

    # --- report / config helpers -----------------------------------------
    New-Item -ItemType Directory -Path (Join-Path $testRoot 'data') -Force | Out-Null
    Write-PortableJson -Path (Join-Path $testRoot 'data\install-report.json') -Value @{ lanHost = '192.168.3.99'; httpsPort = 14443; profile = 'portable-test' }
    Assert-PortableTest ((Get-PortableLanHostFromConfig -Root $testRoot) -eq '192.168.3.99') 'The LAN host must come from the install report.'
    Assert-PortableTest ((Get-PortableHttpsPortFromConfig -Root $testRoot) -eq 14443) 'The HTTPS port must come from the install report.'
    Assert-PortableTest (($null -ne (Get-PortableProfileConfigRoots))) 'Profile config roots must always be a list.'

    # --- LAN exposure plan -------------------------------------------------
    # Windows Firewall offers "allow this program" when a program listens beyond
    # loopback without an inbound allow rule covering it. The plan derives the
    # listen addresses and the Caddyfile from that coverage, so an installation
    # without administrator rights never produces the prompt at all.
    $caddyPath = 'D:\Apps\U Workers Portable\payload\runtime\caddy\caddy.exe'
    Assert-PortableTest (Test-PortableLocalAddress '127.0.0.1') 'Loopback is always a local address.'
    Assert-PortableTest (-not (Test-PortableLocalAddress '203.0.113.17')) 'A TEST-NET address is not an address of this machine.'
    Assert-PortableTest (Test-PortableLocalAddress (Get-PortablePublicHost)) 'The detected LAN address must be a local address.'
    # The plan math below is about firewall coverage, not about this test machine's
    # interfaces, so the address check is held true for it.
    function Test-PortableLocalAddress { return $true }
    function New-TestFirewallRule {
        param(
            [string]$DisplayName = 'test-rule',
            [string]$Direction = 'Inbound',
            [string]$Action = 'Allow',
            [string]$Enabled = 'True',
            [string]$Program = '',
            [string]$Protocol = 'TCP',
            [string]$LocalPort = '13443',
            [string]$RemoteAddress = ''
        )
        return [pscustomobject]@{
            DisplayName = $DisplayName; Direction = $Direction; Action = $Action; Enabled = $Enabled
            Program = $Program; Protocol = $Protocol; LocalPort = $LocalPort; RemoteAddress = $RemoteAddress
        }
    }
    $programRule = New-TestFirewallRule -DisplayName 'UWorkers-Portable-HTTPS-13443' -Program $caddyPath
    Assert-PortableTest ($null -ne (Find-PortableFirewallCoverage -Rules @($programRule) -ProgramPath $caddyPath -Port 13443)) 'A program-scoped inbound allow rule must cover the LAN listener.'
    $stalePathRule = New-TestFirewallRule -Program 'D:\Old\payload\runtime\caddy\caddy.exe'
    Assert-PortableTest ($null -eq (Find-PortableFirewallCoverage -Rules @($stalePathRule) -ProgramPath $caddyPath -Port 13443)) 'A rule for an old caddy.exe path must not cover the new one; that stale path is how the prompt comes back after a move.'
    $udpRule = New-TestFirewallRule -Protocol 'UDP'
    Assert-PortableTest ($null -eq (Find-PortableFirewallCoverage -Rules @($udpRule) -ProgramPath $caddyPath -Port 13443)) 'A UDP rule must not cover the TCP listener.'
    $disabledRule = New-TestFirewallRule -Enabled 'False' -Program $caddyPath
    Assert-PortableTest ($null -eq (Find-PortableFirewallCoverage -Rules @($disabledRule) -ProgramPath $caddyPath -Port 13443)) 'A disabled rule must not cover the listener.'
    $anyPortRule = New-TestFirewallRule -Program $caddyPath -LocalPort 'Any'
    Assert-PortableTest ($null -ne (Find-PortableFirewallCoverage -Rules @($anyPortRule) -ProgramPath $caddyPath -Port 13443)) 'A rule that does not constrain the port must still cover it.'
    $rangeRule = New-TestFirewallRule -Program $caddyPath -LocalPort '13440-13450'
    Assert-PortableTest ($null -ne (Find-PortableFirewallCoverage -Rules @($rangeRule) -ProgramPath $caddyPath -Port 13443)) 'A port range must cover a port inside it.'
    $anyProgramRule = New-TestFirewallRule -Program ''
    Assert-PortableTest ($null -eq (Find-PortableFirewallCoverage -Rules @($anyProgramRule) -ProgramPath $caddyPath -Port 13443)) 'A rule that names no program must not count: the prompt and the permission are per program, and Windows ships dozens of "any program" packaged-app rules.'

    # The rule has to admit the LAN address, or the LAN entry point would be
    # advertised while the firewall drops the traffic (a leftover scope from an
    # earlier network is the realistic case).
    $scopedRule = New-TestFirewallRule -Program $caddyPath -RemoteAddress '192.168.3.0/255.255.255.0'
    Assert-PortableTest ($null -ne (Find-PortableFirewallCoverage -Rules @($scopedRule) -ProgramPath $caddyPath -Port 13443 -Address '192.168.3.49')) 'A rule scoped to the LAN subnet must cover an address inside it.'
    Assert-PortableTest ($null -eq (Find-PortableFirewallCoverage -Rules @($scopedRule) -ProgramPath $caddyPath -Port 13443 -Address '192.168.5.49')) 'A rule scoped to another subnet must not cover the current LAN address.'
    $anyScopeRule = New-TestFirewallRule -Program $caddyPath -RemoteAddress 'Any'
    Assert-PortableTest ($null -ne (Find-PortableFirewallCoverage -Rules @($anyScopeRule) -ProgramPath $caddyPath -Port 13443 -Address '192.168.5.49')) 'An any-address rule covers every LAN address.'
    $localSubnetRule = New-TestFirewallRule -Program $caddyPath -RemoteAddress 'LocalSubnet'
    Assert-PortableTest ($null -ne (Find-PortableFirewallCoverage -Rules @($localSubnetRule) -ProgramPath $caddyPath -Port 13443 -Address '192.168.5.49')) 'The LocalSubnet keyword covers whatever subnet the machine is on.'
    Assert-PortableTest (Test-PortableFirewallRemoteAddressCovers -Value 'some-future-keyword' -Address '192.168.3.49') 'An unknown address keyword must be treated as covering, never as a reason to switch LAN access off.'
    $staleScopePlan = Resolve-PortableLanPlan -RequestedHost '192.168.5.49' -HttpsPort 13443 -CaddyPath $caddyPath -SkipFirewallProbe -Rules @($scopedRule)
    Assert-PortableTest (-not $staleScopePlan.Exposed -and $staleScopePlan.Firewall -eq 'needs-admin') 'A rule scoped to another network must not be advertised as a working LAN entry.'

    $loopbackPlan = Resolve-PortableLanPlan -RequestedHost '127.0.0.1' -HttpsPort 13443 -CaddyPath $caddyPath -SkipFirewallProbe -Rules @($programRule)
    Assert-PortableTest (-not $loopbackPlan.Exposed -and $loopbackPlan.LanHost -eq '127.0.0.1') 'A loopback request must stay loopback even when a rule exists.'
    Assert-PortableTest ($loopbackPlan.Firewall -eq 'loopback') 'A loopback request must be reported as loopback.'

    $coveredPlan = Resolve-PortableLanPlan -RequestedHost '192.168.3.49' -HttpsPort 13443 -CaddyPath $caddyPath -SkipFirewallProbe -Rules @($programRule)
    Assert-PortableTest ($coveredPlan.Exposed -and $coveredPlan.LanHost -eq '192.168.3.49') 'A covered LAN request must bind the LAN address.'
    Assert-PortableTest ($coveredPlan.Firewall -eq 'existing') 'An existing rule must be reported as the coverage source.'

    $script:firewallCalls = New-Object System.Collections.Generic.List[object]
    function Test-PortableElevated { return $false }
    $noAdminPlan = Resolve-PortableLanPlan -RequestedHost '192.168.3.49' -HttpsPort 13443 -CaddyPath $caddyPath -SkipFirewallProbe -Rules @() -AllowRuleCreation
    Assert-PortableTest (-not $noAdminPlan.Exposed -and $noAdminPlan.LanHost -eq '127.0.0.1') 'Without a rule and without administrator rights the entry point must stay loopback.'
    Assert-PortableTest ($noAdminPlan.Firewall -eq 'needs-admin') 'The plan must report why LAN access is off.'
    Assert-PortableTest ($noAdminPlan.Message -match 'administrator') 'The message must say that an administrator is needed.'
    Assert-PortableTest ($noAdminPlan.Message -match 'localhost:13443') 'The message must name the address that still works.'
    Assert-PortableTest ($script:firewallCalls.Count -eq 0) 'No rule may be added without administrator rights.'

    function Test-PortableElevated { return $true }
    function Install-PortableFirewallRule {
        param([string]$ProgramPath, [int]$Port, [string]$RemoteAddress, [string]$DisplayName)
        $script:firewallCalls.Add([pscustomobject]@{ ProgramPath = $ProgramPath; Port = $Port; RemoteAddress = $RemoteAddress; DisplayName = $DisplayName })
        return [pscustomobject]@{ Added = $true; DisplayName = $DisplayName; Detail = 'added' }
    }
    $createdPlan = Resolve-PortableLanPlan -RequestedHost '203.0.113.17' -HttpsPort 13443 -CaddyPath $caddyPath -SkipFirewallProbe -Rules @() -AllowRuleCreation
    Assert-PortableTest ($createdPlan.Exposed) 'An administrator install must be able to grant LAN access by adding the rule.'
    Assert-PortableTest ($createdPlan.Firewall -eq 'created') 'The plan must say the rule was created rather than found.'
    Assert-PortableTest ($script:firewallCalls.Count -eq 1) 'Exactly one firewall rule must be added.'
    Assert-PortableTest ($script:firewallCalls[0].ProgramPath -eq $caddyPath) 'The rule must name this installation caddy.exe path so upgrades keep it.'
    Assert-PortableTest ($script:firewallCalls[0].Port -eq 13443) 'The rule must name the HTTPS port.'
    Assert-PortableTest ($script:firewallCalls[0].RemoteAddress -eq '203.0.113.0/24') 'The rule must be scoped to the LAN subnet.'
    Assert-PortableTest ($script:firewallCalls[0].DisplayName -eq 'UWorkers-Portable-HTTPS-13443') 'The rule needs the documented display name.'

    $startPlan = Resolve-PortableLanPlan -RequestedHost '203.0.113.17' -HttpsPort 13443 -CaddyPath $caddyPath -SkipFirewallProbe -Rules @()
    Assert-PortableTest (-not $startPlan.Exposed -and $script:firewallCalls.Count -eq 1) 'A start must never add a firewall rule; only install and upgrade may.'

    $hostnamePlan = Resolve-PortableLanPlan -RequestedHost 'board.example' -HttpsPort 13443 -CaddyPath $caddyPath -SkipFirewallProbe -Rules @($programRule)
    Assert-PortableTest (-not $hostnamePlan.Exposed -and $hostnamePlan.Firewall -eq 'hostname') 'A DNS name cannot be bound to a rule-covered address; it must stay loopback.'

    # A recorded address that DHCP moved away from must not be bound: Caddy would
    # fail to start. The plan falls back to loopback and says why.
    function Test-PortableLocalAddress { return $false }
    $stalePlan = Resolve-PortableLanPlan -RequestedHost '192.168.3.49' -HttpsPort 13443 -CaddyPath $caddyPath -SkipFirewallProbe -Rules @($programRule)
    Assert-PortableTest (-not $stalePlan.Exposed -and $stalePlan.Firewall -eq 'stale-address') 'A stale LAN address must not be exposed.'
    Assert-PortableTest ($stalePlan.Message -match '192\.168\.3\.49') 'The stale-address message must name the address that no longer exists.'
    function Test-PortableLocalAddress { return $true }

    # LAN report round-trip: what the operator asked for survives a restart even
    # when the grant was refused.
    $planReportPath = Join-Path $testRoot 'data\install-report.json'
    Write-PortableJson -Path $planReportPath -Value @{ lanHost = '192.168.3.99'; httpsPort = 13443; profile = 'portable-test' }
    Update-PortableLanReport -Root $testRoot -Plan $noAdminPlan | Out-Null
    $planReport = Read-PortableJson $planReportPath
    Assert-PortableTest ([bool]$planReport.lanExposed -eq $false -and [string]$planReport.lanHost -eq '127.0.0.1') 'The report must record the address that is really served.'
    Assert-PortableTest ((Get-PortableRequestedLanHostFromConfig -Root $testRoot) -eq '192.168.3.49') 'The report must keep what the operator asked for, so a restart can retry the grant.'
    Assert-PortableTest ((Get-PortableLanSubnet '203.0.113.17') -eq '203.0.113.0/24') 'The firewall rule must be scoped to the interface subnet.'


    # --- captured output is UTF-8: non-ASCII device name --------------------
    # The status document includes the device name, which is the Windows computer
    # name and can be Chinese. Windows PowerShell 5.1 read Invoke-PortableCommand's
    # captured stdout file in the ANSI code page, which mangled those bytes badly
    # enough to swallow the quote that ends a JSON string: ConvertFrom-Json failed
    # and a running daemon was reported as stopped. Push the exact bytes the CLI
    # writes through the real capture path, then judge them the way the daemon
    # status does.
    $deviceName = -join @([char]0x79FB, [char]0x52A8, [char]0x4E13, [char]0x7528, [char]0x673A)
    $statusWriter = Join-Path $testRoot 'write-status.ps1'
    $statusDocument = Join-Path $testRoot 'status-payload.json'
    $writer = @'
$payloadPath = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) 'status-payload.json'
$bytes = [IO.File]::ReadAllBytes($payloadPath)
$stdout = [Console]::OpenStandardOutput()
$stdout.Write($bytes, 0, $bytes.Length)
$stdout.Flush()
exit 0
'@
    [IO.File]::WriteAllText($statusWriter, $writer, (New-Object Text.UTF8Encoding($false)))
    $windowsPowerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    function Write-FakeStatus {
        param([string]$Status)
        $json = '{"status":"' + $Status + '","pid":4242,"device_name":"' + $deviceName + '"}'
        [IO.File]::WriteAllText($statusDocument, $json, (New-Object Text.UTF8Encoding($false)))
    }
    function Read-CapturedStatus {
        $capture = Invoke-PortableCommand -Executable $windowsPowerShell -Arguments @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $statusWriter) -Utf8Output
        return (ConvertFrom-PortableDaemonStatus -ExitCode ([int]$capture.ExitCode) -Output $capture.Output)
    }
    Write-FakeStatus 'running'
    $utf8Running = Read-CapturedStatus
    Assert-PortableTest ([bool]$utf8Running.Running) 'A running daemon with a non-ASCII device name must be reported as running.'
    Assert-PortableTest ([string]$utf8Running.Detail.device_name -eq $deviceName) 'The captured device name must decode as the UTF-8 bytes the CLI wrote.'
    Write-FakeStatus 'starting'
    Assert-PortableTest ([bool](Read-CapturedStatus).Running) 'A starting daemon with a non-ASCII device name must count as alive.'
    Write-FakeStatus 'stopped'
    $utf8Stopped = Read-CapturedStatus
    Assert-PortableTest (-not $utf8Stopped.Running) 'A stopped status document must be reported as stopped.'
    Assert-PortableTest ([string]$utf8Stopped.Detail.device_name -eq $deviceName) 'A stopped status document must parse instead of reading as unparseable output.'

    # The wrapper itself must ask for UTF-8, or the daemon status path silently
    # goes back to the shell's default decoding.
    $script:captureAskedUtf8 = $false
    function Invoke-PortableCommand {
        param([string]$Executable, [string[]]$Arguments, [hashtable]$Environment, [string]$WorkingDirectory, [int]$TimeoutSec = 3600, [switch]$Utf8Output)
        $script:captureAskedUtf8 = [bool]$Utf8Output
        return [pscustomobject]@{ ExitCode = 0; Output = '{"status":"running"}' }
    }
    function Get-PortableCliPath { param([string]$Root) return 'u-workers.exe' }
    Invoke-PortableCli -Root $testRoot -Arguments @('daemon', 'status', '--output', 'json') | Out-Null
    Assert-PortableTest $script:captureAskedUtf8 'Invoke-PortableCli must capture the CLI output as UTF-8.'

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
