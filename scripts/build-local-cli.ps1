param(
    [string]$GoExecutable = 'go',
    [string]$Version
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $repoRoot
try {
    if ($Version) {
        $sourceVersion = $Version
    } else {
        $sourceVersion = git describe --tags --match 'v[0-9]*' --always --dirty
        if ($LASTEXITCODE -ne 0) { throw 'Could not resolve the source version.' }
    }
    if ($sourceVersion -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9]+-g[0-9a-f]+)?(?:-dirty)?$') {
        throw 'Build from a tagged checkout or pass -Version vX.Y.Z matching your self-hosted release.'
    }
    $sourceCommit = git rev-parse --short HEAD
    if ($LASTEXITCODE -ne 0) { throw 'Could not resolve the source commit.' }
    $buildDate = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $cliOutput = Join-Path $repoRoot 'server/bin/u-workers.exe'
    $linkFlags = "-s -w -X main.version=$sourceVersion -X main.commit=$sourceCommit -X main.date=$buildDate"
    Push-Location -LiteralPath (Join-Path $repoRoot 'server')
    try {
        & $GoExecutable build -ldflags $linkFlags -o $cliOutput ./cmd/multica
        if ($LASTEXITCODE -ne 0) { throw 'Local CLI build failed; the running executable was not replaced.' }
    } finally {
        Pop-Location
    }
    & $cliOutput version --output json
    if ($LASTEXITCODE -ne 0) { throw 'Could not verify the built CLI version.' }
} finally {
    Pop-Location
}
