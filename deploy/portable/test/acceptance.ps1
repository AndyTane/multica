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
       daemon;
    4. rebuilds the installer from the bundled toolchain while still offline;
    5. simulates a second machine with the packaged CLI plus a separate CLI
       profile, joins it with 'connect', and observes a new runtime;
    6. rehearses an overlay upgrade from a newer package: backup exists, version
       changed, and the data written before the upgrade is still there;
    7. cleans up: stops the service, removes the start-on-login item and the
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
    [switch]$ResumeExisting
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
        Invoke-Robocopy $packagePath $serverCopy -ExcludeDirectories @('tools')
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

    # --- installer artifact + offline rebuild -------------------------------
    Write-Step 'Checking the prebuilt installer and rebuilding it offline'
    $installer = Get-ChildItem -LiteralPath (Join-Path $serverCopy 'payload\installer') -Filter 'u-workers-setup-*.exe' | Select-Object -First 1
    Assert-Check 'prebuilt installer shipped in the package' ($null -ne $installer -and $installer.Length -gt 10MB) ([string]$installer.Length)
    $help = & $installer.FullName --help 2>&1 | Out-String
    Assert-Check 'prebuilt installer starts and prints usage' ($LASTEXITCODE -eq 0 -and $help -match 'u-workers-setup')
    if (-not $SkipInstallerBuild -and [bool]$manifest.buildKit) {
        $rebuildDir = Join-Path $workRoot 'installer-rebuild'
        New-Item -ItemType Directory -Path $rebuildDir -Force | Out-Null
        $env:GOPROXY = 'off'
        $rebuild = & (Join-Path $serverCopy 'build-installer.ps1') -Version $version -OutputDir $rebuildDir 2>&1 | Out-String
        $rebuilt = Get-ChildItem -LiteralPath $rebuildDir -Filter '*.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
        Assert-Check 'installer rebuilt offline from the bundled toolchain' ($null -ne $rebuilt -and $rebuilt.Length -gt 10MB) ($rebuild -split "\r?\n" | Select-Object -Last 2 | Out-String)
    } elseif (-not $SkipInstallerBuild) {
        Add-Check 'installer rebuild' $false 'the package was built without -SkipBuildKit: no go toolchain bundled'
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
    Assert-Check 'second machine added a runtime to the board' ($runtimesAfterSecond.Count -gt $runtimesBeforeSecond) (($runtimesAfterSecond | ForEach-Object { [string]$_.name + '/' + [string]$_.provider }) -join ', ')

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

    $evidence.checks = @($checks)
    $evidence.version = $version
    $evidence.package = $packagePath
    $evidence.packageSha256 = [ordered]@{ server = [string]$manifest.sha256.server; cli = [string]$manifest.sha256.cli; installer = [string]$manifest.sha256.installer }
    $evidence.installSeconds = [math]::Round(((Get-ChildItem -LiteralPath (Join-Path $serverCopy 'data\install-report.json')).LastWriteTime - $started).TotalSeconds, 1)
    $evidence.offlineSimulation = 'HTTP_PROXY/HTTPS_PROXY/ALL_PROXY pointed at http://127.0.0.1:9 for all child processes'
    $evidence.heap = $null
    $evidence.boardUrl = $serverUrl
    $evidence.runtimes = @($runtimesAfterSecond | ForEach-Object { [ordered]@{ name = [string]$_.name; provider = [string]$_.provider; status = [string]$_.status } })
    $evidence.os = [ordered]@{ caption = (Get-CimInstance Win32_OperatingSystem).Caption; version = [Environment]::OSVersion.VersionString; user = [Environment]::UserName }
} catch {
    Add-Check 'acceptance aborted' $false $_.Exception.Message
    $evidence.error = $_.Exception.ToString()
} finally {
    try {
    $evidence.checks = @($checks)
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
            try { Remove-Item -LiteralPath $workRoot -Recurse -Force } catch { }
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
            $evidence.checks = @($checks)
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
