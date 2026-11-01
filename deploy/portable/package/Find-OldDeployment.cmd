@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0find-old-deployment.ps1"
echo.
pause
