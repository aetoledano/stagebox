//go:build !windows

package controller

import (
	"context"
	"fmt"
)

type ControllerBridge struct{}
type KeyboardBridge struct{}
type GameKeyMap struct {
	Buttons          map[string][]string
	LeftStick        map[string][]string
	RightStick       map[string][]string
	Mouse            string
	MouseSensitivity int
}

func StartControllerBridge(ctx context.Context, volumeAction func(string), downloadDatabase bool, guideAction ...func(string)) *ControllerBridge {
	return nil
}
func StartKeyboardBridge(ctx context.Context, volumeAction func(string)) *KeyboardBridge {
	return nil
}
func (b *ControllerBridge) Pause()                                              {}
func (b *ControllerBridge) Resume()                                             {}
func (b *ControllerBridge) EnableMouseMode()                                    {}
func (b *ControllerBridge) EnableMouseModeForPID(pid uint32)                    {}
func (b *ControllerBridge) EnableMouseModeForWindow(pid uint32, window uintptr) {}
func (b *ControllerBridge) DisableMouseMode()                                   {}
func (b *ControllerBridge) EnablePairingKeyboardModeForPID(pid uint32) {
}
func (b *ControllerBridge) DisablePairingKeyboardMode() {}
func (b *ControllerBridge) EnableGameKeyMap(keyMap GameKeyMap) bool {
	return false
}
func (b *ControllerBridge) SetGameInputTargetPID(pid uint32) {}
func (b *ControllerBridge) DisableGameKeyMap()               {}
func (b *ControllerBridge) Stop()                            {}
func VirtualInputAvailability() (bool, string) {
	return false, "Virtual Xbox input requires Windows x64."
}
func (b *ControllerBridge) EnableVirtualInput(context.Context, bool) error {
	return fmt.Errorf("virtual Xbox input requires Windows x64")
}
func (b *ControllerBridge) DisableVirtualInput() error    { return nil }
func (b *ControllerBridge) Controllers() []ControllerInfo { return []ControllerInfo{} }
func (b *ControllerBridge) SetControllerDefault(string, string) error {
	return fmt.Errorf("controller settings require Windows SDL input")
}
func (b *ControllerBridge) SelectController(string) error {
	return fmt.Errorf("controller selection requires Windows SDL input")
}
func (b *ControllerBridge) TestController(string) error { return nil }
func (b *ControllerBridge) TestControllerRumble(string) error {
	return fmt.Errorf("rumble testing requires Windows SDL input")
}
func (b *ControllerBridge) NeedsVirtualInput(bool) bool { return false }
func (b *KeyboardBridge) Pause()                        {}
func (b *KeyboardBridge) Resume()                       {}
func (b *KeyboardBridge) Stop()                         {}
func LauncherHasFocus() bool {
	// Non-Windows builds have no global input bridges; keep normal Wails actions available.
	return true
}
func FocusProcessWindow(pid uint32) bool     { return false }
func FocusLauncherWindow() bool              { return true }
func HasVisibleWindowForPID(pid uint32) bool { return false }
func MoveCursorTo(x int, y int)              {}
func SendKeyStrokeToPID(pid uint32, key string) bool {
	return false
}
func ConnectedCount() int                                         { return 0 }
func (b *ControllerBridge) HasConnectedBluetoothController() bool { return false }

func (b *ControllerBridge) IsTesting() bool                                { return false }
func (b *ControllerBridge) RememberBluetoothControllers() error            { return nil }
func (b *ControllerBridge) ControllerProfilesForBluetooth(string) []string { return nil }
func (b *ControllerBridge) RemoveControllerProfiles([]string) error {
	return fmt.Errorf("controller settings require Windows SDL input")
}
