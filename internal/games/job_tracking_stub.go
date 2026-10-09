//go:build !windows

package games

import "errors"

func startAdvancedGameProcess(game *GameData) (gameProcess, error) {
	return nil, errors.New("advancedTracking requires Windows; disable it to use normal process tracking")
}

func advancedJobActive(name string) bool {
	return false
}
