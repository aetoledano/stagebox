//go:build !windows

package bluetooth

import "fmt"

func PairedDevices() ([]PairedDevice, error) {
	return nil, fmt.Errorf("Bluetooth device management requires Windows")
}
func PairedDevicesForDisplay() PairedDeviceList {
	return PairedDeviceList{Devices: []PairedDevice{}, Warning: "Bluetooth device management requires Windows", Details: "Bluetooth device management requires Windows"}
}
func ForgetDevice(string) (PairedDevice, error) {
	return PairedDevice{}, fmt.Errorf("Bluetooth device management requires Windows")
}
