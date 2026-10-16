//go:build windows

package bluetooth

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var pairedDevicesMu sync.Mutex

// Enumerate Windows Classic and paired LE records; do not scan nearby devices.
// Adapter-owned pairings are not Windows records.
func PairedDevices() ([]PairedDevice, error) {
	pairedDevicesMu.Lock()
	defer pairedDevicesMu.Unlock()
	return pairedDevicesForDisplay().verifiedDevices()
}

func PairedDevicesForDisplay() PairedDeviceList {
	pairedDevicesMu.Lock()
	defer pairedDevicesMu.Unlock()
	return pairedDevicesForDisplay()
}

func pairedDevicesForDisplay() PairedDeviceList {
	classic, classicErr := pairedClassicDevices()
	le, leErr := pairedBLEDevices()
	return pairedDeviceList(classic, classicErr, le, leErr)
}

func pairedClassicDevices() ([]PairedDevice, error) {
	if !initBluetoothProcs() {
		return nil, fmt.Errorf("Windows Bluetooth APIs are unavailable")
	}
	params := BLUETOOTH_DEVICE_SEARCH_PARAMS{FReturnAuthenticated: 1, FReturnRemembered: 1}
	params.DwSize = uint32(unsafe.Sizeof(params))
	var info BLUETOOTH_DEVICE_INFO
	info.DwSize = uint32(unsafe.Sizeof(info))
	devices := []PairedDevice{}
	find, _, callErr := procFindFirstDevice.Call(uintptr(unsafe.Pointer(&params)), uintptr(unsafe.Pointer(&info)))
	if find == 0 {
		if errors.Is(callErr, windows.ERROR_NO_MORE_ITEMS) {
			return devices, nil
		}
		return nil, fmt.Errorf("list paired Bluetooth devices: %w", callErr)
	}
	defer procFindDeviceClose.Call(find)
	seen := map[string]int{}
	for {
		if (info.FRemembered != 0 || info.FAuthenticated != 0) && info.Address.Addr != 0 {
			id := formatBluetoothAddress(info.Address.Addr)
			name := strings.TrimSpace(windows.UTF16ToString(info.SzName[:]))
			if name == "" {
				name = "Bluetooth device"
			}
			device := PairedDevice{ID: id, Name: name, Address: displayAddress(id), Connected: info.FConnected != 0, Transport: "Classic"}
			if index, ok := seen[id]; ok {
				devices[index].Connected = devices[index].Connected || device.Connected
			} else {
				seen[id] = len(devices)
				devices = append(devices, device)
			}
		}
		info.DwSize = uint32(unsafe.Sizeof(info))
		next, _, nextErr := procFindNextDevice.Call(find, uintptr(unsafe.Pointer(&info)))
		if next == 0 {
			if !errors.Is(nextErr, windows.ERROR_NO_MORE_ITEMS) {
				return nil, fmt.Errorf("list paired Bluetooth devices: %w", nextErr)
			}
			break
		}
	}
	return devices, nil
}

func ForgetDevice(id string) (PairedDevice, error) {
	if err := validatePairedDeviceID(id); err != nil {
		return PairedDevice{}, err
	}
	pairedDevicesMu.Lock()
	defer pairedDevicesMu.Unlock()
	// Revalidate the exact paired address; never remove arbitrary PnP nodes.
	var devices []PairedDevice
	var err error
	if strings.HasPrefix(id, "le:") {
		devices, err = pairedBLEDevices()
	} else {
		devices, err = pairedClassicDevices()
	}
	if err != nil {
		return PairedDevice{}, err
	}
	found := false
	var selected PairedDevice
	for _, device := range devices {
		if device.ID == id {
			if device.Transport == "LE" {
				return forgetBLEDevice(device)
			}
			selected = device
			found = true
			break
		}
	}
	if !found {
		return PairedDevice{}, fmt.Errorf("this device is no longer paired; refresh the list")
	}
	for _, dll := range []*windows.LazyDLL{bthMainDLL, bthAltDLL} {
		remove := dll.NewProc("BluetoothRemoveDevice")
		if remove.Find() != nil {
			continue
		}
		address, _ := pairedDeviceAddress(id)
		value := BLUETOOTH_ADDRESS{Addr: address}
		result, _, _ := remove.Call(uintptr(unsafe.Pointer(&value)))
		if result != 0 {
			return PairedDevice{}, fmt.Errorf("forget Bluetooth device: %w", windows.Errno(result))
		}
		return selected, nil
	}
	return PairedDevice{}, fmt.Errorf("Windows Bluetooth device removal is unavailable")
}
