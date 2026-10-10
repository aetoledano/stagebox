//go:build !windows

package games

import (
	"errors"
	"time"
)

func startSteamGameProcess(game *GameData, _ func(SteamLaunchStatus)) (gameProcess, error) {
	return nil, errors.New("Steam game launching is only implemented on Windows")
}

func steamInstallActive(directory string) bool                   { return false }
func steamLaunchOwnerActive(pid int, lockCreated time.Time) bool { return false }
