@echo off
setlocal ENABLEDELAYEDEXPANSION
chcp 65001 >nul

set "WD=C:\launcher"
set "EXE=%WD%\launcher.exe"
set "LOG=%WD%\bootstrapper.log"
set "EXIT_FLAG=%WD%\exit-to-windows.flag"
set "WV2_EXE=C:\Program Files (x86)\Microsoft\EdgeWebView\Application\msedgewebview2.exe"
rem Optional offline installer (drop in this path if you have it)
set "WV2_INST=%WD%\MicrosoftEdgeWebView2RuntimeInstallerX64.exe"

if not exist "%WD%" (
  echo [%date% %time%] Working dir missing: %WD% >>"%LOG%"
  start "" explorer.exe
  exit /b 1
)

cd /d "%WD%"
del /q "%EXIT_FLAG%" >nul 2>&1

rem >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>
rem Ensure Bluetooth services/radio are up (silent, best-effort)
call :EnsureBT
rem <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<

echo.>>"%LOG%"
echo [%date% %time%] === Shell bootstrap start ===>>"%LOG%"

rem ---- Ensure WebView2 runtime present (Wails needs it) ----
if not exist "%WV2_EXE%" (
  echo [%date% %time%] WebView2 runtime not found.>>"%LOG%"
  if exist "%WV2_INST%" (
    echo [%date% %time%] Installing WebView2 silently...>>"%LOG%"
    "%WV2_INST%" /silent /install >>"%LOG%" 2>&1
    if not exist "%WV2_EXE%" (
      echo [%date% %time%] WebView2 still missing after installer.>>"%LOG%"
    ) else (
      echo [%date% %time%] WebView2 installed OK.>>"%LOG%"
    )
  ) else (
    echo [%date% %time%] Place the WebView2 installer at %WV2_INST% to auto-install.>>"%LOG%"
  )
)

set "FAILS=0"

:loop
if not exist "%EXE%" (
  echo [%date% %time%] Launcher not found at %EXE%. Starting Explorer...>>"%LOG%"
  start "" explorer.exe
  exit /b 2
)

echo [%date% %time%] Starting launcher...>>"%LOG%"

rem Time the run & capture exit code
del /q "%WD%\last_run_secs.txt" >nul 2>&1
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$sw=[Diagnostics.Stopwatch]::StartNew(); " ^
  "$p=Start-Process -FilePath '%EXE%' -WorkingDirectory '%WD%' -PassThru; " ^
  "$p.WaitForExit(); $sw.Stop(); " ^
  "Set-Content -Path '%WD%\last_run_secs.txt' -Value ([int]$sw.Elapsed.TotalSeconds); " ^
  "exit $p.ExitCode"
set "RC=%errorlevel%"

set "SECS="
for /f "usebackq delims=" %%S in ("%WD%\last_run_secs.txt") do set "SECS=%%S"
if "%SECS%"=="" set "SECS=0"

echo [%date% %time%] Launcher exited. rc=%RC% ran=%SECS%s >>"%LOG%"

if exist "%EXIT_FLAG%" (
  echo [%date% %time%] Exit-to-Windows requested. Starting Explorer and stopping bootstrapper.>>"%LOG%"
  del /q "%EXIT_FLAG%" >nul 2>&1
  start "" explorer.exe
  exit /b 0
)

rem If it ran at least 10s, reset the quick-fail counter
for /f "delims=" %%A in ('powershell -NoProfile -Command "[int](%SECS% -ge 10)"') do set OK=%%A
if "%OK%"=="1" set "FAILS=0" & goto loop

set /a FAILS+=1
if %FAILS% GEQ 3 (
  echo [%date% %time%] Too many quick failures. Launching Explorer and exiting.>>"%LOG%"
  start "" explorer.exe
  exit /b 3
)

echo [%date% %time%] Quick exit detected (%FAILS%). Retrying in 2s...>>"%LOG%"
timeout /t 2 >nul
goto loop


rem ========================== HELPERS ==========================
:EnsureBT
echo [%date% %time%] EnsureBT: begin >>"%LOG%"

rem Start core service: bthserv (no admin -> best-effort; ignore errors)
sc query bthserv | findstr /i "RUNNING" >nul 2>&1
if errorlevel 1 (
  sc start bthserv >>"%LOG%" 2>&1
) else (
  echo [%date% %time%] EnsureBT: bthserv already running >>"%LOG%"
)

rem Kick per-user BT service & turn radio ON via WinRT (hidden PowerShell)
powershell -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference='SilentlyContinue';" ^
  "$svc=Get-Service BthUserService* | Select-Object -First 1; " ^
  "if ($svc) { Start-Service $svc.Name | Out-Null }; " ^
  "Add-Type -AssemblyName System.Runtime.WindowsRuntime; " ^
  "$rad=[Windows.Devices.Radios.Radio]::GetRadiosAsync().GetAwaiter().GetResult(); " ^
  "$bt=$rad | Where-Object { $_.Kind -eq [Windows.Devices.Radios.RadioKind]::Bluetooth }; " ^
  "if ($bt) { $null=$bt[0].SetStateAsync([Windows.Devices.Radios.RadioState]::On).GetAwaiter().GetResult(); " ^
  "          Write-Output ('EnsureBT: WinRT radios={0} btState={1}' -f $rad.Count,$bt[0].State) } " ^
  "else { Write-Output 'EnsureBT: WinRT radios=0' }" ^
  >>"%LOG%" 2>&1

echo [%date% %time%] EnsureBT: end >>"%LOG%"
goto :eof
