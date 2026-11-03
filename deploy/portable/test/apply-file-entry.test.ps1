#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference='Stop'
$portable=Split-Path -Parent $PSScriptRoot
. (Join-Path $portable 'oss-release-common.ps1')
$work=Join-Path ([IO.Path]::GetTempPath()) ('apply file entry '+[char]0x4e2d+[char]0x6587+' '+[guid]::NewGuid().ToString('N'))
$root=Join-Path $work 'installed';$delta=Join-Path $work 'delta'
New-Item -ItemType Directory -Path $root,$delta -Force | Out-Null
foreach($relative in @('payload/release','payload/installer')){New-Item -ItemType Directory -Path (Join-Path $root $relative) -Force|Out-Null}
function Text([string]$Path,[string]$Value){New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force|Out-Null;[IO.File]::WriteAllText($Path,$Value,[Text.UTF8Encoding]::new($false))}
Text (Join-Path $root 'deployment.json') '{"Version":"v1.0.0"}'
Text (Join-Path $root 'manifest.json') '{"version":"v1.0.0","commit":"111111111"}'
Text (Join-Path $delta 'manifest.json') '{"version":"v1.0.1","commit":"222222222"}'
$old=(Get-FileHash (Join-Path $root 'manifest.json') -Algorithm SHA256).Hash
$entry=@{path='manifest.json';action='replace';old_sha256=$old;sha256=(Get-FileHash (Join-Path $delta 'manifest.json') -Algorithm SHA256).Hash;bytes=(Get-Item (Join-Path $delta 'manifest.json')).Length}
Write-ReleaseJson (Join-Path $delta 'UPGRADE-FILES.json') @{from='v1.0.0';to='v1.0.1';commit=('2'*40);baselineCommit='111111111';write=@($entry);keep=@();delete=@()}
foreach($name in @('Apply-Upgrade.ps1','oss-release-common.ps1')){Copy-Item (Join-Path $portable $name) (Join-Path $delta $name)}
$checks=[Collections.Generic.List[string]]::new()
foreach($explicit in @($false,$true)){
    $args=@('-NoProfile','-ExecutionPolicy','Bypass','-File',(Join-Path $delta 'Apply-Upgrade.ps1'),'-Root',$root,'-PublicOrigin','https://localhost:13443','-Profile','fixture','-CheckOnly')
    if($explicit){$args+=@('-UpgradeDirectory',$delta)}
    $start=New-Object Diagnostics.ProcessStartInfo
    $start.FileName=(Get-Process -Id $PID).Path;$start.UseShellExecute=$false;$start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    $start.Arguments=(($args|ForEach-Object {ConvertTo-ReleaseNativeArgument $_}) -join ' ')
    $process=New-Object Diagnostics.Process;$process.StartInfo=$start
    try{
        $null=$process.Start();$out=$process.StandardOutput.ReadToEndAsync();$err=$process.StandardError.ReadToEndAsync()
        $process.WaitForExit();$log=$out.GetAwaiter().GetResult()+$err.GetAwaiter().GetResult()
        Text (Join-Path $work ('file-entry-'+$explicit+'.log')) $log
        if($process.ExitCode -ne 0 -or -not $log.Contains('Read-only preflight passed.')){throw ('-File preflight failed; explicit directory='+$explicit+'; '+$log)}
        $checks.Add(('real -File preflight succeeds; explicit directory='+$explicit))
    }finally{$process.Dispose()}
}
if((Get-FileHash (Join-Path $root 'manifest.json') -Algorithm SHA256).Hash -ne $old -or (Test-Path (Join-Path $root 'backups'))){throw 'Read-only entry changed installation.'}
$checks.Add('default and explicit entries leave installation untouched')
$failed=$false
try{& (Join-Path $delta 'Apply-Upgrade.ps1') -Root $root -PublicOrigin https://localhost:13443 -UpgradeDirectory '   ' -CheckOnly|Out-Null}catch{$failed=$_.Exception.Message -match 'UpgradeDirectory'}
if(-not $failed){throw 'Explicit whitespace directory did not retain its diagnostic.'}
$checks.Add('explicit whitespace retains parameter diagnostic')
$result=@{passed=$checks.Count;checks=@($checks.ToArray());shell=$PSVersionTable.PSVersion.ToString();fixture=$work;realServices=$false}
if($EvidencePath){Write-ReleaseJson $EvidencePath $result}
Write-Host ('Apply -File entry tests passed: '+$checks.Count)
