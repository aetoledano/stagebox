//go:build windows

package games

import (
	"errors"
	"fmt"
	lgx "launcher/internal/logx"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procIsProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

// Steam's singleton can hand the request to an existing client. Attach only
// this game's processes to a fresh job, never the Steam client/request process.
type steamGameProcess struct {
	mu           sync.Mutex
	game         *GameData
	tracker      steamTracker
	clientUI     *steamTracker
	uiSettlesAt  time.Time
	uiWarning    bool
	job          windows.Handle
	jobName      string
	request      *exec.Cmd
	requestExit  <-chan error
	startedPID   chan int
	pidPublished bool
	killed       atomic.Bool
	onStatus     func(SteamLaunchStatus)
}

func startSteamGameProcess(game *GameData, onStatus func(SteamLaunchStatus)) (gameProcess, error) {
	stopSteamUIBackgrounding()
	if err := validateSteamGame(game); err != nil {
		return nil, err
	}
	// Cold-starting Steam and requesting a game in one invocation can race
	// Steam API initialization in older games. Start/register the client first.
	if err := ensureSteamClientReady(game, onStatus); err != nil {
		return nil, err
	}
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		return nil, err
	}
	jobName := fmt.Sprintf("Local\\StageBox-Steam-%d-%d-%d", game.SteamAppID, os.Getpid(), time.Now().UnixNano())
	name, err := windows.UTF16PtrFromString(jobName)
	if err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, name)
	if err != nil {
		return nil, fmt.Errorf("create Steam game tracking job: %w", err)
	}
	p := &steamGameProcess{
		game: game, job: job, jobName: jobName, startedPID: make(chan int, 1),
		tracker:  steamTracker{exe: filepath.Clean(game.Executable), sessionID: sessionID, processes: map[uint32]*steamTrackedProcess{}},
		clientUI: &steamTracker{exe: filepath.Clean(game.Executable), sessionID: sessionID, processes: map[uint32]*steamTrackedProcess{}},
	}
	p.cleanStartupUI()
	// Attach any already-running copy before sending the game's activation URL.
	if _, _, err := p.sampleLocked(); err != nil {
		p.Close()
		return nil, err
	}
	cmd := exec.Command(game.Executable, steamGameLaunchArgs(game.SteamAppID)...)
	cmd.Dir = filepath.Dir(game.Executable)
	if err := cmd.Start(); err != nil {
		p.Close()
		return nil, err
	}
	p.request = cmd
	exit := make(chan error, 1)
	p.requestExit = exit
	go func() { exit <- cmd.Wait() }()
	lgx.Infof("Steam game requested appID=%d name=%q install=%q requestPID=%d job=%q", game.SteamAppID, game.Name, game.StartsOn, cmd.Process.Pid, jobName)
	p.logClientSnapshot("game-requested")
	return p, nil
}

func (p *steamGameProcess) PID() int                                           { return p.request.Process.Pid }
func (p *steamGameProcess) JobName() string                                    { return p.jobName }
func (p *steamGameProcess) StartedPID() <-chan int                             { return p.startedPID }
func (p *steamGameProcess) SetStatusCallback(callback func(SteamLaunchStatus)) { p.onStatus = callback }

func (p *steamGameProcess) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tracker.close()
	if p.clientUI != nil {
		p.clientUI.close()
		p.clientUI = nil
	}
	_ = windows.CloseHandle(p.job)
	if !p.pidPublished {
		close(p.startedPID)
		p.pidPublished = true
	}
}

func (p *steamGameProcess) Wait() (result error) {
	defer func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		lgx.Infof("Steam game tracking completed appID=%d killed=%v err=%v", p.game.SteamAppID, p.killed.Load(), result)
		p.logClientSnapshot("game-ended")
	}()
	state := steamGameSessionState{started: time.Now()}
	var failures int
	var previous []uint32
	var previousVisible bool
	var requestErr error
	requestExit := p.requestExit
	var lastUpdate steamGameState
	var lastStatus SteamLaunchStatus
	for {
		select {
		case requestErr = <-requestExit:
			lgx.Debugf("Steam game activation exited appID=%d err=%v; waiting for game processes", p.game.SteamAppID, requestErr)
			requestExit = nil
		default:
		}
		p.mu.Lock()
		p.cleanStartupUI()
		pids, visible, err := p.sampleLocked()
		gamePIDs := p.activityPIDs(pids)
		if err == nil && !p.pidPublished {
			// Publish the game's PID for focusing, not the Steam activation PID.
			for _, pid := range gamePIDs {
				if processTreeHasVisibleWindow(map[uint32]struct{}{pid: {}}) {
					// Steam may minimize its desktop UI only once the game
					// takes focus. Cover that late transition asynchronously.
					p.uiSettlesAt = time.Now().Add(2 * time.Second)
					p.startedPID <- int(pid)
					close(p.startedPID)
					p.pidPublished = true
					lgx.Infof("Steam game first window appID=%d pid=%d", p.game.SteamAppID, pid)
					p.logClientSnapshot("game-window")
					break
				}
			}
		}
		p.mu.Unlock()
		if err != nil {
			failures++
			state.quietSince = time.Time{}
			state.emptySince = time.Time{}
			lgx.Warnf("Steam game sample failed appID=%d attempt=%d err=%v", p.game.SteamAppID, failures, err)
			if failures >= 3 {
				return fmt.Errorf("track Steam game: %w", err)
			}
			time.Sleep(steamPollInterval)
			continue
		}
		failures = 0
		if !state.sawWindow && !visible {
			if update, err := readSteamGameState(p.game); err == nil {
				// Keep waiting while download/install work makes progress. A
				// stalled/paused update still gets the normal startup timeout.
				if update.flags&steamUpdateActivityFlags != 0 && update.flags&512 == 0 && update != lastUpdate {
					state.started = time.Now()
				}
				lastUpdate = update
				status := update.launchStatus()
				if status != lastStatus && p.onStatus != nil {
					p.onStatus(status)
					lastStatus = status
				}
			}
		}
		if !slices.Equal(previous, pids) || previousVisible != visible {
			lgx.Infof("Steam game tracking appID=%d pids=%v gamePIDs=%v visible=%v", p.game.SteamAppID, pids, gamePIDs, visible)
			previous = pids
			previousVisible = visible
		}
		if p.killed.Load() && len(pids) == 0 {
			return nil
		}
		if finished, neverStarted := state.finished(time.Now(), visible, len(gamePIDs)); finished {
			if neverStarted {
				return fmt.Errorf("Steam game did not open within %s (activation error: %v); check Steam sign-in or launch dialogs", steamGameStartGrace, requestErr)
			}
			if wait := waitAfterExitDuration(p.game); wait > 0 {
				time.Sleep(wait)
			}
			return nil
		}
		time.Sleep(steamPollInterval)
	}
}

// Called during setup or under p.mu. Diagnostics own their handles and never
// activate Steam or change window visibility.
func (p *steamGameProcess) logClientSnapshot(stage string) {
	tracker := steamTracker{exe: p.tracker.exe, sessionID: p.tracker.sessionID, processes: map[uint32]*steamTrackedProcess{}}
	defer tracker.close()
	if _, _, err := tracker.sample(); err != nil {
		lgx.Warnf("Steam readiness snapshot appID=%d stage=%s sampleErr=%v", p.game.SteamAppID, stage, err)
	}
	registration, reason, _ := registeredSteamClient(&tracker)
	logSteamClientSnapshot(&tracker, p.game.SteamAppID, stage, 0, registration, reason)
}

// Called during setup or under p.mu. The cleanup lives through bootstrap,
// download and game startup, plus a brief settling period after the first
// game window. It does not participate in game correlation or termination.
func (p *steamGameProcess) cleanStartupUI() {
	if p.clientUI == nil {
		return
	}
	if !p.uiSettlesAt.IsZero() && !time.Now().Before(p.uiSettlesAt) {
		p.clientUI.close()
		p.clientUI = nil
		return
	}
	if err := hideMinimizedSteamClientUI(p.clientUI); err != nil && !p.uiWarning {
		lgx.Warnf("Steam minimized-window cleanup while awaiting game appID=%d: %v", p.game.SteamAppID, err)
		p.uiWarning = true
	}
}

func (p *steamGameProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	pids, _, err := p.sampleLocked()
	if err != nil {
		return fmt.Errorf("inspect Steam game before stopping: %w", err)
	}
	if len(pids) == 0 {
		return errors.New("Steam game is still starting; no game process is available to stop yet")
	}
	var failures []error
	jobPIDs, err := jobProcessIDs(p.job)
	if err != nil {
		return err
	}
	if err := windows.TerminateJobObject(p.job, 1); err != nil {
		failures = append(failures, err)
		jobPIDs = nil // Try verified process handles if job termination failed.
	}
	// Some games reject job assignment. Stop only verified game handles, never
	// taskkill /T on Steam, which would also terminate the client/other games.
	for _, pid := range pids {
		if slices.Contains(jobPIDs, pid) {
			continue
		}
		process := p.tracker.processes[pid]
		if err := process.refresh(); err != nil {
			failures = append(failures, err)
			continue
		}
		if !process.alive {
			continue
		}
		handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|processQueryLimitedInformation, false, pid)
		if err != nil {
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
				continue
			}
			failures = append(failures, fmt.Errorf("stop Steam game pid=%d: %w", pid, err))
			continue
		}
		var created, exited, kernel, user windows.Filetime
		err = windows.GetProcessTimes(handle, &created, &exited, &kernel, &user)
		if err == nil && uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) == process.lifetime.created {
			err = windows.TerminateProcess(handle, 1)
		}
		_ = windows.CloseHandle(handle)
		if err != nil {
			failures = append(failures, err)
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	p.killed.Store(true)
	lgx.Infof("Steam game kill requested appID=%d pids=%v", p.game.SteamAppID, pids)
	return nil
}

func (p *steamGameProcess) sampleLocked() ([]uint32, bool, error) {
	entries, err := processEntries()
	if err != nil {
		return nil, false, err
	}
	for pid, process := range p.tracker.processes {
		wasAlive := process.alive
		if err := process.refresh(); err != nil {
			return nil, false, err
		}
		if wasAlive && !process.alive {
			var exitCode uint32
			err := windows.GetExitCodeProcess(process.handle, &exitCode)
			lgx.Infof("Steam game process exited appID=%d pid=%d exe=%q exitCode=%d queryErr=%v", p.game.SteamAppID, pid, process.path, exitCode, err)
		}
	}
	jobPIDs, err := jobProcessIDs(p.job)
	if err != nil {
		return nil, false, err
	}
	jobMembers := map[uint32]bool{}
	for _, pid := range jobPIDs {
		jobMembers[pid] = true
	}
	parents := map[uint32][]processEntry{}
	for _, entry := range entries {
		parents[entry.parentPID] = append(parents[entry.parentPID], entry)
		if known := p.tracker.processes[entry.pid]; known != nil && known.alive {
			continue
		}
		process, err := p.openCandidate(entry.pid)
		if err != nil || process == nil {
			continue
		}
		member := jobMembers[entry.pid]
		if member {
			var inJob int32
			ok, _, err := procIsProcessInJob.Call(uintptr(process.handle), uintptr(p.job), uintptr(unsafe.Pointer(&inJob)))
			if ok == 0 {
				_ = windows.CloseHandle(process.handle)
				return nil, false, fmt.Errorf("verify Steam job member: %w", err)
			}
			member = inJob != 0
		}
		if !p.gameProcessPath(process.path) || (!member && !steamPathWithin(process.path, p.game.StartsOn)) {
			_ = windows.CloseHandle(process.handle)
			continue
		}
		p.adopt(entry.pid, process, member)
	}
	// Retain handles/timestamps of exited intermediates for descendant handoffs.
	queue := make([]uint32, 0, len(p.tracker.processes))
	for pid := range p.tracker.processes {
		queue = append(queue, pid)
	}
	seen := map[uint32]bool{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		parent := p.tracker.processes[pid]
		for _, entry := range parents[pid] {
			if known := p.tracker.processes[entry.pid]; known != nil && known.alive {
				queue = append(queue, entry.pid)
				continue
			}
			process, err := p.openCandidate(entry.pid)
			if err != nil || process == nil {
				continue
			}
			if err := parent.refresh(); err != nil {
				_ = windows.CloseHandle(process.handle)
				return nil, false, err
			}
			if !p.gameProcessPath(process.path) || !parent.lifetime.canParent(process.lifetime.created) {
				_ = windows.CloseHandle(process.handle)
				continue
			}
			p.adopt(entry.pid, process, jobMembers[entry.pid])
			queue = append(queue, entry.pid)
		}
	}
	pids := []uint32{}
	visiblePIDs := map[uint32]struct{}{}
	for pid, process := range p.tracker.processes {
		if !process.alive {
			continue
		}
		pids = append(pids, pid)
		if !steamGameCrashHelper(process.path) {
			visiblePIDs[pid] = struct{}{}
		}
	}
	slices.Sort(pids)
	return pids, processTreeHasVisibleWindow(visiblePIDs), nil
}

// Called under p.mu. Keep reporting helpers in the kill/job set while giving
// startup/window/exit timing only the actual game/launcher processes.
func (p *steamGameProcess) activityPIDs(pids []uint32) []uint32 {
	active := make([]uint32, 0, len(pids))
	for _, pid := range pids {
		if process := p.tracker.processes[pid]; process != nil && !steamGameCrashHelper(process.path) {
			active = append(active, pid)
		}
	}
	return active
}

func (p *steamGameProcess) gameProcessPath(path string) bool {
	return !steamBackgroundHelper(path) && !p.tracker.clientExecutable(path)
}

func (p *steamGameProcess) openCandidate(pid uint32) (*steamTrackedProcess, error) {
	if pid == uint32(os.Getpid()) || pid == 0 {
		return nil, nil
	}
	return p.tracker.open(pid)
}

func (p *steamGameProcess) adopt(pid uint32, process *steamTrackedProcess, alreadyInJob bool) {
	if !alreadyInJob {
		// Request assignment rights only after this process was correlated to
		// the game. Verify its creation time again before replacing the handle.
		access := uint32(processQueryLimitedInformation | windows.SYNCHRONIZE | windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE)
		full, err := p.tracker.openWithAccess(pid, access)
		if err == nil && full != nil {
			if full.lifetime.created == process.lifetime.created {
				_ = windows.CloseHandle(process.handle)
				process = full
			} else {
				_ = windows.CloseHandle(full.handle)
			}
		}
		if err := windows.AssignProcessToJobObject(p.job, process.handle); err != nil {
			lgx.Warnf("Steam game job assignment unavailable appID=%d pid=%d exe=%q err=%v; retaining handle tracking", p.game.SteamAppID, pid, process.path, err)
		}
	}
	p.tracker.add(pid, process)
	lgx.Debugf("Steam game process attached appID=%d pid=%d exe=%q", p.game.SteamAppID, pid, process.path)
	if steamGameCrashHelper(process.path) {
		lgx.Infof("Steam game reporting helper attached appID=%d pid=%d exe=%q; excluded from game activity", p.game.SteamAppID, pid, process.path)
	}
}

func steamInstallActive(directory string) bool {
	if directory == "" {
		return false
	}
	entries, err := processEntries()
	if err != nil {
		return true
	} // A failed inspection cannot prove a stale lock.
	var sessionID uint32
	if windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID) != nil {
		return true
	}
	tracker := steamTracker{sessionID: sessionID}
	for _, entry := range entries {
		process, err := tracker.open(entry.pid)
		if err != nil || process == nil {
			continue
		}
		active := process.alive && steamPathWithin(process.path, directory) && !steamBackgroundHelper(process.path) && !steamGameCrashHelper(process.path)
		_ = windows.CloseHandle(process.handle)
		if active {
			return true
		}
	}
	return false
}

func steamLaunchOwnerActive(pid int, lockCreated time.Time) bool {
	if pid <= 0 || lockCreated.IsZero() {
		return false
	}
	handle, err := windows.OpenProcess(processQueryLimitedInformation|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return false
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(handle, &created, &exited, &kernel, &user) != nil {
		return false
	}
	// A reused PID created after the lock cannot own that session.
	return created.Nanoseconds() <= lockCreated.UnixNano()
}
