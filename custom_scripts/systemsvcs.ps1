<#
  systemsvcs.ps1

  Idempotent service/adapters primer for StageBox.
  Intended to run from scheduled tasks created by install-stagebox.ps1:
  - SYSTEM at startup for privileged services.
  - current user at logon for per-user Bluetooth service/radio work.
#>

param(
  [string]$LogPath = "C:\launcher\svc.log",
  [switch]$Diagnostics
)

$ErrorActionPreference = "Continue"

function Log([string]$msg) {
  $stamp = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss.fff")
  Add-Content -Path $LogPath -Value ("[systemsvcs] {0} {1}" -f $stamp, $msg) -Encoding UTF8
}

function Test-Admin {
  try {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  } catch { $false }
}

function Ensure-Service([string]$Name, [string]$StartupType = "") {
  try {
    $svc = Get-Service -Name $Name -ErrorAction Stop
    if ($StartupType) {
      try {
        Set-Service -Name $Name -StartupType $StartupType -ErrorAction Stop
        Log "svc:$Name startup=$StartupType"
      } catch {
        Log "svc:$Name startup failed: $($_.Exception.Message)"
      }
    }
    if ($svc.Status -ne "Running") {
      Start-Service -Name $Name -ErrorAction Stop
      Start-Sleep -Milliseconds 300
      $svc = Get-Service -Name $Name -ErrorAction Stop
    }
    Log "svc:$Name status=$($svc.Status)"
  } catch {
    Log "svc:$Name error: $($_.Exception.Message)"
  }
}

function Ensure-BluetoothUserService {
  try {
    $services = @(Get-Service "BluetoothUserService*" -ErrorAction SilentlyContinue)
    if ($services.Count -eq 0) {
      Log "bt-user-svc:none"
      return
    }
    foreach ($svc in $services) {
      if ($svc.Status -ne "Running") {
        try {
          Start-Service -Name $svc.Name -ErrorAction Stop
          Start-Sleep -Milliseconds 300
        } catch {
          Log "bt-user-svc:$($svc.Name) start failed: $($_.Exception.Message)"
        }
      }
      $fresh = Get-Service -Name $svc.Name -ErrorAction SilentlyContinue
      Log "bt-user-svc:$($svc.Name) status=$($fresh.Status)"
    }
  } catch {
    Log "bt-user-svc error: $($_.Exception.Message)"
  }
}

function Set-BluetoothRadioOn {
  try {
    Add-Type -AssemblyName System.Runtime.WindowsRuntime -ErrorAction Stop
    $radios = [Windows.Devices.Radios.Radio]::GetRadiosAsync().GetAwaiter().GetResult()
    $bt = @($radios | Where-Object { $_.Kind -eq [Windows.Devices.Radios.RadioKind]::Bluetooth })
    if ($bt.Count -gt 0) {
      $null = $bt[0].SetStateAsync([Windows.Devices.Radios.RadioState]::On).GetAwaiter().GetResult()
      Log "bt-radio state=$($bt[0].State)"
    } else {
      Log "bt-radio none"
    }
  } catch {
    Log "bt-radio error: $($_.Exception.Message)"
  }
}

$WiFiNamePattern = 'Wi-?Fi|Wireless|WLAN|802\.11|Intel\(R\).*Wireless|Realtek.*Wireless|MediaTek|Mediatek|Qualcomm.*Wireless|Broadcom.*Wireless'
$WiFiIdPattern = 'PCI\\VEN_8086|PCI\\VEN_10EC|PCI\\VEN_14C3|PCI\\VEN_168C|PCI\\VEN_14E4|USB\\VID_0BDA|USB\\VID_0E8D'

function Ensure-WiFiAdapters {
  try {
    $canSetPnpDevice = [bool](Get-Command Set-PnpDevice -ErrorAction SilentlyContinue)
    $targets = @(Get-PnpDevice -Class Net -PresentOnly:$false -ErrorAction SilentlyContinue |
      Where-Object {
        ($_.FriendlyName -match $WiFiNamePattern -or $_.InstanceId -match $WiFiIdPattern) -and
        $_.FriendlyName -notmatch 'Wi-Fi Direct Virtual|Xbox Wireless Adapter'
      })
    Log "wifi-targets count=$($targets.Count)"
    foreach ($device in $targets) {
      Log "wifi-target status=$($device.Status) name=$($device.FriendlyName) id=$($device.InstanceId)"
      if ($device.Status -in @("Error", "Unknown", "Disabled")) {
        if (-not $canSetPnpDevice) {
          Log "wifi-target enable skipped Set-PnpDevice unavailable id=$($device.InstanceId)"
          continue
        }
        try {
          Set-PnpDevice -InstanceId $device.InstanceId -Enable -Confirm:$false -ErrorAction Stop
          Log "wifi-target enabled id=$($device.InstanceId)"
        } catch {
          Log "wifi-target enable failed id=$($device.InstanceId): $($_.Exception.Message)"
        }
      }
    }
  } catch {
    Log "wifi-targets error: $($_.Exception.Message)"
  }
}

function Write-Diagnostics {
  try {
    $drivers = & netsh.exe wlan show drivers 2>&1
    Log ("diag wlan drivers: {0}" -f (($drivers | Out-String).Trim()))
  } catch {
    Log "diag wlan drivers failed: $($_.Exception.Message)"
  }
  try {
    $interfaces = & netsh.exe wlan show interfaces 2>&1
    Log ("diag wlan interfaces: {0}" -f (($interfaces | Out-String).Trim()))
  } catch {
    Log "diag wlan interfaces failed: $($_.Exception.Message)"
  }
}

function Connect-StageBoxWiFiProfile {
  param(
    [int]$TimeoutSeconds = 60
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  try {
    do {
      $interfaces = & netsh.exe wlan show interfaces 2>&1
      $interfaceText = ($interfaces | Out-String)
      if ($interfaceText -match '(?im)^\s*State\s*:\s*connected\s*$' -or $interfaceText -match '(?im)^\s*Estado\s*:\s*conectado\s*$') {
        Log "wifi-autoconnect already connected"
        return
      }

      $profiles = & netsh.exe wlan show profiles 2>&1
      $profileText = ($profiles | Out-String).Trim()
      $names = @()
      foreach ($line in $profiles) {
        if ($line -match ':\s*(StageBox-[A-Fa-f0-9]+)\s*$') {
          $names += $Matches[1]
        }
      }
      if ($names.Count -eq 0) {
        Log "wifi-autoconnect no StageBox profiles visible yet: $profileText"
        Start-Sleep -Seconds 5
        continue
      }

      foreach ($name in $names) {
        $out = & netsh.exe wlan connect name="$name" 2>&1
        Log ("wifi-autoconnect profile={0}: {1}" -f $name, (($out | Out-String).Trim()))
        Start-Sleep -Seconds 5
        $interfaces = & netsh.exe wlan show interfaces 2>&1
        $text = ($interfaces | Out-String)
        if ($text -match '(?im)^\s*State\s*:\s*connected\s*$' -or $text -match '(?im)^\s*Estado\s*:\s*conectado\s*$') {
          Log "wifi-autoconnect connected profile=$name"
          return
        }
      }

      if ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 5
      }
    } while ((Get-Date) -lt $deadline)

    Log "wifi-autoconnect timed out after ${TimeoutSeconds}s"
  } catch {
    Log "wifi-autoconnect failed: $($_.Exception.Message)"
  }
}

function Connect-KnownWiFiProfile {
  param(
    [int]$TimeoutSeconds = 45
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  try {
    do {
      $interfaces = & netsh.exe wlan show interfaces 2>&1
      $text = ($interfaces | Out-String)
      if ($text -match '(?im)^\s*State\s*:\s*connected\s*$' -or $text -match '(?im)^\s*Estado\s*:\s*conectado\s*$') {
        Log "wifi-known-autoconnect already connected"
        return
      }

      $profiles = & netsh.exe wlan show profiles 2>&1
      $names = @()
      foreach ($line in $profiles) {
        if ($line -match ':\s*(.+?)\s*$') {
          $name = $Matches[1].Trim()
          if ($name -and $name -notmatch '^StageBox-') {
            $names += $name
          }
        }
      }
      foreach ($name in $names) {
        $out = & netsh.exe wlan connect name="$name" 2>&1
        Log ("wifi-known-autoconnect profile={0}: {1}" -f $name, (($out | Out-String).Trim()))
        Start-Sleep -Seconds 5
        $interfaces = & netsh.exe wlan show interfaces 2>&1
        $text = ($interfaces | Out-String)
        if ($text -match '(?im)^\s*State\s*:\s*connected\s*$' -or $text -match '(?im)^\s*Estado\s*:\s*conectado\s*$') {
          Log "wifi-known-autoconnect connected profile=$name"
          return
        }
      }

      if ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 5
      }
    } while ((Get-Date) -lt $deadline)

    Log "wifi-known-autoconnect timed out after ${TimeoutSeconds}s"
  } catch {
    Log "wifi-known-autoconnect failed: $($_.Exception.Message)"
  }
}

Log "----- services start ----- admin=$((Test-Admin)) pid=$PID"

Ensure-Service WlanSvc Automatic
Ensure-WiFiAdapters
Connect-StageBoxWiFiProfile
Connect-KnownWiFiProfile
Ensure-Service bthserv Automatic
Ensure-Service RmSvc Manual
Ensure-Service DeviceAssociationService Manual
Ensure-Service DsmSvc Manual
Ensure-Service BthAvctpSvc Manual
Ensure-BluetoothUserService
Set-BluetoothRadioOn
if ($Diagnostics) { Write-Diagnostics }

Log "----- services end -----"
