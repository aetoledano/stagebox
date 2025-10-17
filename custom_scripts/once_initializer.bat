@echo off
setlocal ENABLEDELAYEDEXPANSION
chcp 65001 >nul

REM ====== CONFIG =============================================================
set "USER_NAME=gamerX"

set "SHARE1_NAME=launcher"
set "SHARE1_PATH=C:\launcher"

set "SHARE2_NAME=gameRoot"
set "SHARE2_PATH=D:\"
REM ===========================================================================

echo:
echo === SMB Quick Setup for user '%USER_NAME%' ===============================

REM --- Admin check ---
net session >nul 2>&1
if errorlevel 1 (
  echo [ERROR] Please run this script as Administrator.
  pause
  exit /b 1
)

REM --- Ensure folders exist ---
if not exist "%SHARE1_PATH%" (
  echo [INFO] Creating "%SHARE1_PATH%"...
  mkdir "%SHARE1_PATH%" || (echo [ERROR] Could not create "%SHARE1_PATH%".& exit /b 1)
)
if not exist "%SHARE2_PATH%" (
  echo [ERROR] Path "%SHARE2_PATH%" not found. Aborting.
  exit /b 1
)

REM --- SMB server service ---
echo [INFO] Enabling SMB server service...
sc config lanmanserver start= auto >nul
sc start  lanmanserver >nul

REM --- Set active NICs to Private profile (so sharing rules apply) ---
echo [INFO] Setting active network profiles to Private...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "Get-NetConnectionProfile | ForEach-Object { if ($_.NetworkCategory -ne 'Private') { try { Set-NetConnectionProfile -InterfaceIndex $_.InterfaceIndex -NetworkCategory Private -ErrorAction Stop } catch {} } }"

REM --- Firewall rules for SMB & discovery ---
echo [INFO] Enabling firewall rules for File and Printer Sharing + Network Discovery...
netsh advfirewall firewall set rule group="File and Printer Sharing" new enable=Yes >nul
netsh advfirewall firewall set rule group="Network Discovery" new enable=Yes >nul

REM --- Ensure local user 'gamerX' exists and SET/RESET a password (secure prompt) ---
echo [INFO] Ensuring local user "%USER_NAME%" exists and has a password...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$u='%USER_NAME%';" ^
  "$acct = Get-LocalUser -Name $u -ErrorAction SilentlyContinue;" ^
  "if (-not $acct) { New-LocalUser -Name $u -NoPassword -PasswordNeverExpires -AccountNeverExpires | Out-Null; }" ^
  "$pw = Read-Host -AsSecureString -Prompt ('Enter NEW password for user ''%USER_NAME%''');" ^
  "Set-LocalUser -Name $u -Password $pw -PasswordNeverExpires $true;" ^
  "Add-LocalGroupMember -Group 'Users' -Member $u -ErrorAction SilentlyContinue;"

REM --- NTFS permissions ---
echo [INFO] Granting NTFS Modify to %COMPUTERNAME%\%USER_NAME% on "%SHARE1_PATH%" (recursive)...
icacls "%SHARE1_PATH%" /grant "%COMPUTERNAME%\%USER_NAME%":(OI)(CI)M /T >nul

echo [INFO] Granting NTFS Modify to %COMPUTERNAME%\%USER_NAME% on "%SHARE2_PATH%" (inheritable, not recursive)...
REM NOTE: This sets an inheritable ACE on D:\. Existing items that block inheritance won't change.
icacls "%SHARE2_PATH%" /grant "%COMPUTERNAME%\%USER_NAME%":(OI)(CI)M >nul

REM --- Create/Update SMB shares via PowerShell (preferred) ---
echo [INFO] Creating SMB shares "%SHARE1_NAME%" and "%SHARE2_NAME%"...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$u='%COMPUTERNAME%\%USER_NAME%';" ^
  "$s1='%SHARE1_NAME%'; $p1='%SHARE1_PATH%';" ^
  "$s2='%SHARE2_NAME%'; $p2='%SHARE2_PATH%';" ^
  "function Ensure-Share($name,$path,$acct) {" ^
  "  if (-not (Get-SmbShare -Name $name -ErrorAction SilentlyContinue)) {" ^
  "    New-SmbShare -Name $name -Path $path -ChangeAccess $acct -CachingMode None | Out-Null" ^
  "  } else {" ^
  "    Grant-SmbShareAccess -Name $name -AccountName $acct -AccessRight Change -Force | Out-Null" ^
  "  }" ^
  "}" ^
  "Ensure-Share $s1 $p1 $u; Ensure-Share $s2 $p2 $u" ^
  " " || goto :LegacyShare

goto :PostShare

:LegacyShare
echo [WARN] PowerShell SMB cmdlets failed; using legacy NET SHARE...
net share "%SHARE1_NAME%" >nul 2>&1 && net share "%SHARE1_NAME%" /delete /y >nul
net share "%SHARE1_NAME%"="%SHARE1_PATH%" /GRANT:"%COMPUTERNAME%\%USER_NAME%",CHANGE /REMARK:"Launcher Share" /CACHE:None
if errorlevel 1 ( echo [ERROR] NET SHARE failed for %SHARE1_NAME%.& exit /b 1 )

net share "%SHARE2_NAME%" >nul 2>&1 && net share "%SHARE2_NAME%" /delete /y >nul
net share "%SHARE2_NAME%"="%SHARE2_PATH%" /GRANT:"%COMPUTERNAME%\%USER_NAME%",CHANGE /REMARK:"Game Root" /CACHE:None
if errorlevel 1 ( echo [ERROR] NET SHARE failed for %SHARE2_NAME%.& exit /b 1 )

:PostShare

REM --- Quick port test ---
for /f "tokens=2 delims=:" %%A in ('powershell -NoProfile -Command "(Test-NetConnection -ComputerName 127.0.0.1 -Port 445).TcpTestSucceeded"') do set "TCP445=%%A"
echo %TCP445% | find /I "True" >nul
if errorlevel 1 (
  echo [WARN] Port 445 test didn't return True yet. SMB may still be initializing.
)

echo:
echo [DONE]
echo   Share 1: \\%COMPUTERNAME%\%SHARE1_NAME%   (path: %SHARE1_PATH%)
echo   Share 2: \\%COMPUTERNAME%\%SHARE2_NAME%   (path: %SHARE2_PATH%)
echo   User   : %COMPUTERNAME%\%USER_NAME%
echo:
echo  Connect using that username and the password you just set.
echo  Tip: try by IP too: \\^<kiosk-ip^>\%SHARE1_NAME%   and   \\^<kiosk-ip^>\%SHARE2_NAME%
echo:
pause
endlocal
