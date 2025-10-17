<# check-bt.ps1 — run from ELEVATED PowerShell
   Quick audit for MiniOS: WinRT availability, BT devices, DISM features/packages.
   Prints to screen and appends to C:\launcher\bootstrapper.log
#>

param(
  [string]$LogPath = "C:\launcher\bootstrapper.log"
)

$ts = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss.fff")
function Log($m){ Add-Content -Path $LogPath -Value ("[check-bt] {0} {1}" -f (Get-Date).ToString("yyyy-MM-dd HH:mm:ss.fff"), $m) -Encoding UTF8 }
function Section($t){ Write-Host "`n=== $t ===" -ForegroundColor Cyan; Log "SECTION: $t" }

Write-Host "check-bt.ps1 starting as $env:USERNAME (Admin=$((New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)))"
Log "---- check-bt start ----"

# 1) WinRT projection / extensions presence
Section "WinRT projection presence"
$wrAssemblies = [AppDomain]::CurrentDomain.GetAssemblies() | Where-Object { $_.GetName().Name -like 'System.Runtime.WindowsRuntime*' }
$wrPaths = Get-ChildItem -Path 'C:\Windows' -Recurse -Filter 'System.Runtime.WindowsRuntime.dll' -ErrorAction SilentlyContinue | Select-Object -First 5 -ExpandProperty FullName

if($wrAssemblies){ $wrAssemblies | ForEach-Object { Write-Host "Loaded: $($_.FullName)"; Log "WinRT loaded: $($_.FullName)" } } else { Write-Warning "No System.Runtime.WindowsRuntime assemblies loaded in current AppDomain."; Log "WinRT not loaded in AppDomain" }
if($wrPaths){ $wrPaths | ForEach-Object { Write-Host "Found DLL: $_"; Log "WinRT dll found: $_" } } else { Write-Warning "No System.Runtime.WindowsRuntime.dll found under C:\Windows (MiniOS often strips this)."; Log "WinRT dll not found on disk" }

# 2) Any Bluetooth devices at all (even non-present)
Section "Bluetooth devices (PnP/CIM)"
$ent = Get-CimInstance Win32_PnPEntity | Where-Object { $_.Name -match 'Bluetooth' }
$ent | Select-Object Name, Status, PNPDeviceID, ConfigManagerErrorCode | Format-Table -AutoSize
Log ("CIM BT count={0}" -f ($ent | Measure-Object | Select-Object -ExpandProperty Count))
foreach($d in $ent){ Log ("CIM: {0} | {1} | CMErr={2} | {3}" -f $d.Name,$d.Status,$d.ConfigManagerErrorCode,$d.PNPDeviceID) }

Section "Bluetooth across classes (Get-PnpDevice)"
$pnps = Get-PnpDevice -Class Bluetooth,Net,Radio,Ports -PresentOnly:$false -ErrorAction SilentlyContinue |
        Where-Object { $_.FriendlyName -match 'Bluetooth' -or $_.InstanceId -match 'BTH' }
$pnps | Select-Object Status, Class, FriendlyName, InstanceId | Format-Table -AutoSize
Log ("PNP BT-ish count={0}" -f ($pnps | Measure-Object | Select-Object -ExpandProperty Count))
foreach($p in $pnps){ Log ("PNP: {0} | {1} | {2} | {3}" -f $p.Status,$p.Class,$p.FriendlyName,$p.InstanceId) }

# 3) DISM packages/features mentioning Bluetooth
Section "DISM: packages/features containing 'bluetooth'"
try {
  $pkg = & dism /online /Get-Packages 2>$null | Select-String -Pattern "bluetooth" -SimpleMatch
  if($pkg){ $pkg | ForEach-Object { $_.ToString() } | % { Write-Host $_; Log ("DISM package line: {0}" -f $_) } }
  else { Write-Warning "No packages with 'bluetooth' string." ; Log "DISM: no packages with bluetooth" }
} catch { Write-Warning "DISM packages query failed: $($_.Exception.Message)"; Log "DISM packages error: $($_.Exception.Message)" }

try {
  $feat = & dism /online /Get-Features /Format:Table 2>$null | Select-String -Pattern "bluetooth" -SimpleMatch
  if($feat){ $feat | ForEach-Object { $_.ToString() } | % { Write-Host $_; Log ("DISM feature line: {0}" -f $_) } }
  else { Write-Warning "No features with 'bluetooth' string." ; Log "DISM: no features with bluetooth" }
} catch { Write-Warning "DISM features query failed: $($_.Exception.Message)"; Log "DISM features error: $($_.Exception.Message)" }

# 4) Summary / suggestions
Section "Summary"
$hasWr = ($wrAssemblies -or $wrPaths)
$btCount = ($ent | Measure-Object).Count + ($pnps | Measure-Object).Count
Write-Host ("WinRT present: {0}" -f $hasWr)
Write-Host ("BT devices found (any list): {0}" -f $btCount)
Log ("Summary: WinRT={0} BTCount={1}" -f $hasWr, $btCount)

if(-not $hasWr){
  Write-Host "→ MiniOS likely missing System.Runtime.WindowsRuntime; WinRT-based radio control will not work." -ForegroundColor Yellow
  Log "Advice: install .NET 4.8 or WindowsRuntime projection if WinRT control is desired"
}
if($btCount -eq 0){
  Write-Host "→ Windows currently sees NO Bluetooth hardware/driver. Install the correct BT driver or plug a supported USB BT dongle." -ForegroundColor Yellow
  Log "Advice: no BT devices; install driver or dongle"
} else {
  Write-Host "→ Bluetooth hardware present. Our lite script can manage services/device restarts; WinRT not strictly required." -ForegroundColor Green
  Log "Advice: BT present; lite management OK"
}

Log "---- check-bt end ----"
