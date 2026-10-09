package main

import (
	lgx "launcher/internal/logx"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"launcher/internal/platform/wifi"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) Shutdown() {
	a.stopLauncherInput()

	cursor.RestoreSystemCursor()
	exec.Command("shutdown", "-s", "-t", "0").Start()
	runtime.Quit(a.ctx)

	lgx.Info("shutting down...")
}

func (a *App) Restart() {
	a.stopLauncherInput()

	cursor.RestoreSystemCursor()
	exec.Command("shutdown", "-r", "-t", "0").Start()
	runtime.Quit(a.ctx)

	lgx.Info("restarting...")
}

func (a *App) ExitLauncher() {
	a.stopLauncherInput()

	cursor.RestoreSystemCursor()
	if !a.config.Debug {
		a.exitToWindows()
		return
	}

	lgx.Info("debug exit requested")
	runtime.Quit(a.ctx)
}

func (a *App) exitToWindows() {
	lgx.Info("exit to Windows requested")

	if err := os.WriteFile("exit-to-windows.flag", []byte("1\n"), 0644); err != nil {
		lgx.Errorf("write exit-to-windows marker failed: %v", err)
	}
	if err := exec.Command("explorer.exe").Start(); err != nil {
		lgx.Errorf("start explorer failed: %v", err)
	}

	runtime.Quit(a.ctx)
}

func (a *App) OpenBluetoothPairing() error {
	a.pairingMu.Lock()
	if a.pairingActive {
		a.pairingMu.Unlock()
		lgx.Debug("device pairing wizard already active")
		return nil
	}
	a.pairingActive = true
	a.pairingMu.Unlock()

	wizard := filepath.Join(os.Getenv("WINDIR"), "System32", "DevicePairingWizard.exe")
	cmd := exec.Command(wizard)
	if err := cmd.Start(); err != nil {
		a.pairingMu.Lock()
		a.pairingActive = false
		a.pairingMu.Unlock()
		lgx.Errorf("device pairing wizard failed: %v", err)
		return err
	}

	cursor.SetCursorHidden(true)
	a.pauseKeyboardInput()
	if a.pad != nil {
		a.pad.EnablePairingKeyboardModeForPID(uint32(cmd.Process.Pid))
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	runtime.EventsEmit(a.ctx, "pairing-ui:start", map[string]any{
		"pid": cmd.Process.Pid,
	})
	runtime.WindowHide(a.ctx)
	go func() {
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		a.waitForPairingWindow(uint32(cmd.Process.Pid), done)
		a.closePairingProcess(cmd, done)
		if a.pad != nil {
			a.pad.DisablePairingKeyboardMode()
		}
		controller.MoveCursorTo(0, 0)
		cursor.SetCursorHidden(true)
		runtime.EventsEmit(a.ctx, "pairing-ui:end", map[string]any{
			"pid": cmd.Process.Pid,
		})
		a.bringBack()
		a.resumeKeyboardInput()
		a.pairingMu.Lock()
		a.pairingActive = false
		a.pairingMu.Unlock()
		lgx.Info("Windows device pairing wizard closed")
	}()
	lgx.Infof("opened Windows device pairing wizard: %s", wizard)
	return nil
}

func (a *App) SetLauncherInputPaused(paused bool) {
	if paused {
		a.pauseLauncherInput()
		cursor.ForceCursorVisible()
		return
	}
	controller.MoveCursorTo(0, 0)
	cursor.SetCursorHidden(true)
	a.resumeLauncherInput()
}

func (a *App) SetLauncherKeyboardPaused(paused bool) {
	if paused {
		a.pauseKeyboardInput()
		cursor.ForceCursorVisible()
		return
	}
	controller.MoveCursorTo(0, 0)
	cursor.SetCursorHidden(true)
	a.resumeKeyboardInput()
}

func (a *App) ScanWifiNetworks() ([]wifi.Network, error) {
	lgx.Info("ScanWifiNetworks requested")
	networks, err := wifi.Scan()
	if err != nil {
		lgx.Errorf("scan WiFi networks failed: %v", err)
		return nil, err
	}
	lgx.Infof("scan WiFi networks found=%d", len(networks))
	return networks, nil
}

func (a *App) ConnectWifiNetwork(ssid string, password string) error {
	lgx.Infof("ConnectWifiNetwork requested ssid=%q", ssid)
	if err := wifi.Connect(ssid, password); err != nil {
		lgx.Errorf("connect WiFi network failed ssid=%q err=%v", ssid, err)
		a.emitHardwareStatus()
		return err
	}
	a.emitHardwareStatus()
	return nil
}

func (a *App) closePairingProcess(cmd *exec.Cmd, processDone <-chan struct{}) {
	select {
	case <-processDone:
		return
	default:
	}

	if cmd.Process == nil {
		return
	}

	lgx.Debugf("device pairing wizard window closed; terminating lingering process pid=%d", cmd.Process.Pid)
	if err := cmd.Process.Kill(); err != nil {
		lgx.Debugf("device pairing wizard kill skipped: %v", err)
	}

	select {
	case <-processDone:
	case <-time.After(1500 * time.Millisecond):
		lgx.Debugf("device pairing wizard process did not exit promptly pid=%d", cmd.Process.Pid)
	}
}

func (a *App) waitForPairingWindow(pid uint32, processDone <-chan struct{}) {
	seenWindow := false
	var lostWindowAt time.Time
	appearDeadline := time.Now().Add(8 * time.Second)
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-processDone:
			return
		case <-t.C:
			visible := controller.HasVisibleWindowForPID(pid)
			if visible {
				seenWindow = true
				lostWindowAt = time.Time{}
				controller.FocusProcessWindow(pid)
				continue
			}
			now := time.Now()
			if seenWindow {
				if lostWindowAt.IsZero() {
					lostWindowAt = now
				}
				if now.Sub(lostWindowAt) >= 700*time.Millisecond {
					return
				}
				continue
			}
			if now.After(appearDeadline) {
				return
			}
		}
	}
}
