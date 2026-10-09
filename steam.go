package main

import (
	"fmt"
	"io"
	"launcher/internal/games"
	lgx "launcher/internal/logx"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
