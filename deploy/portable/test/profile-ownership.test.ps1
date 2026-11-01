# R1-2 regression tests: ownership must come from the CLI's actual config root
# and the uninstaller must delete only this package's own profile, in both
# keep-data and PurgeData modes. Windows PowerShell 5.1 compatible.
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

function New-UninstallFixture {
    param([string]$Fixture, [string[]]$SelectedRoots, [string]$ForeignRoot, [string]$BoardUrl)
    New-TestFile (Join-Path $Fixture 'deployment.json') '{"Version":"v0.2.0"}'
    New-TestFile (Join-Path $Fixture 'data\install-report.json') ('{"profile":"portable","version":"v0.2.0"}')
    New-TestFile (Join-Path $Fixture 'data\package-sentinel.txt') 'package data'
    New-TestFile (Join-Path $Fixture 'uploads\upload-sentinel.txt') 'package upload'
    Copy-Item -LiteralPath (Join-Path $portableRoot 'portable-common.ps1') -Destination $Fixture -Force
    # uninstall.ps1 dot-sources this copy, so isolate external effects there,
    # after the real definitions. ConfigRoots alone does not isolate Startup,
    # certificate stores or process control from the current Windows account.
    $isolatedEffects = @'

function Stop-PortableDaemon { param([string]$Root, [string]$Profile) return $false }
function Stop-PortableCaddy { param([string]$Root) return $false }
function Invoke-PortableNativeAction { param([string]$Root, [string]$Action) return '' }
function Remove-PortableBrowserTrust { param([string]$Root) return $false }
function Get-PortableStartupShortcutPath { return (Join-Path $PSScriptRoot 'startup-sentinel.vbs') }
'@
    [IO.File]::AppendAllText((Join-Path $Fixture 'portable-common.ps1'), $isolatedEffects)
    New-TestFile (Join-Path $Fixture 'startup-sentinel.vbs') 'test startup entry'
    Copy-Item -LiteralPath (Join-Path $portableRoot 'uninstall.ps1') -Destination $Fixture -Force
    $selected = $SelectedRoots[0]
    New-TestFile (Join-Path $selected 'profiles\portable\config.json') ('{"server_url":"' + $BoardUrl + '"}')
    New-TestFile (Join-Path $selected 'profiles\other-tool\config.json') '{"server_url":"https://other.invalid"}'
    New-TestFile (Join-Path $selected 'profiles\other-tool\sentinel.txt') 'other profile'
    New-TestFile (Join-Path $foreignRoot 'config.json') '{"server_url":"https://127.0.0.1:13443"}'
    New-TestFile (Join-Path $foreignRoot 'profiles\portable\config.json') ('{"server_url":"' + $BoardUrl + '"}')
    New-TestFile (Join-Path $foreignRoot 'profiles\portable\sentinel.txt') 'foreign profile'
    $marked = @(Register-PortableProfileOwnership -Profile 'portable' -PackageRoot $Fixture -ConfigRoots $SelectedRoots -ServerUrl $BoardUrl)
    return $marked
}

$sandbox = Join-Path ([IO.Path]::GetTempPath()) ('uworkers-profile-test-' + [guid]::NewGuid().ToString('N'))
try {
    $boardUrl = 'https://127.0.0.1:23443'
    $selectedRoot = Join-Path $sandbox 'config-selected'
    $foreignRoot = Join-Path $sandbox 'config-foreign'
    $packageA = Join-Path $sandbox 'package-a'
    $packageB = Join-Path $sandbox 'package-b'
    New-Item -ItemType Directory -Path $packageA, $packageB -Force | Out-Null

    # --- two roots with the same profile name and the same server URL -----
    New-TestFile (Join-Path $selectedRoot 'profiles\portable\config.json') ('{"server_url":"' + $boardUrl + '"}')
    New-TestFile (Join-Path $foreignRoot 'profiles\portable\config.json') ('{"server_url":"' + $boardUrl + '"}')
    $registered = @(Register-PortableProfileOwnership -Profile 'portable' -PackageRoot $packageA -ConfigRoots @($selectedRoot, $foreignRoot) -ServerUrl $boardUrl)
    Assert-Test ($registered.Count -eq 1) ('Registration must claim only the CLI selected root even when the other root has the same URL, got: ' + ($registered -join '; '))
    Assert-Test (Test-Path -LiteralPath (Join-Path $selectedRoot 'profiles\portable\uworkers-portable-profile.json')) 'The selected root profile must be marked.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $foreignRoot 'profiles\portable\uworkers-portable-profile.json'))) 'The other config root with the same URL must not be claimed.'
    $removable = @(Get-PortableOwnedProfileDirectories -Profile 'portable' -PackageRoot $packageA -ConfigRoots @($selectedRoot, $foreignRoot))
    Assert-Test ($removable.Count -eq 1) ('Only the selected root profile may be removable, got: ' + ($removable -join '; '))
    $marker = Read-PortableJson (Join-Path $selectedRoot 'profiles\portable\uworkers-portable-profile.json')
    Assert-Test ($marker.configRoot -ieq [IO.Path]::GetFullPath($selectedRoot).TrimEnd('\')) 'The ownership marker must record the CLI config root.'

    # A marker naming the other root is not honoured.
    Write-PortableJson (Join-Path $foreignRoot 'profiles\portable\uworkers-portable-profile.json') ([ordered]@{ profile = 'portable'; packageRoot = [IO.Path]::GetFullPath($packageA).TrimEnd('\'); configRoot = [IO.Path]::GetFullPath($foreignRoot).TrimEnd('\'); registeredAt = 'now' })
    $wrongRoot = @(Get-PortableOwnedProfileDirectories -Profile 'portable' -PackageRoot $packageA -ConfigRoots @($selectedRoot, $foreignRoot))
    Assert-Test ($wrongRoot.Count -eq 1) 'A marker for the non-selected root must not be removable.'

    # --- escape attempts are still rejected before any lookup or delete --
    foreach ($bad in @('.', '..', '.hidden', 'a b', 'a\b', 'a/b', '')) {
        Assert-Throws { Assert-PortableProfileName $bad } '' ('Profile name "' + $bad + '" must be rejected')
    }

    # --- real uninstall control flow: data kept ---------------------------
    $fixtureKeep = Join-Path $sandbox 'uninstall-keep'
    $selectedKeep = Join-Path $sandbox 'root-keep-selected'
    $foreignKeep = Join-Path $sandbox 'root-keep-foreign'
    $markedKeep = @(New-UninstallFixture -Fixture $fixtureKeep -SelectedRoots @($selectedKeep) -ForeignRoot $foreignKeep -BoardUrl $boardUrl)
    Assert-Test ($markedKeep.Count -eq 1) 'The keep-data fixture must register exactly one owned profile.'
    & (Join-Path $fixtureKeep 'uninstall.ps1') -Force -ConfigRoots @($selectedKeep, $foreignKeep) | Out-Null
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $fixtureKeep 'startup-sentinel.vbs'))) 'The real startup removal must target only the fixture startup entry.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $selectedKeep 'profiles\portable'))) 'The owned profile must be removed.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $selectedKeep 'profiles\other-tool\sentinel.txt')) 'Other profiles in the selected root must remain.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $foreignKeep 'profiles\portable\sentinel.txt')) 'The same-named profile in the other root must remain.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $foreignKeep 'config.json')) 'The other root default configuration must remain.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $fixtureKeep 'data\package-sentinel.txt')) 'Keeping data must keep the package data.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $fixtureKeep 'deployment.json')) 'Keeping data must keep the deployment record.'

    # --- real uninstall control flow: PurgeData ---------------------------
    $fixturePurge = Join-Path $sandbox 'uninstall-purge'
    $selectedPurge = Join-Path $sandbox 'root-purge-selected'
    $foreignPurge = Join-Path $sandbox 'root-purge-foreign'
    $outsideSentinel = Join-Path $sandbox 'outside-sentinel.txt'
    New-TestFile $outsideSentinel 'outside'
    $markedPurge = @(New-UninstallFixture -Fixture $fixturePurge -SelectedRoots @($selectedPurge) -ForeignRoot $foreignPurge -BoardUrl $boardUrl)
    Assert-Test ($markedPurge.Count -eq 1) 'The purge fixture must register exactly one owned profile.'
    & (Join-Path $fixturePurge 'uninstall.ps1') -Force -PurgeData -ConfigRoots @($selectedPurge, $foreignPurge) | Out-Null
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $fixturePurge 'startup-sentinel.vbs'))) 'PurgeData must also target only the fixture startup entry.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $selectedPurge 'profiles\portable'))) 'PurgeData must still remove only the owned profile.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $selectedPurge 'profiles\other-tool\sentinel.txt')) 'PurgeData must not remove other profiles.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $foreignPurge 'profiles\portable\sentinel.txt')) 'PurgeData must not remove the other root profile.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $foreignPurge 'config.json')) 'PurgeData must not remove the other root default configuration.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $fixturePurge 'data'))) 'PurgeData must delete the package data.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $fixturePurge 'uploads'))) 'PurgeData must delete the package uploads.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $fixturePurge 'deployment.json'))) 'PurgeData must delete the deployment record.'
    Assert-Test (Test-Path -LiteralPath $outsideSentinel) 'PurgeData must not touch files outside the package.'

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
