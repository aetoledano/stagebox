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
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procOpenJobObject = windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenJobObjectW")

type jobGameProcess struct {
	job       windows.Handle
	root      windows.Handle
	pid       uint32
	name      string
	jobName   string
	extraWait time.Duration
	killed    atomic.Bool
}

func startAdvancedGameProcess(game *GameData) (gameProcess, error) {
	exe, err := exec.LookPath(game.Executable)
	if err != nil {
		return nil, err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return nil, err
	}
	appName, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return nil, err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{exe}, game.Args...)))
	if err != nil {
		return nil, err
	}
	var dir *uint16
	if game.StartsOn != "" {
		workingDir, err := filepath.Abs(game.StartsOn)
		if err != nil {
			return nil, err
		}
		dir, err = windows.UTF16PtrFromString(workingDir)
		if err != nil {
			return nil, err
		}
	}

	jobName := fmt.Sprintf("Local\\StageBox-%s-%d-%d", game.ID, os.Getpid(), time.Now().UnixNano())
	jobNamePtr, err := windows.UTF16PtrFromString(jobName)
	if err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, jobNamePtr)
	if err != nil {
		return nil, fmt.Errorf("create tracking job: %w", err)
	}
	ready := false
	defer func() {
		if !ready {
			_ = windows.CloseHandle(job)
		}
	}()

	var startup windows.StartupInfo
	startup.Cb = uint32(unsafe.Sizeof(startup))
	var process windows.ProcessInformation
	if err := windows.CreateProcess(appName, commandLine, nil, nil, false, windows.CREATE_SUSPENDED, nil, dir, &startup, &process); err != nil {
		return nil, fmt.Errorf("create suspended game process: %w", err)
	}
	defer windows.CloseHandle(process.Thread)
	defer func() {
		if !ready {
			if err := windows.TerminateProcess(process.Process, 1); err != nil {
				lgx.Errorf("terminate suspended game after tracking setup failure pid=%d err=%v", process.ProcessId, err)
			} else {
				_, _ = windows.WaitForSingleObject(process.Process, 5000)
			}
			_ = windows.CloseHandle(process.Process)
		}
	}()
	// No kill-on-close or resource/UI limits: closing StageBox must not kill the game.
	if err := windows.AssignProcessToJobObject(job, process.Process); err != nil {
		return nil, fmt.Errorf("assign game to tracking job (disable advancedTracking for incompatible games): %w", err)
	}
	if _, err := windows.ResumeThread(process.Thread); err != nil {
		return nil, fmt.Errorf("resume tracked game process: %w", err)
	}
	ready = true
	lgx.Infof("advanced tracking started name=%q rootPID=%d job=%q", game.Name, process.ProcessId, jobName)
	return &jobGameProcess{
		job: job, root: process.Process, pid: process.ProcessId, name: game.Name,
		jobName: jobName, extraWait: waitAfterExitDuration(game),
	}, nil
}

func (p *jobGameProcess) PID() int        { return int(p.pid) }
func (p *jobGameProcess) JobName() string { return p.jobName }

func (p *jobGameProcess) Kill() error {
	if err := windows.TerminateJobObject(p.job, 1); err != nil {
		return fmt.Errorf("terminate game job: %w", err)
	}
	p.killed.Store(true)
	lgx.Infof("advanced tracking job kill requested name=%q rootPID=%d", p.name, p.pid)
	return nil
}

func (p *jobGameProcess) Close() {
	_ = windows.CloseHandle(p.root)
	_ = windows.CloseHandle(p.job)
}

func (p *jobGameProcess) Wait() error {
	state := jobSessionState{started: time.Now()}
	var queryFailures int
	var previousPIDs []uint32
	for {
		pids, err := jobProcessIDs(p.job)
		if err != nil {
			queryFailures++
			lgx.Warnf("advanced tracking job query failed name=%q attempt=%d err=%v", p.name, queryFailures, err)
			if queryFailures >= 3 {
				return fmt.Errorf("query game job: %w", err)
			}
			time.Sleep(jobPollInterval)
			continue
		}
		queryFailures = 0
		if !slices.Equal(previousPIDs, pids) {
			lgx.Debugf("advanced tracking members name=%q rootPID=%d pids=%v", p.name, p.pid, pids)
			previousPIDs = pids
		}
		rootState, err := windows.WaitForSingleObject(p.root, 0)
		if err != nil {
			return fmt.Errorf("query game root state: %w", err)
		}
		visible, childVisible := jobVisibleWindows(pids, p.pid)
		if p.killed.Load() {
			if len(pids) == 0 {
				break
			}
		} else if state.finished(time.Now(), len(pids), rootState == windows.WAIT_OBJECT_0, visible, childVisible) {
			if len(pids) > 0 {
				lgx.Warnf("advanced tracking ended without visible game windows name=%q remainingPIDs=%v sawWindow=%v sawChildWindow=%v", p.name, pids, state.sawWindow, state.sawChildWindow)
			}
			break
		}
		time.Sleep(jobPollInterval)
	}
	if p.extraWait > 0 && !p.killed.Load() {
		time.Sleep(p.extraWait)
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(p.root, &exitCode); err != nil {
		return fmt.Errorf("read game launcher exit code: %w", err)
	}
	if exitCode != 0 && exitCode != stillActive {
		return fmt.Errorf("game launcher exited with status %d", exitCode)
	}
	return nil
}

type jobProcessListHeader struct {
	assigned uint32
	count    uint32
}

func jobProcessIDs(job windows.Handle) ([]uint32, error) {
	capacity := 32
	for attempt := 0; attempt < 10; attempt++ {
		// The Win32 header contains two DWORDs followed by a ULONG_PTR array.
		buffer := make([]uintptr, 8/int(unsafe.Sizeof(uintptr(0)))+capacity)
		header := (*jobProcessListHeader)(unsafe.Pointer(&buffer[0]))
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(header)), uint32(len(buffer))*uint32(unsafe.Sizeof(uintptr(0))), nil)
		if err != nil && !errors.Is(err, windows.ERROR_MORE_DATA) && !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil, err
		}
		if err != nil || header.assigned > header.count || header.count > uint32(capacity) {
			capacity = max(capacity*2, int(header.assigned))
			continue
		}
		ids := unsafe.Slice((*uintptr)(unsafe.Add(unsafe.Pointer(header), 8)), int(header.count))
		pids := make([]uint32, len(ids))
		for i, pid := range ids {
			pids[i] = uint32(pid)
		}
		slices.Sort(pids)
		return pids, nil
	}
	return nil, errors.New("game job membership kept changing while querying")
}

func jobVisibleWindows(pids []uint32, rootPID uint32) (visible, childVisible bool) {
	all := make(map[uint32]struct{}, len(pids))
	children := make(map[uint32]struct{}, len(pids))
	for _, pid := range pids {
		all[pid] = struct{}{}
		if pid != rootPID {
			children[pid] = struct{}{}
		}
	}
	return processTreeHasVisibleWindow(all), processTreeHasVisibleWindow(children)
}

func advancedJobActive(name string) bool {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	const jobObjectQuery = 0x0004
	handle, _, callErr := procOpenJobObject.Call(jobObjectQuery, 0, uintptr(unsafe.Pointer(namePtr)))
	if handle == 0 {
		if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
			return false
		}
		lgx.Warnf("launch lock job check failed job=%q err=%v", name, callErr)
		return true
	}
	job := windows.Handle(handle)
	defer windows.CloseHandle(job)
	pids, err := jobProcessIDs(job)
	if err != nil {
		lgx.Warnf("launch lock job query failed job=%q err=%v", name, err)
		return true
	}
	return len(pids) > 0
}
