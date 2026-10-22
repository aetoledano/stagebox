//go:build windows

package controller

import (
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (b *ControllerBridge) emitLauncherActions(actions []launcherPadAction) {
	for _, action := range actions {
		switch action.kind {
		case "button":
			runtime.EventsEmit(b.ctx, "pad:button", map[string]any{"name": action.name, "down": action.down, "cancelled": action.cancelled, "source": action.source})
		case "nav":
			runtime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": action.name, "source": action.source})
		case "press":
			runtime.EventsEmit(b.ctx, "pad:press", map[string]any{"name": action.name, "source": action.source})
		case "volume":
			if b.volumeAction != nil {
				b.volumeAction(action.name)
			}
		}
	}
}

// Sample one native backend family: XInput first, then WinMM only if no Xbox
// slots are connected, preserving fallback deduplication and selection policy.
func pollNativeLauncherPads() []launcherPadSample {
	var pads []launcherPadSample
	if xinputGetState != nil {
		for slot := uint32(0); slot < 4; slot++ {
			var state xinputState
			result, _, _ := xinputGetState.Call(uintptr(slot), uintptr(unsafe.Pointer(&state)))
			if result == 0 {
				pads = append(pads, launcherPadSample{id: uint64(1)<<32 | uint64(slot), state: state.Gamepad})
			}
		}
	}
	if len(pads) > 0 || loadLegacyJoystick() != nil {
		return pads
	}
	n, _, _ := procJoyGetNumDevs.Call()
	if n > 16 {
		n = 16
	}
	for slot := uint32(0); slot < uint32(n); slot++ {
		if state, ok := pollLegacyIndex(slot); ok {
			pads = append(pads, launcherPadSample{id: uint64(2)<<32 | uint64(slot), state: state})
		}
	}
	return pads
}
