//go:build !windows

package controller

import "context"

type ControllerBridge struct{}
type KeyboardBridge struct{}
type GameKeyMap struct {
	Buttons          map[string][]string
	LeftStick        map[string][]string
	RightStick       map[string][]string
	Mouse            string
	MouseSensitivity int
}

func StartControllerBridge(ctx context.Context, volumeAction func(string), downloadDatabase bool) *ControllerBridge {
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
func (b *KeyboardBridge) Pause()                             {}
func (b *KeyboardBridge) Resume()                            {}
func (b *KeyboardBridge) Stop()                              {}
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
func ConnectedCount() int { return 0 }
