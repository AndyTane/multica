@echo off
setlocal
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0client.ps1" -Action Connect
set "result=%ERRORLEVEL%"
pause
exit /b %result%
