package controller

import (
	"testing"
	"time"
)

func TestLauncherEveryControllerHasIndependentEdgesAndRepeats(t *testing.T) {
	var nav launcherNavigation
	now := time.Unix(1, 0)
	pads := []launcherPadSample{{id: 1}, {id: 2}}
	nav.update(pads, true, now)
	pads[0].state.sThumbLX = 22000
	pads[1].state.wButtons = XINPUT_GAMEPAD_A | XINPUT_GAMEPAD_DPAD_DOWN
	actions := nav.update(pads, true, now.Add(8*time.Millisecond))
	if !hasLauncherAction(actions, 1, "nav", "right", false, false) || !hasLauncherAction(actions, 2, "nav", "down", false, false) || !hasLauncherAction(actions, 2, "button", "A", true, false) {
		t.Fatalf("not all controllers navigated: %+v", actions)
	}
	if repeated := nav.update(pads, true, now.Add(16*time.Millisecond)); len(repeated) != 0 {
		t.Fatalf("held inputs repeated too soon: %+v", repeated)
	}
	pads[1].state.wButtons = 0
	actions = nav.update(pads, true, now.Add(24*time.Millisecond))
	if !hasLauncherAction(actions, 2, "button", "A", false, false) || len(actions) != 1 {
		t.Fatalf("release crossed source state: %+v", actions)
	}
	actions = nav.update(pads, true, now.Add(310*time.Millisecond))
	if !hasLauncherAction(actions, 1, "nav", "right", false, false) {
		t.Fatalf("first controller lost repeat when second released: %+v", actions)
	}
}

func TestLauncherHotplugHeldPadDoesNotBlockOtherPad(t *testing.T) {
	var nav launcherNavigation
	now := time.Unix(1, 0)
	pads := []launcherPadSample{{id: 1}}
	nav.update(pads, true, now)
	pads = append(pads, launcherPadSample{id: 2, state: xinputGamepad{wButtons: XINPUT_GAMEPAD_A}})
	pads[0].state.wButtons = XINPUT_GAMEPAD_B
	actions := nav.update(pads, true, now.Add(8*time.Millisecond))
	if len(actions) != 1 || !hasLauncherAction(actions, 1, "press", "B", false, false) {
		t.Fatalf("held replacement blocked existing pad or activated: %+v", actions)
	}
	pads[0].state.wButtons, pads[1].state.wButtons = 0, 0
	nav.update(pads, true, now.Add(16*time.Millisecond))
	pads[1].state.wButtons = XINPUT_GAMEPAD_A
	if actions = nav.update(pads, true, now.Add(24*time.Millisecond)); !hasLauncherAction(actions, 2, "button", "A", true, false) {
		t.Fatalf("replacement never rearmed: %+v", actions)
	}
	actions = nav.update(pads[:1], true, now.Add(32*time.Millisecond))
	if !hasLauncherAction(actions, 2, "button", "A", false, true) {
		t.Fatalf("disconnect did not cancel its own hold: %+v", actions)
	}
}

func TestLauncherDisabledFocusAndTestCaptureRearmEveryPad(t *testing.T) {
	var nav launcherNavigation
	now := time.Unix(1, 0)
	pads := []launcherPadSample{{id: 1}, {id: 2}}
	nav.update(pads, true, now)
	pads[1].state.wButtons = XINPUT_GAMEPAD_A
	nav.update(pads, true, now.Add(time.Millisecond))
	if actions := nav.update(pads, false, now.Add(2*time.Millisecond)); !hasLauncherAction(actions, 2, "button", "A", false, true) {
		t.Fatalf("disable did not cancel pending A: %+v", actions)
	}
	pads[0].state.wButtons = XINPUT_GAMEPAD_START
	if actions := nav.update(pads, true, now.Add(3*time.Millisecond)); len(actions) != 0 {
		t.Fatalf("held inputs leaked across focus/capture boundary: %+v", actions)
	}
	pads[0].state.wButtons = 0
	nav.update(pads, true, now.Add(4*time.Millisecond))
	pads[0].state.wButtons = XINPUT_GAMEPAD_START
	if actions := nav.update(pads, true, now.Add(5*time.Millisecond)); !hasLauncherAction(actions, 1, "press", "START", false, false) {
		t.Fatalf("one held pad blocked another neutral pad: %+v", actions)
	}
}

func TestLauncherStartAndBackCannotFormCrossControllerChord(t *testing.T) {
	var nav launcherNavigation
	now := time.Unix(1, 0)
	pads := []launcherPadSample{{id: 1}, {id: 2}}
	nav.update(pads, true, now)
	pads[0].state.wButtons = XINPUT_GAMEPAD_BACK
	pads[1].state.wButtons = XINPUT_GAMEPAD_START
	if actions := nav.update(pads, true, now.Add(time.Millisecond)); !hasLauncherAction(actions, 2, "press", "START", false, false) {
		t.Fatalf("Start suppressed by another pad's Back: %+v", actions)
	}
	pads[1].state.wButtons |= XINPUT_GAMEPAD_BACK
	if actions := nav.update(pads, true, now.Add(2*time.Millisecond)); len(actions) != 0 {
		t.Fatalf("local kill chord emitted navigation: %+v", actions)
	}
}

func TestLauncherRightStickVolumeWorksOnSecondPad(t *testing.T) {
	var nav launcherNavigation
	now := time.Unix(1, 0)
	pads := []launcherPadSample{{id: 1}, {id: 2}}
	nav.update(pads, true, now)
	pads[1].state.sThumbRY = 22000
	if actions := nav.update(pads, true, now.Add(time.Millisecond)); !hasLauncherAction(actions, 2, "volume", "up", false, false) {
		t.Fatalf("second pad volume did not work: %+v", actions)
	}
}

func hasLauncherAction(actions []launcherPadAction, id uint64, kind, name string, down, cancelled bool) bool {
	for _, action := range actions {
		if action.source == id && action.kind == kind && action.name == name && action.down == down && action.cancelled == cancelled {
			return true
		}
	}
	return false
}
