# R1-2 regression tests: the uninstaller must validate the profile name before
# any side effect and delete only profile directories this package owns.
# Windows PowerShell 5.1 compatible.
# Run: powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy\portable\test\profile-ownership.test.ps1
$ErrorActionPreference = 'Stop'
$portableRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $portableRoot 'portable-common.ps1')

function Assert-Test {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw ('FAILED: ' + $Message) }
}

function Assert-Throws {
    param([scriptblock]$Action, [string]$Pattern, [string]$Message)
    $threw = $false
    try { & $Action | Out-Null } catch { $threw = $true; if ($Pattern -and $_.Exception.Message -notmatch $Pattern) { throw ('FAILED: ' + $Message + ' (unexpected message: ' + $_.Exception.Message + ')') } }
    if (-not $threw) { throw ('FAILED: ' + $Message + ' (nothing was rejected)') }
}

function New-TestFile {
    param([string]$Path, [string]$Content = 'x')
    $directory = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory -Force | Out-Null }
    [IO.File]::WriteAllText($Path, $Content)
}

$sandbox = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-profile-test-' + [guid]::NewGuid().ToString('N'))
try {
    $packageA = Join-Path $sandbox 'package-a'
    $packageB = Join-Path $sandbox 'package-b'
    $rootA = Join-Path $sandbox 'config-a'
    $rootB = Join-Path $sandbox 'config-b'
    $boardUrl = 'https://127.0.0.1:23443'
    New-Item -ItemType Directory -Path $packageA, $packageB -Force | Out-Null

    # rootA holds this deployment's profile; rootB holds another product's
    # identically named profile (its saved server URL points elsewhere) plus an
    # unrelated profile name.
    $ownedDirectory = Join-Path $rootA 'profiles\portable'
    $foreignDirectory = Join-Path $rootB 'profiles\portable'
    $otherDirectory = Join-Path $rootB 'profiles\other-tool'
    New-TestFile (Join-Path $ownedDirectory 'config.json') ('{"server_url":"' + $boardUrl + '"}')
    New-TestFile (Join-Path $foreignDirectory 'config.json') '{"server_url":"https://example.invalid"}'
    New-TestFile (Join-Path $otherDirectory 'config.json') '{}'

    # --- registration only claims the profile that points at this board -----
    $registered = @(Register-PortableProfileOwnership -Profile 'portable' -PackageRoot $packageA -ConfigRoots @($rootA, $rootB) -ServerUrl $boardUrl)
    Assert-Test ($registered.Count -eq 1) ('Registration must mark only the profile saved for this board, got: ' + ($registered -join '; '))
    Assert-Test (Test-Path -LiteralPath (Join-Path $ownedDirectory 'uworkers-portable-profile.json')) 'The owned profile must get an ownership marker.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $foreignDirectory 'uworkers-portable-profile.json'))) 'A same-named profile saved for another server must not be claimed.'
    $marker = Read-PortableJson (Join-Path $ownedDirectory 'uworkers-portable-profile.json')
    Assert-Test ($marker.profile -eq 'portable') 'The marker must record the profile name.'
    Assert-Test ([string]::Equals([string]$marker.packageRoot, [IO.Path]::GetFullPath($packageA).TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)) 'The marker must record the package folder.'
    $noneMarked = @(Register-PortableProfileOwnership -Profile 'portable' -PackageRoot $packageA -ConfigRoots @($rootA) -ServerUrl 'https://127.0.0.1:1')
    Assert-Test ($noneMarked.Count -eq 0) 'A profile saved for a different server must not be marked.'

    # --- escape attempts are rejected before any lookup or delete --------
    foreach ($bad in @('.', '..', '.hidden', 'a b', 'a\b', 'a/b', '')) {
        Assert-Throws { Assert-PortableProfileName $bad } '' ('Profile name "' + $bad + '" must be rejected')
    }
    Assert-Test (Test-Path -LiteralPath $ownedDirectory) 'A rejected profile name must not delete the owned profile.'
    Assert-Test (Test-Path -LiteralPath $foreignDirectory) 'A rejected profile name must not delete another product profile.'
    Assert-Test (Test-Path -LiteralPath $otherDirectory) 'A rejected profile name must not delete unrelated profiles.'

    # --- ownership selects only the marked directory ---------------------
    $owned = @(Get-PortableOwnedProfileDirectories -Profile 'portable' -PackageRoot $packageA -ConfigRoots @($rootA, $rootB))
    Assert-Test ($owned.Count -eq 1) ('Only the marked profile is owned, got: ' + ($owned -join '; '))
    Assert-Test ([string]::Equals($owned[0], $ownedDirectory, [StringComparison]::OrdinalIgnoreCase)) 'The owned profile must be the one this package registered.'

    # A marker written by a different package folder does not transfer ownership.
    Write-PortableJson (Join-Path $foreignDirectory 'uworkers-portable-profile.json') ([ordered]@{ profile = 'portable'; packageRoot = [IO.Path]::GetFullPath($packageB).TrimEnd('\'); registeredAt = 'now' })
    $ownedByPackageA = @(Get-PortableOwnedProfileDirectories -Profile 'portable' -PackageRoot $packageA -ConfigRoots @($rootB))
    Assert-Test ($ownedByPackageA.Count -eq 0) 'A marker from another package folder must not make a profile removable.'
    $ownedByPackageB = @(Get-PortableOwnedProfileDirectories -Profile 'portable' -PackageRoot $packageB -ConfigRoots @($rootB))
    Assert-Test ($ownedByPackageB.Count -eq 1) 'The profile must be removable by the package that registered it.'

    # --- uninstall deletes only owned profiles ---------------------------
    foreach ($directory in $owned) { if (Test-Path -LiteralPath $directory) { Remove-Item -LiteralPath $directory -Recurse -Force } }
    Assert-Test (-not (Test-Path -LiteralPath $ownedDirectory)) 'The owned profile directory must be removed.'
    Assert-Test (Test-Path -LiteralPath $foreignDirectory) 'Another product profile with the same name must survive.'
    Assert-Test (Test-Path -LiteralPath $otherDirectory) 'Unrelated profiles must survive.'

    # --- the real uninstall script refuses before any stop ---------------
    $fixture = Join-Path $sandbox 'uninstall-fixture'
    New-TestFile (Join-Path $fixture 'deployment.json') '{}'
    New-TestFile (Join-Path $fixture 'data\install-report.json') ('{"profile":"..","version":"v0.2.0"}')
    Copy-Item -LiteralPath (Join-Path $portableRoot 'portable-common.ps1') -Destination $fixture -Force
    Copy-Item -LiteralPath (Join-Path $portableRoot 'uninstall.ps1') -Destination $fixture -Force
    Assert-Throws { & (Join-Path $fixture 'uninstall.ps1') -Force } 'unexpected CLI profile|Refusing' 'uninstall.ps1 must reject ".." from the install report before stopping anything'
    Assert-Test (Test-Path -LiteralPath (Join-Path $fixture 'deployment.json')) 'A rejected uninstall must not delete the deployment.'

    # --- validation happens before the first stop in the script source ---
    $uninstallText = [IO.File]::ReadAllText((Join-Path $portableRoot 'uninstall.ps1'))
    $validationIndex = $uninstallText.IndexOf('Assert-PortableProfileName $Profile')
    $stopIndex = $uninstallText.IndexOf('Stop-PortableDaemon')
    Assert-Test ($validationIndex -ge 0) 'uninstall.ps1 must validate the profile name.'
    Assert-Test ($stopIndex -gt $validationIndex) 'uninstall.ps1 must validate the profile before stopping the daemon.'

    Write-Host 'profile ownership tests passed.'
} finally {
    Remove-Item -LiteralPath $sandbox -Recurse -Force -ErrorAction SilentlyContinue
}
