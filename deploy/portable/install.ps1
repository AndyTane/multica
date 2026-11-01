#Requires -Version 5.1
<#
.SYNOPSIS
    One-step installer for the U Workers portable package.

.DESCRIPTION
    Turns this folder into a running U Workers server: PostgreSQL, the Go API,
    the web app, LAN HTTPS with a private root CA, the desktop login account and
    a local opencode runtime all come from the package itself. Nothing is
    downloaded, nothing needs an administrator account, and every file the
    server writes stays inside this folder.

    Windows PowerShell 5.1 compatible: a fresh Windows machine is the target.

.PARAMETER PublicHost
    Address other machines use to reach this server. Defaults to this machine's
    first LAN IPv4 address. Only this address (plus localhost) is put in the
    generated certificate.

.PARAMETER HttpsPort
    LAN HTTPS entry point. Default 13443.

.PARAMETER ApiPort
    Loopback API port. Default 18080.

.PARAMETER WebPort
    Loopback web port. Default 13000.

.PARAMETER DatabasePort
    Loopback PostgreSQL port. Default 16433.

.PARAMETER Profile
    U Workers CLI profile that owns the local daemon. Default 'portable' keeps
    this package's daemon separate from any other CLI install on the machine.

.PARAMETER AdoptRoot
    Path to an existing native Windows U Workers deployment to take over. The
    existing database and uploads are copied (the old deployment is backed up
    and stopped first) and the existing accounts, workspaces and data are kept.

.PARAMETER OperatorEmail
    After adopting an existing deployment, mint the local daemon token for this
    account. Defaults to the sole workspace owner.

.PARAMETER SkipBrowserTrust
    Do not add the package root CA to the current Windows user's certificate
    store. The CLI and daemon still trust it; the browser will warn once.

.PARAMETER SkipDaemon
    Do not start the local agent daemon (server only).

.PARAMETER NoBrowser
    Do not open the browser at the end.

.PARAMETER NoStartup
    Do not register the current user's start-on-login item.

.PARAMETER Force
    Continue even when a deployment already exists in this folder (starts it
    instead of reinstalling).

.EXAMPLE
    Setup.cmd

.EXAMPLE
    .\install.ps1 -AdoptRoot D:\UWorkers\clawsta80\native -OperatorEmail tane@example.com
#>
[CmdletBinding()]
param(
    [string]$PublicHost,
    [int]$HttpsPort = 13443,
    [int]$ApiPort = 18080,
    [int]$WebPort = 13000,
    [int]$DatabasePort = 16433,
    [string]$Profile = 'portable',
    [string]$AdoptRoot,
    [string]$OperatorEmail,
    [switch]$SkipBrowserTrust,
    [switch]$SkipDaemon,
    [switch]$NoBrowser,
    [switch]$NoStartup,
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
. (Join-Path $root 'portable-common.ps1')

function Write-Step {
    param([string]$Text)
    Write-Host ''
    Write-Host ('==> ' + $Text) -ForegroundColor Cyan
}

function Invoke-PortablePasswordInput {
    param([string]$Executable, [string[]]$Arguments, [string]$Password)
    # Passwords are never put on a command line: local-auth reads one line from
    # stdin. A short-lived file under state\ is the only way PowerShell 5.1 can
    # redirect stdin, and it is removed again before the call returns.
    $stateDirectory = Join-Path $root 'state'
    New-PortableDirectory $stateDirectory
    $inputFile = Join-Path $stateDirectory ('credential-input-' + [guid]::NewGuid().ToString('N') + '.txt')
    [IO.File]::WriteAllText($inputFile, $Password + [Environment]::NewLine, (New-Object Text.UTF8Encoding($false)))
    try {
        $options = @{
            FilePath = $Executable
            ArgumentList = (ConvertTo-PortableArgumentLine $Arguments)
            RedirectStandardInput = $inputFile
            RedirectStandardOutput = Join-Path $stateDirectory 'local-auth.out.log'
            RedirectStandardError = Join-Path $stateDirectory 'local-auth.err.log'
            NoNewWindow = $true
            PassThru = $true
            Wait = $true
        }
        $process = Start-Process @options
        $output = ''
        foreach ($name in @('local-auth.out.log', 'local-auth.err.log')) {
            $path = Join-Path $stateDirectory $name
            if (Test-Path -LiteralPath $path) {
                $output += (Get-Content -LiteralPath $path -Raw -ErrorAction SilentlyContinue)
                Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
            }
        }
        return [pscustomobject]@{ ExitCode = $process.ExitCode; Output = [string]$output }
    } finally {
        Remove-Item -LiteralPath $inputFile -Force -ErrorAction SilentlyContinue
    }
}

function Invoke-PortableSqlFile {
    param([string]$Sql, [string]$Database = 'postgres')
    $config = Get-PortableDeployment $root
    $stateDirectory = Join-Path $root 'state'
    New-PortableDirectory $stateDirectory
    $sqlPath = Join-Path $stateDirectory ('sql-' + [guid]::NewGuid().ToString('N') + '.sql')
    [IO.File]::WriteAllText($sqlPath, $Sql, (New-Object Text.UTF8Encoding($false)))
    $psql = Join-Path (Get-PortablePostgresBin $root) 'psql.exe'
    $env:PGPASSWORD = [string]$config.DatabasePassword
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $psql -h 127.0.0.1 -p ([int]$config.DatabasePort) -U ([string]$config.DatabaseUser) -d $Database -X -A -t -v ON_ERROR_STOP=1 -f $sqlPath 2>&1 | Out-String
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
        Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $sqlPath -Force -ErrorAction SilentlyContinue
    }
    if ($exitCode -ne 0) { throw ('Database step failed: ' + $output.Trim()) }
    return @($output -split "\r?\n" | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ -ne '' })
}

function Get-PortableIdentity {
    param([string]$Email, [string]$DisplayName)
    $sql = @'
BEGIN;
DO $do$
DECLARE
  existing_id uuid;
  existing_name text;
BEGIN
  SELECT id, name INTO existing_id, existing_name FROM "user" WHERE lower(email) = lower('__EMAIL__');
  IF existing_id IS NOT NULL THEN
    IF existing_name IS DISTINCT FROM '__NAME__' THEN
      RAISE EXCEPTION 'existing identity has a different display name';
    END IF;
  ELSE
    INSERT INTO "user" (name, email) VALUES ('__NAME__', '__EMAIL__') RETURNING id INTO existing_id;
  END IF;
END
$do$;
COMMIT;
SELECT id::text FROM "user" WHERE lower(email) = lower('__EMAIL__');
'@
    $sql = $sql.Replace('__EMAIL__', $Email).Replace('__NAME__', $DisplayName)
    $rows = Invoke-PortableSqlFile $sql
    $identityRows = @($rows | Where-Object { $_ -match '^[0-9a-fA-F-]{36}$' })
    if ($identityRows.Count -eq 0) { throw 'Could not create or find the local operator account.' }
    return [string]$identityRows[$identityRows.Count - 1]
}

function Get-PortableSavedCredential {
    $path = Join-Path $root 'data\local-login.txt'
    if (-not (Test-Path -LiteralPath $path)) { return $null }
    $lines = @(Get-Content -LiteralPath $path -Encoding UTF8)
    if ($lines.Count -lt 2) { return $null }
    return [pscustomobject]@{ Username = ([string]$lines[0]).Trim(); Password = ([string]$lines[1]).Trim() }
}

function Save-PortableCredential {
    param([string]$Username, [string]$Password)
    $path = Join-Path $root 'data\local-login.txt'
    New-PortableDirectory (Split-Path -Parent $path)
    [IO.File]::WriteAllText($path, $Username + [Environment]::NewLine + $Password + [Environment]::NewLine, (New-Object Text.UTF8Encoding($false)))
    return $path
}

function Initialize-PortableLocalCredential {
    param([string]$UserId, [string]$Username, [string]$Password)
    $existing = Invoke-PortableSql "SELECT username_normalized FROM local_password_credential WHERE user_id = '" + $UserId + "';" ([string](Get-PortableDeployment $root).Database)
    $tool = Join-Path ([string](Get-PortableDeployment $root).Release) 'local-auth.exe'
    if (-not (Test-Path -LiteralPath $tool)) { throw ('local-auth.exe is missing from the staged release: ' + $tool) }
    $deploymentConfig = Join-Path $root 'deployment.json'
    if ($existing.Count -gt 0) {
        $result = Invoke-PortablePasswordInput -Executable $tool -Arguments @('reset', '--username', $Username, '--deployment-config', $deploymentConfig, '--password-stdin') -Password $Password
        if ($result.ExitCode -ne 0) { throw ('Could not reset the local password: ' + ([string]$result.Output).Trim()) }
        return $true
    }
    $result = Invoke-PortablePasswordInput -Executable $tool -Arguments @('create', '--user-id', $UserId, '--username', $Username, '--deployment-config', $deploymentConfig, '--password-stdin') -Password $Password
    if ($result.ExitCode -ne 0) { throw ('Could not create the local password: ' + ([string]$result.Output).Trim()) }
    return $false
}

function Invoke-PortablePasswordLogin {
    param([string]$ServerUrl, [string]$RootCAPath, [string]$Username, [string]$Password)
    $body = (@{ username = $Username; password = $Password } | ConvertTo-Json -Compress)
    $response = Invoke-PortableWebRequest -Uri ($ServerUrl + '/auth/password') -Method POST -Body $body -RootCAPath $RootCAPath
    if ($response.StatusCode -ne 200) { throw ('Local sign-in failed (' + $response.StatusCode + '): ' + $response.Content) }
    return (ConvertFrom-PortableResponse $response)
}

function Get-PortableWorkspaces {
    param([string]$ServerUrl, [string]$RootCAPath, [string]$Token)
    $response = Invoke-PortableWebRequest -Uri ($ServerUrl + '/api/workspaces') -RootCAPath $RootCAPath -Headers @{ Authorization = 'Bearer ' + $Token }
    if ($response.StatusCode -ne 200) { throw ('Listing workspaces failed (' + $response.StatusCode + '): ' + $response.Content) }
    return @(ConvertFrom-PortableResponse $response)
}

function New-PortableWorkspace {
    param([string]$ServerUrl, [string]$RootCAPath, [string]$Token, [string]$Name, [string]$Slug)
    $body = (@{ name = $Name; slug = $Slug } | ConvertTo-Json -Compress)
    $response = Invoke-PortableWebRequest -Uri ($ServerUrl + '/api/workspaces') -Method POST -Body $body -RootCAPath $RootCAPath -Headers @{ Authorization = 'Bearer ' + $Token }
    if ($response.StatusCode -ne 201) { throw ('Creating the workspace failed (' + $response.StatusCode + '): ' + $response.Content) }
    return (ConvertFrom-PortableResponse $response)
}

function Complete-PortableOnboarding {
    param([string]$ServerUrl, [string]$RootCAPath, [string]$Token, [string]$WorkspaceId)
    $body = (@{ workspace_id = $WorkspaceId } | ConvertTo-Json -Compress)
    $response = Invoke-PortableWebRequest -Uri ($ServerUrl + '/api/me/onboarding/complete') -Method POST -Body $body -RootCAPath $RootCAPath -Headers @{ Authorization = 'Bearer ' + $Token }
    if ($response.StatusCode -ne 200) { throw ('Completing onboarding failed (' + $response.StatusCode + '): ' + $response.Content) }
}

function New-PortableAccessToken {
    param([string]$ServerUrl, [string]$RootCAPath, [string]$SessionToken)
    $body = (@{ name = 'portable-daemon' } | ConvertTo-Json -Compress)
    $response = Invoke-PortableWebRequest -Uri ($ServerUrl + '/api/tokens') -Method POST -Body $body -RootCAPath $RootCAPath -Headers @{ Authorization = 'Bearer ' + $SessionToken }
    if ($response.StatusCode -ne 201) { throw ('Creating the daemon token failed (' + $response.StatusCode + '): ' + $response.Content) }
    $created = ConvertFrom-PortableResponse $response
    if (-not $created.token) { throw 'The server returned no daemon token.' }
    return [string]$created.token
}

function New-PortableUserAccessToken {
    param([string]$UserId, [string]$TokenName)
    # Host-local token mint for an account the installer cannot sign in as
    # (an adopted deployment with an unknown password). It writes the same row
    # the server's POST /api/tokens writes: SHA-256 of the raw mul_ token.
    $bytes = New-Object byte[] 20
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    $raw = 'mul_' + (($bytes | ForEach-Object { $_.ToString('x2') }) -join '')
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $hash = (($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($raw))) | ForEach-Object { $_.ToString('x2') }) -join '' } finally { $sha.Dispose() }
    $prefix = $raw.Substring(0, 12)
    $config = Get-PortableDeployment $root
    $sql = "INSERT INTO personal_access_token (user_id, name, token_hash, token_prefix) VALUES ('" + $UserId + "', '" + ($TokenName -replace "'", "''") + "', '" + $hash + "', '" + $prefix + "') RETURNING token_prefix;"
    $rows = Invoke-PortableSql $sql ([string]$config.Database)
    if ($rows.Count -eq 0) { throw 'Could not mint the local daemon token.' }
    return $raw
}

function Get-PortableOwnerUserId {
    param([string]$Email)
    $config = Get-PortableDeployment $root
    if ($Email) {
        $rows = Invoke-PortableSql ("SELECT id::text FROM ""user"" WHERE lower(email) = lower('" + ($Email -replace "'", "''") + "');") ([string]$config.Database)
        if ($rows.Count -eq 0) { throw ('No account matches -OperatorEmail ' + $Email + '.') }
        return [string]$rows[$rows.Count - 1]
    }
    $rows = Invoke-PortableSql "SELECT m.user_id::text FROM member m JOIN workspace w ON m.workspace_id = w.id WHERE m.role = 'owner' ORDER BY w.created_at ASC LIMIT 1;" ([string]$config.Database)
    if ($rows.Count -eq 0) { throw 'Adopted deployment has no workspace owner to attach the local daemon to. Pass -OperatorEmail.' }
    return [string]$rows[0]
}

function Get-PortableRuntimeSnapshot {
    param([string]$ServerUrl, [string]$RootCAPath, [string]$Token, [string]$WorkspaceId)
    $response = Invoke-PortableWebRequest -Uri ($ServerUrl + '/api/runtimes') -RootCAPath $RootCAPath -Headers @{ Authorization = 'Bearer ' + $Token; 'X-Workspace-ID' = $WorkspaceId }
    if ($response.StatusCode -ne 200) { return @() }
    $runtimes = @(ConvertFrom-PortableResponse $response)
    return $runtimes
}

function Start-PortableDaemonConnection {
    param([string]$ServerUrl, [string]$RootCAPath, [string]$AccessToken)
    $cli = Get-PortableCliPath $root
    $arguments = @('--profile', $Profile, 'connect', '--server-url', $ServerUrl, '--app-url', $ServerUrl, '--root-ca', $RootCAPath, '--token', $AccessToken, '--skip-browser-trust')
    $result = Invoke-PortableCli -Root $root -Arguments $arguments
    if ($result.ExitCode -ne 0) {
        throw ('Connecting the local daemon failed: ' + ([string]$result.Output).Trim() + '  Fix the error and run Start.cmd.')
    }
    return $result.Output
}

function Start-PortableRuntimeAndWait {
    param([string]$ServerUrl, [string]$RootCAPath, [string]$Token, [string]$WorkspaceId)
    # The daemon registers runtimes a few seconds after it starts; give it up to
    # two minutes so the report only claims what the server actually shows.
    $deadline = (Get-Date).AddSeconds(120)
    $snapshot = @()
    while ((Get-Date) -lt $deadline) {
        try { $snapshot = @(Get-PortableRuntimeSnapshot -ServerUrl $ServerUrl -RootCAPath $RootCAPath -Token $Token -WorkspaceId $WorkspaceId) } catch { $snapshot = @() }
        if ($snapshot.Count -gt 0) { break }
        Start-Sleep -Seconds 2
    }
    return $snapshot
}

function Invoke-PortableStageRuntime {
    Write-Step 'Staging the bundled server runtime'
    $runtime = Join-Path $root 'runtime'
    New-PortableDirectory $runtime
    $postgresSource = Join-Path $root 'payload\runtime\pgsql'
    $postgresTarget = Join-Path $runtime 'pgsql'
    if (-not (Test-Path -LiteralPath (Join-Path $postgresTarget 'bin\psql.exe'))) {
        if (-not (Test-Path -LiteralPath $postgresSource)) { throw 'payload\runtime\pgsql is missing from the package.' }
        Move-Item -LiteralPath $postgresSource -Destination $postgresTarget -Force
    }
    $nodeSource = Join-Path $root 'payload\runtime\node.exe'
    $nodeTarget = Join-Path $runtime 'node\node.exe'
    if (-not (Test-Path -LiteralPath $nodeTarget)) {
        if (-not (Test-Path -LiteralPath $nodeSource)) { throw 'payload\runtime\node.exe is missing from the package.' }
        New-PortableDirectory (Split-Path -Parent $nodeTarget)
        Move-Item -LiteralPath $nodeSource -Destination $nodeTarget -Force
    }
    $runnerSource = Join-Path $root 'payload\scripts\native-windows.ps1'
    if (-not (Test-Path -LiteralPath $runnerSource)) { throw 'payload\scripts\native-windows.ps1 is missing from the package.' }
    Copy-Item -LiteralPath $runnerSource -Destination (Join-Path $runtime 'native-windows.ps1') -Force
}

function Invoke-PortablePrepare {
    param([string]$HostName)
    Write-Step 'Preparing the deployment (database cluster, secrets, ports)'
    $origins = Get-PortableCorsOrigins -PublicHost $HostName -HttpsPort $HttpsPort
    $arguments = @(
        '-Version', (Get-PortableVersion $root),
        '-Database', 'uworkers',
        '-DatabasePort', [string]$DatabasePort,
        '-ApiPort', [string]$ApiPort,
        '-WebPort', [string]$WebPort,
        '-NodeExe', (Get-PortableNodePath $root),
        '-PublicOrigin', ('https://' + $HostName + ':' + $HttpsPort),
        '-CorsOrigins', $origins,
        '-WebHost', '127.0.0.1'
    )
    Invoke-PortableNativeAction -Root $root -Action 'Prepare' -ExtraArguments (@('-Clean') + $arguments) | Out-Null
    Set-PortableLocalLoginFlags
}

function Set-PortableLocalLoginFlags {
    # The packaged deployment signs its operator in with a host-provisioned
    # local account; email-code login has no mail service to talk to, so the
    # password form is the entry point and the email flow is switched off.
    $config = Get-PortableDeployment $root
    $config.BackendEnvironment | Add-Member -NotePropertyName AUTH_LOCAL_PASSWORD_ENABLED -NotePropertyValue 'true' -Force
    $config.BackendEnvironment | Add-Member -NotePropertyName AUTH_EMAIL_CODE_ENABLED -NotePropertyValue 'false' -Force
    Write-PortableJson (Join-Path $root 'deployment.json') $config
}

function Invoke-PortableStageRelease {
    Write-Step 'Staging the prebuilt server release'
    $releaseSource = Join-Path $root 'payload\release'
    if (-not (Test-Path -LiteralPath (Join-Path $releaseSource 'server.exe'))) { throw 'payload\release is missing server.exe; the package is incomplete.' }
    Invoke-PortableNativeAction -Root $root -Action 'Stage' -ExtraArguments @('-ReleaseSource', $releaseSource) | Out-Null
}

function Invoke-PortableStartServer {
    Write-Step 'Starting PostgreSQL, the API and the web app'
    Invoke-PortableNativeAction -Root $root -Action 'Start' | Out-Null
}

function Start-PortableHttps {
    param([string]$HostName)
    Write-Step ('Starting the LAN HTTPS entry on port ' + $HttpsPort)
    Start-PortableCaddy -Root $root -PublicHost $HostName -HttpsPort $HttpsPort -ApiPort $ApiPort -WebPort $WebPort | Out-Null
    Invoke-PortableTlsHandshake -Uri ('https://localhost:' + $HttpsPort + '/readyz')
    $rootCA = Export-PortableRootCA $root
    Write-Host ('  root CA: ' + $rootCA.Path + '  (SHA-256 ' + $rootCA.Sha256 + ')')
    return $rootCA
}

function Install-PortableAccount {
    param([string]$HostName, [string]$RootCAPath)
    Write-Step 'Creating the local operator account and the daemon token'
    $serverUrl = 'https://' + $HostName + ':' + $HttpsPort
    $email = 'admin@uworkers.local'
    $identity = Get-PortableIdentity -Email $email -DisplayName 'Admin'
    $saved = Get-PortableSavedCredential
    if (-not $saved) {
        $saved = [pscustomobject]@{ Username = 'admin'; Password = (New-PortablePassword 24) }
        Save-PortableCredential -Username $saved.Username -Password $saved.Password | Out-Null
    }
    Initialize-PortableLocalCredential -UserId $identity -Username $saved.Username -Password $saved.Password | Out-Null
    $login = Invoke-PortablePasswordLogin -ServerUrl $serverUrl -RootCAPath $RootCAPath -Username $saved.Username -Password $saved.Password
    $sessionToken = [string]$login.token
    if (-not $sessionToken) { throw 'The local sign-in returned no session token.' }
    $workspaces = @(Get-PortableWorkspaces -ServerUrl $serverUrl -RootCAPath $RootCAPath -Token $sessionToken)
    if ($workspaces.Count -eq 0) {
        $workspace = $null
        foreach ($slug in @('uworkers', 'uworkers-board', 'board')) {
            try {
                $workspace = New-PortableWorkspace -ServerUrl $serverUrl -RootCAPath $RootCAPath -Token $sessionToken -Name 'U Workers' -Slug $slug
                break
            } catch {
                if ($_.Exception.Message -notmatch '409|already exists|slug') { throw }
            }
        }
        if (-not $workspace) { throw 'Could not create the initial workspace.' }
    } else {
        $workspace = $workspaces[0]
    }
    $workspaceId = [string]$workspace.id
    $workspaceSlug = [string]$workspace.slug
    Complete-PortableOnboarding -ServerUrl $serverUrl -RootCAPath $RootCAPath -Token $sessionToken -WorkspaceId $workspaceId
    $accessToken = New-PortableAccessToken -ServerUrl $serverUrl -RootCAPath $RootCAPath -SessionToken $sessionToken
    $identityInfo = [ordered]@{
        email = $email
        username = $saved.Username
        user_id = $identity
        workspace_id = $workspaceId
        workspace_slug = $workspaceSlug
        created_at = (Get-Date).ToUniversalTime().ToString('o')
    }
    Write-PortableJson (Join-Path $root 'data\local-login.json') $identityInfo
    return [pscustomobject]@{ ServerUrl = $serverUrl; Username = $saved.Username; PasswordFile = (Join-Path $root 'data\local-login.txt'); UserId = $identity; WorkspaceId = $workspaceId; WorkspaceSlug = $workspaceSlug; AccessToken = $accessToken; SessionToken = $sessionToken }
}

function Get-PortableAdoptedDatabaseConfiguration {
    param([string]$OldRoot)
    $oldConfig = Read-PortableJson (Join-Path $OldRoot 'deployment.json')
    if (-not $oldConfig) { throw ('No deployment.json under -AdoptRoot ' + $OldRoot + '.') }
    return $oldConfig
}

function Invoke-PortableAdoption {
    param([string]$OldRoot, [string]$HostName)
    $oldConfig = Get-PortableAdoptedDatabaseConfiguration $OldRoot
    $oldRunner = Join-Path $OldRoot 'runtime\native-windows.ps1'
    if (-not (Test-Path -LiteralPath $oldRunner)) { throw ('The deployment under -AdoptRoot has no runtime\native-windows.ps1: ' + $OldRoot) }
    Write-Step 'Starting the existing deployment so its data can be backed up'
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    # A stopped deployment cannot be dumped; Start is idempotent when it is
    # already running, and a failure here is reported by the backup step below.
    Invoke-PortableCommand -Executable $shell -Arguments @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $oldRunner, '-Action', 'Start', '-Root', $OldRoot) | Out-Null
    Write-Step 'Backing up the existing deployment'
    $backup = Invoke-PortableCommand -Executable $shell -Arguments @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $oldRunner, '-Action', 'Backup', '-Root', $OldRoot)
    if ($backup.ExitCode -ne 0) { throw ('The existing deployment could not be backed up; nothing was changed: ' + $backup.Output.Trim()) }
    Write-Host ([string]$backup.Output).Trim()
    Write-Step 'Stopping the existing deployment'
    $stop = Invoke-PortableCommand -Executable $shell -Arguments @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $oldRunner, '-Action', 'Stop', '-Root', $OldRoot)
    if ($stop.ExitCode -ne 0) { throw ('The existing deployment could not be stopped; nothing was changed: ' + $stop.Output.Trim()) }
    Write-Step 'Copying the existing database cluster and uploads'
    $oldData = Join-Path $OldRoot 'data\postgres'
    if (-not (Test-Path -LiteralPath (Join-Path $oldData 'PG_VERSION'))) { throw ('No PostgreSQL data directory under ' + $OldRoot + '\data\postgres.') }
    New-PortableDirectory (Join-Path $root 'data')
    $copyResult = Invoke-PortableCommand -Executable 'robocopy.exe' -Arguments @($oldData, (Join-Path $root 'data\postgres'), '/E', '/SJ', '/SL', '/R:1', '/W:1', '/MT:4', '/NP', '/NFL', '/NDL', '/NJH', '/NJS')
    if ($copyResult.ExitCode -ge 8) { throw ('Copying the existing database failed (robocopy exit ' + $copyResult.ExitCode + ').') }
    $plainGlobal =Join-Path $root 'data\postgres\postmaster.pid'
    Remove-Item -LiteralPath $plainGlobal -Force -ErrorAction SilentlyContinue
    $uploads = Join-Path $OldRoot 'uploads'
    if (Test-Path -LiteralPath $uploads) {
        $copyUploads = Invoke-PortableCommand -Executable 'robocopy.exe' -Arguments @($uploads, (Join-Path $root 'uploads'), '/E', '/SJ', '/SL', '/R:1', '/W:1', '/MT:4', '/NP', '/NFL', '/NDL', '/NJH', '/NJS')
        if ($copyUploads.ExitCode -ge 8) { throw ('Copying the existing uploads failed (robocopy exit ' + $copyUploads.ExitCode + ').') }
    }
    Write-Step 'Preparing the portable deployment on top of the copied data'
    Invoke-PortablePrepare -HostName $HostName
    # Keep the credentials the copied cluster was initialized with, and keep the
    # existing JWT secret so signed-in sessions survive the move.
    $config = Get-PortableDeployment $root
    $config.DatabaseUser = [string]$oldConfig.DatabaseUser
    $config.DatabasePassword = [string]$oldConfig.DatabasePassword
    $config.SourceDatabase = [string]$oldConfig.SourceDatabase
    $config.Database = [string]$oldConfig.Database
    $config.BackendEnvironment = $oldConfig.BackendEnvironment
    if ($oldConfig.FrontendEnvironment) { $config.FrontendEnvironment = $oldConfig.FrontendEnvironment }
    $origin = 'https://' + $HostName + ':' + $HttpsPort
    $config.BackendEnvironment | Add-Member -NotePropertyName FRONTEND_ORIGIN -NotePropertyValue $origin -Force
    $config.BackendEnvironment | Add-Member -NotePropertyName MULTICA_APP_URL -NotePropertyValue $origin -Force
    $config.BackendEnvironment | Add-Member -NotePropertyName CORS_ALLOWED_ORIGINS -NotePropertyValue (Get-PortableCorsOrigins -PublicHost $HostName -HttpsPort $HttpsPort) -Force
    Write-PortableJson (Join-Path $root 'deployment.json') $config
    # The copied cluster keeps its own postgresql.conf; pin the loopback listener
    # and the port this deployment uses (later entries win in PostgreSQL).
    $postgresConfig = Join-Path $root 'data\postgres\postgresql.conf'
    $override = [Environment]::NewLine + "listen_addresses = '127.0.0.1'" + [Environment]::NewLine + 'port = ' + $DatabasePort + [Environment]::NewLine
    [IO.File]::AppendAllText($postgresConfig, $override, (New-Object Text.UTF8Encoding($false)))
    Invoke-PortableStageRelease
    Invoke-PortableStartServer
    $rootCA = Start-PortableHttps -HostName $HostName
    $serverUrl = 'https://' + $HostName + ':' + $HttpsPort
    Wait-PortableHttp -Uri ($serverUrl + '/readyz') -RootCAPath $rootCA.Path -TimeoutSec 90 -Description 'the adopted API' | Out-Null
    $operatorId = Get-PortableOwnerUserId -Email $OperatorEmail
    $accessToken = New-PortableUserAccessToken -UserId $operatorId -TokenName 'portable-daemon'
    $ownerRows = @()
    try { $ownerRows = Invoke-PortableSql ('SELECT w.id::text FROM workspace w JOIN member m ON m.workspace_id = w.id WHERE m.user_id = ''' + $operatorId + ''' ORDER BY w.created_at ASC LIMIT 1;') } catch { $ownerRows = @() }
    $workspaceId = ''
    if ($ownerRows.Count -gt 0) { $workspaceId = [string]$ownerRows[0] }
    return [pscustomobject]@{ RootCA = $rootCA; ServerUrl = $serverUrl; WorkspaceId = $workspaceId; AccessToken = $accessToken; UserId = $operatorId; Backup = ([string]$backup.Output).Trim() }
}

# --------------------------------------------------------------------------
# Main flow
# --------------------------------------------------------------------------
$startedAt = Get-Date
Write-Host ''
Write-Host 'U Workers portable server' -ForegroundColor Green
Write-Host ('Package: ' + $root)
Write-Host ''

$manifest = Get-PortableManifest $root
$installedConfig = Read-PortableJson (Join-Path $root 'deployment.json')

$installReportPath = Join-Path $root 'datainstall-report.json'
$resumeInstall = $false
if ($installedConfig -and -not $AdoptRoot) {
    $installedVersion = [string]$installedConfig.Version
    if ($manifest.version -ne $installedVersion -and -not $Force) {
        Write-Host ('This folder already runs U Workers ' + $installedVersion + '; the package is ' + $manifest.version + '.') -ForegroundColor Yellow
        Write-Host 'Running the overlay upgrade (backup first, data preserved)...'
        $upgradeScript = Join-Path $root 'upgrade.ps1'
        & $upgradeScript -PublicHost $PublicHost -HttpsPort $HttpsPort -Profile $Profile -SkipBrowserTrust:$SkipBrowserTrust -SkipDaemon:$SkipDaemon -NoBrowser:$NoBrowser -NoStartup:$NoStartup
        exit 0
    }
    if (-not (Test-Path -LiteralPath $installReportPath)) {
        # A previous run configured the deployment but stopped before finishing
        # (account, HTTPS or daemon). Re-running Setup finishes those steps on the
        # existing data instead of refusing or creating a second deployment.
        $resumeInstall = $true
        Write-Host 'A previous install did not finish; resuming the remaining steps.' -ForegroundColor Yellow
    } else {
        Write-Host 'U Workers is already installed in this folder; starting it.' -ForegroundColor Yellow
        & (Join-Path $root 'start.ps1') -Profile $Profile
        Write-Host ('Board: https://' + (Get-PortablePublicHost $PublicHost) + ':' + $HttpsPort + '/')
        exit 0
    }
}

if (([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host 'Note: this installer needs no administrator rights; running it elevated only makes the folder permissions stricter.' -ForegroundColor DarkYellow
}

# Validate the package before touching anything. A resumed install already
# moved PostgreSQL and Node out of payload into runtime, so only the pristine
# path is checked here.
if (-not $resumeInstall) {
    foreach ($relative in @('payload\release\server.exe', 'payload\release\web\apps\web\server.js', 'payload\release\migrations', 'payload\runtime\pgsql\bin\psql.exe', 'payload\runtime\node.exe', 'payload\runtime\caddy\caddy.exe', 'payload\runtime\opencode\opencode.exe', 'payload\bin\u-workers.exe', 'payload\scripts\native-windows.ps1')) {
        Get-PortableFile $root $relative | Out-Null
    }
}

$hostName = Get-PortablePublicHost $PublicHost
Write-Host ('LAN address for other machines: ' + $hostName + ':' + $HttpsPort)
if (-not $AdoptRoot -and -not $resumeInstall) {
    foreach ($port in @($ApiPort, $WebPort, $DatabasePort, $HttpsPort)) {
        if (-not (Test-PortablePortFree $port)) { throw ('Port ' + $port + ' is already in use. Close the other program or pass different ports to Setup.cmd.') }
    }
}

$result = $null
if ($AdoptRoot) {
    $adopted = Invoke-PortableAdoption -OldRoot ([IO.Path]::GetFullPath($AdoptRoot)) -HostName $hostName
    $result = [ordered]@{
        ServerUrl = $adopted.ServerUrl
        RootCA = $adopted.RootCA
        WorkspaceId = $adopted.WorkspaceId
        AccessToken = $adopted.AccessToken
        Mode = 'adopted'
        Backup = $adopted.Backup
        PasswordFile = $null
    }
} else {
    if (-not $resumeInstall) {
        Invoke-PortableStageRuntime
        Invoke-PortablePrepare -HostName $hostName
        Invoke-PortableStageRelease
    } else {
        Set-PortableLocalLoginFlags
        $resumedDeployment = Get-PortableDeployment $root
        if (-not $resumedDeployment.Release -or -not (Test-Path -LiteralPath (Join-Path ([string]$resumedDeployment.Release) 'server.exe'))) {
            Invoke-PortableStageRelease
        }
    }
    Invoke-PortableStartServer
    $rootCA = Start-PortableHttps -HostName $hostName
    $serverUrl = 'https://' + $hostName + ':' + $HttpsPort
    Write-Step 'Checking the board over HTTPS'
    Wait-PortableHttp -Uri ($serverUrl + '/login') -RootCAPath $rootCA.Path -TimeoutSec 90 -Description 'the sign-in page' | Out-Null
    $board = Invoke-PortableWebRequest -Uri ($serverUrl + '/') -RootCAPath $rootCA.Path
    # The Next.js app answers "/" with a redirect (onboarding or workspace); any
    # non-error response proves the HTTPS entry serves the web app.
    if ($board.StatusCode -lt 200 -or $board.StatusCode -ge 400) { throw ('The board did not answer over HTTPS (status ' + $board.StatusCode + ').') }
    $account = Install-PortableAccount -HostName $hostName -RootCAPath $rootCA.Path
    $result = [ordered]@{
        ServerUrl = $account.ServerUrl
        RootCA = $rootCA
        WorkspaceId = $account.WorkspaceId
        AccessToken = $account.AccessToken
        SessionToken = $account.SessionToken
        Mode = 'clean'
        Username = $account.Username
        PasswordFile = $account.PasswordFile
        WorkspaceSlug = $account.WorkspaceSlug
    }
}

# Browser trust is optional and always a click-through, never silent.
$browserTrust = [pscustomobject]@{ Trusted = $false; AlreadyInstalled = $false; Thumbprint = ''; Skipped = $true }
if (-not $SkipBrowserTrust) {
    Write-Step 'Trusting the package root CA for this Windows user (one confirmation dialog; -SkipBrowserTrust skips)'
    $browserTrust = Install-PortableBrowserTrust -Root $root -TimeoutSec 90
    if (-not $browserTrust.Trusted) { Write-Host '  The certificate was not trusted. The CLI and daemon still work; rerun Setup.cmd later or use Trust-Browser.cmd.' -ForegroundColor Yellow }
} else {
    Write-Host 'Browser trust skipped (-SkipBrowserTrust).' -ForegroundColor Yellow
}

$runtimes = @()
if (-not $SkipDaemon) {
    Write-Step 'Connecting the local daemon (bundled opencode will register a runtime)'
    Start-PortableDaemonConnection -ServerUrl $result.ServerUrl -RootCAPath $result.RootCA.Path -AccessToken $result.AccessToken | Out-Null
    $runtimes = @(Start-PortableRuntimeAndWait -ServerUrl $result.ServerUrl -RootCAPath $result.RootCA.Path -Token $result.AccessToken -WorkspaceId $result.WorkspaceId)
    if ($runtimes.Count -eq 0) {
        Write-Host '  The daemon is running but has not registered a runtime yet. Give it a minute, then open the board.' -ForegroundColor Yellow
    } else {
        foreach ($runtime in $runtimes) { Write-Host ('  runtime: ' + $runtime.name + ' (' + $runtime.provider + ', ' + $runtime.status + ')') }
    }
} else {
    Write-Host 'Daemon start skipped (-SkipDaemon).' -ForegroundColor Yellow
}

$startupPath = ''
if (-not $NoStartup) {
    Write-Step 'Registering start-on-login for the current Windows user'
    $startupPath = Install-PortableStartup -Root $root
    Write-Host ('  startup item: ' + $startupPath)
}

$report = [ordered]@{
    installedAt = (Get-Date).ToUniversalTime().ToString('o')
    version = [string]$manifest.version
    commit = [string]$manifest.commit
    mode = $result.Mode
    boardUrl = $result.ServerUrl
    lanHost = $hostName
    httpsPort = $HttpsPort
    apiPort = $ApiPort
    webPort = $WebPort
    databasePort = $DatabasePort
    profile = $Profile
    rootCaFile = $result.RootCA.Path
    rootCaSha256 = $result.RootCA.Sha256
    browserTrust = $browserTrust.Trusted
    browserTrustSkipped = [bool]$SkipBrowserTrust
    username = $(if ($result.Contains('Username')) { $result.Username } else { '' })
    passwordFile = $result.PasswordFile
    workspaceId = $result.WorkspaceId
    startOnLogin = $startupPath
    runtimes = @($runtimes | ForEach-Object { [ordered]@{ name = $_.name; provider = $_.provider; status = $_.status; id = $_.id } })
    backup = $result.Backup
    durationSeconds = [math]::Round(((Get-Date) - $startedAt).TotalSeconds, 1)
}
$reportPath = Write-PortableInstallReport -Root $root -Report $report
$report.runtimes = @($runtimes | ForEach-Object { [ordered]@{ name = $_.name; provider = $_.provider; status = $_.status; id = $_.id } })
Write-PortableInstallReport -Root $root -Report $report | Out-Null

Write-Host ''
Write-Host 'U Workers is ready.' -ForegroundColor Green
Write-Host ('  Board (this machine):   https://localhost:' + $HttpsPort + '/')
Write-Host ('  Board (other machines): ' + $result.ServerUrl + '/')
if ($result.Contains('Username') -and $result.Username) {
    Write-Host ('  Sign-in: ' + $result.Username + '  (password in ' + $result.PasswordFile + ')')
} elseif ($result.Mode -eq 'adopted') {
    Write-Host '  Sign-in: your existing U Workers account still works.'
}
Write-Host ('  Root certificate:       ' + $result.RootCA.Path)
Write-Host ('  Start / stop / status:  Start.cmd / Stop.cmd / Status.cmd')
Write-Host ('  Install report:         ' + $reportPath)

if (-not $NoBrowser) {
    try { Start-Process -FilePath ('https://localhost:' + $HttpsPort + '/') | Out-Null } catch { }
}
