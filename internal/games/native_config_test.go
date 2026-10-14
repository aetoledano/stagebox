package games

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNativeVirtualInputPersistsOnlySetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lc.json")
	original := []byte(`{"name":"Native fixture","executable":"bin/game.exe","startsOn":"bin","image":"cover.png","args":["--offline"],"advancedTracking":true,"keyMap":{"buttons":{"A":["Enter"]}}}`)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	game := &GameData{nativeConfigPath: path, Executable: "C:/resolved/bin/game.exe", Image: "C:/resolved/cover.png"}
	if enabled, err := NativeVirtualInput(game); err != nil || enabled {
		t.Fatalf("default enabled=%v err=%v", enabled, err)
	}
	if err := SetNativeVirtualInput(game, true); err != nil {
		t.Fatal(err)
	}
	if enabled, err := NativeVirtualInput(game); err != nil || !enabled {
		t.Fatalf("saved enabled=%v err=%v", enabled, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	json.Unmarshal(original, &before)
	json.Unmarshal(data, &after)
	delete(after, "enableVirtualInput")
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("other metadata changed: %s", data)
	}
	if err := SetNativeVirtualInput(game, false); err != nil {
		t.Fatal(err)
	}
	if enabled, err := NativeVirtualInput(game); err != nil || enabled {
		t.Fatalf("disabled enabled=%v err=%v", enabled, err)
	}
}

func TestNativeVirtualInputRejectsSteamAndInvalidMetadata(t *testing.T) {
	for _, sample := range []struct {
		name, metadata string
		steam          uint32
	}{
		{"steam-card", `{"name":"Steam fixture"}`, 42},
		{"steam-metadata", `{"name":"Steam fixture","steamAppId":42}`, 0},
		{"unknown-field", `{"name":"Fixture","misspelledSetting":true}`, 0},
		{"trailing", `{"name":"Fixture"}{}`, 0},
	} {
		t.Run(sample.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lc.json")
			if err := os.WriteFile(path, []byte(sample.metadata), 0o644); err != nil {
				t.Fatal(err)
			}
			game := &GameData{SteamAppID: sample.steam, nativeConfigPath: path}
			if err := SetNativeVirtualInput(game, true); err == nil {
				t.Fatal("invalid native setting accepted")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != sample.metadata {
				t.Fatalf("metadata modified: data=%q err=%v", data, err)
			}
		})
	}
}
