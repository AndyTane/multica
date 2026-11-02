# Windows PowerShell 5.1 and PowerShell 7. No live services or credentials.
$ErrorActionPreference = 'Stop'
$portableRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $portableRoot 'portable-common.ps1')
$ast = [Management.Automation.Language.Parser]::ParseFile((Join-Path $portableRoot 'install.ps1'), [ref]$null, [ref]$null)
foreach ($definition in $ast.FindAll({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] }, $false)) {
    . ([scriptblock]::Create($definition.Extent.Text))
}
function Assert-Test {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw ('FAILED: ' + $Message) }
}
function Assert-Throws {
    param([scriptblock]$Action, [string]$Pattern)
    try { & $Action | Out-Null } catch {
        Assert-Test ($_.Exception.Message -match $Pattern) ('unexpected error: ' + $_.Exception.Message)
        return
    }
    throw ('FAILED: expected error matching ' + $Pattern)
}
$root = Join-Path ([IO.Path]::GetTempPath()) ('uw-account-test-' + [guid]::NewGuid().ToString('N'))
$identityId = '11111111-1111-4111-8111-111111111111'
$HttpsPort = 13443
$script:sqlRows = @()
$script:commands = @()
$script:commandExitCode = 0
$script:credentialUserId = ''
function Invoke-PortableSqlFile { param($Sql, $Database) return $script:sqlRows }
function Get-PortableSavedCredential { return [pscustomobject]@{ Username = 'admin'; Password = 'fixture-unused' } }
function Initialize-PortableLocalCredential { param($UserId, $Username, $Password) $script:credentialUserId = $UserId }
function Invoke-PortablePasswordLogin { param($ServerUrl, $RootCAPath, $Username, $Password) return [pscustomobject]@{ token = 'fixture-unused' } }
function Get-PortableWorkspaces { param($ServerUrl, $RootCAPath, $Token) return [pscustomobject]@{ id = 'ws1'; slug = 'fixture' } }
function Complete-PortableOnboarding { param($ServerUrl, $RootCAPath, $Token, $WorkspaceId) }
function New-PortableAccessToken { param($ServerUrl, $RootCAPath, $SessionToken) return 'fixture-unused' }
function Invoke-PortableCommand {
    param($Executable, $Arguments, $Environment, [switch]$Utf8Output)
    $script:commands += [pscustomobject]@{ Executable = $Executable; Arguments = $Arguments; Environment = $Environment }
    return [pscustomobject]@{ ExitCode = $script:commandExitCode; Output = 'fixture result' }
}
try {
    New-Item -ItemType Directory -Path (Join-Path $root 'release') -Force | Out-Null
    New-Item -ItemType File -Path (Join-Path $root 'release/local-auth.exe') | Out-Null
    Write-PortableJson (Join-Path $root 'deployment.json') @{ Release = (Join-Path $root 'release'); Database = 'fixture' }

    $script:sqlRows = @('BEGIN', ($identityId + '|true'), 'COMMIT')
    $identity = Get-PortableIdentity -Email 'admin@uworkers.local' -DisplayName 'Admin'
    Assert-Test ($identity.UserId -eq $identityId -and $identity.Created) 'new identity must return its UUID and creation result'
    $account = Install-PortableAccount -HostName 'localhost' -RootCAPath 'unused'
    Assert-Test ($account.UserId -eq $identityId -and $script:credentialUserId -eq $identityId) 'grant and credential use the same new identity'
    Assert-Test ($script:commands.Count -eq 1) 'new operator is granted exactly once'
    $command = $script:commands[0]
    Assert-Test (($command.Arguments -join '|') -eq ('grant-admin|--user-id|' + $identityId + '|--deployment-config|' + (Join-Path $root 'deployment.json'))) 'only the newly returned UUID is granted'
    Assert-Test ($command.Environment.ContainsKey('DATABASE_URL') -and $command.Environment.DATABASE_URL -eq '') 'ambient database overrides cannot redirect the grant'

    $script:sqlRows = @($identityId + '|false')
    $script:commands = @()
    Install-PortableAccount -HostName 'localhost' -RootCAPath 'unused' | Out-Null
    Assert-Test ($script:commands.Count -eq 0) 'existing identity (including a resumed installation) keeps revoked permissions'

    $script:sqlRows = @($identityId + '|true')
    $script:commandExitCode = 1
    Assert-Throws { Install-PortableAccount -HostName 'localhost' -RootCAPath 'unused' } 'Could not initialize instance administrator'
    $script:commandExitCode = 0
    Remove-Item -LiteralPath (Join-Path $root 'release/local-auth.exe')
    Assert-Throws { Install-PortableAccount -HostName 'localhost' -RootCAPath 'unused' } 'local-auth.exe is missing'

    foreach ($badRows in @(@(), @($identityId + '|true', $identityId + '|false'), @($identityId + '|unknown'))) {
        $script:sqlRows = $badRows
        Assert-Throws { Get-PortableIdentity -Email 'admin@uworkers.local' -DisplayName 'Admin' } 'Could not confirm'
    }
    Write-Host 'account-initialization tests passed'
} finally {
    $resolved = [IO.Path]::GetFullPath($root)
    if (-not $resolved.StartsWith([IO.Path]::GetTempPath(), [StringComparison]::OrdinalIgnoreCase) -or (Split-Path -Leaf $resolved) -notlike 'uw-account-test-*') { throw 'Unsafe test cleanup path' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
