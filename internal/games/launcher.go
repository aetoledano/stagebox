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
	curMu      sync.Mutex
	curProcess gameProcess
)

const (
	TrackModeProcess     = "process"
	TrackModeProcessTree = "processTree"
	TrackModeJob         = "job"
	TrackModeSteamGame   = "steamGame"

	lockVersion       = 1
	pendingLockMaxAge = 2 * time.Minute
)

type RunningGame struct {
	Done      <-chan error
	PID       <-chan int
	WindowPID <-chan int
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
	Version         int       `json:"version"`
	GameID          string    `json:"gameId"`
	GameName        string    `json:"gameName"`
	Executable      string    `json:"executable"`
	PID             int       `json:"pid"`
	LauncherID      int       `json:"launcherPid"`
	TrackMode       string    `json:"trackMode"`
	JobName         string    `json:"jobName,omitempty"`
	SteamInstallDir string    `json:"steamInstallDir,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type lockState struct {
	meta  launchLockMetadata
	stat  fs.FileInfo
	valid bool
}

type gameProcess interface {
	PID() int
	JobName() string
	Wait() error
	Kill() error
	Close()
}

type commandGameProcess struct {
	cmd       *exec.Cmd
	game      *GameData
	trackMode string
}

func KillRunning() error {
	curMu.Lock()
	defer curMu.Unlock()
	if curProcess == nil {
		return errors.New("no game running")
	}
	return curProcess.Kill()
}

type LaunchOptions struct{ OnStatus func(SteamLaunchStatus) }

func LaunchGame(game *GameData) RunningGame {
	return LaunchGameWithOptions(game, LaunchOptions{})
}

func LaunchGameWithOptions(game *GameData, opts LaunchOptions) RunningGame {
	done := make(chan error, 1) // buffer 1 so the goroutine never blocks on send
	pid := make(chan int, 1)
	windowPID := make(chan int, 1)
	exited := make(chan struct{})
	trackMode := normalizeTrackMode("")
	result := RunningGame{
		Done:      done,
		PID:       pid,
		WindowPID: windowPID,
		Exited:    exited,
		TrackMode: trackMode,
	}

	if game == nil || game.Executable == "" {
		done <- errors.New("invalid game data")
		close(done)
		close(pid)
		close(windowPID)
		close(exited)
		return result
	}

	trackMode = gameTrackingMode(game)
	result.TrackMode = trackMode
	result.LockPath = gameLockPath(game)
	result.StartedAt = time.Now()

	lock, err := acquireLaunchLock(game, trackMode)
	if err != nil {
		done <- err
		close(done)
		close(pid)
		close(windowPID)
		close(exited)
		return result
	}

	go func() {
		var waitErr error
		defer func() {
			lock.release()
			done <- waitErr
			close(done)
			close(exited)
		}()

		lgx.Infof("game process start requested name=%q exe=%q dir=%q args=%q trackMode=%s lock=%q", game.Name, game.Executable, game.StartsOn, game.Args, trackMode, lock.path)
		process, err := startGameProcess(game, trackMode, opts)
		if err != nil {
			close(pid)
			close(windowPID)
			lgx.Errorf("game process start failed name=%q exe=%q dir=%q err=%v", game.Name, game.Executable, game.StartsOn, err)
			waitErr = err
			return
		}
		curMu.Lock()
		curProcess = process
		curMu.Unlock()
		defer func() {
			curMu.Lock()
			if curProcess == process {
				curProcess = nil
			}
			process.Close()
			curMu.Unlock()
		}()
		lock.update(game, trackMode, process.PID(), process.JobName())
		if source, ok := process.(interface{ SetStatusCallback(func(SteamLaunchStatus)) }); ok {
			source.SetStatusCallback(opts.OnStatus)
		}
		lgx.Infof("game process started name=%q pid=%d trackMode=%s", game.Name, process.PID(), trackMode)
		if source, ok := process.(interface{ StartedPID() <-chan int }); ok {
			go func() {
				defer close(pid)
				defer close(windowPID)
				if actualPID, ok := <-source.StartedPID(); ok {
					pid <- actualPID
					windowPID <- actualPID
				}
			}()
		} else {
			pid <- process.PID()
			close(pid)
			go publishLocalWindowPID(process, windowPID, exited)
		}

		waitErr = process.Wait()
		if waitErr != nil {
			lgx.Warnf("game process exited with error name=%q pid=%d err=%v", game.Name, process.PID(), waitErr)
		} else {
			lgx.Infof("game process exited name=%q pid=%d", game.Name, process.PID())
		}
	}()

	return result
}

// Window readiness is separate from process startup: callers using PID for
// process bookkeeping retain its original timing, while the UI can await a
// real window. Serialize sampling with Kill/Close to protect native job handles.
func publishLocalWindowPID(process gameProcess, ready chan<- int, exited <-chan struct{}) {
	defer close(ready)
	known := map[uint32]struct{}{uint32(process.PID()): {}}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		curMu.Lock()
		if curProcess != process {
			curMu.Unlock()
			return
		}
		pid := localGameWindowPID(process, known)
		curMu.Unlock()
		if pid > 0 {
			ready <- pid
			return
		}
		select {
		case <-exited:
			return
		case <-ticker.C:
		}
	}
}

func startGameProcess(game *GameData, trackMode string, opts LaunchOptions) (gameProcess, error) {
	if game.SteamAppID != 0 {
		return startSteamGameProcess(game, opts.OnStatus)
	}
	if game.AdvancedTracking {
		return startAdvancedGameProcess(game)
	}
	cmd := exec.Command(game.Executable, game.Args...)
	cmd.Dir = game.StartsOn
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &commandGameProcess{cmd: cmd, game: game, trackMode: trackMode}, nil
}

func (p *commandGameProcess) PID() int        { return p.cmd.Process.Pid }
func (p *commandGameProcess) JobName() string { return "" }
func (p *commandGameProcess) Close()          {}
func (p *commandGameProcess) Kill() error     { return killProcessTree(p.cmd.Process) }

func (p *commandGameProcess) Wait() error {
	err := p.cmd.Wait()
	if p.trackMode == TrackModeProcessTree {
		lgx.Debugf("waiting for game process tree name=%q rootPID=%d extraWait=%s", p.game.Name, p.PID(), waitAfterExitDuration(p.game))
		waitForProcessTreeExit(p.PID(), waitAfterExitDuration(p.game))
	} else if wait := waitAfterExitDuration(p.game); wait > 0 {
		lgx.Debugf("waiting after game process exit name=%q wait=%s", p.game.Name, wait)
		time.Sleep(wait)
	}
	return err
}

func gameTrackingMode(game *GameData) string {
	if game.SteamAppID != 0 {
		return TrackModeSteamGame
	}
	if game.AdvancedTracking {
		return TrackModeJob
	}
	return normalizeTrackMode(game.TrackMode)
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
			lock.update(game, trackMode, 0, "")
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
	if state.valid && state.meta.JobName != "" {
		if advancedJobActive(state.meta.JobName) {
			return true
		}
		if state.meta.TrackMode == TrackModeSteamGame {
			if steamLaunchOwnerActive(state.meta.LauncherID, state.meta.CreatedAt) {
				return true // A long Steam update can leave the game job empty.
			}
			if steamInstallActive(state.meta.SteamInstallDir) {
				return true
			}
			return time.Since(state.meta.CreatedAt) < pendingLockMaxAge
		}
		return false
	}
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

func (l *launchLock) update(game *GameData, trackMode string, pid int, jobName string) {
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
		JobName:    jobName,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if game.SteamAppID != 0 {
		meta.SteamInstallDir = game.StartsOn
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
