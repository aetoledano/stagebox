<# 
  ensure-bt.ps1 — v2.8 (Kiosk-safe, WinRT-free)
  - No Explorer spawn. No desktop/UI side effects.
  - Ensures core services (bthserv, RmSvc, DsmSvc, etc.) and per-user BluetoothUserService_*.
  - Uses CIM as health source of truth; only restarts devices if health is NOT OK.
  - Logs to C:\launcher\bootstrapper.log
#>

param()

$LogPath = "C:\launcher\bootstrapper.log"

function Log([string]$msg){
  $stamp = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss.fff")
  Add-Content -Path $LogPath -Value ("[ensure-bt] {0} {1}" -f $stamp, $msg) -Encoding UTF8
}

function Is-Admin {
  try {
    $wid=[Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal($wid)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  } catch { $false }
}

function Start-Svc($n){
  try{
    $s=Get-Service -Name $n -ErrorAction Stop
    Log "svc:$n Status=$($s.Status)"
    if($s.Status -ne 'Running'){
      Start-Service $n -ErrorAction Stop
      Start-Sleep -Milliseconds 250
      $s=Get-Service $n -ErrorAction Stop
      Log "svc:$n -> $($s.Status)"
    }
    $true
  }catch{
    Log "svc:$n error: $($_.Exception.Message)"
    $false
  }
}

# Kiosk-safe: start per-user BluetoothUserService without spawning Explorer
function Ensure-UserBluetoothSvc {
  try {
    $svcs = Get-Service 'BluetoothUserService*' -ErrorAction SilentlyContinue
    if(-not $svcs){ Log "user-svc: BluetoothUserService* not found"; return $false }
    $ok=$false
    foreach($s in $svcs){
      Log "user-svc:$($s.Name) Status=$($s.Status)"
      if($s.Status -ne 'Running'){
        try{
          Start-Service -Name $s.Name -ErrorAction Stop
          Start-Sleep -Milliseconds 300
          $s2 = Get-Service $s.Name
          Log "user-svc:$($s2.Name) -> $($s2.Status)"
          if($s2.Status -eq 'Running'){ $ok=$true }
        } catch {
          try {
            Log "user-svc:$($s.Name) retry via sc.exe"
            & sc.exe start $s.Name | Out-Null
            Start-Sleep -Milliseconds 300
            $s3 = Get-Service $s.Name
            Log "user-svc:$($s3.Name) -> $($s3.Status)"
            if($s3.Status -eq 'Running'){ $ok=$true }
          } catch {
            Log "user-svc:$($s.Name) start failed: $($_.Exception.Message)"
          }
        }
      } else { $ok=$true }
    }
    return $ok
  } catch {
    Log "user-svc: query error: $($_.Exception.Message)"
    return $false
  }
}

# Health check (CIM authoritative on MiniOS)
function Get-BTHealth {
  try {
    $cim = Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue
    $adapter = @($cim | Where-Object {
      ($_.Name -match 'Bluetooth' -or $_.PNPDeviceID -match 'USB\\VID_8087|USB\\VID_0A5C|USB\\VID_0BDA|USB\\VID_0A12|BTH') -and
      $_.Status -eq 'OK'
    })
    $enum   = @($cim | Where-Object { $_.Name -eq 'Microsoft Bluetooth Enumerator' -and $_.Status -eq 'OK' })
    $enumLE = @($cim | Where-Object { $_.Name -eq 'Microsoft Bluetooth LE Enumerator' -and $_.Status -eq 'OK' })
    $radio  = @($cim | Where-Object { $_.PNPDeviceID -match '^SWD\\RADIO\\BLUETOOTH' -and $_.Status -eq 'OK' })
    $ok = (($adapter.Count -gt 0) -or ($radio.Count -gt 0)) -and (($enum.Count -gt 0) -or ($enumLE.Count -gt 0))
    Log ("health: adapter={0} enum={1} enumLE={2} radio={3} ok={4}" -f ($adapter.Count -gt 0),($enum.Count -gt 0),($enumLE.Count -gt 0),($radio.Count -gt 0),$ok)
    return [bool]$ok
  } catch {
    Log "health: error: $($_.Exception.Message)"
    return $false
  }
}

# Find candidate devices to restart only if needed
$BtNamePattern = 'Bluetooth|RFCOMM|Personal Area Network|Intel\(R\)\s*Wireless\s*Bluetooth'
$BtIdPattern   = 'BTH|USB\\VID_8087|USB\\VID_0A5C|USB\\VID_0BDA|USB\\VID_0A12'

function Find-BTTargets {
  $pnp = Get-PnpDevice -PresentOnly:$false -ErrorAction SilentlyContinue |
         Where-Object { $_.FriendlyName -match $BtNamePattern -or $_.InstanceId -match $BtIdPattern }
  if(($pnp|Measure-Object).Count -gt 0){ return $pnp }

  $cim = Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue |
         Where-Object { $_.Name -match $BtNamePattern -or $_.PNPDeviceID -match $BtIdPattern } |
         Select-Object @{n='Status';e={$_.Status}},
                       @{n='Class';e={'(CIM)'}},
                       @{n='FriendlyName';e={$_.Name}},
                       @{n='InstanceId';e={$_.PNPDeviceID}}
  return $cim
}

function Enable-And-Restart($list) {
  $changed=0; $restarted=0
  foreach($d in $list){
    if($d.InstanceId -notmatch '^(USB|PCI|BTH|ACPI|SWD|HID|ROOT)\\'){ continue }
    if($d.Status -in @('Error','Unknown','Disabled')){
      try{ Set-PnpDevice -InstanceId $d.InstanceId -Enable -Confirm:$false -ErrorAction Stop; Log ("dev: enabled {0}" -f $d.InstanceId); $changed++ }catch{ Log ("dev: enable failed {0}: {1}" -f $d.InstanceId,$_.Exception.Message) }
    }
    try{ pnputil /restart-device "$($d.InstanceId)" | Out-Null; Log ("dev: restart issued -> {0}" -f $d.InstanceId); $restarted++ }catch{ Log ("dev: restart failed {0}: {1}" -f $d.InstanceId,$_.Exception.Message) }
  }
  Log ("dev: changed={0} restarted={1}" -f $changed,$restarted)
}

# --- MAIN ---
Log "----- ensure-bt start ----- (Admin=$((Is-Admin))) KioskSafe=True PID=$PID"

# Core services
Start-Svc bthserv | Out-Null
Start-Svc RmSvc   | Out-Null
Start-Svc DeviceAssociationService | Out-Null
Start-Svc DsmSvc  | Out-Null
Start-Svc BthAvctpSvc | Out-Null

# Per-user service (no Explorer)
Ensure-UserBluetoothSvc | Out-Null

# If health is good, exit quietly
if(Get-BTHealth){
  Log "health: OK (no action)"
  Log "----- ensure-bt end -----`n"
  exit 0
}

# Not healthy → try healing
$targets = Find-BTTargets
Log ("targets: {0}" -f (($targets|Measure-Object).Count))
if(($targets|Measure-Object).Count -gt 0){
  Enable-And-Restart $targets
  Start-Sleep -Seconds 3
} else {
  Log "targets: none; pnputil scan"
  try { pnputil /scan-devices | Out-Null } catch {}
  Start-Sleep -Seconds 2
}

# Final health check
$ok = Get-BTHealth
Log ("result: ok={0}" -f $ok)
Log "----- ensure-bt end -----`n"
if($ok){ exit 0 } else { exit 2 }
