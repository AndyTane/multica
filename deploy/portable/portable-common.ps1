# Shared helpers for the U Workers portable package.
#
# Windows PowerShell 5.1 compatible on purpose: the package must run on a
# fresh Windows machine with no developer tools and no administrator rights, so
# every package script targets what Windows 10/11 already ships.
#
# The caller dot-sources this file and passes the package root explicitly, so
# the package can be copied anywhere (including paths with spaces or non-ASCII
# characters).

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:PortableManifestName = 'manifest.json'
$script:PortableDeploymentName = 'deployment.json'
$script:PortableLogStamp = Get-Date -Format 'yyyyMMdd-HHmmss'

function Write-PortableJson {
    param([string]$Path, $Value)
    $directory = Split-Path -Parent $Path
    if ($directory -and -not (Test-Path -LiteralPath $directory)) {
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
    }
    $text = $Value | ConvertTo-Json -Depth 12
    $temporary = $Path + '.tmp'
    [IO.File]::WriteAllText($temporary, $text, (New-Object Text.UTF8Encoding($false)))
    Move-Item -LiteralPath $temporary -Destination $Path -Force
}

function Read-PortableJson {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { return $null }
    return (Get-Content -LiteralPath $Path -Raw -Encoding UTF8 | ConvertFrom-Json)
}

function Read-PortableUtf8Text {
    # Everything this package's own tools write is UTF-8: the CLI is a Go program,
    # and its JSON carries the device name, which is the Windows computer name and
    # can be Chinese. Get-Content without -Encoding decodes a file without a BOM
    # in the ANSI code page on Windows PowerShell 5.1 but as UTF-8 on PowerShell 7,
    # so the same captured bytes decoded differently per shell. 5.1 mangled them
    # badly enough to swallow the quote that ends a JSON string, the parse failed,
    # and a running daemon was reported as stopped. Read those bytes with an
    # explicit UTF-8 decoder so both shells see the same text.
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { return '' }
    return [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
}

function Get-PortableDeployment {
    param([string]$Root)
    $config = Read-PortableJson (Join-Path $Root $script:PortableDeploymentName)
    if (-not $config) { throw ('This folder is not an installed U Workers portable package yet: ' + (Join-Path $Root $script:PortableDeploymentName) + ' is missing. Run Setup.cmd first.') }
    return $config
}

function Get-PortableManifest {
    param([string]$Root)
    $manifest = Read-PortableJson (Join-Path $Root $script:PortableManifestName)
    if (-not $manifest) { throw ('The package manifest is missing: ' + (Join-Path $Root $script:PortableManifestName)) }
    return $manifest
}

function Get-PortableVersion {
    param([string]$Root)
    return [string](Get-PortableManifest $Root).version
}

function Get-PortableFile {
    param([string]$Root, [string]$RelativePath)
    $path = Join-Path $Root $RelativePath
    if (-not (Test-Path -LiteralPath $path)) { throw ('The package is missing ' + $RelativePath + '. Re-download the package; do not copy single files between versions.') }
    return $path
}

function Get-PortableServerScriptPath {
    param([string]$Root)
    return (Join-Path $Root 'runtime\native-windows.ps1')
}

function Get-PortableNodePath {
    param([string]$Root)
    return (Join-Path $Root 'runtime\node\node.exe')
}

function Get-PortablePostgresBin {
    param([string]$Root)
    return (Join-Path $Root 'runtime\pgsql\bin')
}

function Get-PortableCliPath {
    param([string]$Root)
    return (Get-PortableFile $Root 'payload\bin\u-workers.exe')
}

function Get-PortableCaddyPath {
    param([string]$Root)
    return (Get-PortableFile $Root 'payload\runtime\caddy\caddy.exe')
}

function Get-PortableOpenCodePath {
    param([string]$Root)
    return (Get-PortableFile $Root 'payload\runtime\opencode\opencode.exe')
}

function Get-PortableCaddyfilePath {
    param([string]$Root)
    return (Join-Path $Root 'https\Caddyfile')
}

function Get-PortableCaddyDataPath {
    param([string]$Root)
    return (Join-Path $Root 'https\data')
}

function Get-PortableRootCertificatePath {
    param([string]$Root)
    return (Join-Path $Root 'https\data\pki\authorities\local\root.crt')
}

function Get-PortablePublicRootCAPath {
    param([string]$Root)
    return (Join-Path $Root 'root-ca.crt')
}

function Get-PortableLogPath {
    param([string]$Root, [string]$Name)
    return (Join-Path $Root ('logs\' + $Name + '-' + $script:PortableLogStamp + '.log'))
}

function New-PortableDirectory {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { New-Item -ItemType Directory -Path $Path -Force | Out-Null }
}

function Test-PortablePortFree {
    param([int]$Port)
    if ($Port -lt 1 -or $Port -gt 65535) { return $false }
    $listener = $null
    try {
        $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, $Port)
        $listener.Start()
        return $true
    } catch {
        return $false
    } finally {
        if ($listener) { $listener.Stop() }
    }
}

function Test-PortablePortListening {
    param([int]$Port, [string]$HostName = '127.0.0.1')
    $client = $null
    try {
        $client = New-Object Net.Sockets.TcpClient
        $async = $client.BeginConnect($HostName, $Port, $null, $null)
        if (-not $async.AsyncWaitHandle.WaitOne(1000)) { return $false }
        $client.EndConnect($async)
        return $true
    } catch {
        return $false
    } finally {
        if ($client) { $client.Close() }
    }
}

# Returns the first usable IPv4 address of this machine (LAN address), never a
# loopback, APIPA (169.254.*) or virtual-adapter placeholder.
function Get-PortablePublicHost {
    param([string]$Override)
    if ($Override) { return $Override }
    $candidates = New-Object System.Collections.Generic.List[string]
    $addresses = @()
    try {
        $addresses = @(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction Stop)
    } catch {
        $addresses = @()
    }
    foreach ($address in $addresses) {
        $ip = [string]$address.IPAddress
        if ([string]$address.InterfaceAlias -match 'Loopback|vEthernet|VirtualBox|VMware|Tailscale|ZeroTier|WSL') { continue }
        if ($address.PrefixOrigin -eq 'WellKnown') { continue }
        $candidates.Add($ip)
    }
    if ($candidates.Count -eq 0) {
        try {
            $hostAddresses = [Net.Dns]::GetHostAddresses([Net.Dns]::GetHostName())
            foreach ($address in $hostAddresses) {
                if ($address.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork) { continue }
                $candidates.Add([string]$address)
            }
        } catch {
        }
    }
    foreach ($candidate in $candidates) {
        $parsed = $null
        if (-not [Net.IPAddress]::TryParse($candidate, [ref]$parsed)) { continue }
        if ([Net.IPAddress]::IsLoopback($parsed)) { continue }
        $bytes = $parsed.GetAddressBytes()
        if ($bytes[0] -eq 169 -and $bytes[1] -eq 254) { continue }
        return $candidate
    }
    return '127.0.0.1'
}

function Test-PortableLoopbackHost {
    # True for the names Caddy serves on the loopback interface only.
    param([string]$HostName)
    if (-not $HostName) { return $false }
    if ($HostName -match '^(?i)localhost$') { return $true }
    $parsed = $null
    if ([Net.IPAddress]::TryParse($HostName, [ref]$parsed)) { return [Net.IPAddress]::IsLoopback($parsed) }
    return $false
}

function Test-PortableElevated {
    # Used to decide whether the installer may add a firewall rule. The package
    # installs without administrator rights; only this one rule needs them, and
    # only when the operator wants LAN access.
    try {
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $principal = New-Object Security.Principal.WindowsPrincipal($identity)
        return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    } catch {
        return $false
    }
}

function Test-PortableLocalAddress {
    # True when this machine currently owns the address. A recorded LAN address
    # goes stale when DHCP hands out a new one; binding it would make Caddy fail
    # to start, so the plan falls back to loopback instead.
    param([string]$Address)
    $parsed = $null
    if (-not [Net.IPAddress]::TryParse([string]$Address, [ref]$parsed)) { return $false }
    if ([Net.IPAddress]::IsLoopback($parsed)) { return $true }
    $addresses = $null
    try { $addresses = @(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction Stop) } catch { $addresses = $null }
    if (-not $addresses) { return $true }
    foreach ($entry in $addresses) {
        if ([string]$entry.IPAddress -eq $Address) { return $true }
    }
    return $false
}

function Get-PortableLanSubnet {
    # The subnet a generated firewall rule should admit: the interface prefix
    # Windows reports, or /24 when it is missing. Scoping the rule to the LAN
    # subnet keeps the LAN entry point off every other network the machine joins.
    param([string]$Address)
    $parsed = $null
    if (-not [Net.IPAddress]::TryParse([string]$Address, [ref]$parsed)) { return '' }
    if ([Net.IPAddress]::IsLoopback($parsed)) { return '' }
    $prefix = 24
    try {
        $entry = Get-NetIPAddress -IPAddress ([string]$Address) -AddressFamily IPv4 -ErrorAction Stop | Select-Object -First 1
        if ($entry -and [int]$entry.PrefixLength -ge 8 -and [int]$entry.PrefixLength -le 30) { $prefix = [int]$entry.PrefixLength }
    } catch {
    }
    $bytes = $parsed.GetAddressBytes()
    $network = @()
    for ($index = 0; $index -lt 4; $index++) {
        $bits = $prefix - ($index * 8)
        if ($bits -ge 8) { $network += $bytes[$index] }
        elseif ($bits -le 0) { $network += 0 }
        else { $network += ($bytes[$index] -band ((0xFF -shl (8 - $bits)) -band 0xFF)) }
    }
    return -join (($network -join '.') + '/' + $prefix)
}

function Test-PortableFirewallToken {
    # Matches one value from a firewall filter list ("Any", a single value, a
    # range such as "13000-13010", or a comma-separated list). An empty filter
    # means the rule does not constrain this field at all.
    param([string]$Value, [string]$Token, [string[]]$Aliases)
    if ([string]::IsNullOrWhiteSpace($Value)) { return $true }
    foreach ($part in ($Value -split ',')) {
        $item = $part.Trim()
        if (-not $item) { continue }
        if ($item -ieq 'Any' -or $item -eq '*') { return $true }
        if ($item -ieq $Token) { return $true }
        foreach ($alias in @($Aliases)) {
            if ($alias -and $item -ieq $alias) { return $true }
        }
        $range = [regex]::Match($item, '^([0-9]+)-([0-9]+)$')
        $parsed = 0
        if ($range.Success -and [int]::TryParse($Token, [ref]$parsed) -and $parsed -ge [int]$range.Groups[1].Value -and $parsed -le [int]$range.Groups[2].Value) { return $true }
    }
    return $false
}

function Test-PortableFirewallProgramMatch {
    # Windows identifies a program rule by the executable path, so only a rule for
    # this installation's caddy.exe covers it. A rule that names no program does
    # not: the security prompt and the permission are both per program, and the
    # machine already has dozens of "any program" rules for packaged apps that
    # must not be mistaken for permission to expose Caddy. A rule left behind for
    # an old package folder does not count either: that stale path is exactly how
    # the prompt comes back after a move.
    param([string]$Program, [string]$ExpectedPath)
    if ([string]::IsNullOrWhiteSpace($Program)) { return $false }
    if ($Program -eq '*' -or $Program -ieq 'Any') { return $false }
    $normalized = $Program
    try { $normalized = [IO.Path]::GetFullPath($Program) } catch { }
    $expected = $ExpectedPath
    try { $expected = [IO.Path]::GetFullPath($ExpectedPath) } catch { }
    return [string]::Equals($normalized, $expected, [StringComparison]::OrdinalIgnoreCase)
}

function Get-PortableFirewallSnapshot {
    # Read-only view of the local inbound rules, flattened so the coverage check
    # is a plain comparison. Reading the policy store needs no administrator
    # rights, which is what lets a standard user find out that a rule exists.
    $rules = @(Get-NetFirewallRule -Direction Inbound -PolicyStore ActiveStore -ErrorAction SilentlyContinue)
    $programs = @{}
    foreach ($filter in @(Get-NetFirewallApplicationFilter -PolicyStore ActiveStore -ErrorAction SilentlyContinue)) {
        $programs[[string]$filter.InstanceID] = [string]$filter.Program
    }
    $ports = @{}
    foreach ($filter in @(Get-NetFirewallPortFilter -PolicyStore ActiveStore -ErrorAction SilentlyContinue)) {
        $ports[[string]$filter.InstanceID] = $filter
    }
    $addresses = @{}
    foreach ($filter in @(Get-NetFirewallAddressFilter -PolicyStore ActiveStore -ErrorAction SilentlyContinue)) {
        $addresses[[string]$filter.InstanceID] = @($filter.RemoteAddress | ForEach-Object { [string]$_ })
    }
    $snapshot = New-Object System.Collections.Generic.List[object]
    foreach ($rule in $rules) {
        $portFilter = $ports[[string]$rule.InstanceID]
        $protocol = ''
        $localPorts = @()
        if ($portFilter) {
            $protocol = [string]$portFilter.Protocol
            $localPorts = @($portFilter.LocalPort | ForEach-Object { [string]$_ })
        }
        $snapshot.Add([pscustomobject]@{
            DisplayName = [string]$rule.DisplayName
            Enabled = [string]$rule.Enabled
            Action = [string]$rule.Action
            Direction = [string]$rule.Direction
            Program = [string]$programs[[string]$rule.InstanceID]
            Protocol = $protocol
            LocalPort = ($localPorts -join ',')
            RemoteAddress = (@($addresses[[string]$rule.InstanceID]) -join ',')
        })
    }
    return $snapshot.ToArray()
}

function ConvertTo-PortableNetworkValue {
    # Canonical network for "a.b.c.d/prefix" or "a.b.c.d/mask". Returns $null for
    # anything it does not understand; callers treat that as "unknown", never as
    # "does not match".
    param([string]$Text)
    $value = ([string]$Text).Trim()
    if (-not $value) { return $null }
    $parts = $value.Split('/')
    if ($parts.Count -ne 2) { return $null }
    $address = $null
    if (-not [Net.IPAddress]::TryParse($parts[0].Trim(), [ref]$address)) { return $null }
    $bytes = $address.GetAddressBytes()
    if ($bytes.Length -ne 4) { return $null }
    $maskPart = $parts[1].Trim()
    $prefix = 0
    if ($maskPart -match '\.') {
        # Windows stores the remote scope as address/mask. A bare number would be
        # accepted by IPAddress.TryParse as a 32-bit address ("32" -> 0.0.0.32),
        # so the prefix form is only used when the part has no dots.
        $maskAddress = $null
        if (-not [Net.IPAddress]::TryParse($maskPart, [ref]$maskAddress)) { return $null }
        $maskBytes = $maskAddress.GetAddressBytes()
        if ($maskBytes.Length -ne 4) { return $null }
        $seenZero = $false
        foreach ($byte in $maskBytes) {
            for ($bit = 7; $bit -ge 0; $bit--) {
                if (($byte -band (1 -shl $bit)) -ne 0) {
                    if ($seenZero) { return $null }
                    $prefix++
                } else {
                    $seenZero = $true
                }
            }
        }
    } else {
        $parsedPrefix = 0
        if (-not [int]::TryParse($maskPart, [ref]$parsedPrefix) -or $parsedPrefix -lt 0 -or $parsedPrefix -gt 32) { return $null }
        $prefix = $parsedPrefix
    }
    $value64 = 0L
    foreach ($byte in $bytes) { $value64 = ($value64 -shl 8) -bor [long]$byte }
    $mask64 = 0L
    for ($index = 0; $index -lt $prefix; $index++) { $mask64 = $mask64 -bor (1L -shl (31 - $index)) }
    return [pscustomobject]@{ Network = ($value64 -band $mask64); Prefix = $prefix }
}

function Test-PortableNetworkContainsAddress {
    # $true/$false, or $null when either side is not a plain IPv4 network.
    param([string]$Network, [string]$Address)
    $target = ConvertTo-PortableNetworkValue ($Address + '/32')
    $range = ConvertTo-PortableNetworkValue $Network
    if (-not $target -or -not $range) { return $null }
    $mask64 = 0L
    for ($index = 0; $index -lt $range.Prefix; $index++) { $mask64 = $mask64 -bor (1L -shl (31 - $index)) }
    return ((($target.Network -band $mask64) -eq $range.Network))
}

function Test-PortableFirewallRemoteAddressCovers {
    # Conservative: $true unless the filter provably excludes the address. An
    # unknown keyword must not switch LAN access off on a machine where the rule
    # demonstrably exists; it only has to catch a scope left over from another
    # network, where the LAN entry point would silently not work.
    param([string]$Value, [string]$Address)
    $parsed = $null
    if (-not [Net.IPAddress]::TryParse([string]$Address, [ref]$parsed)) { return $true }
    if ([string]::IsNullOrWhiteSpace($Value)) { return $true }
    foreach ($entry in ($Value -split ',')) {
        $item = $entry.Trim()
        if (-not $item) { continue }
        if ($item -eq '*' -or $item -ieq 'Any' -or $item -ieq 'LocalSubnet') { return $true }
        $range = [regex]::Match($item, '^([0-9.]+)-([0-9.]+)$')
        if ($range.Success) {
            $low = ConvertTo-PortableNetworkValue ($range.Groups[1].Value + '/32')
            $high = ConvertTo-PortableNetworkValue ($range.Groups[2].Value + '/32')
            $target = ConvertTo-PortableNetworkValue ($Address + '/32')
            if (-not $low -or -not $high -or -not $target) { return $true }
            if ($target.Network -ge $low.Network -and $target.Network -le $high.Network) { return $true }
            continue
        }
        $network = $item
        if ($network -notmatch '/') { $network = $network + '/32' }
        $contains = Test-PortableNetworkContainsAddress -Network $network -Address $Address
        if ($null -eq $contains) { return $true }
        if ($contains) { return $true }
    }
    return $false
}

function Find-PortableFirewallCoverage {
    # The rule that admits this exact listen socket, or $null. Covered means an
    # enabled inbound allow rule for TCP (or any protocol), this port (or any
    # port), and this caddy.exe path (or any program).
    param([object[]]$Rules, [string]$ProgramPath, [int]$Port, [string]$Address)
    foreach ($rule in @($Rules)) {
        if (-not $rule) { continue }
        if ([string]$rule.Direction -ne 'Inbound') { continue }
        if ([string]$rule.Action -ne 'Allow') { continue }
        if (-not [string]::IsNullOrWhiteSpace([string]$rule.Enabled) -and [string]$rule.Enabled -ne 'True') { continue }
        if (-not (Test-PortableFirewallToken -Value ([string]$rule.Protocol) -Token 'TCP' -Aliases @('6'))) { continue }
        if (-not (Test-PortableFirewallToken -Value ([string]$rule.LocalPort) -Token ([string]$Port))) { continue }
        if (-not (Test-PortableFirewallProgramMatch -Program ([string]$rule.Program) -ExpectedPath $ProgramPath)) { continue }
        if ($Address -and -not (Test-PortableFirewallRemoteAddressCovers -Value ([string]$rule.RemoteAddress) -Address $Address)) { continue }
        return $rule
    }
    return $null
}

function Install-PortableFirewallRule {
    # Adds the program-scoped inbound rule for the LAN HTTPS entry point. The rule
    # names the installed caddy.exe path, which is what Windows matches, so an
    # in-place upgrade of the same folder keeps the permission and never asks
    # again. Without administrator rights nothing is attempted: this returns a
    # result instead of a prompt.
    param([string]$ProgramPath, [int]$Port, [string]$RemoteAddress, [string]$DisplayName)
    $result = [pscustomobject]@{ Added = $false; DisplayName = $DisplayName; Detail = '' }
    if (-not (Test-PortableElevated)) {
        $result.Detail = 'adding a firewall rule needs administrator rights'
        return $result
    }
    if (-not (Get-Command New-NetFirewallRule -ErrorAction SilentlyContinue)) {
        $result.Detail = 'New-NetFirewallRule is not available on this machine'
        return $result
    }
    try {
        foreach ($existing in @(Get-NetFirewallRule -DisplayName $DisplayName -ErrorAction SilentlyContinue)) {
            Remove-NetFirewallRule -Name $existing.Name -ErrorAction SilentlyContinue
        }
        $arguments = @{
            DisplayName = $DisplayName
            Description = 'U Workers portable: allow the LAN HTTPS entry point for this installation only.'
            Direction = 'Inbound'
            Action = 'Allow'
            Program = $ProgramPath
            Protocol = 'TCP'
            LocalPort = $Port
            Profile = 'Any'
            Enabled = 'True'
        }
        if ($RemoteAddress) { $arguments.RemoteAddress = $RemoteAddress }
        New-NetFirewallRule @arguments | Out-Null
        $result.Added = $true
        $result.Detail = 'added'
    } catch {
        $result.Detail = $_.Exception.Message
    }
    return $result
}

function Remove-PortableFirewallRule {
    param([string]$DisplayName)
    if (-not $DisplayName) { return $false }
    if (-not (Test-PortableElevated)) { return $false }
    try {
        $removed = $false
        foreach ($existing in @(Get-NetFirewallRule -DisplayName $DisplayName -ErrorAction SilentlyContinue)) {
            Remove-NetFirewallRule -Name $existing.Name -ErrorAction Stop
            $removed = $true
        }
        return $removed
    } catch {
        return $false
    }
}

function Resolve-PortableLanPlan {
    # One decision point for "does this installation listen beyond loopback":
    # the requested address is exposed only when an inbound firewall rule already
    # covers this caddy.exe, or when this process may add one. Otherwise the
    # entry point stays loopback-only and the plan carries the sentence that
    # tells the operator what that means and how to change it.
    param(
        [string]$Root,
        [string]$RequestedHost,
        [int]$HttpsPort,
        [string]$CaddyPath,
        [switch]$AllowRuleCreation,
        [object[]]$Rules,
        [switch]$SkipFirewallProbe
    )
    $hostName = [string]$RequestedHost
    if (-not $hostName) { $hostName = Get-PortablePublicHost }
    if (-not $CaddyPath -and $Root) {
        try { $CaddyPath = Get-PortableCaddyPath $Root } catch { $CaddyPath = '' }
    }
    $plan = [pscustomobject]@{
        RequestedHost = $hostName
        LanHost = '127.0.0.1'
        Exposed = $false
        Firewall = 'loopback'
        FirewallRule = ''
        RemoteAddress = ''
        Message = ('LAN access is off: the board is served on this machine at https://localhost:' + $HttpsPort + '/ only.')
    }
    if (Test-PortableLoopbackHost $hostName) {
        return $plan
    }
    $parsed = $null
    if (-not [Net.IPAddress]::TryParse($hostName, [ref]$parsed)) {
        # A DNS name cannot be bound or covered by an address rule; the package
        # has always signed the certificate for the address, not the name.
        $plan.Firewall = 'hostname'
        $plan.Message = ('LAN access is off: ' + $hostName + ' is not an IPv4 address of this machine, so the board stays on https://localhost:' + $HttpsPort + '/.')
        return $plan
    }
    if (-not (Test-PortableLocalAddress $hostName)) {
        $plan.Firewall = 'stale-address'
        $plan.Message = ('LAN access is off: ' + $hostName + ' is not an address of this machine any more (the network changed), so the board stays on https://localhost:' + $HttpsPort + '/ until Setup.cmd or Upgrade.cmd re-issues the certificate for the current address.')
        return $plan
    }
    if (-not $CaddyPath) {
        $plan.Firewall = 'no-caddy'
        $plan.Message = ('LAN access is off: the packaged Caddy executable was not found, so the board stays on https://localhost:' + $HttpsPort + '/.')
        return $plan
    }
    $remoteAddress = Get-PortableLanSubnet $hostName
    $ruleName = 'UWorkers-Portable-HTTPS-' + $HttpsPort
    $snapshot = @()
    if ($SkipFirewallProbe) {
        $snapshot = @($Rules)
    } else {
        try { $snapshot = @(Get-PortableFirewallSnapshot) } catch { $snapshot = @() }
    }
    $coverage = Find-PortableFirewallCoverage -Rules $snapshot -ProgramPath $CaddyPath -Port $HttpsPort -Address $hostName
    $firewall = 'existing'
    if (-not $coverage) {
        if ($AllowRuleCreation) {
            $install = Install-PortableFirewallRule -ProgramPath $CaddyPath -Port $HttpsPort -RemoteAddress $remoteAddress -DisplayName $ruleName
            if ($install.Added) {
                $coverage = [pscustomobject]@{ DisplayName = [string]$install.DisplayName }
                $firewall = 'created'
            } else {
                $firewall = 'needs-admin'
                $plan.Message = ('LAN access is off: Windows Firewall has no inbound rule for this installation and one cannot be added here (' + [string]$install.Detail + '). The board works on this machine at https://localhost:' + $HttpsPort + '/. Run Setup.cmd once as an administrator to add the rule, or ask IT for an inbound TCP ' + $HttpsPort + ' rule for ' + $CaddyPath + '.')
            }
        } else {
            $firewall = 'needs-admin'
            $plan.Message = ('LAN access is off: Windows Firewall has no inbound rule for this installation. The board works on this machine at https://localhost:' + $HttpsPort + '/. Run Setup.cmd once as an administrator to add the rule.')
        }
    } else {
        $plan.FirewallRule = [string]$coverage.DisplayName
    }
    if (-not $coverage) { $plan.Firewall = $firewall }
    if ($coverage) {
        $plan.Exposed = $true
        $plan.LanHost = $hostName
        $plan.Firewall = $firewall
        $plan.RemoteAddress = $remoteAddress
        $plan.FirewallRule = [string]$coverage.DisplayName
        $scope = ''
        if ($remoteAddress) { $scope = ' from ' + $remoteAddress }
        $plan.Message = ('LAN entry: https://' + $hostName + ':' + $HttpsPort + '/ (firewall rule "' + $plan.FirewallRule + '" allows TCP ' + $HttpsPort + $scope + ').')
    }
    return $plan
}

function Get-PortableLanReportFields {
    param($Plan)
    return [ordered]@{
        lanRequestedHost = [string]$Plan.RequestedHost
        lanHost = [string]$Plan.LanHost
        lanExposed = [bool]$Plan.Exposed
        lanFirewall = [string]$Plan.Firewall
        lanFirewallRule = [string]$Plan.FirewallRule
        lanMessage = [string]$Plan.Message
    }
}

function Update-PortableLanReport {
    # Keeps the install report honest after a later start (for example when an
    # administrator added the missing rule since the install).
    param([string]$Root, $Plan)
    $reportPath = Join-Path $Root 'data\install-report.json'
    $report = Read-PortableJson $reportPath
    if (-not $report) { return $null }
    foreach ($entry in (Get-PortableLanReportFields $Plan).GetEnumerator()) {
        $report | Add-Member -NotePropertyName $entry.Key -NotePropertyValue $entry.Value -Force
    }
    Write-PortableJson $reportPath $report
    return $report
}

function Get-PortableOptionalProperty {
    # Reads an optional property without tripping Set-StrictMode: an install
    # report written by an older package does not carry the LAN fields.
    param($Object, [string]$Name, $Default = $null)
    if (-not $Object) { return $Default }
    $property = $Object.PSObject.Properties[$Name]
    if (-not $property -or $null -eq $property.Value) { return $Default }
    return $property.Value
}

function Get-PortableRequestedLanHostFromConfig {
    # What the operator asked for, not what was granted: a loopback value means
    # "this machine only" and must stay that way across restarts.
    param([string]$Root)
    $report = Read-PortableJson (Join-Path $Root 'data\install-report.json')
    $requested = [string](Get-PortableOptionalProperty $report 'lanRequestedHost' '')
    if ($requested) { return $requested }
    $served = [string](Get-PortableOptionalProperty $report 'lanHost' '')
    if ($served) { return $served }
    return (Get-PortablePublicHost)
}

function Get-PortableCaddyfile {
    param(
        [string]$Root,
        [string]$PublicHost,
        [int]$HttpsPort,
        [int]$ApiPort,
        [int]$WebPort,
        [bool]$LanExposed
    )
    # Caddy accepts forward slashes on Windows and this keeps the file free of
    # escape sequences; the storage path keeps the private CA inside the
    # package so moving the folder moves the trust anchor with it.
    $storage = ((Join-Path $Root 'https\data') -replace '\\', '/')
    $hosts = @('localhost', '127.0.0.1')
    $loopbackOnly = $true
    if ($LanExposed -and $PublicHost -and -not (Test-PortableLoopbackHost $PublicHost)) {
        $hosts += $PublicHost
        $loopbackOnly = $false
    }
    $sites = ($hosts | ForEach-Object { 'https://' + $_ + ':' + $HttpsPort }) -join ', '
    # Never leave the listen address to Caddy's default wildcard. A wildcard
    # socket is what makes Windows Firewall offer "allow this program" for every
    # interface and every network profile that comes up; binding the exact
    # addresses keeps the loopback entry private and limits the LAN entry to the
    # one address the operator asked for.
    $binds = @('127.0.0.1')
    if (-not $loopbackOnly) { $binds += $PublicHost }
    $newLine = [Environment]::NewLine
    # Parenthesise every concatenation inside the array literal: PowerShell
    # otherwise folds the following lines into the same element.
    $lines = @(
        '{',
        # Automatic root-certificate installation makes Caddy block on a Windows
        # security prompt when it starts from a task or a service context, and it
        # needs administrator rights the package promises not to use. Trust is
        # installed explicitly and opt-in instead: Trust-Browser.cmd with its
        # click-through prompt, or `u-workers connect --root-ca` on a client.
        '    skip_install_trust',
        '    admin off',
        '    auto_https disable_redirects',
        ('    storage file_system "' + $storage + '"'),
        # HTTP/3 makes Caddy open a second, UDP listener on the same port. A
        # TCP-only firewall rule leaves that UDP socket unrecognised, so Windows
        # Firewall asks for permission on every start, at a machine where nobody
        # can click it. HTTP/2 over TCP is plenty for a LAN board.
        '    servers {',
        '        protocols h1 h2',
        '    }',
        '}',
        '',
        ($sites + ' {'),
        ('    bind ' + ($binds -join ' ')),
        '    tls internal',
        '    encode gzip',
        '    @api path /api/* /auth/* /ws /uploads/* /v1/* /health* /readyz /metrics',
        ('    reverse_proxy @api 127.0.0.1:' + $ApiPort),
        ('    reverse_proxy 127.0.0.1:' + $WebPort),
        '}',
        ''
    )
    return ($lines -join $newLine)
}

function Get-PortableProcessRecord {
    param([int]$ProcessId, [string]$ExpectedPath)
    $process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
    if (-not $process) { return $null }
    $path = $null
    try { $path = $process.Path } catch { return $null }
    if (-not $path) { return $null }
    if ($ExpectedPath) { $path = [IO.Path]::GetFullPath($ExpectedPath) }
    return [pscustomobject]@{ Pid = $process.Id; StartedAt = $process.StartTime.ToUniversalTime().ToString('o'); Executable = $path }
}

function Test-PortableProcessRecord {
    param($Record)
    if (-not $Record) { return $false }
    $process = Get-Process -Id $Record.Pid -ErrorAction SilentlyContinue
    if (-not $process) { return $false }
    try {
        $path = $process.Path
        return ($path -eq $Record.Executable -and
            [math]::Abs(($process.StartTime.ToUniversalTime() - ([datetime]$Record.StartedAt).ToUniversalTime()).TotalMilliseconds) -lt 100)
    } catch {
        return $false
    }
}

function Stop-PortableProcessRecord {
    param($Record)
    if (-not $Record) { return }
    if (Test-PortableProcessRecord $Record) {
        $process = Get-Process -Id $Record.Pid -ErrorAction SilentlyContinue
        Stop-Process -Id $Record.Pid -ErrorAction SilentlyContinue
        if ($process) { $null = $process.WaitForExit(5000) }
    }
}

function ConvertTo-PortableArgumentLine {
    param([string[]]$Arguments)
    $quoted = foreach ($argument in $Arguments) {
        $value = [string]$argument
        if ($value -match '[\r\n]') { throw 'Refusing to launch a process with a newline in an argument.' }
        if ($value -match '^[^\s"]+$') { $value } else { '"' + ($value -replace '"', '""') + '"' }
    }
    return ($quoted -join ' ')
}

function Start-PortableHiddenProcess {
    param(
        [string]$Executable,
        [string[]]$Arguments,
        [string]$WorkingDirectory,
        [hashtable]$Environment,
        [string]$LogPath,
        [string]$ErrorLogPath
    )
    if (-not (Test-Path -LiteralPath $Executable)) { throw ('Missing executable: ' + $Executable) }
    $saved = @{}
    try {
        if ($Environment) {
            foreach ($key in $Environment.Keys) {
                $saved[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
                [Environment]::SetEnvironmentVariable($key, [string]$Environment[$key], 'Process')
            }
        }
        $options = @{
            FilePath = $Executable
            WorkingDirectory = $WorkingDirectory
            WindowStyle = 'Hidden'
            PassThru = $true
            RedirectStandardOutput = $LogPath
        }
        if ($ErrorLogPath) { $options.RedirectStandardError = $ErrorLogPath }
        if ($Arguments -and $Arguments.Count -gt 0) { $options.ArgumentList = (ConvertTo-PortableArgumentLine $Arguments) }
        $process = Start-Process @options
        return (Get-PortableProcessRecord -ProcessId $process.Id -ExpectedPath $Executable)
    } finally {
        foreach ($key in $saved.Keys) {
            $original = $saved[$key]
            if ($null -eq $original) { $original = [NullString]::Value }
            [Environment]::SetEnvironmentVariable($key, $original, 'Process')
        }
    }
}

function ConvertTo-PortableCommandToken {
    # Quote one argument for a generated .cmd file. cmd.exe expands % and treats
    # characters such as & and | outside quotes, so anything that is not a plain
    # path/number token is wrapped in double quotes and % is doubled. Trailing
    # backslashes are doubled so the closing quote is not escaped for the
    # receiving program.
    param([string]$Value)
    if ($Value -match '[\r\n]') { throw 'Refusing to launch a process with a newline in an argument.' }
    $escaped = $Value -replace '%', '%%'
    if ($escaped -and $escaped -match '^[A-Za-z0-9_.:/-]+$') { return $escaped }
    $escaped = $escaped -replace '"', '""'
    $trailing = [regex]::Match($escaped, '\\+$').Value
    if ($trailing) { $escaped = $escaped + $trailing }
    return '"' + $escaped + '"'
}

function Invoke-PortableCommand {
    # Runs one command and returns its exit code plus captured stdout and stderr.
    #
    # The command is written into a generated batch file that cmd.exe runs with
    # both streams redirected straight to files. PowerShell pipelines and
    # redirections are avoided on purpose: a command that leaves a long-lived
    # child behind (the native supervisor, the daemon) keeps an inherited pipe
    # open, so a pipeline would wait forever. The batch file records %ERRORLEVEL%
    # in its own file afterwards, because Windows PowerShell 5.1 — what Setup.cmd
    # runs on a fresh machine — leaves Start-Process' ExitCode empty unless
    # -Wait is passed, and -Wait waits for those same long-lived children.
    #
    # -Utf8Output decodes the captured streams as UTF-8. Use it for the package's
    # own CLI, which always writes UTF-8; tools that write the console code page
    # (caddy, PostgreSQL, robocopy, the native server script) keep the default.
    param([string]$Executable, [string[]]$Arguments, [hashtable]$Environment, [string]$WorkingDirectory, [int]$TimeoutSec = 3600, [switch]$Utf8Output)
    $temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-command-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $temporaryDirectory -Force | Out-Null
    $stdoutPath = Join-Path $temporaryDirectory 'stdout.log'
    $stderrPath = Join-Path $temporaryDirectory 'stderr.log'
    $exitCodePath = Join-Path $temporaryDirectory 'exit-code.txt'
    $batchPath = Join-Path $temporaryDirectory 'run-command.cmd'
    $process = $null
    try {
        $lines = New-Object System.Collections.Generic.List[string]
        $lines.Add('@echo off')
        $lines.Add('setlocal')
        if ($Environment) {
            foreach ($key in $Environment.Keys) {
                $name = ([string]$key) -replace '[^A-Za-z0-9_]', ''
                if (-not $name) { continue }
                $value = ([string]$Environment[$key]) -replace '%', '%%'
                $lines.Add('set "' + $name + '=' + $value + '"')
            }
        }
        if ($WorkingDirectory) { $lines.Add('cd /d "' + (([string]$WorkingDirectory) -replace '%', '%%') + '"') }
        $tokens = @()
        foreach ($argument in @($Arguments)) { $tokens += (ConvertTo-PortableCommandToken ([string]$argument)) }
        $commandLine = '"' + ([string]$Executable) + '"'
        if ($tokens.Count -gt 0) { $commandLine += ' ' + ($tokens -join ' ') }
        $lines.Add($commandLine + ' 1> "' + $stdoutPath + '" 2> "' + $stderrPath + '"')
        $lines.Add('> "' + $exitCodePath + '" echo %ERRORLEVEL%')
        $ansi = [Text.Encoding]::GetEncoding([Globalization.CultureInfo]::CurrentCulture.TextInfo.ANSICodePage)
        [IO.File]::WriteAllLines($batchPath, $lines, $ansi)
        $process = Start-Process -FilePath (Join-Path $env:SystemRoot 'System32\cmd.exe') -ArgumentList ('/d /c ""' + $batchPath + '""') -WindowStyle Hidden -PassThru
        $deadline = (Get-Date).AddSeconds($TimeoutSec)
        while (-not (Test-Path -LiteralPath $exitCodePath)) {
            if ($process.HasExited) {
                Start-Sleep -Milliseconds 200
                break
            }
            if ((Get-Date) -gt $deadline) {
                Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
                break
            }
            Start-Sleep -Milliseconds 100
        }
        $exitCode = $null
        if (Test-Path -LiteralPath $exitCodePath) {
            $parsed = 0
            $rawCode = (Get-Content -LiteralPath $exitCodePath -Raw -ErrorAction SilentlyContinue)
            if ($rawCode -and [int]::TryParse(([string]$rawCode).Trim(), [ref]$parsed)) { $exitCode = $parsed }
        }
        $output = ''
        foreach ($path in @($stdoutPath, $stderrPath)) {
            if (Test-Path -LiteralPath $path) {
                if ($Utf8Output) { $output += (Read-PortableUtf8Text $path) }
                else { $output += (Get-Content -LiteralPath $path -Raw -ErrorAction SilentlyContinue) }
            }
        }
        if ($null -eq $exitCode) {
            $exitCode = 1
            $output += [Environment]::NewLine + 'The command did not report an exit code.'
        }
        return [pscustomobject]@{ ExitCode = $exitCode; Output = [string]$output }
    } finally {
        if ($process -and -not $process.HasExited) { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue }
        Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
}
function Invoke-PortableSql {
    param($Root, [string]$Sql, [string]$Database = 'postgres', [string]$FieldSeparator = '|')
    $config = Get-PortableDeployment $Root
    $psql = Join-Path (Get-PortablePostgresBin $Root) 'psql.exe'
    if (-not (Test-Path -LiteralPath $psql)) { throw ('PostgreSQL tools are missing under ' + (Get-PortablePostgresBin $Root)) }
    $env:PGPASSWORD = [string]$config.DatabasePassword
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $psql -h 127.0.0.1 -p ([int]$config.DatabasePort) -U ([string]$config.DatabaseUser) -d $Database -X -A -t -v ON_ERROR_STOP=1 -F $FieldSeparator -c $Sql 2>&1 | Out-String
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
        Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    }
    if ($exitCode -ne 0) { throw ('PostgreSQL command failed: ' + $output.Trim()) }
    return @($output -split "\r?\n" | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ -ne '' })
}

function New-PortablePassword {
    param([int]$Length = 24)
    $alphabet = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'
    $bytes = New-Object byte[] $Length
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    $characters = New-Object char[] $Length
    for ($index = 0; $index -lt $Length; $index++) {
        $characters[$index] = $alphabet[$bytes[$index] % $alphabet.Length]
    }
    return (-join $characters)
}

# HTTPS requests from the package scripts go through curl.exe. It ships with
# Windows 10 1803+ and validates a private CA with --cacert without touching
# the user's certificate store, and it behaves the same from Windows PowerShell
# 5.1 and PowerShell 7 (unlike .NET TLS callbacks, which need a runspace on the
# networking thread only the older runtime has).
function Invoke-PortableWebRequest {
    param(
        [string]$Uri,
        [string]$Method = 'GET',
        [string]$Body,
        [hashtable]$Headers,
        [string]$RootCAPath,
        [int]$TimeoutSec = 30
    )
    $curl = Join-Path $env:SystemRoot 'System32\curl.exe'
    if (-not (Test-Path -LiteralPath $curl)) { $curl = 'curl.exe' }
    $workingDirectory = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-curl-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $workingDirectory -Force | Out-Null
    $bodyFile = Join-Path $workingDirectory 'request-body.txt'
    $responseFile = Join-Path $workingDirectory 'response-body.txt'
    $stderrFile = Join-Path $workingDirectory 'curl-error.txt'
    try {
        $arguments = @('--silent', '--show-error', '--max-time', [string]$TimeoutSec, '--output', $responseFile, '--write-out', '%{http_code}')
        if ($RootCAPath) { $arguments += @('--cacert', $RootCAPath, '--ssl-no-revoke') }
        if ($Method -and $Method -ne 'GET') { $arguments += @('--request', $Method) }
        if ($Body) {
            [IO.File]::WriteAllText($bodyFile, $Body, (New-Object Text.UTF8Encoding($false)))
            $arguments += @('--data-binary', ('@' + $bodyFile))
            $arguments += @('--header', 'Content-Type: application/json; charset=utf-8')
        }
        if ($Headers) {
            foreach ($key in $Headers.Keys) { $arguments += @('--header', ([string]$key + ': ' + [string]$Headers[$key])) }
        }
        $arguments += $Uri
        $statusText = & $curl @arguments 2> $stderrFile
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) {
            $details = ''
            if (Test-Path -LiteralPath $stderrFile) { $details = [string](Get-Content -LiteralPath $stderrFile -Raw -ErrorAction SilentlyContinue) }
            throw ('HTTPS request failed (' + $exitCode + '): ' + $details.Trim())
        }
        $statusCode = 0
        if (-not [int]::TryParse(([string]$statusText).Trim(), [ref]$statusCode)) { throw ('HTTPS request returned no status code: ' + $Uri) }
        # The server answers with UTF-8 JSON (runtime names and other values carry
        # the device name); read the response body the same way in both shells.
        $content = ''
        if (Test-Path -LiteralPath $responseFile) { $content = [string](Read-PortableUtf8Text $responseFile) }
        return [pscustomobject]@{ StatusCode = $statusCode; Content = $content; Headers = @{} }
    } finally {
        Remove-Item -LiteralPath $workingDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function ConvertFrom-PortableResponse {
    param($Response)
    if (-not $Response -or -not $Response.Content) { return $null }
    return ($Response.Content | ConvertFrom-Json)
}

function Wait-PortableCondition {
    param([scriptblock]$Condition, [int]$TimeoutSec, [string]$Description, [int]$IntervalMs = 500)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            if (& $Condition) { return $true }
        } catch {
        }
        Start-Sleep -Milliseconds $IntervalMs
    }
    throw ('Timed out waiting for ' + $Description)
}

function Wait-PortableHttp {
    param([string]$Uri, [string]$RootCAPath, [int]$TimeoutSec = 90, [string]$Description)
    $ok = Wait-PortableCondition -TimeoutSec $TimeoutSec -Description ($Description + ' (' + $Uri + ')') -Condition {
        try {
            $response = Invoke-PortableWebRequest -Uri $Uri -RootCAPath $RootCAPath -TimeoutSec 5
            return ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500)
        } catch {
            return $false
        }
    }
    return $ok
}

function Invoke-PortableTlsHandshake {
    # Caddy issues the leaf certificate while serving the first TLS handshake
    # for a site; a request with the default (untrusted) validation is expected
    # to fail, but it makes Caddy create the certificate and its CA on disk.
    param([string]$Uri)
    try { Invoke-PortableWebRequest -Uri $Uri -TimeoutSec 10 | Out-Null } catch { }
}

function Export-PortableRootCA {
    param([string]$Root, [int]$TimeoutSec = 60)
    $source = Get-PortableRootCertificatePath $Root
    Wait-PortableCondition -TimeoutSec $TimeoutSec -Description 'the generated root CA certificate' -Condition { Test-Path -LiteralPath $source } | Out-Null
    $destination = Get-PortablePublicRootCAPath $Root
    Copy-Item -LiteralPath $source -Destination $destination -Force
    $publicDirectory = Join-Path $Root 'https\public'
    New-PortableDirectory $publicDirectory
    Copy-Item -LiteralPath $source -Destination (Join-Path $publicDirectory 'root-ca.crt') -Force
    $certificate = New-Object Security.Cryptography.X509Certificates.X509Certificate2($source)
    return [pscustomobject]@{
        Path = $destination
        Sha256 = ($certificate.GetCertHashString([Security.Cryptography.HashAlgorithmName]::SHA256))
        Subject = $certificate.Subject
        Thumbprint = $certificate.Thumbprint
    }
}

function Test-PortableRootCAInstalled {
    param([string]$Thumbprint)
    $store = New-Object Security.Cryptography.X509Certificates.X509Store('Root', 'CurrentUser')
    try {
        $store.Open([Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly)
        $matches = @($store.Certificates | Where-Object { $_.Thumbprint -eq $Thumbprint })
        return ($matches.Count -gt 0)
    } finally {
        $store.Close()
    }
}

function Install-PortableBrowserTrust {
    param([string]$Root, [int]$TimeoutSec = 90)
    # A root-store install raises the standard Windows security-warning dialog;
    # there is no documented way to suppress it, so this waits a bounded time
    # for the operator to click 是/Yes and reports failure without touching the
    # deployment. The caller can continue with -SkipBrowserTrust.
    $certificatePath = Get-PortablePublicRootCAPath $Root
    if (-not (Test-Path -LiteralPath $certificatePath)) { throw 'root-ca.crt is missing; run the HTTPS step first.' }
    $certificate = New-Object Security.Cryptography.X509Certificates.X509Certificate2($certificatePath)
    if (Test-PortableRootCAInstalled $certificate.Thumbprint) {
        return [pscustomobject]@{ Trusted = $true; AlreadyInstalled = $true; Thumbprint = $certificate.Thumbprint }
    }
    $process = Start-Process -FilePath 'certutil.exe' -ArgumentList @('-user', '-addstore', 'Root', ('"' + $certificatePath + '"')) -PassThru
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-PortableRootCAInstalled $certificate.Thumbprint) {
            return [pscustomobject]@{ Trusted = $true; AlreadyInstalled = $false; Thumbprint = $certificate.Thumbprint }
        }
        Start-Sleep -Milliseconds 500
    }
    if (-not $process.HasExited) { $process.Kill() }
    return [pscustomobject]@{ Trusted = $false; AlreadyInstalled = $false; Thumbprint = $certificate.Thumbprint }
}

function Remove-PortableBrowserTrust {
    param([string]$Root)
    $certificatePath = Get-PortablePublicRootCAPath $Root
    if (-not (Test-Path -LiteralPath $certificatePath)) { return $false }
    $certificate = New-Object Security.Cryptography.X509Certificates.X509Certificate2($certificatePath)
    if (-not (Test-PortableRootCAInstalled $certificate.Thumbprint)) { return $false }
    & certutil.exe -user -delstore Root $certificate.Thumbprint 2>&1 | Out-Null
    return $true
}

function Get-PortableStartupShortcutPath {
    return (Join-Path ([Environment]::GetFolderPath('Startup')) 'U Workers Portable.vbs')
}

function Install-PortableStartup {
    param([string]$Root)
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $startScript = Join-Path $Root 'start.ps1'
    if (-not (Test-Path -LiteralPath $startScript)) { throw 'start.ps1 is missing from the package.' }
    $startCommand = '"' + $shell + '" -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "' + $startScript + '"'
    $escaped = $startCommand.Replace('"', '""')
    $vbs = 'CreateObject("WScript.Shell").Run "' + $escaped + '", 0, False' + [Environment]::NewLine
    $shortcut = Get-PortableStartupShortcutPath
    [IO.File]::WriteAllText($shortcut, $vbs, (New-Object Text.UnicodeEncoding($false, $true)))
    return $shortcut
}

function Remove-PortableStartup {
    $shortcut = Get-PortableStartupShortcutPath
    if (Test-Path -LiteralPath $shortcut) {
        Remove-Item -LiteralPath $shortcut -Force
        return $true
    }
    return $false
}

function Start-PortableCaddy {
    param([string]$Root, [string]$PublicHost, [int]$HttpsPort, [int]$ApiPort, [int]$WebPort, [bool]$LanExposed)
    $caddyExe = Get-PortableCaddyPath $Root
    $statePath = Join-Path $Root 'state\caddy.json'
    $existing = Read-PortableJson $statePath
    if ($existing -and (Test-PortableProcessRecord $existing)) { return $existing }
    Remove-Item -LiteralPath $statePath -Force -ErrorAction SilentlyContinue
    if (Test-PortablePortListening $HttpsPort) { throw ('Port ' + $HttpsPort + ' is already in use; another HTTPS listener is running.') }
    New-PortableDirectory (Join-Path $Root 'state')
    New-PortableDirectory (Join-Path $Root 'logs')
    New-PortableDirectory (Join-Path $Root 'https')
    $caddyfile = Get-PortableCaddyfilePath $Root
    [IO.File]::WriteAllText($caddyfile, (Get-PortableCaddyfile -Root $Root -PublicHost $PublicHost -HttpsPort $HttpsPort -ApiPort $ApiPort -WebPort $WebPort -LanExposed $LanExposed), (New-Object Text.UTF8Encoding($false)))
    $validation = Invoke-PortableCommand -Executable $caddyExe -Arguments @('validate', '--config', $caddyfile, '--adapter', 'caddyfile')
    if ($validation.ExitCode -ne 0) { throw ('Caddy rejected the generated HTTPS configuration: ' + $validation.Output.Trim()) }
    $record = Start-PortableHiddenProcess -Executable $caddyExe -Arguments @('run', '--config', $caddyfile, '--adapter', 'caddyfile') -WorkingDirectory $Root -LogPath (Get-PortableLogPath $Root 'caddy') -ErrorLogPath (Get-PortableLogPath $Root 'caddy-error')
    Write-PortableJson $statePath $record
    Wait-PortableCondition -TimeoutSec 60 -Description ('the HTTPS listener on port ' + $HttpsPort) -Condition { Test-PortablePortListening $HttpsPort } | Out-Null
    return $record
}

function Stop-PortableCaddy {
    param([string]$Root)
    $statePath = Join-Path $Root 'state\caddy.json'
    $record = Read-PortableJson $statePath
    if (-not $record) { return $false }
    Stop-PortableProcessRecord $record
    Remove-Item -LiteralPath $statePath -Force -ErrorAction SilentlyContinue
    return $true
}

function Invoke-PortableNativeAction {
    param([string]$Root, [string]$Action, [string[]]$ExtraArguments)
    $script = Get-PortableServerScriptPath $Root
    if (-not (Test-Path -LiteralPath $script)) { throw ('The server script is missing: ' + $script + '. Run Setup.cmd again.') }
    $shell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $script, '-Action', $Action, '-Root', $Root)
    if ($ExtraArguments) { $arguments += $ExtraArguments }
    $result = Invoke-PortableCommand -Executable $shell -Arguments $arguments
    if ($result.ExitCode -ne 0) {
        throw ('The server ' + $Action + ' step failed: ' + $result.Output.Trim())
    }
    return $result.Output
}

function Get-PortableCorsOrigins {
    # A loopback-only installation asks for 127.0.0.1 here and would otherwise
    # list the same origin twice.
    param([string]$PublicHost, [int]$HttpsPort)
    $origins = New-Object System.Collections.Generic.List[string]
    foreach ($hostName in @($PublicHost, 'localhost', '127.0.0.1')) {
        if (-not $hostName) { continue }
        $origin = 'https://' + $hostName + ':' + $HttpsPort
        if (-not $origins.Contains($origin)) { $origins.Add($origin) }
    }
    return ($origins -join ',')
}

function Get-PortableCliEnvironment {
    param([string]$Root)
    # The daemon discovers agent CLIs on PATH. Prepending the bundled opencode
    # directory is what makes an opencode runtime appear without installing
    # anything on the machine, and the bundled CLI directory is how the
    # installer's own commands resolve.
    $bin = Join-Path $Root 'payload\bin'
    $opencode = Join-Path $Root 'payload\runtime\opencode'
    $path = $bin + ';' + $opencode + ';' + [Environment]::GetEnvironmentVariable('PATH', 'Process')
    # A portable install may be prepared from inside another agent's task
    # (for example while the package is being tested): never let that task
    # identity leak into the installed CLI. The CLI refuses human commands such
    # as login/connect inside a daemon-managed task, and it must not inherit
    # another deployment's server URL or token either.
    $environment = @{ PATH = $path }
    foreach ($name in @(
        'MULTICA_AGENT_ID', 'U_WORKERS_AGENT_ID', 'MULTICA_TASK_ID', 'U_WORKERS_TASK_ID',
        'MULTICA_DAEMON_PORT', 'U_WORKERS_DAEMON_PORT', 'MULTICA_TASK_CONFIG_ROOT', 'U_WORKERS_TASK_CONFIG_ROOT',
        'MULTICA_TOKEN', 'U_WORKERS_TOKEN', 'MULTICA_SERVER_URL', 'U_WORKERS_SERVER_URL',
        'MULTICA_APP_URL', 'U_WORKERS_APP_URL', 'MULTICA_DAEMON_PROFILE', 'U_WORKERS_DAEMON_PROFILE'
    )) {
        $environment[$name] = ''
    }
    return $environment
}

function Invoke-PortableCli {
    # The CLI is a Go program, so its output is always UTF-8 regardless of the
    # console code page; ask the capture to decode it that way.
    param([string]$Root, [string[]]$Arguments)
    $cli = Get-PortableCliPath $Root
    $result = Invoke-PortableCommand -Executable $cli -Arguments $Arguments -Environment (Get-PortableCliEnvironment $Root) -WorkingDirectory $Root -Utf8Output
    return $result
}

function Write-PortableInstallReport {
    param([string]$Root, $Report)
    $path = Join-Path $Root 'data\install-report.json'
    New-PortableDirectory (Split-Path -Parent $path)
    Write-PortableJson $path $Report
    return $path
}

function ConvertFrom-PortableDaemonStatus {
    # The verdict comes from the reported status field, never the exit code:
    # `daemon status` exits 0 for a stopped daemon too (it is a query, not a
    # check), so treating a zero exit as "running" printed a healthy line for a
    # daemon that was not there and made start.ps1 skip the start that would
    # have brought it up. "starting" counts as alive: the process exists and the
    # CLI itself refuses a second start, so starting again would only fail.
    param([int]$ExitCode, [string]$Output)
    $text = ([string]$Output).Trim()
    if ($ExitCode -ne 0) {
        return [pscustomobject]@{ Running = $false; Output = $text }
    }
    $parsed = $null
    try { $parsed = ($text | ConvertFrom-Json) } catch { $parsed = $null }
    $alive = [bool]($parsed -and @('running', 'starting') -contains [string]$parsed.status)
    return [pscustomobject]@{ Running = $alive; Detail = $parsed; Output = $text }
}

function Get-PortableDaemonStatus {
    param([string]$Root, [string]$Profile)
    $arguments = @()
    if ($Profile) { $arguments += @('--profile', $Profile) }
    $arguments += @('daemon', 'status', '--output', 'json')
    $result = Invoke-PortableCli -Root $Root -Arguments $arguments
    return (ConvertFrom-PortableDaemonStatus -ExitCode ([int]$result.ExitCode) -Output $result.Output)
}

function Start-PortableDaemon {
    param([string]$Root, [string]$Profile)
    $status = Get-PortableDaemonStatus -Root $Root -Profile $Profile
    if ($status.Running) { return $status }
    $arguments = @()
    if ($Profile) { $arguments += @('--profile', $Profile) }
    $arguments += @('daemon', 'start')
    $result = Invoke-PortableCli -Root $Root -Arguments $arguments
    if ($result.ExitCode -ne 0) {
        throw ('Starting the local daemon failed: ' + ([string]$result.Output).Trim())
    }
    return (Get-PortableDaemonStatus -Root $Root -Profile $Profile)
}

function Stop-PortableDaemon {
    param([string]$Root, [string]$Profile)
    $arguments = @()
    if ($Profile) { $arguments += @('--profile', $Profile) }
    $arguments += @('daemon', 'stop')
    $result = Invoke-PortableCli -Root $Root -Arguments $arguments
    return $result
}

function Get-PortableLanHostFromConfig {
    param([string]$Root)
    $report = Read-PortableJson (Join-Path $Root 'data\install-report.json')
    if ($report -and $report.lanHost) { return [string]$report.lanHost }
    $config = Get-PortableDeployment $Root
    if ($config.PublicOrigin) {
        try { return ([Uri][string]$config.PublicOrigin).Host } catch { }
    }
    return '127.0.0.1'
}

function Get-PortableHttpsPortFromConfig {
    param([string]$Root)
    $report = Read-PortableJson (Join-Path $Root 'data\install-report.json')
    if ($report -and $report.httpsPort) { return [int]$report.httpsPort }
    $config = Get-PortableDeployment $Root
    if ($config.PublicOrigin) {
        try { return ([Uri][string]$config.PublicOrigin).Port } catch { }
    }
    return 13443
}

function Get-PortablePostgresMajorVersion {
    # The bundled PostgreSQL distribution has no PG_VERSION file (that file lives
    # in a data directory), so ask the binary itself. Adoption compares this
    # major version with the old cluster's PG_VERSION before copying anything.
    param([string]$Root)
    $bin = Join-Path $Root 'payload\runtime\pgsql\bin'
    if (-not (Test-Path -LiteralPath (Join-Path $bin 'postgres.exe'))) {
        $bin = Join-Path $Root 'runtime\pgsql\bin'
    }
    $exe = Join-Path $bin 'postgres.exe'
    if (-not (Test-Path -LiteralPath $exe)) { throw ('The package has no bundled PostgreSQL binary: ' + $exe) }
    $result = Invoke-PortableCommand -Executable $exe -Arguments @('--version')
    if ($result.ExitCode -ne 0) { throw ('Could not determine the bundled PostgreSQL version: ' + ([string]$result.Output).Trim()) }
    $match = [regex]::Match([string]$result.Output, '(?i)PostgreSQL\)?\s+(\d+)')
    if (-not $match.Success) { throw ('Could not parse the bundled PostgreSQL version: ' + ([string]$result.Output).Trim()) }
    return $match.Groups[1].Value
}

function Assert-PortableProfileName {
    # One plain path segment: dots are allowed inside a name (uworkers-acceptance),
    # but "." and ".." and any leading dot would escape the profiles directory.
    param([string]$Profile)
    if (-not $Profile) { throw 'The CLI profile name is empty; refusing to touch any profile directory.' }
    if ($Profile -ne $Profile.Trim()) { throw ('Refusing a CLI profile name with surrounding whitespace: ' + $Profile) }
    if ($Profile -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]*$') {
        throw ('Refusing an unexpected CLI profile name: ' + $Profile + '. The portable package only manages its own profile (letters, digits, dot, dash, underscore).')
    }
    return $Profile
}

function Get-PortableOwnedProfileDirectories {
    # The uninstaller may only delete profile directories this package marked as
    # its own in the CLI's actual config root. The other root is never scanned by
    # default, so another product's same-named profile cannot be claimed by
    # matching a server URL.
    param([string]$Profile, [string]$PackageRoot, [string[]]$ConfigRoots)
    Assert-PortableProfileName $Profile | Out-Null
    if (-not $ConfigRoots) { $ConfigRoots = @(Get-PortableProfileConfigRoots) }
    if (@($ConfigRoots).Count -eq 0) { return @() }
    $activeRoot = [IO.Path]::GetFullPath([string]$ConfigRoots[0]).TrimEnd('\')
    $profilesParent = [IO.Path]::GetFullPath((Join-Path $activeRoot 'profiles')).TrimEnd('\')
    $candidate = [IO.Path]::GetFullPath((Join-Path $profilesParent $Profile))
    $parent = [IO.Path]::GetFullPath((Split-Path -Parent $candidate)).TrimEnd('\')
    if (-not [string]::Equals($parent, $profilesParent, [StringComparison]::OrdinalIgnoreCase)) { return @() }
    if (-not (Test-Path -LiteralPath $candidate)) { return @() }
    $marker = Read-PortableJson (Join-Path $candidate 'uworkers-portable-profile.json')
    if (-not $marker) { return @() }
    $package = [IO.Path]::GetFullPath($PackageRoot).TrimEnd('\')
    if (-not [string]::Equals([string]$marker.packageRoot, $package, [StringComparison]::OrdinalIgnoreCase)) { return @() }
    if (-not [string]::Equals([string]$marker.profile, $Profile, [StringComparison]::OrdinalIgnoreCase)) { return @() }
    # A marker from the earlier version has no configRoot; accept it only in the
    # active root (it is where the CLI actually wrote this profile). New markers
    # must name this exact root.
    if ($marker.PSObject.Properties['configRoot']) {
        $recordedRoot = [IO.Path]::GetFullPath([string]$marker.configRoot).TrimEnd('\')
        if (-not [string]::Equals($recordedRoot, $activeRoot, [StringComparison]::OrdinalIgnoreCase)) { return @() }
    }
    return @($candidate)
}

function Register-PortableProfileOwnership {
    # Record which profile directory belongs to this package so the uninstaller
    # can prove ownership. Only the CLI's actual config root is marked: passing a
    # root for the other config tree is not evidence that this installation used
    # it. Returns the marked directory (or nothing).
    param([string]$Profile, [string]$PackageRoot, [string[]]$ConfigRoots, [string]$ServerUrl)
    Assert-PortableProfileName $Profile | Out-Null
    if (-not $ConfigRoots) { $ConfigRoots = @(Get-PortableProfileConfigRoots) }
    if (@($ConfigRoots).Count -eq 0) { return @() }
    $activeRoot = [IO.Path]::GetFullPath([string]$ConfigRoots[0]).TrimEnd('\')
    $candidate = Join-Path (Join-Path $activeRoot 'profiles') $Profile
    if (-not (Test-Path -LiteralPath $candidate)) { return @() }
    if ($ServerUrl) {
        $config = Read-PortableJson (Join-Path $candidate 'config.json')
        $configured = ''
        if ($config -and $config.server_url) { $configured = [string]$config.server_url }
        if (-not $configured) { return @() }
        if (-not [string]::Equals($configured.TrimEnd('/'), $ServerUrl.TrimEnd('/'), [StringComparison]::OrdinalIgnoreCase)) { return @() }
    }
    $marker = [ordered]@{
        profile = $Profile
        packageRoot = [IO.Path]::GetFullPath($PackageRoot).TrimEnd('\')
        configRoot = $activeRoot
        registeredAt = (Get-Date).ToUniversalTime().ToString('o')
    }
    Write-PortableJson (Join-Path $candidate 'uworkers-portable-profile.json') $marker
    return @($candidate)
}

function Get-PortableCliConfigRoot {
    # Mirrors server/internal/brand.ConfigRoot for the current user: a
    # ~/.u-workers directory wins once it holds config.json or profiles, an
    # existing legacy ~/.multica keeps working, and a new machine starts in
    # ~/.u-workers. This is the single root the CLI actually uses.
    $homeDirectory = [Environment]::GetFolderPath('UserProfile')
    $current = Join-Path $homeDirectory '.u-workers'
    if ((Test-Path -LiteralPath (Join-Path $current 'config.json')) -or (Test-Path -LiteralPath (Join-Path $current 'profiles'))) {
        return $current
    }
    $legacy = Join-Path $homeDirectory '.multica'
    if (Test-Path -LiteralPath $legacy) { return $legacy }
    return $current
}

function Get-PortableProfileConfigRoots {
    # The CLI's actual config root comes first. Other existing roots follow for
    # reporting only: ownership registration and removal use only the first
    # entry, so a same-named profile in the other root is never claimed.
    $active = [IO.Path]::GetFullPath((Get-PortableCliConfigRoot)).TrimEnd('\')
    $roots = @($active)
    $homeDirectory = [Environment]::GetFolderPath('UserProfile')
    foreach ($name in @('.u-workers', '.multica')) {
        $path = Join-Path $homeDirectory $name
        if (-not (Test-Path -LiteralPath $path)) { continue }
        $normalized = [IO.Path]::GetFullPath($path).TrimEnd('\')
        if ([string]::Equals($normalized, $active, [StringComparison]::OrdinalIgnoreCase)) { continue }
        $roots += $normalized
    }
    return $roots
}

function Get-PortableReparsePoints {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { return @() }
    return @(Get-ChildItem -LiteralPath $Path -Recurse -Force -ErrorAction SilentlyContinue | Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 })
}

function Repair-PortableWebRelease {
    # Next.js standalone output contains a nearly complete pnpm store inside the
    # folder, but a few dozen package directories are junctions into the build
    # checkout. Store-internal junctions are expanded to real copies here; the
    # app-level ones cannot be expanded without breaking dependency resolution,
    # so they are recorded and recreated as package-local junctions by Setup.
    param([string]$WebPath, [string]$CheckoutNodeModules)
    $WebPath = [IO.Path]::GetFullPath($WebPath).TrimEnd('\')
    if (-not (Test-Path -LiteralPath $WebPath)) { throw ('Web release folder missing: ' + $WebPath) }
    if (-not (Test-Path -LiteralPath (Join-Path $WebPath 'node_modules\.pnpm'))) { throw ('The standalone web folder has no pnpm store: ' + $WebPath) }
    $storePrefix = [IO.Path]::GetFullPath($CheckoutNodeModules).TrimEnd('\') + '\.pnpm\'
    $links = @(Get-PortableReparsePoints -Path $WebPath)
    $manifest = New-Object System.Collections.Generic.List[object]
    $expanded = 0
    $recorded = 0
    foreach ($link in $links) {
        $linkPath = $link.FullName
        $relative = $linkPath.Substring($WebPath.Length).TrimStart('\')
        $targets = @($link.Target)
        if ($targets.Count -eq 0 -or -not $targets[0]) { throw ('Cannot resolve link target: ' + $linkPath) }
        $target = [IO.Path]::GetFullPath([string]$targets[0])
        if (-not $target.StartsWith($storePrefix, [StringComparison]::OrdinalIgnoreCase)) {
            throw ('Unexpected web dependency link outside the pnpm store: ' + $linkPath + ' -> ' + $target)
        }
        $storeRelative = $target.Substring($storePrefix.Length)
        $isStoreInternal = $relative.Contains('\.pnpm\')
        $isDirectory = (($link.Attributes -band [IO.FileAttributes]::Directory) -ne 0)
        if ($isStoreInternal) {
            if ($isDirectory) {
                [IO.Directory]::Delete($linkPath, $false)
                New-Item -ItemType Directory -Path $linkPath -Force | Out-Null
                $copy = Invoke-PortableCommand -Executable 'robocopy.exe' -Arguments @($target, $linkPath, '/E', '/R:1', '/W:1', '/MT:4', '/NP', '/NFL', '/NDL', '/NJH', '/NJS')
                if ($copy.ExitCode -ge 8) { throw ('Expanding a web dependency failed (robocopy exit ' + $copy.ExitCode + '): ' + $linkPath) }
                $expanded++
            } else {
                Copy-Item -LiteralPath $target -Destination $linkPath -Force
                $expanded++
            }
        } else {
            $manifest.Add([ordered]@{ path = $relative; target = ('node_modules\.pnpm\' + $storeRelative) }) | Out-Null
            if ($isDirectory) { [IO.Directory]::Delete($linkPath, $false) } else { [IO.File]::Delete($linkPath) }
            $recorded++
        }
    }
    # Windows still has a 260-character path limit for most tools. The pnpm
    # store directory names (package@version_peer-hash) are the longest path
    # components; renaming them to short stable hashes keeps the installed web
    # inside the limit for normal install folders.
    $storeRoot = Join-Path $WebPath 'node_modules\.pnpm'
    $renames = @{}
    foreach ($storeDir in @(Get-ChildItem -LiteralPath $storeRoot -Directory -Force -ErrorAction SilentlyContinue)) {
        if ($storeDir.Name.Length -le 16) { continue }
        $hasher = [Security.Cryptography.SHA256]::Create()
        try { $hashBytes = $hasher.ComputeHash([Text.Encoding]::UTF8.GetBytes($storeDir.Name)) } finally { $hasher.Dispose() }
        $shortName = 'p' + ((($hashBytes | ForEach-Object { $_.ToString('x2') }) -join '').Substring(0, 10))
        $shortPath = Join-Path $storeRoot $shortName
        $suffix = 0
        while (Test-Path -LiteralPath $shortPath) {
            $suffix++
            $shortPath = Join-Path $storeRoot ($shortName + '-' + [string]$suffix)
        }
        Rename-Item -LiteralPath $storeDir.FullName -NewName (Split-Path -Leaf $shortPath)
        $renames[[string]$storeDir.Name] = (Split-Path -Leaf $shortPath)
    }
    foreach ($entry in $manifest) {
        foreach ($oldName in @($renames.Keys)) {
            $prefix = 'node_modules\.pnpm\' + $oldName + '\'
            if (([string]$entry.target).StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
                $entry.target = 'node_modules\.pnpm\' + $renames[$oldName] + '\' + ([string]$entry.target).Substring($prefix.Length)
                break
            }
        }
    }

    $manifestPath = Join-Path $WebPath 'portable-links.json'
    $linkEntries = @($manifest | ForEach-Object { [pscustomobject]@{ path = [string]$_.path; target = [string]$_.target } })
    $manifestObject = [pscustomobject]@{ links = $linkEntries }
    $manifestJson = $manifestObject | ConvertTo-Json -Depth 5
    [IO.File]::WriteAllText($manifestPath, $manifestJson, (New-Object Text.UTF8Encoding($false)))
    $remaining = @(Get-PortableReparsePoints -Path $WebPath)
    if ($remaining.Count -gt 0) { throw ('The web release still contains ' + $remaining.Count + ' links that would point outside the package. First: ' + $remaining[0].FullName) }
    return [pscustomobject]@{ Expanded = $expanded; Recorded = $recorded; Manifest = $manifestPath }
}

function Restore-PortableWebLinks {
    # Recreates the app-level dependency junctions with the installed package's
    # absolute paths. Creating junctions does not need administrator rights, so
    # the installed folder no longer references the build machine.
    param([string]$WebPath)
    $manifestPath = Join-Path $WebPath 'portable-links.json'
    if (-not (Test-Path -LiteralPath $manifestPath)) { return 0 }
    $manifest = Read-PortableJson -Path $manifestPath
    if (-not $manifest -or -not $manifest.links) { return 0 }
    $created = 0
    foreach ($entry in @($manifest.links)) {
        if (-not $entry) { continue }
        $linkPath = [IO.Path]::GetFullPath((Join-Path $WebPath ([string]$entry.path)))
        $targetPath = [IO.Path]::GetFullPath((Join-Path $WebPath ([string]$entry.target)))
        if (-not (Test-Path -LiteralPath $targetPath)) { throw ('Web dependency link target is missing from the release: ' + $targetPath) }
        if (Test-Path -LiteralPath $linkPath) {
            $existing = Get-Item -LiteralPath $linkPath -Force
            if ((($existing.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)) {
                $existingTarget = @($existing.Target)
                if ($existingTarget.Count -gt 0 -and $existingTarget[0] -and ([IO.Path]::GetFullPath([string]$existingTarget[0]) -eq $targetPath)) { continue }
                if (($existing.Attributes -band [IO.FileAttributes]::Directory) -ne 0) { [IO.Directory]::Delete($linkPath, $false) } else { [IO.File]::Delete($linkPath) }
            } else {
                throw ('Refusing to replace a real file with a dependency link: ' + $linkPath)
            }
        }
        New-Item -ItemType Junction -Path $linkPath -Target $targetPath | Out-Null
        $created++
    }
    return $created
}

function Get-PortableExternalWebLinks {
    param([string]$WebPath)
    $WebPath = [IO.Path]::GetFullPath($WebPath).TrimEnd('\')
    $prefix = $WebPath + '\'
    $links = @(Get-PortableReparsePoints -Path $WebPath)
    $outside = @()
    foreach ($link in $links) {
        $targets = @($link.Target)
        if ($targets.Count -eq 0 -or -not $targets[0]) { $outside += $link.FullName; continue }
        if (-not ([IO.Path]::GetFullPath([string]$targets[0]).StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase))) { $outside += $link.FullName }
    }
    return [pscustomobject]@{ Total = $links.Count; Outside = $outside.Count; OutsidePaths = $outside }
}

