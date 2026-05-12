//go:build windows

package games

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
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
	return process.Kill()
}
