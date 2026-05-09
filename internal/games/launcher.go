package games

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"sync"
)

var (
	curMu  sync.Mutex
	curCmd *exec.Cmd
)

type RunningGame struct {
	Done   <-chan error
	PID    <-chan int
	Exited <-chan struct{}
}

func KillRunning() error {
	curMu.Lock()
	defer curMu.Unlock()
	if curCmd == nil || curCmd.Process == nil {
		return errors.New("no game running")
	}
	// Best-effort kill. (Later: upgrade to a Job Object to kill trees.)
	return curCmd.Process.Kill()
}

func LaunchGame(game *GameData) RunningGame {
	done := make(chan error, 1) // buffer 1 so the goroutine never blocks on send
	pid := make(chan int, 1)
	exited := make(chan struct{})
	result := RunningGame{
		Done:   done,
		PID:    pid,
		Exited: exited,
	}

	if game == nil || game.Executable == "" {
		done <- errors.New("invalid game data")
		close(done)
		close(pid)
		close(exited)
		return result
	}

	lockPath := os.TempDir() + "/" + game.ID.String() + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			done <- errors.New("already running")
		} else {
			done <- err
		}
		close(done)
		close(pid)
		close(exited)
		return result
	}

	go func() {
		defer func() {
			_ = lockFile.Close()
			_ = os.Remove(lockPath)
			close(done)
			close(exited)
		}()

		cmd := exec.Command(game.Executable)
		cmd.Dir = game.StartsOn
		curMu.Lock()
		curCmd = cmd
		curMu.Unlock()
		// cmd.Args = append([]string{game.Executable}, game.Args...)

		if err := cmd.Start(); err != nil {
			curMu.Lock()
			curCmd = nil
			curMu.Unlock()
			close(pid)
			done <- err
			return
		}
		pid <- cmd.Process.Pid
		close(pid)
		done <- cmd.Wait()

		curMu.Lock()
		curCmd = nil
		curMu.Unlock()
	}()

	return result
}
