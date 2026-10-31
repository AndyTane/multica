@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Set-UWorkers-Login.ps1" -Root "%~dp0native"
exit /b %ERRORLEVEL%
