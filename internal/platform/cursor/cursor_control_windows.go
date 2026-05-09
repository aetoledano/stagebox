//go:build windows

package cursor

import (
	"launcher/internal/platform/winapi"
	"sync"
	"time"
	"unsafe"
)

var procShowCursor = winapi.User32.NewProc("ShowCursor")
var procSetCursorPos = winapi.User32.NewProc("SetCursorPos")
var procGetSystemMetrics = winapi.User32.NewProc("GetSystemMetrics")
var procMouseEvent = winapi.User32.NewProc("mouse_event")
var procCreateCursor = winapi.User32.NewProc("CreateCursor")
var procSetSystemCursor = winapi.User32.NewProc("SetSystemCursor")
var procSystemParametersInfo = winapi.User32.NewProc("SystemParametersInfoW")

const (
	smCXScreen              = 0
	smCYScreen              = 1
	mouseEventFMove         = 0x0001
	cursorNudgeDelta        = 1
	ocrNormal               = 32512
	ocrIBeam                = 32513
	ocrWait                 = 32514
	ocrCross                = 32515
	ocrUp                   = 32516
	ocrSizeNWSE             = 32642
	ocrSizeNESW             = 32643
	ocrSizeWE               = 32644
	ocrSizeNS               = 32645
	ocrSizeAll              = 32646
	ocrNo                   = 32648
	ocrHand                 = 32649
	ocrAppStarting          = 32650
	ocrHelp                 = 32651
	spiSetCursors           = 0x0057
	systemCursorMaskBytes32 = 128
)

var cursorIDs = []uintptr{
	ocrNormal,
	ocrIBeam,
	ocrWait,
	ocrCross,
	ocrUp,
	ocrSizeNWSE,
	ocrSizeNESW,
	ocrSizeWE,
	ocrSizeNS,
	ocrSizeAll,
	ocrNo,
	ocrHand,
	ocrAppStarting,
	ocrHelp,
}

var (
	systemCursorMu     sync.Mutex
	systemCursorHidden bool
)

// Hides/shows cursor globally using the display-count semantics:
// visible when count >= 0, hidden when count < 0.
func SetCursorHidden(hidden bool) {
	setCursorHidden(hidden)
}

func ForceCursorHiddenFor(duration time.Duration) {
	HideSystemCursor()

	deadline := time.Now().Add(duration)
	for {
		setCursorHidden(true)
		if duration <= 0 || time.Now().After(deadline) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func ForceCursorVisible() {
	RestoreSystemCursor()
	setCursorHidden(false)

	x, y := screenCenter()
	procSetCursorPos.Call(uintptr(x), uintptr(y))

	// On mouse-less Windows systems, ShowCursor alone may not materialize the
	// pointer. A tiny synthetic mouse move makes Windows treat the cursor as
	// active without requiring a physical mouse.
	procMouseEvent.Call(mouseEventFMove, cursorNudgeDelta, 0, 0, 0)
	procSetCursorPos.Call(uintptr(x), uintptr(y))

	setCursorHidden(false)
}

func MoveToBottomRight() {
	x, y := screenBottomRight()
	procSetCursorPos.Call(uintptr(x), uintptr(y))
}

func HideSystemCursor() {
	systemCursorMu.Lock()
	defer systemCursorMu.Unlock()

	if systemCursorHidden {
		return
	}

	changed := false
	for _, cursorID := range cursorIDs {
		if setTransparentSystemCursor(cursorID) {
			changed = true
		}
	}
	systemCursorHidden = changed
}

func setTransparentSystemCursor(cursorID uintptr) bool {
	andMask := make([]byte, systemCursorMaskBytes32)
	xorMask := make([]byte, systemCursorMaskBytes32)
	for i := range andMask {
		andMask[i] = 0xFF
	}

	cursor, _, _ := procCreateCursor.Call(
		0,
		0,
		0,
		32,
		32,
		uintptr(unsafe.Pointer(&andMask[0])),
		uintptr(unsafe.Pointer(&xorMask[0])),
	)
	if cursor == 0 {
		return false
	}

	if r, _, _ := procSetSystemCursor.Call(cursor, cursorID); r == 0 {
		return false
	}
	return true
}

func RestoreSystemCursor() {
	systemCursorMu.Lock()
	defer systemCursorMu.Unlock()

	if !systemCursorHidden {
		return
	}

	procSystemParametersInfo.Call(spiSetCursors, 0, 0, 0)
	systemCursorHidden = false
}

func setCursorHidden(hidden bool) {
	if hidden {
		for {
			r, _, _ := procShowCursor.Call(0) // FALSE
			if int32(r) < 0 {
				break // now hidden
			}
		}
	} else {
		for {
			r, _, _ := procShowCursor.Call(1) // TRUE
			if int32(r) >= 0 {
				break // now visible
			}
		}
	}
}

func screenCenter() (int32, int32) {
	width, _, _ := procGetSystemMetrics.Call(smCXScreen)
	height, _, _ := procGetSystemMetrics.Call(smCYScreen)
	if width == 0 || height == 0 {
		return 0, 0
	}
	return int32(width / 2), int32(height / 2)
}

func screenBottomRight() (int32, int32) {
	width, _, _ := procGetSystemMetrics.Call(smCXScreen)
	height, _, _ := procGetSystemMetrics.Call(smCYScreen)
	if width == 0 || height == 0 {
		return 0, 0
	}
	return int32(width - 1), int32(height - 1)
}
