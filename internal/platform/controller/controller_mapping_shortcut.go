package controller

import "time"

// The settings shortcut is driven by a physical button, independent of the
// existing mapping. A short tap retains its normal menu action on release.
type controllerMappingShortcut struct {
	id       string
	instance uint32
	armed    bool
	button   int
	since    time.Time
	fired    bool
	tap      *launcherPadAction
}

func (s *controllerMappingShortcut) update(buttons []bool, now time.Time) (fire, released bool) {
	pressed := -1
	for i, down := range buttons {
		if down {
			pressed = i
			break
		}
	}
	if !s.armed {
		if pressed < 0 {
			s.armed = true
		}
		return false, false
	}
	if s.fired {
		return false, false
	}
	if pressed < 0 {
		wasHeld := !s.since.IsZero()
		s.since = time.Time{}
		s.button = -1
		return false, wasHeld
	}
	if s.since.IsZero() || pressed != s.button {
		s.since = now
		s.button = pressed
		return false, false
	}
	if now.Sub(s.since) >= 1500*time.Millisecond {
		s.fired = true
		return true, false
	}
	return false, false
}
