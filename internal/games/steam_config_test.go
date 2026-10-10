package games

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSteamConfigsPreserveEditsAndReloadForLaunch(t *testing.T) {
	root, steam := t.TempDir(), t.TempDir()
	manifest := steamManifestFixture(t, steam, 42, 6, "Game", true)
	game, err := steamGameFromManifest(manifest, filepath.Join(steam, "Steam.exe"))
	if err != nil {
		t.Fatal(err)
	}
	library := steamLibrarySnapshot{games: []*GameData{game}, present: map[uint32]bool{42: true}, complete: true}
	syncSteamConfigs(root, library, nil)
	path := filepath.Join(root, "SteamGames", "42-lc.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	manual := []byte(`{"steamAppId":42,"showCursor":false,"controls":2,"name":"My Game","waitAfterExitSeconds":1,"delayedKey":{"seconds":1,"key":"Enter"}}`)
	if err := os.WriteFile(path, manual, 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSteamGame(game)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.ShowCursor || prepared.Name != "My Game" || prepared.Controls != 2 || prepared.DelayedKey == nil || !prepared.SteamNeedsUpdate {
		t.Fatalf("manual launch overrides lost: %+v", prepared)
	}
	if !game.ShowCursor || game.Name != "Game 42" {
		t.Fatal("launch preparation mutated shared card data")
	}
	fresh, _ := steamGameFromManifest(manifest, game.Executable)
	library.games = []*GameData{fresh}
	syncSteamConfigs(root, library, nil)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(manual) {
		t.Fatal("reload rewrote manual config")
	}
	if fresh.ShowCursor || fresh.Name != "My Game" {
		t.Fatal("reload did not apply overrides")
	}
	if err := os.WriteFile(path, []byte(`{"steamAppId":42}`), 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err = PrepareSteamGame(fresh)
	if err != nil || !prepared.ShowCursor || prepared.Controls != -1 || prepared.Name != "Game 42" {
		t.Fatal("omitted fields did not restore discovery defaults")
	}
	if err := os.WriteFile(path, []byte(`{"steamAppId":99,"showCursor":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareSteamGame(fresh); err == nil {
		t.Fatal("accepted config for another AppID")
	}
}

func TestSteamConfigCleanupRequiresCompleteInventory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "SteamGames")
	owned := filepath.Join(dir, "42-lc.json")
	writeSteamFixture(t, owned, []byte(`{"steamAppId":42,"showCursor":false}`))
	writeSteamFixture(t, filepath.Join(dir, "notes.txt"), []byte("keep"))
	writeSteamFixture(t, filepath.Join(dir, "99-lc.json"), []byte(`{"steamAppId":100}`))
	writeSteamFixture(t, filepath.Join(dir, "43-lc.json"), []byte(`{"steamAppId":43`))
	for _, library := range []steamLibrarySnapshot{
		{present: map[uint32]bool{}, complete: false},        // missing drive / Steam unavailable
		{present: map[uint32]bool{42: true}, complete: true}, // downloading manifest, card temporarily excluded
	} {
		syncSteamConfigs(root, library, nil)
		if _, err := os.Stat(owned); err != nil {
			t.Fatal("temporary absence erased settings")
		}
	}
	syncSteamConfigs(root, steamLibrarySnapshot{present: map[uint32]bool{}, complete: true}, nil)
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatal("uninstalled game's config was retained")
	}
	for _, name := range []string{"notes.txt", "99-lc.json", "43-lc.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("removed unrelated/invalid file: %s", name)
		}
	}
}

func TestSteamConfigFolderIsNotALocalGame(t *testing.T) {
	root := t.TempDir()
	writeSteamFixture(t, filepath.Join(root, "SteamGames", "42-lc.json"), []byte(`{"steamAppId":42}`))
	errors := 0
	loaded := LoadGamesWithOptions(root, LoadOptions{OnError: func(string, error) { errors++ }})
	if len(loaded) != 0 || errors != 0 {
		t.Fatal("SteamGames was scanned as a local game directory")
	}
}
