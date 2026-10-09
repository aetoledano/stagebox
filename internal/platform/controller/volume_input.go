package controller

import "time"

func volumeKeyAction(vk uint32) string {
	switch vk {
	case 0xAF:
		return "up"
	case 0xAE:
		return "down"
	case 0xAD:
		return "mute"
	}
	return ""
}

type volumeStickState struct {
	gate   launcherInputGate
	action string
	next   time.Time
}

func (s *volumeStickState) reset() {
	s.gate.reset()
	s.action = ""
	s.next = time.Time{}
}

func (s *volumeStickState) update(now time.Time, x, y int, enabled bool) string {
	const deadzone = 12000
	neutral := x*x+y*y < deadzone*deadzone
	if !s.gate.allow(enabled, neutral) {
		s.action = ""
		return ""
	}
	action := ""
	if y > deadzone && y > x && y > -x {
		action = "up"
	} else if y < -deadzone && -y > x && -y > -x {
		action = "down"
	}
	if action == "" {
		s.action = ""
		return ""
	}
	if action != s.action {
		s.action = action
		s.next = now.Add(350 * time.Millisecond)
		return action
	}
	if !now.Before(s.next) {
		s.next = now.Add(120 * time.Millisecond)
		return action
	}
	return ""
}
