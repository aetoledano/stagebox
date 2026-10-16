package main

import (
	"fmt"
	lgx "launcher/internal/logx"
	"launcher/internal/platform/bluetooth"
	"launcher/internal/platform/controller"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Steam/RetroBat pass false: only the saved controller defaults select routes.
// Native cards may additionally opt in through their existing game setting.
func (a *App) prepareSessionVirtualInput(id, name string, gameEnabled bool) error {
	if a.pad == nil || !a.pad.NeedsVirtualInput(gameEnabled) {
		return nil
	}
	_ = a.pad.TestController("")
	runtime.EventsEmit(a.ctx, "game:launch-status", map[string]any{
		"id": id, "name": name, "stage": "virtual-input", "message": "Preparing Xbox controller…", "progress": -1,
	})
	return a.pad.EnableVirtualInput(a.ctx, gameEnabled)
}

func (a *App) finishSessionVirtualInput(name string, sessionErr error) error {
	if a.pad != nil {
		if err := a.pad.DisableVirtualInput(); err != nil {
			lgx.Warnf("virtual input cleanup name=%q: %v", name, err)
			if sessionErr == nil {
				return err
			}
		}
	}
	return sessionErr
}

func (a *App) GetControllers() []controller.ControllerInfo {
	if a.pad != nil {
		if err := a.pad.RememberBluetoothControllers(); err != nil {
			lgx.Warnf("controller Bluetooth association save failed: %v", err)
		}
	}
	return a.pad.Controllers()
}

func (a *App) RemoveSavedController(id string) ([]controller.ControllerInfo, error) {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.gameSessionActive {
		return nil, fmt.Errorf("close the game before removing a controller")
	}
	a.pairingMu.Lock()
	defer a.pairingMu.Unlock()
	if a.pairingActive {
		return nil, fmt.Errorf("close Bluetooth Pair before removing a controller")
	}
	if a.pad == nil {
		return nil, fmt.Errorf("controller input is unavailable")
	}
	if a.pad.IsTesting() {
		return nil, fmt.Errorf("release the controller test before removing a controller")
	}
	var target *controller.ControllerInfo
	for _, info := range a.pad.Controllers() {
		if info.ID == id {
			copy := info
			target = &copy
			break
		}
	}
	if target == nil || !target.Known {
		return nil, fmt.Errorf("saved controller not found; refresh the list")
	}
	var pairedAddresses []string
	if controller.ControllerNeedsPairingCheck(*target) {
		devices, err := bluetooth.PairedDevices()
		if err != nil {
			return nil, fmt.Errorf("cannot verify Bluetooth pairing: %w", err)
		}
		for _, device := range devices {
			pairedAddresses = append(pairedAddresses, device.ControllerAddress())
		}
	}
	if err := controller.CheckSavedControllerRemoval(*target, pairedAddresses); err != nil {
		return nil, err
	}
	if err := a.pad.RemoveControllerProfiles([]string{id}); err != nil {
		return nil, err
	}
	controllers := a.pad.Controllers()
	runtime.EventsEmit(a.ctx, "controllers:update", controllers)
	lgx.Infof("saved controller removed id=%s name=%q", id, target.Name)
	return controllers, nil
}

func (a *App) SetControllerDefault(id, mode string) ([]controller.ControllerInfo, error) {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.gameSessionActive {
		return nil, fmt.Errorf("close the game before changing controller defaults")
	}
	if a.pad == nil {
		return nil, fmt.Errorf("controller input is unavailable")
	}
	if err := a.pad.SetControllerDefault(id, mode); err != nil {
		return nil, err
	}
	lgx.Infof("controller default saved id=%s mode=%s", id, mode)
	controllers := a.pad.Controllers()
	runtime.EventsEmit(a.ctx, "controllers:update", controllers)
	return controllers, nil
}

func (a *App) SelectController(id string) error {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.gameSessionActive {
		return fmt.Errorf("close the game before selecting a controller")
	}
	if a.pad == nil {
		return fmt.Errorf("controller input is unavailable")
	}
	return a.pad.SelectController(id)
}

func (a *App) TestController(id string) error {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if id != "" && a.gameSessionActive {
		return fmt.Errorf("close the game before testing a controller")
	}
	if a.pad == nil {
		return fmt.Errorf("controller input is unavailable")
	}
	if id != "" {
		a.clearSteamGuideInputLocked(false)
	}
	if err := a.pad.TestController(id); err != nil {
		return err
	}
	if id != "" {
		a.SetLauncherKeyboardPaused(true)
	} else if !a.gameSessionActive {
		a.SetLauncherKeyboardPaused(false)
	}
	return nil
}

func (a *App) TestControllerRumble(id string) error {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if id != "" && a.gameSessionActive {
		return fmt.Errorf("close the game before testing controller rumble")
	}
	if a.pad == nil {
		return fmt.Errorf("controller input is unavailable")
	}
	return a.pad.TestControllerRumble(id)
}
