#Requires -Version 5.1
# Shared release contracts. This module never reads deployment secrets or .env.
$script:ReleaseMaxObjectBytes = 5GB
$script:ReleaseCheckedDirectories = @{}
$script:ReleaseProgramFiles = @('manifest.json','README.txt','Setup.cmd','Start.cmd','Stop.cmd','Status.cmd','Upgrade.cmd','Uninstall.cmd','Trust-Browser.cmd','Find-OldDeployment.cmd','Build-Installer.cmd','install.ps1','start.ps1','stop.ps1','status.ps1','upgrade.ps1','uninstall.ps1','trust-browser.ps1','portable-common.ps1','find-old-deployment.ps1','build-installer.ps1',([string][char]0x64CD+[char]0x4F5C+[char]0x5355+'-'+[char]0x53F0+[char]0x5F0F+[char]0x673A+'.md'))

function Assert-ReleaseVersion([string]$Version) {
    if ($Version -cnotmatch '^v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)*)?$') { throw 'Invalid release version; expected v1.2.3.' }
}
function Assert-ReleaseCommit([string]$Commit) {
    if ($Commit -cnotmatch '^[0-9a-f]{9,40}$') { throw 'Invalid release commit.' }
}
function Assert-ReleaseHash([string]$Hash) {
    if ($Hash -notmatch '^[0-9a-f]{64}$') { throw 'Invalid SHA-256 in release metadata.' }
}
function Assert-ReleaseRelativePath([string]$Relative) {
    if (-not $Relative -or $Relative.Length -gt 1024 -or $Relative -match '[\\:\x00-\x1f<>"|?*]' -or $Relative.StartsWith('/') -or $Relative.EndsWith('/')) { throw 'Unsafe release relative path.' }
    foreach ($part in $Relative.Split('/')) {
        if (-not $part -or $part -eq '.' -or $part -eq '..' -or $part -match '[. ]$' -or $part -match '^(?i:con|prn|aux|nul|com[0-9]|lpt[0-9])(?:\.|$)') { throw 'Unsafe release path component.' }
    }
}
function Assert-ReleaseProgramPath([string]$Relative) {
    Assert-ReleaseRelativePath $Relative
    if ($Relative -match '(?i)(^|/)(\.env[^/]*|deployment\.json|credentials[^/]*|id_rsa[^/]*|id_ed25519[^/]*)(/|$)' -or $Relative -match '(?i)\.(pem|key|pfx|p12)$') { throw 'Sensitive file is outside the release contract.' }
    if ($script:ReleaseProgramFiles -contains $Relative -or $Relative -eq 'runtime/native-windows.ps1' -or $Relative -match '^(?i:payload/(release|bin|runtime|installer|scripts|source)/)') { return }
    throw 'Path is outside the managed program file set.'
}
function Resolve-ReleasePath([string]$Parent, [string]$Relative) {
    Assert-ReleaseRelativePath $Relative
    $base = [IO.Path]::GetFullPath($Parent).TrimEnd('\','/')
    $path = [IO.Path]::GetFullPath((Join-Path $base $Relative))
    if (-not $path.StartsWith($base+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Release path escaped its root.' }
    $cursor = $path
    while ($cursor) {
        if ($cursor -ne $path -and $script:ReleaseCheckedDirectories.ContainsKey($cursor)) { break }
        if (Test-Path -LiteralPath $cursor) {
            $item = Get-Item -LiteralPath $cursor -Force
            if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Release path contains a junction or link.' }
            if ($cursor -ne $path -and $item.PSIsContainer) { $script:ReleaseCheckedDirectories[$cursor] = $true }
        }
        $next = Split-Path -Parent $cursor
        if ($next -eq $cursor) { break }; $cursor = $next
    }
    return $path
}
function Assert-ReleaseSeparateDirectory([string]$Directory, [string]$Root) {
    $a = [IO.Path]::GetFullPath($Directory).TrimEnd('\','/')
    $b = [IO.Path]::GetFullPath($Root).TrimEnd('\','/')
    if ($a -eq $b -or $a.StartsWith($b+'\',[StringComparison]::OrdinalIgnoreCase) -or $b.StartsWith($a+'\',[StringComparison]::OrdinalIgnoreCase)) { throw 'Use separate, non-nested source, output and installation directories.' }
}
function Read-ReleaseJson([string]$Path) {
    if ((Get-Item -LiteralPath $Path).Length -gt 8MB) { throw 'Release metadata exceeds the size limit.' }
    try { return (Get-Content -Raw -LiteralPath $Path -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop) } catch { throw 'Invalid release JSON.' }
}
function Write-ReleaseJson([string]$Path, $Value) {
    [IO.File]::WriteAllText($Path,($Value | ConvertTo-Json -Depth 12),(New-Object Text.UTF8Encoding($false)))
}
function Assert-ReleaseFile([string]$Path, [string]$Hash, [long]$Bytes = -1) {
    Assert-ReleaseHash $Hash
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw 'Release file is missing.' }
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Release file is a link.' }
    if ($Bytes -ge 0 -and $item.Length -ne $Bytes) { throw 'Release file size mismatch.' }
    if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -ne $Hash) { throw 'Release SHA-256 mismatch.' }
}
function Assert-UpgradePlan($Plan) {
    Assert-ReleaseVersion ([string]$Plan.from); Assert-ReleaseVersion ([string]$Plan.to)
    Assert-ReleaseCommit ([string]$Plan.commit); Assert-ReleaseCommit ([string]$Plan.baselineCommit)
    if ($Plan.from -eq $Plan.to -or @($Plan.write).Count -eq 0 -or $null -eq $Plan.keep -or $null -eq $Plan.delete) { throw 'Incomplete upgrade plan.' }
    $seen = @{}
    foreach ($kind in @('write','keep','delete')) {
        foreach ($entry in @($Plan.$kind)) {
            $path = [string]$entry.path
            Assert-ReleaseProgramPath $path; Assert-ReleaseHash ([string]$entry.sha256)
            if ($seen.ContainsKey($path)) { throw 'Duplicate or conflicting upgrade plan path.' }; $seen[$path] = $true
            if ($kind -ne 'delete' -and ($null -eq $entry.bytes -or $entry.bytes -is [string] -or [long]$entry.bytes -lt 0)) { throw 'Invalid upgrade file size.' }
            if ($kind -eq 'write') {
                if ($entry.action -notin @('add','replace')) { throw 'Invalid upgrade write action.' }
                if ($entry.old_sha256) { Assert-ReleaseHash ([string]$entry.old_sha256) }
                if ($entry.action -eq 'replace' -and -not $entry.old_sha256) { throw 'Replacement is missing its baseline hash.' }
                if ($path -match '^payload/runtime/(node\.exe|pgsql/)') { throw 'Node/PostgreSQL replacement requires a separate runtime upgrade.' }
            }
            if ($kind -eq 'delete' -and $path -notmatch '^payload/(release|installer|source)/') { throw 'Deletion is outside immutable generated payload.' }
        }
    }
    if (-not (@($Plan.write)+@($Plan.keep) | Where-Object { $_.path -eq 'manifest.json' })) { throw 'Upgrade plan is missing the target manifest.' }
}
function Get-ReleasePlan([string]$Directory) {
    $plan = Read-ReleaseJson (Resolve-ReleasePath $Directory 'UPGRADE-FILES.json')
    Assert-UpgradePlan $plan
    return $plan
}
function Get-ReleaseFiles([string]$Directory) {
    # Walk without following reparse points; never inspect extra installation data.
    $stack = New-Object 'System.Collections.Generic.Stack[string]'; $stack.Push($Directory)
    while ($stack.Count) {
        foreach ($item in Get-ChildItem -LiteralPath $stack.Pop() -Force) {
            if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Package contains a junction or symbolic link.' }
            if ($item.PSIsContainer) { $stack.Push($item.FullName) } else { $item }
        }
    }
}
function Assert-ReleaseDirectory($Directory, $Plan) {
    foreach ($entry in $Plan.write) { Assert-ReleaseFile (Resolve-ReleasePath $Directory $entry.path) $entry.sha256 ([long]$entry.bytes) }
    $manifest = Read-ReleaseJson (Resolve-ReleasePath $Directory 'manifest.json')
    if ($manifest.version -ne $Plan.to -or -not ([string]$Plan.commit).StartsWith([string]$manifest.commit) -or ([string]$manifest.commit).Length -lt 9) { throw 'Target manifest and upgrade plan disagree.' }
}
function Get-ReleaseOssConfig {
    foreach ($name in @('OSS_ACCESS_KEY_ID','OSS_ACCESS_KEY_SECRET','OSS_REGION','OSS_ENDPOINT','UWORKERS_OSS_BUCKET')) {
        if (-not [Environment]::GetEnvironmentVariable($name,'Process')) { throw ('Missing environment variable: '+$name) }
    }
    $region = $env:OSS_REGION; $bucket = $env:UWORKERS_OSS_BUCKET
    if ($region -cnotmatch '^cn-[a-z0-9-]+$' -or $bucket -cnotmatch '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$') { throw 'Invalid OSS region or bucket.' }
    if ($env:OSS_ENDPOINT.TrimEnd('/') -cne ('https://oss-'+$region+'.aliyuncs.com')) { throw 'OSS_ENDPOINT must be the official HTTPS external endpoint for OSS_REGION.' }
    return @{Bucket=$bucket; Region=$region; Endpoint=$env:OSS_ENDPOINT.TrimEnd('/')}
}
function ConvertTo-ReleaseNativeArgument([string]$Value) {
    # CommandLineToArgvW quoting, including trailing slashes before the closing quote.
    return '"'+([regex]::Replace([regex]::Replace($Value,'(\\*)"','$1$1\"'),'(\\+)$','$1$1'))+'"'
}
function Invoke-ReleaseOss([string]$Ossutil, [string[]]$Arguments) {
    $command = Get-Command $Ossutil -ErrorAction Stop
    $exe = $command.Source
    if ([IO.Path]::GetExtension($exe) -ne '.exe') { throw 'Pass the official ossutil 2 executable.' }
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $exe; $start.UseShellExecute = $false; $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true
    foreach ($name in @($start.EnvironmentVariables.Keys)) {
        if ($name -like 'OSS_*' -and $name -notin @('OSS_ACCESS_KEY_ID','OSS_ACCESS_KEY_SECRET','OSS_REGION','OSS_ENDPOINT')) { $start.EnvironmentVariables.Remove($name) }
    }
    $start.Arguments = (($Arguments | ForEach-Object { ConvertTo-ReleaseNativeArgument $_ }) -join ' ')
    $process = New-Object Diagnostics.Process; $process.StartInfo = $start
    try {
        $null = $process.Start()
        $stdout = $process.StandardOutput.ReadToEndAsync(); $stderr = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit(); $raw = $stdout.GetAwaiter().GetResult(); $null = $stderr.GetAwaiter().GetResult()
        if ($process.ExitCode -ne 0) { throw ('OSS operation failed (exit '+$process.ExitCode+'). Check credentials/permissions and network locally; raw tool output is suppressed.') }
        return $raw
    } finally { $process.Dispose() }
}
function Assert-ReleaseOssutil([string]$Ossutil) {
    $version = Invoke-ReleaseOss $Ossutil @('version')
    if ($version -notmatch '(?im)^\s*(?:ossutil(?: version)?[: ]+)?v?2\.[0-9]+\.[0-9]+') { throw 'This release channel requires ossutil 2.' }
}
function Invoke-ReleaseObject([string]$Ossutil, $Config, [ValidateSet('put','get')][string]$Operation, [string]$Key, [string]$File, [switch]$Mutable) {
    if ($Key -cnotmatch '^(releases/v[0-9A-Za-z.-]+/(upgrade\.zip|full\.zip|release\.json|UPGRADE\.md)|channels/stable/latest\.json)$') { throw 'Object key is outside the release channel.' }
    if ($Mutable -and $Key -ne 'channels/stable/latest.json') { throw 'Only latest.json is mutable.' }
    $arguments = @('api',($Operation+'-object'),'--bucket',$Config.Bucket,'--key',$Key,'--region',$Config.Region,'--endpoint',$Config.Endpoint,'--mode','AK','--config-file','NUL','--loglevel','off','--log-file','NUL')
    if ($Operation -eq 'put') {
        if ((Get-Item -LiteralPath $File).Length -gt $script:ReleaseMaxObjectBytes) { throw 'Single-object release upload exceeds 5 GiB.' }
        $arguments += @('--body',('file://'+[IO.Path]::GetFullPath($File)),('--forbid-overwrite='+([string](-not $Mutable)).ToLowerInvariant()))
    } else {
        # API get-object prints its body; exact-object cp writes binary safely.
        # Its HEAD/Get both use oss:GetObject; no ListObjects permission is needed.
        $arguments = @('cp',('oss://'+$Config.Bucket+'/'+$Key),[IO.Path]::GetFullPath($File),'--force','--region',$Config.Region,'--endpoint',$Config.Endpoint,'--mode','AK','--config-file','NUL','--loglevel','off','--log-file','NUL')
    }
    $null = Invoke-ReleaseOss $Ossutil $arguments
    Write-Host ('OSS '+$Operation+' '+$Key+' OK')
}
function Assert-ReleaseManifest($Release, [string]$Version) {
    Assert-ReleaseVersion $Version
    $full = $Release.schemaVersion -eq 2 -and $Release.packageType -ceq 'full'
    if ((-not $full -and $Release.schemaVersion -ne 1) -or $Release.platform -cne 'windows-amd64' -or $Release.to -cne $Version) { throw 'Unsupported or mismatched release manifest.' }
    Assert-ReleaseCommit ([string]$Release.commit)
    if ($full) {
        if ($Release.commit -cnotmatch '^[0-9a-f]{40}$' -or $Release.tag -cne $Version -or $Release.from -or $Release.baselineCommit) { throw 'Invalid full release source identity.' }
    } else {
        if ($Release.packageType -and $Release.packageType -cne 'delta') { throw 'Invalid delta package type.' }
        Assert-ReleaseVersion ([string]$Release.from); Assert-ReleaseCommit ([string]$Release.baselineCommit)
        if ($Release.from -eq $Release.to) { throw 'Invalid release baseline version.' }
    }
    $packageName = if ($full) {'full.zip'} else {'upgrade.zip'}
    foreach ($pair in @(@('package',$packageName),@('instructions','UPGRADE.md'))) {
        $object = $Release.($pair[0])
        if ($object.key -cne ('releases/'+$Version+'/'+$pair[1]) -or $null -eq $object.bytes -or $object.bytes -is [string] -or [long]$object.bytes -le 0 -or [long]$object.bytes -gt $script:ReleaseMaxObjectBytes) { throw 'Invalid release object key or size.' }
        Assert-ReleaseHash ([string]$object.sha256)
    }
}
function Assert-FullReleaseDirectory([string]$Directory, [string]$Version, [string]$Commit) {
    $inventory = Read-ReleaseJson (Resolve-ReleasePath $Directory 'FULL-FILES.json')
    if ($inventory.schemaVersion -ne 1 -or $inventory.version -cne $Version -or $inventory.commit -cne $Commit -or @($inventory.files).Count -eq 0) { throw 'Full inventory identity mismatch.' }
    $allowed = @{'FULL-FILES.json'=$true}
    foreach ($entry in @($inventory.files)) {
        $path = [string]$entry.path
        Assert-ReleaseRelativePath $path
        if ($path -notin @('UPGRADE.md','download-tools/Fetch-Upgrade.ps1','download-tools/Apply-Upgrade.ps1','download-tools/oss-release-common.ps1','download-tools/ossutil.exe')) { Assert-ReleaseProgramPath $path }
        if ($allowed.ContainsKey($path) -or $null -eq $entry.bytes -or $entry.bytes -is [string] -or [long]$entry.bytes -lt 0) { throw 'Invalid full inventory entry.' }
        $allowed[$path] = $true
        Assert-ReleaseFile (Resolve-ReleasePath $Directory $path) $entry.sha256 ([long]$entry.bytes)
    }
    foreach ($required in @('manifest.json','upgrade.ps1','portable-common.ps1','UPGRADE.md','payload/release/server.exe','payload/release/migrate.exe','payload/release/local-auth.exe','payload/release/web/apps/web/server.js','payload/release/web/apps/web/.next/BUILD_ID','payload/bin/u-workers.exe','payload/runtime/node.exe','payload/runtime/pgsql/bin/psql.exe','payload/runtime/caddy/caddy.exe','payload/runtime/opencode/opencode.exe','payload/scripts/native-windows.ps1',('payload/installer/u-workers-setup-'+$Version+'-windows-amd64.exe'),'download-tools/Fetch-Upgrade.ps1','download-tools/Apply-Upgrade.ps1','download-tools/oss-release-common.ps1','download-tools/ossutil.exe')) {
        if (-not $allowed.ContainsKey($required)) { throw ('Full package is missing '+$required) }
    }
    foreach ($file in Get-ReleaseFiles $Directory) {
        $relative = $file.FullName.Substring([IO.Path]::GetFullPath($Directory).TrimEnd('\').Length+1).Replace('\','/')
        if (-not $allowed.ContainsKey($relative)) { throw 'Full package contains an unlisted file.' }
    }
    $manifest = Read-ReleaseJson (Resolve-ReleasePath $Directory 'manifest.json')
    if ($manifest.version -cne $Version -or ([string]$manifest.commit).Length -lt 9 -or -not $Commit.StartsWith([string]$manifest.commit)) { throw 'Full package manifest identity mismatch.' }
}
function Expand-ReleaseArchive([string]$Archive, [string]$Destination) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    if (Test-Path -LiteralPath $Destination) { throw 'Extraction requires a new directory.' }
    $zip = [IO.Compression.ZipFile]::OpenRead($Archive)
    try {
        $seen = @{}; $total = 0L
        if ($zip.Entries.Count -gt 100000) { throw 'Archive has too many entries.' }
        foreach ($entry in $zip.Entries) {
            $name = $entry.FullName.TrimEnd('/')
            Assert-ReleaseRelativePath $name
            $unixType = ([long]$entry.ExternalAttributes -shr 16) -band 61440
            if ($unixType -eq 40960 -or ($entry.ExternalAttributes -band 1024)) { throw 'Archive contains a link.' }
            if ($seen.ContainsKey($name)) { throw 'Archive has duplicate or case-colliding paths.' }; $seen[$name] = $entry
            $total += $entry.Length
            if ($total -gt 20GB) { throw 'Archive expansion exceeds the size limit.' }
        }
        foreach ($name in @($seen.Keys)) {
            $parent = $name
            while ($parent.Contains('/')) {
                $parent = $parent.Substring(0,$parent.LastIndexOf('/'))
                if ($seen.ContainsKey($parent) -and -not $seen[$parent].FullName.EndsWith('/')) { throw 'Archive file/directory collision.' }
            }
        }
        New-Item -ItemType Directory -Path $Destination | Out-Null
        foreach ($entry in $zip.Entries) {
            $path = Resolve-ReleasePath $Destination $entry.FullName.TrimEnd('/')
            if ($entry.FullName.EndsWith('/')) { New-Item -ItemType Directory -Path $path -Force | Out-Null; continue }
            New-Item -ItemType Directory -Path (Split-Path -Parent $path) -Force | Out-Null
            [IO.Compression.ZipFileExtensions]::ExtractToFile($entry,$path,$false)
        }
    } finally { $zip.Dispose() }
}
