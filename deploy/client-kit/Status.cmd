@echo off
setlocal
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0client.ps1" -Action Status %*
set "result=%ERRORLEVEL%"
if not defined UWORKERS_NO_PAUSE pause
exit /b %result%
