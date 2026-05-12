package main

import (
	"context"
	"fmt"
	"io"
	"launcher/internal/games"
	"launcher/internal/platform/bluetooth"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"launcher/internal/platform/display"
	"launcher/internal/platform/shell"
	"launcher/internal/platform/wallpaper"
	"launcher/internal/platform/wifi"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"launcher/internal/config"
	lgx "launcher/internal/logx"
)

const (
	steamInstallerURL = "https://cdn.akamai.steamstatic.com/client/installer/SteamSetup.exe"
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
	loadingWallpaper   string
	pad                *controller.XInputBridge
	keyboard           *controller.KeyboardBridge
	btStatus           string
	btConnected        bool
	controllerCount    int
	wifiStatus         wifi.Status
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

	_ = display.ForcePrimary1080pOrBest()

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

	a.pad = controller.StartXInputBridge(ctx)
	a.keyboard = controller.StartKeyboardBridge(ctx)

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

// ... actions ...
func (a *App) Shutdown() {
	a.stopLauncherInput()

	cursor.RestoreSystemCursor()
	exec.Command("shutdown", "-s", "-t", "0").Start()
	runtime.Quit(a.ctx)

	log.Println("shutting down...")
}

func (a *App) Restart() {
	a.stopLauncherInput()

	cursor.RestoreSystemCursor()
	exec.Command("shutdown", "-r", "-t", "0").Start()
	runtime.Quit(a.ctx)

	log.Println("restarting...")
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

func (a *App) Launch(id uuid.UUID) {
	a.gamesMu.RLock()
	game := a.games[id]
	a.gamesMu.RUnlock()
	if game == nil {
		log.Printf("Game with ID %s not found", id)
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

	log.Printf("Launching game: %s", game.Name)

	// Tell the frontend to pause gamepad loops, overlays, etc.
	runtime.EventsEmit(a.ctx, "game:start", map[string]any{
		"id":   game.ID.String(),
		"name": game.Name,
	})

	runtime.WindowHide(a.ctx)

	run := games.LaunchGame(game)
	if game.DelayedKey != nil || gameKeyMapActive {
		a.handleGameProcessStarted(game, run.PID, run.Exited, gameKeyMapActive)
	} else {
		a.focusGameAtStartup(run.PID)
	}

	// Don't block the call; wait in background
	go func() {
		if game.ShowCursor {
			cursor.ForceCursorVisible()
		} else {
			cursor.ForceCursorHiddenFor(2 * time.Second)
		}

		err := <-run.Done
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
		a.resumeKeyboardInput()
	}()
}

func (a *App) handleGameProcessStarted(game *games.GameData, pidCh <-chan int, exited <-chan struct{}, gameKeyMapActive bool) {
	go func() {
		pid, ok := <-pidCh
		if !ok || pid <= 0 {
			return
		}

		go a.focusGamePIDFor(uint32(pid), 4*time.Second)
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
		a.focusGamePIDFor(uint32(pid), 4*time.Second)
	}()
}

func (a *App) focusGamePIDFor(pid uint32, duration time.Duration) {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		controller.FocusProcessWindow(pid)
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

// steam

func (a *App) OpenSteam() error {
	lgx.Info("OpenSteam requested")
	if steamExe := findSteamExecutable(); steamExe != "" {
		lgx.Infof("Steam executable found: %s", steamExe)
		a.launchSteamSession(steamExe)
		return nil
	}

	installer, err := ensureSteamInstaller()
	if err != nil {
		lgx.Errorf("prepare Steam installer failed: %v", err)
		return err
	}
	if err := startSteamInstaller(installer); err != nil {
		lgx.Errorf("start Steam installer failed path=%q err=%v", installer, err)
		return err
	}
	return nil
}

func (a *App) launchSteamSession(steamExe string) {
	a.pauseKeyboardInput()
	if a.pad != nil {
		a.pad.Pause()
	}

	runtime.EventsEmit(a.ctx, "game:start", map[string]any{
		"id":   "steam",
		"name": "Steam",
	})
	runtime.WindowHide(a.ctx)
	cursor.RestoreSystemCursor()
	cursor.MoveToBottomRight()

	run := games.LaunchSteamSession(steamExe)
	go func() {
		err := <-run.Done
		if err != nil {
			lgx.Errorf("Steam session ended with error: %v", err)
		} else {
			lgx.Info("Steam session ended")
		}

		cursor.RestoreSystemCursor()
		controller.MoveCursorTo(0, 0)
		cursor.SetCursorHidden(true)
		a.bringBack()

		runtime.EventsEmit(a.ctx, "game:end", map[string]any{
			"id":   "steam",
			"name": "Steam",
			"ok":   err == nil,
		})

		if a.pad != nil {
			a.pad.Resume()
		}
		a.resumeKeyboardInput()
	}()
}

func findSteamExecutable() string {
	candidates := []string{}
	add := func(path string) {
		if path == "" {
			return
		}
		candidates = append(candidates, filepath.Clean(os.ExpandEnv(path)))
	}
	addRoot := func(path string) {
		if path == "" {
			return
		}
		if strings.EqualFold(filepath.Base(path), "steam.exe") {
			add(path)
			return
		}
		add(filepath.Join(path, "Steam.exe"))
	}

	addRoot(os.Getenv("STEAM_PATH"))
	addRoot(os.Getenv("STEAM_DIR"))
	if programFilesX86 := os.Getenv("ProgramFiles(x86)"); programFilesX86 != "" {
		add(filepath.Join(programFilesX86, "Steam", "Steam.exe"))
	}
	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		add(filepath.Join(programFiles, "Steam", "Steam.exe"))
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func ensureSteamInstaller() (string, error) {
	installerDir := filepath.Join("cache", "installers")
	if err := os.MkdirAll(installerDir, 0o755); err != nil {
		return "", err
	}

	target := filepath.Join(installerDir, "SteamSetup.exe")
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	tmp := target + ".tmp"
	lgx.Infof("downloading Steam installer from %s", steamInstallerURL)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(steamInstallerURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected installer download status: %s", resp.Status)
	}

	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", err
	}
	ok = true

	if info, err := os.Stat(targetAbs); err == nil {
		lgx.Infof("Steam installer ready path=%s size=%d", targetAbs, info.Size())
	} else {
		lgx.Warnf("Steam installer downloaded but stat failed path=%s err=%v", targetAbs, err)
	}
	return targetAbs, nil
}

func startSteamInstaller(installer string) error {
	if info, err := os.Stat(installer); err != nil {
		return fmt.Errorf("installer missing: %w", err)
	} else if info.IsDir() {
		return fmt.Errorf("installer path is a directory: %s", installer)
	}

	cmd := exec.Command(installer)
	cmd.Dir = filepath.Dir(installer)
	if err := cmd.Start(); err != nil {
		lgx.Warnf("direct Steam installer start failed path=%s err=%v; trying Explorer fallback", installer, err)
		fallback := exec.Command("explorer.exe", installer)
		if fallbackErr := fallback.Start(); fallbackErr != nil {
			return fmt.Errorf("direct start failed: %v; explorer fallback failed: %w", err, fallbackErr)
		}
		if fallback.Process != nil {
			lgx.Infof("started Steam installer through Explorer fallback pid=%d path=%s", fallback.Process.Pid, installer)
		} else {
			lgx.Infof("started Steam installer through Explorer fallback path=%s", installer)
		}
		return nil
	}
	if cmd.Process != nil {
		lgx.Infof("started Steam installer pid=%d path=%s", cmd.Process.Pid, installer)
	} else {
		lgx.Infof("started Steam installer path=%s", installer)
	}
	return nil
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
