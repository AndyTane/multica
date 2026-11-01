# R1-1 regression tests: first-time adoption must stage the packaged runtime,
# refuse unsafe input before stopping the old service, check the PostgreSQL major
# version before copying, and keep data, secrets and a locatable backup.
# External processes are stubbed; no real deployment, database or service is
# touched. Windows PowerShell 5.1 compatible.
# Run: powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\adoption.test.ps1
$ErrorActionPreference = 'Stop'
$portableRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $portableRoot 'portable-common.ps1')

function Assert-Test {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw ('FAILED: ' + $Message) }
}

function Assert-Throws {
    param([scriptblock]$Action, [string]$Pattern, [string]$Message)
    $threw = $false
    try { & $Action | Out-Null } catch { $threw = $true; if ($Pattern -and $_.Exception.Message -notmatch $Pattern) { throw ('FAILED: ' + $Message + ' (unexpected message: ' + $_.Exception.Message + ')') } }
    if (-not $threw) { throw ('FAILED: ' + $Message + ' (nothing was rejected)') }
}

function New-TestFile {
    param([string]$Path, [string]$Content = 'x')
    $directory = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory -Force | Out-Null }
    [IO.File]::WriteAllText($Path, $Content)
}

function Get-FreeTcpPort {
    $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = [int]$listener.LocalEndpoint.Port
    $listener.Stop()
    return $port
}

# Load install.ps1 functions without running its parameter block and main flow.
$ast = [Management.Automation.Language.Parser]::ParseFile((Join-Path $portableRoot 'install.ps1'), [ref]$null, [ref]$null)
foreach ($definition in $ast.FindAll({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] }, $false)) {
    . ([scriptblock]::Create($definition.Extent.Text))
}

# Variables the adoption functions read from the script scope.
$root = ''
$HttpsPort = 1
$ApiPort = 1
$WebPort = 1
$DatabasePort = 1
$OperatorEmail = 'admin@uworkers.local'

# --- stubs for the external-process and service boundaries ----------------
$script:calls = @()
$script:nativeCalls = @()
$script:stubOldRoot = ''
$script:packageMajor = '17'

function Invoke-PortableCommand {
    param([string]$Executable, [string[]]$Arguments, [hashtable]$Environment, [string]$WorkingDirectory)
    $leaf = Split-Path -Leaf $Executable
    $script:calls += ($leaf + ' ' + (@($Arguments) -join ' '))
    if ($leaf -eq 'postgres.exe') { return [pscustomobject]@{ ExitCode = 0; Output = ('postgres (PostgreSQL) ' + $script:packageMajor + '.11') } }
    if ($leaf -eq 'robocopy.exe') {
        $source = [string]$Arguments[0]
        $target = [string]$Arguments[1]
        if (Test-Path -LiteralPath $source) {
            New-Item -ItemType Directory -Path $target -Force | Out-Null
            Copy-Item -Path (Join-Path $source '*') -Destination $target -Recurse -Force -ErrorAction SilentlyContinue
        }
        return [pscustomobject]@{ ExitCode = 0; Output = '' }
    }
    if ((@($Arguments) -join ' ') -match '-Action Backup') {
        return [pscustomobject]@{ ExitCode = 0; Output = ('backup: ' + (Join-Path $script:stubOldRoot 'backups\native-test')) }
    }
    return [pscustomobject]@{ ExitCode = 0; Output = 'stand-in success' }
}

function Invoke-PortableNativeAction {
    param([string]$Root, [string]$Action, [string[]]$ExtraArguments)
    $script:nativeCalls += $Action
    return 'stand-in success'
}

function Invoke-PortablePrepare {
    param([string]$HostName)
    $script:nativeCalls += 'Prepare'
    $script:runtimePresentAtPrepare = Test-Path -LiteralPath (Join-Path $root 'runtime\native-windows.ps1')
    $config = [ordered]@{
        Version = 'v0.2.0'
        Release = ''
        DatabaseUser = 'uworkers'
        DatabasePassword = 'package-secret'
        SourceDatabase = 'uworkers'
        Database = 'uworkers'
        DatabasePort = $DatabasePort
        ApiPort = $ApiPort
        WebPort = $WebPort
        HttpsPort = $HttpsPort
        PublicOrigin = ('https://' + $HostName + ':' + $HttpsPort)
        BackendEnvironment = [ordered]@{ JWT_SECRET = 'package-secret'; APP_ENV = 'production' }
        FrontendEnvironment = [ordered]@{}
    }
    Write-PortableJson (Join-Path $root 'deployment.json') $config
}

function Invoke-PortableStageRelease {
    $script:nativeCalls += 'StageRelease'
}
function Invoke-PortableStartServer {
    $script:nativeCalls += 'StartServer'
}
function Start-PortableHttps {
    param([string]$HostName)
    $script:nativeCalls += 'Https'
    return [pscustomobject]@{ Path = (Join-Path $root 'root-ca.crt'); Sha256 = 'ABCDEF0123456789' }
}
function Wait-PortableHttp {
    param([string]$Uri, [string]$RootCAPath, [int]$TimeoutSec, [string]$Description)
}
function Get-PortableOwnerUserId {
    param([string]$Email)
    return 'operator-user-id'
}
function New-PortableUserAccessToken {
    param([string]$UserId, [string]$TokenName)
    return 'mul_test_token'
}
function Invoke-PortableSql {
    param($Root, [string]$Sql, [string]$Database, [string]$FieldSeparator)
    return @('workspace-1')
}

function Initialize-AdoptionFixture {
    param([string]$NewRoot, [string]$OldRoot, [string]$OldPostgresMajor = '17', [bool]$WithUploads = $true)
    if (Test-Path -LiteralPath $NewRoot) { Remove-Item -LiteralPath $NewRoot -Recurse -Force }
    if (Test-Path -LiteralPath $OldRoot) { Remove-Item -LiteralPath $OldRoot -Recurse -Force }
    # Old native deployment: config, runner, cluster and uploads with test data.
    $oldConfig = [ordered]@{
        Version = 'v0.1.0'
        Root = $OldRoot
        DatabaseUser = 'uworkers'
        DatabasePassword = 'old-cluster-password'
        SourceDatabase = 'uworkers'
        Database = 'uworkers'
        DatabasePort = 16433
        ApiPort = 18080
        WebPort = 13000
        PublicOrigin = 'http://localhost:13000'
        BackendEnvironment = [ordered]@{ JWT_SECRET = 'old-test-jwt-secret'; APP_ENV = 'production' }
        FrontendEnvironment = [ordered]@{}
    }
    Write-PortableJson (Join-Path $OldRoot 'deployment.json') $oldConfig
    New-TestFile (Join-Path $OldRoot 'runtime\native-windows.ps1') '# old native runner stand-in'
    New-TestFile (Join-Path $OldRoot ('data\postgres\PG_VERSION')) $OldPostgresMajor
    New-TestFile (Join-Path $OldRoot 'data\postgres\postgresql.conf') ('# postgresql.conf' + [Environment]::NewLine)
    New-TestFile (Join-Path $OldRoot 'data\postgres\base\1\marker') 'cluster-data'
    New-TestFile (Join-Path $OldRoot 'data\postgres\postmaster.pid') '1234'
    if ($WithUploads) { New-TestFile (Join-Path $OldRoot 'uploads\report.txt') 'uploaded-file' }
    # Fresh package folder: manifest and the payload the clean path would consume.
    New-TestFile (Join-Path $NewRoot 'manifest.json') '{"version":"v0.2.0","commit":"deadbee"}'
    New-TestFile (Join-Path $NewRoot 'payload\release\server.exe') 'server'
    New-TestFile (Join-Path $NewRoot 'payload\release\web\apps\web\server.js') 'web'
    New-TestFile (Join-Path $NewRoot 'payload\release\migrations\001.sql') 'select 1;'
    New-TestFile (Join-Path $NewRoot 'payload\runtime\node.exe') 'node'
    New-TestFile (Join-Path $NewRoot 'payload\runtime\pgsql\bin\psql.exe') 'psql'
    New-TestFile (Join-Path $NewRoot 'payload\runtime\pgsql\bin\postgres.exe') 'postgres'
    New-TestFile (Join-Path $NewRoot 'payload\scripts\native-windows.ps1') '# packaged runner'
    $script:calls = @()
    $script:nativeCalls = @()
    $script:runtimePresentAtPrepare = $false
    $script:stubOldRoot = $OldRoot
}

$sandbox = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-adoption-test-' + [guid]::NewGuid().ToString('N'))
try {
    $newRoot = Join-Path $sandbox 'new-package'
    $oldRoot = Join-Path $sandbox 'old-native'
    $port = @{ Https = Get-FreeTcpPort; Api = Get-FreeTcpPort; Web = Get-FreeTcpPort; Database = Get-FreeTcpPort }
    $HttpsPort = $port.Https
    $ApiPort = $port.Api
    $WebPort = $port.Web
    $DatabasePort = $port.Database

    # --- preflight failures happen before the old service is touched -----
    $script:packageMajor = '17'
    $root = $newRoot
    $nestedOld = Join-Path $newRoot 'nested-old'
    Initialize-AdoptionFixture -NewRoot $newRoot -OldRoot $nestedOld
    Assert-Throws { Invoke-PortableAdoption -OldRoot $nestedOld -HostName '127.0.0.1' } 'inside the new package' 'adopting from a folder inside the new package must be refused'
    Assert-Test ($script:calls.Count -eq 0) 'A refused source/target layout must not run any external command.'
    Assert-Test ($script:nativeCalls.Count -eq 0) 'A refused source/target layout must not run any deployment action.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $newRoot 'payload\runtime\pgsql\bin\psql.exe')) 'A refused layout must not consume the packaged runtime.'

    Initialize-AdoptionFixture -NewRoot $newRoot -OldRoot $oldRoot
    Assert-Throws { Invoke-PortableAdoption -OldRoot $newRoot -HostName '127.0.0.1' } 'own folder|new package folder itself' 'adopting the package folder itself must be refused'
    Assert-Test ($script:calls.Count -eq 0) 'Adopting the package folder itself must not touch services.'

    Initialize-AdoptionFixture -NewRoot $newRoot -OldRoot $oldRoot
    New-TestFile (Join-Path $newRoot 'data\postgres\PG_VERSION') '17'
    Assert-Throws { Invoke-PortableAdoption -OldRoot $oldRoot -HostName '127.0.0.1' } 'already contains' 'adopting into a folder that already holds a deployment must be refused'
    Assert-Test ($script:calls.Count -eq 0) 'A dirty target must not touch the old service.'

    Initialize-AdoptionFixture -NewRoot $newRoot -OldRoot $oldRoot -OldPostgresMajor '16'
    $script:packageMajor = '17'
    Assert-Throws { Invoke-PortableAdoption -OldRoot $oldRoot -HostName '127.0.0.1' } 'PostgreSQL 16|PostgreSQL' 'a PostgreSQL major mismatch must be refused before copying the cluster'
    Assert-Test (@($script:calls | Where-Object { $_ -match '-Action ' }).Count -eq 0) 'A PostgreSQL major mismatch must not stop the old service or copy anything.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $newRoot 'payload\runtime\pgsql\bin\psql.exe')) 'A PostgreSQL major mismatch must not consume the packaged runtime.'

    Initialize-AdoptionFixture -NewRoot $newRoot -OldRoot $oldRoot
    $busy = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, 0)
    $busy.Start()
    try {
        $HttpsPort = [int]$busy.LocalEndpoint.Port
        Assert-Throws { Invoke-PortableAdoption -OldRoot $oldRoot -HostName '127.0.0.1' } 'already in use' 'a port already used by another program must be refused before stopping the old service'
        Assert-Test (@($script:calls | Where-Object { $_ -match '-Action ' }).Count -eq 0) 'A port conflict must not stop the old service.'
    } finally {
        $busy.Stop()
        $HttpsPort = $port.Https
    }

    # --- the successful adoption ------------------------------------------
    Initialize-AdoptionFixture -NewRoot $newRoot -OldRoot $oldRoot
    $script:packageMajor = '17'
    $result = Invoke-PortableAdoption -OldRoot $oldRoot -HostName '127.0.0.1'
    Assert-Test ($null -ne $result) 'The adoption must return its result.'
    Assert-Test ($script:runtimePresentAtPrepare) 'The packaged runtime must be staged before Prepare runs (the original R1-1 failure).'
    Assert-Test (Test-Path -LiteralPath (Join-Path $newRoot 'runtime\native-windows.ps1')) 'The runner must be staged into runtime\.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $newRoot 'runtime\pgsql\bin\psql.exe')) 'PostgreSQL must be staged from the payload.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $newRoot 'runtime\node\node.exe')) 'Node must be staged from the payload.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $newRoot 'data\postgres\base\1\marker')) 'The old PostgreSQL cluster must be copied.'
    Assert-Test ((Get-Content -LiteralPath (Join-Path $newRoot 'data\postgres\base\1\marker') -Raw).Trim() -eq 'cluster-data') 'The copied cluster data must keep the old contents.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $newRoot 'data\postgres\postmaster.pid'))) 'A stale postmaster.pid must not be copied.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $newRoot 'uploads\report.txt')) 'The old uploads must be copied.'
    $newConfig = Get-PortableDeployment $newRoot
    Assert-Test ($newConfig.Version -eq 'v0.2.0') 'The new deployment must use the package version.'
    Assert-Test ($newConfig.DatabaseUser -eq 'uworkers') 'The copied cluster credentials must be kept.'
    Assert-Test ($newConfig.DatabasePassword -eq 'old-cluster-password') 'The copied cluster password must be kept.'
    Assert-Test ($newConfig.BackendEnvironment.JWT_SECRET -eq 'old-test-jwt-secret') 'The old JWT secret must be preserved so existing sessions survive.'
    $postgresConfig = Get-Content -LiteralPath (Join-Path $newRoot 'data\postgres\postgresql.conf') -Raw
    Assert-Test ($postgresConfig -match ('port = ' + $DatabasePort)) 'The copied cluster must be pinned to the portable database port.'
    Assert-Test ($postgresConfig -match "listen_addresses = '127.0.0.1'") 'The copied cluster must listen on loopback only.'
    Assert-Test (([string]$result.Backup) -match 'backup: ') 'The adoption result must report the backup location.'
    Assert-Test (([string]$result.Backup) -match 'native-test') 'The reported backup must be the one the old deployment produced.'
    # Backup before stop, and the old service was started before the backup.
    $startIndex = -1; $backupIndex = -1; $stopIndex = -1
    for ($i = 0; $i -lt $script:calls.Count; $i++) {
        if ($script:calls[$i] -match '-Action Start' -and $startIndex -lt 0) { $startIndex = $i }
        if ($script:calls[$i] -match '-Action Backup' -and $backupIndex -lt 0) { $backupIndex = $i }
        if ($script:calls[$i] -match '-Action Stop' -and $stopIndex -lt 0) { $stopIndex = $i }
    }
    Assert-Test ($startIndex -ge 0) 'The old deployment must be started so it can be dumped.'
    Assert-Test ($backupIndex -gt $startIndex) 'The backup must run after the old deployment starts.'
    Assert-Test ($stopIndex -gt $backupIndex) 'The old deployment may only stop after a successful backup.'
    Assert-Test (($script:nativeCalls -join ',') -match 'Prepare') 'Prepare must run after the data was copied.'
    Assert-Test (($script:nativeCalls -join ',') -match 'StageRelease') 'The prebuilt release must be staged after Prepare.'
    Assert-Test (($script:nativeCalls -join ',') -match 'StartServer') 'The adopted deployment must be started.'
    # The old deployment folder itself is only read; its files are unchanged.
    $oldConfigAfter = Get-Content -LiteralPath (Join-Path $oldRoot 'deployment.json') -Raw | ConvertFrom-Json
    Assert-Test ($oldConfigAfter.Version -eq 'v0.1.0') 'The old deployment configuration must not be modified.'
    Assert-Test ($oldConfigAfter.BackendEnvironment.JWT_SECRET -eq 'old-test-jwt-secret') 'The old deployment secrets must not be modified.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $oldRoot 'data\postgres\base\1\marker')) 'The old cluster files must stay in place.'

    Write-Host 'adoption tests passed.'
} finally {
    Remove-Item -LiteralPath $sandbox -Recurse -Force -ErrorAction SilentlyContinue
}
