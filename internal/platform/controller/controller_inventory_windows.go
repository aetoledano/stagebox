//go:build windows

package controller

import (
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type controllerTestRequest struct{ id string }

func (b *ControllerBridge) Controllers() []ControllerInfo {
	if b == nil {
		return []ControllerInfo{}
	}
	var devices []controllerDevice
	if inventory := b.inventory.Load(); inventory != nil {
		devices = inventory.devices
	}
	return b.preferences.list(devices)
}

func (b *ControllerBridge) SetControllerDefault(id, mode string) error {
	for _, info := range b.Controllers() {
		if info.ID == id {
			return b.preferences.save(info, mode)
		}
	}
	return fmt.Errorf("controller not found; reconnect it and retry")
}

func (b *ControllerBridge) SelectController(id string) error {
	for _, info := range b.Controllers() {
		if info.ID == id && info.Connected {
			b.selectController.Store(&controllerTestRequest{id: id})
			return nil
		}
	}
	return fmt.Errorf("controller is disconnected")
}

func (b *ControllerBridge) TestController(id string) error {
	if id == "" {
		b.testing.Store(nil)
		b.resetInput.Store(true)
		return nil
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
func (b *ControllerBridge) publishInventory(devices []controllerDevice, now time.Time) {
	b.inventory.Store(&controllerInventory{devices: devices})
	if !now.Before(b.nextInventoryEvent) {
		b.nextInventoryEvent = now.Add(250 * time.Millisecond)
		runtime.EventsEmit(b.ctx, "controllers:update", b.Controllers())
	}
	if test := b.testing.Load(); test != nil && !now.Before(b.nextTestEvent) {
		b.nextTestEvent = now.Add(32 * time.Millisecond)
		state := ControllerState{ID: test.id, Buttons: []string{}}
		for _, device := range devices {
			if device.info.ID == test.id {
				state = diagnosticState(test.id, device.state)
				break
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

func (b *ControllerBridge) IsTesting() bool { return b != nil && b.testing.Load() != nil }
