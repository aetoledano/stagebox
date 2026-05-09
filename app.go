package main

import (
	"context"
	"launcher/internal/games"
	"launcher/internal/platform/bluetooth"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"launcher/internal/platform/display"
	"launcher/internal/platform/shell"
	"launcher/internal/platform/wallpaper"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"launcher/internal/config"
	lgx "launcher/internal/logx"
)

// App struct
type App struct {
	ctx                context.Context
	config             *config.Config
	games              map[uuid.UUID]*games.GameData
	gamesMu            sync.RWMutex
	pairingMu          sync.Mutex
	pairingActive      bool
	thumbnailDir       string
	thumbnailURLPrefix string
	pad                *controller.XInputBridge
	btStatus           string
	btConnected        bool
	controllerCount    int
}

// NewApp creates a new App application struct
func NewApp(config *config.Config, thumbnailDir string, thumbnailURLPrefix string) *App {
	return &App{
		config:             config,
		games:              map[uuid.UUID]*games.GameData{},
		thumbnailDir:       thumbnailDir,
		thumbnailURLPrefix: thumbnailURLPrefix,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	cursor.RestoreSystemCursor()
	cursor.SetCursorHidden(true)

	// Make this launcher the per-user shell every time we start (Windows only).
	if err := shell.EnsureSelfAsShell(); err != nil {
		// don’t crash on failure; just log
		println("ensureSelfAsShell failed:", err.Error())
	}

	_ = display.ForcePrimary1080pOrBest()

	// Start the bottom-most wallpaper host
	if stop, err := wallpaper.StartWallpaper("loading.jpg", "cover"); err == nil {
		// ensure it is torn down cleanly
		// if you already have a global cleanup, call stop() there instead
		go func() { <-ctx.Done(); stop() }() // on app exit
	} else {
		println("wallpaper host failed:", err.Error())
	}

	a.pad = controller.StartXInputBridge(ctx)

	go a.watchBluetooth()

	// Go fullscreen as soon as the window is ready
	runtime.WindowFullscreen(ctx)
}

func (a *App) bringBack() {
	// Order matters on Windows after other apps had focus.
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	runtime.WindowFullscreen(a.ctx)

	// Brief topmost pulse helps reclaim z-order.
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	time.Sleep(50 * time.Millisecond)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
}

// ... actions ...
func (a *App) Shutdown() {
	if a.pad != nil {
		a.pad.Stop()
	}

	cursor.RestoreSystemCursor()
	exec.Command("shutdown", "-s", "-t", "0").Start()
	runtime.Quit(a.ctx)

	log.Println("shutting down...")
}

func (a *App) Restart() {
	if a.pad != nil {
		a.pad.Stop()
	}

	cursor.RestoreSystemCursor()
	exec.Command("shutdown", "-r", "-t", "0").Start()
	runtime.Quit(a.ctx)

	log.Println("restarting...")
}

func (a *App) ExitLauncher() {
	if a.pad != nil {
		a.pad.Stop()
	}

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
		a.pairingMu.Lock()
		a.pairingActive = false
		a.pairingMu.Unlock()
		lgx.Info("Windows device pairing wizard closed")
	}()
	lgx.Infof("opened Windows device pairing wizard: %s", wizard)
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

func (a *App) LoadGames() map[uuid.UUID]*games.GameData {
	log.Println("Loading games from:", a.config.GamesRoot)

	a.gamesMu.Lock()
	a.games = map[uuid.UUID]*games.GameData{}
	a.gamesMu.Unlock()

	var progressMu sync.Mutex
	processed := 0
	totalDirs := 0
	emitProgress := func(loaded int, total int) {
		progressMu.Lock()
		processed = loaded
		totalDirs = total
		progressMu.Unlock()

		runtime.EventsEmit(a.ctx, "games:loading", map[string]any{
			"loading": loaded < total,
			"loaded":  loaded,
			"total":   total,
		})
	}

	loaded := games.LoadGamesWithOptions(a.config.GamesRoot, games.LoadOptions{
		ThumbnailDir:       a.thumbnailDir,
		ThumbnailURLPrefix: a.thumbnailURLPrefix,
		Workers:            games.DefaultLoadWorkers,
		OnGame: func(g *games.GameData) {
			a.gamesMu.Lock()
			a.games[g.ID] = g
			a.gamesMu.Unlock()

			runtime.EventsEmit(a.ctx, "game:loaded", g)
		},
		OnProgress: emitProgress,
		OnError: func(gamePath string, err error) {
			runtime.EventsEmit(a.ctx, "game:load-error", map[string]any{
				"path":  gamePath,
				"error": err.Error(),
			})
		},
	})

	a.gamesMu.Lock()
	a.games = loaded
	a.gamesMu.Unlock()

	progressMu.Lock()
	finalLoaded := processed
	finalTotal := totalDirs
	progressMu.Unlock()
	runtime.EventsEmit(a.ctx, "games:loading", map[string]any{
		"loading": false,
		"loaded":  finalLoaded,
		"total":   finalTotal,
	})

	return a.games
}

func (a *App) Launch(id uuid.UUID) {
	a.gamesMu.RLock()
	game := a.games[id]
	a.gamesMu.RUnlock()
	if game == nil {
		log.Printf("Game with ID %s not found", id)
		return
	}

	gameKeyMapActive := false
	if a.pad != nil {
		if game.KeyMap != nil {
			gameKeyMapActive = a.pad.EnableGameKeyMap(controller.GameKeyMap{
				Buttons:          game.KeyMap.Buttons,
				LeftStick:        game.KeyMap.LeftStick,
				RightStick:       game.KeyMap.RightStick,
				Mouse:            game.KeyMap.Mouse,
				MouseSensitivity: game.KeyMap.MouseSensitivity,
			})
			if !gameKeyMapActive {
				a.pad.Pause()
			}
		} else {
			a.pad.Pause()
		}
	}

	log.Printf("Launching game: %s", game.Name)

	// Tell the frontend to pause gamepad loops, overlays, etc.
	runtime.EventsEmit(a.ctx, "game:start", map[string]any{
		"id":   game.ID.String(),
		"name": game.Name,
	})

	runtime.WindowHide(a.ctx)

	ch := games.LaunchGame(game)

	// Don't block the call; wait in background
	go func() {
		if game.ShowCursor {
			cursor.ForceCursorVisible()
		} else {
			cursor.ForceCursorHiddenFor(2 * time.Second)
		}

		err := <-ch
		if err != nil {
			log.Printf("Error launching game %s: %v", game.Name, err)
		} else {
			log.Printf("Game %s exited", game.Name)
		}

		cursor.RestoreSystemCursor()
		if a.pad != nil && gameKeyMapActive {
			a.pad.DisableGameKeyMap()
		}
		controller.MoveCursorTo(0, 0)
		cursor.SetCursorHidden(true)

		a.bringBack()

		// Let frontend resume input
		runtime.EventsEmit(a.ctx, "game:end", map[string]any{
			"id":   game.ID.String(),
			"name": game.Name,
			"ok":   err == nil,
		})

		if a.pad != nil {
			a.pad.Resume()
		}
	}()
}

func (a *App) KillRunning() error {
	return games.KillRunning()
}

func (a *App) Explorer() {
	exec.Command("explorer.exe", a.config.GamesRoot).Start()
}

func (a *App) GetStatuses() map[string]any {
	st, connected, count := a.readHardwareStatus()
	return a.statusPayload(st, connected, count)
}

func (a *App) watchBluetooth() {
	lgx.Info("Starting Bluetooth state watcher")

	// initial emit
	s, connected, count := a.readHardwareStatus()
	a.btStatus = s
	a.btConnected = connected
	a.controllerCount = count
	runtime.EventsEmit(a.ctx, "status:update", a.statusPayload(s, connected, count))

	t := time.NewTicker(2 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			st, connected, count := a.readHardwareStatus()
			if st != a.btStatus || connected != a.btConnected || count != a.controllerCount {
				a.btStatus = st
				a.btConnected = connected
				a.controllerCount = count
				runtime.EventsEmit(a.ctx, "status:update", a.statusPayload(st, connected, count))
			}
		case <-a.ctx.Done():
			return
		}
	}
}

// app.go
func (a *App) EnsureBT() {
	bluetooth.EnsureUp()
	// emit a fresh status so UI updates
	st, connected, count := a.readHardwareStatus()
	runtime.EventsEmit(a.ctx, "status:update", a.statusPayload(st, connected, count))
}

func (a *App) readHardwareStatus() (string, bool, int) {
	st := bluetooth.State()
	count := controller.ConnectedCount()
	return st, st == "on" && bluetooth.AnyConnectedDevice(), count
}

func (a *App) statusPayload(bt string, btConnected bool, controllerCount int) map[string]any {
	return map[string]any{
		"bt":              bt,
		"btConnected":     btConnected,
		"controllerCount": controllerCount,
		"debug":           a.config.Debug,
	}
}

// Logging wrappers
func (a *App) Info(x any) {
	lgx.Info(x)
}

func (a *App) Warn(x any) {
	lgx.Warn(x)
}

func (a *App) Error(x any) {
	lgx.Error(x)
}
