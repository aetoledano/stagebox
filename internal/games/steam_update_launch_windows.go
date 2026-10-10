//go:build windows

package games

import (
	"fmt"
	lgx "launcher/internal/logx"
	"os/exec"
	"path/filepath"
)

func requestSteamGameUpdate(game *GameData) error {
	stopSteamUIBackgrounding()
	cmd := exec.Command(game.Executable, steamGameUpdateArgs(game.SteamAppID)...)
	cmd.Dir = filepath.Dir(game.Executable)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("request Steam update: %w", err)
	}
	lgx.Infof("Steam update-only requested appID=%d requestPID=%d; game launch deferred", game.SteamAppID, cmd.Process.Pid)
	// Never bind this singleton request to the wait's cancellation context.
	go func() {
		if err := cmd.Wait(); err != nil {
			lgx.Warnf("Steam update activation exited appID=%d err=%v", game.SteamAppID, err)
		}
	}()
	return nil
}
