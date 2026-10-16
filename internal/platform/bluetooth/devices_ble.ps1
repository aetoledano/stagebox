# Windows PowerShell 5.1 hosts the built-in WinRT projection; no extra runtime
# or helper deployment. Invoked hidden with a static encoded script and exact
# endpoint/address values in environment variables, never source interpolation.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$stage = 'initialize WinRT'
$script:deviceDiagnostics = 0
try {
    Add-Type -AssemblyName System.Runtime.WindowsRuntime
    $null = [Windows.Devices.Bluetooth.BluetoothLEDevice, Windows.Devices.Bluetooth, ContentType = WindowsRuntime]
    $null = [Windows.Devices.Enumeration.DeviceInformation, Windows.Devices.Enumeration, ContentType = WindowsRuntime]
    $null = [Windows.Devices.Enumeration.DeviceInformationCollection, Windows.Devices.Enumeration, ContentType = WindowsRuntime]
    $null = [Windows.Devices.Enumeration.DeviceInformationKind, Windows.Devices.Enumeration, ContentType = WindowsRuntime]
    $null = [Windows.Devices.Enumeration.DeviceUnpairingResult, Windows.Devices.Enumeration, ContentType = WindowsRuntime]
    $asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
        $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and
        $_.GetGenericArguments().Count -eq 1 -and $_.GetParameters().Count -eq 1 -and
        $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
    } | Select-Object -First 1
    if ($null -eq $asTask) { throw 'Windows Runtime asynchronous projection is unavailable.' }
    # Invoke the projected dictionary interface explicitly. PowerShell's dynamic
    # indexer/member adapter can return null for WinRT map objects, making absent
    # metadata indistinguishable from a failed lookup (and null becomes false).
    $propertiesGetter = [Windows.Devices.Enumeration.DeviceInformation].GetProperty('Properties')
    $tryGetProperty = [System.Collections.Generic.IReadOnlyDictionary[string, object]].GetMethod('TryGetValue')

    function Await-WinRT($operation, [Type]$resultType, [int]$timeout) {
        $task = $asTask.MakeGenericMethod($resultType).Invoke($null, @($operation))
        if (-not $task.Wait($timeout)) { throw 'Windows Bluetooth operation timed out. Refresh devices before retrying.' }
        return ,$task.GetAwaiter().GetResult()
    }
    function Device-Properties($device) {
        $map = $propertiesGetter.GetValue($device, $null)
        if ($null -eq $map) { throw 'Windows returned no Bluetooth LE property map.' }
        $snapshot = @{}
        foreach ($key in $properties) {
            $arguments = [object[]]@($key, $null)
            if ($tryGetProperty.Invoke($map, $arguments)) {
                $snapshot[$key] = $arguments[1]
            }
        }
        return ,$snapshot
    }
    function Device-Address($metadata) {
        $rawAddress = $metadata['System.Devices.Aep.DeviceAddress']
        $address = ([string]$rawAddress -replace '[:-]', '').ToLowerInvariant()
        if ($address -notmatch '^[0-9a-f]{12}$' -or $address -eq '000000000000') {
            return ''
        }
        return $address
    }
    function Write-DeviceMetadata($device, $metadata) {
        # Include devices with valid addresses too: missing/false connection
        # metadata is independent of address validity. Keep diagnostics bounded
        # and separate from stdout JSON, without changing pairing behavior.
        if ($script:deviceDiagnostics -ge 8) { return }
        $script:deviceDiagnostics++
        try {
            $rawAddress = $metadata['System.Devices.Aep.DeviceAddress']
            $rawConnected = $metadata['System.Devices.Aep.IsConnected']
            $rawText = [string]$rawAddress
            $connectedText = [string]$rawConnected
            $name = [string]$device.Name
            $keys = @($metadata.Keys)
            $addressType = 'null'
            $connectedType = 'null'
            if ($null -ne $rawAddress) { $addressType = $rawAddress.GetType().FullName }
            if ($null -ne $rawConnected) { $connectedType = $rawConnected.GetType().FullName }
            $details = @{
                name = $name.Substring(0, [Math]::Min(96, $name.Length))
                paired = [bool]$device.Pairing.IsPaired
                connected = [bool]$rawConnected
                connectedPropertyPresent = ($keys -contains 'System.Devices.Aep.IsConnected')
                connectedType = $connectedType
                connectedRaw = $connectedText.Substring(0, [Math]::Min(96, $connectedText.Length))
                addressPropertyPresent = ($keys -contains 'System.Devices.Aep.DeviceAddress')
                addressType = $addressType
                addressRaw = $rawText.Substring(0, [Math]::Min(96, $rawText.Length))
                address = Device-Address $metadata
                propertyReader = 'IReadOnlyDictionary.TryGetValue'
                propertyKeys = @($keys | Select-Object -First 16 | ForEach-Object {
                    $key = [string]$_
                    $key.Substring(0, [Math]::Min(96, $key.Length))
                })
            }
            [Console]::Error.WriteLine('device metadata ' + (ConvertTo-Json -InputObject $details -Depth 3 -Compress))
        } catch {
            [Console]::Error.WriteLine('device metadata diagnostics failed: ' + $_.Exception.GetBaseException().Message + ' at line ' + $_.InvocationInfo.ScriptLineNumber)
        }
    }

    $stage = 'enumerate paired LE endpoints'
    $selector = [Windows.Devices.Bluetooth.BluetoothLEDevice]::GetDeviceSelectorFromPairingState($true)
    $properties = [string[]]@('System.Devices.Aep.DeviceAddress', 'System.Devices.Aep.IsConnected', 'System.Devices.Aep.IsPaired')
    $operation = [Windows.Devices.Enumeration.DeviceInformation]::FindAllAsync(
        $selector, $properties, [Windows.Devices.Enumeration.DeviceInformationKind]::AssociationEndpoint)
    $devices = Await-WinRT $operation ([Windows.Devices.Enumeration.DeviceInformationCollection]) 10000
    if ($devices.Count -gt 1024) { throw 'Windows Bluetooth LE list is too large.' }

    if ($env:STAGEBOX_BT_ACTION -eq 'list') {
        $stage = 'read paired LE endpoint properties'
        $items = @(
            foreach ($device in $devices) {
                if (-not $device.Pairing.IsPaired) { continue }
                $metadata = Device-Properties $device
                Write-DeviceMetadata $device $metadata
                [pscustomobject]@{
                    deviceID = [string]$device.Id
                    address = Device-Address $metadata
                    name = [string]$device.Name
                    connected = [bool]$metadata['System.Devices.Aep.IsConnected']
                }
            }
        )
        [Console]::Out.Write((ConvertTo-Json -InputObject @{devices = $items} -Depth 4 -Compress))
    } elseif ($env:STAGEBOX_BT_ACTION -eq 'forget') {
        $stage = 'revalidate selected LE endpoint'
        $matches = @($devices | Where-Object { $_.Id -ceq $env:STAGEBOX_BT_DEVICE -and $_.Pairing.IsPaired })
        if ($matches.Count -ne 1) {
            throw 'This Bluetooth LE pairing changed or no longer exists. Refresh the list.'
        }
        $metadata = Device-Properties $matches[0]
        Write-DeviceMetadata $matches[0] $metadata
        $currentAddress = Device-Address $metadata
        if ($env:STAGEBOX_BT_ADDRESS -and $currentAddress -and $currentAddress -cne $env:STAGEBOX_BT_ADDRESS) {
            throw 'This Bluetooth LE device address changed. Refresh the list.'
        }
        $stage = 'unpair selected LE endpoint'
        $result = Await-WinRT ($matches[0].Pairing.UnpairAsync()) ([Windows.Devices.Enumeration.DeviceUnpairingResult]) 30000
        $status = $result.Status.ToString()
        if ($status -ne 'Unpaired' -and $status -ne 'AlreadyUnpaired') { throw "Windows refused Bluetooth LE unpairing: $status" }
        $addressForProfile = $currentAddress
        if (-not $addressForProfile) { $addressForProfile = $env:STAGEBOX_BT_ADDRESS }
        [Console]::Out.Write((ConvertTo-Json -InputObject @{status = $status; address = $addressForProfile} -Compress))
    } else { throw 'Invalid Bluetooth operation.' }
} catch {
    [Console]::Error.WriteLine("${stage}: $($_.Exception.GetBaseException().Message) at line $($_.InvocationInfo.ScriptLineNumber)")
    exit 1
}
