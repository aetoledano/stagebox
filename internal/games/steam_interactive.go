package games

import (
	"fmt"
	"time"
)

func steamGameDetailsArgs(appID uint32) []string {
	return []string{fmt.Sprintf("steam://nav/games/details/%d", appID)}
}

// Download-only must never send the game's Play request.
func steamCardLaunchArgs(appID uint32, updateOnly bool) []string {
	if updateOnly {
		return steamSessionLaunchArgs()
	}
	return steamGameLaunchArgs(appID)
}

type steamInteractiveState struct{ steamSessionState }

func (s *steamInteractiveState) finished(now time.Time, clients int, uiVisible, bigPicture, dialog bool, gameProcesses int) bool {
	// A visible setup dialog must survive even after Big Picture temporarily
	// disappears. Exiting Big Picture to the ordinary desktop client returns
	// promptly; the tray process alone cannot retain this session.
	return s.steamSessionState.finished(now, steamSessionActivity{
		clients: clients, visibleUI: uiVisible, bigPicture: bigPicture,
		visibleGame: dialog, gameProcesses: gameProcesses,
	})
}
