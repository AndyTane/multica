param(
    [Parameter(Mandatory = $true)][string]$WebOrigin,
    [Parameter(Mandatory = $true)][string]$AccessLog,
    [string]$Profile = '',
    [string]$CliPath = (Join-Path $PSScriptRoot '../server/bin/multica.exe'),
    [string]$BridgePath = (Join-Path $PSScriptRoot '../server/bin/multica-dsh-web.exe')
)

$ErrorActionPreference = 'Stop'
$origin = [Uri]$WebOrigin
if ($origin.Scheme -ne 'http' -or $origin.Host -notin @('127.0.0.1', 'localhost', '[::1]', '::1') -or
    $origin.UserInfo -or $origin.Query -or $origin.Fragment -or $origin.AbsolutePath -ne '/') {
    throw 'Use the local DSH homepage URL without a launch token.'
}
$AccessLog = (Resolve-Path -LiteralPath $AccessLog).Path
$CliPath = (Resolve-Path -LiteralPath $CliPath).Path
$BridgePath = (Resolve-Path -LiteralPath $BridgePath).Path
# Same config-root resolution as the CLI (server/internal/brand.ConfigRoot):
# the current directory wins once it holds CLI state (config.json or profiles/),
# an existing legacy ~/.multica is used by an upgraded install, and a machine
# that never ran the product uses ~/.u-workers. A mere legacy directory — the
# Pi runtime creates ~/.multica/pi-sessions — must not move the CLI root.
$userHome = [Environment]::GetFolderPath('UserProfile')
$currentConfigDirectory = Join-Path $userHome '.u-workers'
$legacyConfigDirectory = Join-Path $userHome '.multica'
$currentHasCliState =
    (Test-Path -LiteralPath (Join-Path $currentConfigDirectory 'config.json')) -or
    (Test-Path -LiteralPath (Join-Path $currentConfigDirectory 'profiles'))
$configDirectory = if ($currentHasCliState) { $currentConfigDirectory } elseif (Test-Path -LiteralPath $legacyConfigDirectory) { $legacyConfigDirectory } else { $currentConfigDirectory }
New-Item -ItemType Directory -Path $configDirectory -Force | Out-Null
$configFile = if($env:MULTICA_DSH_WEB_CONFIG){[IO.Path]::GetFullPath($env:MULTICA_DSH_WEB_CONFIG)}else{Join-Path $configDirectory 'dsh-web.json'}
New-Item -ItemType Directory -Path (Split-Path -Parent $configFile) -Force | Out-Null
@{ origin = $origin.GetLeftPart([UriPartial]::Authority); accessLog = $AccessLog } |
    ConvertTo-Json | Set-Content -LiteralPath $configFile -Encoding utf8

& $BridgePath --profile multica --probe
if ($LASTEXITCODE -ne 0) { throw 'DSH web connection check failed.' }
$profileArgs = @()
if ($Profile) { $profileArgs = @('--profile', $Profile) }
$profileOutput = & $CliPath @profileArgs runtime profile list --output json
if ($LASTEXITCODE -ne 0) { throw 'Cannot list Multica runtime profiles.' }
$profiles = $profileOutput | ConvertFrom-Json
$matches = @($profiles | Where-Object { $_.command_name -eq 'multica-dsh-web' -and $_.protocol_family -eq 'dsh' })
if ($matches.Count -gt 1) { throw 'Multiple DSH web profiles exist; select one explicitly in Multica.' }
if ($matches.Count -eq 0) {
    $runtimeProfile = & $CliPath @profileArgs runtime profile create --protocol-family dsh --command-name multica-dsh-web --display-name 'DSH Web' --output json | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0) { throw 'Cannot create the DSH web runtime profile.' }
} else { $runtimeProfile = $matches[0] }
& $CliPath @profileArgs runtime profile set-path $runtimeProfile.id --path $BridgePath
if ($LASTEXITCODE -ne 0) { throw 'Cannot pin the DSH web bridge executable.' }
Write-Output "DSH web profile: $($runtimeProfile.id)"
Write-Output 'Restart the daemon, then bind the DSH agent to this runtime.'
