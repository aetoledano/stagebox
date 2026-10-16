package controller

import (
	"fmt"
	"strings"
)

// pairedAddresses must be a fresh successful Windows pairing snapshot for a
// Bluetooth profile; unavailable enumeration is an error at the App boundary.
func CheckSavedControllerRemoval(info ControllerInfo, pairedAddresses []string) error {
	if !info.Known {
		return fmt.Errorf("saved controller not found; refresh the list")
	}
	if info.Connected {
		return fmt.Errorf("disconnect the controller before removing its saved profile")
	}
	if !ControllerNeedsPairingCheck(info) {
		return nil
	}
	for _, address := range pairedAddresses {
		if info.BluetoothAddress == address {
			return fmt.Errorf("this controller is still paired; use Forget device in Bluetooth devices")
		}
	}
	if info.BluetoothAddress == "" && len(pairedAddresses) > 0 {
		return fmt.Errorf("this profile has no verified Bluetooth address; forget existing Bluetooth pairings before removing it")
	}
	return nil
}

func bluetoothAddressFromDeviceID(id string) string {
	parts := strings.Split(strings.ToLower(id), `\`)
	if len(parts) != 3 || (parts[0] != "bthenum" && parts[0] != "bthle") || !strings.HasPrefix(parts[1], "dev_") {
		return ""
	}
	address := strings.TrimPrefix(parts[1], "dev_")
	if !validBluetoothAddress(address) {
		return ""
	}
	return address
}
