//go:build windows

package games

import (
	"context"
	"fmt"
	lgx "launcher/internal/logx"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSteamShowWindowAsync = windows.NewLazySystemDLL("user32.dll").NewProc("ShowWindowAsync")
	procSteamIsIconic        = windows.NewLazySystemDLL("user32.dll").NewProc("IsIconic")
	steamUIWindowCallback    = windows.NewCallback(hideSteamUIWindow)
	steamUIBackgroundMu      sync.Mutex
	steamUIBackgroundCancel  context.CancelFunc
)

// New explicit activations cancel the settling watcher before any Steam UI
// opens. Serialize cancellation with native hides so it cannot hide new UI.
func stopSteamUIBackgrounding() {
	steamUIBackgroundMu.Lock()
	defer steamUIBackgroundMu.Unlock()
	if steamUIBackgroundCancel != nil {
		steamUIBackgroundCancel()
		steamUIBackgroundCancel = nil
	}
}

// Hide client windows immediately, including iconic windows that otherwise
// leave miniature title bars in a kiosk shell. Let late desktop/UI transitions
// settle in the background without delaying launcher restoration.
func BackgroundSteamClient(parent context.Context, steamExe string) error {
	stopSteamUIBackgrounding()
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	steamUIBackgroundMu.Lock()
	steamUIBackgroundCancel = cancel
	steamUIBackgroundMu.Unlock()
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		cancel()
		return err
	}
	tracker := &steamTracker{exe: filepath.Clean(steamExe), sessionID: sessionID, processes: map[uint32]*steamTrackedProcess{}}
	handedOff := false
	defer func() {
		if !handedOff {
			cancel()
			tracker.close()
		}
	}()
	steamUIBackgroundMu.Lock()
	if ctx.Err() != nil {
		steamUIBackgroundMu.Unlock()
		return nil
	}
	clients, _, err := tracker.sample()
	if err != nil || len(clients) == 0 {
		steamUIBackgroundMu.Unlock()
		if err != nil {
			return fmt.Errorf("inspect Steam UI before return: %w", err)
		}
		return nil // Never restart a client the user closed.
	}
	bigPicture, err := hideSteamClientUI(tracker, false)
	steamUIBackgroundMu.Unlock()
	if err != nil {
		return err
	}
	// Close Big Picture only if its window exists (including hidden/minimized
	// windows). Hiding alone would leave its controller UI session running.
	if bigPicture {
		cmd := exec.Command(steamExe, "-silent", "steam://close/bigpicture")
		cmd.Dir = filepath.Dir(steamExe)
		if err := cmd.Start(); err != nil {
			lgx.Warnf("close Steam Big Picture during return: %v", err)
		} else {
			go func() {
				if err := cmd.Wait(); err != nil {
					lgx.Warnf("Steam Big Picture close request exited: %v", err)
				}
			}()
		}
	}
	handedOff = true
	go func() {
		defer cancel()
		defer tracker.close()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			steamUIBackgroundMu.Lock()
			if ctx.Err() != nil {
				steamUIBackgroundMu.Unlock()
				return
			}
			_, _, err := tracker.sample()
			if err == nil {
				_, err = hideSteamClientUI(tracker, false)
			}
			steamUIBackgroundMu.Unlock()
			if err != nil {
				lgx.Warnf("Steam background UI settling stopped: %v", err)
				return
			}
		}
	}()
	lgx.Infof("Steam UI return: client windows hidden, Big Picture close=%v; settling asynchronously", bigPicture)
	return nil
}

type steamUIWindowSearch struct {
	processes     map[uint32]*steamTrackedProcess
	bigPicture    bool
	minimizedOnly bool
}

// Reuse the startup owner's handles across samples, including late helpers
// and client restarts. Only iconic client UI is hidden; visible sign-in/update
// dialogs and all game windows remain usable.
func hideMinimizedSteamClientUI(tracker *steamTracker) error {
	steamUIBackgroundMu.Lock()
	defer steamUIBackgroundMu.Unlock()
	if _, _, err := tracker.sample(); err != nil {
		return err
	}
	_, err := hideSteamClientUI(tracker, true)
	return err
}

func hideSteamClientUI(tracker *steamTracker, minimizedOnly bool) (bool, error) {
	search := steamUIWindowSearch{processes: map[uint32]*steamTrackedProcess{}, minimizedOnly: minimizedOnly}
	for pid, process := range tracker.processes {
		if process.alive && (process.client || strings.EqualFold(filepath.Base(process.path), "steamwebhelper.exe")) {
			search.processes[pid] = process
		}
	}
	if err := windows.EnumWindows(steamUIWindowCallback, unsafe.Pointer(&search)); err != nil {
		return search.bigPicture, fmt.Errorf("hide Steam client UI: %w", err)
	}
	return search.bigPicture, nil
}

func hideSteamUIWindow(hwnd uintptr, parameter uintptr) uintptr {
	search := (*steamUIWindowSearch)(unsafe.Pointer(parameter))
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	process := search.processes[pid]
	if process == nil {
		return 1
	}
	// A retained handle prevents PID reuse from targeting another app.
	if err := process.refresh(); err != nil || !process.alive {
		return 1
	}
	var title [512]uint16
	procSteamGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
	var class [256]uint16
	procSteamGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	if steamBigPictureWindow(windows.UTF16ToString(class[:]), windows.UTF16ToString(title[:])) {
		search.bigPicture = true
	}
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if visible != 0 {
		if search.minimizedOnly {
			iconic, _, _ := procSteamIsIconic.Call(hwnd)
			if iconic == 0 {
				return 1 // Keep visible sign-in/update/first-run dialogs usable.
			}
		}
		const showHide = 0
		procSteamShowWindowAsync.Call(hwnd, showHide)
	}
	return 1
}
