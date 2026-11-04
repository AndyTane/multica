#Requires -Version 5.1
param([string]$EvidencePath)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../client-settings.ps1')
$testRoot=Join-Path ([IO.Path]::GetTempPath()) ('opencode 中文 test '+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
$checks=New-Object 'System.Collections.Generic.List[string]'
function Check([bool]$Ok,[string]$Label) { if (-not $Ok) { throw $Label }; $checks.Add($Label) }
function Put([string]$Relative,[string]$Content='fixture') {
    $path=Join-Path $testRoot $Relative
    New-Item -ItemType Directory -Path (Split-Path -Parent $path) -Force | Out-Null
    [IO.File]::WriteAllText($path,$Content,(New-Object Text.UTF8Encoding($false)))
    return $path
}
function Reject([scriptblock]$Run,[string]$Text) {
    $caught=''; try { & $Run | Out-Null } catch { $caught=$_.Exception.Message }
    Check ($caught -like ('*'+$Text+'*')) ('readable failure: '+$Text)
}
$saved=@{}; foreach ($name in @('PATH','OPENCODE_CONFIG','OPENCODE_CONFIG_DIR','XDG_CONFIG_HOME','MULTICA_OPENCODE_PATH','KIT_TEST_ROOT','KIT_EXPECT_OPENCODE_PATH')) {$saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
try {
    $env:PATH=''; $env:OPENCODE_CONFIG=''; $env:OPENCODE_CONFIG_DIR=''; $env:XDG_CONFIG_HOME=Join-Path $testRoot 'user config'
    $one=Put 'Tools/First 安装/opencode.exe'
    $two=Put 'Programs/Second 安装/opencode.exe'
    & "$env:WINDIR/Microsoft.NET/Framework64/v4.0.30319/csc.exe" /nologo /target:exe ('/out:'+$one) (Join-Path $PSScriptRoot 'fake-client.cs') | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'native fixture compile failed'}
    Copy-Item -LiteralPath $one -Destination $two -Force
    Check ([Text.Encoding]::ASCII.GetString([IO.File]::ReadAllBytes($one),0,2) -eq 'MZ') 'discovered fixture is a native Windows executable, never a paid runtime'
    $config=Put 'Projects/非默认模型/opencode.jsonc' '{ // comment
        "provider":{}, "mcp":{"fixture":{"command":["fake","https://safe.invalid/a,b"]}},
    }'
    $before=(Get-FileHash -LiteralPath $config).Hash
    $found=@(Find-OpenCodeFiles @((Join-Path $testRoot 'Tools')))
    Check ($found.Count -eq 1 -and $found[0] -eq $one) 'no PATH: native executable found in bounded Tools search with Unicode and spaces'
    $env:PATH=Split-Path -Parent $one
    $found=@(Find-OpenCodeFiles @($env:PATH,(Join-Path $testRoot 'Programs')))
    Check ($found.Count -eq 2 -and $found -contains $one -and $found -contains $two) 'PATH candidate does not hide a second independent installation'
    $script:answers=New-Object 'System.Collections.Generic.Queue[string]'
    foreach ($answer in @('99','not a path','2')) {$script:answers.Enqueue($answer)}
    $picked=Select-OpenCodePath $found '程序' $true '' {param($text) $script:answers.Dequeue()}
    Check ($picked -eq $found[1]) 'invalid input retries and stable number selects complete program path'
    Reject {Select-OpenCodePath $found '程序' $false} '显式传'
    Reject {Select-OpenCodePath $found '程序' $true '' {param($text) 'Q'}} '已取消连接'
    Check (-not (Select-OpenCodePath $found '程序' $true '' {param($text) 'S'})) 'skip permits another runtime'
    $native=Put 'npm/node_modules/opencode-ai/bin/opencode.exe'
    $null=Put 'npm/opencode.cmd'; $null=Put 'npm/opencode.ps1'
    $npm=@(Find-OpenCodeFiles @((Join-Path $testRoot 'npm')))
    Check ($npm.Count -eq 1 -and $npm[0] -eq $native) 'npm cmd/ps1/native paths are one real installation'
    $shim=Put 'scoop/shims/opencode.exe'
    $null=Put 'scoop/shims/opencode.shim' ('path = "'+$one+'"')
    Check ((Get-OpenCodeFile $shim) -eq $one) 'Scoop shim resolves to its native installation'
    $deep=Put 'bounded/a/b/c/d/opencode.exe'
    Check (@(Find-OpenCodeFiles @((Join-Path $testRoot 'bounded')) -Depth 2).Count -eq 0) 'depth is bounded'
    Check (@(Find-OpenCodeFiles @((Join-Path $testRoot 'bounded')) -MaxDirectories 1).Count -eq 0) 'directory count is bounded'
    Check (@(Find-OpenCodeFiles @('\\server.invalid\share')).Count -eq 0) 'network roots are never traversed'
    Assert-OpenCodeConfig $config
    Check ($true) 'JSONC comments/trailing commas/URL and comma inside strings accepted'
    $bad=Put 'bad/opencode.json' '{"secret":"DO_NOT_PRINT", broken'
    Reject {Assert-OpenCodeConfig $bad} '配置无效'
    try {Assert-OpenCodeConfig $bad} catch {Check ($_.Exception.Message -notmatch 'DO_NOT_PRINT') 'invalid config never leaks content'}
    $expected=Join-Path $env:XDG_CONFIG_HOME 'opencode/opencode.json'
    $missingOutput=@(& {try {Select-OpenCodePath @() '配置' $false $expected} catch {Write-Host $_.Exception.Message}} 6>&1) -join "`n"
    Check ($missingOutput.Contains($expected) -and $missingOutput.Contains('未找到 OpenCode 配置')) 'missing config prints expanded current-user destination and retry instructions'
    $kit=Join-Path $testRoot 'kit'; New-Item -ItemType Directory -Path $kit | Out-Null
    Set-ClientOpenCode -Directory $kit -Profile office -CanPrompt $false -OpenCodePath $one -OpenCodeConfig (Split-Path -Parent $config)
    $state=@(Read-OpenCodeSelections $kit)
    Check ($state.Count -eq 1 -and $state[0].configPath -eq $config -and $env:OPENCODE_CONFIG -eq $config -and $env:MULTICA_OPENCODE_PATH -eq $one) 'nondefault JSONC selection exported to actual child environment and persisted'
    $prior=(Get-FileHash (Join-Path $kit 'opencode-selection.json')).Hash
    Reject {Set-ClientOpenCode -Directory $kit -Profile office -CanPrompt $false -OpenCodePath (Join-Path $testRoot 'missing.exe')} '显式传'
    Check ((Get-FileHash (Join-Path $kit 'opencode-selection.json')).Hash -eq $prior) 'unavailable explicit installation never silently switches or changes selection'
    $env:OPENCODE_CONFIG=''; $env:MULTICA_OPENCODE_PATH=''
    Set-ClientOpenCode -Directory $kit -Profile office -CanPrompt $false
    Check ($env:OPENCODE_CONFIG -eq $config -and $env:MULTICA_OPENCODE_PATH -eq $one) 'reconnect reloads selection without PATH'
    Set-ClientOpenCode -Directory $kit -Profile other -CanPrompt $false -SkipOpenCode
    Check (@(Read-OpenCodeSelections $kit).Count -eq 2) 'selections bound to separate profiles'
    Check ((Get-FileHash -LiteralPath $config).Hash -eq $before) 'user config remains byte-for-byte untouched'
    Check (-not ([IO.File]::ReadAllText((Join-Path $kit 'opencode-selection.json')).Contains('provider'))) 'stable data record contains paths only'
    foreach ($name in @('client.ps1','client-settings.ps1')) {Copy-Item (Join-Path $PSScriptRoot ('../'+$name)) $kit}
    & "$env:WINDIR/Microsoft.NET/Framework64/v4.0.30319/csc.exe" /nologo /target:exe ('/out:'+(Join-Path $kit 'u-workers.exe')) (Join-Path $PSScriptRoot 'fake-client.cs') | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'fixture compile failed'}
    [IO.File]::WriteAllText((Join-Path $kit 'connection.json'),'{"profile":"office","serverUrl":"https://server.invalid","rootCa":""}')
    $env:KIT_TEST_ROOT=$kit; $env:KIT_EXPECT_OPENCODE_PATH=$one
    $env:PATH=$saved['PATH']; $env:OPENCODE_CONFIG=''; $env:MULTICA_OPENCODE_PATH=''
    & (Join-Path $kit 'client.ps1') -Action Reconnect -NonInteractive
    $child=Get-Content -LiteralPath (Join-Path $kit 'runtime-env.txt') -Encoding UTF8
    Check ($child[0] -eq $one -and $child[1] -eq $config) 'real launcher child receives selected program and external config'
    Check (-not $env:OPENCODE_CONFIG -and -not $env:MULTICA_OPENCODE_PATH) 'launcher restores caller runtime environment'
    $slot='programs/v0.2.12-aaaaaaaaaaaa-1'; $program=Join-Path $kit $slot
    New-Item -ItemType Directory -Path $program -Force | Out-Null
    foreach ($name in @('client.ps1','client-settings.ps1','u-workers.exe')) {Copy-Item (Join-Path $kit $name) $program}
    [IO.File]::WriteAllText((Join-Path $kit 'current.json'),(@{program=$slot}|ConvertTo-Json))
    [IO.File]::WriteAllText((Join-Path $kit 'state.txt'),'stopped')
    & (Join-Path $kit 'client.ps1') -Action Reconnect -NonInteractive
    $child=Get-Content -LiteralPath (Join-Path $kit 'runtime-env.txt') -Encoding UTF8
    Check ($child[0] -eq $one -and $child[1] -eq $config -and @(Read-OpenCodeSelections $kit).Count -eq 2) 'version-directory delegation retains stable profile selection'
    $null=Put 'retry/opencode.json' '{}'
    $retry=Select-OpenCodePath @() '配置' $true '' {param($text) 'R'} {Find-OpenCodeFiles @((Join-Path $testRoot 'retry')) -Config}
    Check ($retry -eq (Join-Path $testRoot 'retry/opencode.json')) 'retry rechecks files without restarting window'
    $env:PATH=''; $env:OPENCODE_CONFIG=''
    $originalRoots=${function:Get-OpenCodeRoots}; $originalConfigs=${function:Get-OpenCodeConfigRoots}
    try {
        function Get-OpenCodeRoots {param($Directory,$SearchDirectory) @((Join-Path $testRoot 'Tools'))+@($env:PATH -split ';'|Where-Object {$_})+@($SearchDirectory)}
        function Get-OpenCodeConfigRoots {param($Executable,$Directory,$WorkDirectory,$SearchDirectory) @(Split-Path -Parent $config)}
        Set-ClientOpenCode -Directory $kit -Profile auto -CanPrompt $false
        Check ($env:MULTICA_OPENCODE_PATH -eq $one -and $env:OPENCODE_CONFIG -eq $config) 'default launcher auto-discovers program outside PATH and nondefault JSONC'
        $env:PATH=Split-Path -Parent $two
        Reject {Set-ClientOpenCode -Directory $kit -Profile multiple -CanPrompt $false} '显式传'
        $env:OPENCODE_CONFIG=''
        function Get-OpenCodeConfigRoots {param($Executable,$Directory,$WorkDirectory,$SearchDirectory) @((Join-Path $testRoot 'not configured'))}
        $env:PATH=''
        Set-ClientOpenCode -Directory $kit -Profile missing -CanPrompt $false
        Check (@(Read-OpenCodeSelections $kit|Where-Object {$_.profile -eq 'missing'}).Count -eq 0 -and $env:MULTICA_OPENCODE_PATH -like '*opencode-disabled') 'unconfigured OpenCode stays unavailable without persisting an implicit skip'
    } finally {${function:Get-OpenCodeRoots}=$originalRoots;${function:Get-OpenCodeConfigRoots}=$originalConfigs}
    [IO.File]::WriteAllText((Join-Path $kit 'opencode-selection.json'),'broken')
    & (Join-Path $kit 'client.ps1') -Action Status -NonInteractive
    & (Join-Path $kit 'client.ps1') -Action Stop -NonInteractive
    Check ((Get-Content (Join-Path $kit 'state.txt')) -eq 'stopped') 'corrupt selection never blocks stable Status/Stop during recovery'
    $result=@{powershell=$PSVersionTable.PSVersion.ToString();passed=$checks.Count;checks=@($checks);transport='temporary files only; no model CLI or network'}
    if ($EvidencePath) {[IO.File]::WriteAllText($EvidencePath,($result|ConvertTo-Json -Depth 5),(New-Object Text.UTF8Encoding($false)))}
    $result | ConvertTo-Json -Depth 5
} finally {
    foreach ($name in $saved.Keys) {[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')}
    if ([IO.Path]::GetFullPath($testRoot).StartsWith([IO.Path]::GetFullPath([IO.Path]::GetTempPath()),[StringComparison]::OrdinalIgnoreCase)) {Remove-Item -LiteralPath $testRoot -Recurse -Force}
}
