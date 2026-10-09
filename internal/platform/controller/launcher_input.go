package controller

const LauncherWindowClass = "StageBoxWindow"

type launcherInputGate struct {
	ready bool
}

func (g *launcherInputGate) reset() {
	g.ready = false
}

func (g *launcherInputGate) allow(enabled, neutral bool) bool {
	if !enabled {
		g.reset()
		return false
	}
	if !g.ready {
		g.ready = neutral
		return false
	}
	return true
}

type launcherKeyState struct {
	held    [256]bool
	blocked [256]bool
}

func (s *launcherKeyState) reset() {
	for key, held := range s.held {
		if held {
			s.blocked[key] = true
		}
	}
}

func (s *launcherKeyState) update(vk uint32, down, enabled bool) (emit, consume bool) {
	if vk >= uint32(len(s.held)) {
		return false, false
	}
	if !down {
		s.held[vk] = false
		s.blocked[vk] = false
		return false, false
	}
	s.held[vk] = true
	if !enabled {
		s.blocked[vk] = true
		return false, false
	}
	// Suppress repeats from keys held outside StageBox until they are released.
	return !s.blocked[vk], true
}
