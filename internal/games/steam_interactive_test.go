package games

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSteamCardLaunchModesKeepPlaySeparateFromUpdateOnly(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		updateOnly bool
		want       []string
	}{
		{"game launch", false, []string{"-gamepadui", "-fullscreen", "steam://run/42"}},
		{"update only", true, []string{"-gamepadui", "-fullscreen", "steam://open/bigpicture"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if got := steamCardLaunchArgs(42, scenario.updateOnly); !slices.Equal(got, scenario.want) {
				t.Fatalf("wrong activation for %s: got %v, want %v", scenario.name, got, scenario.want)
			}
		})
	}
}

func TestSteamInteractiveSetupHasNoGameStartupDeadline(t *testing.T) {
	now := time.Now()
	state := steamInteractiveState{steamSessionState: steamSessionState{started: now}}
	if state.finished(now, 1, true, true, false, 0) {
		t.Fatal("Big Picture ended setup")
	}
	if state.finished(now.Add(10*time.Minute), 1, true, true, false, 0) {
		t.Fatal("ordinary game timeout ended setup")
	}
	if state.finished(now.Add(11*time.Minute), 1, true, false, true, 0) {
		t.Fatal("visible setup dialog ended setup")
	}
	if state.finished(now.Add(12*time.Minute), 1, true, false, false, 0) {
		t.Fatal("desktop return skipped debounce")
	}
	if !state.finished(now.Add(12*time.Minute+steamUICloseGrace), 1, true, false, false, 0) {
		t.Fatal("tray/desktop survivor retained setup")
	}
}

func TestSteamInteractiveWindowlessGameHandoff(t *testing.T) {
	now := time.Now()
	state := steamInteractiveState{steamSessionState: steamSessionState{started: now}}
	state.finished(now, 1, true, true, false, 0)
	if state.finished(now.Add(time.Second), 1, false, false, false, 1) {
		t.Fatal("live selected game lost its handoff")
	}
	if state.finished(now.Add(2*time.Second), 1, true, true, false, 0) {
		t.Fatal("returning Big Picture did not reset absence")
	}
	// A minimized Big Picture is passed as present by the native owner. It
	// must not be mistaken for Exit Big Picture while Play/setup is pending.
	if state.finished(now.Add(5*time.Minute), 1, false, true, false, 0) {
		t.Fatal("minimized Big Picture lost pending setup")
	}
}

func TestSteamInteractiveNoInterfaceIsBounded(t *testing.T) {
	now := time.Now()
	state := steamInteractiveState{steamSessionState: steamSessionState{started: now}}
	if state.finished(now.Add(time.Minute), 1, false, false, false, 0) {
		t.Fatal("Steam UI startup ended early")
	}
	if !state.finished(now.Add(steamUIStartGrace), 1, false, false, false, 0) {
		t.Fatal("invisible client waited forever")
	}
}

func TestSteamInteractiveUpdateKeepsInstalledCardAndConfig(t *testing.T) {
	root := t.TempDir()
	manifest := steamManifestFixture(t, root, 42, 6, "Game", true)
	game, err := steamGameFromManifest(manifest, filepath.Join(root, "Steam.exe"))
	if err != nil || game == nil {
		t.Fatalf("installed card: %v", err)
	}
	game.steamConfigPath = filepath.Join(root, "42-lc.json")
	config := []byte(`{"steamAppId":42,"showCursor":false}`)
	if err := os.WriteFile(game.steamConfigPath, config, 0644); err != nil {
		t.Fatal(err)
	}
	steamManifestFixture(t, root, 42, 1030, "Game", true)
	if found, err := steamGameFromManifest(manifest, game.Executable); err != nil || found != nil {
		t.Fatalf("discovery admitted active install: %v %v", found, err)
	}
	if err := validateSteamGame(game); err == nil {
		t.Fatal("strict discovery validation admitted active install")
	}
	prepared, err := PrepareSteamGame(game)
	if err != nil || !prepared.SteamNeedsUpdate {
		t.Fatalf("interactive preparation: %v", err)
	}
	if err := validateSteamGameForUpdate(prepared, true); err != nil {
		t.Fatalf("update page rejected existing card: %v", err)
	}
	actual, err := os.ReadFile(game.steamConfigPath)
	if err != nil || string(actual) != string(config) {
		t.Fatalf("setup changed manual config: %s %v", actual, err)
	}
}
