#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference = 'Stop'
$portable = Split-Path -Parent $PSScriptRoot
. (Join-Path $portable 'oss-release-common.ps1')
$work = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-full-test-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
$checks = New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok,[string]$Label) { if (-not $Ok) { throw $Label }; $checks.Add($Label) }
function Fail([scriptblock]$Action,[string]$Label) { $failed=$false; try { & $Action | Out-Null } catch { $failed=$true }; Check $failed $Label }
function Text([string]$Path,[string]$Value) { New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force | Out-Null; [IO.File]::WriteAllText($Path,$Value,(New-Object Text.UTF8Encoding($false))) }
$saved=@{}
foreach ($name in @('OSS_ACCESS_KEY_ID','OSS_ACCESS_KEY_SECRET','OSS_REGION','OSS_ENDPOINT','UWORKERS_OSS_BUCKET','FAKE_STORE','FAKE_FAIL_KEY','FAKE_CORRUPT_KEY')) { $saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
try {
    $fake=Join-Path $work 'ossutil.exe'
    & "$env:WINDIR/Microsoft.NET/Framework64/v4.0.30319/csc.exe" /nologo /target:exe ('/out:'+$fake) (Join-Path $PSScriptRoot 'fake-ossutil.cs') | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Fake compile failed' }
    $env:OSS_ACCESS_KEY_ID='FAKE_ONLY'; $env:OSS_ACCESS_KEY_SECRET='FAKE_ONLY'
    $env:OSS_REGION='cn-shanghai'; $env:OSS_ENDPOINT='https://oss-cn-shanghai.aliyuncs.com'; $env:UWORKERS_OSS_BUCKET='fake-bucket'
    $env:FAKE_STORE=Join-Path $work 'store'; $env:FAKE_FAIL_KEY=''; $env:FAKE_CORRUPT_KEY=''
    $package=Join-Path $work 'package'; $output=Join-Path $work 'prepared'; $version='v1.2.3'; $commit='3333333333333333333333333333333333333333'
    foreach ($path in @('upgrade.ps1','portable-common.ps1','payload/release/server.exe','payload/release/migrate.exe','payload/release/local-auth.exe','payload/release/web/apps/web/server.js','payload/release/web/apps/web/.next/BUILD_ID','payload/bin/u-workers.exe','payload/runtime/node.exe','payload/runtime/pgsql/bin/psql.exe','payload/runtime/caddy/caddy.exe','payload/runtime/opencode/opencode.exe','payload/scripts/native-windows.ps1','payload/installer/u-workers-setup-v1.2.3-windows-amd64.exe')) { Text (Join-Path $package $path) 'fixture' }
    Text (Join-Path $package 'manifest.json') '{"version":"v1.2.3","commit":"333333333"}'
    Text (Join-Path $package '.env') 'FAKE_EXCLUDED'
    Text (Join-Path $package 'data/private.txt') 'FAKE_EXCLUDED'
    & pwsh.exe -NoProfile -File (Join-Path $portable 'build-full-release.ps1') -PackageDirectory $package -OutputDirectory $output -Version $version -Commit $commit -Ossutil $fake | Out-Null
    Check ($LASTEXITCODE -eq 0) 'full package prepares and verifies compressed contents'
    $archive=Join-Path $output 'archive'
    Check (-not (Test-Path (Join-Path $archive '.env')) -and -not (Test-Path (Join-Path $archive 'data'))) 'private root files excluded'
    $release=Read-ReleaseJson (Join-Path $output 'release.json'); Assert-ReleaseManifest $release $version
    Check ($release.packageType -eq 'full' -and $release.tag -eq $version -and $release.commit -eq $commit) 'full manifest pins version tag and complete commit'
    $config=Get-ReleaseOssConfig
    foreach ($name in @('full.zip','UPGRADE.md','release.json','latest.json')) {
        $key=if ($name -eq 'latest.json') {'channels/stable/latest.json'} else {'releases/'+$version+'/'+$name}
        Invoke-ReleaseObject $fake $config put $key (Join-Path $output $name) -Mutable:($name -eq 'latest.json')
    }
    $calls=Get-Content (Join-Path $env:FAKE_STORE 'calls.txt')
    Check (($calls -join '|') -eq 'put releases/v1.2.3/full.zip true|put releases/v1.2.3/UPGRADE.md true|put releases/v1.2.3/release.json true|put channels/stable/latest.json false') 'full PUTs forbid overwrite and latest is last; no reads'
    Fail { Invoke-ReleaseObject $fake $config put 'releases/v1.2.3/full.zip' (Join-Path $output 'full.zip') } 'duplicate full ZIP rejected'
    $fetch=Join-Path $portable 'Fetch-Upgrade.ps1'
    foreach ($v in @('v1.2.3','latest')) {
        $dest=Join-Path $work ('fetch-'+$v)
        & $fetch -Version $v -DestinationDirectory $dest -Ossutil $fake | Out-Null
        $result=Read-ReleaseJson (Join-Path $dest 'fetch-result.json')
        Check ($result.verified -and -not $result.applied -and $result.packageType -eq 'full') ('full fetch '+$v+' verifies without lifecycle operations')
    }
    $env:FAKE_CORRUPT_KEY='releases/v1.2.3/full.zip'
    Fail { & $fetch -Version latest -DestinationDirectory (Join-Path $work 'corrupt') -Ossutil $fake } 'corrupt full ZIP rejected'
    $env:FAKE_CORRUPT_KEY=''
    Fail { & $fetch -Version latest -DestinationDirectory (Join-Path $work 'apply') -Ossutil $fake -Apply -Root (Join-Path $work 'absent-installation') -Profile fixture -PublicOrigin https://localhost } 'full package never enters differential Apply'
    Check (-not (Test-Path (Join-Path $work 'absent-installation'))) 'full Apply rejection leaves installation untouched'
    $bad=Read-ReleaseJson (Join-Path $output 'release.json'); $bad.commit='4444444444444444444444444444444444444444'
    Fail { Assert-FullReleaseDirectory $archive $version $bad.commit } 'full inventory commit mismatch rejected'
    $bad.tag='v1.2.4'; Fail { Assert-ReleaseManifest $bad $version } 'tag mismatch rejected'
    $bad.schemaVersion=1; Fail { Assert-ReleaseManifest $bad $version } 'full package cannot masquerade as schema 1 delta'
    Text (Join-Path $archive 'payload/release/unknown.txt') 'unlisted'
    Fail { Assert-FullReleaseDirectory $archive $version $commit } 'unlisted archive file rejected'
    Text (Join-Path $archive 'payload/bin/u-workers.exe') 'tampered'
    Fail { Assert-FullReleaseDirectory $archive $version $commit } 'changed full program hash rejected'
    # Exercise the real publishing orchestrator against a fake source builder.
    # Git identity here belongs only to an isolated test fixture with no remote.
    $source=Join-Path $work 'source'; $sourcePortable=Join-Path $source 'deploy/portable'
    New-Item -ItemType Directory -Path $sourcePortable,(Join-Path $source 'docs') -Force | Out-Null
    foreach ($name in @('build-full-release.ps1','Fetch-Upgrade.ps1','Apply-Upgrade.ps1','oss-release-common.ps1')) { Copy-Item (Join-Path $portable $name) (Join-Path $sourcePortable $name) }
    Copy-Item (Join-Path $portable '../../docs/windows-full-package-upgrade.md') (Join-Path $source 'docs/windows-full-package-upgrade.md')
    $builder=@'
param($Version,$OutputRoot,[switch]$SkipBuildKit,[switch]$NoZip)
$target=Join-Path $OutputRoot ('UWorkers-Portable-'+$Version)
New-Item -ItemType Directory -Path $target -Force | Out-Null
Copy-Item '__PACKAGE__/*' $target -Recurse
$commit=(& git -C (Join-Path $PSScriptRoot '../..') rev-parse --short=9 HEAD | Out-String).Trim()
[IO.File]::WriteAllText((Join-Path $target 'manifest.json'),(@{version=$Version;commit=$commit}|ConvertTo-Json))
'@
    Text (Join-Path $sourcePortable 'build-package.ps1') $builder.Replace('__PACKAGE__',$package.Replace("'","''"))
    & git -C $source init --quiet
    & git -C $source add -- deploy docs
    & git -C $source -c user.name=Fixture -c user.email=fixture@example.invalid -c commit.gpgsign=false commit --quiet -m fixture
    if ($LASTEXITCODE -ne 0) { throw 'Fixture commit failed' }
    & git -C $source tag v1.2.3
    foreach ($failKey in @('full.zip','UPGRADE.md','release.json','latest.json','success')) {
        $env:FAKE_STORE=Join-Path $work ('orchestrator-'+$failKey.Replace('.','-'))
        $latest=Join-Path $env:FAKE_STORE 'channels/stable/latest.json'; Text $latest 'UNCHANGED'
        $env:FAKE_FAIL_KEY=if ($failKey -eq 'success') {''} elseif ($failKey -eq 'latest.json') {'channels/stable/latest.json'} else {'releases/v1.2.3/'+$failKey}
        $out=Join-Path $work ('publish-'+$failKey.Replace('.','-'))
        $start=New-Object Diagnostics.ProcessStartInfo
        $start.FileName=(Get-Command pwsh.exe).Source; $start.UseShellExecute=$false; $start.CreateNoWindow=$true
        $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
        $args=@('-NoProfile','-File',(Join-Path $portable 'publish-release.ps1'),'-Full','-Version','v1.2.3','-SourceDirectory',$source,'-OutputDirectory',$out,'-Ossutil',$fake)
        $start.Arguments=(($args | ForEach-Object { ConvertTo-ReleaseNativeArgument $_ }) -join ' ')
        $process=New-Object Diagnostics.Process; $process.StartInfo=$start; $null=$process.Start()
        $stdout=$process.StandardOutput.ReadToEndAsync(); $stderr=$process.StandardError.ReadToEndAsync()
        $process.WaitForExit(); $log=$stdout.GetAwaiter().GetResult()+$stderr.GetAwaiter().GetResult()
        $exitCode=$process.ExitCode; $process.Dispose()
        Check (-not (($log|Out-String) -match 'LTAI_FAKE_PRIVATE_ID|FAKE_SECRET_DO_NOT_LOG|FAKE_SIGNATURE')) 'full orchestrator suppresses tool secrets'
        if ($failKey -eq 'success') {
            if ($exitCode -ne 0) { throw ('Fixture full publish failed: '+$log) }
            Check ($exitCode -eq 0) 'full source publish orchestrator succeeds'
            $result=Read-ReleaseJson (Join-Path $out 'full/publish-result.json')
            Check ($result.published -and $result.completedObjects.Count -eq 4 -and $result.sourceMode -eq 'source-build-full') 'full publish records all objects and source mode'
        } else {
            Check ($exitCode -ne 0 -and (Get-Content -Raw $latest) -eq 'UNCHANGED') ('full upload failure '+$failKey+' preserves latest')
            $expectedCalls=1+[array]::IndexOf(@('full.zip','UPGRADE.md','release.json','latest.json'),$failKey)
            Check (@(Get-Content (Join-Path $env:FAKE_STORE 'calls.txt')).Count -eq $expectedCalls) ('failure reached expected upload step '+$failKey)
        }
    }
    $result=@{powershell=$PSVersionTable.PSVersion.ToString();passed=$checks.Count;checks=@($checks);transport='local fake; no cloud or service access'}
    if ($EvidencePath) { Write-ReleaseJson $EvidencePath $result }
    $result | ConvertTo-Json -Depth 5
} finally {
    foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') }
}
