package controller

import "strings"

func hasConnectedBluetoothController(devices []controllerDevice) bool {
	for _, device := range devices {
		if device.info.Connected && device.info.Connection == "Bluetooth" {
			return true
		}
	}
	return false
}

func controllerTransportFromDeviceIDs(ids []string) (connection, address string) {
	for _, id := range ids {
		lower := strings.ToLower(id)
		if strings.HasPrefix(lower, `bthenum\`) || strings.HasPrefix(lower, `bthledevice\`) || strings.HasPrefix(lower, `bthle\`) {
			connection = "Bluetooth"
		}
		if candidate := bluetoothAddressFromDeviceID(id); candidate != "" {
			return "Bluetooth", candidate
		}
		if connection == "" && strings.HasPrefix(lower, `usb\`) {
			connection = "USB"
		}
	}
	return connection, ""
}

// SDL distinguishes wired/wireless, not Bluetooth vs a radio receiver. Prefer
// Windows ancestry; an unrecognized HID path must never imply a USB cable.
func controllerConnectionLabel(path, transport string, sdlState int32) string {
	lower := strings.ToLower(path)
	if transport == "Bluetooth" || strings.Contains(lower, "bth") || strings.Contains(lower, "00001124") {
		return "Bluetooth"
	}
	if transport == "USB" {
		if sdlState == 2 {
			return "USB / wireless receiver"
		}
		return "USB"
	}
	switch sdlState {
	case 1:
		return "Wired"
	case 2:
		return "Wireless"
	default:
		return "Unknown"
	}
}

func ControllerNeedsPairingCheck(info ControllerInfo) bool {
	switch info.Connection {
	case "Bluetooth", "Wireless", "Unknown", "USB / other", "XInput":
		return true
	}
	return info.BluetoothAddress != ""
}
