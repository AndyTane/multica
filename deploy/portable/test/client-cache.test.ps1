#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../oss-release-common.ps1')
$work=Join-Path ([IO.Path]::GetTempPath()) ('client-cache-'+[guid]::NewGuid().ToString('N'))
$source=Join-Path $work 'source';$root=Join-Path $work 'install';$input=Join-Path $work 'download'
New-Item -ItemType Directory -Path $source,$root,$input -Force|Out-Null
$version='v0.2.11';$commit=('a'*40)
foreach($name in $script:ClientKitFiles){[IO.File]::WriteAllText((Join-Path $source $name),'fixture')}
[IO.File]::WriteAllText((Join-Path $source 'connection.json'),'{{"profile":"","serverUrl":"","rootCa":""}}'.Replace('{{','{').Replace('}}','}'))
$files=@(Get-ChildItem $source -File|ForEach-Object {@{path=$_.Name;bytes=$_.Length;sha256=(Get-FileHash $_.FullName).Hash.ToLowerInvariant()}})
Write-ReleaseJson (Join-Path $source 'CLIENT-FILES.json') @{schemaVersion=2;version=$version;commit=$commit;platform='windows-amd64';files=$files}
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip=Join-Path $input 'client-kit.zip';[IO.Compression.ZipFile]::CreateFromDirectory($source,$zip)
$obj=@{key=('releases/'+$version+'/client-kit.zip');sha256=(Get-FileHash $zip).Hash.ToLowerInvariant();bytes=(Get-Item $zip).Length}
$dummyHash=('b'*64)
$release=@{schemaVersion=1;to=$version;from='v0.2.9';commit=$commit;baselineCommit='abcdef123';platform='windows-amd64';package=@{key=('releases/'+$version+'/upgrade.zip');sha256=$dummyHash;bytes=1};instructions=@{key=('releases/'+$version+'/UPGRADE.md');sha256=$dummyHash;bytes=1};clientKit=$obj}
Write-ReleaseJson (Join-Path $input 'release.json') $release
Set-StrictMode -Version Latest
$checks=New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok,[string]$Name){if(-not $Ok){throw $Name};$checks.Add($Name)}
function Fail([scriptblock]$Run,[string]$Name){$bad=$false;try{& $Run|Out-Null}catch{$bad=$true};Check $bad $Name}
$cache=Initialize-PortableClientCache $root $input $version $commit
Check (-not (Test-Path (Join-Path $cache 'ready.json'))) 'downloaded cache is inactive before final readiness'
Enable-PortableClientCache $cache $version $commit
$offer=Read-ReleaseJson (Join-Path $cache 'ready.json')
Check ($offer.version -ceq $version -and $offer.commit -ceq $commit -and $offer.sha256 -ceq $obj.sha256) 'activated cache matches installed version source and package hash'
Check ((Get-FileHash (Join-Path $cache 'client-kit.zip')).Hash.ToLowerInvariant() -ceq $obj.sha256) 'cached archive preserves verified bytes'
Fail {Initialize-PortableClientCache $root $input $version ('c'*40)} 'mismatched server commit rejected'
Fail {Enable-PortableClientCache $cache 'v0.2.12' $commit} 'different server version cannot activate cache'
[IO.File]::AppendAllText($zip,'broken')
Fail {Initialize-PortableClientCache $root $input $version $commit} 'corrupt download rejected before cache publication'
$result=@{powershell=$PSVersionTable.PSVersion.ToString();passed=$checks.Count;checks=@($checks)}
if($EvidencePath){Write-ReleaseJson $EvidencePath $result};$result|ConvertTo-Json -Depth 5
Remove-Item -LiteralPath $work -Recurse -Force
