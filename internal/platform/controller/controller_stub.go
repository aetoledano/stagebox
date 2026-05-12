//go:build !windows

package controller

import "context"

type XInputBridge struct{}
type KeyboardBridge struct{}
type GameKeyMap struct {
	Buttons          map[string][]string
	LeftStick        map[string][]string
	RightStick       map[string][]string
	Mouse            string
	MouseSensitivity int
}

func StartXInputBridge(ctx context.Context) *XInputBridge { return nil }
func StartKeyboardBridge(ctx context.Context) *KeyboardBridge {
	return nil
}
func (b *XInputBridge) Pause()                           {}
func (b *XInputBridge) Resume()                          {}
func (b *XInputBridge) EnableMouseMode()                 {}
func (b *XInputBridge) EnableMouseModeForPID(pid uint32) {}
func (b *XInputBridge) DisableMouseMode()                {}
func (b *XInputBridge) EnablePairingKeyboardModeForPID(pid uint32) {
}
func (b *XInputBridge) DisablePairingKeyboardMode() {}
func (b *XInputBridge) EnableGameKeyMap(keyMap GameKeyMap) bool {
	return false
}
func (b *XInputBridge) SetGameInputTargetPID(pid uint32) {}
func (b *XInputBridge) DisableGameKeyMap()               {}
func (b *XInputBridge) Stop()                            {}
func (b *KeyboardBridge) Pause()                         {}
func (b *KeyboardBridge) Resume()                        {}
func (b *KeyboardBridge) Stop()                          {}
func ForegroundBelongsToCurrentProcess() bool {
	return false
}
func FocusProcessWindow(pid uint32) bool     { return false }
func HasVisibleWindowForPID(pid uint32) bool { return false }
func MoveCursorTo(x int, y int)              {}
func SendKeyStrokeToPID(pid uint32, key string) bool {
	return false
}
func ConnectedCount() int { return 0 }
