//go:build windows

package games

import (
	lgx "launcher/internal/logx"
	"launcher/internal/platform/winapi"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
	processTreePollInterval        = 500 * time.Millisecond
	processTreeQuietPeriod         = 2 * time.Second
	processTreeWindowStartupGrace  = 30 * time.Second
)

var (
	procEnumWindows              = winapi.User32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = winapi.User32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = winapi.User32.NewProc("IsWindowVisible")
)

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return true
	}
	return exitCode == stillActive
}

func waitForProcessTreeExit(rootPID int, extraWait time.Duration) {
	if rootPID <= 0 {
		return
	}

	known := map[uint32]struct{}{uint32(rootPID): {}}
	quietSince := time.Time{}
	started := time.Now()
	sawVisibleWindow := false
	for {
		running := processTreeRunning(known)
		if len(running) > 0 {
			for pid := range running {
				known[pid] = struct{}{}
			}

			if processTreeHasVisibleWindow(running) {
				sawVisibleWindow = true
				quietSince = time.Time{}
				time.Sleep(processTreePollInterval)
				continue
			}

			if !sawVisibleWindow && time.Since(started) < processTreeWindowStartupGrace {
				quietSince = time.Time{}
				time.Sleep(processTreePollInterval)
				continue
			}

			if !sawVisibleWindow {
				lgx.Warnf("process tree saw no visible game window rootPID=%d tracked=%d running=%v", rootPID, len(known), processIDList(running))
				break
			}
		}

		if quietSince.IsZero() {
			quietSince = time.Now()
		}
		if time.Since(quietSince) >= processTreeQuietPeriod {
			break
		}
		time.Sleep(processTreePollInterval)
	}

	if extraWait > 0 {
		time.Sleep(extraWait)
	}
	lgx.Debugf("process tree exited rootPID=%d tracked=%d", rootPID, len(known))
}

func processTreeHasVisibleWindow(processes map[uint32]struct{}) bool {
	if len(processes) == 0 {
		return false
	}
	search := &visibleWindowSearch{pids: processes}
	procEnumWindows.Call(windows.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		s := (*visibleWindowSearch)(unsafe.Pointer(lparam))
		var windowPID uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if _, ok := s.pids[windowPID]; !ok {
			return 1
		}
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible != 0 {
			s.found = true
			return 0
		}
		return 1
	}), uintptr(unsafe.Pointer(search)))
	return search.found
}

type visibleWindowSearch struct {
	pids  map[uint32]struct{}
	found bool
}

func processIDList(processes map[uint32]struct{}) []uint32 {
	pids := make([]uint32, 0, len(processes))
	for pid := range processes {
		pids = append(pids, pid)
	}
	return pids
}

func processTreeRunning(known map[uint32]struct{}) map[uint32]struct{} {
	entries, err := processEntries()
	if err != nil {
		lgx.Warnf("process tree snapshot failed: %v", err)
		return nil
	}

	parents := make(map[uint32][]uint32, len(entries))
	for _, entry := range entries {
		parents[entry.parentPID] = append(parents[entry.parentPID], entry.pid)
	}

	running := make(map[uint32]struct{})
	queue := make([]uint32, 0, len(known))
	for pid := range known {
		queue = append(queue, pid)
	}

	seen := map[uint32]struct{}{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		if processExists(int(pid)) {
			running[pid] = struct{}{}
		}
		for _, child := range parents[pid] {
			queue = append(queue, child)
		}
	}
	return running
}

type processEntry struct {
	pid       uint32
	parentPID uint32
}

func processEntries() ([]processEntry, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}

	entries := []processEntry{}
	for {
		entries = append(entries, processEntry{
			pid:       entry.ProcessID,
			parentPID: entry.ParentProcessID,
		})
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err == syscall.ERROR_NO_MORE_FILES {
				break
			}
			return entries, err
		}
	}
	return entries, nil
}
