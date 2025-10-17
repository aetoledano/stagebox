# once_register_ensureBT.ps1 — v2 (PowerShell-native, kiosk-safe)
# Creates two tasks under \Launcher\ :
#  - EnsureBT-System : At startup (SYSTEM), primes core services (no Explorer)
#  - EnsureBT-User   : At logon   (Highest), runs ensure-bt.ps1 (no Explorer)

$ErrorActionPreference = "Stop"

$PSExe  = 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'
$Script = 'C:\launcher\ensure-bt.ps1'
$TaskPath = '\Launcher\'

# Remove any old tasks
Get-ScheduledTask -TaskName 'EnsureBT-System' -TaskPath $TaskPath -ErrorAction SilentlyContinue | Unregister-ScheduledTask -Confirm:$false
Get-ScheduledTask -TaskName 'EnsureBT-User'   -TaskPath $TaskPath -ErrorAction SilentlyContinue | Unregister-ScheduledTask -Confirm:$false

# SYSTEM @ startup — start core services early (hidden, no desktop)
$actionSys   = New-ScheduledTaskAction -Execute $PSExe -Argument '-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -Command "& { Start-Service bthserv; Start-Service RmSvc -ErrorAction SilentlyContinue; Start-Service DsmSvc -ErrorAction SilentlyContinue; Start-Service BthAvctpSvc -ErrorAction SilentlyContinue }"'
$triggerSys  = New-ScheduledTaskTrigger -AtStartup
$triggerSys.Delay = 'PT10S'   # 10-second delay at boot
$settingsSys = New-ScheduledTaskSettingsSet -StartWhenAvailable
Register-ScheduledTask -TaskName 'EnsureBT-System' -TaskPath $TaskPath -Action $actionSys -Trigger $triggerSys -Settings $settingsSys -RunLevel Highest -User 'SYSTEM' -Force

# USER @ logon — run the kiosk-safe ensure script (no Explorer)
$actionUser   = New-ScheduledTaskAction -Execute $PSExe -Argument ('-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "{0}"' -f $Script)
$triggerUser  = New-ScheduledTaskTrigger -AtLogOn
$triggerUser.Delay = 'PT07S'   # 7-second delay after logon
$settingsUser = New-ScheduledTaskSettingsSet -StartWhenAvailable
Register-ScheduledTask -TaskName 'EnsureBT-User' -TaskPath $TaskPath -Action $actionUser -Trigger $triggerUser -Settings $settingsUser -RunLevel Highest -Force

Write-Host "Registered tasks under $TaskPath"
Get-ScheduledTask -TaskPath $TaskPath | Format-Table TaskName, State, LastRunTime -AutoSize
