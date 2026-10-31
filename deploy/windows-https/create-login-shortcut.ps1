# Create the desktop shortcut for the local login entry (run once on the host).
$ErrorActionPreference = 'Stop'
$base = 'D:\UWorkers\clawsta80'
$target = Join-Path $base 'Set-UWorkers-Login.cmd'
if (-not (Test-Path -LiteralPath $target)) { throw ('entry point missing: ' + $target) }
$desktop = [Environment]::GetFolderPath('CommonDesktopDirectory')
$shortcutPath = Join-Path $desktop '设置 U Workers 登录.lnk'
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $target
$shortcut.WorkingDirectory = $base
$shortcut.Description = '设置 U Workers 本机账号密码登录'
$shortcut.IconLocation = 'C:\Windows\System32\shell32.dll,21'
$shortcut.Save()
Write-Output ('shortcut=' + $shortcutPath + ' exists=' + (Test-Path -LiteralPath $shortcutPath))
Write-Output 'SHORTCUT_DONE'
