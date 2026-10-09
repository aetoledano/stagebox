//go:build windows

package games

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func killProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	cmd := exec.Command("taskkill.exe", "/PID", strconv.Itoa(process.Pid), "/T", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err == nil {
		return nil
	}
	killDescendantProcesses(process.Pid)
	return process.Kill()
}

func killDescendantProcesses(rootPID int) {
	entries, err := processEntries()
	if err != nil {
		return
	}
	children := make(map[uint32][]uint32, len(entries))
	for _, entry := range entries {
		children[entry.parentPID] = append(children[entry.parentPID], entry.pid)
	}

	queue := append([]uint32{}, children[uint32(rootPID)]...)
	seen := map[uint32]struct{}{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		queue = append(queue, children[pid]...)

		handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			continue
		}
		_ = windows.TerminateProcess(handle, 1)
		_ = windows.CloseHandle(handle)
	}
}
