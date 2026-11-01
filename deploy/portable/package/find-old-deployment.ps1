# 查找本机上的 U Workers 原生部署（deployment.json），帮 Tane 确认操作单里的旧目录默认值。
$ErrorActionPreference = 'SilentlyContinue'
$defaultPath = 'D:\UWorkers\clawsta80\native'
$candidates = New-Object System.Collections.Generic.List[string]

function Add-IfDeployment {
    param([string]$Path)
    if (-not $Path) { return }
    $config = Join-Path $Path 'deployment.json'
    if ((Test-Path -LiteralPath $config) -and -not ($candidates -contains $Path)) { $candidates.Add($Path) }
}

foreach ($root in @('D:\UWorkers', 'C:\UWorkers')) {
    if (-not (Test-Path -LiteralPath $root)) { continue }
    Add-IfDeployment $root
    foreach ($child in @(Get-ChildItem -LiteralPath $root -Directory | Select-Object -First 60)) {
        Add-IfDeployment $child.FullName
        Add-IfDeployment (Join-Path $child.FullName 'native')
        foreach ($grand in @(Get-ChildItem -LiteralPath $child.FullName -Directory | Select-Object -First 30)) {
            Add-IfDeployment $grand.FullName
            Add-IfDeployment (Join-Path $grand.FullName 'native')
        }
    }
}
foreach ($local in @((Join-Path $env:LOCALAPPDATA 'Multica\native'), (Join-Path $env:LOCALAPPDATA 'U Workers\native'))) { Add-IfDeployment $local }

Write-Host ''
Write-Host 'U Workers 旧部署目录检查' -ForegroundColor Cyan
Write-Host ('默认旧目录：' + $defaultPath + ' -> ' + $(if (Test-Path -LiteralPath (Join-Path $defaultPath 'deployment.json')) { '存在' } else { '不存在' }))
Write-Host ''
if ($candidates.Count -eq 0) {
    Write-Host '没有找到任何原生部署（deployment.json）。' -ForegroundColor Yellow
    Write-Host '先别继续，把这个窗口截图发回来。'
} else {
    Write-Host '找到的旧部署目录：' -ForegroundColor Green
    foreach ($path in $candidates) {
        $config = Get-Content -LiteralPath (Join-Path $path 'deployment.json') -Raw -ErrorAction SilentlyContinue | ConvertFrom-Json
        $version = if ($config -and $config.Version) { [string]$config.Version } else { '?' }
        Write-Host ('  ' + $path + '   版本 ' + $version)
    }
    Write-Host ''
    if ($candidates[0] -eq $defaultPath) {
        Write-Host '第一条就是操作单里的默认目录，按操作单第 1 步继续即可。' -ForegroundColor Green
    } else {
        Write-Host ('操作单第 4 步的命令里，把 ' + $defaultPath + ' 换成上面的 ' + $candidates[0] + '。') -ForegroundColor Yellow
    }
}
Write-Host ''
