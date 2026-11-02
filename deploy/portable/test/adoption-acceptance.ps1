# R1-1 acceptance: adopt a real, isolated native deployment (database with a
# user, a workspace and an issue, uploads and its generated secrets) into a fresh
# copy of the fixed package, without administrator rights and without touching any
# existing deployment on the machine. The test drives the real scripts, server,
# PostgreSQL and HTTPS entry, then removes everything it created.
# Windows PowerShell 5.1 compatible.
# Run: powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\adoption-acceptance.ps1 -NewPackage <fixed package> [-OldPackage <pre-existing package>]
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$NewPackage,
    [string]$OldPackage = '',
    [string]$WorkRoot = '',
    [int]$OldHttpsPort = 25443,
    [int]$OldApiPort = 29080,
    [int]$OldWebPort = 24000,
    [int]$OldDatabasePort = 27433,
    [int]$NewHttpsPort = 24443,
    [int]$NewApiPort = 28081,
    [int]$NewWebPort = 23001,
    [int]$NewDatabasePort = 26434,
    [switch]$KeepWorkRoot
)

$ErrorActionPreference = 'Stop'
$portableRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $portableRoot 'portable-common.ps1')

if (-not $OldPackage) { $OldPackage = $NewPackage }
$NewPackage = [IO.Path]::GetFullPath($NewPackage)
$OldPackage = [IO.Path]::GetFullPath($OldPackage)
if (-not $WorkRoot) { $WorkRoot = Join-Path ([IO.Path]::GetTempPath()) 'uworkers-r1-adoption' }
$WorkRoot = [IO.Path]::GetFullPath($WorkRoot)
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
if (-not $WorkRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and -not $WorkRoot.StartsWith('C:\UWorkers\', [StringComparison]::OrdinalIgnoreCase)) {
    throw ('The adoption acceptance work root must be a scratch directory: ' + $WorkRoot)
}

$checks = 0
$failed = 0
function Add-Check {
    param([string]$Name, [bool]$Ok, [string]$Detail = '')
    $script:checks++
    if ($Ok) {
        Write-Host ('  [PASS] ' + $Name + $(if ($Detail) { '  ' + $Detail } else { '' }))
    } else {
        $script:failed++
        Write-Host ('  [FAIL] ' + $Name + '  ' + $Detail) -ForegroundColor Red
    }
}

function Copy-Package {
    param([string]$Source, [string]$Destination)
    $empty = New-Item -ItemType Directory -Path (Join-Path $env:TEMP ('uworkers-empty-' + [guid]::NewGuid().ToString('N'))) -Force
    & robocopy.exe $Source $Destination /E /XD (Join-Path $Source 'tools') /R:1 /W:1 /MT:8 /NP /NFL /NDL /NJH /NJS | Out-Null
    $code = $LASTEXITCODE
    Remove-Item -LiteralPath $empty.FullName -Recurse -Force -ErrorAction SilentlyContinue
    if ($code -ge 8) { throw ('Copying the package failed (robocopy exit ' + $code + ').') }
}

$oldRoot = Join-Path $WorkRoot 'old-native'
$newRoot = Join-Path $WorkRoot 'new-package'
$started = Get-Date
$oldStarted = $false
$newStarted = $false
try {
    Write-Host '==> Preparing an isolated adoption acceptance area' -ForegroundColor Cyan
    if (Test-Path -LiteralPath $WorkRoot) { Remove-Item -LiteralPath $WorkRoot -Recurse -Force }
    New-Item -ItemType Directory -Path $WorkRoot -Force | Out-Null
    foreach ($port in @($OldHttpsPort, $OldApiPort, $OldWebPort, $OldDatabasePort, $NewHttpsPort, $NewApiPort, $NewWebPort, $NewDatabasePort)) {
        Add-Check ('port ' + $port + ' is free before the test') (Test-PortablePortFree $port)
    }
    Copy-Package $OldPackage $oldRoot
    Add-Check 'old package copied to a clean folder' (Test-Path -LiteralPath (Join-Path $oldRoot 'install.ps1'))
    Copy-Package $NewPackage $newRoot
    Add-Check 'fixed package copied to a clean folder' (Test-Path -LiteralPath (Join-Path $newRoot 'install.ps1'))

    Write-Host '==> Installing the old native deployment (real server, PostgreSQL and HTTPS)' -ForegroundColor Cyan
    & (Join-Path $oldRoot 'install.ps1') -PublicHost '127.0.0.1' -HttpsPort $OldHttpsPort -ApiPort $OldApiPort -WebPort $OldWebPort -DatabasePort $OldDatabasePort -Profile 'uworkers-r1-old' -SkipBrowserTrust -SkipDaemon -NoBrowser -NoStartup
    if ($LASTEXITCODE -ne 0) { throw ('Creating the old deployment failed (exit ' + $LASTEXITCODE + ').') }
    $oldStarted = $true
    $oldReport = Read-PortableJson (Join-Path $oldRoot 'data\install-report.json')
    Add-Check 'old deployment installed and reports ready' ($null -ne $oldReport)
    $oldConfig = Get-PortableDeployment $oldRoot
    $oldJwt = [string]$oldConfig.BackendEnvironment.JWT_SECRET
    Add-Check 'old deployment has a generated JWT secret to preserve' ([bool]$oldJwt)

    Write-Host '==> Seeding the old deployment with a user-visible issue and an upload' -ForegroundColor Cyan
    $rootCA = Join-Path $oldRoot 'root-ca.crt'
    $credentials = @(Get-Content -LiteralPath (Join-Path $oldRoot 'data\local-login.txt') -Encoding UTF8)
    $oldServerUrl = 'https://127.0.0.1:' + $OldHttpsPort
    $loginBody = (@{ username = [string]$credentials[0]; password = [string]$credentials[1] } | ConvertTo-Json -Compress)
    $login = Invoke-PortableWebRequest -Uri ($oldServerUrl + '/auth/password') -Method POST -Body $loginBody -RootCAPath $rootCA
    if ($login.StatusCode -ne 200) { throw ('Old deployment sign-in failed (' + $login.StatusCode + ').') }
    $session = ConvertFrom-PortableResponse $login
    $workspaceId = [string]$oldReport.workspaceId
    if (-not $workspaceId) { throw 'The old install report has no workspace id.' }
    $issueTitle = 'R1-01 adoption marker ' + (Get-Date -Format 'HHmmss')
    $issueBody = (@{ title = $issueTitle; description = 'Created before adoption; must survive it.' } | ConvertTo-Json -Compress)
    $headers = @{ Authorization = 'Bearer ' + [string]$session.token; 'X-Workspace-ID' = $workspaceId }
    $created = Invoke-PortableWebRequest -Uri ($oldServerUrl + '/api/issues') -Method POST -Body $issueBody -RootCAPath $rootCA -Headers $headers
    Add-Check 'old deployment has a workspace and a new issue in it' ($created.StatusCode -eq 200 -or $created.StatusCode -eq 201) ([string]$created.StatusCode)
    $uploadDir = Join-Path $oldRoot 'uploads'
    New-PortableDirectory $uploadDir
    [IO.File]::WriteAllText((Join-Path $uploadDir 'r1-upload-marker.txt'), 'uploaded before adoption')
    $oldWorkspaceName = ''
    $workspaces = ConvertFrom-PortableResponse (Invoke-PortableWebRequest -Uri ($oldServerUrl + '/api/workspaces') -RootCAPath $rootCA -Headers $headers)
    if (@($workspaces).Count -gt 0) { $oldWorkspaceName = [string](@($workspaces)[0].name) }
    Add-Check 'old deployment has a workspace name to preserve' ([bool]$oldWorkspaceName) $oldWorkspaceName

    Write-Host '==> Adopting the old deployment into the fresh fixed package' -ForegroundColor Cyan
    & (Join-Path $newRoot 'install.ps1') -PublicHost '127.0.0.1' -AdoptRoot $oldRoot -HttpsPort $NewHttpsPort -ApiPort $NewApiPort -WebPort $NewWebPort -DatabasePort $NewDatabasePort -Profile 'uworkers-r1-new' -SkipBrowserTrust -SkipDaemon -NoBrowser -NoStartup
    if ($LASTEXITCODE -ne 0) { throw ('Adoption failed (exit ' + $LASTEXITCODE + ').') }
    $newStarted = $true
    $newReport = Read-PortableJson (Join-Path $newRoot 'data\install-report.json')
    Add-Check 'adoption completed and wrote its report' ($null -ne $newReport -and [string]$newReport.mode -eq 'adopted')
    Add-Check 'adoption recorded the old backup location' ([bool][string]$newReport.backup) ([string]$newReport.backup)

    Write-Host '==> Verifying the adopted deployment' -ForegroundColor Cyan
    $newConfig = Get-PortableDeployment $newRoot
    Add-Check 'the new deployment keeps the old JWT secret' ([string]$newConfig.BackendEnvironment.JWT_SECRET -eq $oldJwt)
    Add-Check 'the new deployment keeps the old database credentials' ([string]$newConfig.DatabaseUser -eq [string]$oldConfig.DatabaseUser)
    Add-Check 'the new deployment version is the package version' ([string]$newConfig.Version -eq [string](Get-PortableManifest $newRoot).version) ([string]$newConfig.Version)
    $backupDirectory = [string]$newReport.backup
    if ($backupDirectory -and -not (Test-Path -LiteralPath $backupDirectory)) { $backupDirectory = (Split-Path -Parent $backupDirectory) }
    $backups = @(Get-ChildItem -LiteralPath (Join-Path $oldRoot 'backups') -Directory -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending)
    $latestBackup = ''
    if ($backups.Count -gt 0) { $latestBackup = $backups[0].FullName }
    Add-Check 'the old deployment has a locatable backup' ([bool]$latestBackup) $latestBackup
    $backupDump = ''
    if ($latestBackup) { $backupDump = @(Get-ChildItem -LiteralPath $latestBackup -Filter '*.dump' -File -ErrorAction SilentlyContinue)[0].FullName }
    Add-Check 'the backup contains the database dump' ([bool]$backupDump) $backupDump
    $newRootCA = Join-Path $newRoot 'root-ca.crt'
    $newServerUrl = 'https://127.0.0.1:' + $NewHttpsPort
    Wait-PortableHttp -Uri ($newServerUrl + '/readyz') -RootCAPath $newRootCA -TimeoutSec 90 -Description 'the adopted API' | Out-Null
    Add-Check 'the adopted API answers over the new HTTPS entry' $true
    $newLoginBody = (@{ username = [string]$credentials[0]; password = [string]$credentials[1] } | ConvertTo-Json -Compress)
    $newLogin = Invoke-PortableWebRequest -Uri ($newServerUrl + '/auth/password') -Method POST -Body $newLoginBody -RootCAPath $newRootCA
    Add-Check 'the old account still signs in on the adopted deployment' ($newLogin.StatusCode -eq 200) ([string]$newLogin.StatusCode)
    $newSession = ConvertFrom-PortableResponse $newLogin
    $newHeaders = @{ Authorization = 'Bearer ' + [string]$newSession.token; 'X-Workspace-ID' = $workspaceId }
    $issues = ConvertFrom-PortableResponse (Invoke-PortableWebRequest -Uri ($newServerUrl + '/api/issues') -RootCAPath $newRootCA -Headers $newHeaders)
    # GET /api/issues wraps the rows in an "issues" object.
    $titles = @(@($issues.issues) | ForEach-Object { [string]$_.title })
    Add-Check 'the issue created before the adoption still exists' ($titles -contains $issueTitle) $issueTitle
    $newWorkspaces = ConvertFrom-PortableResponse (Invoke-PortableWebRequest -Uri ($newServerUrl + '/api/workspaces') -RootCAPath $newRootCA -Headers $newHeaders)
    $workspaceNames = @(@($newWorkspaces) | ForEach-Object { [string]$_.name })
    Add-Check 'the old workspace still exists' ($workspaceNames -contains $oldWorkspaceName) $oldWorkspaceName
    Add-Check 'the old upload was copied into the adopted deployment' (Test-Path -LiteralPath (Join-Path $newRoot 'uploads\r1-upload-marker.txt'))
    Add-Check 'the old service ports are no longer listening' ((Test-PortablePortFree $OldHttpsPort) -and (Test-PortablePortFree $OldApiPort))
    Add-Check 'the adopted deployment uses the new ports' ((Test-PortablePortListening $NewHttpsPort) -and (Test-PortablePortListening $NewApiPort))
    Add-Check 'the old deployment folder still exists untouched' ((Test-Path -LiteralPath (Join-Path $oldRoot 'deployment.json')) -and (Test-Path -LiteralPath (Join-Path $oldRoot 'data\postgres\PG_VERSION')))
} catch {
    Add-Check 'adoption acceptance aborted' $false $_.Exception.Message
} finally {
    Write-Host '==> Cleaning up the adoption acceptance area' -ForegroundColor Cyan
    if ($newStarted) { try { & (Join-Path $newRoot 'stop.ps1') -Profile 'uworkers-r1-new' | Out-Null } catch { Write-Host ('  stop new: ' + $_.Exception.Message) -ForegroundColor Yellow } }
    if ($oldStarted) {
        $oldRunner = Join-Path $oldRoot 'runtime\native-windows.ps1'
        if (Test-Path -LiteralPath $oldRunner) {
            $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
            try { Invoke-PortableCommand -Executable $shell -Arguments @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $oldRunner, '-Action', 'Stop', '-Root', $oldRoot) | Out-Null } catch { Write-Host ('  stop old: ' + $_.Exception.Message) -ForegroundColor Yellow }
        }
    }
    # A test component may still own a process even after its stop script ran.
    Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Path -and $_.Path.StartsWith($WorkRoot + '\', [StringComparison]::OrdinalIgnoreCase) } | ForEach-Object { Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Seconds 3
    $portsStillBusy = @()
    foreach ($port in @($OldHttpsPort, $OldApiPort, $OldWebPort, $OldDatabasePort, $NewHttpsPort, $NewApiPort, $NewWebPort, $NewDatabasePort)) {
        if (-not (Test-PortablePortFree $port)) { $portsStillBusy += $port }
    }
    Add-Check 'cleanup left no test port listening' ($portsStillBusy.Count -eq 0) (($portsStillBusy | ForEach-Object { [string]$_ }) -join ',')
    $startup = Join-Path ([Environment]::GetFolderPath('Startup')) 'U Workers Portable.vbs'
    Add-Check 'cleanup left no start-on-login item behind' (-not (Test-Path -LiteralPath $startup))
    if (-not $KeepWorkRoot) {
        try {
            $empty = Join-Path $env:TEMP ('uworkers-empty-' + [guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path $empty -Force | Out-Null
            & robocopy.exe $empty $WorkRoot /MIR /XJ /R:0 /W:0 /MT:16 /NP /NFL /NDL /NJH /NJS | Out-Null
            Remove-Item -LiteralPath $empty -Recurse -Force -ErrorAction SilentlyContinue
            Remove-Item -LiteralPath $WorkRoot -Recurse -Force -ErrorAction SilentlyContinue
        } catch { }
    }
    Add-Check 'cleanup removed the scratch area' (-not (Test-Path -LiteralPath $WorkRoot))
    $duration = [math]::Round(((Get-Date) - $started).TotalSeconds, 1)
    Write-Host ''
    if ($failed -eq 0) {
        Write-Host ('ADOPTION ACCEPTANCE PASSED (' + $checks + ' checks, ' + $duration + ' s)') -ForegroundColor Green
        exit 0
    }
    Write-Host ('ADOPTION ACCEPTANCE FAILED (' + $failed + ' failed of ' + $checks + ')') -ForegroundColor Red
    exit 1
}
