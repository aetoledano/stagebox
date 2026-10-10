//go:build !windows

package games

import "fmt"

func requestSteamGameUpdate(game *GameData) error {
	return fmt.Errorf("Steam game updates are only implemented on Windows")
}
