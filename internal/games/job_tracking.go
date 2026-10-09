package games

import "time"

const (
	jobPollInterval = 500 * time.Millisecond
	jobQuietPeriod  = 2 * time.Second
	jobWindowGrace  = 30 * time.Second
)

type jobSessionState struct {
	started        time.Time
	rootExitedAt   time.Time
	quietSince     time.Time
	sawWindow      bool
	sawChildWindow bool
}

func (s *jobSessionState) finished(now time.Time, count int, rootExited, visible, childVisible bool) bool {
	if count == 0 {
		return true
	}
	if rootExited && s.rootExitedAt.IsZero() {
		s.rootExitedAt = now
	}
	if visible {
		s.sawWindow = true
		s.sawChildWindow = s.sawChildWindow || childVisible
		s.quietSince = time.Time{}
		return false
	}
	// A launcher's window can disappear well before its child game creates a window.
	if !s.sawChildWindow && !s.rootExitedAt.IsZero() && now.Sub(s.rootExitedAt) < jobWindowGrace {
		s.quietSince = time.Time{}
		return false
	}
	if !s.sawWindow {
		return now.Sub(s.started) >= jobWindowGrace
	}
	if s.quietSince.IsZero() {
		s.quietSince = now
	}
	return now.Sub(s.quietSince) >= jobQuietPeriod
}
