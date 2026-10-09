//go:build !windows

package games

import (
	"os"
	"syscall"
	"time"
)

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func waitForProcessTreeExit(_ int, extraWait time.Duration) {
	if extraWait > 0 {
		time.Sleep(extraWait)
	}
}
