package main

import (
	"context"
	"launcher/internal/games"
	"launcher/tools"
	"log"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"launcher/internal/config"
	lgx "launcher/internal/logx"
)

// App struct
type App struct {
	ctx      context.Context
	config   *config.Config
	games    map[uuid.UUID]*games.GameData
	pad      *tools.XInputBridge
	btStatus string
}

// NewApp creates a new App application struct
func NewApp(config *config.Config) *App {
	return &App{
		config: config,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Make this launcher the per-user shell every time we start (Windows only).
	if err := tools.EnsureSelfAsShell(); err != nil {
		// don’t crash on failure; just log
		println("ensureSelfAsShell failed:", err.Error())
	}

	_ = tools.ForcePrimary1080pOrBest()

	// Start the bottom-most wallpaper host
	if stop, err := tools.StartWallpaper("loading.jpg", "cover"); err == nil {
		// ensure it is torn down cleanly
		// if you already have a global cleanup, call stop() there instead
		go func() { <-ctx.Done(); stop() }() // on app exit
	} else {
		println("wallpaper host failed:", err.Error())
	}

	a.pad = tools.StartXInputBridge(ctx)

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

	runtime.Quit(a.ctx)

	log.Println("shutting down...")

	exec.Command("shutdown", "-s", "-t", "0").Start()
}

func (a *App) Restart() {
	if a.pad != nil {
		a.pad.Stop()
	}

	runtime.Quit(a.ctx)

	log.Println("restarting...")

	exec.Command("shutdown", "-r", "-t", "0").Start()
}

func (a *App) LoadGames() map[uuid.UUID]*games.GameData {
	log.Println("Loading games from:", a.config.GamesRoot)

	a.games = games.LoadGames(a.config.GamesRoot)

	return a.games
}

func (a *App) Launch(id uuid.UUID) {
	game := a.games[id]
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
			tools.SetCursorHidden(true)
		}
		defer tools.SetCursorHidden(false)

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
		"bt":       tools.BluetoothState(),
		"autopair": tools.AutoPairRunning(),
		// "mapper": tools.MapperRunning(), // if you kept it
	}
}

func (a *App) watchBluetooth() {
	lgx.Info("Starting Bluetooth state watcher")

	// initial emit
	s := tools.BluetoothState()
	a.btStatus = s
	runtime.EventsEmit(a.ctx, "status:update", map[string]any{"bt": s})

	t := time.NewTicker(2 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			st := tools.BluetoothState()
			if st != a.btStatus {
				a.btStatus = st
				runtime.EventsEmit(a.ctx, "status:update", map[string]any{"bt": st})
			}
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) AutoPairStart(whitelist string) error {
	err := tools.AutoPairStart(whitelist)
	if err == nil {
		// optional: ping UI
		runtime.EventsEmit(a.ctx, "status:update", map[string]any{"autopair": true})
	}
	return err
}

func (a *App) AutoPairStop() {
	tools.AutoPairStop()
	runtime.EventsEmit(a.ctx, "status:update", map[string]any{"autopair": false})
}

func (a *App) AutoPairRunning() bool { return tools.AutoPairRunning() }

// app.go
func (a *App) EnsureBT() {
	tools.EnsureBluetoothUp()
	// emit a fresh status so UI updates
	runtime.EventsEmit(a.ctx, "status:update", map[string]any{
		"bt": tools.BluetoothState(),
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
