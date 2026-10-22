//go:build windows

package controller

import (
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (b *ControllerBridge) SetControllerMappingShortcut(id string) error {
	if id == "" {
		b.mappingShortcut.Store(nil)
		return nil
	}
	if b.IsTesting() || b.virtualInput.Load() != nil {
		return fmt.Errorf("close the current controller capture or game first")
	}
	if inventory := b.inventory.Load(); inventory != nil {
		for _, device := range inventory.devices {
			if device.info.ID == id && device.info.Connected && !device.info.NativeXInput && validMappingGUID(device.guid) {
				b.mappingShortcut.Store(&controllerMappingShortcut{id: id, instance: device.instance, button: -1})
				return nil
			}
		}
	}
	return fmt.Errorf("controller is disconnected or does not support custom mapping")
}

// Called once per poll on SDL's owner. Only the selected source is deferred;
// another controller/keyboard can keep navigating the settings normally.
func (b *ControllerBridge) mappingShortcutActions(sdl *sdlBackend, actions []launcherPadAction, enabled bool, now time.Time) []launcherPadAction {
	s := b.mappingShortcut.Load()
	if s == nil {
		return actions
	}
	source := uint64(3)<<32 | uint64(s.instance)
	if !enabled || sdl == nil || sdl.info[s.instance].ID != s.id || sdl.gamepads[s.instance] == 0 {
		s.armed = false
		s.since = time.Time{}
		s.tap = nil
		return actions
	}
	in, err := sdl.mappingInput(sdl.gamepads[s.instance])
	if err != nil {
		return actions
	}
	wasHeld := !s.since.IsZero()
	fire, released := s.update(in.buttons, now)
	deferTarget := wasHeld || !s.since.IsZero() || s.fired || !s.armed
	result := make([]launcherPadAction, 0, len(actions)+2)
	for _, action := range actions {
		if action.source != source {
			result = append(result, action)
			continue
		}
		if deferTarget {
			if s.armed && !s.fired && s.tap == nil && (action.kind == "press" || action.kind == "nav" || (action.kind == "button" && action.down)) {
				saved := action
				s.tap = &saved
			}
			continue
		}
		result = append(result, action)
	}
	if fire {
		s.tap = nil
		runtime.EventsEmit(b.ctx, "controller:mapping-shortcut", map[string]string{"id": s.id})
	} else if released && s.tap != nil && !s.fired {
		// Scope delayed taps to this settings view. A tap released after the
		// modal closed must never activate an underlying game card.
		runtime.EventsEmit(b.ctx, "controller:mapping-shortcut-tap", map[string]string{"id": s.id, "kind": s.tap.kind, "name": s.tap.name})
		s.tap = nil
	}
	return result
}
