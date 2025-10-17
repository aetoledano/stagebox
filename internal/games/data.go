package games

import "github.com/google/uuid"

type GameData struct {
	ID          uuid.UUID `json:"id"`
	StartsOn    string    `json:"startsOn"`
	Executable  string    `json:"executable"`
	Image       string    `json:"image"`
	ImageB64    *string   `json:"b64"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ShowCursor  bool      `json:"showCursor"`
}

func NewGameData() GameData {
	return GameData{
		ID: uuid.New(),
	}
}
