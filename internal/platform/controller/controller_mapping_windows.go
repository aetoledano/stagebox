//go:build windows

package controller

import (
	"bytes"
	"fmt"
	lgx "launcher/internal/logx"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (s *sdlBackend) mappingInput(handle uintptr) (mappingInput, error) {
	joystick, _, _ := s.getJoystick.Call(handle)
	if joystick == 0 {
		return mappingInput{}, fmt.Errorf("controller raw input is unavailable")
	}
	axes, _, _ := s.numJoystickAxes.Call(joystick)
	buttons, _, _ := s.numJoystickButtons.Call(joystick)
	hats, _, _ := s.numJoystickHats.Call(joystick)
	if int32(axes) < 0 || axes > 32 || int32(buttons) < 0 || buttons > 128 || int32(hats) < 0 || hats > 16 {
		return mappingInput{}, fmt.Errorf("unsupported controller input counts")
	}
	in := mappingInput{axes: make([]int16, int(axes)), buttons: make([]bool, int(buttons)), hats: make([]uint8, int(hats))}
	for i := range in.axes {
		v, _, _ := s.joystickAxis.Call(joystick, uintptr(i))
		in.axes[i] = int16(v)
	}
	for i := range in.buttons {
		v, _, _ := s.joystickButton.Call(joystick, uintptr(i))
		in.buttons[i] = v&0xff != 0
	}
	for i := range in.hats {
		v, _, _ := s.joystickHat.Call(joystick, uintptr(i))
		in.hats[i] = uint8(v)
	}
	return in, nil
}

func (b *ControllerBridge) IsMapping() bool {
	return b != nil && b.mappingActive.Load()
}

func (b *ControllerBridge) BeginControllerMapping(id string) (ControllerMappingState, error) {
	b.mappingMu.Lock()
	defer b.mappingMu.Unlock()
	if b.mappingCapture != nil || b.testing.Load() != nil || b.virtualInput.Load() != nil {
		return ControllerMappingState{}, fmt.Errorf("close the current controller test or game first")
	}
	if inventory := b.inventory.Load(); inventory != nil {
		for _, device := range inventory.devices {
			if device.info.ID != id || !device.info.Connected {
				continue
			}
			if device.info.NativeXInput || !validMappingGUID(device.guid) {
				return ControllerMappingState{}, fmt.Errorf("this controller already provides native Xbox input; custom mapping requires a physical SDL HID controller")
			}
			b.mappingCapture = newMappingCapture(id, device.guid, device.info.Name, device.instance, device.revision)
			b.mappingShortcut.Store(nil)
			b.mappingActive.Store(true)
			b.testing.Store(&controllerTestRequest{id: id})
			b.resetInput.Store(true)
			b.rumbleTarget.Store(nil)
			return b.mappingCapture.state(), nil
		}
	}
	return ControllerMappingState{}, fmt.Errorf("controller is disconnected; reconnect it before mapping")
}

func (b *ControllerBridge) ControllerMappingAction(id, action string) (ControllerMappingState, error) {
	b.mappingMu.Lock()
	defer b.mappingMu.Unlock()
	c := b.mappingCapture
	if c == nil || c.id != id {
		return ControllerMappingState{}, fmt.Errorf("controller mapping is no longer active")
	}
	if action == "cancel" {
		b.mappingCapture = nil
		b.mappingActive.Store(false)
		b.testing.Store(nil)
		b.resetInput.Store(true)
		return ControllerMappingState{}, nil
	}
	if c.phase == "error" {
		return c.state(), fmt.Errorf("%s", c.message)
	}
	switch action {
	case "skip":
		if err := c.skip(); err != nil {
			return c.state(), err
		}
	case "previous":
		c.previous()
	case "save":
		row, err := c.row()
		if err != nil {
			return c.state(), err
		}
		// File IO stays on this request goroutine. SDL uses TryLock for capture
		// so it can reload/publish inventory while this save waits for application.
		if err := saveControllerMappingOverride(c.guid, row); err != nil {
			return c.state(), err
		}
		if err := b.waitForControllerMapping(c, row); err != nil {
			c.phase = "error"
			c.message = err.Error()
			b.testing.Store(nil)
			b.resetInput.Store(true)
			return c.state(), err
		}
		lgx.Infof("controller mapping saved id=%s guid=%s; matching controllers use Virtual Xbox", c.id, c.guid)
		b.mappingCapture = nil
		b.mappingActive.Store(false)
		b.testing.Store(nil)
		b.resetInput.Store(true)
		return ControllerMappingState{ID: id, Phase: "saved"}, nil
	default:
		return c.state(), fmt.Errorf("unknown controller mapping action")
	}
	return c.state(), nil
}

func (b *ControllerBridge) waitForControllerMapping(c *controllerMappingCapture, row string) error {
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		if inventory := b.inventory.Load(); inventory != nil {
			for _, device := range inventory.devices {
				if device.info.ID == c.id && device.info.CustomMapping && sameMappingBindings(device.mapping, row) {
					if b.preferences != nil {
						if err := b.preferences.save(device.info, ControllerVirtual); err != nil {
							return fmt.Errorf("mapping saved and active, but controller preferences could not be saved: %w", err)
						}
					}
					return nil
				}
			}
		}
		select {
		case <-b.ctx.Done():
			return fmt.Errorf("mapping saved; launcher stopped before application was confirmed")
		case <-deadline.C:
			return fmt.Errorf("mapping saved but application could not be confirmed. Cancel, reconnect the controller and inspect stagebox.log")
		case <-ticker.C:
		}
	}
}

// Runs on SDL's locked owner; the mutex protects capture state from UI actions.
func (b *ControllerBridge) pollControllerMapping(s *sdlBackend, now time.Time) {
	if !b.mappingMu.TryLock() {
		return
	}
	c := b.mappingCapture
	if c == nil {
		b.mappingMu.Unlock()
		return
	}
	if !LauncherHasFocus() {
		c.since = time.Time{}
		c.skipSince = time.Time{}
		if c.phase == "press" {
			c.candidate = ""
		}
		b.mappingMu.Unlock()
		return
	}
	// A file save can hold this lock briefly; polling catches up without a FIFO.
	found := false
	if s != nil {
		for instance, info := range s.info {
			if info.ID != c.id {
				continue
			}
			found = true
			if instance != c.instance || s.mappingRevision != c.revision {
				c.phase = "error"
				c.message = "Controller connection or mapping changed. Cancel and start again."
				break
			}
			in, err := s.mappingInput(s.gamepads[instance])
			if err != nil {
				c.phase = "error"
				c.message = err.Error()
			} else {
				c.update(in, now)
			}
			break
		}
	}
	if !found {
		c.phase = "error"
		c.message = "Controller disconnected. Cancel and reconnect to try again."
	}
	state := c.state()
	if c.phase == "error" && b.testing.Swap(nil) != nil {
		b.resetInput.Store(true)
	}
	emit := !now.Before(b.nextMappingEvent)
	if emit {
		b.nextMappingEvent = now.Add(32 * time.Millisecond)
		c.command = ""
	}
	b.mappingMu.Unlock()
	if emit {
		runtime.EventsEmit(b.ctx, "controller:mapping-state", state)
	}
}

func saveControllerMappingOverride(guid, row string) error {
	paths, err := controllerDatabasePaths()
	if err != nil {
		return err
	}
	path := paths[2]
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	read := func() ([]byte, error) {
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		defer file.Close()
		return readControllerMappingData(file)
	}
	data, err := read()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(data) > controllerDatabaseLimit {
		return fmt.Errorf("controller overrides file is too large")
	}
	if _, err := validateControllerMappingData(data); err != nil {
		return err
	}
	updated := upsertMappingOverride(data, guid, row)
	if len(updated) > controllerDatabaseLimit {
		return fmt.Errorf("controller overrides file would exceed the size limit")
	}
	if _, err := validateControllerMappingData(updated); err != nil {
		return err
	}
	// Preserve the first pre-wizard file and every unrelated row/comment.
	if len(data) > 0 {
		if err := publishControllerMappingFile(filepath.Join(filepath.Dir(path), "gamecontrollerdb-overrides-before-remap.txt"), data, false); err != nil {
			return fmt.Errorf("back up controller overrides: %w", err)
		}
	}
	latest, err := read()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !bytes.Equal(data, latest) {
		return fmt.Errorf("controller overrides changed while saving; retry Save")
	}
	return publishControllerMappingFile(path, updated, true)
}

// Mark only physical model GUIDs, never SDL's generic xinput mapping namespace.
func (s *sdlBackend) applyCustomMapping(info ControllerInfo, mapping string) ControllerInfo {
	guid := strings.SplitN(mapping, ",", 2)[0]
	if !validMappingGUID(guid) {
		return info
	}
	info.MappingGUID = guid
	for _, line := range s.overrideRows {
		if mappingOverrideMatches(line, guid) && !info.NativeXInput {
			info.CustomMapping = true
			return info
		}
	}
	return info
}

func controllerOverrideRows(files []controllerMappingFile) []string {
	var rows []string
	for _, file := range files {
		if file.source != "overrides" {
			continue
		}
		for _, line := range strings.Split(string(file.data), "\n") {
			if _, _, windows := mappingRowIdentity(line); windows {
				rows = append(rows, line)
			}
		}
	}
	return rows
}
