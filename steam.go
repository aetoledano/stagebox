package main

import (
	"context"
	"fmt"
	"io"
	"launcher/internal/games"
	lgx "launcher/internal/logx"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"launcher/internal/platform/network"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	steamInstallerURL = "https://cdn.akamai.steamstatic.com/client/installer/SteamSetup.exe"
)

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
	if !a.beginGameSession("Steam") {
		return
	}
	if a.pad != nil && a.pad.NeedsVirtualInput(false) {
		runtime.EventsEmit(a.ctx, "game:launch-begin", map[string]any{"id": "steam", "name": "Steam"})
		if err := a.prepareSessionVirtualInput("steam", "Steam", false); err != nil {
			a.endGameSession()
			lgx.Warnf("Steam controller setup blocked: %v", err)
			runtime.EventsEmit(a.ctx, "game:launch-blocked", map[string]any{"id": "steam", "name": "Steam", "error": err.Error()})
			return
		}
	}
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
		defer a.endGameSession()
		err := <-run.Done
		if uiErr := games.BackgroundSteamClient(a.ctx, steamExe); uiErr != nil {
			lgx.Warnf("Steam UI cleanup before launcher return failed: %v", uiErr)
		}
		err = a.finishSessionVirtualInput("Steam", err)
		if err != nil {
			lgx.Errorf("Steam session ended with error: %v", err)
			runtime.EventsEmit(a.ctx, "game:launch-error", map[string]any{"id": "steam", "name": "Steam", "error": err.Error()})
		} else {
			lgx.Info("Steam session ended")
		}

		cursor.RestoreSystemCursor()
		controller.MoveCursorTo(0, 0)
		cursor.SetCursorHidden(true)
		a.endGameLaunchFeedback("steam", "Steam", err == nil)
		a.bringBack()
		a.focusAfterSteamSession()

		if a.pad != nil {
			a.pad.Resume()
		}
		a.resumeKeyboardInput()
	}()
}

func findSteamExecutable() string {
	return games.FindSteamExecutable()
}

func (a *App) prepareSteamCard(game *games.GameData) (*games.GameData, bool, error) {
	runtime.EventsEmit(a.ctx, "game:launch-status", map[string]any{"id": game.ID.String(), "name": game.Name,
		"stage": "checking", "message": "Checking internet connection…", "progress": -1})
	prepared, err := games.PrepareSteamGame(game)
	if err != nil {
		return nil, false, err
	}
	if !network.InternetAvailable(a.ctx) {
		return nil, false, fmt.Errorf("An internet connection is required to run this Steam game. Connect to the internet and try again.")
	}
	if !controller.LauncherHasFocus() {
		return nil, false, fmt.Errorf("Return to StageBox before launching the game.")
	}
	runtime.EventsEmit(a.ctx, "game:update-status", map[string]any{"id": prepared.ID.String(), "needsUpdate": prepared.SteamNeedsUpdate})
	updateOnly := false
	if prepared.SteamNeedsUpdate {
		choice, err := a.chooseSteamLaunch(prepared)
		if err != nil {
			return nil, false, err
		}
		if !controller.LauncherHasFocus() {
			return nil, false, fmt.Errorf("Return to StageBox before launching the game.")
		}
		updateOnly = choice == "update"
	}
	runtime.EventsEmit(a.ctx, "game:launch-status", map[string]any{"id": prepared.ID.String(), "name": prepared.Name,
		"stage": "steam-opening", "message": "Launching Steam…", "progress": -1})
	return prepared, updateOnly, nil
}

func (a *App) chooseSteamLaunch(game *games.GameData) (string, error) {
	choice := make(chan string, 1)
	a.gameSessionMu.Lock()
	a.steamLaunchID, a.steamLaunchChoice = game.ID, choice
	a.gameSessionMu.Unlock()
	defer func() {
		a.gameSessionMu.Lock()
		a.steamLaunchChoice = nil
		a.gameSessionMu.Unlock()
	}()
	message := "Choose Open in Steam to update and launch this game in Big Picture. To download while playing something else, choose Update in Steam, select Steam's Update button, then exit Big Picture."
	runtime.EventsEmit(a.ctx, "game:launch-status", map[string]any{"id": game.ID.String(), "name": game.Name,
		"stage": "setup", "message": message, "progress": -1, "canOpenSteam": true, "canUpdateInSteam": true, "canGoBack": true})
	select {
	case action := <-choice:
		if action == "back" {
			return "", context.Canceled
		}
		return action, nil
	case <-a.ctx.Done():
		return "", a.ctx.Err()
	}
}

// SteamLaunchAction accepts one current update-panel choice. Game cards all
// use the same combined Steam activation; there is no live launch rescue.
func (a *App) SteamLaunchAction(id uuid.UUID, action string) error {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if !a.gameSessionActive || id != a.steamLaunchID || a.steamLaunchChoice == nil {
		return fmt.Errorf("This Steam launch choice is no longer active")
	}
	if action != "steam" && action != "update" && action != "back" {
		return fmt.Errorf("That action is unavailable for this game")
	}
	select {
	case a.steamLaunchChoice <- action:
		a.steamLaunchChoice = nil
		return nil
	default:
		return fmt.Errorf("A Steam launch action is already pending")
	}
}

func (a *App) SteamUpdateStates() map[string]bool {
	a.gamesMu.RLock()
	steamGames := make([]*games.GameData, 0)
	for _, game := range a.games {
		if game.SteamAppID != 0 {
			steamGames = append(steamGames, game)
		}
	}
	a.gamesMu.RUnlock()
	states := map[string]bool{}
	for _, game := range steamGames {
		if needed, err := games.SteamUpdateNeeded(game); err == nil {
			states[game.ID.String()] = needed
		}
	}
	return states
}

func (a *App) focusAfterSteamSession() {
	deadline := time.Now().Add(2 * time.Second)
	var focusedSince time.Time
	for {
		if controller.LauncherHasFocus() {
			if focusedSince.IsZero() {
				focusedSince = time.Now()
			}
			if time.Since(focusedSince) >= 400*time.Millisecond {
				lgx.Info("launcher focus reclaimed after Steam session")
				return
			}
		} else {
			focusedSince = time.Time{}
			controller.FocusLauncherWindow()
		}
		if time.Now().After(deadline) || a.ctx.Err() != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	lgx.Warn("launcher is visible but foreground focus could not be reclaimed after Steam session")
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
