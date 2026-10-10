package games

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSteamGameLaunchUsesTrayClient(t *testing.T) {
	if args := steamGameLaunchArgs(42); !slices.Equal(args, []string{"-silent", "steam://run/42"}) {
		t.Fatalf("game cards must start the tray client: %v", args)
	}
}

func TestSteamGameStartupAndHandoffGrace(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamGameSessionState{started: start}
	for _, elapsed := range []time.Duration{0, 10 * time.Second, 90 * time.Second} {
		if finished, _ := state.finished(start.Add(elapsed), false, 1); finished {
			t.Fatal("slow Steam game startup returned to the launcher")
		}
	}
	if finished, _ := state.finished(start.Add(100*time.Second), true, 1); finished {
		t.Fatal("visible game ended session")
	}
	if finished, _ := state.finished(start.Add(101*time.Second), false, 1); finished {
		t.Fatal("launcher-to-game handoff ended immediately")
	}
	if finished, _ := state.finished(start.Add(110*time.Second), false, 1); finished {
		t.Fatal("handoff grace was shortened")
	}
	state.finished(start.Add(110*time.Second), true, 1)
	state.finished(start.Add(120*time.Second), false, 1)
	if finished, neverStarted := state.finished(start.Add(130*time.Second), false, 1); !finished || neverStarted {
		t.Fatal("invisible leftover helpers prevented session completion")
	}
	missing := steamGameSessionState{started: start}
	if finished, neverStarted := missing.finished(start.Add(steamGameStartGrace), false, 0); !finished || !neverStarted {
		t.Fatal("a missing game needs a bounded startup error")
	}
}

func TestSteamInstallPathBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Game")
	if !steamPathWithin(filepath.Join(root, "bin", "Game.exe"), root) {
		t.Fatal("nested game executable was excluded")
	}
	for _, path := range []string{root, filepath.Join(root+"Other", "Game.exe"), filepath.Join(root, "..", "Other", "Game.exe")} {
		if steamPathWithin(path, root) {
			t.Fatalf("unrelated path was accepted: %s", path)
		}
	}
}

func TestSteamGameEmptyExitAndLateHandoff(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamGameSessionState{started: start}
	state.finished(start, true, 1)
	if ended, _ := state.finished(start.Add(time.Second), false, 0); ended {
		t.Fatal("an empty snapshot ended the game without debounce")
	}
	// A process appears before empty grace expires, but has no window yet.
	state.finished(start.Add(time.Second+500*time.Millisecond), false, 1)
	if ended, _ := state.finished(start.Add(2*time.Second), false, 1); ended {
		t.Fatal("a windowless successor lost its handoff grace")
	}
	state.finished(start.Add(3*time.Second), true, 1)
	state.finished(start.Add(4*time.Second), false, 0)
	if ended, _ := state.finished(start.Add(4*time.Second+steamGameExitGrace-time.Millisecond), false, 0); ended {
		t.Fatal("confirmed exit debounce ended early")
	}
	if ended, neverStarted := state.finished(start.Add(4*time.Second+steamGameExitGrace), false, 0); !ended || neverStarted {
		t.Fatal("all exited native handles still used the long window handoff delay")
	}
}
