package games

import (
	"testing"
	"time"
)

func TestSteamSessionRestartGrace(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamSessionState{}
	if state.finished(start, 1, false) {
		t.Fatal("a running Steam client must keep the session active")
	}
	if state.finished(start.Add(time.Second), 0, false) {
		t.Fatal("initial process exit must allow a restart")
	}
	if state.finished(start.Add(9*time.Second), 0, false) {
		t.Fatal("restart grace ended too soon")
	}
	if state.finished(start.Add(10*time.Second), 1, false) || !state.absentSince.IsZero() {
		t.Fatal("replacement client must reset the restart grace")
	}
	if state.finished(start.Add(11*time.Second), 0, false) {
		t.Fatal("the replacement client must get its own exit grace")
	}
	if state.finished(start.Add(20*time.Second), 0, false) {
		t.Fatal("normal shutdown must not end before ten seconds of absence")
	}
	if !state.finished(start.Add(21*time.Second), 0, false) {
		t.Fatal("normal shutdown must end after the grace period")
	}
}

func TestSteamSessionVisibleGame(t *testing.T) {
	start := time.Unix(1000, 0)
	state := steamSessionState{}
	state.finished(start, 0, false)
	if state.finished(start.Add(time.Minute), 0, true) || !state.absentSince.IsZero() {
		t.Fatal("a visible descendant game must keep tracking active without Steam")
	}
	if state.finished(start.Add(2*time.Minute), 0, false) {
		t.Fatal("game exit must begin a fresh grace period")
	}
	if !state.finished(start.Add(2*time.Minute+steamRestartGrace), 0, false) {
		t.Fatal("invisible helpers must not keep the session alive")
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
