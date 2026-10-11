package controller

// The existing input processing uses this XInput-compatible layout, regardless
// of the input backend. This struct is also the native XInput ABI payload; it
// does not expose or create a virtual controller.
type xinputGamepad struct {
	wButtons      uint16
	bLeftTrigger  byte
	bRightTrigger byte
	sThumbLX      int16
	sThumbLY      int16
	sThumbRX      int16
	sThumbRY      int16
}

const (
	XINPUT_GAMEPAD_DPAD_UP        = 0x0001
	XINPUT_GAMEPAD_DPAD_DOWN      = 0x0002
	XINPUT_GAMEPAD_DPAD_LEFT      = 0x0004
	XINPUT_GAMEPAD_DPAD_RIGHT     = 0x0008
	XINPUT_GAMEPAD_START          = 0x0010
	XINPUT_GAMEPAD_BACK           = 0x0020
	XINPUT_GAMEPAD_LEFT_THUMB     = 0x0040
	XINPUT_GAMEPAD_RIGHT_THUMB    = 0x0080
	XINPUT_GAMEPAD_LEFT_SHOULDER  = 0x0100
	XINPUT_GAMEPAD_RIGHT_SHOULDER = 0x0200
	XINPUT_GAMEPAD_A              = 0x1000
	XINPUT_GAMEPAD_B              = 0x2000
	XINPUT_GAMEPAD_X              = 0x4000
	XINPUT_GAMEPAD_Y              = 0x8000
)

// SDL3's gamepad buttons are physical positions, not printed labels. South/
// east/west/north map to the launcher's existing A/B/X/Y action vocabulary.
var sdlButtonMasks = [...]uint16{
	XINPUT_GAMEPAD_A, XINPUT_GAMEPAD_B, XINPUT_GAMEPAD_X, XINPUT_GAMEPAD_Y,
	XINPUT_GAMEPAD_BACK, 0, XINPUT_GAMEPAD_START,
	XINPUT_GAMEPAD_LEFT_THUMB, XINPUT_GAMEPAD_RIGHT_THUMB,
	XINPUT_GAMEPAD_LEFT_SHOULDER, XINPUT_GAMEPAD_RIGHT_SHOULDER,
	XINPUT_GAMEPAD_DPAD_UP, XINPUT_GAMEPAD_DPAD_DOWN,
	XINPUT_GAMEPAD_DPAD_LEFT, XINPUT_GAMEPAD_DPAD_RIGHT,
}

func normalizedSDLGamepad(buttons uint32, axes [6]int16) xinputGamepad {
	var mask uint16
	for index, target := range sdlButtonMasks {
		if buttons&(1<<index) != 0 {
			mask |= target
		}
	}
	return xinputGamepad{
		wButtons: mask, bLeftTrigger: sdlTrigger(axes[4]), bRightTrigger: sdlTrigger(axes[5]),
		sThumbLX: axes[0], sThumbLY: sdlVerticalAxis(axes[1]),
		sThumbRX: axes[2], sThumbRY: sdlVerticalAxis(axes[3]),
	}
}

func sdlVerticalAxis(value int16) int16 {
	if value == -32768 {
		return 32767
	}
	return -value
}

func sdlTrigger(value int16) byte {
	if value <= 0 {
		return 0
	}
	return byte(int32(value) * 255 / 32767)
}

func gamepadNeutral(state xinputGamepad) bool {
	const deadzone = 8500
	lx, ly, rx, ry := int64(state.sThumbLX), int64(state.sThumbLY), int64(state.sThumbRX), int64(state.sThumbRY)
	return state.wButtons == 0 && state.bLeftTrigger <= 30 && state.bRightTrigger <= 30 &&
		lx*lx+ly*ly < deadzone*deadzone && rx*rx+ry*ry < deadzone*deadzone
}

func selectGamepadID(active uint32, ids []uint32) uint32 {
	for _, id := range ids {
		if id == active {
			return active
		}
	}
	if len(ids) != 0 {
		return ids[0]
	}
	return 0
}
