#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference='Stop'
$portable=Split-Path -Parent $PSScriptRoot
. (Join-Path $portable 'oss-release-common.ps1')
$work=Join-Path ([IO.Path]::GetTempPath()) ('entry 中文 '+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
$checks=New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok,[string]$Label) { if (-not $Ok) { throw $Label }; $checks.Add($Label) }
foreach ($path in @('Update.ps1','Fetch-Upgrade.ps1','Apply-Upgrade.ps1','oss-release-common.ps1','../client-kit/client.ps1','../client-kit/client-settings.ps1')) {
    $tokens=$null; $errors=$null
    $null=[Management.Automation.Language.Parser]::ParseFile((Join-Path $portable $path),[ref]$tokens,[ref]$errors)
    Check (@($errors).Count -eq 0) ('PowerShell parser accepts '+$path)
}
$zip=& (Join-Path $portable 'build-fetch-bootstrap.ps1') -OutputDirectory (Join-Path $work 'bootstrap output')
Expand-ReleaseArchive $zip (Join-Path $work 'extracted')
$bootstrap=Join-Path $work 'extracted/UWorkers-Fetch'
foreach ($name in @('Update.cmd','Update.ps1','Fetch-Upgrade.ps1','Apply-Upgrade.ps1','oss-release-common.ps1','UPGRADE.md','README.md','CHECKSUMS.sha256')) {
    Check (Test-Path -LiteralPath (Join-Path $bootstrap $name) -PathType Leaf) ('bootstrap contains '+$name)
}
foreach ($line in Get-Content -LiteralPath (Join-Path $bootstrap 'CHECKSUMS.sha256')) {
    $parts=$line -split '  ',2
    Check ((Get-FileHash -LiteralPath (Join-Path $bootstrap $parts[1]) -Algorithm SHA256).Hash -eq $parts[0]) ('bootstrap checksum matches '+$parts[1])
}
$start=New-Object Diagnostics.ProcessStartInfo
$start.FileName=$env:ComSpec; $start.UseShellExecute=$false; $start.CreateNoWindow=$true
$start.EnvironmentVariables['UWORKERS_NO_PAUSE']='1'
$start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
$start.Arguments='/d /c ""'+(Join-Path $bootstrap 'Update.cmd')+'" -NonInteractive"'
$proc=New-Object Diagnostics.Process; $proc.StartInfo=$start; $null=$proc.Start()
$out=$proc.StandardOutput.ReadToEndAsync(); $err=$proc.StandardError.ReadToEndAsync(); $proc.WaitForExit()
$null=$out.GetAwaiter().GetResult(); $null=$err.GetAwaiter().GetResult()
Check ($proc.ExitCode -ne 0) 'Update.cmd returns nonzero on missing parameters in Unicode path'
$proc.Dispose()
# Production daemon JSON omits port_conflict in the normal stopped case.
Assert-ReleaseDaemonStopped ([pscustomobject]@{Running=$false;Detail=[pscustomobject]@{status='stopped'}})
Check ($true) 'normal stopped daemon JSON works without optional conflict field'
$result=@{powershell=$PSVersionTable.PSVersion.ToString();passed=$checks.Count;checks=@($checks);transport='parser, bootstrap and cmd only; no OSS or services'}
if ($EvidencePath) { Write-ReleaseJson $EvidencePath $result }
$result|ConvertTo-Json -Depth 5
