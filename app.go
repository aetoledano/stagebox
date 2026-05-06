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
	"os/exec"
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
	thumbnailDir       string
	thumbnailURLPrefix string
	pad                *controller.XInputBridge
	btStatus           string
	autoPairRunning    bool
	autoPairStatus     string
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

	exec.Command("shutdown", "-s", "-t", "0").Start()
	runtime.Quit(a.ctx)

	log.Println("shutting down...")
}

func (a *App) Restart() {
	if a.pad != nil {
		a.pad.Stop()
	}

	exec.Command("shutdown", "-r", "-t", "0").Start()
	runtime.Quit(a.ctx)

	log.Println("restarting...")
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

	if a.pad != nil {
		a.pad.Pause()
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
		if !game.ShowCursor {
			cursor.SetCursorHidden(true)
		}
		defer cursor.SetCursorHidden(false)

		err := <-ch
		if err != nil {
			log.Printf("Error launching game %s: %v", game.Name, err)
		} else {
			log.Printf("Game %s exited", game.Name)
		}

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
	return map[string]any{
		"bt":             bluetooth.State(),
		"autopair":       bluetooth.AutoPairRunning(),
		"autopairStatus": bluetooth.AutoPairStatus(),
	}
}

func (a *App) watchBluetooth() {
	lgx.Info("Starting Bluetooth state watcher")

	// initial emit
	s := bluetooth.State()
	apRunning := bluetooth.AutoPairRunning()
	apStatus := bluetooth.AutoPairStatus()
	a.btStatus = s
	a.autoPairRunning = apRunning
	a.autoPairStatus = apStatus
	runtime.EventsEmit(a.ctx, "status:update", map[string]any{
		"bt":             s,
		"autopair":       apRunning,
		"autopairStatus": apStatus,
	})

	t := time.NewTicker(2 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			st := bluetooth.State()
			apRunning := bluetooth.AutoPairRunning()
			apStatus := bluetooth.AutoPairStatus()
			if st != a.btStatus || apRunning != a.autoPairRunning || apStatus != a.autoPairStatus {
				a.btStatus = st
				a.autoPairRunning = apRunning
				a.autoPairStatus = apStatus
				runtime.EventsEmit(a.ctx, "status:update", map[string]any{
					"bt":             st,
					"autopair":       apRunning,
					"autopairStatus": apStatus,
				})
			}
		case <-a.ctx.Done():
			return
		}
	}
}

// in app.go
func (a *App) AutoPairStart(whitelist string) error {
	lgx.Infof("AutoPairStart called with whitelist=%q", whitelist)
	err := bluetooth.AutoPairStart(whitelist)
	if err != nil {
		lgx.Errorf("AutoPairStart error: %v", err)
		return err
	}
	runtime.EventsEmit(a.ctx, "status:update", map[string]any{
		"autopair":       true,
		"autopairStatus": bluetooth.AutoPairStatus(),
	})
	lgx.Infof("AutoPairStart OK (loop running)")
	return nil
}

func (a *App) AutoPairStop() {
	bluetooth.AutoPairStop()
	runtime.EventsEmit(a.ctx, "status:update", map[string]any{
		"autopair":       false,
		"autopairStatus": bluetooth.AutoPairStatus(),
	})
}

func (a *App) AutoPairRunning() bool { return bluetooth.AutoPairRunning() }

// app.go
func (a *App) EnsureBT() {
	bluetooth.EnsureUp()
	// emit a fresh status so UI updates
	runtime.EventsEmit(a.ctx, "status:update", map[string]any{
		"bt": bluetooth.State(),
	})
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
