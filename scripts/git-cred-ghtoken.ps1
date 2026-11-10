param([string]$Operation)

$ErrorActionPreference = 'Stop'

function Get-GitHubUserToken {
    # Read persistent user settings on every request. Long-lived daemons and
    # agent shell policies can leave process environment variables stale/missing.
    [Environment]::GetEnvironmentVariable('GH_TOKEN', 'User')
}

function Invoke-GitHubUserTokenCredential {
    param(
        [string]$Operation,
        [IO.TextReader]$InputReader = [Console]::In,
        [IO.TextWriter]$OutputWriter = [Console]::Out
    )

    # Git owns the credential protocol. Never persist values received via store.
    if ($Operation -ne 'get') { return }

    $request = @{}
    while ($null -ne ($line = $InputReader.ReadLine()) -and $line.Length -gt 0) {
        $separator = $line.IndexOf('=')
        if ($separator -gt 0) {
            $request[$line.Substring(0, $separator)] = $line.Substring($separator + 1)
        }
    }

    if ($request.protocol -ne 'https' -or $request.host -notin @('github.com', 'github.com:443')) {
        return
    }

    $token = Get-GitHubUserToken
    if ([string]::IsNullOrWhiteSpace($token)) { return }
    if ($token.IndexOfAny([char[]]@("`r", "`n", "`0")) -ge 0) {
        throw 'GH_TOKEN contains an invalid credential-protocol character.'
    }

    # Only Git receives this output. Do not log it or run `git credential fill`
    # directly in an agent transcript.
    $OutputWriter.WriteLine('username=x-access-token')
    $OutputWriter.WriteLine('password=' + $token)
    $OutputWriter.WriteLine()
}

if ($MyInvocation.InvocationName -ne '.') {
    try {
        Invoke-GitHubUserTokenCredential -Operation $Operation
    } catch {
        [Console]::Error.WriteLine('GitHub credential helper could not read a valid Windows user GH_TOKEN.')
        exit 1
    }
}
