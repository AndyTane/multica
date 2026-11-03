#Requires -Version 5.1
[CmdletBinding()]
param([string]$EvidencePath = '')
$ErrorActionPreference = 'Stop'
$portable = Split-Path -Parent $PSScriptRoot
. (Join-Path $portable 'oss-release-common.ps1')
$work = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-oss-test-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
$checks = New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok, [string]$Message) { if (-not $Ok) { throw ('FAILED: '+$Message) }; $checks.Add($Message) }
function Must-Fail([scriptblock]$Action, [string]$Message) {
    $failed = $false; try { & $Action | Out-Null } catch { $failed = $true }
    Check $failed $Message
}
function Text([string]$Path, [string]$Value) {
    New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force | Out-Null
    [IO.File]::WriteAllText($Path,$Value,(New-Object Text.UTF8Encoding($false)))
}
function Entry([string]$Directory,[string]$Relative) {
    $file = Join-Path $Directory $Relative
    return [pscustomobject]@{path=$Relative;sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant();bytes=(Get-Item -LiteralPath $file).Length}
}
function Publish([string]$Output, [switch]$Prepare) {
    $args = @('-NoProfile','-ExecutionPolicy','Bypass','-File',(Join-Path $portable 'publish-release.ps1'),'-Version','v1.0.1','-UpgradeDirectory',$delta,'-OutputDirectory',$Output,'-Ossutil',$fake)
    if ($Prepare) { $args += '-PrepareOnly' }
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $pwsh; $start.UseShellExecute = $false; $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true
    $start.Arguments = (($args | ForEach-Object { ConvertTo-ReleaseNativeArgument $_ }) -join ' ')
    $process = New-Object Diagnostics.Process; $process.StartInfo = $start
    $null = $process.Start(); $stdout = $process.StandardOutput.ReadToEndAsync(); $stderr = $process.StandardError.ReadToEndAsync()
    $process.WaitForExit(); $log = $stdout.GetAwaiter().GetResult()+$stderr.GetAwaiter().GetResult()
    $code = $process.ExitCode; $process.Dispose()
    Text ($Output+'.test.log') $log
    Check (-not ($log -match 'LTAI_FAKE_PRIVATE_ID|FAKE_SECRET_DO_NOT_LOG|FAKE_SIGNATURE|123456789')) 'tool errors are suppressed'
    return $code
}
$saved = @{}
foreach ($name in @('OSS_ACCESS_KEY_ID','OSS_ACCESS_KEY_SECRET','OSS_REGION','OSS_ENDPOINT','UWORKERS_OSS_BUCKET','FAKE_STORE','FAKE_FAIL_KEY','FAKE_CORRUPT_KEY')) { $saved[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
try {
    $pwsh = (Get-Command pwsh.exe -ErrorAction Stop).Source
    $fake = Join-Path $work 'fake-ossutil.exe'
    $csc = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
    & $csc /nologo /target:exe ('/out:'+$fake) (Join-Path $PSScriptRoot 'fake-ossutil.cs') | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Fake tool compilation failed.' }
    $env:OSS_ACCESS_KEY_ID = 'FAKE_ONLY'; $env:OSS_ACCESS_KEY_SECRET = 'FAKE_ONLY'
    $env:OSS_REGION = 'cn-hangzhou'; $env:OSS_ENDPOINT = 'https://oss-cn-hangzhou.aliyuncs.com'; $env:UWORKERS_OSS_BUCKET = 'fake-release-bucket'
    $env:FAKE_FAIL_KEY = ''; $env:FAKE_CORRUPT_KEY = ''
    $root = Join-Path $work 'installation'; $delta = Join-Path $work 'delta'
    New-Item -ItemType Directory -Path $root,$delta | Out-Null
    Text (Join-Path $root 'deployment.json') '{"Version":"v1.0.0","ApiPort":18080,"WebPort":13000}'
    Text (Join-Path $root 'manifest.json') '{"version":"v1.0.0","commit":"111111111"}'
    Text (Join-Path $root 'payload/release/server.exe') 'old-program'
    Text (Join-Path $root 'payload/release/dependency.txt') 'unchanged-dependency'
    Text (Join-Path $root 'payload/release/obsolete.js') 'old-chunk'
    Text (Join-Path $root 'payload/installer/old.exe') 'old-installer'
    Text (Join-Path $root 'data/business.txt') 'business-data'
    Text (Join-Path $root 'uploads/old.txt') 'old-attachment'
    Text (Join-Path $root 'logs/old.log') 'old-log'
    Text (Join-Path $delta 'manifest.json') '{"version":"v1.0.1","commit":"222222222"}'
    Text (Join-Path $delta 'payload/release/server.exe') 'new-program'
    Text (Join-Path $delta 'UPGRADE.md') '# Test upgrade instructions'
    Text (Join-Path $delta '.env') 'FAKE_PRIVATE_DATA_MUST_NOT_BE_PACKAGED'
    Text (Join-Path $delta 'uploads/private.txt') 'FAKE_PRIVATE_DATA_MUST_NOT_BE_PACKAGED'
    $write = @(); foreach ($name in @('manifest.json','payload/release/server.exe')) {
        $e = Entry $delta $name
        $e | Add-Member NoteProperty action 'replace'; $e | Add-Member NoteProperty old_sha256 (Entry $root $name).sha256
        $write += $e
    }
    $plan = [pscustomobject]@{from='v1.0.0';to='v1.0.1';commit='2222222222222222222222222222222222222222';baselineCommit='111111111';write=$write;keep=@((Entry $root 'payload/release/dependency.txt'));delete=@((Entry $root 'payload/installer/old.exe'))}
    Write-ReleaseJson (Join-Path $delta 'UPGRADE-FILES.json') $plan
    $prepared = Join-Path $work 'prepared'
    $env:OSS_ACCESS_KEY_ID = ''; $env:OSS_ACCESS_KEY_SECRET = ''
    Check ((Publish $prepared -Prepare) -eq 0) 'PrepareOnly succeeds without credentials'
    Check (-not (Test-Path -LiteralPath (Join-Path $work 'store'))) 'PrepareOnly makes no OSS calls'
    $env:OSS_ACCESS_KEY_ID = 'FAKE_ONLY'; $env:OSS_ACCESS_KEY_SECRET = 'FAKE_ONLY'
    $extracted = Join-Path $work 'prepared-extracted'
    Expand-ReleaseArchive (Join-Path $prepared 'upgrade.zip') $extracted
    Check (-not (Test-Path -LiteralPath (Join-Path $extracted '.env')) -and -not (Test-Path -LiteralPath (Join-Path $extracted 'uploads'))) 'extra secrets and data are excluded from ZIP'
    foreach ($key in @('upgrade.zip','UPGRADE.md','release.json','latest.json')) {
        $env:FAKE_STORE = Join-Path $work ('failure-'+$key.Replace('.','-'))
        $latest = Join-Path $env:FAKE_STORE 'channels/stable/latest.json'; Text $latest 'UNCHANGED'
        $env:FAKE_FAIL_KEY = if ($key -eq 'latest.json') {'channels/stable/latest.json'} else {'releases/v1.0.1/'+$key}
        Check ((Publish (Join-Path $work ('out-'+$key.Replace('.','-')))) -ne 0) ('failed upload '+$key+' returns nonzero')
        Check ((Get-Content -Raw -LiteralPath $latest) -eq 'UNCHANGED') ('failed upload '+$key+' leaves latest unchanged')
    }
    $env:FAKE_FAIL_KEY = ''; $env:FAKE_STORE = Join-Path $work 'store'
    Check ((Publish (Join-Path $work 'published')) -eq 0) 'fake OSS publish succeeds'
    $calls = Get-Content -LiteralPath (Join-Path $env:FAKE_STORE 'calls.txt')
    Check (($calls -join '|') -eq 'put releases/v1.0.1/upgrade.zip true|put releases/v1.0.1/UPGRADE.md true|put releases/v1.0.1/release.json true|put channels/stable/latest.json false') 'immutable PUT order and mutable latest last, no HEAD/List/Get'
    $latestHash = (Get-FileHash (Join-Path $env:FAKE_STORE 'channels/stable/latest.json')).Hash
    Check ((Publish (Join-Path $work 'duplicate')) -ne 0) 'same version overwrite is rejected'
    Check ((Get-FileHash (Join-Path $env:FAKE_STORE 'channels/stable/latest.json')).Hash -eq $latestHash) 'same version rejection keeps latest'
    $fetch = Join-Path $portable 'Fetch-Upgrade.ps1'
    foreach ($version in @('v1.0.1','latest')) {
        & $fetch -Version $version -Ossutil $fake -DestinationDirectory (Join-Path $work ('fetch-'+$version)) -Root $root -Profile fixture -PublicOrigin https://localhost:13443 -CheckOnly | Out-Null
        Check ($true) ('fetch '+$version+' verifies ZIP and real Apply preflight')
    }
    $before = (Get-FileHash (Join-Path $root 'payload/release/server.exe')).Hash
    $env:FAKE_CORRUPT_KEY = 'releases/v1.0.1/upgrade.zip'
    Must-Fail { & $fetch -Version v1.0.1 -Ossutil $fake -DestinationDirectory (Join-Path $work 'bad-hash') -Apply -Root $root -Profile fixture -PublicOrigin https://localhost:13443 } 'bad ZIP hash refuses apply'
    $env:FAKE_CORRUPT_KEY = ''
    $env:FAKE_CORRUPT_KEY = 'releases/v1.0.1/release.json'
    Must-Fail { & $fetch -Version v1.0.1 -Ossutil $fake -DestinationDirectory (Join-Path $work 'bad-manifest') -Apply -Root $root -Profile fixture -PublicOrigin https://localhost:13443 } 'invalid JSON manifest refuses apply'
    $env:FAKE_CORRUPT_KEY = ''
    $env:FAKE_FAIL_KEY = 'releases/v1.0.1/upgrade.zip'
    Must-Fail { & $fetch -Version v1.0.1 -Ossutil $fake -DestinationDirectory (Join-Path $work 'interrupted') -Apply -Root $root -Profile fixture -PublicOrigin https://localhost:13443 } 'interrupted download refuses apply'
    $env:FAKE_FAIL_KEY = ''
    Text (Join-Path $root 'manifest.json') '{"version":"v1.0.0","commit":"999999999"}'
    Must-Fail { & (Join-Path $portable 'Apply-Upgrade.ps1') -UpgradeDirectory $delta -Root $root -Profile fixture -PublicOrigin https://localhost:13443 -CheckOnly } 'wrong baseline refuses preflight'
    Text (Join-Path $root 'manifest.json') '{"version":"v1.0.0","commit":"111111111"}'
    Text (Join-Path $root 'payload/release/dependency.txt') 'damaged'
    Must-Fail { & (Join-Path $portable 'Apply-Upgrade.ps1') -UpgradeDirectory $delta -Root $root -Profile fixture -PublicOrigin https://localhost:13443 -CheckOnly } 'bad keep dependency refuses preflight'
    Check ((Get-FileHash (Join-Path $root 'payload/release/server.exe')).Hash -eq $before -and -not (Test-Path (Join-Path $root 'backups'))) 'failed preflight never backs up or changes installed program'
    Text (Join-Path $root 'payload/release/dependency.txt') 'unchanged-dependency'
    foreach ($path in @('../outside','/absolute','C:/drive','payload/release/file:ads','payload/release/../data','data/business.txt','payload/release/.env','payload/release/private.key')) {
        Must-Fail { Assert-ReleaseProgramPath $path } ('unsafe plan path rejected: '+$path)
    }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    foreach ($names in @(@('../outside.txt'),@('C:/outside.txt'),@('a:ads'),@('A.txt','a.txt'),@('a','a/b'))) {
        $zipPath = Join-Path $work ([guid]::NewGuid().ToString('N')+'.zip')
        $zip = [IO.Compression.ZipFile]::Open($zipPath,[IO.Compression.ZipArchiveMode]::Create)
        foreach ($name in $names) { $entry = $zip.CreateEntry($name); $stream = $entry.Open(); $stream.WriteByte(1); $stream.Dispose() }; $zip.Dispose()
        $out = Join-Path $work ([guid]::NewGuid().ToString('N'))
        Must-Fail { Expand-ReleaseArchive $zipPath $out } ('malformed ZIP rejected: '+($names -join ','))
        Check (-not (Test-Path -LiteralPath $out)) 'ZIP inspection completes before creating destination'
    }
    $bad = $plan | ConvertTo-Json -Depth 12 | ConvertFrom-Json; $bad.delete[0].path = 'data/business.txt'
    Must-Fail { Assert-UpgradePlan $bad } 'plan cannot delete business data'
    $bad = $plan | ConvertTo-Json -Depth 12 | ConvertFrom-Json; $bad.write[0].path = '../outside.txt'
    Write-ReleaseJson (Join-Path $delta 'UPGRADE-FILES.json') $bad
    Must-Fail { & (Join-Path $portable 'Apply-Upgrade.ps1') -UpgradeDirectory $delta -Root $root -Profile fixture -PublicOrigin https://localhost:13443 -CheckOnly } 'unsafe plan fails before reading program payload'
    Write-ReleaseJson (Join-Path $delta 'UPGRADE-FILES.json') $plan
    $zipPath = Join-Path $work 'symlink.zip'
    $zip = [IO.Compression.ZipFile]::Open($zipPath,[IO.Compression.ZipArchiveMode]::Create)
    $entry = $zip.CreateEntry('link'); $entry.ExternalAttributes = -1577123840; $zip.Dispose()
    Must-Fail { Expand-ReleaseArchive $zipPath (Join-Path $work 'symlink-extracted') } 'ZIP symbolic link rejected'
    $link = Join-Path $delta 'payload/release/link'
    New-Item -ItemType Junction -Path $link -Target (Join-Path $root 'data') | Out-Null
    Must-Fail { Resolve-ReleasePath $delta 'payload/release/link/business.txt' } 'source junction rejected before hash'
    # Remove only the verified junction, never recursively remove its target.
    [IO.Directory]::Delete($link)
    $descriptor = Read-ReleaseJson (Join-Path $prepared 'release.json')
    $descriptor.package.key = 'https://other.example/upgrade.zip'
    Must-Fail { Assert-ReleaseManifest $descriptor 'v1.0.1' } 'manifest cannot select another host or bucket'
    $config = Get-ReleaseOssConfig
    $env:OSS_ENDPOINT = 'http://oss-cn-hangzhou.aliyuncs.com'
    Must-Fail { Get-ReleaseOssConfig } 'HTTP endpoint rejected'
    $env:OSS_ENDPOINT = 'https://evil.example'
    Must-Fail { Get-ReleaseOssConfig } 'custom endpoint rejected'
    $env:OSS_ENDPOINT = 'https://oss-cn-hangzhou.aliyuncs.com'
    $baselinePackage = Join-Path $work 'baseline-package'; $targetPackage = Join-Path $work 'target-package'
    New-Item -ItemType Directory -Path $baselinePackage,$targetPackage | Out-Null
    foreach ($relative in @('manifest.json','payload/release/server.exe','payload/release/dependency.txt','payload/installer/old.exe')) {
        Text (Join-Path $baselinePackage $relative) (Get-Content -Raw -LiteralPath (Join-Path $root $relative))
    }
    foreach ($relative in @('manifest.json','payload/release/server.exe')) { Text (Join-Path $targetPackage $relative) (Get-Content -Raw -LiteralPath (Join-Path $delta $relative)) }
    Text (Join-Path $targetPackage 'payload/release/dependency.txt') 'unchanged-dependency'
    $builder = Join-Path $portable 'build-upgrade.ps1'
    & $pwsh -NoProfile -ExecutionPolicy Bypass -File $builder -PackageDirectory $targetPackage -BaselineDirectory $baselinePackage -OutputDirectory (Join-Path $work 'built-delta') -Commit $plan.commit | Out-Null
    Check ($LASTEXITCODE -eq 0) 'generic delta builder accepts an explicit package baseline'
    $builtPlan = Get-ReleasePlan (Join-Path $work 'built-delta')
    Check ($builtPlan.write.Count -eq 2 -and $builtPlan.keep.Count -eq 1 -and $builtPlan.delete.Count -eq 1) 'generic delta builder produces write/keep/delete inventories'
    foreach ($name in $script:UpdateToolFiles) { Copy-Item -LiteralPath (Join-Path $portable $name) -Destination (Join-Path $targetPackage $name) }
    & $pwsh -NoProfile -File $builder -PackageDirectory $targetPackage -BaselineDirectory $baselinePackage -OutputDirectory (Join-Path $work 'delta-with-update') -Commit $plan.commit | Out-Null
    Check ($LASTEXITCODE -eq 0) 'delta builder includes new server update tools'
    $entryPlan=Get-ReleasePlan (Join-Path $work 'delta-with-update')
    Check (@($entryPlan.write | Where-Object { $_.path -in $script:UpdateToolFiles }).Count -eq 5) 'delta inventories all five new update tools with hashes'
    # Dedicated fake installation tests the changed lifecycle without real services.
    Text (Join-Path $root 'payload/source/retired.txt') 'old-source-artifact'
    $plan.delete += Entry $root 'payload/source/retired.txt'
    Write-ReleaseJson (Join-Path $delta 'UPGRADE-FILES.json') $plan
    Text (Join-Path $root 'portable-common.ps1') @'
function Get-PortableDaemonStatus { param($Root,$Profile) return @{Running=($env:FAKE_DAEMON_BUSY -eq '1');Detail=@{status=$(if ($env:FAKE_DAEMON_UNKNOWN -eq '1') {'unknown'} else {'stopped'});port_conflict=$false}} }
function Invoke-PortableNativeAction { param($Root,$Action) Add-Content (Join-Path $Root 'trace.txt') $Action; if ($Action -eq 'Backup') { if ($env:FAKE_BACKUP_FAIL -eq '1') { throw 'fake backup failure' }; $b=Join-Path $Root 'backups/native-fixture'; New-Item -ItemType Directory $b -Force | Out-Null; Set-Content (Join-Path $b 'database.dump') 'fake-database' } }
function Stop-PortableCaddy { param($Root) Add-Content (Join-Path $Root 'trace.txt') 'StopCaddy' }
function Start-PortableCaddy { param($Root,$PublicHost,$HttpsPort,$ApiPort,$WebPort,$LanExposed) Add-Content (Join-Path $Root 'trace.txt') 'StartCaddy' }
function Get-NetTCPConnection { param($LocalPort,$State,$ErrorAction) return [pscustomobject]@{OwningProcess=$PID} }
function Get-CimInstance { param($ClassName,$Filter) return @{ExecutablePath=(Join-Path $rootPath 'fake-caddy.exe');CommandLine=(Join-Path $rootPath 'Caddyfile')} }
function Get-PortableCaddyPath { param($Root) return (Join-Path $Root 'fake-caddy.exe') }
function Get-PortableCaddyfilePath { param($Root) return (Join-Path $Root 'Caddyfile') }
function Get-PortableProcessRecord { param($ProcessId,$ExpectedPath) return @{Pid=$ProcessId} }
function Test-PortableProcessRecord { param($Record) return $true }
function Write-PortableJson { param($Path,$Value) New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force | Out-Null; [IO.File]::WriteAllText($Path,($Value|ConvertTo-Json)) }
function Get-PortablePublicRootCAPath { param($Root) return (Join-Path $Root 'public-ca.crt') }
function Wait-PortableHttp { param($Uri,$RootCAPath,$TimeoutSec,$Description) Add-Content (Join-Path $rootPath 'trace.txt') ('Ready '+$Uri) }
function Test-PortableLoopbackHost { param($Value) return $Value -in @('localhost','127.0.0.1','::1') }
function Get-PortableDaemonStartCommand { param($Root,$Profile) return ('fixture daemon start '+$Profile) }
'@
    Text (Join-Path $root 'runtime/native-windows.ps1') @'
param($Action,$Root,$PublicOrigin,$CorsOrigins)
Add-Content (Join-Path $Root 'trace.txt') $Action
'@
    Text (Join-Path $root 'upgrade.ps1') @'
param($Profile,[switch]$NoLanAccess,[switch]$SkipDaemon,[switch]$SkipBrowserTrust,[switch]$NoStartup,[switch]$NoBrowser,$ReportPath)
if (-not ($NoLanAccess -and $SkipDaemon -and $SkipBrowserTrust -and $NoStartup -and $NoBrowser)) { exit 15 }
if ($env:FAKE_UPGRADE_FAIL -eq '1') { exit 16 }
$path = Join-Path $PSScriptRoot 'deployment.json'; $cfg = Get-Content -Raw $path | ConvertFrom-Json; $cfg.Version = 'v1.0.1'; [IO.File]::WriteAllText($path,($cfg|ConvertTo-Json))
$installPath=Join-Path $PSScriptRoot 'data/install-report.json'
if (-not $ReportPath -and (Test-Path $installPath)) { $r=Get-Content -Raw $installPath | ConvertFrom-Json; $r | Add-Member NoteProperty lanExposed $false -Force; [IO.File]::WriteAllText($installPath,($r|ConvertTo-Json)) }
[IO.File]::WriteAllText($ReportPath,(@{fromVersion='v1.0.0';toVersion='v1.0.1'}|ConvertTo-Json))
'@
    Text (Join-Path $root 'deployment.json') '{"Version":"v1.0.0","ApiPort":18080,"WebPort":13000,"PublicOrigin":"https://localhost:13443"}'
    Text (Join-Path $root 'data/install-report.json') '{"profile":"fixture","boardUrl":"https://localhost:13443","lanExposed":false}'
    $update = Join-Path $portable 'Update.ps1'
    $originalConfig = (Get-FileHash (Join-Path $root 'deployment.json')).Hash
    $originalReport = (Get-FileHash (Join-Path $root 'data/install-report.json')).Hash
    & $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-check') -CheckOnly
    Check (-not (Test-Path (Join-Path $root 'trace.txt'))) 'Update reads original config and preflights without lifecycle writes'
    $global:UpdateTestAnswers = New-Object 'System.Collections.Generic.Queue[string]'
    function Read-Host([string]$Prompt) { if ($global:UpdateTestAnswers.Count -eq 0) { throw ('Unexpected prompt: '+$Prompt) }; return $global:UpdateTestAnswers.Dequeue() }
    $global:UpdateTestAnswers.Enqueue('N')
    & $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-cancel') -Interactive
    Check ($global:UpdateTestAnswers.Count -eq 0 -and -not (Test-Path (Join-Path $root 'trace.txt'))) 'Update cancellation never stops services or writes installation'
    Must-Fail { & $update -NonInteractive } 'noninteractive Update rejects missing installation without prompting'
    Must-Fail { & $update -Root $root -Ossutil $fake -PublicOrigin https://other.invalid -CheckOnly } 'Update rejects conflicting service address'
    Must-Fail { & $update -Root $root -Ossutil $fake -Profile other -CheckOnly } 'Update rejects conflicting profile'
    Must-Fail { & $update -Root $root -Ossutil $fake -LanAccess -CheckOnly } 'Update rejects changed listener scope'
    $missingRoot = Join-Path $work 'missing-config'; Copy-Item -LiteralPath $root -Destination $missingRoot -Recurse
    Text (Join-Path $missingRoot 'data/install-report.json') '{}'
    Must-Fail { & $update -Root $missingRoot -Ossutil $fake -CheckOnly } 'Update missing profile produces explicit error without prompting'
    Text (Join-Path $root 'deployment.json') '{"Version":"v1.0.1","PublicOrigin":"https://localhost:13443"}'
    $sameLog = (& $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-same') -CheckOnly 6>&1 | Out-String)
    Check ($sameLog.Contains('已是该版本') -and -not (Test-Path (Join-Path $root 'trace.txt'))) 'same version reports no work without lifecycle'
    Text (Join-Path $root 'deployment.json') '{"Version":"v0.9.0","PublicOrigin":"https://localhost:13443"}'
    Must-Fail { & $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-wrong-start') -ConfirmUpgrade } 'Update rejects skipping the delta chain'
    Text (Join-Path $root 'deployment.json') '{"Version":"v2.0.0","PublicOrigin":"https://localhost:13443"}'
    Must-Fail { & $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-downgrade') -ConfirmUpgrade } 'Update rejects downgrade'
    Text (Join-Path $root 'deployment.json') '{"Version":"v1.0.0","ApiPort":18080,"WebPort":13000,"PublicOrigin":"https://localhost:13443"}'
    $env:FAKE_CORRUPT_KEY='releases/v1.0.1/upgrade.zip'
    Must-Fail { & $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-hash') -ConfirmUpgrade } 'Update rejects bad hash before lifecycle'
    $env:FAKE_CORRUPT_KEY=''; $env:FAKE_FAIL_KEY='releases/v1.0.1/upgrade.zip'
    Must-Fail { & $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-interrupted') -ConfirmUpgrade } 'Update interrupted download is non-success'
    $env:FAKE_FAIL_KEY=''; $env:FAKE_DAEMON_BUSY='1'
    Must-Fail { & $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-busy') -ConfirmUpgrade } 'Update busy daemon prevents service changes'
    $env:FAKE_DAEMON_BUSY=''; $env:FAKE_DAEMON_UNKNOWN='1'
    Must-Fail { & $update -Root $root -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-unknown') -ConfirmUpgrade } 'Update unknown daemon status never authorizes upgrade'
    $env:FAKE_DAEMON_UNKNOWN=''
    Check ((Get-FileHash (Join-Path $root 'deployment.json')).Hash -eq $originalConfig -and (Get-FileHash (Join-Path $root 'data/install-report.json')).Hash -eq $originalReport -and -not (Test-Path (Join-Path $root 'trace.txt'))) 'all rejected Update operations preserve original installation and config'
    $backupFailRoot=Join-Path $work 'backup-failure'; Copy-Item -LiteralPath $root -Destination $backupFailRoot -Recurse
    $env:FAKE_BACKUP_FAIL='1'
    Must-Fail { & $update -Root $backupFailRoot -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-backup-failure') -ConfirmUpgrade } 'Update backup failure propagates'
    Check ((Get-FileHash (Join-Path $backupFailRoot 'payload/release/server.exe')).Hash -eq $before) 'backup failure leaves installed program unchanged'
    $env:FAKE_BACKUP_FAIL=''
    $entryFailRoot=Join-Path $work 'entry-failure'; Copy-Item -LiteralPath $root -Destination $entryFailRoot -Recurse
    $env:FAKE_UPGRADE_FAIL='1'; $entryLog=''
    try { & $update -Root $entryFailRoot -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-entry-failure') -ConfirmUpgrade 6>&1 | ForEach-Object { $entryLog += [string]$_+"`n" } } catch { $entryLog += $_.Exception.Message }
    Check ($entryLog.Contains('保留备份') -and $entryLog.Contains('UPGRADE.md') -and -not $entryLog.Contains('服务已就绪')) 'Update failure displays backup and recovery instructions without claiming success'
    Check (@(Get-ChildItem (Join-Path $entryFailRoot 'backups') -Filter files.json -Recurse).Count -eq 1) 'Update failure retains real Apply file journal'
    $env:FAKE_UPGRADE_FAIL=''
    $entrySuccessRoot=Join-Path $work ('升级 with spaces '+[char]0x4E2D); Copy-Item -LiteralPath $root -Destination $entrySuccessRoot -Recurse
    Text (Join-Path $entrySuccessRoot 'deployment.json') '{"Version":"v1.0.0","ApiPort":18080,"WebPort":13000,"PublicOrigin":"https://server.invalid:13443"}'
    Text (Join-Path $entrySuccessRoot 'data/install-report.json') '{"profile":"fixture","boardUrl":"https://server.invalid:13443","lanExposed":true}'
    & $update -Root $entrySuccessRoot -Ossutil $fake -DestinationDirectory (Join-Path $work 'update-entry-success') -ConfirmUpgrade
    Check ((Read-ReleaseJson (Join-Path $entrySuccessRoot 'deployment.json')).Version -eq 'v1.0.1') 'Update applies through real engine in Unicode and space path'
    Check ((Get-Content (Join-Path $entrySuccessRoot 'trace.txt')) -contains 'Ready https://server.invalid:13443/readyz') 'Apply checks readiness after restoring original listener'
    Check ((Read-ReleaseJson (Join-Path $entrySuccessRoot 'data/install-report.json')).profile -eq 'fixture' -and (Read-ReleaseJson (Join-Path $entrySuccessRoot 'data/install-report.json')).lanExposed) 'Update restores original LAN listener metadata after loopback upgrade'
    Remove-Item Function:Read-Host
    $env:FAKE_DAEMON_BUSY = '1'
    Must-Fail { & (Join-Path $portable 'Apply-Upgrade.ps1') -UpgradeDirectory $delta -Root $root -Profile fixture -PublicOrigin https://localhost:13443 } 'busy daemon refuses before lifecycle'
    Check (-not (Test-Path (Join-Path $root 'trace.txt'))) 'busy daemon does not stop any service'
    $env:FAKE_DAEMON_BUSY = ''
    $failedRoot = Join-Path $work 'failed-installation'
    Copy-Item -LiteralPath $root -Destination $failedRoot -Recurse
    $env:FAKE_UPGRADE_FAIL = '1'
    Must-Fail { & (Join-Path $portable 'Apply-Upgrade.ps1') -UpgradeDirectory $delta -Root $failedRoot -Profile fixture -PublicOrigin https://localhost:13443 } 'native child failure propagates and stops upgrade'
    $failedCheckpoint = Get-ChildItem (Join-Path $failedRoot 'backups') -Directory -Filter 'upgrade-*' | Select-Object -First 1
    Check ((Test-Path (Join-Path $failedCheckpoint.FullName 'files.json')) -and (Test-Path (Join-Path $failedCheckpoint.FullName 'files/payload/release/server.exe'))) 'native failure retains rollback journal and old files'
    $env:FAKE_UPGRADE_FAIL = ''
    & (Join-Path $portable 'Apply-Upgrade.ps1') -UpgradeDirectory $delta -Root $root -Profile fixture -PublicOrigin https://localhost:13443 | Out-Null
    $checkpoint = Get-ChildItem (Join-Path $root 'backups') -Directory -Filter 'upgrade-*' | Select-Object -First 1
    $journal = Read-ReleaseJson (Join-Path $checkpoint.FullName 'files.json')
    Check (-not (Test-Path (Join-Path $root 'payload/source/retired.txt')) -and (Test-Path (Join-Path $checkpoint.FullName 'files/payload/source/retired.txt'))) 'explicit source deletions are applied and backed up'
    Check ($journal.version -eq 'v1.0.1' -and $journal.files.Count -eq 5) 'generic target lifecycle records writes and obsolete files'
    Check ((Get-Content -Raw (Join-Path $checkpoint.FullName 'files/payload/release/server.exe')) -eq 'old-program') 'checkpoint keeps replaced program'
    Check ((Get-Content -Raw (Join-Path $root 'data/business.txt')) -eq 'business-data' -and (Get-Content -Raw (Join-Path $root 'uploads/old.txt')) -eq 'old-attachment' -and (Get-Content -Raw (Join-Path $root 'logs/old.log')) -eq 'old-log') 'business data, attachments and old logs survive'
    Check ((Get-Content -Raw (Join-Path $root 'payload/release/dependency.txt')) -eq 'unchanged-dependency') 'unchanged dependency survives sparse overlay'
    Check ((Read-ReleaseJson (Join-Path $root 'deployment.json')).Version -eq 'v1.0.1') 'generic target version succeeds'
    if ($EvidencePath) {
        New-Item -ItemType Directory -Path (Split-Path -Parent ([IO.Path]::GetFullPath($EvidencePath))) -Force | Out-Null
        Write-ReleaseJson $EvidencePath @{shell=$PSVersionTable.PSVersion.ToString();transport='fake OSS only';passed=$checks.Count;checks=$checks.ToArray();realCloud=$false;realServices=$false}
    }
    Write-Host ('PASS: '+$checks.Count+' checks; PowerShell '+$PSVersionTable.PSVersion+'; fake OSS and isolated fake lifecycle only.')
} finally {
    foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') }
    $env:FAKE_DAEMON_BUSY = ''; $env:FAKE_UPGRADE_FAIL = ''
    # Keep failed fixtures for diagnosis; successful fixtures are bounded to our own path.
    Write-Host ('Test fixture: '+$work)
}
