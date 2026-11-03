@echo off
setlocal
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0client.ps1" -Action Status
set "result=%ERRORLEVEL%"
pause
exit /b %result%
