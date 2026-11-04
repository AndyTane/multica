#Requires -Version 5.1
function Assert-ClientPath([string]$Path) {
    $cursor = [IO.Path]::GetFullPath($Path)
    while ($cursor) {
        if (Test-Path -LiteralPath $cursor) {
            if ((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw '路径含目录链接，请使用原目录中的文件。' }
        }
        $parent = Split-Path -Parent $cursor
        if ($cursor -eq $parent) { break }; $cursor = $parent
    }
}
function Read-ClientSettings([string]$Directory) {
    $path = Join-Path $Directory 'connection.json'
    Assert-ClientPath $path
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { return [pscustomobject]@{profile='';serverUrl='';rootCa=''} }
    if ((Get-Item -LiteralPath $path).Length -gt 64KB) { throw '连接设置过大，请使用原客户端的 connection.json。' }
    try { $value = Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop }
    catch { throw '无法读取连接设置，请检查 connection.json 格式，或重新导入旧包。' }
    if ($null -eq $value -or @($value.PSObject.Properties.Name).Count -ne 3) { throw '连接设置仅允许 profile、serverUrl 和 rootCa 三项。' }
    foreach ($name in @('profile','serverUrl','rootCa')) {
        if ($value.PSObject.Properties.Name -cnotcontains $name -or $value.$name -isnot [string]) { throw '连接设置仅允许三项文本：profile、serverUrl 和 rootCa。' }
    }
    return $value
}
function Get-ClientCertificate([string]$Directory,[string]$Reference) {
    if (-not $Reference) { return $null }
    $absolute = [IO.Path]::IsPathRooted($Reference)
    $path = if ($absolute) { [IO.Path]::GetFullPath($Reference) } else { [IO.Path]::GetFullPath((Join-Path $Directory $Reference)) }
    $base = [IO.Path]::GetFullPath($Directory).TrimEnd('\','/')
    if (-not $absolute -and -not $path.StartsWith($base+'\',[StringComparison]::OrdinalIgnoreCase)) { throw '相对证书路径不能离开旧包目录。' }
    Assert-ClientPath $path
    if (-not (Test-Path -LiteralPath $path -PathType Leaf) -or (Get-Item -LiteralPath $path).Length -gt 1MB) { throw '找不到公开根证书，或文件过大。请向管理员索取公开证书。' }
    $bytes = [IO.File]::ReadAllBytes($path)
    $text = [Text.Encoding]::UTF8.GetString($bytes)
    if ($text -match '(?i)PRIVATE\s+KEY') { throw '证书文件含私钥，不能导入。请仅选择公开根证书。' }
    $certificates = @()
    try {
        if ($text -match '-----BEGIN') {
            $pattern = '-----BEGIN CERTIFICATE-----\s*([A-Za-z0-9+/=\s]+?)\s*-----END CERTIFICATE-----'
            if ([regex]::Replace($text,$pattern,'').Trim().Length -ne 0) { throw 'unexpected certificate content' }
            foreach ($match in [regex]::Matches($text,$pattern)) { $certificates += ,([Convert]::FromBase64String($match.Groups[1].Value)) }
        } else { $certificates += ,$bytes }
        if ($certificates.Count -eq 0) { throw 'empty certificate' }
        foreach ($raw in $certificates) {
            if ([Security.Cryptography.X509Certificates.X509Certificate2]::GetCertContentType($raw) -ne [Security.Cryptography.X509Certificates.X509ContentType]::Cert) { throw 'not an X.509 certificate' }
            $cert = New-Object Security.Cryptography.X509Certificates.X509Certificate2 -ArgumentList @(,$raw)
            try {
                if ($cert.HasPrivateKey -or $cert.RawData.Length -ne $raw.Length) { throw 'not a public certificate' }
            } finally { $cert.Dispose() }
        }
    } catch { throw '证书无效，请选择仅含公开 X.509 证书的 CRT、CER 或 PEM 文件。' }
    return [pscustomobject]@{Path=$path;Bytes=$bytes;Relative=(-not $absolute)}
}
function Assert-ClientSettings($Value,[string]$Directory,[switch]$RequireServer,[switch]$SkipCertificate) {
    if ($Value.profile -match '[\\/:\x00-\x1f<>"|?*]' -or $Value.profile -match '^[. ]|[. ]$' -or $Value.profile.Length -gt 128 -or $Value.profile -match '^(?i:con|prn|aux|nul|com[0-9]|lpt[0-9])(?:\.|$)') { throw 'profile 无效，请使用原来的名称；默认配置留空。' }
    if ($RequireServer -and -not $Value.serverUrl) { throw '缺少 HTTPS 服务地址。双击 Connect.cmd 填写，或传 -ServerUrl。' }
    if ($Value.serverUrl) {
        $uri = $null
        if (-not [Uri]::TryCreate($Value.serverUrl,[UriKind]::Absolute,[ref]$uri) -or $uri.Scheme -ne 'https' -or $uri.AbsolutePath -ne '/' -or $uri.UserInfo -or $uri.Query -or $uri.Fragment) { throw '服务地址须为 HTTPS 地址，不含路径、账号或查询参数。' }
    }
    if (-not $SkipCertificate) { return (Get-ClientCertificate $Directory $Value.rootCa) }
}
function Save-ClientSettings($Value,[string]$Directory) {
    $path = Join-Path $Directory 'connection.json'
    Assert-ClientPath $path
    $temp = Join-Path $Directory ('connection-'+[guid]::NewGuid().ToString('N')+'.tmp')
    try {
        [IO.File]::WriteAllText($temp,($Value | ConvertTo-Json -Depth 3),(New-Object Text.UTF8Encoding($false)))
        if (Test-Path -LiteralPath $path) { [IO.File]::Replace($temp,$path,[NullString]::Value) }
        else { [IO.File]::Move($temp,$path) }
    } finally { if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp } }
}
function Import-ClientSettings($Value,$Certificate,[string]$Directory) {
    $created = ''
    try {
        if ($Certificate -and $Certificate.Relative) {
            $sha = [Security.Cryptography.SHA256]::Create()
            try { $hash = [BitConverter]::ToString($sha.ComputeHash($Certificate.Bytes)).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
            $Value.rootCa = 'certificates/public-'+$hash+'.crt'
            $target = Join-Path $Directory $Value.rootCa
            Assert-ClientPath $target
            New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force | Out-Null
            if (Test-Path -LiteralPath $target) {
                if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ne $hash) { throw '新包中的同名证书不一致，请检查后重试。' }
            } else {
                $stream = [IO.File]::Open($target,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write)
                $created = $target
                try { $stream.Write($Certificate.Bytes,0,$Certificate.Bytes.Length) } finally { $stream.Dispose() }
            }
        }
        Save-ClientSettings $Value $Directory
    } catch {
        if ($created -and (Test-Path -LiteralPath $created)) { Remove-Item -LiteralPath $created }
        throw
    }
}

# Discovery reads paths only; it never launches a discovered model runtime.
function Get-OpenCodeNpmProgram([string]$Prefix) {
    $paths=@('node_modules/opencode-ai/bin/opencode.exe')
    foreach ($package in @('opencode-windows-x64','opencode-windows-x64-baseline','opencode-windows-arm64')) {
        $paths+=@(('node_modules/opencode-ai/node_modules/'+$package+'/bin/opencode.exe'),('node_modules/'+$package+'/bin/opencode.exe'))
    }
    foreach ($relative in $paths) {
        $native=Join-Path $Prefix $relative
        if (Test-Path -LiteralPath $native -PathType Leaf) { return [IO.Path]::GetFullPath($native) }
    }
}
function Get-OpenCodeFile([string]$Path,[switch]$Config,[int]$Hop=0) {
    if (-not $Path) { return }
    if ($Hop -gt 4) { return }
    try {
        $full = [IO.Path]::GetFullPath([Environment]::ExpandEnvironmentVariables($Path.Trim().Trim('"')))
        if ($full.StartsWith('\\')) { return }
        if ((New-Object IO.DriveInfo ([IO.Path]::GetPathRoot($full))).DriveType -ne 'Fixed') { return }
        $item = Get-Item -LiteralPath $full -Force -ErrorAction Stop
        if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { return }
        if ($Config) {
            if ($item.Extension -notin @('.json','.jsonc')) { return }
        } elseif ($item.Extension -notin @('.exe','.cmd','.bat','.ps1')) { return }
        # npm's wrappers and native binary are one installation, not two choices.
        if (-not $Config -and $item.Extension -in @('.cmd','.ps1','.bat')) {
            $prefix = $item.DirectoryName
            $native=Get-OpenCodeNpmProgram $prefix
            if ($native) { return $native }
            foreach ($name in @('opencode.exe','opencode.cmd')) {
                $sibling=Join-Path $prefix $name
                if (Test-Path -LiteralPath $sibling -PathType Leaf) { return [IO.Path]::GetFullPath($sibling) }
            }
        }
        if (-not $Config -and $item.Extension -eq '.exe') {
            $shim=[IO.Path]::ChangeExtension($full,'.shim')
            if ((Test-Path -LiteralPath $shim -PathType Leaf) -and (Get-Item -LiteralPath $shim).Length -lt 16KB) {
                $match=[regex]::Match([IO.File]::ReadAllText($shim),'(?m)^path\s*=\s*"([^"]+)"\s*$')
                if ($match.Success -and $match.Groups[1].Value -ne $full) { return Get-OpenCodeFile $match.Groups[1].Value -Hop ($Hop+1) }
            }
        }
        return $item.FullName
    } catch { return }
}
function Find-OpenCodeFiles {
    param([string[]]$Roots,[switch]$Config,[int]$Depth=3,[int]$MaxDirectories=600,[int]$Seconds=5)
    $found = @{}; $seen = @{}
    $queue = New-Object 'System.Collections.Generic.Queue[object]'
    $pathDirectories=@($env:PATH -split ';' | Where-Object { $_ })
    foreach ($root in $Roots) { if ($root) { $startDepth=if (-not $Config -and $root -in $pathDirectories) {$Depth} else {0}; $queue.Enqueue(@($root,$startDepth)) } }
    $clock = [Diagnostics.Stopwatch]::StartNew(); $visited=0
    while ($queue.Count -and $visited -lt $MaxDirectories -and $clock.Elapsed.TotalSeconds -lt $Seconds) {
        $entry=$queue.Dequeue()
        try {
            $path=[IO.Path]::GetFullPath([string]$entry[0])
            if ($path.StartsWith('\\') -or $seen.ContainsKey($path)) { continue }; $seen[$path]=$true
            if ((New-Object IO.DriveInfo ([IO.Path]::GetPathRoot($path))).DriveType -ne 'Fixed') { continue }
            $userRoot=[Environment]::GetFolderPath('UserProfile').TrimEnd('\')
            $usersBase=Split-Path -Parent $userRoot
            if ($path.StartsWith($usersBase+'\',[StringComparison]::OrdinalIgnoreCase) -and -not ($path -eq $userRoot -or $path.StartsWith($userRoot+'\',[StringComparison]::OrdinalIgnoreCase))) { continue }
            $dir=Get-Item -LiteralPath $path -Force -ErrorAction Stop
            if (-not $dir.PSIsContainer) {
                $file=Get-OpenCodeFile $path -Config:$Config
                if ($file) { $found[$file]=$true }; continue
            }
            if ($dir.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue }; $visited++
            $names=if ($Config) { @('opencode.json','opencode.jsonc') } else { @('opencode.exe','opencode.cmd','opencode.ps1','opencode.bat') }
            foreach ($name in $names) {
                $file=Get-OpenCodeFile (Join-Path $path $name) -Config:$Config
                if ($file) { $found[$file]=$true }
            }
            # Resolve npm layouts directly without traversing all node_modules.
            if (-not $Config) {
                $native=Get-OpenCodeNpmProgram $path
                if ($native) { $found[$native]=$true }
            }
            if ([int]$entry[1] -ge $Depth) { continue }
            foreach ($child in Get-ChildItem -LiteralPath $path -Directory -Force -ErrorAction SilentlyContinue | Select-Object -First ([Math]::Max(1,$MaxDirectories-$queue.Count)) | Sort-Object Name) {
                if ($child.Name -in @('node_modules','.git','.managed','programs','backups','data','uploads','logs') -or ($child.Attributes -band [IO.FileAttributes]::ReparsePoint)) { continue }
                if ($queue.Count -ge $MaxDirectories) { break }
                $queue.Enqueue(@($child.FullName,([int]$entry[1]+1)))
            }
        } catch { continue }
    }
    if ($queue.Count) { Write-Host '已达到搜索边界；未列出的安装可直接输入完整路径。' }
    return @($found.Keys | Sort-Object)
}
function Get-OpenCodeRoots([string]$Directory,[string[]]$SearchDirectory) {
    $userRoot=[Environment]::GetFolderPath('UserProfile')
    $roots=@($Directory,(Join-Path $userRoot '.opencode/bin'),(Join-Path $userRoot '.local/bin'),(Join-Path $userRoot 'scoop/shims'),(Join-Path $userRoot 'scoop/apps/opencode'),(Join-Path $userRoot '.bun/bin'))
    foreach ($base in @($env:APPDATA,$env:LOCALAPPDATA,$env:ProgramFiles,${env:ProgramFiles(x86)})) {
        if ($base) { foreach ($name in @('npm','OpenCode','Programs/OpenCode','opencode')) { $roots+=Join-Path $base $name } }
    }
    foreach ($drive in [IO.DriveInfo]::GetDrives()) {
        if ($drive.DriveType -eq 'Fixed' -and $drive.IsReady) {
            foreach ($name in @('Tools','Programs','UWorkers/Portable/payload/runtime/opencode','UWorkers/local/native/runtime/opencode')) { $roots+=Join-Path $drive.RootDirectory.FullName $name }
        }
    }
    # PATH is checked alongside known directories, even when it has a candidate.
    $roots+=@($env:PATH -split ';' | Where-Object { $_ -and [IO.Path]::IsPathRooted($_) })
    return @($roots+$SearchDirectory | Where-Object { $_ } | Select-Object -Unique)
}
function Get-OpenCodeConfigRoots([string]$Executable,[string]$Directory,[string]$WorkDirectory,[string[]]$SearchDirectory) {
    $userRoot=[Environment]::GetFolderPath('UserProfile')
    $configHome=if ($env:XDG_CONFIG_HOME) { $env:XDG_CONFIG_HOME } else { Join-Path $userRoot '.config' }
    $roots=@((Join-Path $configHome 'opencode'),$env:OPENCODE_CONFIG_DIR,$Directory)
    $program=Split-Path -Parent $Executable
    $roots+=@($program,(Join-Path $program 'config'),(Join-Path (Split-Path -Parent $program) 'config'))
    foreach ($name in @('Documents','Desktop','Downloads')) { $roots+=Join-Path $userRoot $name }
    $cursor=if ($WorkDirectory) { $WorkDirectory } else { (Get-Location).Path }
    for ($level=0;$cursor -and $level -lt 12;$level++) {
        foreach ($name in @('opencode.json','opencode.jsonc','.opencode/opencode.json','.opencode/opencode.jsonc')) { $roots+=Join-Path $cursor $name }
        if (Test-Path -LiteralPath (Join-Path $cursor '.git')) { break }
        $parent=Split-Path -Parent $cursor; if ($parent -eq $cursor) { break }; $cursor=$parent
    }
    return @($roots+$SearchDirectory | Where-Object { $_ } | Select-Object -Unique)
}
function Assert-OpenCodeConfig([string]$Path) {
    try {
        if ((Get-Item -LiteralPath $Path -ErrorAction Stop).Length -gt 4MB) { throw 'size' }
        $raw=Get-Content -LiteralPath $Path -Raw -Encoding UTF8 -ErrorAction Stop
        # Preserve strings while removing comments and trailing commas.
        $tokens='"(?:\\.|[^"\\])*"|//[^\r\n]*|/\*[\s\S]*?\*/'
        $json=[regex]::Replace($raw,$tokens,{param($m) if ($m.Value.StartsWith('"')) {$m.Value} else {' '}})
        $json=[regex]::Replace($json,'"(?:\\.|[^"\\])*"|,\s*(?=[}\]])',{param($m) if ($m.Value.StartsWith('"')) {$m.Value} else {''}})
        $value=ConvertFrom-Json -InputObject $json -ErrorAction Stop
        if ($value -isnot [pscustomobject]) { throw 'object' }
    } catch { throw ('配置无效：'+$Path+'。请修正 JSON/JSONC 格式后重试；配置内容未输出。') }
}
function Select-OpenCodePath {
    param([string[]]$Candidates,[string]$Kind,[bool]$CanPrompt,[string]$Expected,[scriptblock]$Prompt={param($text) Read-Host $text},[scriptblock]$Retry,[hashtable]$Sources=@{})
    $items=@($Candidates | Where-Object { $_ })
    while ($true) {
        if ($items.Count -eq 1) {
            $path=$items[0]
            if ($Kind -eq '配置') {
                try { Assert-OpenCodeConfig $path } catch {
                    Write-Host $_.Exception.Message
                    if (-not $CanPrompt) { throw }; $items=@(); continue
                }
            }
            Write-Host ('OpenCode '+$Kind+'：'+$path); return $path
        }
        if ($items.Count -gt 1) {
            Write-Host ('发现多个 OpenCode '+$Kind+'，请选择完整路径：')
            for ($i=0;$i -lt $items.Count;$i++) {
                $source=if ($Sources.ContainsKey($items[$i])) {' ['+$Sources[$items[$i]]+']'} else {''}
                Write-Host ('  '+($i+1)+'. '+$items[$i]+$source)
            }
        } else {
            Write-Host ('未找到 OpenCode '+$Kind+'。')
            if ($Expected) { Write-Host ('请在此路径放置配置后重试：'+$Expected) }
        }
        if (-not $CanPrompt) { throw ('请用 Connect.cmd 选择，或显式传 -OpenCodePath / -OpenCodeConfig；只用其他运行环境可传 -SkipOpenCode。') }
        $answer=[string](& $Prompt '输入编号或完整文件/目录路径；R 重新检查，S 跳过 OpenCode，Q 取消连接')
        $answer=$answer.Trim().Trim('"')
        if ($answer -eq 'Q' -or -not $answer) { throw '已取消连接，原选择保留。' }
        if ($answer -eq 'S') { return '' }
        if ($answer -eq 'R') {
            if ($Retry) { $items=@(& $Retry | Where-Object { $_ }) }
            elseif ($Expected -and (Test-Path -LiteralPath $Expected -PathType Leaf)) { $items=@($Expected) }
            continue
        }
        $number=0
        if ([int]::TryParse($answer,[ref]$number) -and $number -ge 1 -and $number -le $items.Count) { $items=@($items[$number-1]); continue }
        $file=Get-OpenCodeFile $answer -Config:($Kind -eq '配置')
        if ($file) { $items=@($file); continue }
        if (Test-Path -LiteralPath $answer -PathType Container) {
            $items=@(Find-OpenCodeFiles @($answer) -Config:($Kind -eq '配置')); continue
        }
        Write-Host '选择无效，请输入列表编号或现有文件的完整路径。'
    }
}
function Read-OpenCodeSelections([string]$Directory) {
    $path=Join-Path $Directory 'opencode-selection.json'
    Assert-ClientPath $path
    if (-not (Test-Path -LiteralPath $path)) { return @() }
    try {
        if ((Get-Item -LiteralPath $path).Length -gt 64KB) { throw 'size' }
        $state=Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop
        if ($state.schemaVersion -ne 1) { throw 'schema' }
        $profiles=@{}
        foreach ($selection in @($state.selections)) {
            if ($null -eq $selection -or $selection.profile -isnot [string] -or $selection.disabled -isnot [bool] -or $profiles.ContainsKey($selection.profile)) { throw 'selection' }
            if (-not $selection.disabled -and ($selection.executable -isnot [string] -or $selection.configPath -isnot [string])) { throw 'paths' }
            $profiles[$selection.profile]=$true
        }
        return @($state.selections)
    } catch { throw '无法读取 opencode-selection.json；请保留原文件并检查格式后重试。' }
}
function Set-ClientOpenCode {
    param([string]$Directory,[string]$Profile,[bool]$CanPrompt,[string]$OpenCodePath,[string]$OpenCodeConfig,[string[]]$SearchDirectory,[string]$WorkDirectory,[switch]$SkipOpenCode)
    $prior=@(Read-OpenCodeSelections $Directory)
    $old=@($prior | Where-Object { $_.profile -ceq $Profile }) | Select-Object -First 1
    $selected=[ordered]@{profile=$Profile;disabled=[bool]$SkipOpenCode;executable='';configPath='';configDir='';configHome=''}
    $implicitSkip=$false
    if ($old -and -not $SkipOpenCode -and -not $OpenCodePath -and -not $OpenCodeConfig) { $selected.disabled=[bool]$old.disabled }
    if (-not $selected.disabled) {
        Write-Host '正在查找 OpenCode（仅搜索常用目录及指定位置，最多 600 个目录 / 每轮 5 秒）...'
        $explicit=if ($OpenCodePath) {$OpenCodePath} elseif ($old -and $old.executable) {$old.executable} else {''}
        $candidates=if ($explicit) { @(Find-OpenCodeFiles @($explicit)) } else { @(Find-OpenCodeFiles (Get-OpenCodeRoots $Directory $SearchDirectory)) }
        if ($explicit -and $candidates.Count -eq 0) { Write-Host ('所选程序不可用：'+$explicit+'；不会自动切换到其他安装。') }
        if (-not $CanPrompt -and -not $explicit -and $candidates.Count -eq 0) {
            Write-Host '未找到 OpenCode 程序；其他运行环境继续连接。安装后可用 Connect.cmd 重新选择。'; $selected.disabled=$true; $implicitSkip=$true
        } else {
            $selected.executable=Select-OpenCodePath $candidates '程序' $CanPrompt -Retry { if ($explicit) { Find-OpenCodeFiles @($explicit) } else { Find-OpenCodeFiles (Get-OpenCodeRoots $Directory $SearchDirectory) } }
            if (-not $selected.executable) { $selected.disabled=$true }
        }
        if (-not $selected.disabled) {
            $configHome=if ($env:XDG_CONFIG_HOME) {$env:XDG_CONFIG_HOME} else { Join-Path ([Environment]::GetFolderPath('UserProfile')) '.config' }
            $expected=if ($env:OPENCODE_CONFIG_DIR) {[IO.Path]::GetFullPath((Join-Path $env:OPENCODE_CONFIG_DIR 'opencode.json'))} else {[IO.Path]::GetFullPath((Join-Path $configHome 'opencode/opencode.json'))}
            $explicitConfig=if ($OpenCodeConfig) {$OpenCodeConfig} elseif ($env:OPENCODE_CONFIG) {$env:OPENCODE_CONFIG} elseif ($old -and $old.configPath) {$old.configPath} else {''}
            $configs=if ($explicitConfig) { @(Find-OpenCodeFiles @($explicitConfig) -Config) } elseif ($env:OPENCODE_CONFIG_DIR) { @(Find-OpenCodeFiles @($env:OPENCODE_CONFIG_DIR) -Config -Depth 0) } else { @(Find-OpenCodeFiles (Get-OpenCodeConfigRoots $selected.executable $Directory $WorkDirectory $SearchDirectory) -Config -Depth 2) }
            if ($explicitConfig -and $configs.Count -eq 0) { Write-Host ('所选配置不可用：'+$explicitConfig+'；不会自动改用其他文件。') }
            $sources=@{}
            foreach ($file in $configs) {
                $sources[$file]='搜索目录'
                if ($file.StartsWith($Directory,[StringComparison]::OrdinalIgnoreCase)) {$sources[$file]='Kit 目录'}
                if ($file.StartsWith((Split-Path -Parent $selected.executable),[StringComparison]::OrdinalIgnoreCase)) {$sources[$file]='所选程序目录'}
                if ($file.StartsWith($configHome,[StringComparison]::OrdinalIgnoreCase)) {$sources[$file]='用户配置目录'}
                if ($env:OPENCODE_CONFIG_DIR -and $file.StartsWith($env:OPENCODE_CONFIG_DIR,[StringComparison]::OrdinalIgnoreCase)) {$sources[$file]='OPENCODE_CONFIG_DIR'}
                if ($explicitConfig) {$sources[$file]='显式或已保存路径'}
            }
            if (-not $CanPrompt -and -not $explicitConfig -and -not $env:OPENCODE_CONFIG_DIR -and -not $OpenCodePath -and -not $old -and $configs.Count -eq 0) {
                Write-Host ('未找到 OpenCode 配置。请在此路径放置配置后重试：'+$expected)
                $selected.configPath=''
                $implicitSkip=$true
            } else { $selected.configPath=Select-OpenCodePath $configs '配置' $CanPrompt $expected -Sources $sources -Retry { if ($explicitConfig) {Find-OpenCodeFiles @($explicitConfig) -Config} elseif ($env:OPENCODE_CONFIG_DIR) {Find-OpenCodeFiles @($env:OPENCODE_CONFIG_DIR) -Config -Depth 0} else {Find-OpenCodeFiles (Get-OpenCodeConfigRoots $selected.executable $Directory $WorkDirectory $SearchDirectory) -Config -Depth 2} } }
            if (-not $selected.configPath) { $selected.disabled=$true }
            $selected.configDir=if ($env:OPENCODE_CONFIG_DIR) {[IO.Path]::GetFullPath($env:OPENCODE_CONFIG_DIR)} elseif ($old) {[string]$old.configDir} else {''}
            $selected.configHome=if ($env:XDG_CONFIG_HOME) {[IO.Path]::GetFullPath($env:XDG_CONFIG_HOME)} elseif ($old) {[string]$old.configHome} else {''}
        }
    }
    if ($selected.disabled) {
        $selected.executable=''; $selected.configPath=''
        Write-Host 'OpenCode 未启用；其他运行环境继续连接。重新选择请用 Connect.cmd -OpenCodePath <完整路径>。'
        $env:MULTICA_OPENCODE_PATH=Join-Path $Directory '.managed/opencode-disabled'
    } else {
        $env:MULTICA_OPENCODE_PATH=$selected.executable; $env:OPENCODE_CONFIG=$selected.configPath
        if ($selected.configDir) {$env:OPENCODE_CONFIG_DIR=$selected.configDir}; if ($selected.configHome) {$env:XDG_CONFIG_HOME=$selected.configHome}
    }
    if ($implicitSkip) { return }
    $state=@{schemaVersion=1;selections=@($prior | Where-Object { $_.profile -cne $Profile })+@([pscustomobject]$selected)}
    $path=Join-Path $Directory 'opencode-selection.json'; Assert-ClientPath $path
    $temp=Join-Path $Directory ('opencode-'+[guid]::NewGuid().ToString('N')+'.tmp')
    try {
        [IO.File]::WriteAllText($temp,($state|ConvertTo-Json -Depth 5),(New-Object Text.UTF8Encoding($false)))
        if (Test-Path -LiteralPath $path) {[IO.File]::Replace($temp,$path,[NullString]::Value)} else {[IO.File]::Move($temp,$path)}
    } finally { if (Test-Path -LiteralPath $temp) {Remove-Item -LiteralPath $temp} }
}
function Show-ClientOpenCode([string]$Directory,[string]$Profile) {
    try {
        $selection=@(Read-OpenCodeSelections $Directory | Where-Object { $_.profile -ceq $Profile }) | Select-Object -First 1
        if ($selection -and -not $selection.disabled) {
            Write-Host ('OpenCode 程序：'+$selection.executable)
            Write-Host ('OpenCode 配置：'+$selection.configPath)
        }
    } catch { Write-Host 'OpenCode 选择记录无法读取；Status/Stop 仍可使用。重新选择前请检查 opencode-selection.json。' }
}
