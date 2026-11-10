$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'git-cred-ghtoken.ps1')

function Assert-CredentialTest {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

# Override the reader with synthetic credentials; never modify real user settings.
$script:userToken = 'synthetic-first'
$script:readCount = 0
function Get-GitHubUserToken {
    $script:readCount++
    $script:userToken
}

function Invoke-TestCredential {
    param([string]$Request, [string]$Operation = 'get')
    $reader = [IO.StringReader]::new($Request)
    $writer = [IO.StringWriter]::new()
    try {
        Invoke-GitHubUserTokenCredential -Operation $Operation -InputReader $reader -OutputWriter $writer
        $writer.ToString()
    } finally {
        $reader.Dispose()
        $writer.Dispose()
    }
}

$savedProcessToken = [Environment]::GetEnvironmentVariable('GH_TOKEN', 'Process')
try {
    [Environment]::SetEnvironmentVariable('GH_TOKEN', $null, 'Process')
    $request = "protocol=https`nhost=github.com`npath=example/repository.git`n`n"
    $result = Invoke-TestCredential $request
    Assert-CredentialTest ($result -match 'password=synthetic-first') 'Missing process GH_TOKEN must not prevent user credential lookup.'
    Assert-CredentialTest ($result -match 'username=x-access-token') 'Git must receive the token username.'

    [Environment]::SetEnvironmentVariable('GH_TOKEN', 'synthetic-stale-process', 'Process')
    $script:userToken = 'synthetic-rotated'
    $result = Invoke-TestCredential $request
    Assert-CredentialTest ($result -match 'password=synthetic-rotated') 'Rotation must take effect on the next request, even with a stale process token.'
    Assert-CredentialTest ($script:readCount -eq 2) 'Every Git request must reread the user setting.'

    $script:userToken = $null
    Assert-CredentialTest ([string]::IsNullOrEmpty((Invoke-TestCredential $request))) 'Removing the user token must not resurrect a stale process token.'
    $readsBeforeRejected = $script:readCount
    foreach ($rejected in @(
        "protocol=http`nhost=github.com`n`n",
        "protocol=https`nhost=github.com.evil.example`n`n",
        "protocol=https`nhost=gitlab.com`n`n",
        "protocol=https`n`n"
    )) {
        Assert-CredentialTest ([string]::IsNullOrEmpty((Invoke-TestCredential $rejected))) 'Unsupported destinations must not receive credentials.'
    }
    foreach ($operation in @('store', 'erase', 'unknown')) {
        Assert-CredentialTest ([string]::IsNullOrEmpty((Invoke-TestCredential $request $operation))) 'Non-get operations must not read or emit credentials.'
    }
    Assert-CredentialTest ($script:readCount -eq $readsBeforeRejected) 'Rejected requests must not read user secrets.'

    $script:userToken = 'synthetic-port'
    Assert-CredentialTest ((Invoke-TestCredential "protocol=https`nhost=github.com:443`n`n") -match 'password=synthetic-port') 'The explicit HTTPS port must work.'
    $script:userToken = "synthetic`nextra=value"
    $rejected = $false
    try { $null = Invoke-TestCredential $request } catch { $rejected = $true }
    Assert-CredentialTest $rejected 'Newlines in credentials must fail before output.'
} finally {
    [Environment]::SetEnvironmentVariable('GH_TOKEN', $savedProcessToken, 'Process')
}

Write-Output 'GitHub user credential helper tests passed.'
