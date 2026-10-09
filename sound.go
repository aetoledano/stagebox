package main

import (
	"fmt"
	lgx "launcher/internal/logx"
	"launcher/internal/platform/audio"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/cursor"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) publishSoundLocked(status audio.Status, feedback bool, err error) {
	message := ""
	if err != nil {
		message = err.Error()
		if message != a.soundError {
			lgx.Warnf("sound status failed: %v", err)
		}
	}
	if !a.soundKnown || a.soundStatus != status || a.soundError != message || feedback {
		a.soundStatus, a.soundError, a.soundKnown = status, message, true
		runtime.EventsEmit(a.ctx, "sound:update", map[string]any{"status": status, "feedback": feedback, "error": message})
	}
}

func (a *App) GetAudioStatus() (audio.Status, error) {
	a.soundMu.Lock()
	defer a.soundMu.Unlock()
	status, err := audio.CurrentStatus()
	a.publishSoundLocked(status, false, err)
	return status, err
}

func (a *App) GetSoundSettings() (audio.Settings, error) {
	a.soundMu.Lock()
	defer a.soundMu.Unlock()
	settings, err := audio.GetSettings()
	a.publishSoundLocked(settings.Status, false, err)
	if err != nil {
		lgx.Errorf("get sound settings failed: %v", err)
	}
	return settings, err
}

func (a *App) SetSoundOutput(id string) (audio.Settings, error) {
	a.soundMu.Lock()
	defer a.soundMu.Unlock()
	lgx.Infof("select sound output requested id=%q", id)
	if err := audio.SetOutput(id); err != nil {
		lgx.Errorf("select sound output failed id=%q err=%v", id, err)
		return audio.Settings{}, err
	}
	var settings audio.Settings
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		settings, err = audio.GetSettings()
		if err != nil || settings.Status.DeviceID == id {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err == nil && settings.Status.DeviceID != id {
		err = fmt.Errorf("Windows did not activate the selected sound output")
	}
	a.publishSoundLocked(settings.Status, false, err)
	if err == nil {
		lgx.Infof("sound output selected id=%q name=%q", id, settings.Status.DeviceName)
	}
	return settings, err
}

func (a *App) AdjustAudioVolume(direction int) (audio.Status, error) {
	a.soundMu.Lock()
	defer a.soundMu.Unlock()
	status, err := audio.ChangeVolume(direction)
	a.publishSoundLocked(status, true, err)
	return status, err
}

func (a *App) SetAudioVolume(level int) (audio.Status, error) {
	a.soundMu.Lock()
	defer a.soundMu.Unlock()
	status, err := audio.SetVolume(level)
	a.publishSoundLocked(status, true, err)
	return status, err
}

func (a *App) ToggleAudioMute() (audio.Status, error) {
	a.soundMu.Lock()
	defer a.soundMu.Unlock()
	status, err := audio.ToggleMute()
	a.publishSoundLocked(status, true, err)
	return status, err
}

func (a *App) SetSoundSettingsOpen(open bool) {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.gameSessionActive {
		return
	}
	if open {
		cursor.ForceCursorVisible()
	} else {
		controller.MoveCursorTo(0, 0)
		cursor.SetCursorHidden(true)
	}
}

func (a *App) queueSoundInput(action string) {
	select {
	case a.soundInput <- action:
	default:
	}
}

func (a *App) handleSoundInput() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case action := <-a.soundInput:
			a.soundMu.Lock()
			focused := controller.LauncherHasFocus()
			global := strings.HasPrefix(action, "media-")
			if !global && !focused {
				a.soundMu.Unlock()
				continue
			}
			action = strings.TrimPrefix(action, "media-")
			var status audio.Status
			var err error
			switch action {
			case "up":
				status, err = audio.ChangeVolume(1)
			case "down":
				status, err = audio.ChangeVolume(-1)
			case "mute":
				status, err = audio.ToggleMute()
			}
			a.publishSoundLocked(status, focused, err)
			a.soundMu.Unlock()
		}
	}
}

func (a *App) watchSound() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	a.GetAudioStatus()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.GetAudioStatus()
		}
	}
}
