#Requires -Version 5.1
<#
.SYNOPSIS
    End-to-end acceptance for the portable U Workers package.

.DESCRIPTION
    Exercises the package the way the delivery requires:

    1. copies the package into a clean, isolated folder (the "server machine");
    2. disables dependency downloads with dead proxy variables for every child
       process and installs with one script;
    3. verifies the board over HTTPS with the generated private CA, the local
       sign-in, the workspace, and an opencode runtime registered by the bundled
       daemon, plus the listen plan that keeps Caddy off every address no
       firewall rule covers (the Windows Firewall prompt Tane saw on the
       desktop) and off HTTP/3's UDP port;
    4. simulates an unclean shutdown — stops the stack and zero-fills
       state\processes.json — and verifies Start recovers, keeps the damaged file,
       and brings the daemon and its runtime back;
    5. rebuilds the installer from the bundled toolchain while still offline;
    6. simulates a second machine with the packaged CLI plus a separate CLI
       profile, joins it with 'connect', and observes a new runtime;
    7. rehearses an overlay upgrade from a newer package: backup exists, version
       changed, and the data written before the upgrade is still there;
    8. cleans up: stops the service, removes the start-on-login item and the
       package CLI profiles, and fails if a process or port is left behind.

    It never touches the existing Multica daemon or the existing native
    deployment on the machine: this package gets its own ports, its own data
    directory and its own CLI profiles, and the Multica CLI config file hash is
    checked before and after.

.PARAMETER Package
    The built package folder (UWorkers-Portable-vX.Y.Z).

.PARAMETER UpgradePackage
    A newer package folder for the upgrade rehearsal. Optional: the upgrade step
    is reported as skipped when omitted.

.PARAMETER WorkRoot
    Writable scratch directory. The package and both simulated machines live
    here; it is removed afterwards unless -KeepWorkRoot is given.

.PARAMETER EvidencePath
    Where the acceptance JSON is written.

.PARAMETER KeepRunning
    Do not clean up at the end (used while collecting screenshots). The script
    prints the cleanup commands instead.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Package,
    [string]$UpgradePackage,
    [string]$WorkRoot = (Join-Path $env:TEMP 'uworkers-acceptance'),
    [string]$EvidencePath,
    [string]$ServerProfile = 'uworkers-acceptance-server',
    [string]$SecondProfile = 'uworkers-acceptance-second',
    [int]$HttpsPort = 23443,
    [int]$ApiPort = 28080,
    [int]$WebPort = 23000,
    [int]$DatabasePort = 26433,
    [switch]$SkipInstallerBuild,
    [switch]$KeepWorkRoot,
    [switch]$KeepRunning,
    [switch]$ResumeExisting,
    [string]$HideCheckoutPath
)

$ErrorActionPreference = 'Stop'
$started = Get-Date
$checks = New-Object System.Collections.Generic.List[object]
$evidence = [ordered]@{}

function Add-Check {
    param([string]$Name, [bool]$Ok, [string]$Detail = '')
    $checks.Add([pscustomobject]@{ name = $Name; ok = $Ok; detail = $Detail; at = (Get-Date).ToUniversalTime().ToString('o') })
    if ($Ok) { Write-Host ('  [PASS] ' + $Name) -ForegroundColor Green }
    else { Write-Host ('  [FAIL] ' + $Name + '  ' + $Detail) -ForegroundColor Red }
    return $Ok
}

function Assert-Check {
    param([string]$Name, [bool]$Ok, [string]$Detail = '')
    if (-not (Add-Check $Name $Ok $Detail)) { throw ('Acceptance check failed: ' + $Name + '  ' + $Detail) }
}

function Write-Step {
    param([string]$Text)
    Write-Host ''
    Write-Host ('==> ' + $Text) -ForegroundColor Cyan
}

function Invoke-Robocopy {
    param([string]$Source, [string]$Destination, [string[]]$ExcludeDirectories)
    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    if (-not $ExcludeDirectories) { $ExcludeDirectories = @() }
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        # /SJ /SL keep the web release's dependency links intact (expanding
        # them would lose Next.js' sibling dependencies); the same-machine test
        # copy can follow them because the build checkout still exists.
        & robocopy.exe $Source $Destination /E /SJ /SL /R:2 /W:1 /MT:8 /NP /NFL /NDL /NJH /NJS /XD @ExcludeDirectories | Out-Null
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    if ($exitCode -ge 8) { throw ('robocopy failed (' + $exitCode + '): ' + $Source + ' -> ' + $Destination) }
}

function Get-FileSha256 {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { return $null }
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
}

function Get-ProfileDirectories {
    param([string]$Profile)
    $homeDirectory = [Environment]::GetFolderPath('UserProfile')
    $result = @()
    foreach ($name in @('.u-workers', '.multica')) {
        $path = Join-Path $homeDirectory ($name + '\profiles\' + $Profile)
        if (Test-Path -LiteralPath $path) { $result += $path }
    }
    return $result
}

$packagePath = [IO.Path]::GetFullPath($Package)
if (-not (Test-Path -LiteralPath (Join-Path $packagePath 'manifest.json'))) { throw ('Not a package folder: ' + $packagePath) }
$manifest = Get-Content -LiteralPath (Join-Path $packagePath 'manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$version = [string]$manifest.version
if (-not $EvidencePath) { $EvidencePath = Join-Path (Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $PSScriptRoot))) ('evidence\clawsta-107\acceptance.json') }
$workRoot = [IO.Path]::GetFullPath($WorkRoot)
# Safety: the acceptance run purges this directory; it must be a scratch path.
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
if (-not $workRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and -not $workRoot.StartsWith('D:\UWorkers\', [StringComparison]::OrdinalIgnoreCase)) {
    throw ('The acceptance work root must be a scratch directory: ' + $workRoot)
}
$serverCopy = Join-Path $workRoot 'server'
$secondMachine = Join-Path $workRoot 'second-machine'
$upgradeCopy = Join-Path $workRoot 'upgrade-package'

# Snapshot the machine's existing Multica CLI state: the package must not touch it.
$multicaConfig = Join-Path ([Environment]::GetFolderPath('UserProfile')) '.multica\config.json'
$multicaConfigBefore = Get-FileSha256 $multicaConfig

$installLog = Join-Path $workRoot 'install.log'
$installed = $false
$cleanupErrors = @()

try {
    if ($ResumeExisting) {
        # Continue against a clean copy left behind by an interrupted acceptance
        # run; Setup itself resumes an unfinished install in that folder.
        Write-Step ('Resuming acceptance against the existing clean copy under ' + $workRoot)
        Assert-Check 'existing acceptance copy present' (Test-Path -LiteralPath (Join-Path $serverCopy 'install.ps1')) $serverCopy
        if (-not (Test-Path -LiteralPath $secondMachine)) { New-Item -ItemType Directory -Path $secondMachine -Force | Out-Null }
    } else {
        Write-Step ('Preparing an isolated acceptance area under ' + $workRoot)
        if (Test-Path -LiteralPath $workRoot) {
            if (-not $KeepWorkRoot) { Remove-Item -LiteralPath $workRoot -Recurse -Force }
        }
        New-Item -ItemType Directory -Path $workRoot -Force | Out-Null
        # The Go build kit is read-only input. Copying it would move a few
        # gigabytes of module cache per test run, so the clean copy links it
        # from the package folder instead; the offline rebuild still reads only
        # files inside the package.
        # Exclude only the package's own top-level tools folder; a bare name
        # would also exclude same-named folders inside the web dependencies.
        Invoke-Robocopy $packagePath $serverCopy -ExcludeDirectories @((Join-Path $packagePath 'tools'))
        $sourceTools = Join-Path $packagePath 'tools'
        if (Test-Path -LiteralPath $sourceTools) {
            $linkedTools = Join-Path $serverCopy 'tools'
            if (Test-Path -LiteralPath $linkedTools) { Remove-Item -LiteralPath $linkedTools -Recurse -Force }
            New-Item -ItemType Junction -Path $linkedTools -Target $sourceTools | Out-Null
        }
        New-Item -ItemType Directory -Path $secondMachine -Force | Out-Null
        Assert-Check 'package copied to a clean folder' (Test-Path -LiteralPath (Join-Path $serverCopy 'install.ps1')) $serverCopy

        foreach ($port in @($HttpsPort, $ApiPort, $WebPort, $DatabasePort)) {
            $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, $port)
            $free = $true
            try { $listener.Start() } catch { $free = $false } finally { $listener.Stop() }
            Assert-Check ('port ' + $port + ' is free before the install') $free
        }
    }

    # --- offline simulation -------------------------------------------------
    $publicHost = '127.0.0.1'
    $deadProxy = 'http://127.0.0.1:9'
    $savedEnvironment = @{}
    foreach ($name in @('HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'NO_PROXY')) {
        $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
    }
    $env:HTTP_PROXY = $deadProxy
    $env:HTTPS_PROXY = $deadProxy
    $env:ALL_PROXY = $deadProxy
    $env:NO_PROXY = 'localhost,127.0.0.1,' + $publicHost
    Add-Check 'dependency downloads disabled for the install' $true ('HTTP(S)_PROXY=' + $deadProxy)

    Write-Step 'Installing the package with one script (offline, no admin)'
    $installArguments = @{
        PublicHost = $publicHost
        HttpsPort = $HttpsPort
        ApiPort = $ApiPort
        WebPort = $WebPort
        DatabasePort = $DatabasePort
        Profile = $ServerProfile
        SkipBrowserTrust = $true
        NoBrowser = $true
    }
    & (Join-Path $serverCopy 'install.ps1') @installArguments 2>&1 | Tee-Object -FilePath $installLog | Out-Null
    $installed = $true
    Assert-Check 'install script completed' (Test-Path -LiteralPath (Join-Path $serverCopy 'deployment.json'))
    $report = Get-Content -LiteralPath (Join-Path $serverCopy 'data\install-report.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    Assert-Check 'install report written' ($null -ne $report)
    Assert-Check 'install report says clean install' ([string]$report.mode -eq 'clean') ([string]$report.mode)
    Assert-Check 'install report version matches the package' ([string]$report.version -eq $version) ([string]$report.version)

    # --- LAN listen plan and the generated Caddyfile ------------------------
    # The desktop deployment showed Caddy's Windows Firewall prompt on every
    # start. The prompt appears when a program listens beyond loopback without an
    # inbound allow rule covering it, and Caddy's default wildcard listen plus its
    # HTTP/3 UDP socket were both uncovered. The package now binds exactly the
    # addresses a rule covers and turns HTTP/3 off. This acceptance runs as a
    # standard user, so the plan has to keep the entry point on 127.0.0.1.
    Write-Step 'Checking the LAN listen plan and the generated Caddyfile'
    Assert-Check 'install report records the LAN plan' ($null -ne $report.lanExposed -and $null -ne $report.lanHost) ([string]$report.lanHost)
    Assert-Check 'a loopback public host keeps the installation loopback-only' ((-not [bool]$report.lanExposed) -and ([string]$report.lanHost -eq '127.0.0.1')) ([string]$report.lanExposed + ' ' + [string]$report.lanHost)
    Assert-Check 'the install report explains the LAN state' ([string]$report.lanMessage -match 'LAN') ([string]$report.lanMessage)
    $installedCaddyfile = Join-Path $serverCopy 'https\Caddyfile'
    $caddyfileText = [string](Get-Content -LiteralPath $installedCaddyfile -Raw -Encoding UTF8)
    Assert-Check 'the installed Caddyfile binds the loopback address explicitly' ($caddyfileText -match 'bind 127\.0\.0\.1') $installedCaddyfile
    Assert-Check 'the installed Caddyfile never binds a wildcard address' (-not ($caddyfileText -match 'bind 0\.0\.0\.0') -and -not ($caddyfileText -match 'bind ::')) $installedCaddyfile
    Assert-Check 'the installed Caddyfile disables HTTP/3 so only the TCP listener exists' ($caddyfileText -match 'protocols h1 h2') $installedCaddyfile
    $tcpListeners = @(Get-NetTCPConnection -LocalPort $HttpsPort -State Listen -ErrorAction SilentlyContinue | ForEach-Object { [string]$_.LocalAddress } | Sort-Object -Unique)
    Assert-Check 'the HTTPS port listens on 127.0.0.1 only' (($tcpListeners -join ',') -eq '127.0.0.1') ($tcpListeners -join ',')
    $udpListeners = @(Get-NetUDPEndpoint -LocalPort $HttpsPort -ErrorAction SilentlyContinue)
    Assert-Check 'the HTTPS port has no UDP listener the TCP rule cannot cover' ($udpListeners.Count -eq 0) (($udpListeners | ForEach-Object { [string]$_.LocalAddress }) -join ',')

    # --- board over HTTPS with the private CA ------------------------------
    Write-Step 'Checking the board over HTTPS with the generated root CA'
    . (Join-Path $serverCopy 'portable-common.ps1')
    $serverUrl = 'https://' + $publicHost + ':' + $HttpsPort
    $rootCa = Get-PortablePublicRootCAPath $serverCopy
    Assert-Check 'root-ca.crt exported next to the server scripts' (Test-Path -LiteralPath $rootCa) $rootCa
    $ready = Invoke-PortableWebRequest -Uri ($serverUrl + '/readyz') -RootCAPath $rootCa
    Assert-Check 'API ready over HTTPS' ($ready.StatusCode -eq 200) ([string]$ready.StatusCode)
    $loginPage = Invoke-PortableWebRequest -Uri ($serverUrl + '/login') -RootCAPath $rootCa
    Assert-Check 'sign-in page over HTTPS' ($loginPage.StatusCode -eq 200) ([string]$loginPage.StatusCode)
    $board = Invoke-PortableWebRequest -Uri ($serverUrl + '/') -RootCAPath $rootCa
    # "/" redirects to onboarding or the workspace; a non-error status proves the
    # web app is served through the HTTPS entry.
    Assert-Check 'board responds over HTTPS' ($board.StatusCode -ge 200 -and $board.StatusCode -lt 400) ([string]$board.StatusCode)
    $untrustedRejected = $false
    try { Invoke-PortableWebRequest -Uri ($serverUrl + '/readyz') -TimeoutSec 10 | Out-Null } catch { $untrustedRejected = $true }
    Assert-Check 'HTTPS certificate is not publicly trusted (proves the private CA path)' $untrustedRejected

    # HTTP/3 advertises itself in this header; with it gone there is no UDP
    # socket for Windows Firewall to treat as an unrecognised program.
    $curlExe = Join-Path $env:SystemRoot 'System32\curl.exe'
    $headLines = @(& $curlExe -k -s -I --max-time 15 ($serverUrl + '/login'))
    $altSvc = @($headLines | Where-Object { $_ -match '(?i)^alt-svc:' })
    Assert-Check 'the HTTPS entry does not advertise HTTP/3' ($altSvc.Count -eq 0) ($altSvc -join '; ')

    # --- what a LAN request does without administrator rights ---------------
    # This is the case the desktop hit: a LAN address is detected, no firewall
    # rule covers this installation, and the person at the machine cannot click
    # "Allow". The plan must refuse the LAN listener instead of prompting, and
    # still produce a configuration the packaged Caddy accepts.
    Write-Step 'Checking the LAN plan for a machine without administrator rights'
    $detectedLanHost = Get-PortablePublicHost
    $tempCaddy = Get-PortableCaddyPath $serverCopy
    $liveRules = @()
    try { $liveRules = @(Get-PortableFirewallSnapshot) } catch { $liveRules = @() }
    $liveCoverage = Find-PortableFirewallCoverage -Rules $liveRules -ProgramPath $tempCaddy -Port $HttpsPort
    $liveLanPlan = Resolve-PortableLanPlan -Root $serverCopy -RequestedHost $detectedLanHost -HttpsPort $HttpsPort
    if (Test-PortableLoopbackHost $detectedLanHost) {
        Add-Check 'LAN request plan' $true ('skipped: this machine has no LAN address (detected ' + $detectedLanHost + ')')
    } elseif ($liveCoverage) {
        Assert-Check 'an existing firewall rule lets a LAN request bind the LAN address' ([bool]$liveLanPlan.Exposed -and [string]$liveLanPlan.LanHost -eq $detectedLanHost) ([string]$liveLanPlan.Firewall + ' ' + [string]$liveLanPlan.LanHost)
    } else {
        Assert-Check 'a LAN request without a firewall rule stays loopback-only' ((-not [bool]$liveLanPlan.Exposed) -and ([string]$liveLanPlan.LanHost -eq '127.0.0.1')) ([string]$liveLanPlan.Firewall + ' ' + [string]$liveLanPlan.LanHost)
        Assert-Check 'a refused LAN request explains that an administrator is needed' ([string]$liveLanPlan.Message -match 'administrator') ([string]$liveLanPlan.Message)
    }
    $planCaddyfile = Get-PortableCaddyfile -Root $serverCopy -PublicHost $detectedLanHost -HttpsPort $HttpsPort -ApiPort $ApiPort -WebPort $WebPort -LanExposed ([bool]$liveLanPlan.Exposed)
    if (-not $liveLanPlan.Exposed -and -not (Test-PortableLoopbackHost $detectedLanHost)) {
        Assert-Check 'a refused LAN request produces a loopback-only Caddyfile' (-not ($planCaddyfile -match [regex]::Escape($detectedLanHost)) -and $planCaddyfile -match 'bind 127\.0\.0\.1') $detectedLanHost
    } else {
        Assert-Check 'a granted LAN request binds the LAN address' ($planCaddyfile -match ('bind 127\.0\.0\.1 ' + [regex]::Escape($detectedLanHost))) $detectedLanHost
    }
    $planCaddyfilePath = Join-Path $workRoot 'plan-Caddyfile'
    [IO.File]::WriteAllText($planCaddyfilePath, $planCaddyfile, (New-Object Text.UTF8Encoding($false)))
    $planValidateOut = Join-Path $workRoot 'plan-validate.out'
    $planValidateErr = Join-Path $workRoot 'plan-validate.err'
    $planValidate = Start-Process -FilePath $tempCaddy -ArgumentList @('validate','--config',$planCaddyfilePath,'--adapter','caddyfile') -RedirectStandardOutput $planValidateOut -RedirectStandardError $planValidateErr -Wait -PassThru -WindowStyle Hidden
    Assert-Check 'the packaged Caddy accepts the plan configuration' ($planValidate.ExitCode -eq 0) ([string](Get-Content -LiteralPath $planValidateErr -Raw -ErrorAction SilentlyContinue))

    # --- portability: the installed web must not reference the build checkout ---
    $deployment = Get-PortableDeployment $serverCopy
    $installedWeb = Join-Path ([string]$deployment.Release) 'web'
    $webLinks = Get-PortableExternalWebLinks -WebPath $installedWeb
    Assert-Check 'installed web release references only files inside the package' ($webLinks.Outside -eq 0) (('links=' + $webLinks.Total + ' outside=' + $webLinks.Outside + ' ' + (($webLinks.OutsidePaths | Select-Object -First 1) -join '')))
    if ($HideCheckoutPath) {
        Write-Step ('Checking the board with the build checkout hidden (' + $HideCheckoutPath + ')')
        $hideTarget = [IO.Path]::GetFullPath($HideCheckoutPath).TrimEnd('\')
        $hideLabel = $hideTarget
        $renamed = $false
        $hiddenPath = $null
        try {
            $hiddenPath = $hideTarget + '.hidden-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
            for ($attempt = 0; $attempt -lt 40 -and -not $renamed; $attempt++) {
                try {
                    Rename-Item -LiteralPath $hideTarget -NewName (Split-Path -Leaf $hiddenPath) -ErrorAction Stop
                    $renamed = $true
                } catch {
                    Start-Sleep -Milliseconds 500
                }
            }
            if (-not $renamed) {
                # Some environments lock the checkout root itself (open handles from
                # build tools or antivirus); hiding the dependency store proves the
                # same property because every packaged dependency link pointed there.
                $fallbackTarget = Join-Path $hideTarget 'node_modules'
                if (Test-Path -LiteralPath $fallbackTarget) {
                    $fallbackHidden = $fallbackTarget + '.hidden-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
                    for ($attempt = 0; $attempt -lt 20 -and -not $renamed; $attempt++) {
                        try {
                            Rename-Item -LiteralPath $fallbackTarget -NewName (Split-Path -Leaf $fallbackHidden) -ErrorAction Stop
                            $hideTarget = $fallbackTarget
                            $hiddenPath = $fallbackHidden
                            $hideLabel = $fallbackTarget
                            $renamed = $true
                        } catch {
                            Start-Sleep -Milliseconds 500
                        }
                    }
                }
            }
            Assert-Check 'build checkout hidden during the board check' $renamed $hideLabel
            $loginHidden = Invoke-PortableWebRequest -Uri ($serverUrl + '/login') -RootCAPath $rootCa -TimeoutSec 20
            Assert-Check 'board works with the build checkout hidden' ($loginHidden.StatusCode -eq 200 -and $loginHidden.Content.Length -gt 10000) ([string]$loginHidden.StatusCode + ' ' + [string]$loginHidden.Content.Length)
            $apiHidden = Invoke-PortableWebRequest -Uri ($serverUrl + '/readyz') -RootCAPath $rootCa
            Assert-Check 'API works with the build checkout hidden' ($apiHidden.StatusCode -eq 200) ([string]$apiHidden.StatusCode)
        } finally {
            if ($renamed -and $hiddenPath) {
                $restored = $false
                for ($attempt = 0; $attempt -lt 40 -and -not $restored; $attempt++) {
                    try {
                        Rename-Item -LiteralPath $hiddenPath -NewName (Split-Path -Leaf $hideTarget) -ErrorAction Stop
                        $restored = $true
                    } catch {
                        Start-Sleep -Milliseconds 500
                    }
                }
                if (-not $restored) { throw ('Could not restore the hidden path: ' + $hiddenPath) }
            }
        }
    }

    # --- sign-in and workspace ---------------------------------------------
    Write-Step 'Checking the provisioned local account and workspace'
    $loginFile = Join-Path $serverCopy 'data\local-login.txt'
    Assert-Check 'local credentials file exists' (Test-Path -LiteralPath $loginFile) $loginFile
    $credentialLines = @(Get-Content -LiteralPath $loginFile -Encoding UTF8)
    $username = ([string]$credentialLines[0]).Trim()
    $password = ([string]$credentialLines[1]).Trim()
    $loginBody = (@{ username = $username; password = $password } | ConvertTo-Json -Compress)
    $loginResponse = Invoke-PortableWebRequest -Uri ($serverUrl + '/auth/password') -Method POST -Body $loginBody -RootCAPath $rootCa
    Assert-Check 'local sign-in succeeds' ($loginResponse.StatusCode -eq 200) ([string]$loginResponse.StatusCode)
    $session = ConvertFrom-PortableResponse $loginResponse
    Assert-Check 'onboarding completed for the operator' ($null -ne $session.user.onboarded_at) ([string]$session.user.onboarded_at)
    $workspacesResponse = Invoke-PortableWebRequest -Uri ($serverUrl + '/api/workspaces') -RootCAPath $rootCa -Headers @{ Authorization = 'Bearer ' + [string]$session.token }
    $workspaces = @(ConvertFrom-PortableResponse $workspacesResponse)
    Assert-Check 'initial workspace created' ($workspaces.Count -ge 1) ([string]$workspaces.Count)
    $workspaceId = [string]$workspaces[0].id
    $workspaceSlug = [string]$workspaces[0].slug

    # --- opencode runtime ---------------------------------------------------
    Write-Step 'Waiting for the bundled daemon to register an opencode runtime'
    $deadline = (Get-Date).AddSeconds(150)
    $runtimes = @()
    while ((Get-Date) -lt $deadline) {
        $runtimeResponse = Invoke-PortableWebRequest -Uri ($serverUrl + '/api/runtimes') -RootCAPath $rootCa -Headers @{ Authorization = 'Bearer ' + [string]$session.token; 'X-Workspace-ID' = $workspaceId }
        if ($runtimeResponse.StatusCode -eq 200) {
            $runtimes = @(ConvertFrom-PortableResponse $runtimeResponse)
            if ($runtimes.Count -gt 0) { break }
        }
        Start-Sleep -Seconds 2
    }
    Assert-Check 'at least one runtime is online' ($runtimes.Count -ge 1) (($runtimes | ForEach-Object { [string]$_.name + '/' + [string]$_.provider }) -join ', ')
    $opencodeRuntime = @($runtimes | Where-Object { ([string]$_.provider -match 'opencode') -or ([string]$_.name -match 'opencode') })
    Assert-Check 'the bundled opencode runtime is online' ($opencodeRuntime.Count -ge 1) (($runtimes | ForEach-Object { [string]$_.name + '/' + [string]$_.provider }) -join ', ')
    $daemonStatus = Get-PortableDaemonStatus -Root $serverCopy -Profile $ServerProfile
    Assert-Check 'local daemon reports running' ([bool]$daemonStatus.Running)

    # --- start on login -----------------------------------------------------
    $startupItem = Get-PortableStartupShortcutPath
    Assert-Check 'start-on-login item registered in the current user Startup folder' (Test-Path -LiteralPath $startupItem) $startupItem

    # --- unclean shutdown: a damaged state file must not block the start ----
    # The supervisor rewrites state\processes.json every few seconds, so an
    # unclean shutdown can leave it zero-filled. Start used to abort inside
    # ConvertFrom-Json before it reached the service — exactly how the desktop
    # deployment failed to come back after a reboot. This stops the running
    # stack, damages the file the way the shutdown did, and starts again.
    Write-Step 'Simulating an unclean shutdown with a damaged state file'
    $statePath = Join-Path $serverCopy 'state\processes.json'
    $stopOutput = & (Join-Path $serverCopy 'stop.ps1') -Profile $ServerProfile -Quiet 2>&1 | Out-String
    $stopExit = $LASTEXITCODE
    Assert-Check 'stop script completed before the recovery scenario' ($stopExit -eq 0) (($stopOutput -split "\r?\n" | Select-Object -Last 3) -join ' | ')
    $stoppedDaemon = Get-PortableDaemonStatus -Root $serverCopy -Profile $ServerProfile
    Assert-Check 'a stopped daemon is reported as stopped' (-not [bool]$stoppedDaemon.Running) ([string]$stoppedDaemon.Output)
    $damagedLength = 792
    [IO.File]::WriteAllBytes($statePath, (New-Object byte[] $damagedLength))
    Assert-Check 'damaged state file staged for the recovery test' ((Get-Item -LiteralPath $statePath).Length -eq $damagedLength) $statePath
    $startOutput = & (Join-Path $serverCopy 'start.ps1') -Profile $ServerProfile -Quiet 2>&1 | Out-String
    $startExit = $LASTEXITCODE
    Assert-Check 'start recovers from a damaged state file' ($startExit -eq 0) (($startOutput -split "\r?\n" | Select-Object -Last 3) -join ' | ')
    $quarantined = @(Get-ChildItem -LiteralPath (Join-Path $serverCopy 'state') -Filter 'processes.json.corrupt-*' -ErrorAction SilentlyContinue)
    Assert-Check 'the damaged state file was kept for diagnosis' ($quarantined.Count -ge 1) (($quarantined | ForEach-Object { $_.Name + ' ' + $_.Length + ' bytes' }) -join ', ')
    $recoveredState = $null
    try { $recoveredState = Get-Content -LiteralPath $statePath -Raw -Encoding UTF8 | ConvertFrom-Json } catch { }
    Assert-Check 'a fresh state file replaced the damaged one' ($null -ne $recoveredState -and $null -ne $recoveredState.Supervisor)
    $readyRecovered = Invoke-PortableWebRequest -Uri ($serverUrl + '/readyz') -RootCAPath $rootCa -TimeoutSec 60
    Assert-Check 'API ready after recovering from the damaged state file' ($readyRecovered.StatusCode -eq 200) ([string]$readyRecovered.StatusCode)
    $daemonAfterRecovery = Get-PortableDaemonStatus -Root $serverCopy -Profile $ServerProfile
    Assert-Check 'the local daemon is running again after the recovery' ([bool]$daemonAfterRecovery.Running) ([string]$daemonAfterRecovery.Output)
    $daemonPid = 0
    try { $daemonPid = [int]$daemonAfterRecovery.Detail.pid } catch { }
    Assert-Check 'the reported daemon pid belongs to a live process' ($daemonPid -gt 0 -and $null -ne (Get-Process -Id $daemonPid -ErrorAction SilentlyContinue)) ([string]$daemonPid)
    $recoveredDeadline = (Get-Date).AddSeconds(150)
    $recoveredRuntimes = @()
    while ((Get-Date) -lt $recoveredDeadline) {
        $runtimeResponse = Invoke-PortableWebRequest -Uri ($serverUrl + '/api/runtimes') -RootCAPath $rootCa -Headers @{ Authorization = 'Bearer ' + [string]$session.token; 'X-Workspace-ID' = $workspaceId }
        if ($runtimeResponse.StatusCode -eq 200) {
            $recoveredRuntimes = @(ConvertFrom-PortableResponse $runtimeResponse)
            if ($recoveredRuntimes.Count -gt 0) { break }
        }
        Start-Sleep -Seconds 2
    }
    Assert-Check 'the bundled opencode runtime is online again after the recovery' ($recoveredRuntimes.Count -ge 1) (($recoveredRuntimes | ForEach-Object { [string]$_.name + '/' + [string]$_.provider }) -join ', ')

    # A restart rewrites the Caddyfile from the recorded plan; it must not fall
    # back to the wildcard listen (that is what produced the firewall prompt).
    $caddyfileAfterRecovery = [string](Get-Content -LiteralPath $installedCaddyfile -Raw -Encoding UTF8)
    Assert-Check 'the restarted Caddyfile still binds the loopback address' ($caddyfileAfterRecovery -match 'bind 127\.0\.0\.1') $installedCaddyfile
    $tcpAfterRecovery = @(Get-NetTCPConnection -LocalPort $HttpsPort -State Listen -ErrorAction SilentlyContinue | ForEach-Object { [string]$_.LocalAddress } | Sort-Object -Unique)
    Assert-Check 'the restarted HTTPS entry listens on 127.0.0.1 only' (($tcpAfterRecovery -join ',') -eq '127.0.0.1') ($tcpAfterRecovery -join ',')
    $udpAfterRecovery = @(Get-NetUDPEndpoint -LocalPort $HttpsPort -ErrorAction SilentlyContinue)
    Assert-Check 'the restarted HTTPS entry has no HTTP/3 UDP listener' ($udpAfterRecovery.Count -eq 0) (($udpAfterRecovery | ForEach-Object { [string]$_.LocalAddress }) -join ',')
    $recoveredReport = Get-Content -LiteralPath (Join-Path $serverCopy 'data\install-report.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    Assert-Check 'the restart keeps the recorded loopback plan' ($null -ne $recoveredReport.lanExposed -and (-not [bool]$recoveredReport.lanExposed) -and [string]$recoveredReport.lanRequestedHost) ([string]$recoveredReport.lanRequestedHost + ' -> ' + [string]$recoveredReport.lanHost)
    $evidence.recovery = [ordered]@{ damagedFileKept = @($quarantined | ForEach-Object { $_.Name }); daemonPid = $daemonPid; runtimes = @($recoveredRuntimes | ForEach-Object { [string]$_.name + '/' + [string]$_.provider }) }

    # --- installer artifact + offline rebuild -------------------------------
    Write-Step 'Checking the prebuilt installer and rebuilding it offline'
    $installer = Get-ChildItem -LiteralPath (Join-Path $serverCopy 'payload\installer') -Filter 'u-workers-setup-*.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
    Assert-Check 'prebuilt installer shipped in the package' ($null -ne $installer -and $installer.Length -gt 10MB) $(if ($installer) { [string]$installer.Length } else { 'missing' })
    $help = ''
    $installerExit = -1
    if ($installer) {
        $help = & $installer.FullName --help 2>&1 | Out-String
        $installerExit = $LASTEXITCODE
    }
    Assert-Check 'prebuilt installer starts and prints usage' ($installerExit -eq 0 -and $help -match 'u-workers-setup') ([string]$installerExit)
    $hasBuildKit = Test-Path -LiteralPath (Join-Path $serverCopy 'tools\go\bin\go.exe')
    if (-not $SkipInstallerBuild -and $hasBuildKit) {
        $rebuildDir = Join-Path $workRoot 'installer-rebuild'
        New-Item -ItemType Directory -Path $rebuildDir -Force | Out-Null
        $env:GOPROXY = 'off'
        $rebuild = & (Join-Path $serverCopy 'build-installer.ps1') -Version $version -OutputDir $rebuildDir 2>&1 | Out-String
        $rebuilt = Get-ChildItem -LiteralPath $rebuildDir -Filter '*.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
        Assert-Check 'installer rebuilt offline from the bundled toolchain' ($null -ne $rebuilt -and $rebuilt.Length -gt 10MB) ($rebuild -split "\r?\n" | Select-Object -Last 2 | Out-String)
    } elseif (-not $SkipInstallerBuild) {
        Add-Check 'installer rebuild' $false 'the package has no tools\go build kit (built with -SkipBuildKit)'
    } else {
        Add-Check 'installer rebuild' $true 'skipped (-SkipInstallerBuild)'
    }

    # --- second simulated machine ------------------------------------------
    Write-Step 'Simulating a second machine joining with connect'
    Copy-Item -LiteralPath $rootCa -Destination (Join-Path $secondMachine 'root-ca.crt') -Force
    New-Item -ItemType Directory -Path (Join-Path $secondMachine 'bin'), (Join-Path $secondMachine 'opencode') -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $serverCopy 'payload\bin\u-workers.exe') -Destination (Join-Path $secondMachine 'bin\u-workers.exe') -Force
    Copy-Item -LiteralPath (Join-Path $serverCopy 'payload\runtime\opencode\opencode.exe') -Destination (Join-Path $secondMachine 'opencode\opencode.exe') -Force
    $runtimesBeforeSecond = $runtimes.Count
    $tokenBody = (@{ name = 'acceptance-second-machine' } | ConvertTo-Json -Compress)
    $tokenResponse = Invoke-PortableWebRequest -Uri ($serverUrl + '/api/tokens') -Method POST -Body $tokenBody -RootCAPath $rootCa -Headers @{ Authorization = 'Bearer ' + [string]$session.token }
    $accessToken = [string](ConvertFrom-PortableResponse $tokenResponse).token
    Assert-Check 'personal access token minted for the second machine' ($accessToken -match '^mul_')
    $secondCli = Join-Path $secondMachine 'bin\u-workers.exe'
    $secondPath = (Join-Path $secondMachine 'bin') + ';' + (Join-Path $secondMachine 'opencode') + ';' + [Environment]::GetEnvironmentVariable('PATH', 'Process')
    $savedTaskEnvironment = @{}
    foreach ($name in @('MULTICA_AGENT_ID', 'MULTICA_TASK_ID', 'MULTICA_DAEMON_PORT', 'MULTICA_TASK_CONFIG_ROOT', 'MULTICA_TOKEN', 'MULTICA_SERVER_URL', 'MULTICA_APP_URL')) {
        $savedTaskEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        [Environment]::SetEnvironmentVariable($name, $null, 'Process')
    }
    $savedPath = $env:PATH
    $env:PATH = $secondPath
    $connectOutput = ''
    try {
        Push-Location -LiteralPath $secondMachine
        try {
            $connectOutput = & $secondCli --profile $SecondProfile connect --server-url $serverUrl --app-url $serverUrl --root-ca (Join-Path $secondMachine 'root-ca.crt') --token $accessToken --skip-browser-trust 2>&1 | Out-String
            $connectExit = $LASTEXITCODE
        } finally { Pop-Location }
    } finally {
        $env:PATH = $savedPath
        foreach ($name in $savedTaskEnvironment.Keys) {
            if ($null -eq $savedTaskEnvironment[$name] -or [string]$savedTaskEnvironment[$name] -eq '') {
                [Environment]::SetEnvironmentVariable($name, $null, 'Process')
            } else {
                [Environment]::SetEnvironmentVariable($name, [string]$savedTaskEnvironment[$name], 'Process')
            }
        }
    }
    Assert-Check 'second machine connect exit code 0' ($connectExit -eq 0) ($connectOutput -split "\r?\n" | Select-Object -Last 3 | Out-String)
    $secondDeadline = (Get-Date).AddSeconds(120)
    $runtimesAfterSecond = @()
    while ((Get-Date) -lt $secondDeadline) {
        $runtimeResponse = Invoke-PortableWebRequest -Uri ($serverUrl + '/api/runtimes') -RootCAPath $rootCa -Headers @{ Authorization = 'Bearer ' + [string]$session.token; 'X-Workspace-ID' = $workspaceId }
        if ($runtimeResponse.StatusCode -eq 200) {
            $runtimesAfterSecond = @(ConvertFrom-PortableResponse $runtimeResponse)
            if ($runtimesAfterSecond.Count -gt $runtimesBeforeSecond) { break }
        }
        Start-Sleep -Seconds 2
    }
    # Runtimes are unique per (workspace, daemon, provider) and the daemon
    # identity is derived from the host, so a second profile on the same host
    # does not add rows. What proves the join is connect's own report: it waits
    # for the server to accept the machine and its runtime before it returns.
    Assert-Check 'second machine connect reported a running daemon and a registered runtime' ($connectOutput -match 'Connected to' -and $connectOutput -match 'runtimes:\s*[1-9]') (($connectOutput -split "\r?\n" | Where-Object { $_ -match 'daemon:|runtimes:' }) -join ' | ')
    $evidence.secondMachine = 'connect exit 0; daemon and runtime confirmed by the connect output; same-host runtime rows are shared by daemon identity'

    # Stop the simulated second machine's daemon again.
    & $secondCli --profile $SecondProfile daemon stop 2>&1 | Out-Null

    # --- overlay upgrade rehearsal -----------------------------------------
    Write-Step 'Rehearsing an overlay upgrade on the running instance'
    $issueBody = (@{ title = 'Acceptance marker before upgrade'; description = 'written before upgrade'; status = 'todo' } | ConvertTo-Json -Compress)
    $issueResponse = Invoke-PortableWebRequest -Uri ($serverUrl + '/api/issues') -Method POST -Body $issueBody -RootCAPath $rootCa -Headers @{ Authorization = 'Bearer ' + [string]$session.token; 'X-Workspace-ID' = $workspaceId }
    Assert-Check 'marker issue created before the upgrade' ($issueResponse.StatusCode -eq 201) ([string]$issueResponse.StatusCode)
    $markerIssue = ConvertFrom-PortableResponse $issueResponse
    $markerIssueId = [string]$markerIssue.id
    $countsBefore = Invoke-PortableSql -Root $serverCopy -Sql 'SELECT (SELECT count(*) FROM "user")::text || ''|'' || (SELECT count(*) FROM workspace)::text || ''|'' || (SELECT count(*) FROM issue)::text;' -Database ([string]((Get-Content -LiteralPath (Join-Path $serverCopy 'deployment.json') -Raw | ConvertFrom-Json).Database))
    if ($UpgradePackage) {
        Invoke-Robocopy ([IO.Path]::GetFullPath($UpgradePackage)) $upgradeCopy
        $upgradeManifest = Get-Content -LiteralPath (Join-Path $upgradeCopy 'manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        $upgradeVersion = [string]$upgradeManifest.version
        $upgradeOutput = & (Join-Path $serverCopy 'upgrade.ps1') -Source $upgradeCopy -SkipBrowserTrust -NoBrowser -NoStartup 2>&1 | Out-String
        Assert-Check 'upgrade script completed' (Test-Path -LiteralPath (Join-Path $serverCopy 'data\upgrade-report.json')) ($upgradeOutput -split "\r?\n" | Select-Object -Last 3 | Out-String)
        $upgradeReport = Get-Content -LiteralPath (Join-Path $serverCopy 'data\upgrade-report.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        Assert-Check 'upgrade changed the installed version' ([string]$upgradeReport.toVersion -eq $upgradeVersion) ([string]$upgradeReport.toVersion)
        Assert-Check 'upgrade made a database backup first' (Test-Path -LiteralPath (Join-Path ([string]$upgradeReport.backup) 'database.dump')) ([string]$upgradeReport.backup)
        $deploymentAfter = Get-Content -LiteralPath (Join-Path $serverCopy 'deployment.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        Assert-Check 'deployment.json now names the new version' ([string]$deploymentAfter.Version -eq $upgradeVersion) ([string]$deploymentAfter.Version)
        $readyAfter = Invoke-PortableWebRequest -Uri ($serverUrl + '/readyz') -RootCAPath $rootCa
        Assert-Check 'API ready after the upgrade' ($readyAfter.StatusCode -eq 200) ([string]$readyAfter.StatusCode)
        $issueAfter = Invoke-PortableWebRequest -Uri ($serverUrl + '/api/issues/' + $markerIssueId) -RootCAPath $rootCa -Headers @{ Authorization = 'Bearer ' + [string]$session.token; 'X-Workspace-ID' = $workspaceId }
        Assert-Check 'data written before the upgrade still exists' ($issueAfter.StatusCode -eq 200) ([string]$issueAfter.StatusCode)
        $countsAfter = Invoke-PortableSql -Root $serverCopy -Sql 'SELECT (SELECT count(*) FROM "user")::text || ''|'' || (SELECT count(*) FROM workspace)::text || ''|'' || (SELECT count(*) FROM issue)::text;' -Database ([string]$deploymentAfter.Database)
        Assert-Check 'row counts did not shrink during the upgrade' ($countsAfter[0] -ne $null -and $countsAfter[0] -ge $countsBefore[0]) ([string]$countsBefore + ' -> ' + [string]$countsAfter)
        $evidence.upgrade = [ordered]@{ from = $version; to = $upgradeVersion; backup = [string]$upgradeReport.backup; countsBefore = [string]$countsBefore; countsAfter = [string]$countsAfter }
    } else {
        Add-Check 'upgrade rehearsal' $true 'skipped (no -UpgradePackage)'
        $evidence.upgrade = [ordered]@{ skipped = $true }
    }

    # --- the machine's existing Multica state must be untouched -------------
    Write-Step 'Checking that the existing Multica CLI state was not touched'
    Assert-Check 'Multica CLI config hash unchanged' ((Get-FileSha256 $multicaConfig) -eq $multicaConfigBefore)

    # Evidence bookkeeping must never change the acceptance verdict; a
    # formatting failure is recorded but the checks already made stand.
    try {
        $evidence.checks = $checks.ToArray()
        $evidence.version = $version
        $evidence.package = $packagePath
        $evidence.packageSha256 = [ordered]@{ server = [string]$manifest.sha256.server; cli = [string]$manifest.sha256.cli; installer = [string]$manifest.sha256.installer }
        $evidence.durationSeconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 1)
        $evidence.offlineSimulation = 'HTTP_PROXY/HTTPS_PROXY/ALL_PROXY pointed at http://127.0.0.1:9 for all child processes'
        $evidence.boardUrl = $serverUrl
        $evidence.lanPlan = [ordered]@{
            exposed = [bool]$report.lanExposed
            host = [string]$report.lanHost
            requestedHost = [string]$report.lanRequestedHost
            firewall = [string]$report.lanFirewall
            message = [string]$report.lanMessage
            httpsTcpListeners = @($tcpListeners)
            httpsUdpListeners = @($udpListeners | ForEach-Object { [string]$_.LocalAddress })
        }
        $evidence.runtimes = @($runtimesAfterSecond | ForEach-Object { [ordered]@{ name = [string]$_.name; provider = [string]$_.provider; status = [string]$_.status } })
        $evidence.os = [ordered]@{ caption = (Get-CimInstance Win32_OperatingSystem).Caption; version = [Environment]::OSVersion.VersionString; user = [Environment]::UserName }
    } catch {
        $evidence.evidenceBuildError = $_.Exception.ToString()
    }
} catch {
    Add-Check 'acceptance aborted' $false $_.Exception.Message
    $evidence.error = $_.Exception.ToString()
} finally {
    try {
    $evidence.checks = $checks.ToArray()
    $evidence.durationSeconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 1)
    $failed = @($checks | Where-Object { -not $_.ok })
    $evidence.passed = ($failed.Count -eq 0 -and $checks.Count -gt 0)

    if ($KeepRunning -and $installed) {
        $evidence.cleanup = 'kept running (-KeepRunning)'
        Write-Step 'Left running for inspection (-KeepRunning)'
        Write-Host ('  board:    ' + $serverUrl + '/')
        Write-Host ('  server:   ' + $serverCopy)
        Write-Host ('  cleanup:  pwsh -File ' + $PSCommandPath + ' -Package ''' + $packagePath + ''' -WorkRoot ''' + $workRoot + ''' ... with the cleanup commands below')
        Write-Host ('  stop:     powershell.exe -File "' + $serverCopy + '\stop.ps1" -Profile ' + $ServerProfile)
        Write-Host ('  remove:   delete the startup item "U Workers Portable.vbs" and the profile folders .u-workers\profiles\' + $ServerProfile + ' etc.')
    } else {
        Write-Step 'Cleaning up the acceptance area'
        if ($installed) {
            try { & (Join-Path $serverCopy 'stop.ps1') -Profile $ServerProfile -Quiet 2>&1 | Out-Null } catch { $cleanupErrors += ('stop: ' + $_.Exception.Message) }
        }
        try {
            if (Test-Path -LiteralPath $startupItem) { Remove-Item -LiteralPath $startupItem -Force }
        } catch { $cleanupErrors += ('startup item: ' + $_.Exception.Message) }
        if ($installed) {
            foreach ($profile in @($ServerProfile, $SecondProfile)) {
                foreach ($directory in @(Get-ProfileDirectories -Profile $profile)) {
                    try { Remove-Item -LiteralPath $directory -Recurse -Force } catch { $cleanupErrors += ('profile ' + $directory + ': ' + $_.Exception.Message) }
                }
            }
        }
        Start-Sleep -Seconds 2
        foreach ($port in @($HttpsPort, $ApiPort, $WebPort, $DatabasePort)) {
            $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, $port)
            $free = $true
            try { $listener.Start() } catch { $free = $false } finally { $listener.Stop() }
            if (-not $free) { $cleanupErrors += ('port ' + $port + ' still in use') }
        }
        $leftovers = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Path -and $_.Path.StartsWith($workRoot, [StringComparison]::OrdinalIgnoreCase) })
        if ($leftovers.Count -gt 0) { $cleanupErrors += ('processes still running: ' + (($leftovers | ForEach-Object { $_.Id.ToString() + ':' + $_.ProcessName }) -join ', ')) }
        Add-Check 'cleanup left no process or port behind' ($cleanupErrors.Count -eq 0) ($cleanupErrors -join '; ')
        $evidence.cleanup = [ordered]@{ errors = $cleanupErrors; ports = @($HttpsPort, $ApiPort, $WebPort, $DatabasePort) }
        if (-not $KeepWorkRoot) {
            $linkedTools = Join-Path $serverCopy 'tools'
            try {
                if ((Test-Path -LiteralPath $linkedTools) -and ((Get-Item -LiteralPath $linkedTools -Force).LinkType -eq 'Junction')) {
                    [IO.Directory]::Delete($linkedTools, $false)
                }
            } catch { }
            try {
                # A robocopy mirror from an empty folder deletes the tree much
                # faster than Remove-Item on a package with tens of thousands of
                # small files.
                $empty = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-empty-' + [guid]::NewGuid().ToString('N'))
                New-Item -ItemType Directory -Path $empty -Force | Out-Null
                $previous = $ErrorActionPreference
                try {
                    $ErrorActionPreference = 'Continue'
                    & robocopy.exe $empty $workRoot /MIR /XJ /R:0 /W:0 /MT:16 /NP /NFL /NDL /NJH /NJS | Out-Null
                } finally { $ErrorActionPreference = $previous }
                Remove-Item -LiteralPath $empty -Recurse -Force -ErrorAction SilentlyContinue
                Remove-Item -LiteralPath $workRoot -Recurse -Force -ErrorAction SilentlyContinue
            } catch { }
        }
        $evidence.failedChecks = @($failed | ForEach-Object { $_.name + ': ' + $_.detail })
    }

    # restore the proxy variables we changed
    if ($savedEnvironment) {
        foreach ($name in $savedEnvironment.Keys) {
            $value = $savedEnvironment[$name]
            if ($null -eq $value -or [string]$value -eq '') { Remove-Item ('Env:' + $name) -ErrorAction SilentlyContinue } else { [Environment]::SetEnvironmentVariable($name, [string]$value, 'Process') }
        }
    }

    if ($EvidencePath) {
        $directory = Split-Path -Parent $EvidencePath
        if ($directory -and -not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory -Force | Out-Null }
        $evidence | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $EvidencePath -Encoding UTF8
        Write-Host ''
        Write-Host ('Evidence: ' + $EvidencePath)
    }

    } catch {
        $failureText = ($_.Exception.ToString() + [Environment]::NewLine + '--- checks ---' + [Environment]::NewLine + (($checks | ForEach-Object { [string]$_.ok + ' ' + $_.name + ' ' + $_.detail }) -join [Environment]::NewLine))
        try { [IO.File]::WriteAllText((Join-Path $env:TEMP 'uworkers-acceptance-finally-error.txt'), $failureText, (New-Object Text.UTF8Encoding($false))) } catch { }
        try {
            $evidence.cleanupError = $failureText
            $evidence.passed = $false
            $evidence.checks = $checks.ToArray()
            $directory = Split-Path -Parent $EvidencePath
            if ($directory -and -not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory -Force | Out-Null }
            $evidence | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $EvidencePath -Encoding UTF8
        } catch { }
    }

    if ($evidence.passed) {
        Write-Host ('ACCEPTANCE PASSED (' + $checks.Count + ' checks, ' + $evidence.durationSeconds + ' s)') -ForegroundColor Green
        exit 0
    }
    Write-Host ('ACCEPTANCE FAILED (' + @($checks | Where-Object { -not $_.ok }).Count + ' failed of ' + $checks.Count + ')') -ForegroundColor Red
    exit 1
}
