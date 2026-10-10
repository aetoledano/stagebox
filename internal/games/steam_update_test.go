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
		{steamGameState{flags: 1030, downloadTotal: 20969944704, stageTotal: 84229582953}, true, -1},
		{steamGameState{flags: 1030, downloaded: 25, downloadTotal: 100}, true, 25},
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

func TestSteamUpdateStartedWaitsForMeasurablePhase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appmanifest_2767030.acf")
	// Values from the Marvel Rivals startup log: known totals alone must not
	// turn generic UpdateStarted into a claimed active transfer or fake 0%.
	writeSteamFixture(t, path, []byte(`"AppState" { "appid" "2767030" "StateFlags" "1030" "BytesDownloaded" "0" "BytesToDownload" "20969944704" "BytesStaged" "0" "BytesToStage" "84229582953" }`))
	state, err := readSteamGameState(&GameData{SteamAppID: 2767030, steamManifest: path})
	if err != nil {
		t.Fatal(err)
	}
	if state.downloadTotal != 20969944704 || state.stageTotal != 84229582953 {
		t.Fatalf("large update totals were truncated: %+v", state)
	}
	status := state.launchStatus()
	if status.Message != "Steam is preparing the update…" || status.Progress != -1 {
		t.Fatalf("update startup without transferred bytes claimed a download: %+v", status)
	}
	// Generic flags must still expose actual partial byte counters when Steam
	// publishes them without the more specific Downloading flag.
	state.downloaded = state.downloadTotal / 2
	status = state.launchStatus()
	if status.Progress != 50 || status.Message != "Steam update download progress…" {
		t.Fatalf("reported download bytes with generic flags were hidden: %+v", status)
	}
	state.downloaded = state.downloadTotal
	if status := state.launchStatus(); status.Progress != -1 {
		t.Fatalf("completed download masqueraded as completed installation: %+v", status)
	}
	// The later user snapshot kept the same generic flags after download
	// completion, but exposed partially staged installation bytes.
	state.staged = 25205160481
	status = state.launchStatus()
	if status.Progress != 29 || status.Message != "Steam update installation progress…" {
		t.Fatalf("partial installation snapshot used completed download progress: %+v", status)
	}
	state.staged = state.stageTotal
	if status := state.launchStatus(); status.Progress != -1 {
		t.Fatalf("completed byte counters claimed the pending update was finished: %+v", status)
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
