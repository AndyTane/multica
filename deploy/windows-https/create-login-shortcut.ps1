# Create the public-desktop shortcut for the pinned local login entry.
$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$target = Join-Path $root 'Set-UWorkers-Login.cmd'
if (-not (Test-Path -LiteralPath $target)) { throw ('entry point missing: ' + $target) }
$desktop = [Environment]::GetFolderPath('CommonDesktopDirectory')
$shortcutPath = Join-Path $desktop '设置 U Workers 登录.lnk'
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $target
$shortcut.WorkingDirectory = $root
$shortcut.Description = '为本人账号设置 U Workers 本机登录（不显示密码）'
$shortcut.IconLocation = 'C:\Windows\System32\shell32.dll,21'
$shortcut.Save()
$check = $shell.CreateShortcut($shortcutPath)
Write-Output ('shortcut=' + $shortcutPath + ' exists=' + (Test-Path -LiteralPath $shortcutPath) + ' target=' + $check.TargetPath)
Write-Output 'SHORTCUT_DONE'
