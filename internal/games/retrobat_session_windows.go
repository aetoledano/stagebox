//go:build windows

package games

import (
	"context"
	"errors"
	"fmt"
	lgx "launcher/internal/logx"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

// Keep this whole frontend session alive through hidden/minimized ES and emulator
// handoffs. Do not use the local-game window-free timeout or register curProcess:
// RetroBat owns its emulator exit hotkeys, including Start+Back.
func LaunchRetroBatSession(ctx context.Context, exe string) (RunningGame, error) {
	if err := ctx.Err(); err != nil {
		return RunningGame{}, err
	}
	if err := rejectExistingRetroBat(exe); err != nil {
		return RunningGame{}, err
	}
	game := &GameData{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("launcher/retrobat/"+strings.ToLower(exe))),
		Name: "RetroBat", Executable: exe, StartsOn: filepath.Dir(exe)}
	process, err := startAdvancedGameProcess(game)
	if err != nil {
		return RunningGame{}, err
	}
	job := process.(*jobGameProcess)
	done := make(chan error, 1)
	pid := make(chan int, 1)
	exited := make(chan struct{})
	pid <- process.PID()
	close(pid)
	lgx.Infof("RetroBat session started exe=%q pid=%d job=%q", exe, process.PID(), process.JobName())
	go func() {
		err := waitRetroBatSession(ctx, job)
		process.Close() // No kill-on-close: shutting down StageBox leaves RetroBat alive.
		done <- err
		close(done)
		close(exited)
	}()
	return RunningGame{Done: done, PID: pid, Exited: exited, StartedAt: time.Now()}, nil
}

func waitRetroBatSession(ctx context.Context, process *jobGameProcess) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var previous []uint32
	failures := 0
	handedOff := false
	for {
		pids, err := jobProcessIDs(process.job)
		if err != nil {
			failures++
			lgx.Warnf("RetroBat session query failed attempt=%d err=%v", failures, err)
			if failures >= 3 {
				return fmt.Errorf("track RetroBat session: %w", err)
			}
		} else {
			failures = 0
			if !slices.Equal(previous, pids) {
				lgx.Infof("RetroBat session members pids=%v", pids)
				previous = pids
			}
			if len(pids) > 1 || len(pids) == 1 && pids[0] != process.pid {
				handedOff = true
			}
			if len(pids) == 0 {
				var exitCode uint32
				if err := windows.GetExitCodeProcess(process.root, &exitCode); err != nil {
					return fmt.Errorf("read RetroBat exit code: %w", err)
				}
				lgx.Infof("RetroBat session ended rootExitCode=%d handedOff=%v", exitCode, handedOff)
				if exitCode != 0 && !handedOff {
					return fmt.Errorf("RetroBat exited with status %d", exitCode)
				}
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// An existing frontend would receive a handoff outside our new job. Refuse it
// before starting rather than hiding StageBox around an untracked session.
func rejectExistingRetroBat(exe string) error {
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		return fmt.Errorf("identify RetroBat Windows session: %w", err)
	}
	entries, err := processEntries()
	if err != nil {
		return fmt.Errorf("inspect existing RetroBat sessions: %w", err)
	}
	expectedPaths := []string{exe, filepath.Join(filepath.Dir(exe), "emulationstation", "emulationstation.exe")}
	for _, entry := range entries {
		if !strings.EqualFold(entry.exeName, "retrobat.exe") && !strings.EqualFold(entry.exeName, "emulationstation.exe") {
			continue
		}
		var candidateSession uint32
		if err := windows.ProcessIdToSessionId(entry.pid, &candidateSession); err != nil || candidateSession != sessionID {
			continue
		}
		handle, err := windows.OpenProcess(processQueryLimitedInformation, false, entry.pid)
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_FOUND) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect RetroBat candidate pid=%d: %w", entry.pid, err)
		}
		var path [32768]uint16
		size := uint32(len(path))
		err = windows.QueryFullProcessImageName(handle, 0, &path[0], &size)
		_ = windows.CloseHandle(handle)
		if err != nil {
			return fmt.Errorf("identify RetroBat candidate pid=%d: %w", entry.pid, err)
		}
		actual := windows.UTF16ToString(path[:size])
		for _, expected := range expectedPaths {
			matches := strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected))
			if !matches {
				want, wantErr := os.Stat(expected)
				got, gotErr := os.Stat(actual)
				matches = wantErr == nil && gotErr == nil && os.SameFile(want, got)
			}
			if matches {
				return errors.New("RetroBat is already running. Quit RetroBat, then open it from StageBox so the session can be tracked")
			}
		}
	}
	return nil
}
