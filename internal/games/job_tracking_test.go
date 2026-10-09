package games

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGameTrackingMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		game GameData
		want string
	}{
		{"default", GameData{}, TrackModeProcess},
		{"tree", GameData{TrackMode: "processTree"}, TrackModeProcessTree},
		{"advanced", GameData{AdvancedTracking: true}, TrackModeJob},
		{"advanced overrides tree", GameData{TrackMode: "processTree", AdvancedTracking: true}, TrackModeJob},
		{"false preserves tree", GameData{TrackMode: "processTree", AdvancedTracking: false}, TrackModeProcessTree},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gameTrackingMode(&tc.game); got != tc.want {
				t.Fatalf("mode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAdvancedTrackingMetadata(t *testing.T) {
	dir := t.TempDir()
	metadata := `{
		"executable":"launcher.exe", "image":"apache-air-assault.jpg",
		"name":"Apache Air Assault", "description":"", "showCursor":false,
		"controls":2, "delayedKey":{"seconds":1,"key":"Enter"},
		"advancedTracking":true
	}`
	if err := os.WriteFile(filepath.Join(dir, "lc.json"), []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	game, err := loadGame(dir, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !game.AdvancedTracking || gameTrackingMode(game) != TrackModeJob {
		t.Fatal("advancedTracking was not enabled")
	}
	if game.DelayedKey == nil || game.DelayedKey.Seconds != 1 || game.DelayedKey.Key != "Enter" || game.ShowCursor || game.Controls != 2 {
		t.Fatal("existing launch settings changed while decoding advancedTracking")
	}
	if game.Executable != filepath.Join(dir, "launcher.exe") || game.StartsOn != dir {
		t.Fatal("game paths were not resolved")
	}
}

func TestJobSessionState(t *testing.T) {
	start := time.Unix(1000, 0)
	t.Run("empty job ends immediately", func(t *testing.T) {
		state := jobSessionState{started: start}
		if !state.finished(start, 0, true, false, false) {
			t.Fatal("empty job must finish")
		}
	})
	t.Run("launcher to delayed child window", func(t *testing.T) {
		state := jobSessionState{started: start}
		if state.finished(start, 1, false, true, false) {
			t.Fatal("visible launcher must remain tracked")
		}
		if state.finished(start.Add(time.Second), 1, true, false, false) {
			t.Fatal("launcher exit must allow child window handoff")
		}
		if state.finished(start.Add(10*time.Second), 1, true, false, false) {
			t.Fatal("slow child startup must remain tracked during grace period")
		}
		if state.finished(start.Add(11*time.Second), 1, true, true, true) {
			t.Fatal("visible child game must remain tracked")
		}
		if state.finished(start.Add(12*time.Second), 1, true, false, false) {
			t.Fatal("helpers should observe the window quiet period")
		}
		if !state.finished(start.Add(14*time.Second), 1, true, false, false) {
			t.Fatal("invisible lingering helpers must not hold the session indefinitely")
		}
	})
	t.Run("startup without windows times out", func(t *testing.T) {
		state := jobSessionState{started: start}
		if state.finished(start.Add(29*time.Second), 1, false, false, false) {
			t.Fatal("startup grace ended too early")
		}
		if !state.finished(start.Add(30*time.Second), 1, false, false, false) {
			t.Fatal("startup without any window must be bounded")
		}
	})
	t.Run("visible window resets quiet period", func(t *testing.T) {
		state := jobSessionState{started: start}
		state.finished(start, 1, true, true, true)
		state.finished(start.Add(time.Second), 1, true, false, false)
		if state.finished(start.Add(2*time.Second), 1, true, true, true) {
			t.Fatal("a visible game must reset the quiet period")
		}
		if state.finished(start.Add(3*time.Second), 1, true, false, false) {
			t.Fatal("the new quiet period must start from the latest disappearance")
		}
	})
}
