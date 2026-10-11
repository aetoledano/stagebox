package controller

import (
	"testing"
	"unsafe"
)

func TestSDLGamepadFaceButtonsAndGuide(t *testing.T) {
	for _, scenario := range []struct {
		button uint32
		want   uint16
	}{
		{1 << 0, XINPUT_GAMEPAD_A}, {1 << 1, XINPUT_GAMEPAD_B},
		{1 << 2, XINPUT_GAMEPAD_X}, {1 << 3, XINPUT_GAMEPAD_Y},
		{1 << 5, 0},  // Guide must not become Start or a launcher action.
		{1 << 15, 0}, // Extended device-specific buttons do not alias core buttons.
	} {
		if got := normalizedSDLGamepad(scenario.button, [6]int16{}).wButtons; got != scenario.want {
			t.Fatalf("SDL buttons %#x: got %#x, want %#x", scenario.button, got, scenario.want)
		}
	}
	state := normalizedSDLGamepad(1<<4|1<<6|1<<7|1<<8|1<<9|1<<10|1<<11|1<<14, [6]int16{})
	want := uint16(XINPUT_GAMEPAD_BACK | XINPUT_GAMEPAD_START | XINPUT_GAMEPAD_LEFT_THUMB | XINPUT_GAMEPAD_RIGHT_THUMB |
		XINPUT_GAMEPAD_LEFT_SHOULDER | XINPUT_GAMEPAD_RIGHT_SHOULDER | XINPUT_GAMEPAD_DPAD_UP | XINPUT_GAMEPAD_DPAD_RIGHT)
	if state.wButtons != want {
		t.Fatalf("chord: got %#x, want %#x", state.wButtons, want)
	}
}

func TestSDLGamepadAxesAndTriggers(t *testing.T) {
	state := normalizedSDLGamepad(0, [6]int16{-32768, -32768, 32767, 32767, 0, 32767})
	if state.sThumbLX != -32768 || state.sThumbLY != 32767 || state.sThumbRX != 32767 || state.sThumbRY != -32767 {
		t.Fatalf("wrong stick orientation/endpoints: %+v", state)
	}
	if state.bLeftTrigger != 0 || state.bRightTrigger != 255 {
		t.Fatalf("trigger endpoints: %+v", state)
	}
	state = normalizedSDLGamepad(0, [6]int16{0, -1000, 0, 1000, -32768, 16384})
	if state.sThumbLY != 1000 || state.sThumbRY != -1000 || state.bLeftTrigger != 0 || state.bRightTrigger != 127 {
		t.Fatalf("axis inversion/trigger scaling: %+v", state)
	}
	if size := unsafe.Sizeof(xinputGamepad{}); size != 12 {
		t.Fatalf("native XInput payload layout changed: %d bytes", size)
	}
}

func TestControllerDeviceChangeRequiresNeutral(t *testing.T) {
	active := uint32(7)
	if got := selectGamepadID(active, []uint32{8, 7}); got != 7 {
		t.Fatalf("changed a still-connected pad to %d", got)
	}
	active = selectGamepadID(active, []uint32{8})
	if active != 8 {
		t.Fatalf("disconnect did not select remaining pad: %d", active)
	}
	var gate launcherInputGate
	held := normalizedSDLGamepad(1, [6]int16{})
	if gate.allow(true, gamepadNeutral(held)) {
		t.Fatal("held A from replacement controller was accepted")
	}
	if gate.allow(true, gamepadNeutral(xinputGamepad{})) {
		t.Fatal("neutral rearm emitted an action")
	}
	if !gate.allow(true, gamepadNeutral(held)) {
		t.Fatal("fresh A after neutral was blocked")
	}
	if got := selectGamepadID(active, nil); got != 0 {
		t.Fatalf("disconnected inventory retained id=%d", got)
	}
	for _, state := range []xinputGamepad{
		{bLeftTrigger: 31}, {bRightTrigger: 255}, {sThumbLX: 9000}, {sThumbRY: 9000},
		{sThumbLX: -32768, sThumbLY: -32768},
	} {
		if gamepadNeutral(state) {
			t.Fatalf("non-neutral replacement state accepted: %+v", state)
		}
	}
}
