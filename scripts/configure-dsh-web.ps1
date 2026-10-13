param(
    [Parameter(Mandatory = $true)][string]$WebOrigin,
    [Parameter(Mandatory = $true)][string]$AccessLog,
    [Parameter(Mandatory = $true)][string]$DataHome,
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
$DataHome = (Resolve-Path -LiteralPath $DataHome).Path
$CliPath = (Resolve-Path -LiteralPath $CliPath).Path
$BridgePath = (Resolve-Path -LiteralPath $BridgePath).Path
$configDirectory = Join-Path ([Environment]::GetFolderPath('UserProfile')) '.multica'
New-Item -ItemType Directory -Path $configDirectory -Force | Out-Null
$configFile = Join-Path $configDirectory 'dsh-web.json'
@{ origin = $origin.GetLeftPart([UriPartial]::Authority); accessLog = $AccessLog; dataHome = $DataHome } |
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
