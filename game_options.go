package main

import (
	"fmt"
	"launcher/internal/games"
	"launcher/internal/platform/controller"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type GameOptions struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	EnableVirtualInput    bool   `json:"enableVirtualInput"`
	VirtualInputAvailable bool   `json:"virtualInputAvailable"`
	VirtualInputMessage   string `json:"virtualInputMessage"`
}

func (a *App) GetGameOptions(id uuid.UUID) (*GameOptions, error) {
	a.gamesMu.RLock()
	game := a.games[id]
	a.gamesMu.RUnlock()
	enabled, err := games.NativeVirtualInput(game)
	if err != nil {
		return nil, err
	}
	available, message := controller.VirtualInputAvailability()
	return &GameOptions{ID: id.String(), Name: game.Name, EnableVirtualInput: enabled,
		VirtualInputAvailable: available, VirtualInputMessage: message}, nil
}

func (a *App) SetGameVirtualInput(id uuid.UUID, enabled bool) (*GameOptions, error) {
	a.gameSessionMu.Lock()
	defer a.gameSessionMu.Unlock()
	if a.gameSessionActive {
		return nil, fmt.Errorf("close the running game before changing its input settings")
	}
	a.gamesMu.RLock()
	game := a.games[id]
	a.gamesMu.RUnlock()
	if err := games.SetNativeVirtualInput(game, enabled); err != nil {
		return nil, err
	}
	a.gamesMu.Lock()
	if current := a.games[id]; current != nil {
		copy := *current
		copy.EnableVirtualInput = enabled
		a.games[id] = &copy
	}
	a.gamesMu.Unlock()
	runtime.EventsEmit(a.ctx, "game:options-changed", map[string]any{"id": id.String(), "enableVirtualInput": enabled})
	return a.GetGameOptions(id)
}
