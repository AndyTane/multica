@echo off
setlocal
cd /d "%~dp0"
echo.
echo Build the second-machine installer - U Workers portable server
echo.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0build-installer.ps1" %*
set "EXITCODE=%ERRORLEVEL%"
if not "%EXITCODE%"=="0" (
  echo.
  echo Something failed. Details are in logs and datainstall-report.json.
  pause
  exit /b %EXITCODE%
)
pause
