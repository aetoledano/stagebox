package games

import (
	"encoding/json"

	"github.com/google/uuid"
)

type GameData struct {
	ID           uuid.UUID   `json:"id"`
	StartsOn     string      `json:"startsOn"`
	Executable   string      `json:"executable"`
	Image        string      `json:"image"`
	ImageHash    string      `json:"imageHash"`
	ThumbnailURL string      `json:"thumbnailUrl"`
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	Controls     int         `json:"controls"`
	ShowCursor   bool        `json:"showCursor"`
	DelayedKey   *DelayedKey `json:"delayedKey"`
	KeyMap       *KeyMap     `json:"keyMap"`
}

type DelayedKey struct {
	Seconds float64 `json:"seconds"`
	Key     string  `json:"key"`
}

type KeyMap struct {
	Buttons          map[string][]string `json:"buttons"`
	LeftStick        map[string][]string `json:"leftStick"`
	RightStick       map[string][]string `json:"rightStick"`
	Mouse            string              `json:"mouse"`
	MouseSensitivity int                 `json:"mouseSensitivity"`
}

func (m *KeyMap) UnmarshalJSON(data []byte) error {
	type structured KeyMap
	var s structured
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if len(s.Buttons) > 0 || len(s.LeftStick) > 0 || len(s.RightStick) > 0 || s.Mouse != "" || s.MouseSensitivity != 0 {
		*m = KeyMap(s)
		return nil
	}

	return nil
}

func NewGameData() GameData {
	return GameData{}
}
