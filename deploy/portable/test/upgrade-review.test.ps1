#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference='Stop'
$portable=Split-Path -Parent $PSScriptRoot
$repo=Split-Path -Parent (Split-Path -Parent $portable)
$work=Join-Path ([IO.Path]::GetTempPath()) ('upgrade review 中文 '+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
. (Join-Path $portable 'oss-release-common.ps1')
$checks=New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok,[string]$Label) { if (-not $Ok) { throw ('FAILED: '+$Label) }; $checks.Add($Label) }
function Text([string]$Path,[string]$Value) {
    New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force | Out-Null
    [IO.File]::WriteAllText($Path,$Value,(New-Object Text.UTF8Encoding($true)))
}
function Entry([string]$Directory,[string]$Relative) {
    $file=Join-Path $Directory $Relative
    return [pscustomobject]@{path=$Relative;sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant();bytes=(Get-Item -LiteralPath $file).Length}
}
function Fail-Message([scriptblock]$Action) { try { & $Action | Out-Null } catch { return $_.Exception.Message }; throw 'Expected failure' }
$shell=(Get-Process -Id $PID).Path
function Run-Script([string]$Script,[string[]]$Arguments,[string]$Log,[string]$Engine=$shell) {
    $start=New-Object Diagnostics.ProcessStartInfo
    $start.FileName=$Engine; $start.UseShellExecute=$false; $start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
    $start.StandardOutputEncoding=[Text.Encoding]::UTF8; $start.StandardErrorEncoding=[Text.Encoding]::UTF8
    $tokens=@("'"+$Script.Replace("'","''")+"'")
    foreach ($value in $Arguments) {
        if ($value -match '^-[A-Za-z]+$') { $tokens+=$value }
        else { $tokens+=("'"+$value.Replace("'","''")+"'") }
    }
    $command="[Console]::OutputEncoding=New-Object Text.UTF8Encoding(`$false); `$ErrorActionPreference='Stop'; try { & "+($tokens -join ' ')+"; exit 0 } catch { [Console]::Error.WriteLine(`$_.Exception.ToString()); exit 1 }"
    $start.Arguments='-NoProfile -ExecutionPolicy Bypass -OutputFormat Text -EncodedCommand '+[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($command))
    $process=New-Object Diagnostics.Process; $process.StartInfo=$start
    try {
        $null=$process.Start(); $out=$process.StandardOutput.ReadToEndAsync(); $err=$process.StandardError.ReadToEndAsync()
        $process.WaitForExit(); Text $Log ($out.GetAwaiter().GetResult()+$err.GetAwaiter().GetResult())
        return $process.ExitCode
    } finally { $process.Dispose() }
}
$oldPath=$env:PATH; $oldReadyFail=$env:REVIEW_FAIL_FINAL_READY
try {
    $base=Join-Path $work 'base'; $delta=Join-Path $work 'delta'
    Text (Join-Path $base 'deployment.json') '{"Version":"v1.0.0","Release":"","Database":"fixture","ApiPort":18080,"WebPort":13000,"PublicOrigin":"https://203.0.113.19:13443"}'
    Text (Join-Path $base 'manifest.json') '{"version":"v1.0.0","commit":"111111111"}'
    Text (Join-Path $base 'payload/release/server.exe') 'old'
    New-Item -ItemType Directory -Path (Join-Path $base 'payload/installer') | Out-Null
    Text (Join-Path $base 'payload/bin/u-workers.exe') 'fake CLI never executed'
    Text (Join-Path $base 'payload/runtime/caddy/caddy.exe') 'fake Caddy never executed'
    Text (Join-Path $base 'root-ca.crt') 'fake public CA'
    New-Item -ItemType Directory -Path (Join-Path $base 'data') | Out-Null
    $settings=[ordered]@{version='v1.0.0';profile="office'中文";boardUrl='https://203.0.113.19:13443';httpsPort=13443;lanRequestedHost='203.0.113.19';lanHost='203.0.113.19';lanExposed=$true;lanFirewall='existing';lanFirewallRule='fixture';lanMessage='old'}
    Write-ReleaseJson (Join-Path $base 'data/install-report.json') $settings
    foreach ($name in @('upgrade.ps1','start.ps1')) { Copy-Item (Join-Path $portable $name) (Join-Path $base $name) }
    $doubles=@'
function Trace($Root,$Text) { Add-Content -LiteralPath (Join-Path $Root 'trace.txt') -Value $Text }
function Get-PortableDaemonStatus { param($Root,$Profile) return [pscustomobject]@{Running=$false;Detail=[pscustomobject]@{status='stopped'}} }
function Invoke-PortableNativeAction {
    param($Root,$Action,$ExtraArguments)
    Trace $Root $Action
    if ($Action -eq 'Backup') { $b=Join-Path $Root ('backups/native-'+[guid]::NewGuid().ToString('N')); New-Item -ItemType Directory $b -Force | Out-Null; [IO.File]::WriteAllText((Join-Path $b 'database.dump'),'fixture') }
}
function Invoke-PortableSql { param($Root,$Sql,$Database) return '2|3|4' }
function Stop-PortableDaemon { param($Root,$Profile) Trace $Root ('StopDaemon '+$Profile) }
function Start-PortableDaemon { param($Root,$Profile) Trace $Root ('StartDaemon '+$Profile); return @{Running=$true} }
function Stop-PortableCaddy { param($Root) Trace $Root 'StopCaddy' }
function Start-PortableCaddy {
    param($Root,$PublicHost,$HttpsPort,$ApiPort,$WebPort,$LanExposed)
    Trace $Root ('Listener '+$PublicHost+' '+$LanExposed)
    Write-PortableJson (Join-Path $Root 'state/listener.json') @{host=$PublicHost;exposed=[bool]$LanExposed}
}
function Wait-PortableHttp {
    param($Uri,$RootCAPath,$TimeoutSec,$Description)
    if ($env:REVIEW_FAIL_FINAL_READY -eq '1' -and $Description -eq 'the restored HTTPS listener') { throw 'fixture final readiness failed' }
}
function Export-PortableRootCA { throw 'Fixture must contain its public CA' }
function Get-NetTCPConnection { param($LocalPort,$State) return [pscustomobject]@{OwningProcess=$PID} }
function Get-CimInstance { param($ClassName,$Filter) return @{ExecutablePath=(Get-PortableCaddyPath $rootPath);CommandLine=(Get-PortableCaddyfilePath $rootPath)} }
function Get-PortableProcessRecord { param($ProcessId,$ExpectedPath) return @{Pid=$ProcessId;Executable=$ExpectedPath} }
function Test-PortableProcessRecord { param($Record) return $true }
function Test-PortableLocalAddress { param($Address) return $true }
function Get-PortableLanSubnet { param($Address) return '203.0.113.0/24' }
function Get-PortableFirewallSnapshot { return @() }
function Find-PortableFirewallCoverage { param($Rules,$ProgramPath,$Port,$Address) return @{DisplayName='fixture'} }
function Install-PortableFirewallRule { throw 'Unexpected firewall mutation' }
'@
    $common=[IO.File]::ReadAllText((Join-Path $portable 'portable-common.ps1'))
    Text (Join-Path $base 'portable-common.ps1') ($common+"`r`n"+$doubles)
    Text (Join-Path $base 'runtime/native-windows.ps1') @'
param($Action,$Root,$PublicOrigin,$CorsOrigins)
if ($Action -ne 'Configure') { throw 'Unexpected native child' }
$path=Join-Path $Root 'deployment.json'
$cfg=Get-Content -Raw -Encoding UTF8 $path | ConvertFrom-Json
$cfg.PublicOrigin=$PublicOrigin
[IO.File]::WriteAllText($path,($cfg|ConvertTo-Json))
'@
    Text (Join-Path $delta 'manifest.json') '{"version":"v1.0.1","commit":"222222222"}'
    Text (Join-Path $delta 'payload/release/server.exe') 'new'
    $write=@()
    foreach ($name in @('manifest.json','payload/release/server.exe')) {
        $entry=Entry $delta $name
        $entry | Add-Member NoteProperty old_sha256 (Entry $base $name).sha256
        $entry | Add-Member NoteProperty action 'replace'
        $write+=$entry
    }
    $plan=[pscustomobject]@{from='v1.0.0';to='v1.0.1';commit=('2'*40);baselineCommit='111111111';write=$write;keep=@();delete=@()}
    Write-ReleaseJson (Join-Path $delta 'UPGRADE-FILES.json') $plan
    $apply=Join-Path $portable 'Apply-Upgrade.ps1'
    foreach ($bad in @('','   ')) {
        $message=Fail-Message { & $apply -Root $base -PublicOrigin https://localhost:13443 -UpgradeDirectory $bad -CheckOnly }
        Check ($message -match 'UpgradeDirectory.*不能为空') 'blank UpgradeDirectory has an explicit parameter diagnostic'
    }
    $emptyDelta=Join-Path $work 'empty-delta'; New-Item -ItemType Directory $emptyDelta | Out-Null
    $message=Fail-Message { & $apply -Root $base -PublicOrigin https://localhost:13443 -UpgradeDirectory $emptyDelta -CheckOnly }
    Check ($message -match 'UPGRADE-FILES.json') 'missing delta manifest has a targeted diagnostic'
    Check (-not (Test-Path (Join-Path $base 'trace.txt'))) 'invalid directory diagnostics precede lifecycle actions'

    foreach ($mode in @('lan','local','explicit-local','pending-lan','ready-failure')) {
        $root=Join-Path $work $mode; Copy-Item -LiteralPath $base -Destination $root -Recurse
        $origin='https://203.0.113.19:13443'
        if ($mode -in @('local','pending-lan')) {
            $r=Read-ReleaseJson (Join-Path $root 'data/install-report.json')
            $r.lanExposed=$false; $r.lanHost='127.0.0.1'; $r.boardUrl='https://localhost:13443'
            if ($mode -eq 'local') { $r.lanRequestedHost='127.0.0.1' }
            Write-ReleaseJson (Join-Path $root 'data/install-report.json') $r
            $cfg=Read-ReleaseJson (Join-Path $root 'deployment.json'); $cfg.PublicOrigin=$r.boardUrl
            Write-ReleaseJson (Join-Path $root 'deployment.json') $cfg
            $origin=$r.boardUrl
        }
        $args=@('-Root',$root,'-PublicOrigin',$origin,'-UpgradeDirectory',$delta)
        if ($mode -eq 'explicit-local') { $args+='-NoLanAccess' }
        $env:REVIEW_FAIL_FINAL_READY=if ($mode -eq 'ready-failure') {'1'} else {''}
        $log=Join-Path $work ($mode+'.log')
        $code=Run-Script $apply $args $log
        $report=Read-ReleaseJson (Join-Path $root 'data/install-report.json')
        if ($mode -eq 'ready-failure') {
            Check ($code -ne 0) 'final listener failure returns a nonzero exit code'
            Check ($report.version -eq 'v1.0.0' -and $report.lanRequestedHost -eq '203.0.113.19') 'final readiness failure leaves the original install report untouched'
            Check (-not (Test-Path (Join-Path $root 'data/upgrade-report.json'))) 'temporary loopback upgrade never publishes a final success report'
            Check (@(Get-ChildItem (Join-Path $root 'backups') -Filter files.json -Recurse).Count -eq 1) 'failed final validation retains the file checkpoint'
            continue
        }
        Check ($code -eq 0) ('real Apply and upgrade scripts succeed: '+$mode)
        $final=Read-ReleaseJson (Join-Path $root 'data/upgrade-report.json')
        $expectedLan=$mode -eq 'lan'
        $expectedHost=if ($expectedLan) {'203.0.113.19'} else {'127.0.0.1'}
        Check ($report.lanExposed -eq $expectedLan -and $final.lanExposed -eq $expectedLan -and $final.lanHost -eq $expectedHost) ('final reports match the actual listener: '+$mode)
        Check ($report.version -eq 'v1.0.1' -and $final.toVersion -eq 'v1.0.1' -and (Read-ReleaseJson (Join-Path $root 'deployment.json')).Version -eq 'v1.0.1') ('deployment, manifest and report versions agree: '+$mode)
        Check ($final.boardUrl -eq $report.boardUrl -and $final.boardUrl -eq (Read-ReleaseJson (Join-Path $root 'deployment.json')).PublicOrigin) ('reports display the final service origin: '+$mode)
        $output=Get-Content -Raw -Encoding UTF8 $log
        Check ($output.Contains((Join-Path $root 'payload\bin\u-workers.exe')) -and $output.Contains("--profile 'office''中文' daemon start")) ('success command quotes the exact installation and profile: '+$mode)
        if ($mode -eq 'pending-lan') {
            Check ($report.lanRequestedHost -eq '203.0.113.19') 'temporarily unavailable LAN retains its requested host'
        } else {
            $startLog=Join-Path $work ($mode+'-restart.log')
            Check ((Run-Script (Join-Path $root 'start.ps1') @() $startLog) -eq 0) ('subsequent real Start script succeeds: '+$mode)
            $listener=Read-ReleaseJson (Join-Path $root 'state/listener.json')
            Check ($listener.exposed -eq $expectedLan -and $listener.host -eq $expectedHost) ('subsequent Start preserves the chosen scope: '+$mode)
        }
    }
    $env:REVIEW_FAIL_FINAL_READY=''

    # Exercise the normal upgrader's own install-report version synchronization.
    $direct=Join-Path $work 'direct-upgrade'; Copy-Item -LiteralPath $base -Destination $direct -Recurse
    Copy-Item (Join-Path $delta 'manifest.json') (Join-Path $direct 'manifest.json') -Force
    Check ((Run-Script (Join-Path $direct 'upgrade.ps1') @('-NoLanAccess','-SkipDaemon','-SkipBrowserTrust','-NoStartup','-NoBrowser') (Join-Path $work 'direct.log')) -eq 0) 'direct upgrade script succeeds with isolated lifecycle doubles'
    Check ((Read-ReleaseJson (Join-Path $direct 'data/install-report.json')).version -eq 'v1.0.1') 'direct upgrader synchronizes install-report version after success'
    $overlay=Join-Path $work 'source-upgrade'; Copy-Item -LiteralPath $base -Destination $overlay -Recurse
    $source=Join-Path $work 'full-source'
    Text (Join-Path $source 'manifest.json') '{"version":"v1.0.1","commit":"222222222"}'
    foreach ($name in $script:UpdateToolFiles) { Copy-Item (Join-Path $portable $name) (Join-Path $source $name) }
    Check ((Run-Script (Join-Path $overlay 'upgrade.ps1') @('-Source',$source,'-NoLanAccess','-SkipDaemon','-SkipBrowserTrust','-NoStartup','-NoBrowser') (Join-Path $work 'source-upgrade.log')) -eq 0) 'complete-source root overlay succeeds without executing update tools'
    foreach ($name in $script:UpdateToolFiles) { Assert-ReleaseFile (Join-Path $overlay $name) (Entry $source $name).sha256 }
    Check ($true) 'complete-source overlay carries all five new update tools into the installation'

    # Build an actual release ZIP with a Chinese top-level managed document.
    $document=([string][char]0x64CD+[char]0x4F5C+[char]0x5355+'-'+[char]0x53F0+[char]0x5F0F+[char]0x673A+'.md')
    Text (Join-Path $delta $document) 'Chinese filename round-trip'
    $entry=Entry $delta $document; $entry | Add-Member NoteProperty action 'add'; $plan.write+= $entry
    Write-ReleaseJson (Join-Path $delta 'UPGRADE-FILES.json') $plan
    $published=Join-Path $work 'prepared'
    Check ((Run-Script (Join-Path $portable 'publish-release.ps1') @('-PrepareOnly','-Version','v1.0.1','-UpgradeDirectory',$delta,'-OutputDirectory',$published) (Join-Path $work 'prepare.log') -Engine (Get-Command pwsh.exe).Source) -eq 0) 'real PS7-required PrepareOnly writer builds the Chinese-name release ZIP'
    $expanded=Join-Path $work 'round-trip'; Expand-ReleaseArchive (Join-Path $published 'upgrade.zip') $expanded
    Assert-ReleaseDirectory $expanded (Get-ReleasePlan $expanded)
    Check ((Entry $expanded $document).sha256 -eq $entry.sha256) 'Chinese ZIP name survives extraction and manifest SHA-256 validation'
    # Reproduce the existing CreateFromDirectory call in each engine, unchanged.
    $unicodeSource=Join-Path $work 'unicode-writer'; Text (Join-Path $unicodeSource $document) 'round-trip 中文'
    $unicodeZip=Join-Path $work 'unicode.zip'
    [IO.Compression.ZipFile]::CreateFromDirectory($unicodeSource,$unicodeZip,[IO.Compression.CompressionLevel]::Optimal,$false)
    $zipBytes=[IO.File]::ReadAllBytes($unicodeZip)
    Check (([BitConverter]::ToUInt16($zipBytes,6) -band 2048) -ne 0) 'existing ZIP method sets the UTF-8 name flag for Chinese text in this engine'
    $unicodeOutput=Join-Path $work 'unicode-extracted'; Expand-ReleaseArchive $unicodeZip $unicodeOutput
    Assert-ReleaseFile (Join-Path $unicodeOutput $document) (Entry $unicodeSource $document).sha256
    Check ($true) 'native engine ZIP writer and release extractor preserve Chinese names and hashes'

    # 8,900 real keep files: the final hash must still be checked, with bounded output.
    $large=Join-Path $work 'large'; Copy-Item -LiteralPath $base -Destination $large -Recurse
    $keep=New-Object 'System.Collections.Generic.List[object]'
    $folder=Join-Path $large 'payload/release/large'; New-Item -ItemType Directory $folder | Out-Null
    for ($i=0;$i -lt 8900;$i++) {
        $relative='payload/release/large/'+$i+'.txt'
        [IO.File]::WriteAllText((Join-Path $large $relative),'x')
        $keep.Add([pscustomobject]@{path=$relative;sha256='2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881';bytes=1})
    }
    $plan.keep=$keep.ToArray(); Write-ReleaseJson (Join-Path $delta 'UPGRADE-FILES.json') $plan
    $largeLog=Join-Path $work 'large.log'
    $timer=[Diagnostics.Stopwatch]::StartNew()
    Check ((Run-Script $apply @('-Root',$large,'-PublicOrigin','https://203.0.113.19:13443','-UpgradeDirectory',$delta,'-CheckOnly') $largeLog) -eq 0) 'large CheckOnly validates all 8,900 real keep files'
    $timer.Stop(); $lines=@(Get-Content -Encoding UTF8 $largeLog | Where-Object { $_ -match '^校验安装与差量文件：' })
    Check ($lines[-1] -match '8903/8903' -and $lines.Count -le [math]::Ceiling($timer.Elapsed.TotalSeconds)+2) 'inventory progress is complete and limited to at most once per second plus endpoints'
    Text (Join-Path $large 'payload/release/large/8899.txt') 'corrupt'
    Check ((Run-Script $apply @('-Root',$large,'-PublicOrigin','https://203.0.113.19:13443','-UpgradeDirectory',$delta,'-CheckOnly') (Join-Path $work 'large-corrupt.log')) -ne 0) 'the last large-inventory hash is still enforced'
    Check (-not (Test-Path (Join-Path $large 'trace.txt'))) 'large preflight never touches lifecycle or installation settings'

    # Use real Windows tar with a conflicting executable first on PATH.
    . (Join-Path $repo 'scripts/native-windows.ps1') -Root $work
    $systemTar=Get-NativeWindowsTarPath
    $conflict=Join-Path $work 'git-bin'; New-Item -ItemType Directory $conflict | Out-Null
    Text (Join-Path $conflict 'tar.cs') 'class Tar { static int Main() { return 77; } }'
    & (Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe') /nologo /target:exe ('/out:'+(Join-Path $conflict 'tar.exe')) (Join-Path $conflict 'tar.cs') | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Conflicting tar double failed to compile' }
    $env:PATH=$conflict+';'+$oldPath
    Check ((Get-Command tar.exe).Source -eq (Join-Path $conflict 'tar.exe') -and (Get-NativeWindowsTarPath) -eq $systemTar) 'Windows tar resolution ignores a conflicting tar first on PATH'
    $tarRoot=Join-Path $work 'native backup'; New-Item -ItemType Directory $tarRoot | Out-Null
    Text (Join-Path $tarRoot 'uploads/旧附件.txt') 'attachment round-trip'
    Text (Join-Path $tarRoot 'deployment.json') '{}'
    $cfg=[pscustomobject]@{Root=$tarRoot;Database='fixture';DatabaseUser='fixture';DatabasePassword='FAKE_ONLY';DatabasePort=15432}
    $script:commands=New-Object 'System.Collections.Generic.List[string]'
    function Invoke-NativeCommand {
        param($Executable,$Arguments)
        $script:commands.Add($Executable)
        if ($Executable -eq $systemTar) { & $Executable @Arguments; if ($LASTEXITCODE -ne 0) { throw 'Real tar failed' } }
        elseif ($Executable -like '*pg_dump.exe') { [IO.File]::WriteAllText($Arguments[-1],'fake dump') }
        elseif ($Executable -like '*pg_restore.exe') { return }
        else { throw 'Unexpected native executable' }
    }
    function Start-NativePostgres { param($Config) $script:commands.Add('FakePostgres') }
    function Invoke-NativeSql { param($Config,$Sql,$Database) $script:commands.Add($Sql) }
    Backup-NativeDeployment $cfg
    $backup=Get-ChildItem (Join-Path $tarRoot 'backups') -Directory | Select-Object -First 1
    Remove-Item -LiteralPath (Join-Path $tarRoot 'uploads/旧附件.txt')
    Restore-NativeDeployment $cfg $backup.FullName 'restored_fixture'
    Check ((Get-Content -Raw -Encoding UTF8 (Join-Path $tarRoot 'uploads/旧附件.txt')) -eq 'attachment round-trip') 'native backup and restore round-trip real Chinese-name uploads with Windows tar'
    Check (@($script:commands | Where-Object { $_ -eq $systemTar }).Count -eq 2) 'backup and restore both pass the exact Windows tar path'
    $commandCount=$script:commands.Count; $backupCount=@(Get-ChildItem (Join-Path $tarRoot 'backups') -Directory).Count
    function Get-NativeWindowsTarPath { throw 'Windows archive tool is missing: fixture' }
    foreach ($failureCase in @({ Backup-NativeDeployment $cfg },{ Backup-DockerDeployment $cfg },{ Restore-NativeDeployment $cfg $backup.FullName 'restore_again' })) {
        Check ((Fail-Message $failureCase) -match 'Windows archive tool is missing') 'missing Windows tar is diagnosed before backup or restore mutations'
    }
    Check ($script:commands.Count -eq $commandCount -and @(Get-ChildItem (Join-Path $tarRoot 'backups') -Directory).Count -eq $backupCount) 'missing archive tool never starts database commands or creates a backup directory'
    if ($EvidencePath) { Write-ReleaseJson $EvidencePath @{shell=$PSVersionTable.PSVersion.ToString();passed=$checks.Count;checks=$checks.ToArray();fixture=$work;realServices=$false;realCloud=$false;realComponents='Windows tar, release ZIP writer/extractor, Apply/upgrade/Start scripts; lifecycle/database are doubles'} }
    Write-Host ('PASS: '+$checks.Count+' review checks; fixture: '+$work)
} finally { $env:PATH=$oldPath; $env:REVIEW_FAIL_FINAL_READY=$oldReadyFail }
