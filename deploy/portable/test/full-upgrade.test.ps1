#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference='Stop'
$portable=Split-Path -Parent $PSScriptRoot
. (Join-Path $portable 'oss-release-common.ps1')
$work=Join-Path ([IO.Path]::GetTempPath()) ('full-upgrade-'+[guid]::NewGuid().ToString('N'))
$source=Join-Path $work 'full';$root=Join-Path $work 'installed'
$checks=New-Object 'System.Collections.Generic.List[string]'
function Text($Path,$Value){New-Item -ItemType Directory (Split-Path -Parent $Path) -Force|Out-Null;[IO.File]::WriteAllText($Path,$Value,(New-Object Text.UTF8Encoding($true)))}
function Check($Ok,$Label){if(-not $Ok){throw $Label};$checks.Add($Label)}
function Inventory {
 $files=@(Get-ReleaseFiles $source|Where-Object {$_.Name-ne 'FULL-FILES.json'}|ForEach-Object {@{path=$_.FullName.Substring($source.Length+1).Replace('\','/');bytes=$_.Length;sha256=(Get-FileHash $_.FullName).Hash.ToLowerInvariant()}})
 Write-ReleaseJson (Join-Path $source 'FULL-FILES.json') @{schemaVersion=1;version='v1.2.3';commit=('3'*40);files=$files}
}
function Run-Upgrade($Arguments) {
 $shell=(Get-Process -Id $PID).Path
 $info=New-Object Diagnostics.ProcessStartInfo
 $info.FileName=$shell;$info.UseShellExecute=$false;$info.CreateNoWindow=$true;$info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
 $argv=@('-NoProfile','-ExecutionPolicy','Bypass','-File',(Join-Path $source 'upgrade.ps1'))+$Arguments
 $info.Arguments=(($argv|ForEach-Object {ConvertTo-ReleaseNativeArgument $_})-join ' ')
 $process=New-Object Diagnostics.Process;$process.StartInfo=$info
 try{$null=$process.Start();$out=$process.StandardOutput.ReadToEndAsync();$err=$process.StandardError.ReadToEndAsync();$process.WaitForExit();$message=$out.GetAwaiter().GetResult()+$err.GetAwaiter().GetResult();if($process.ExitCode-ne 0){return $message};return ''}finally{$process.Dispose()}
}
try {
 $required=@('manifest.json','upgrade.ps1','portable-common.ps1','UPGRADE.md','payload/release/server.exe','payload/release/migrate.exe','payload/release/local-auth.exe','payload/release/web/apps/web/server.js','payload/release/web/apps/web/.next/BUILD_ID','payload/bin/u-workers.exe','payload/runtime/node.exe','payload/runtime/pgsql/bin/psql.exe','payload/runtime/caddy/caddy.exe','payload/runtime/opencode/opencode.exe','payload/scripts/native-windows.ps1','payload/installer/u-workers-setup-v1.2.3-windows-amd64.exe','download-tools/Fetch-Upgrade.ps1','download-tools/Apply-Upgrade.ps1','download-tools/oss-release-common.ps1','download-tools/ossutil.exe')
 foreach($name in @($required)+@($script:ReleaseProgramFiles)){Text (Join-Path $source $name) 'fixture'}
 Copy-Item (Join-Path $portable 'upgrade.ps1') (Join-Path $source 'upgrade.ps1') -Force
 Copy-Item (Join-Path $portable 'oss-release-common.ps1') (Join-Path $source 'oss-release-common.ps1') -Force
 $doubles=@'
function Invoke-PortableNativeAction {
 param($Root,$Action,$ExtraArguments)
 Add-Content (Join-Path $Root 'trace.txt') $Action
 if($Action-eq 'Backup'){
  if(Test-Path (Join-Path $Root 'fail-backup')){throw 'fixture backup failure'}
  $b=Join-Path $Root 'backups/native';New-Item -ItemType Directory $b -Force|Out-Null
  [IO.File]::WriteAllText((Join-Path $b 'database.dump'),'fixture')
 }
 return ''
}
function Invoke-PortableSql { return '1|2|3' }
function Stop-PortableDaemon { return @{ExitCode=0} }
function Stop-PortableCaddy {}
function Start-PortableCaddy {}
function Get-PortableHttpsPortFromConfig { return 13443 }
function Resolve-PortableLanPlan { return @{LanHost='127.0.0.1';Exposed=$false;Message='fixture'} }
function Wait-PortableHttp {}
function Get-PortableDaemonStartCommand { return 'fixture stopped daemon' }
function Update-PortableLanReport {}
'@
 Text (Join-Path $source 'portable-common.ps1') ([IO.File]::ReadAllText((Join-Path $portable 'portable-common.ps1'))+"`r`n"+$doubles)
 Text (Join-Path $source 'manifest.json') '{"version":"v1.2.3","commit":"333333333"}'
 Text (Join-Path $root 'deployment.json') '{"Version":"v1.2.2","Release":"","Database":"original","ApiPort":18080,"WebPort":13000}'
 Text (Join-Path $root 'root-ca.crt') 'original public CA'
 Text (Join-Path $root 'runtime/pgsql/bin/psql.exe') 'fixture'
 Text (Join-Path $root 'runtime/native-windows.ps1') 'original runner'
 Text (Join-Path $root 'manifest.json') 'original manifest'
 Inventory
 $argv=@('-Root',$root,'-Source',$source,'-Profile','original','-NoLanAccess','-SkipDaemon','-SkipBrowserTrust','-NoStartup','-NoBrowser')
 $message=Run-Upgrade @('-Root',$root)
 Check ($message-match 'requires.*Source') 'external root requires full source'
 $message=Run-Upgrade @('-Root',$root,'-Source',$root)
 Check ($message-match 'from the full Source') 'external root pins the executing full source'
 $message=Run-Upgrade @('-Root',$source,'-Source',$source)
 Check ($message-match 'non-nested') 'overlapping source and installation rejected'
 Text (Join-Path $source 'payload/release/server.exe') 'changed'
 $message=Run-Upgrade $argv
 Check ($message-match 'SHA-256|hash|size') 'tampered full package rejected before lifecycle'
 Inventory
 Text (Join-Path $root 'runtime/pgsql/bin/psql.exe') 'different PG'
 $message=Run-Upgrade $argv
 Check ($message-match 'SHA-256|hash|size') 'retained PostgreSQL mismatch rejected before lifecycle'
 Check (-not(Test-Path (Join-Path $root 'trace.txt'))) 'failed preflight never starts or backs up installation'
 Text (Join-Path $root 'runtime/pgsql/bin/psql.exe') 'fixture'
 Text (Join-Path $root 'fail-backup') 'fixture'
 $message=Run-Upgrade $argv
 Check ($message-match 'fixture backup failure') 'backup failure aborts full upgrade'
 Check ((Get-Content (Join-Path $root 'manifest.json') -Raw)-eq 'original manifest') 'backup failure preserves original program'
 Remove-Item -LiteralPath (Join-Path $root 'fail-backup')
 $message=Run-Upgrade $argv
 Check (-not $message) ('complete external Source upgrade succeeds: '+$message)
 foreach($name in @($script:ReleaseProgramFiles)|Where-Object {Test-Path (Join-Path $source $_)}){
  Check ((Get-FileHash (Join-Path $root $name)).Hash-eq (Get-FileHash (Join-Path $source $name)).Hash) ('root file delivered: '+$name)
 }
 Check ((Get-FileHash (Join-Path $root 'runtime/native-windows.ps1')).Hash-eq (Get-FileHash (Join-Path $source 'payload/scripts/native-windows.ps1')).Hash) 'lifecycle runner follows delivered release'
 Check ((Get-Content (Join-Path $root 'deployment.json') -Raw|ConvertFrom-Json).Database-eq 'original') 'business database selection preserved'
 if($EvidencePath){Write-ReleaseJson $EvidencePath @{passed=$true;count=$checks.Count;checks=@($checks)}}
 Write-Host ($checks.Count.ToString()+' full Source upgrade checks passed')
}finally{
 $absolute=[IO.Path]::GetFullPath($work)
 if($absolute.StartsWith(([IO.Path]::GetTempPath()),[StringComparison]::OrdinalIgnoreCase)){Remove-Item -LiteralPath $absolute -Recurse -Force}
}
