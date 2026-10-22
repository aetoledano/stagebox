//go:build windows

package controller

import (
	"fmt"
	lgx "launcher/internal/logx"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type controllerTestRequest struct{ id string }

// Status reads consume the immutable live SDL snapshot, never saved profiles,
// SDL APIs on another thread, or paired-but-disconnected Windows PnP records.
func (b *ControllerBridge) HasConnectedBluetoothController() bool {
	if b == nil {
		return false
	}
	if inventory := b.inventory.Load(); inventory != nil {
		return hasConnectedBluetoothController(inventory.devices)
	}
	return false
}

func (b *ControllerBridge) Controllers() []ControllerInfo {
	if b == nil {
		return []ControllerInfo{}
	}
	var devices []controllerDevice
	if inventory := b.inventory.Load(); inventory != nil {
		devices = inventory.devices
	}
	result := b.preferences.list(devices)
	if inventory := b.inventory.Load(); inventory != nil {
		for i, info := range result {
			if info.NativeXInput || !validMappingGUID(info.MappingGUID) {
				continue
			}
			result[i].CustomMapping = false
			for _, row := range inventory.overrides {
				if mappingOverrideMatches(row, info.MappingGUID) {
					result[i].CustomMapping = true
					result[i].Mode = ControllerVirtual
					result[i].Known = true
					break
				}
			}
		}
	}
	return result
}

func (b *ControllerBridge) SetControllerDefault(id, mode string) error {
	for _, info := range b.Controllers() {
		if info.ID == id {
			if !info.Connected {
				return fmt.Errorf("controller disconnected; reconnect it to change settings")
			}
			return b.preferences.save(info, mode)
		}
	}
	return fmt.Errorf("controller not found; reconnect it and retry")
}

// Called from Wails, never the SDL owner: upgrade existing connected profiles
// with their pairing address without creating defaults for unknown devices.
func (b *ControllerBridge) RememberBluetoothControllers() error {
	if b == nil || b.preferences == nil {
		return nil
	}
	if inventory := b.inventory.Load(); inventory != nil {
		return b.preferences.rememberBluetooth(inventory.devices)
	}
	return nil
}

// Persist links for already-saved pads soon after discovery, including when the
// settings UI is never opened. File IO stays off SDL's locked polling thread.
func (b *ControllerBridge) syncBluetoothControllers() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastError := ""
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			if err := b.RememberBluetoothControllers(); err != nil {
				if err.Error() != lastError {
					lgx.Warnf("controller Bluetooth association save failed: %v", err)
					lastError = err.Error()
				}
			} else {
				lastError = ""
			}
		}
	}
}

func (b *ControllerBridge) ControllerProfilesForBluetooth(address string) []string {
	var ids []string
	if !validBluetoothAddress(address) {
		return ids
	}
	for _, info := range b.Controllers() {
		if info.BluetoothAddress == address {
			ids = append(ids, info.ID)
		}
	}
	return ids
}

// Persisted pairing-address metadata survives Windows tearing down HID nodes.
// Manual removal separately checks current connection/pairing in App.
func (b *ControllerBridge) RemoveControllerProfiles(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return b.preferences.remove(ids)
}

func (b *ControllerBridge) SelectController(id string) error {
	for _, info := range b.Controllers() {
		if info.ID == id && info.Connected {
			b.rumbleTarget.Store(nil)
			b.selectController.Store(&controllerTestRequest{id: id})
			return nil
		}
	}
	return fmt.Errorf("controller is disconnected")
}

func (b *ControllerBridge) TestController(id string) error {
	b.rumbleTarget.Store(nil)
	if id == "" {
		b.mappingShortcut.Store(nil)
		b.mappingMu.Lock()
		b.mappingCapture = nil
		b.mappingActive.Store(false)
		b.mappingMu.Unlock()
		b.testing.Store(nil)
		b.resetInput.Store(true)
		return nil
	}
	if b.IsMapping() {
		return fmt.Errorf("finish or cancel custom mapping before testing")
	}
	if err := b.SelectController(id); err != nil {
		return err
	}
	b.testing.Store(&controllerTestRequest{id: id})
	b.resetInput.Store(true)
	return nil
}

// Called only on SDL's owner. The UI sees immutable snapshots; profiles and
// file IO are handled by Wails request goroutines, not by SDL callbacks.
func (b *ControllerBridge) publishInventory(devices []controllerDevice, now time.Time, overrides ...[]string) {
	snapshot := &controllerInventory{devices: devices}
	if len(overrides) > 0 {
		snapshot.overrides = overrides[0]
	}
	b.inventory.Store(snapshot)
	if lost := b.releaseMissingTest(devices); lost != nil {
		lgx.Infof("controller test grab released id=%s reason=disconnected", lost.id)
		runtime.EventsEmit(b.ctx, "controller:test-released", map[string]string{"id": lost.id, "reason": "disconnected"})
	}
	if !now.Before(b.nextInventoryEvent) {
		b.nextInventoryEvent = now.Add(250 * time.Millisecond)
		controllers := b.Controllers()
		runtime.EventsEmit(b.ctx, "controllers:update", controllers)
		// A native foreground owner can be ready before WebView has DOM focus.
		// Reoffer unknown pads until the UI can show them or a choice is saved.
		if b.paused.Load() == 0 && b.mouse.Load() == 0 && b.keyboard.Load() == 0 && b.gameKeys.Load() == 0 && b.testing.Load() == nil && LauncherHasFocus() {
			for _, info := range controllers {
				if info.Connected && !info.Known {
					runtime.EventsEmit(b.ctx, "controller:prompt-ready", map[string]string{"id": info.ID})
				}
			}
		}
	}
}

// Release only the missing test lifetime. Other connected pads must immediately
// regain normal navigation; a reconnect never silently grabs input again.
func (b *ControllerBridge) releaseMissingTest(devices []controllerDevice) *controllerTestRequest {
	test := b.testing.Load()
	if test == nil {
		return nil
	}
	for _, device := range devices {
		if device.info.ID == test.id && device.info.Connected {
			return nil
		}
	}
	if !b.testing.CompareAndSwap(test, nil) {
		return nil
	}
	b.rumbleTarget.Store(nil)
	b.resetInput.Store(true)
	return test
}

func (b *ControllerBridge) publishTestState(s *sdlBackend, now time.Time) {
	if test := b.testing.Load(); test != nil && !now.Before(b.nextTestEvent) {
		b.nextTestEvent = now.Add(32 * time.Millisecond)
		state := ControllerState{ID: test.id, Buttons: []string{}}
		if s != nil {
			for id, info := range s.info {
				if info.ID == test.id {
					state = diagnosticState(test.id, s.states[id])
					// Guide is tracked outside the normalized XInput payload.
					// Include its held state only in this controller's diagnostics.
					if s.guides[id] {
						state.Buttons = append(state.Buttons, "GUIDE")
					}
					sources := triggerMappingSources(s.mappings[id])
					state.LeftTriggerSource, state.RightTriggerSource = sources[0], sources[1]
					handle := s.gamepads[id]
					left, _, _ := s.axis.Call(handle, 4)
					right, _, _ := s.axis.Call(handle, 5)
					state.SDLLeftTrigger, state.SDLRightTrigger = int16(left), int16(right)
					joystick, _, _ := s.getJoystick.Call(handle)
					if joystick != 0 {
						count, _, _ := s.numJoystickAxes.Call(joystick)
						if int32(count) >= 0 && int32(count) <= 32 {
							state.RawAxes = make([]int16, int(count))
							for axis := range state.RawAxes {
								value, _, _ := s.joystickAxis.Call(joystick, uintptr(axis))
								state.RawAxes[axis] = int16(value)
							}
						}
					}
					if b.loggedTest != test {
						lgx.Infof("controller input test id=%s name=%q LT=%q RT=%q rawAxes=%v mapping=%q", test.id, info.Name, sources[0], sources[1], state.RawAxes, s.mappings[id])
						b.loggedTest = test
					}
					break
				}
			}
		}
		runtime.EventsEmit(b.ctx, "controller:test-state", state)
	}
}

func (b *ControllerBridge) virtualRoutes(gameEnabled bool) []controllerDevice {
	var routes []controllerDevice
	if inventory := b.inventory.Load(); inventory != nil {
		for _, device := range inventory.devices {
			info := b.preferences.apply(device.info)
			if controllerUsesVirtual(info, gameEnabled) {
				routes = append(routes, device)
			}
		}
	}
	return routes
}

func (b *ControllerBridge) NeedsVirtualInput(gameEnabled bool) bool {
	return len(b.virtualRoutes(gameEnabled)) > 0
}

func (b *ControllerBridge) IsTesting() bool {
	return b != nil && (b.testing.Load() != nil || b.mappingActive.Load())
}
