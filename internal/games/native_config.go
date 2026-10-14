package games

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var nativeConfigMu sync.Mutex

func readNativeConfig(game *GameData) ([]byte, GameData, error) {
	if game == nil || game.SteamAppID != 0 || game.nativeConfigPath == "" {
		return nil, GameData{}, fmt.Errorf("virtual input settings are only available for native game cards")
	}
	data, err := os.ReadFile(game.nativeConfigPath)
	if err != nil {
		return nil, GameData{}, err
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config GameData
	if err := decoder.Decode(&config); err != nil {
		return nil, GameData{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, GameData{}, fmt.Errorf("invalid trailing game metadata")
	}
	if config.SteamAppID != 0 {
		return nil, GameData{}, fmt.Errorf("Steam games manage their own controller input")
	}
	return data, config, nil
}

// Refresh only this setting at launch; other card metadata retains its existing
// reload behavior. Never serialize resolved runtime paths back into lc.json.
func NativeVirtualInput(game *GameData) (bool, error) {
	nativeConfigMu.Lock()
	defer nativeConfigMu.Unlock()
	_, config, err := readNativeConfig(game)
	return config.EnableVirtualInput, err
}

func SetNativeVirtualInput(game *GameData, enabled bool) error {
	nativeConfigMu.Lock()
	defer nativeConfigMu.Unlock()
	data, _, err := readNativeConfig(game)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key := range fields {
		if strings.EqualFold(key, "enableVirtualInput") {
			delete(fields, key)
		}
	}
	fields["enableVirtualInput"], _ = json.Marshal(enabled)
	updated, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	info, err := os.Stat(game.nativeConfigPath)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(game.nativeConfigPath), ".lc-options-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := file.Write(updated); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	current, err := os.ReadFile(game.nativeConfigPath)
	if err != nil {
		return err
	}
	current = bytes.TrimPrefix(current, []byte{0xef, 0xbb, 0xbf})
	if !bytes.Equal(current, data) {
		return fmt.Errorf("lc.json changed while saving; reopen game options and retry")
	}
	return os.Rename(file.Name(), game.nativeConfigPath)
}
