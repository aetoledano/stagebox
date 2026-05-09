//go:build windows

package games

import (
	"errors"
	"os/exec"
	"path/filepath"
)

func LaunchSteamSession(steamExe string) RunningGame {
	done := make(chan error, 1)
	pid := make(chan int, 1)
	exited := make(chan struct{})
	result := RunningGame{Done: done, PID: pid, Exited: exited}

	go func() {
		defer func() {
			close(done)
			close(exited)
		}()

		if steamExe == "" {
			close(pid)
			done <- errors.New("missing Steam executable")
			return
		}

		cmd := exec.Command(steamExe)
		cmd.Dir = filepath.Dir(steamExe)
		if err := cmd.Start(); err != nil {
			close(pid)
			done <- err
			return
		}
		pid <- cmd.Process.Pid
		close(pid)
		done <- cmd.Wait()
	}()

	return result
}
