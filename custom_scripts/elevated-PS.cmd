@echo off
setlocal ENABLEEXTENSIONS

rem --- Configure your working folder ---
set "WD=C:\launcher"

rem --- 64-bit Windows PowerShell path ---
set "PS=%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe"

rem --- Check if this CMD is elevated (admin token) ---
net session >nul 2>&1
if %errorlevel% NEQ 0 (
  rem Not elevated: relaunch THIS script elevated via UAC, then exit
  "%PS%" -NoProfile -Command ^
    "Start-Process -Verb RunAs -FilePath '%ComSpec%' -ArgumentList '/c','\"%~f0\"' "
  exit /b
)

rem --- Now elevated: open an elevated PowerShell that STAYS OPEN ---
pushd "%WD%"
"%PS%" -NoLogo -NoProfile -ExecutionPolicy Bypass -NoExit -Command ^
  "Set-Location -LiteralPath '%WD%';" ^
  "Write-Host 'Elevated PowerShell opened in %WD%' -ForegroundColor Green"
popd

endlocal
