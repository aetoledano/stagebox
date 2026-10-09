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
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func LaunchSteamSession(steamExe string) RunningGame {
	done := make(chan error, 1)
	pid := make(chan int, 1)
	exited := make(chan struct{})
	result := RunningGame{Done: done, PID: pid, Exited: exited}

	go func() {
		var sessionErr error
		defer func() {
			done <- sessionErr
			close(done)
			close(exited)
		}()
		sessionErr = runSteamSession(steamExe, pid)
	}()

	return result
}

func runSteamSession(steamExe string, pid chan<- int) error {
	published := false
	defer func() {
		if !published {
			close(pid)
		}
	}()
	if steamExe == "" {
		return errors.New("missing Steam executable")
	}
	exe, err := filepath.Abs(steamExe)
	if err != nil {
		return err
	}
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		return fmt.Errorf("identify Steam Windows session: %w", err)
	}
	tracker := steamTracker{exe: filepath.Clean(exe), sessionID: sessionID, processes: make(map[uint32]*steamTrackedProcess)}
	defer tracker.close()
	clients, _, err := tracker.sample()
	if err != nil {
		return fmt.Errorf("inspect existing Steam clients: %w", err)
	}
	if len(clients) > 0 {
		lgx.Infof("Steam session attaching to existing clients path=%q session=%d pids=%v", exe, sessionID, clients)
	}

	// Also send the normal activation request when Steam already owns the singleton.
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return err
	}
	pid <- cmd.Process.Pid
	close(pid)
	published = true
	initialPID := uint32(cmd.Process.Pid)
	lgx.Infof("Steam session start path=%q session=%d requestPID=%d restartGrace=%s", exe, sessionID, initialPID, steamRestartGrace)
	rootExit := make(chan error, 1)
	go func() { rootExit <- cmd.Wait() }()
	// Capture the request process promptly; subsequent samples find independent restarts.
	if process, err := tracker.open(initialPID); err == nil && process != nil && strings.EqualFold(process.path, tracker.exe) {
		process.client = true
		tracker.add(initialPID, process)
	} else if process != nil {
		_ = windows.CloseHandle(process.handle)
	}

	state := steamSessionState{}
	handedOff := len(clients) > 0
	var initialErr error
	var failures int
	previousClients := clients
	previousVisible := false
	for {
		select {
		case initialErr = <-rootExit:
			lgx.Infof("Steam activation process exited pid=%d err=%v; continuing session tracking", initialPID, initialErr)
			rootExit = nil
		default:
		}
		clients, visible, err := tracker.sample()
		if err != nil {
			failures++
			state.absentSince = time.Time{}
			lgx.Warnf("Steam session sample failed attempt=%d err=%v", failures, err)
			if failures >= 3 {
				return fmt.Errorf("track Steam session: %w", err)
			}
			time.Sleep(steamPollInterval)
			continue
		}
		failures = 0
		for _, client := range clients {
			if client != initialPID {
				handedOff = true
			}
		}
		if !slices.Equal(clients, previousClients) || visible != previousVisible {
			lgx.Infof("Steam session changed clients=%v visibleGame=%v", clients, visible)
			previousClients, previousVisible = clients, visible
		}
		wasAbsent := !state.absentSince.IsZero()
		if state.finished(time.Now(), len(clients), visible) {
			lgx.Infof("Steam session ended after restart grace path=%q", exe)
			if !handedOff && initialErr != nil {
				return initialErr
			}
			return nil
		}
		if !wasAbsent && !state.absentSince.IsZero() {
			lgx.Infof("Steam session waiting for restart grace=%s", steamRestartGrace)
		}
		time.Sleep(steamPollInterval)
	}
}

type steamTrackedProcess struct {
	handle   windows.Handle
	path     string
	lifetime steamProcessLifetime
	alive    bool
	client   bool
}

type steamTracker struct {
	exe       string
	sessionID uint32
	processes map[uint32]*steamTrackedProcess
}

func (t *steamTracker) close() {
	for _, process := range t.processes {
		_ = windows.CloseHandle(process.handle)
	}
}

func (t *steamTracker) add(pid uint32, process *steamTrackedProcess) {
	if old := t.processes[pid]; old != nil {
		_ = windows.CloseHandle(old.handle)
	}
	t.processes[pid] = process
}

func (t *steamTracker) open(pid uint32) (*steamTrackedProcess, error) {
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(pid, &sessionID); err != nil {
		return nil, err
	}
	if sessionID != t.sessionID {
		return nil, nil
	}
	handle, err := windows.OpenProcess(processQueryLimitedInformation|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, err
	}
	process := &steamTrackedProcess{handle: handle}
	var path [32768]uint16
	size := uint32(len(path))
	if err := windows.QueryFullProcessImageName(handle, 0, &path[0], &size); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	process.path = filepath.Clean(windows.UTF16ToString(path[:size]))
	if err := process.refresh(); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return process, nil
}

func (p *steamTrackedProcess) refresh() error {
	state, err := windows.WaitForSingleObject(p.handle, 0)
	if err != nil {
		return err
	}
	p.alive = state == uint32(windows.WAIT_TIMEOUT)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(p.handle, &created, &exited, &kernel, &user); err != nil {
		return err
	}
	p.lifetime.created = uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	p.lifetime.exited = 0
	if !p.alive {
		p.lifetime.exited = uint64(exited.HighDateTime)<<32 | uint64(exited.LowDateTime)
	}
	return nil
}

func (t *steamTracker) sample() ([]uint32, bool, error) {
	entries, err := processEntries()
	if err != nil {
		return nil, false, err
	}
	for _, process := range t.processes {
		if err := process.refresh(); err != nil {
			return nil, false, err
		}
	}
	parents := make(map[uint32][]processEntry, len(entries))
	for _, entry := range entries {
		parents[entry.parentPID] = append(parents[entry.parentPID], entry)
		if !strings.EqualFold(entry.exeName, filepath.Base(t.exe)) {
			continue
		}
		if known := t.processes[entry.pid]; known != nil && known.alive && known.client {
			continue
		}
		process, err := t.open(entry.pid)
		if err != nil {
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_FOUND) {
				continue // Exited between the snapshot and query.
			}
			return nil, false, fmt.Errorf("inspect Steam candidate pid=%d: %w", entry.pid, err)
		}
		if process == nil {
			continue
		}
		if !strings.EqualFold(process.path, t.exe) {
			_ = windows.CloseHandle(process.handle)
			continue
		}
		process.client = true
		t.add(entry.pid, process)
	}

	// Retain exited parents' handles/timestamps so a handoff remains identifiable.
	queue := make([]uint32, 0, len(t.processes))
	for pid := range t.processes {
		queue = append(queue, pid)
	}
	seen := make(map[uint32]bool, len(queue))
	for len(queue) > 0 {
		parentPID := queue[0]
		queue = queue[1:]
		if seen[parentPID] {
			continue
		}
		seen[parentPID] = true
		parent := t.processes[parentPID]
		for _, entry := range parents[parentPID] {
			if known := t.processes[entry.pid]; known != nil && known.alive {
				queue = append(queue, entry.pid)
				continue
			}
			process, err := t.open(entry.pid)
			if err != nil {
				lgx.Debugf("Steam descendant query skipped pid=%d err=%v", entry.pid, err)
				continue
			}
			if process == nil {
				continue
			}
			// Parent IDs alone can match unrelated processes after Windows reuses a PID.
			if err := parent.refresh(); err != nil {
				_ = windows.CloseHandle(process.handle)
				return nil, false, err
			}
			if !parent.lifetime.canParent(process.lifetime.created) {
				_ = windows.CloseHandle(process.handle)
				continue
			}
			t.add(entry.pid, process)
			queue = append(queue, entry.pid)
		}
	}

	clients := make([]uint32, 0)
	games := make(map[uint32]struct{})
	for pid, process := range t.processes {
		if !process.alive {
			continue
		}
		if process.client {
			clients = append(clients, pid)
		} else if !steamBackgroundHelper(process.path) {
			games[pid] = struct{}{}
		}
	}
	slices.Sort(clients)
	return clients, processTreeHasVisibleWindow(games), nil
}

func steamBackgroundHelper(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case "steamwebhelper.exe", "steamservice.exe", "steamerrorreporter.exe", "steamerrorreporter64.exe",
		"crashhandler.exe", "crashhandler64.exe", "gameoverlayui.exe", "steam_monitor.exe":
		return true
	}
	return false
}
