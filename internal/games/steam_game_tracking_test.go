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

func TestSteamGameUpdateDoesNotUseGameWindowTimeout(t *testing.T) {
	start := time.Unix(1000, 0)
	for _, update := range []steamGameState{
		{flags: 2}, // Preflight update evidence while Steam rewrites the manifest.
		{flags: 6}, // Only UpdateRequired: Steam has not published counters yet.
		{flags: 2 | 1048576, downloaded: 25, downloadTotal: 100},
		{flags: 2 | 524288}, // Preallocation can keep counters unchanged.
		{flags: 2 | 2097152, staged: 50, stageTotal: 100},
	} {
		state := steamGameSessionState{started: start}
		state.observeUpdate(start, update)
		// Re-reading the same manifest must not consume the game-window budget
		// or continually renew a genuinely stalled update's separate budget.
		state.observeUpdate(start.Add(10*time.Minute), update)
		if finished, _ := state.finished(start.Add(10*time.Minute), false, 0); finished {
			t.Fatalf("update flags=%d hit the two-minute game timeout", update.flags)
		}
		if finished, neverStarted := state.finished(start.Add(steamGameUpdateGrace), false, 0); !finished || !neverStarted {
			t.Fatalf("unchanged update flags=%d did not have a bounded wait", update.flags)
		}
	}
}

func TestSteamGameUpdateProgressAndCompletionRenewTheWait(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamGameSessionState{started: start}
	update := steamGameState{flags: 2 | 1048576, downloadTotal: 100}
	state.observeUpdate(start, update)
	progressAt := start.Add(steamGameUpdateGrace - time.Minute)
	update.downloaded = 50
	state.observeUpdate(progressAt, update)
	if finished, _ := state.finished(start.Add(steamGameUpdateGrace), false, 0); finished {
		t.Fatal("new download progress did not renew the update wait")
	}
	completedAt := progressAt.Add(20 * time.Minute)
	state.observeUpdate(completedAt, steamGameState{flags: 4})
	if finished, _ := state.finished(completedAt.Add(steamGameStartGrace-time.Millisecond), false, 1); finished {
		t.Fatal("update completion did not give the game a fresh startup budget")
	}
	if finished, neverStarted := state.finished(completedAt.Add(steamGameStartGrace), false, 0); !finished || !neverStarted {
		t.Fatal("missing game after a completed update did not time out")
	}
}

func TestSteamGamePausedUpdateAndMissingManifestDoNotRenewWait(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamGameSessionState{started: start}
	state.observeUpdate(start, steamGameState{flags: 6})
	// A missing/unreadable manifest invokes no observeUpdate call. Its last
	// known update budget must still expire instead of being renewed blindly.
	if finished, _ := state.finished(start.Add(steamGameUpdateGrace), false, 0); !finished {
		t.Fatal("missing manifest made update wait unbounded")
	}
	pausedAt := start.Add(time.Minute)
	paused := steamGameSessionState{started: start}
	paused.observeUpdate(start, steamGameState{flags: 6})
	paused.observeUpdate(pausedAt, steamGameState{flags: 2 | 512})
	if paused.updateWait {
		t.Fatal("paused update retained the long update budget")
	}
	if finished, _ := paused.finished(pausedAt.Add(steamGameStartGrace), false, 0); !finished {
		t.Fatal("paused update did not have a bounded error")
	}
}
