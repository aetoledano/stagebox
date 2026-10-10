package games

import (
	"encoding/json"
	"errors"
	"fmt"
	"launcher/internal/jsonx"
	lgx "launcher/internal/logx"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var steamConfigMu sync.Mutex

// Steam launch identity, executable, install directory and tracking stay
// discovery-owned. Pointer fields distinguish omitted settings from false/0.
type steamGameConfig struct {
	SteamAppID           uint32      `json:"steamAppId"`
	Name                 *string     `json:"name,omitempty"`
	Description          *string     `json:"description,omitempty"`
	Image                *string     `json:"image,omitempty"`
	Controls             *int        `json:"controls,omitempty"`
	ShowCursor           *bool       `json:"showCursor,omitempty"`
	WaitAfterExitSeconds *float64    `json:"waitAfterExitSeconds,omitempty"`
	DelayedKey           *DelayedKey `json:"delayedKey"`
	KeyMap               *KeyMap     `json:"keyMap"`
}

func syncSteamConfigs(root string, library steamLibrarySnapshot, onError func(string, error)) {
	if root == "" {
		return
	}
	steamConfigMu.Lock()
	defer steamConfigMu.Unlock()
	report := func(path string, err error) {
		lgx.Warnf("Steam game config path=%q: %v", path, err)
		if onError != nil {
			onError(path, err)
		}
	}
	dir := filepath.Join(root, "SteamGames")
	if len(library.games) > 0 {
		if err := os.MkdirAll(dir, 0755); err != nil {
			report(dir, err)
			return
		}
	}
	for _, game := range library.games {
		base := *game
		game.steamDefaults = &base
		game.steamConfigPath = filepath.Join(dir, fmt.Sprintf("%d-lc.json", game.SteamAppID))
		defaults := steamGameConfig{SteamAppID: game.SteamAppID, Name: &base.Name, Description: &base.Description,
			Controls: &base.Controls, ShowCursor: &base.ShowCursor, WaitAfterExitSeconds: &base.WaitAfterExitSeconds}
		if err := createSteamConfig(game.steamConfigPath, defaults); err != nil {
			report(game.steamConfigPath, err)
		}
		if err := applySteamConfig(game); err != nil {
			report(game.steamConfigPath, err)
		}
	}
	// Never infer an uninstall from missing Steam, an inaccessible library,
	// malformed manifest, or an excluded downloading game.
	if !library.complete {
		return
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		report(dir, err)
		return
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), "-lc.json") {
			continue
		}
		id, err := strconv.ParseUint(strings.TrimSuffix(entry.Name(), "-lc.json"), 10, 32)
		if err != nil || id == 0 || entry.Name() != fmt.Sprintf("%d-lc.json", id) || library.present[uint32(id)] {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		// Delete only a valid config claiming the same ID as its owned filename.
		var config steamGameConfig
		if jsonx.LoadJSONFile(path, &config) != nil || config.SteamAppID != uint32(id) {
			continue
		}
		if err := os.Remove(path); err != nil {
			report(path, err)
		} else {
			lgx.Infof("removed uninstalled Steam config %q", path)
		}
	}
}

func createSteamConfig(path string, config steamGameConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func applySteamConfig(game *GameData) error {
	if game.steamConfigPath == "" {
		return nil
	}
	var config steamGameConfig
	if err := jsonx.LoadJSONFile(game.steamConfigPath, &config); err != nil {
		return fmt.Errorf("read Steam game config %s: %w", game.steamConfigPath, err)
	}
	if config.SteamAppID != game.SteamAppID {
		return fmt.Errorf("Steam config app ID must remain %d", game.SteamAppID)
	}
	if config.Controls != nil && (*config.Controls < -1 || *config.Controls > 2) {
		return fmt.Errorf("controls must be -1, 0, 1 or 2")
	}
	if config.WaitAfterExitSeconds != nil && *config.WaitAfterExitSeconds < 0 {
		return fmt.Errorf("waitAfterExitSeconds cannot be negative")
	}
	if config.DelayedKey != nil && config.DelayedKey.Seconds < 0 {
		return fmt.Errorf("delayedKey.seconds cannot be negative")
	}
	if config.Name != nil {
		game.Name = *config.Name
	}
	if config.Description != nil {
		game.Description = *config.Description
	}
	if config.Controls != nil {
		game.Controls = *config.Controls
	}
	if config.ShowCursor != nil {
		game.ShowCursor = *config.ShowCursor
	}
	if config.WaitAfterExitSeconds != nil {
		game.WaitAfterExitSeconds = *config.WaitAfterExitSeconds
	}
	game.DelayedKey, game.KeyMap = config.DelayedKey, config.KeyMap
	if config.Image != nil && *config.Image != "" {
		game.Image = *config.Image
		if !filepath.IsAbs(game.Image) {
			game.Image = filepath.Join(filepath.Dir(game.steamConfigPath), game.Image)
		}
	}
	return nil
}

// PrepareSteamGame re-reads manual edits without mutating the shared card.
func PrepareSteamGame(game *GameData) (*GameData, error) {
	if err := validateSteamGameForUpdate(game, true); err != nil {
		return nil, err
	}
	prepared := *game
	if game.steamDefaults != nil {
		prepared = *game.steamDefaults
		prepared.steamConfigPath, prepared.steamDefaults = game.steamConfigPath, game.steamDefaults
	}
	if err := applySteamConfig(&prepared); err != nil {
		return nil, err
	}
	state, err := readSteamGameState(&prepared)
	if err != nil {
		return nil, err
	}
	prepared.SteamNeedsUpdate = state.needsUpdate()
	return &prepared, nil
}
