package main

import (
	"fmt"
	"launcher/internal/platform/controller"
)

func (a *App) SetControllerMappingShortcut(id string) error {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.pad == nil {
		return fmt.Errorf("controller input is unavailable")
	}
	if id != "" && a.gameSessionActive {
		return fmt.Errorf("close the game before mapping")
	}
	return a.pad.SetControllerMappingShortcut(id)
}

func (a *App) BeginControllerMapping(id string) (controller.ControllerMappingState, error) {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	a.pairingMu.Lock()
	defer a.pairingMu.Unlock()
	if a.gameSessionActive || a.pairingActive {
		return controller.ControllerMappingState{}, fmt.Errorf("close the game or Bluetooth pairing before mapping")
	}
	if a.pad == nil {
		return controller.ControllerMappingState{}, fmt.Errorf("controller input is unavailable")
	}
	a.clearSteamGuideInputLocked(false)
	state, err := a.pad.BeginControllerMapping(id)
	if err == nil {
		a.SetLauncherKeyboardPaused(true)
	}
	return state, err
}

func (a *App) ControllerMappingAction(id, action string) (controller.ControllerMappingState, error) {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.pad == nil {
		return controller.ControllerMappingState{}, fmt.Errorf("controller input is unavailable")
	}
	if a.gameSessionActive {
		return controller.ControllerMappingState{}, fmt.Errorf("close the game before mapping")
	}
	state, err := a.pad.ControllerMappingAction(id, action)
	if err == nil && (action == "save" || action == "cancel") {
		a.SetLauncherKeyboardPaused(false)
	}
	return state, err
}
