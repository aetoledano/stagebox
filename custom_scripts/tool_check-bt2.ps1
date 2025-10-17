# Show wireless/BT-adjacent network adapters (to infer vendor)
Get-PnpDevice -Class Net -PresentOnly:$false |
  Where-Object { $_.FriendlyName -match 'Wireless|Wi-Fi|AX|AC|Intel|Realtek|Mediatek|MediaTek' } |
  Select-Object Status, FriendlyName, InstanceId

# Dump likely BT hardware IDs (even if device is missing)
Get-CimInstance Win32_PnPEntity |
  Where-Object { $_.Name -match 'Bluetooth|Wireless|Wi-Fi|AX|AC|Intel|Realtek|Mediatek|MediaTek' } |
  Select-Object Name, PNPDeviceID, ConfigManagerErrorCode | Format-List

# Quick vendor hint from PCI IDs (8086=Intel, 10EC=Realtek, 14C3=Mediatek)
Get-CimInstance Win32_PnPEntity |
  Where-Object { $_.PNPDeviceID -match '^PCI\\VEN_' } |
  Select-Object Name, @{n='VEN';e={$_.PNPDeviceID -replace '.*VEN_([0-9A-F]{4}).*','$1'}},
                        @{n='DEV';e={$_.PNPDeviceID -replace '.*DEV_([0-9A-F]{4}).*','$1'}} |
  Sort-Object Name
