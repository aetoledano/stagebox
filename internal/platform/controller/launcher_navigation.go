package controller

import "time"

type launcherPadSample struct {
	id    uint64
	state xinputGamepad
}

type launcherPadAction struct {
	source          uint64
	kind, name      string
	down, cancelled bool
}

type launcherPadNavigation struct {
	gate       launcherInputGate
	volume     volumeStickState
	previous   uint16
	held       string
	nextRepeat time.Time
	aPressed   bool
}

type launcherNavigation struct {
	pads map[uint64]*launcherPadNavigation
}

func (p *launcherPadNavigation) reset(id uint64) []launcherPadAction {
	var actions []launcherPadAction
	if p.aPressed {
		actions = append(actions, launcherPadAction{source: id, kind: "button", name: "A", cancelled: true})
	}
	*p = launcherPadNavigation{}
	return actions
}

func (n *launcherNavigation) reset() []launcherPadAction {
	var actions []launcherPadAction
	for id, pad := range n.pads {
		actions = append(actions, pad.reset(id)...)
	}
	n.pads = nil
	return actions
}

// Every source owns its own neutral rearm, edges, repeat and volume state.
// Never OR physical states: that would create chords across different pads.
func (n *launcherNavigation) update(samples []launcherPadSample, enabled bool, now time.Time) []launcherPadAction {
	if n.pads == nil {
		n.pads = make(map[uint64]*launcherPadNavigation)
	}
	seen := make(map[uint64]bool, len(samples))
	var actions []launcherPadAction
	for _, sample := range samples {
		seen[sample.id] = true
		pad := n.pads[sample.id]
		if pad == nil {
			pad = &launcherPadNavigation{}
			n.pads[sample.id] = pad
		}
		actions = append(actions, pad.update(sample, enabled, now)...)
	}
	for id, pad := range n.pads {
		if !seen[id] {
			actions = append(actions, pad.reset(id)...)
			delete(n.pads, id)
		}
	}
	return actions
}

func (p *launcherPadNavigation) update(sample launcherPadSample, enabled bool, now time.Time) []launcherPadAction {
	gp, id := sample.state, sample.id
	buttons := gp.wButtons
	both := buttons&(XINPUT_GAMEPAD_START|XINPUT_GAMEPAD_BACK) == XINPUT_GAMEPAD_START|XINPUT_GAMEPAD_BACK
	if !enabled || both {
		return p.reset(id)
	}
	if !p.gate.allow(true, gamepadNeutral(gp)) {
		p.volume.update(now, int(gp.sThumbRX), int(gp.sThumbRY), true)
		return nil
	}
	var actions []launcherPadAction
	add := func(kind, name string) {
		actions = append(actions, launcherPadAction{source: id, kind: kind, name: name})
	}
	just := func(mask uint16) bool { return buttons&mask != 0 && p.previous&mask == 0 }
	for _, direction := range []struct {
		mask uint16
		name string
	}{
		{XINPUT_GAMEPAD_DPAD_LEFT, "left"}, {XINPUT_GAMEPAD_DPAD_RIGHT, "right"},
		{XINPUT_GAMEPAD_DPAD_UP, "up"}, {XINPUT_GAMEPAD_DPAD_DOWN, "down"},
	} {
		if just(direction.mask) {
			add("nav", direction.name)
		}
	}
	lx, ly := int64(gp.sThumbLX), int64(gp.sThumbLY)
	held := ""
	if lx*lx+ly*ly >= 8500*8500 {
		if lx*lx >= ly*ly {
			if lx < 0 {
				held = "left"
			} else {
				held = "right"
			}
		} else if ly > 0 {
			held = "up"
		} else {
			held = "down"
		}
	}
	if held == "" {
		p.held = ""
	} else if held != p.held {
		p.held, p.nextRepeat = held, now.Add(300*time.Millisecond)
		add("nav", held)
	} else if !now.Before(p.nextRepeat) {
		p.nextRepeat = now.Add(180 * time.Millisecond)
		add("nav", held)
	}
	if action := p.volume.update(now, int(gp.sThumbRX), int(gp.sThumbRY), true); action != "" {
		add("volume", action)
	}
	if just(XINPUT_GAMEPAD_A) {
		p.aPressed = true
		actions = append(actions, launcherPadAction{source: id, kind: "button", name: "A", down: true})
	}
	if p.aPressed && buttons&XINPUT_GAMEPAD_A == 0 {
		p.aPressed = false
		add("button", "A")
	}
	for _, button := range []struct {
		mask uint16
		name string
	}{
		{XINPUT_GAMEPAD_B, "B"}, {XINPUT_GAMEPAD_LEFT_SHOULDER, "LB"}, {XINPUT_GAMEPAD_RIGHT_SHOULDER, "RB"},
	} {
		if just(button.mask) {
			add("press", button.name)
		}
	}
	if just(XINPUT_GAMEPAD_START) && buttons&XINPUT_GAMEPAD_BACK == 0 {
		add("press", "START")
	}
	p.previous = buttons
	return actions
}
