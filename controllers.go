package main

import (
	"fmt"
	lgx "launcher/internal/logx"
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

func (a *App) GetControllers() []controller.ControllerInfo { return a.pad.Controllers() }

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
