# Create one local identity for this deployment (admin, host-local, idempotent).
#
# The app has no public signup on an offline instance and its email-code flow is
# disabled, so a native deployment needs a host-local way to provision the one
# identity an operator asked for. This is an admin script run on the host; it
# creates no network surface, back door or bypass. Re-running with the same email
# reuses the existing row and never overwrites name, status or credentials.
#
# It mirrors the CreateUser query fields (name, email; database defaults for id,
# timestamps, onboarding) and wraps the check/insert in one transaction.
#
#   powershell -File create-local-identity.ps1 -Email a@b.c -DisplayName "Name" `
#       -IdentityFile <deployment>local-login-identity.json
#
# Passing -IdentityFile also pins that identity for Set-UWorkers-Login.ps1.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Email,
    [Parameter(Mandatory = $true)][string]$DisplayName,
    [string]$Root = 'D:\UWorkers\clawsta80\native',
    [string]$IdentityFile = ''
)

$ErrorActionPreference = 'Stop'
$configPath = Join-Path $Root 'deployment.json'
$config = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
$psql = Join-Path $Root 'runtime\pgsql\bin\psql.exe'
$sqlPath = Join-Path $Root 'create-local-identity.sql'

if ($Email -notmatch '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+$') { throw 'invalid identity email' }
if ([string]::IsNullOrWhiteSpace($DisplayName)) { throw 'display name is required' }
if (-not (Test-Path -LiteralPath $psql)) { throw ('psql not found under ' + $Root) }

function Invoke-Psql {
    param([string]$Database, [string]$Path)
    # psql writes NOTICE lines to stderr; keep those non-terminating.
    $previous = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $text = & $psql -h 127.0.0.1 -p ([int]$config.DatabasePort) -U $config.DatabaseUser -d $Database -v ON_ERROR_STOP=1 -tA -f $Path 2>&1 | Out-String
        return [pscustomobject]@{ ExitCode = $LASTEXITCODE; Output = $text }
    } finally {
        $ErrorActionPreference = $previous
    }
}

$sqlTemplate = @'
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
    RAISE NOTICE 'USER_REUSED';
  ELSE
    INSERT INTO "user" (name, email) VALUES ('__NAME__', '__EMAIL__') RETURNING id INTO existing_id;
    RAISE NOTICE 'USER_CREATED';
  END IF;
END
$do$;
COMMIT;
SELECT id::text || '|' || email || '|' || name || '|' || (onboarded_at IS NOT NULL)::text FROM "user" WHERE lower(email) = lower('__EMAIL__');
'@
$sql = $sqlTemplate.Replace("__EMAIL__", $Email).Replace("__NAME__", $DisplayName)

$saved = $env:PGPASSWORD
try {
    $env:PGPASSWORD = [string]$config.DatabasePassword
    [IO.File]::WriteAllText($sqlPath, $sql, [Text.UTF8Encoding]::new($false))
    $result = Invoke-Psql $config.Database $sqlPath
} finally {
    $env:PGPASSWORD = $saved
    Remove-Item -LiteralPath $sqlPath -Force -ErrorAction SilentlyContinue
}
if ($result.ExitCode -ne 0) { throw ('identity transaction failed: ' + $result.Output.Trim()) }
$rows = @($result.Output -split "`r?`n" | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ -match '\|' })
if ($rows.Count -eq 0) { throw 'identity transaction returned no row' }
$parts = $rows[$rows.Count - 1].Split('|')
$identity = [ordered]@{
    user_id = $parts[0]
    email = $parts[1]
    display_name = $parts[2]
    onboarded = ($parts[3] -eq 't')
}
Write-Output ('IDENTITY_OK user_id=' + $identity.user_id + ' name=' + $identity.display_name + ' onboarded=' + $identity.onboarded)
if ($IdentityFile) {
    $directory = Split-Path -Parent $IdentityFile
    if ($directory -and -not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory -Force | Out-Null }
    [IO.File]::WriteAllText($IdentityFile, ($identity | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    Write-Output ('identity_file=' + $IdentityFile)
}
