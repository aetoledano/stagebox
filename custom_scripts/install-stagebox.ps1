<#
  install-stagebox.ps1

  One elevated setup entry point for a StageBox Windows kiosk.
  Run as Administrator from the folder containing this script.

  Kept scripts after install:
  - install-stagebox.ps1
  - svc.vbs
  - systemsvcs.ps1
  - elevated-PS.cmd
#>

param(
  [string]$LauncherDir = "C:\launcher",
  [string]$GamesRoot = "C:\games",
  [switch]$SkipShares,
  [switch]$SkipAutoLogon,
  [switch]$SkipConfig,
  [switch]$SkipShell
)

$ErrorActionPreference = "Stop"

function Test-Admin {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (-not (Test-Admin)) {
  throw "Run this script as Administrator."
}

$SourceDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$TaskPath = "\Launcher\"
$LogPath = Join-Path $LauncherDir "installation.log"
$CurrentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
$CurrentFullAccount = $CurrentIdentity.Name
$CurrentAccount = if ($CurrentFullAccount.Contains("\")) { $CurrentFullAccount.Split("\")[-1] } else { $CurrentFullAccount }
$CurrentSid = $CurrentIdentity.User.Value
$CurrentProfile = $env:USERPROFILE

function Log([string]$msg) {
  $stamp = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss.fff")
  $line = "[install-stagebox] $stamp $msg"
  Write-Host $line
  Add-Content -Path $LogPath -Value $line -Encoding UTF8
}

function Ask-YesNo([string]$Prompt, [bool]$Default = $true) {
  $suffix = if ($Default) { "[Y/n]" } else { "[y/N]" }
  $answer = Read-Host "$Prompt $suffix"
  if ([string]::IsNullOrWhiteSpace($answer)) { return $Default }
  return $answer.Trim().ToLowerInvariant().StartsWith("y")
}

function Ask-Value([string]$Prompt, [string]$Default = "") {
  $suffix = if ($Default) { " [$Default]" } else { "" }
  $answer = Read-Host "$Prompt$suffix"
  if ([string]::IsNullOrWhiteSpace($answer)) { return $Default }
  return $answer.Trim()
}

function Resolve-LocalUserName([string]$User) {
  if ([string]::IsNullOrWhiteSpace($User)) { return "" }
  if ($User.Contains("\")) { return $User.Split("\")[-1] }
  return $User
}

function Resolve-DomainName([string]$User) {
  if ([string]::IsNullOrWhiteSpace($User)) { return $env:COMPUTERNAME }
  if ($User.Contains("\")) {
    $domain = $User.Split("\")[0]
    if ($domain -eq ".") { return $env:COMPUTERNAME }
    return $domain
  }
  return $env:COMPUTERNAME
}

function ConvertTo-PlainText([Security.SecureString]$Secure) {
  if (-not $Secure -or $Secure.Length -eq 0) { return "" }
  $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Secure)
  try {
    return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
  }
}

function Log-AccountDiagnostics([string]$Account) {
  $localName = Resolve-LocalUserName $Account
  Log "account diagnostics account=$Account localName=$localName currentSid=$CurrentSid profile=$CurrentProfile"
  try {
    $users = @(Get-LocalUser -Name $localName -ErrorAction SilentlyContinue)
    foreach ($user in $users) {
      Log "local user name=$($user.Name) sid=$($user.SID.Value) enabled=$($user.Enabled) lastLogon=$($user.LastLogon)"
    }
    if ($users.Count -eq 0) {
      Log "local user not found by Get-LocalUser name=$localName"
    }
  } catch {
    Log "local user diagnostics failed: $($_.Exception.Message)"
  }
  try {
    $escaped = $localName.Replace("'", "''")
    $accounts = @(Get-CimInstance Win32_UserAccount -Filter "LocalAccount=True AND Name='$escaped'" -ErrorAction SilentlyContinue)
    foreach ($acct in $accounts) {
      Log "cim user name=$($acct.Name) domain=$($acct.Domain) sid=$($acct.SID) disabled=$($acct.Disabled)"
    }
    if ($accounts.Count -eq 0) {
      Log "cim local user not found name=$localName"
    }
  } catch {
    Log "cim user diagnostics failed: $($_.Exception.Message)"
  }
  try {
    $profiles = @(Get-ChildItem "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList" -ErrorAction SilentlyContinue)
    foreach ($profile in $profiles) {
      $profilePath = (Get-ItemProperty -Path $profile.PSPath -Name ProfileImagePath -ErrorAction SilentlyContinue).ProfileImagePath
      if ($profilePath -and ($profilePath -like "*\$localName" -or $profilePath -like "*\$localName.*")) {
        Log "profile sid=$($profile.PSChildName) path=$profilePath"
      }
    }
  } catch {
    Log "profile diagnostics failed: $($_.Exception.Message)"
  }
}

function Copy-StageBoxScripts {
  foreach ($file in @(
    "stagebox.exe",
    "svc.vbs",
    "systemsvcs.ps1"
  )) {
    $src = Join-Path $SourceDir $file
    $dst = Join-Path $LauncherDir $file
    if (Test-Path $src) {
      if ([IO.Path]::GetFullPath($src) -ne [IO.Path]::GetFullPath($dst)) {
        Copy-Item -Path $src -Destination $dst -Force
      }
      Unblock-File -Path $dst -ErrorAction SilentlyContinue
      Log "copied $file"
    }
  }
}

function Assert-StageBoxExecutable {
  $exe = Join-Path $LauncherDir "stagebox.exe"
  if (-not (Test-Path $exe)) {
    throw "stagebox.exe was not found at $exe. Put stagebox.exe beside install-stagebox.ps1 in the staging folder, then rerun the installer."
  }
  Log "verified stagebox.exe at $exe"
}

function Write-StageBoxConfig {
  if ($SkipConfig) {
    Log "config.json setup skipped"
    return
  }

  $configPath = Join-Path $LauncherDir "config.json"
  if ((Test-Path $configPath) -and -not (Ask-YesNo "Overwrite existing config.json?" $false)) {
    Log "config.json preserved"
    return
  }

  $config = [ordered]@{
    gamesRoot = $GamesRoot
    debug = $true
  }
  $json = $config | ConvertTo-Json -Depth 4
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [IO.File]::WriteAllText($configPath, $json + [Environment]::NewLine, $utf8NoBom)
  Log "wrote config.json gamesRoot=$GamesRoot debug=true"
}

function Remove-LegacyFiles {
  foreach ($file in @(
    "bootstrapper.cmd",
    "bootstrapper.vbs",
    "bootstrapper.log",
    "ensure-bt.ps1",
    "install-stagebox.ps1",
    "launcher.exe",
    "launcher.log",
    "once_autologon.bat",
    "once_initializer.bat",
    "once_register_bootstrapper.bat",
    "once_register_ensureBT.ps1",
    "stagebox-debug.log",
    "stagebox-svc.log",
    "stagebox-services.ps1",
    "tool_check-bt.ps1",
    "tool_check-bt2.ps1"
  )) {
    $path = Join-Path $LauncherDir $file
    if (Test-Path $path) {
      try {
        Remove-Item -Path $path -Force -ErrorAction Stop
        Log "removed legacy file $file"
      } catch {
        Log "legacy file removal skipped $file`: $($_.Exception.Message)"
      }
    }
  }
}

function Ensure-ServiceReady([string]$Name, [string]$StartupType = "Automatic") {
  $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
  if (-not $svc) {
    Log "service:$Name not found"
    return
  }
  try {
    Set-Service -Name $Name -StartupType $StartupType -ErrorAction Stop
    Log "service:$Name startup=$StartupType"
  } catch {
    Log "service:$Name startup change failed: $($_.Exception.Message)"
  }
  try {
    if ($svc.Status -ne "Running") {
      Start-Service -Name $Name -ErrorAction Stop
      Start-Sleep -Milliseconds 300
    }
    $svc = Get-Service -Name $Name -ErrorAction Stop
    Log "service:$Name status=$($svc.Status)"
  } catch {
    Log "service:$Name start failed: $($_.Exception.Message)"
  }
}

function Register-StageBoxTasks([string]$TaskUser) {
  $existingTasks = @(Get-ScheduledTask -TaskPath $TaskPath -ErrorAction SilentlyContinue)
  foreach ($task in $existingTasks) {
    $task | Unregister-ScheduledTask -Confirm:$false
    Log "removed scheduled task $($task.TaskPath)$($task.TaskName)"
  }

  $svcScript = Join-Path $LauncherDir "svc.vbs"
  $systemAction = New-ScheduledTaskAction -Execute "wscript.exe" -Argument ('//B //Nologo "{0}" systemsvcs -Diagnostics' -f $svcScript)
  $systemTrigger = New-ScheduledTaskTrigger -AtStartup
  $systemTrigger.Delay = "PT10S"
  $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew
  $systemPrincipal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -RunLevel Highest
  Register-ScheduledTask -TaskName "StageBox-EnsureSystem" -TaskPath $TaskPath -Action $systemAction -Trigger $systemTrigger -Settings $settings -Principal $systemPrincipal -Force | Out-Null
  Log "registered task StageBox-EnsureSystem"

  if ($TaskUser) {
    $userAction = New-ScheduledTaskAction -Execute "wscript.exe" -Argument ('//B //Nologo "{0}" systemsvcs' -f $svcScript)
    $userTrigger = New-ScheduledTaskTrigger -AtLogOn -User $TaskUser
    $userTrigger.Delay = "PT07S"
    $userPrincipal = New-ScheduledTaskPrincipal -UserId $TaskUser -RunLevel LeastPrivilege -LogonType Interactive
    Register-ScheduledTask -TaskName "StageBox-EnsureUser" -TaskPath $TaskPath -Action $userAction -Trigger $userTrigger -Settings $settings -Principal $userPrincipal -Force | Out-Null
    Log "registered task StageBox-EnsureUser for $TaskUser"
  }
}

function Ensure-Share([string]$Name, [string]$Path, [string]$Account) {
  if (-not (Test-Path $Path)) {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
    Log "created folder $Path"
  }

  if ($Account) {
    icacls $Path /grant "$Account`:(OI)(CI)M" /T | Out-Null
    Log "granted NTFS Modify on $Path to $Account"
  }

  $share = Get-SmbShare -Name $Name -ErrorAction SilentlyContinue
  if (-not $share) {
    if ($Account) {
      New-SmbShare -Name $Name -Path $Path -ChangeAccess $Account -CachingMode None | Out-Null
    } else {
      New-SmbShare -Name $Name -Path $Path -CachingMode None | Out-Null
    }
    Log "created SMB share $Name -> $Path"
  } else {
    if ($Account) {
      Grant-SmbShareAccess -Name $Name -AccountName $Account -AccessRight Change -Force | Out-Null
      Log "updated SMB share $Name access for $Account"
    }
  }
}

function Configure-Shares([string]$Account) {
  if ($SkipShares -or -not (Ask-YesNo "Configure SMB shares?" $true)) {
    Log "share setup skipped"
    return
  }

  Ensure-ServiceReady lanmanserver Automatic
  netsh advfirewall firewall set rule group="File and Printer Sharing" new enable=Yes | Out-Null
  netsh advfirewall firewall set rule group="Network Discovery" new enable=Yes | Out-Null
  Log "enabled SMB/discovery firewall rules"

  $launcherShare = Ask-Value "Launcher share name (blank skips)" "launcher"
  if ($launcherShare) {
    $launcherPath = Ask-Value "Launcher share path" $LauncherDir
    Ensure-Share $launcherShare $launcherPath $Account
  }

  $gamesShare = Ask-Value "Games share name (blank skips)" "gameRoot"
  if ($gamesShare) {
    $gamesPath = Ask-Value "Games share path" "D:\"
    Ensure-Share $gamesShare $gamesPath $Account
  }
}

function Configure-AutoLogon([string]$Account) {
  if ($SkipAutoLogon -or -not $Account -or -not (Ask-YesNo "Configure Windows AutoAdminLogon for '$Account'?" $false)) {
    Log "autologon setup skipped"
    return
  }

  Write-Host "AutoAdminLogon stores the password in the Winlogon registry key. Use only on this kiosk image." -ForegroundColor Yellow
  $password = Read-Host -AsSecureString "Password for AutoAdminLogon user '$Account' (blank skips AutoAdminLogon)"
  if (-not $password -or $password.Length -eq 0) {
    Log "autologon setup skipped: blank password"
    return
  }
  $plainPassword = ConvertTo-PlainText $password
  $localName = Resolve-LocalUserName $Account
  $domain = Resolve-DomainName $Account
  $winlogon = "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon"

  Set-ItemProperty -Path $winlogon -Name AutoAdminLogon -Type String -Value "1"
  Set-ItemProperty -Path $winlogon -Name ForceAutoLogon -Type String -Value "1"
  Set-ItemProperty -Path $winlogon -Name DefaultUserName -Type String -Value $localName
  Set-ItemProperty -Path $winlogon -Name DefaultDomainName -Type String -Value $domain
  Set-ItemProperty -Path $winlogon -Name DefaultPassword -Type String -Value $plainPassword
  Log "configured AutoAdminLogon username=$localName domain=$domain"
}

function Register-Shell {
  if ($SkipShell -or -not (Ask-YesNo "Register current user's shell to StageBox service loop?" $true)) {
    Log "shell registration skipped"
    return
  }
  $shellValue = 'wscript.exe //B //Nologo "{0}"' -f (Join-Path $LauncherDir "svc.vbs")
  $hkcuWinlogon = "HKCU:\Software\Microsoft\Windows NT\CurrentVersion\Winlogon"
  New-Item -Path $hkcuWinlogon -Force | Out-Null
  Set-ItemProperty -Path $hkcuWinlogon -Name Shell -Value $shellValue
  Log "registered HKCU shell: $shellValue"

  $hkuWinlogon = "Registry::HKEY_USERS\$CurrentSid\Software\Microsoft\Windows NT\CurrentVersion\Winlogon"
  New-Item -Path $hkuWinlogon -Force | Out-Null
  Set-ItemProperty -Path $hkuWinlogon -Name Shell -Value $shellValue
  $readBack = (Get-ItemProperty -Path $hkuWinlogon -Name Shell -ErrorAction Stop).Shell
  Log "registered HKU shell sid=$CurrentSid shell=$readBack"
}

New-Item -ItemType Directory -Path $LauncherDir -Force | Out-Null
New-Item -ItemType File -Path $LogPath -Force | Out-Null
Log "----- install start ----- source=$SourceDir target=$LauncherDir"
Log "current full account=$CurrentFullAccount"
Log "registration account=$CurrentAccount"
Log "current sid=$CurrentSid"
Log "current profile=$CurrentProfile"

$account = $CurrentAccount
Log-AccountDiagnostics $account

Copy-StageBoxScripts
Assert-StageBoxExecutable
Write-StageBoxConfig
Remove-LegacyFiles

Ensure-ServiceReady WlanSvc Automatic
Ensure-ServiceReady bthserv Automatic
Ensure-ServiceReady RmSvc Manual
Ensure-ServiceReady DeviceAssociationService Manual
Ensure-ServiceReady DsmSvc Manual
Ensure-ServiceReady BthAvctpSvc Manual

Register-StageBoxTasks $account
try {
  Start-ScheduledTask -TaskPath $TaskPath -TaskName "StageBox-EnsureSystem"
  Log "started task StageBox-EnsureSystem"
} catch {
  Log "start task StageBox-EnsureSystem failed: $($_.Exception.Message)"
}

Configure-Shares $account
Configure-AutoLogon $account
Register-Shell

Log "----- install end -----"
