package main

import (
	"context"
	"launcher/internal/games"
	"launcher/internal/platform/audio"
	"launcher/internal/platform/bluetooth"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"launcher/internal/platform/shell"
	"launcher/internal/platform/wallpaper"
	"launcher/internal/platform/wifi"
	"os"
	"os/exec"
	"strings"
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
	gameSessionMu      sync.Mutex
	gameSessionActive  bool
	pairingMu          sync.Mutex
	pairingActive      bool
	thumbnailDir       string
	thumbnailURLPrefix string
	loadingWallpaper   string
	pad                *controller.XInputBridge
	keyboard           *controller.KeyboardBridge
	btStatus           string
	btConnected        bool
	controllerCount    int
	wifiStatus         wifi.Status
	soundMu            sync.Mutex
	soundStatus        audio.Status
	soundError         string
	soundKnown         bool
	soundInput         chan string
}

// NewApp creates a new App application struct
func NewApp(config *config.Config, thumbnailDir string, thumbnailURLPrefix string, loadingWallpaper string) *App {
	return &App{
		config:             config,
		games:              map[uuid.UUID]*games.GameData{},
		thumbnailDir:       thumbnailDir,
		thumbnailURLPrefix: thumbnailURLPrefix,
		loadingWallpaper:   loadingWallpaper,
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

	// Start the bottom-most wallpaper host
	loadingWallpaper := a.loadingWallpaper
	if loadingWallpaper == "" {
		loadingWallpaper = "loading.jpg"
	}
	if stop, err := wallpaper.StartWallpaper(loadingWallpaper, "cover"); err == nil {
		// ensure it is torn down cleanly
		// if you already have a global cleanup, call stop() there instead
		go func() { <-ctx.Done(); stop() }() // on app exit
	} else {
		println("wallpaper host failed:", err.Error())
	}

	a.soundInput = make(chan string, 16)
	go a.watchSound()
	go a.handleSoundInput()
	a.pad = controller.StartXInputBridge(ctx, a.queueSoundInput)
	a.keyboard = controller.StartKeyboardBridge(ctx, a.queueSoundInput)

	go a.watchBluetooth()

	// Go fullscreen as soon as the window is ready
	runtime.WindowFullscreen(ctx)
}

func (a *App) domReady(ctx context.Context) {
	a.ctx = ctx
	lgx.Info("dom ready")
	a.bringBack()
	go func() {
		for _, delay := range []time.Duration{1500 * time.Millisecond, 4 * time.Second} {
			time.Sleep(delay)
			lgx.Debug("dom ready delayed bringBack")
			a.gameSessionMu.Lock()
			if !a.gameSessionActive {
				a.bringBack()
			}
			a.gameSessionMu.Unlock()
		}
	}()
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

func (a *App) pauseKeyboardInput() {
	if a.keyboard != nil {
		a.keyboard.Pause()
	}
}

func (a *App) resumeKeyboardInput() {
	if a.keyboard != nil {
		a.keyboard.Resume()
	}
}

func (a *App) stopLauncherInput() {
	if a.pad != nil {
		a.pad.Stop()
	}
	if a.keyboard != nil {
		a.keyboard.Stop()
	}
}

func (a *App) pauseLauncherInput() {
	if a.pad != nil {
		a.pad.Pause()
	}
	a.pauseKeyboardInput()
}

func (a *App) resumeLauncherInput() {
	if a.pad != nil {
		a.pad.Resume()
	}
	a.resumeKeyboardInput()
}

func (a *App) LoadGames() map[uuid.UUID]*games.GameData {
	lgx.Infof("loading games from %q", a.config.GamesRoot)
	if strings.TrimSpace(a.config.GamesRoot) == "" {
		lgx.Errorf("gamesRoot is empty; check config.json in the launcher working directory")
	} else if info, err := os.Stat(a.config.GamesRoot); err != nil {
		lgx.Errorf("gamesRoot is not readable path=%q err=%v", a.config.GamesRoot, err)
	} else if !info.IsDir() {
		lgx.Errorf("gamesRoot is not a directory path=%q", a.config.GamesRoot)
	}

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
			lgx.Errorf("game load error path=%q err=%v", gamePath, err)
			runtime.EventsEmit(a.ctx, "game:load-error", map[string]any{
				"path":  gamePath,
				"error": err.Error(),
			})
		},
	})

	a.gamesMu.Lock()
	a.games = loaded
	a.gamesMu.Unlock()
	lgx.Infof("loaded games count=%d", len(loaded))

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

func (a *App) beginGameSession(name string) bool {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.gameSessionActive {
		lgx.Debugf("launch ignored name=%q: another session is active", name)
		return false
	}
	if !controller.LauncherHasFocus() {
		lgx.Debugf("launch ignored name=%q: StageBox is not foreground", name)
		return false
	}
	a.gameSessionActive = true
	return true
}

func (a *App) endGameSession() {
	a.gameSessionMu.Lock()
	a.gameSessionActive = false
	a.gameSessionMu.Unlock()
}

func (a *App) Launch(id uuid.UUID) {
	a.gamesMu.RLock()
	game := a.games[id]
	a.gamesMu.RUnlock()
	if game == nil {
		lgx.Warnf("launch requested for missing game id=%s", id)
		return
	}
	if !a.beginGameSession(game.Name) {
		return
	}

	gameKeyMapActive := false
	a.pauseKeyboardInput()
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

	lgx.Infof("launch requested id=%s name=%q exe=%q dir=%q args=%q trackMode=%q advancedTracking=%v waitAfterExit=%.2f showCursor=%v keyMap=%v delayedKey=%v",
		game.ID, game.Name, game.Executable, game.StartsOn, game.Args, game.TrackMode, game.AdvancedTracking, game.WaitAfterExitSeconds, game.ShowCursor, game.KeyMap != nil, game.DelayedKey != nil)

	// Tell the frontend to pause gamepad loops, overlays, etc.
	runtime.EventsEmit(a.ctx, "game:start", map[string]any{
		"id":   game.ID.String(),
		"name": game.Name,
	})

	runtime.WindowHide(a.ctx)

	run := games.LaunchGame(game)
	lgx.Debugf("launch session created id=%s name=%q lock=%q trackMode=%s", game.ID, game.Name, run.LockPath, run.TrackMode)
	if game.DelayedKey != nil || gameKeyMapActive {
		a.handleGameProcessStarted(game, run.PID, run.Exited, gameKeyMapActive)
	} else {
		a.focusGameAtStartup(run.PID)
	}

	// Don't block the call; wait in background
	go func() {
		defer a.endGameSession()
		if game.ShowCursor {
			cursor.ForceCursorVisible()
		} else {
			cursor.ForceCursorHiddenFor(2 * time.Second)
		}

		err := <-run.Done
		if err != nil {
			lgx.Errorf("launch session ended with error id=%s name=%q err=%v", game.ID, game.Name, err)
			runtime.EventsEmit(a.ctx, "game:launch-error", map[string]any{
				"id":    game.ID.String(),
				"name":  game.Name,
				"error": err.Error(),
			})
		} else {
			lgx.Infof("launch session ended id=%s name=%q", game.ID, game.Name)
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
		a.resumeKeyboardInput()
	}()
}

func (a *App) handleGameProcessStarted(game *games.GameData, pidCh <-chan int, exited <-chan struct{}, gameKeyMapActive bool) {
	go func() {
		pid, ok := <-pidCh
		if !ok || pid <= 0 {
			return
		}

		go a.focusGamePIDFor(uint32(pid), 6*time.Second)
		if gameKeyMapActive && a.pad != nil {
			a.pad.SetGameInputTargetPID(uint32(pid))
			lgx.Debugf("game key-map target pid=%d", pid)
		}

		if game.DelayedKey != nil {
			a.scheduleDelayedKeyForPID(game, uint32(pid), exited)
		}
	}()
}

func (a *App) focusGameAtStartup(pidCh <-chan int) {
	go func() {
		pid, ok := <-pidCh
		if !ok || pid <= 0 {
			return
		}
		a.focusGamePIDFor(uint32(pid), 6*time.Second)
	}()
}

func (a *App) focusGamePIDFor(pid uint32, duration time.Duration) {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if controller.FocusProcessWindow(pid) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (a *App) scheduleDelayedKeyForPID(game *games.GameData, pid uint32, exited <-chan struct{}) {
	delayed := game.DelayedKey
	if delayed == nil || delayed.Key == "" || delayed.Seconds < 0 {
		return
	}

	go func() {
		timer := time.NewTimer(time.Duration(delayed.Seconds * float64(time.Second)))
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-exited:
			lgx.Debugf("delayed key skipped for %s: game exited before %.2fs delay", game.Name, delayed.Seconds)
			return
		}

		if !controller.SendKeyStrokeToPID(pid, delayed.Key) {
			lgx.Warnf("delayed key skipped for %s: unsupported key %q", game.Name, delayed.Key)
			return
		}
		lgx.Debugf("delayed key sent for %s: key=%q delay=%.2fs", game.Name, delayed.Key, delayed.Seconds)
	}()
}

func (a *App) KillRunning() error {
	if err := games.KillRunning(); err != nil {
		lgx.Warnf("kill running game failed: %v", err)
		return err
	}
	lgx.Infof("kill running game requested")
	return nil
}

func (a *App) Explorer() {
	exec.Command("explorer.exe", a.config.GamesRoot).Start()
}

func (a *App) GetStatuses() map[string]any {
	st, connected, count, wifiSt := a.readHardwareStatus()
	return a.statusPayload(st, connected, count, wifiSt)
}

func (a *App) watchBluetooth() {
	lgx.Info("Starting Bluetooth state watcher")

	// initial emit
	s, connected, count, wifiSt := a.readHardwareStatus()
	a.btStatus = s
	a.btConnected = connected
	a.controllerCount = count
	a.wifiStatus = wifiSt
	runtime.EventsEmit(a.ctx, "status:update", a.statusPayload(s, connected, count, wifiSt))

	t := time.NewTicker(2 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			st, connected, count, wifiSt := a.readHardwareStatus()
			if st != a.btStatus || connected != a.btConnected || count != a.controllerCount || wifiSt != a.wifiStatus {
				a.btStatus = st
				a.btConnected = connected
				a.controllerCount = count
				a.wifiStatus = wifiSt
				runtime.EventsEmit(a.ctx, "status:update", a.statusPayload(st, connected, count, wifiSt))
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
	a.emitHardwareStatus()
}

func (a *App) emitHardwareStatus() {
	st, connected, count, wifiSt := a.readHardwareStatus()
	runtime.EventsEmit(a.ctx, "status:update", a.statusPayload(st, connected, count, wifiSt))
}

func (a *App) readHardwareStatus() (string, bool, int, wifi.Status) {
	st := bluetooth.State()
	count := controller.ConnectedCount()
	return st, st == "on" && bluetooth.AnyConnectedDevice(), count, wifi.CurrentStatus()
}

func (a *App) statusPayload(bt string, btConnected bool, controllerCount int, wifiSt wifi.Status) map[string]any {
	return map[string]any{
		"bt":              bt,
		"btConnected":     btConnected,
		"controllerCount": controllerCount,
		"wifiAvailable":   wifiSt.Available,
		"wifiConnected":   wifiSt.Connected,
		"wifiSSID":        wifiSt.SSID,
		"wifiState":       wifiSt.State,
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
