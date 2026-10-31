[CmdletBinding()]
param(
    # Native deployment root: the directory that holds deployment.json.
    [string]$Root = (Join-Path $env:LOCALAPPDATA 'Multica\native'),
    # Everything after the named parameters is passed to local-auth verbatim.
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments
)

$ErrorActionPreference = 'Stop'

# Thin entry point for the local password credential tool of a native Windows
# deployment. It locates the release that deployment.json points at and hands the
# tool that same deployment.json, so the operator never passes a database
# connection string or sees one printed.
$configPath = Join-Path $Root 'deployment.json'
if (-not (Test-Path -LiteralPath $configPath)) {
    throw "No native deployment configuration at $configPath. Pass -Root <deployment root>."
}
$config = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
if (-not $config.Release) { throw 'deployment.json has no Release; run the native Build action first.' }
$tool = Join-Path ([string]$config.Release) 'local-auth.exe'
if (-not (Test-Path -LiteralPath $tool)) {
    throw "local-auth.exe is missing from $($config.Release). Rebuild the deployment with -Action Build."
}

if (-not $Arguments -or $Arguments.Count -eq 0) {
    & $tool --deployment-config $configPath
    exit $LASTEXITCODE
}
# The subcommand comes first (local-auth <command> [flags]); the deployment
# config is an ordinary flag of that command, so it is appended after the
# caller's arguments.
& $tool @Arguments --deployment-config $configPath
exit $LASTEXITCODE
