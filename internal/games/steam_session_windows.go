//go:build windows

package games

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSteamGetWindowText = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowTextW")
	procSteamGetClassName  = windows.NewLazySystemDLL("user32.dll").NewProc("GetClassNameW")
	steamSessionUICallback = windows.NewCallback(inspectSteamSessionWindow)
)

type steamClientUIActivity struct {
	visible       bool
	bigPicture    bool
	game          bool
	gameProcesses int
}

type steamSessionWindowSearch struct {
	processes map[uint32]*steamTrackedProcess
	activity  steamClientUIActivity
	err       error
}

func (t *steamTracker) clientUIActivity() (steamClientUIActivity, error) {
	search := steamSessionWindowSearch{processes: t.processes}
	for _, process := range t.processes {
		if process.alive && !process.client && !steamBackgroundHelper(process.path) {
			search.activity.gameProcesses++
		}
	}
	if err := windows.EnumWindows(steamSessionUICallback, unsafe.Pointer(&search)); err != nil {
		return search.activity, err
	}
	return search.activity, search.err
}

func inspectSteamSessionWindow(hwnd uintptr, parameter uintptr) uintptr {
	search := (*steamSessionWindowSearch)(unsafe.Pointer(parameter))
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	process := search.processes[pid]
	if process == nil || !process.alive {
		return 1
	}
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	iconic, _, _ := procSteamIsIconic.Call(hwnd)
	if visible == 0 || iconic != 0 {
		return 1
	}
	// Recheck the retained handle so a reused PID cannot sustain the session.
	if err := process.refresh(); err != nil {
		search.err = fmt.Errorf("verify Steam window owner pid=%d: %w", pid, err)
		return 1
	}
	if !process.alive {
		return 1
	}
	if !process.client && !strings.EqualFold(filepath.Base(process.path), "steamwebhelper.exe") {
		if !steamBackgroundHelper(process.path) {
			search.activity.game = true
		}
		return 1
	}
	var title [512]uint16
	n, _, _ := procSteamGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
	if n == 0 {
		return 1 // Invisible tray/bootstrap utility windows are not a UI session.
	}
	var class [256]uint16
	procSteamGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	search.activity.visible = true
	if steamBigPictureWindow(windows.UTF16ToString(class[:]), windows.UTF16ToString(title[:])) {
		search.activity.bigPicture = true
	}
	return 1
}
