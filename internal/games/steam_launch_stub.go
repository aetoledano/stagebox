//go:build !windows

package games

import "errors"

func LaunchSteamSession(steamExe string) RunningGame {
	done := make(chan error, 1)
	pid := make(chan int)
	exited := make(chan struct{})
	done <- errors.New("Steam is only implemented on Windows")
	close(done)
	close(pid)
	close(exited)
	return RunningGame{Done: done, PID: pid, Exited: exited}
}
