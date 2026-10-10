package games

import (
	"strings"
	"time"
)

const (
	steamPollInterval    = 250 * time.Millisecond
	steamRestartGrace    = 10 * time.Second
	steamUICloseGrace    = 750 * time.Millisecond
	steamClientExitGrace = 1500 * time.Millisecond
	steamUIStartGrace    = 2 * time.Minute
)

func steamSessionLaunchArgs() []string {
	// The URI also switches a client that already runs in tray/desktop mode.
	return []string{"-gamepadui", "-fullscreen", "steam://open/bigpicture"}
}

func steamBigPictureWindow(class, title string) bool {
	title = strings.TrimSpace(title)
	return strings.EqualFold(title, "Steam Big Picture Mode") ||
		(strings.EqualFold(class, "CUIEngineWin32") && strings.EqualFold(title, "Steam"))
}

type steamSessionActivity struct {
	clients       int
	visibleUI     bool
	bigPicture    bool
	visibleGame   bool
	gameProcesses int
}

type steamSessionState struct {
	started       time.Time
	absentSince   time.Time
	sawActivity   bool
	sawBigPicture bool
}

func (s *steamSessionState) finished(now time.Time, activity steamSessionActivity) bool {
	if s.started.IsZero() {
		s.started = now
	}
	if activity.bigPicture {
		s.sawBigPicture = true
	}
	// Sign-in/update windows may be needed before Big Picture opens. Once it
	// opens, returning to the desktop client ends this couch session too.
	if activity.bigPicture || activity.visibleGame || (!s.sawBigPicture && activity.visibleUI) {
		s.sawActivity = true
		s.absentSince = time.Time{}
		return false
	}
	if !s.sawActivity && activity.clients > 0 {
		s.absentSince = time.Time{}
		return now.Sub(s.started) >= steamUIStartGrace
	}
	if s.absentSince.IsZero() {
		s.absentSince = now
	}
	grace := steamUICloseGrace
	if !s.sawActivity || activity.gameProcesses > 0 {
		grace = steamRestartGrace // Initial restarts and windowless game handoffs.
	} else if activity.clients == 0 {
		grace = steamClientExitGrace // Briefly debounce a client replacement.
	}
	return now.Sub(s.absentSince) >= grace
}

type steamProcessLifetime struct {
	created uint64
	exited  uint64
}

func (p steamProcessLifetime) canParent(childCreated uint64) bool {
	return childCreated >= p.created && (p.exited == 0 || childCreated <= p.exited)
}
