package main

import (
	"fmt"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	lgx "launcher/internal/logx"
	"launcher/internal/platform/bluetooth"
)

func (a *App) GetPairedBluetoothDevices() (bluetooth.PairedDeviceList, error) {
	if a.pad != nil {
		if err := a.pad.RememberBluetoothControllers(); err != nil {
			lgx.Warnf("controller Bluetooth association save failed: %v", err)
		}
	}
	result := bluetooth.PairedDevicesForDisplay()
	if !result.Complete {
		lgx.Warnf("Bluetooth device list incomplete: %s", result.Details)
	} else {
		lgx.Debugf("Bluetooth devices listed count=%d", len(result.Devices))
	}
	for _, device := range result.Devices {
		if device.Transport == "LE" {
			lgx.Debugf("Bluetooth LE list result id=%s name=%q address=%q connected=%t", device.ID, device.Name, device.Address, device.Connected)
		}
	}
	return result, nil
}

func (a *App) ForgetBluetoothDevice(id string) (string, error) {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.gameSessionActive {
		return "", fmt.Errorf("close the game before forgetting a Bluetooth device")
	}
	a.pairingMu.Lock()
	defer a.pairingMu.Unlock()
	if a.pairingActive {
		return "", fmt.Errorf("close Bluetooth Pair before forgetting a device")
	}
	if a.pad != nil && a.pad.IsTesting() {
		return "", fmt.Errorf("release the controller test before forgetting a device")
	}
	device, err := bluetooth.ForgetDevice(id)
	if err != nil {
		lgx.Warnf("Bluetooth forget failed id=%q: %v", id, err)
		return "", err
	}
	lgx.Infof("Bluetooth device forgotten id=%s", id)
	if a.pad != nil {
		address := device.ControllerAddress()
		if address == "" {
			lgx.Warnf("Bluetooth device forgotten but profile cleanup could not be linked id=%s: Windows did not report its address", id)
			return "Saved profile cleanup could not be verified because Windows did not report the device address.", nil
		}
		profiles := a.pad.ControllerProfilesForBluetooth(address)
		if err := a.pad.RemoveControllerProfiles(profiles); err != nil {
			lgx.Warnf("Bluetooth device forgotten but controller profile cleanup failed id=%s profiles=%v err=%v", id, profiles, err)
			return "", fmt.Errorf("Bluetooth device forgotten, but its saved controller profile could not be removed: %w. Remove it from Controllers after fixing the file", err)
		}
		runtime.EventsEmit(a.ctx, "controllers:update", a.pad.Controllers())
		lgx.Infof("Bluetooth controller profiles removed address=%s profiles=%v", address, profiles)
	}
	return "", nil
}
