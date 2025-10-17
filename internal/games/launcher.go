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

func KillRunning() error {
	curMu.Lock()
	defer curMu.Unlock()
	if curCmd == nil || curCmd.Process == nil {
		return errors.New("no game running")
	}
	// Best-effort kill. (Later: upgrade to a Job Object to kill trees.)
	return curCmd.Process.Kill()
}

func LaunchGame(game *GameData) chan error {
	ch := make(chan error, 1) // buffer 1 so the goroutine never blocks on send

	if game == nil || game.Executable == "" {
		ch <- errors.New("invalid game data")
		close(ch)
		return ch
	}

	lockPath := os.TempDir() + "/" + game.ID.String() + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			ch <- errors.New("already running")
		} else {
			ch <- err
		}
		close(ch)
		return ch
	}

	go func() {
		defer func() {
			_ = lockFile.Close()
			_ = os.Remove(lockPath)
			close(ch)
		}()

		cmd := exec.Command(game.Executable)
		cmd.Dir = game.StartsOn
		curMu.Lock()
		curCmd = cmd
		curMu.Unlock()
		// cmd.Args = append([]string{game.Executable}, game.Args...)

		if err := cmd.Start(); err != nil {
			ch <- err
			return
		}
		ch <- cmd.Wait()
		
		curMu.Lock()
		curCmd = nil
		curMu.Unlock()
	}()

	return ch
}
