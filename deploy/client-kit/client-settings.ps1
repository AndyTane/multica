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
