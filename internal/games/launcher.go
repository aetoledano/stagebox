package games

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	lgx "launcher/internal/logx"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	curMu  sync.Mutex
	curCmd *exec.Cmd
)

const (
	TrackModeProcess     = "process"
	TrackModeProcessTree = "processTree"

	lockVersion       = 1
	pendingLockMaxAge = 2 * time.Minute
)

type RunningGame struct {
	Done      <-chan error
	PID       <-chan int
	Exited    <-chan struct{}
	LockPath  string
	TrackMode string
	StartedAt time.Time
}

type launchLock struct {
	path string
	file *os.File
}

type launchLockMetadata struct {
	Version    int       `json:"version"`
	GameID     string    `json:"gameId"`
	GameName   string    `json:"gameName"`
	Executable string    `json:"executable"`
	PID        int       `json:"pid"`
	LauncherID int       `json:"launcherPid"`
	TrackMode  string    `json:"trackMode"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type lockState struct {
	meta  launchLockMetadata
	stat  fs.FileInfo
	valid bool
}

func KillRunning() error {
	curMu.Lock()
	defer curMu.Unlock()
	if curCmd == nil || curCmd.Process == nil {
		return errors.New("no game running")
	}
	return killProcessTree(curCmd.Process)
}

func LaunchGame(game *GameData) RunningGame {
	done := make(chan error, 1) // buffer 1 so the goroutine never blocks on send
	pid := make(chan int, 1)
	exited := make(chan struct{})
	trackMode := normalizeTrackMode("")
	result := RunningGame{
		Done:      done,
		PID:       pid,
		Exited:    exited,
		TrackMode: trackMode,
	}

	if game == nil || game.Executable == "" {
		done <- errors.New("invalid game data")
		close(done)
		close(pid)
		close(exited)
		return result
	}

	trackMode = normalizeTrackMode(game.TrackMode)
	result.TrackMode = trackMode
	result.LockPath = gameLockPath(game)
	result.StartedAt = time.Now()

	lock, err := acquireLaunchLock(game, trackMode)
	if err != nil {
		done <- err
		close(done)
		close(pid)
		close(exited)
		return result
	}

	go func() {
		defer func() {
			lock.release()
			close(done)
			close(exited)
		}()

		lgx.Infof("game process start requested name=%q exe=%q dir=%q args=%q trackMode=%s lock=%q", game.Name, game.Executable, game.StartsOn, game.Args, trackMode, lock.path)
		cmd := exec.Command(game.Executable, game.Args...)
		cmd.Dir = game.StartsOn
		curMu.Lock()
		curCmd = cmd
		curMu.Unlock()

		if err := cmd.Start(); err != nil {
			curMu.Lock()
			curCmd = nil
			curMu.Unlock()
			close(pid)
			lgx.Errorf("game process start failed name=%q exe=%q dir=%q err=%v", game.Name, game.Executable, game.StartsOn, err)
			done <- err
			return
		}
		lock.update(game, trackMode, cmd.Process.Pid)
		lgx.Infof("game process started name=%q pid=%d trackMode=%s", game.Name, cmd.Process.Pid, trackMode)
		pid <- cmd.Process.Pid
		close(pid)

		waitErr := cmd.Wait()
		if trackMode == TrackModeProcessTree {
			lgx.Debugf("waiting for game process tree name=%q rootPID=%d extraWait=%s", game.Name, cmd.Process.Pid, waitAfterExitDuration(game))
			waitForProcessTreeExit(cmd.Process.Pid, waitAfterExitDuration(game))
		} else if wait := waitAfterExitDuration(game); wait > 0 {
			lgx.Debugf("waiting after game process exit name=%q wait=%s", game.Name, wait)
			time.Sleep(wait)
		}
		if waitErr != nil {
			lgx.Warnf("game process exited with error name=%q pid=%d err=%v", game.Name, cmd.Process.Pid, waitErr)
		} else {
			lgx.Infof("game process exited name=%q pid=%d", game.Name, cmd.Process.Pid)
		}
		done <- waitErr

		curMu.Lock()
		curCmd = nil
		curMu.Unlock()
	}()

	return result
}

func normalizeTrackMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case strings.ToLower(TrackModeProcessTree):
		return TrackModeProcessTree
	default:
		return TrackModeProcess
	}
}

func waitAfterExitDuration(game *GameData) time.Duration {
	if game == nil || game.WaitAfterExitSeconds <= 0 {
		return 0
	}
	return time.Duration(game.WaitAfterExitSeconds * float64(time.Second))
}

func gameLockPath(game *GameData) string {
	return filepath.Join(os.TempDir(), game.ID.String()+".lock")
}

func acquireLaunchLock(game *GameData, trackMode string) (*launchLock, error) {
	path := gameLockPath(game)
	for {
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			lock := &launchLock{path: path, file: file}
			lock.update(game, trackMode, 0)
			return lock, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}

		state := readLaunchLock(path)
		if launchLockActive(state) {
			pidInfo := ""
			if state.meta.PID > 0 {
				pidInfo = fmt.Sprintf(" pid=%d", state.meta.PID)
			}
			return nil, fmt.Errorf("already running%s lock=%s", pidInfo, path)
		}

		lgx.Warnf("removing stale game launch lock path=%q game=%q pid=%d valid=%v", path, game.Name, state.meta.PID, state.valid)
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("remove stale launch lock %s: %w", path, err)
		}
	}
}

func readLaunchLock(path string) lockState {
	state := lockState{}
	stat, statErr := os.Stat(path)
	if statErr == nil {
		state.stat = stat
	}

	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return state
	}
	if err := json.Unmarshal(data, &state.meta); err != nil {
		return state
	}
	state.valid = true
	return state
}

func launchLockActive(state lockState) bool {
	if state.valid && state.meta.PID > 0 {
		return processExists(state.meta.PID)
	}

	t := state.meta.UpdatedAt
	if t.IsZero() {
		t = state.meta.CreatedAt
	}
	if t.IsZero() && state.stat != nil {
		t = state.stat.ModTime()
	}
	return !t.IsZero() && time.Since(t) < pendingLockMaxAge
}

func (l *launchLock) update(game *GameData, trackMode string, pid int) {
	if l == nil || l.file == nil {
		return
	}
	now := time.Now()
	meta := launchLockMetadata{
		Version:    lockVersion,
		GameID:     game.ID.String(),
		GameName:   game.Name,
		Executable: game.Executable,
		PID:        pid,
		LauncherID: os.Getpid(),
		TrackMode:  trackMode,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if existing := readLaunchLock(l.path); existing.valid && !existing.meta.CreatedAt.IsZero() {
		meta.CreatedAt = existing.meta.CreatedAt
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		lgx.Warnf("marshal launch lock failed path=%q err=%v", l.path, err)
		return
	}
	if err := l.file.Truncate(0); err != nil {
		lgx.Warnf("truncate launch lock failed path=%q err=%v", l.path, err)
		return
	}
	if _, err := l.file.Seek(0, 0); err != nil {
		lgx.Warnf("seek launch lock failed path=%q err=%v", l.path, err)
		return
	}
	if _, err := l.file.Write(data); err != nil {
		lgx.Warnf("write launch lock failed path=%q err=%v", l.path, err)
		return
	}
}

func (l *launchLock) release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	if l.path != "" {
		_ = os.Remove(l.path)
	}
}
