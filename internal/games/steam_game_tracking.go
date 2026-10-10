package games

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func steamGameLaunchArgs(appID uint32) []string {
	return []string{"-silent", fmt.Sprintf("steam://run/%d", appID)}
}

// These helpers can outlive their game or show their own reporting window.
// Retain their correlated handles/job membership for cleanup, but never count
// them as evidence that the game itself opened or remains active. Crashpad may
// run throughout a healthy session; its presence alone does not prove a crash.
func steamGameCrashHelper(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case "crashreportclient.exe", "crashpad_handler.exe":
		return true
	}
	return false
}

const (
	steamGameStartGrace   = 2 * time.Minute
	steamGameUpdateGrace  = 30 * time.Minute
	steamGameHandoffGrace = 10 * time.Second
	steamGameExitGrace    = 750 * time.Millisecond
)

type steamGameSessionState struct {
	started    time.Time
	quietSince time.Time
	emptySince time.Time
	sawWindow  bool
	updateWait bool
	updateAt   time.Time
	lastUpdate steamGameState
}

// Manifest byte counters can stay unchanged during queueing, allocation and
// installation. Give a known update its own inactivity budget instead of
// interpreting it as a game that failed to open. Missing manifest samples do
// not reset either budget. Completion/pause starts a fresh game-window budget.
func (s *steamGameSessionState) observeUpdate(now time.Time, update steamGameState) {
	waiting := update.needsUpdate() && update.flags&512 == 0
	if waiting {
		if !s.updateWait || update != s.lastUpdate {
			s.updateAt = now
		}
	} else if s.updateWait {
		s.started = now
	}
	s.updateWait = waiting
	s.lastUpdate = update
}

func (s *steamGameSessionState) finished(now time.Time, visible bool, running int) (finished, neverStarted bool) {
	if visible {
		s.sawWindow = true
		s.quietSince = time.Time{}
		s.emptySince = time.Time{}
		return false, false
	}
	if !s.sawWindow {
		if s.updateWait {
			return now.Sub(s.updateAt) >= steamGameUpdateGrace, true
		}
		return now.Sub(s.started) >= steamGameStartGrace, true
	}
	if s.quietSince.IsZero() {
		s.quietSince = now
	}
	if running == 0 {
		if s.emptySince.IsZero() {
			s.emptySince = now
		}
		if now.Sub(s.emptySince) >= steamGameExitGrace {
			return true, false
		}
	} else {
		s.emptySince = time.Time{}
	}
	return now.Sub(s.quietSince) >= steamGameHandoffGrace, false
}

func steamPathWithin(path, directory string) bool {
	path = strings.ToLower(filepath.Clean(path))
	directory = strings.ToLower(filepath.Clean(directory))
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
