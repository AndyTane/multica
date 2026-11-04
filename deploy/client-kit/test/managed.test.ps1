#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference='Stop'
$template=Split-Path -Parent $PSScriptRoot
$work=Join-Path ([IO.Path]::GetTempPath()) ('managed kit-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work|Out-Null
foreach($name in @('client.ps1','client-settings.ps1')){Copy-Item (Join-Path $template $name) $work}
& "$env:WINDIR/Microsoft.NET/Framework64/v4.0.30319/csc.exe" /nologo /target:exe ('/out:'+(Join-Path $work 'u-workers.exe')) (Join-Path $PSScriptRoot 'fake-client.cs')|Out-Null
if($LASTEXITCODE-ne 0){throw 'Fixture compile failed'}
[IO.File]::WriteAllText((Join-Path $work 'connection.json'),'{"profile":"office","serverUrl":"https://server.invalid","rootCa":""}')
[IO.File]::WriteAllText((Join-Path $work 'state.txt'),'running')
$checks=New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok,[string]$Name){if(-not $Ok){throw $Name};$checks.Add($Name)}
$names=@('KIT_TEST_ROOT','KIT_MANAGED_ID');$saved=@{};foreach($n in $names){$saved[$n]=[Environment]::GetEnvironmentVariable($n,'Process')}
try{
 $env:KIT_TEST_ROOT=$work;$env:KIT_MANAGED_ID=''
 $client=Join-Path $work 'client.ps1'
 & $client -Action PauseUpdate
 $id=Get-Content (Join-Path $work 'kit-install.json') -Raw|ConvertFrom-Json
 Check ($id.paused -and $id.profile-eq 'office') 'pause persists per-installation identity and profile'
 & $client -Action ResumeUpdate
 $again=Get-Content (Join-Path $work 'kit-install.json') -Raw|ConvertFrom-Json
 Check (-not $again.paused -and $again.id-eq $id.id) 'resume preserves installation ID'
 $slot='programs/v0.2.16-aaaaaaaaaaaa-1';$program=Join-Path $work $slot;New-Item -ItemType Directory -Path $program -Force|Out-Null
 foreach($n in @('client.ps1','client-settings.ps1')){Copy-Item (Join-Path $template $n) $program}
 [IO.File]::WriteAllText((Join-Path $program 'u-workers.exe'),'invalid candidate')
 [IO.File]::WriteAllText((Join-Path $program 'CLIENT-FILES.json'),'{"version":"v0.2.16"}')
 [IO.File]::WriteAllText((Join-Path $work 'current.json'),(@{program=$slot}|ConvertTo-Json))
 New-Item -ItemType Directory -Path (Join-Path $work '.managed')|Out-Null
 $sha=('a'*64)
 [IO.File]::WriteAllText((Join-Path $work '.managed/update.json'),(@{phase='verifying';reason='pending';offer=@{sha256=$sha}}|ConvertTo-Json -Depth 4))
 $env:KIT_MANAGED_ID=$id.id
 & $client -Action Status
 Check ($true) 'status uses stable CLI while candidate exe is invalid'
 & $client -Action Stop
 $stop=Get-Content (Join-Path $work '.managed/stop.json') -Raw|ConvertFrom-Json
 Check ($stop.sha256-eq $sha) 'explicit stop records intent before stopping candidate'
 $env:KIT_MANAGED_ID=[guid]::NewGuid().ToString();$failed=$false
 try{& $client -Action Status|Out-Null}catch{$failed=$true}
 Check $failed 'same-profile daemon belonging to another kit is rejected'
 [IO.File]::WriteAllText((Join-Path $work 'current.json'),'{"program":"../escape"}')
 $failed=$false;try{& $client -Action Status|Out-Null}catch{$failed=$true}
 Check $failed 'launcher rejects program-pointer traversal'
 $result=@{powershell=$PSVersionTable.PSVersion.ToString();passed=$checks.Count;checks=@($checks);transport='fake lifecycle CLI; no helpers/services/network'}
 if($EvidencePath){[IO.File]::WriteAllText($EvidencePath,($result|ConvertTo-Json -Depth 6),(New-Object Text.UTF8Encoding($false)))}
 $result|ConvertTo-Json -Depth 6
}finally{
 foreach($n in $names){[Environment]::SetEnvironmentVariable($n,$saved[$n],'Process')}
 if([IO.Path]::GetFullPath($work).StartsWith([IO.Path]::GetFullPath([IO.Path]::GetTempPath()),[StringComparison]::OrdinalIgnoreCase)){Remove-Item -LiteralPath $work -Recurse -Force}
}
