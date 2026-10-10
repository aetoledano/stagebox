package games

import (
	"testing"
	"time"
)

func TestSteamSessionStartupRestartAndFullExit(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamSessionState{started: start}
	if state.finished(start, steamSessionActivity{}) || state.finished(start.Add(9*time.Second), steamSessionActivity{}) {
		t.Fatal("initial client startup must allow the full restart grace")
	}
	active := steamSessionActivity{clients: 1, bigPicture: true, visibleUI: true}
	if state.finished(start.Add(10*time.Second), active) || !state.absentSince.IsZero() {
		t.Fatal("visible replacement client did not reset startup grace")
	}
	state.finished(start.Add(11*time.Second), steamSessionActivity{})
	if state.finished(start.Add(11*time.Second+steamClientExitGrace-time.Millisecond), steamSessionActivity{}) {
		t.Fatal("full client exit returned before debounce")
	}
	if !state.finished(start.Add(11*time.Second+steamClientExitGrace), steamSessionActivity{}) {
		t.Fatal("full shutdown still waited the long restart grace")
	}
}

func TestSteamSessionClosesWithTrayOrDesktopClient(t *testing.T) {
	for _, desktop := range []bool{false, true} {
		start := time.Unix(1000, 0)
		state := steamSessionState{}
		state.finished(start, steamSessionActivity{clients: 1, bigPicture: true, visibleUI: true})
		remaining := steamSessionActivity{clients: 1, visibleUI: desktop}
		if state.finished(start.Add(time.Second), remaining) {
			t.Fatal("Big Picture close must allow a UI/game handoff")
		}
		if !state.finished(start.Add(time.Second+steamUICloseGrace), remaining) {
			t.Fatalf("Big Picture close must return even with a live client (desktop=%v)", desktop)
		}
	}
}

func TestSteamSessionVisibleGame(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamSessionState{}
	state.finished(start, steamSessionActivity{clients: 1, bigPicture: true})
	state.finished(start.Add(time.Second), steamSessionActivity{clients: 1})
	if state.finished(start.Add(time.Minute), steamSessionActivity{visibleGame: true}) || !state.absentSince.IsZero() {
		t.Fatal("a visible descendant game must keep tracking active without Steam UI")
	}
	if state.finished(start.Add(2*time.Minute), steamSessionActivity{clients: 1}) {
		t.Fatal("game exit must begin a fresh grace period")
	}
	if !state.finished(start.Add(2*time.Minute+steamUICloseGrace), steamSessionActivity{clients: 1}) {
		t.Fatal("invisible helpers/tray client must not keep the session alive")
	}
}

func TestSteamSessionStartupAndSignIn(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamSessionState{started: start}
	tray := steamSessionActivity{clients: 1}
	if state.finished(start.Add(steamRestartGrace), tray) {
		t.Fatal("a cold tray client needs time to create Big Picture")
	}
	if !state.finished(start.Add(steamUIStartGrace), tray) || state.sawActivity {
		t.Fatal("a tray-only startup must time out without pretending to show UI")
	}
	state = steamSessionState{started: start}
	if state.finished(start.Add(steamUIStartGrace), steamSessionActivity{clients: 1, visibleUI: true}) {
		t.Fatal("sign-in UI must remain usable before Big Picture opens")
	}
	state.finished(start.Add(3*time.Minute), tray)
	if !state.finished(start.Add(3*time.Minute+steamUICloseGrace), tray) {
		t.Fatal("closing sign-in to tray must return to StageBox")
	}
}

func TestSteamBigPictureWindow(t *testing.T) {
	for _, tc := range []struct {
		class, title string
		want         bool
	}{
		{"SDL_app", "Steam Big Picture Mode", true},
		{"CUIEngineWin32", "Steam", true},
		{"SDL_app", "Steam", false},
		{"SDL_app", "Friends List", false},
		{"SDL_app", "Sign in to Steam", false},
	} {
		if got := steamBigPictureWindow(tc.class, tc.title); got != tc.want {
			t.Fatalf("Big Picture window (%q, %q) = %v, want %v", tc.class, tc.title, got, tc.want)
		}
	}
}

func TestSteamProcessLifetime(t *testing.T) {
	for _, tc := range []struct {
		name   string
		parent steamProcessLifetime
		child  uint64
		want   bool
	}{
		{"live parent", steamProcessLifetime{created: 100}, 200, true},
		{"child predates parent", steamProcessLifetime{created: 100}, 99, false},
		{"child survived parent", steamProcessLifetime{created: 100, exited: 200}, 150, true},
		{"parent PID reused", steamProcessLifetime{created: 100, exited: 200}, 201, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.parent.canParent(tc.child); got != tc.want {
				t.Fatalf("canParent = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSteamSessionWindowlessGameHandoff(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamSessionState{started: start}
	state.finished(start, steamSessionActivity{clients: 1, bigPicture: true})
	handoff := steamSessionActivity{clients: 1, gameProcesses: 1}
	state.finished(start.Add(time.Second), handoff)
	if state.finished(start.Add(9*time.Second), handoff) {
		t.Fatal("a running game process lost its full window handoff grace")
	}
	if state.finished(start.Add(10*time.Second), steamSessionActivity{clients: 1, visibleGame: true, gameProcesses: 1}) {
		t.Fatal("visible successor failed to keep the session active")
	}
	state.finished(start.Add(11*time.Second), handoff)
	if !state.finished(start.Add(11*time.Second+steamRestartGrace), handoff) {
		t.Fatal("windowless leftover process kept the session indefinitely")
	}
}
