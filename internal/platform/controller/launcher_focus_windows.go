//go:build windows

package controller

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procLauncherAttachThreadInput = user32.NewProc("AttachThreadInput")
	procLauncherPeekMessage       = user32.NewProc("PeekMessageW")
	launcherWindowCallback        = windows.NewCallback(findLauncherWindow)
)

type launcherWindowSearch struct{ hwnd uintptr }

type launcherMessage struct {
	window         uintptr
	message        uint32
	wParam, lParam uintptr
	time           uint32
	x, y           int32
	private        uint32
}

func findLauncherWindow(hwnd uintptr, parameter uintptr) uintptr {
	search := (*launcherWindowSearch)(unsafe.Pointer(parameter))
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != windows.GetCurrentProcessId() {
		return 1
	}
	var name [256]uint16
	n, _, _ := procKeyboardGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if n != 0 && windows.UTF16ToString(name[:]) == LauncherWindowClass {
		search.hwnd = hwnd
		return 0
	}
	return 1
}

// FocusLauncherWindow targets StageBox's WebView window, never its wallpaper.
// Keep attach/set/detach on one OS thread so Go scheduling cannot leave input
// queues joined when reclaiming focus from Steam's UI thread.
func FocusLauncherWindow() bool {
	if LauncherHasFocus() {
		return true
	}
	search := launcherWindowSearch{}
	_ = windows.EnumWindows(launcherWindowCallback, unsafe.Pointer(&search))
	if search.hwnd == 0 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// AttachThreadInput requires a message queue on the calling thread.
	// PM_NOREMOVE creates it without consuming Wails/keyboard messages.
	var message launcherMessage
	procLauncherPeekMessage.Call(uintptr(unsafe.Pointer(&message)), ^uintptr(0), 0, 0, 0)
	foreground, _, _ := procKeyboardGetForegroundWindow.Call()
	threadID, _, _ := procGetWindowThreadProcessID.Call(foreground, 0)
	caller := uintptr(windows.GetCurrentThreadId())
	if threadID != 0 && threadID != caller {
		attached, _, _ := procLauncherAttachThreadInput.Call(caller, threadID, 1)
		if attached != 0 {
			defer procLauncherAttachThreadInput.Call(caller, threadID, 0)
		}
	}
	procSetForegroundWindow.Call(search.hwnd)
	return LauncherHasFocus()
}
