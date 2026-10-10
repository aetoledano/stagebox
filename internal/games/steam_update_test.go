package games

import (
	"path/filepath"
	"testing"
)

func TestSteamUpdateStatusAndProgress(t *testing.T) {
	for _, tc := range []struct {
		state    steamGameState
		needed   bool
		progress int
	}{
		{steamGameState{flags: 4}, false, -1},
		{steamGameState{flags: 6}, true, -1},
		{steamGameState{flags: 2 | 1048576, downloaded: 25, downloadTotal: 100}, true, 25},
		{steamGameState{flags: 2 | 2097152, staged: 100, stageTotal: 50}, true, 100},
		{steamGameState{flags: 2 | 512, downloaded: 25, downloadTotal: 100}, true, -1},
	} {
		status := tc.state.launchStatus()
		if status.NeedsUpdate != tc.needed || status.Progress != tc.progress {
			t.Fatalf("unexpected update status: %+v", status)
		}
	}
}

func TestSteamUpdateManifestIdentityAndCounters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appmanifest_42.acf")
	writeSteamFixture(t, path, []byte(`"AppState" { "appid" "42" "StateFlags" "1048578" "BytesDownloaded" "50" "BytesToDownload" "200" }`))
	game := &GameData{SteamAppID: 42, steamManifest: path}
	state, err := readSteamGameState(game)
	if err != nil || !state.needsUpdate() || state.launchStatus().Progress != 25 {
		t.Fatalf("update data lost: %+v %v", state, err)
	}
	game.SteamAppID = 43
	if _, err := readSteamGameState(game); err == nil {
		t.Fatal("accepted another game's update manifest")
	}
}
