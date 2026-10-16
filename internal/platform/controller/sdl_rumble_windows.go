//go:build windows

package controller

import (
	lgx "launcher/internal/logx"
	"time"
)

type sdlRumbleState struct {
	level  virtualRumble
	at     time.Time
	warned bool
}

// Only the locked SDL owner touches handles. The IPC reader publishes immutable
// snapshots, and path lookup keeps rumble independent of navigation selection.
func (s *sdlBackend) forwardVirtualRumble(session *virtualInputSession, now time.Time) {
	levels := session.rumbleLevels(now)
	for id, handle := range s.gamepads {
		if id == s.testRumbleID {
			continue
		}
		level := virtualRumble{}
		if session != nil {
			for i, path := range session.sourcePaths {
				if s.paths[id] == path && i < len(levels) {
					level = levels[i]
					break
				}
			}
		}
		previous := s.rumbling[id]
		if previous.level == level && (level == (virtualRumble{}) || now.Sub(previous.at) < 100*time.Millisecond) {
			continue
		}
		// Finite effects also stop if the owner stalls or the feeder disappears.
		result, _, _ := s.rumble.Call(handle, uintptr(level.low), uintptr(level.high), 250)
		if result&0xff == 0 && !previous.warned {
			lgx.Warnf("SDL3 physical rumble unavailable name=%q path=%q: %s", s.info[id].Name, s.paths[id], s.errorText())
			previous.warned = true
		}
		s.rumbling[id] = sdlRumbleState{level: level, at: now, warned: previous.warned}
	}
}

func (s *sdlBackend) stopGamepadRumble(id uint32, handle uintptr) {
	if s.rumble != nil && handle != 0 && (s.rumbling[id].level != (virtualRumble{}) || id == s.testRumbleID) {
		s.rumble.Call(handle, 0, 0, 0)
	}
	if id == s.testRumbleID {
		s.testRumbleID, s.testRumbleOwner = 0, nil
		s.testRumbleUntil = time.Time{}
	}
	delete(s.rumbling, id)
}
