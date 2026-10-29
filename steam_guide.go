package main

import (
	"launcher/internal/games"
	lgx "launcher/internal/logx"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type launcherGuideRequest struct {
	id         string
	at         time.Time
	diagnostic bool
}

// Called on SDL's owner: only queue a bounded notification, never query Steam.
func (a *App) queueLauncherGuide(id string, diagnostic bool) {
	select {
	case a.launcherGuideInput <- launcherGuideRequest{id: id, at: time.Now(), diagnostic: diagnostic}:
	default:
	}
}

func (a *App) watchLauncherGuide() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case request := <-a.launcherGuideInput:
			if time.Since(request.at) <= time.Second {
				a.handleLauncherGuide(request)
			}
		}
	}
}

// Caller holds gameSessionMu. Ownership prevents a delayed guard from clearing
// pointer/key mappings installed by a newly started game or pairing wizard.
func (a *App) clearSteamGuideInputLocked(reclaim bool) {
	if a.steamGuideInput {
		a.steamGuideInput = false
		if a.pad != nil {
			a.pad.DisableMouseMode()
		}
		if a.ctx.Err() == nil {
			a.resumeKeyboardInput()
			if reclaim || controller.LauncherHasFocus() {
				cursor.SetCursorHidden(true)
			}
			runtime.EventsEmit(a.ctx, "steam:guide-input", map[string]any{"active": false})
		}
	}
	if reclaim && a.ctx.Err() == nil && !a.gameSessionActive {
		a.bringBack()
		if !controller.FocusLauncherWindow() {
			lgx.Warn("Steam guide UI: launcher focus recovery failed")
		}
	}
}

func (a *App) handleLauncherGuide(request launcherGuideRequest) {
	exe := findSteamExecutable()
	if exe == "" {
		return
	}
	monitor, err := games.NewSteamGuideMonitor(exe)
	if err != nil {
		lgx.Debugf("Steam guide monitor unavailable: %v", err)
		return
	}
	defer monitor.Close()
	defer func() {
		a.gameSessionMu.Lock()
		a.clearSteamGuideInputLocked(false)
		a.gameSessionMu.Unlock()
	}()
	deadline := time.Now().Add(5 * time.Second)
	var target uintptr
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}
		ui, err := monitor.Poll()
		if err != nil {
			lgx.Warnf("Steam guide UI inspection stopped: %v", err)
			return
		}
		a.gameSessionMu.Lock()
		a.pairingMu.Lock()
		pairing := a.pairingActive
		a.pairingMu.Unlock()
		if a.gameSessionActive || pairing || a.pad == nil || a.pad.IsTesting() != request.diagnostic {
			a.gameSessionMu.Unlock()
			return
		}
		if request.diagnostic {
			// Steam observes the physical button independently of SDL. Keep
			// its verified UI out of the diagnostic capture without switching
			// to pointer mode, releasing the test, or sending dialog actions.
			if ui.Window != 0 {
				if err := monitor.BackgroundForControllerTest(ui); err == nil {
					a.bringBack()
					focused := controller.FocusLauncherWindow()
					lgx.Infof("Steam guide UI: controller test focus recovered=%t pid=%d title=%q", focused, ui.PID, ui.Title)
				}
			}
			a.gameSessionMu.Unlock()
			if time.Now().After(deadline) {
				return
			}
			continue
		}
		if target != 0 && ui.Window != target {
			a.clearSteamGuideInputLocked(ui.Window != 0 || controller.LauncherHasFocus())
			a.gameSessionMu.Unlock()
			return // Prompt closed or focus intentionally moved elsewhere.
		}
		if ui.Window != 0 && ui.Minimized {
			if err := monitor.HideMinimized(ui); err == nil {
				lgx.Infof("Steam guide UI: recovering launcher from minimized/hidden Steam window title=%q", ui.Title)
				a.clearSteamGuideInputLocked(true)
			}
		} else if ui.Window != 0 && target == 0 {
			if err := a.pad.SelectController(request.id); err != nil {
				a.gameSessionMu.Unlock()
				return
			}
			target = ui.Window
			a.steamGuideInput = true
			a.pad.EnableMouseModeForWindow(ui.PID, ui.Window)
			a.pauseKeyboardInput()
			cursor.ForceCursorVisible()
			runtime.EventsEmit(a.ctx, "steam:guide-input", map[string]any{"active": true})
			lgx.Infof("Steam guide UI: controller pointer enabled pid=%d window=%x title=%q", ui.PID, ui.Window, ui.Title)
		}
		a.gameSessionMu.Unlock()
		if target == 0 && time.Now().After(deadline) {
			return
		}
	}
}
