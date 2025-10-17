@echo off
REM Run in an elevated Command Prompt
set "USER=gamerX"
set "PASS=gamerX"

REM Enable AutoAdminLogon for local user
reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" /v AutoAdminLogon     /t REG_SZ /d 1 /f
reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" /v ForceAutoLogon     /t REG_SZ /d 1 /f
reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" /v DefaultUserName    /t REG_SZ /d %USER% /f
reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" /v DefaultDomainName  /t REG_SZ /d %COMPUTERNAME% /f
reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" /v DefaultPassword    /t REG_SZ /d %PASS% /f

echo.
echo AutoAdminLogon configured for %COMPUTERNAME%\%USER%.
echo After you set your shares, REBOOT to test auto-login.
pause
