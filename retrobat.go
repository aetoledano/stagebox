package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"launcher/internal/games"
	lgx "launcher/internal/logx"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	stdRuntime "runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const retroBatReleaseURL = "https://api.github.com/repos/RetroBat-Official/retrobat/releases/latest"

type retroBatRelease struct {
	Draft      bool                     `json:"draft"`
	Prerelease bool                     `json:"prerelease"`
	Assets     []retroBatInstallerAsset `json:"assets"`
}

type retroBatInstallerAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

func (a *App) installRetroBat() error {
	if stdRuntime.GOOS != "windows" {
		err := fmt.Errorf("RetroBat installation is only supported on Windows")
		runtime.EventsEmit(a.ctx, "game:launch-blocked", map[string]any{"id": "retrobat", "name": "RetroBat", "error": err.Error()})
		return err
	}
	runtime.EventsEmit(a.ctx, "game:launch-begin", map[string]any{"id": "retrobat", "name": "RetroBat"})
	status := func(message string, progress int) {
		runtime.EventsEmit(a.ctx, "game:launch-status", map[string]any{
			"id": "retrobat", "name": "RetroBat", "stage": "installer", "message": message, "progress": progress,
		})
	}
	status("Finding the RetroBat installer…", -1)
	installer, err := ensureRetroBatInstaller(a.ctx, func(progress int) { status("Downloading RetroBat installer…", progress) })
	if err == nil {
		status("Opening RetroBat setup…", -1)
		cursor.ForceCursorVisible()
		err = startRetroBatInstaller(installer)
		if err != nil {
			cursor.SetCursorHidden(true)
		}
	}
	if a.ctx.Err() != nil {
		return a.ctx.Err()
	}
	if err != nil {
		lgx.Errorf("RetroBat installer failed: %v", err)
		runtime.EventsEmit(a.ctx, "game:launch-error", map[string]any{"id": "retrobat", "name": "RetroBat", "error": err.Error()})
	}
	// Like Steam's missing-client flow, setup runs independently of game tracking.
	// Leave StageBox visible; foreground gating keeps navigation out of setup.
	a.endGameLaunchFeedback("retrobat", "RetroBat", err == nil)
	return err
}

func ensureRetroBatInstaller(ctx context.Context, onProgress func(int)) (string, error) {
	metadataCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(metadataCtx, http.MethodGet, retroBatReleaseURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "StageBox")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("find official RetroBat installer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("find official RetroBat installer: %s", response.Status)
	}
	var release retroBatRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release); err != nil {
		return "", fmt.Errorf("read RetroBat release: %w", err)
	}
	asset, err := selectRetroBatInstaller(release)
	if err != nil {
		return "", err
	}
	return downloadRetroBatInstaller(ctx, asset, filepath.Join("cache", "installers", "RetroBatSetup.exe"), onProgress)
}

func selectRetroBatInstaller(release retroBatRelease) (retroBatInstallerAsset, error) {
	if !release.Draft && !release.Prerelease {
		for _, asset := range release.Assets {
			name := strings.ToLower(asset.Name)
			if !strings.HasPrefix(name, "retrobat-") || !strings.HasSuffix(name, "-win64-setup.exe") || asset.Size <= 0 {
				continue
			}
			u, err := url.Parse(asset.URL)
			if err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil &&
				strings.HasPrefix(u.Path, "/RetroBat-Official/retrobat/releases/download/") {
				return asset, nil
			}
		}
	}
	return retroBatInstallerAsset{}, fmt.Errorf("The official RetroBat release has no stable Windows 64-bit installer")
}

func downloadRetroBatInstaller(ctx context.Context, asset retroBatInstallerAsset, target string, onProgress func(int)) (string, error) {
	if asset.Size <= 0 {
		return "", fmt.Errorf("RetroBat installer has no valid download size")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	lgx.Infof("downloading RetroBat installer name=%q url=%s size=%d", asset.Name, asset.URL, asset.Size)
	client := &http.Client{Timeout: 30 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download RetroBat installer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download RetroBat installer: %s", response.Status)
	}
	out, err := os.CreateTemp(filepath.Dir(target), "RetroBatSetup-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	progress := &retroBatDownloadProgress{total: asset.Size, notify: onProgress}
	if onProgress != nil {
		onProgress(0)
	}
	n, err := io.Copy(io.MultiWriter(out, progress), io.LimitReader(response.Body, asset.Size+1))
	if err != nil {
		return "", fmt.Errorf("save RetroBat installer: %w", err)
	}
	if n != asset.Size {
		return "", fmt.Errorf("RetroBat installer download was incomplete: received %d of %d bytes", n, asset.Size)
	}
	// A successful HTTP response containing an error page must never be executed.
	var header [2]byte
	if _, err := out.ReadAt(header[:], 0); err != nil || string(header[:]) != "MZ" {
		return "", fmt.Errorf("RetroBat download is not a Windows executable")
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(out.Name(), target); err != nil {
		return "", err
	}
	lgx.Infof("RetroBat installer ready path=%s size=%d", target, n)
	return target, nil
}

type retroBatDownloadProgress struct {
	total, received int64
	lastUpdate      time.Time
	notify          func(int)
}

func (p *retroBatDownloadProgress) Write(data []byte) (int, error) {
	p.received += int64(len(data))
	if p.notify != nil && (time.Since(p.lastUpdate) >= time.Second || p.received >= p.total) {
		p.lastUpdate = time.Now()
		p.notify(int(min(100, p.received*100/p.total)))
	}
	return len(data), nil
}

func startRetroBatInstaller(installer string) error {
	if info, err := os.Stat(installer); err != nil {
		return fmt.Errorf("RetroBat installer missing: %w", err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("RetroBat installer is not a regular file: %s", installer)
	}
	cmd := exec.Command(installer)
	cmd.Dir = filepath.Dir(installer)
	if err := cmd.Start(); err != nil {
		lgx.Warnf("direct RetroBat installer start failed: %v; trying Explorer fallback", err)
		fallback := exec.Command("explorer.exe", installer)
		if fallbackErr := fallback.Start(); fallbackErr != nil {
			return fmt.Errorf("RetroBat installer direct start failed: %v; Explorer fallback failed: %w", err, fallbackErr)
		}
		lgx.Infof("started RetroBat installer through Explorer pid=%d path=%s", fallback.Process.Pid, installer)
		go func() { _ = fallback.Wait() }()
		return nil
	}
	lgx.Infof("started RetroBat installer pid=%d path=%s", cmd.Process.Pid, installer)
	go func() { _ = cmd.Wait() }()
	return nil
}

func (a *App) OpenRetroBat() error {
	lgx.Info("OpenRetroBat requested")
	if !a.beginGameSession("RetroBat") {
		return nil
	}
	exe, err := games.FindRetroBatExecutable(a.config.RetroBatPath)
	if errors.Is(err, games.ErrRetroBatNotFound) {
		defer a.endGameSession()
		return a.installRetroBat()
	}
	if err != nil {
		a.endGameSession()
		lgx.Errorf("RetroBat discovery failed: %v", err)
		runtime.EventsEmit(a.ctx, "game:launch-blocked", map[string]any{"id": "retrobat", "name": "RetroBat", "error": err.Error()})
		return err
	}
	lgx.Infof("RetroBat executable found: %s", exe)
	if a.pad != nil && a.pad.NeedsVirtualInput(false) {
		runtime.EventsEmit(a.ctx, "game:launch-begin", map[string]any{"id": "retrobat", "name": "RetroBat"})
		if err := a.prepareSessionVirtualInput("retrobat", "RetroBat", false); err != nil {
			a.endGameSession()
			lgx.Warnf("RetroBat controller setup blocked: %v", err)
			runtime.EventsEmit(a.ctx, "game:launch-blocked", map[string]any{"id": "retrobat", "name": "RetroBat", "error": err.Error()})
			return err
		}
	}
	a.pauseKeyboardInput()
	if a.pad != nil {
		a.pad.Pause()
	}
	runtime.EventsEmit(a.ctx, "game:start", map[string]any{"id": "retrobat", "name": "RetroBat"})
	runtime.WindowHide(a.ctx)
	cursor.RestoreSystemCursor()
	cursor.MoveToBottomRight()

	go func() {
		defer a.endGameSession()
		run, err := games.LaunchRetroBatSession(a.ctx, exe)
		if err == nil {
			err = <-run.Done
		}
		err = a.finishSessionVirtualInput("RetroBat", err)
		if a.ctx.Err() != nil {
			return
		}
		if err != nil {
			lgx.Errorf("RetroBat session failed: %v", err)
			runtime.EventsEmit(a.ctx, "game:launch-error", map[string]any{"id": "retrobat", "name": "RetroBat", "error": err.Error()})
		}
		cursor.RestoreSystemCursor()
		controller.MoveCursorTo(0, 0)
		cursor.SetCursorHidden(true)
		a.endGameLaunchFeedback("retrobat", "RetroBat", err == nil)
		a.bringBack()
		// Settle native foreground ownership before accepting navigation.
		deadline := time.Now().Add(2 * time.Second)
		var focusedSince time.Time
		for time.Now().Before(deadline) && a.ctx.Err() == nil {
			if controller.LauncherHasFocus() {
				if focusedSince.IsZero() {
					focusedSince = time.Now()
				}
				if time.Since(focusedSince) >= 400*time.Millisecond {
					break
				}
			} else {
				focusedSince = time.Time{}
				controller.FocusLauncherWindow()
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !controller.LauncherHasFocus() {
			lgx.Warn("launcher foreground focus was not reclaimed after RetroBat session")
		}
		if a.pad != nil {
			a.pad.Resume()
		}
		a.resumeKeyboardInput()
	}()
	return nil
}
