//go:build windows

package games

import (
	"errors"
	"fmt"
	lgx "launcher/internal/logx"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Ask Steam to exit normally once; never terminate its process tree or edit
// ActiveProcess. The replacement starts only after verified clients exit.
func recoverStaleSteamClient(tracker *steamTracker, game *GameData, stalePID uint32, warm steamClientRegistration, onStatus func(SteamLaunchStatus)) error {
	clients, _, err := tracker.sample()
	if err != nil || len(clients) != 1 || clients[0] != warm.pid {
		return fmt.Errorf("Cannot verify a single idle Steam client; fully exit Steam and retry (inspection: %v)", err)
	}
	client := tracker.processes[warm.pid]
	if client.lifetime.created != warm.created {
		return errors.New("Steam restarted during inspection; retry the game")
	}
	created := windows.Filetime{HighDateTime: uint32(warm.created >> 32), LowDateTime: uint32(warm.created)}
	if time.Since(time.Unix(0, created.Nanoseconds())) < 10*time.Second {
		return errors.New("Steam is still starting; let it finish sign-in or updates, then retry")
	}
	sameUser, clientElevated, launcherElevated, err := querySteamClientAccessContext(client.handle)
	if err != nil || !sameUser || clientElevated != launcherElevated {
		return errors.New("Cannot verify matching Steam access; fully exit Steam and reopen it using the topbar")
	}
	ui, err := tracker.clientUIActivity()
	if err != nil || ui.bigPicture || ui.gameProcesses != 0 {
		return errors.New("Steam may have another game or Big Picture open; close it and fully exit Steam before retrying")
	}
	if err := steamRecoveryLibraryIdle(tracker); err != nil {
		return fmt.Errorf("Cannot safely restart Steam: %w. Finish other Steam activity and fully exit Steam before retrying", err)
	}
	// Library/process inspection takes time. Recheck registration and the
	// retained client immediately before issuing the graceful shutdown request.
	registration, _, currentStale := registeredSteamClient(tracker)
	if registration.pid != 0 {
		return nil // Steam repaired itself; the caller still stabilizes it.
	}
	if currentStale != stalePID {
		return errors.New("Steam registration changed during inspection; retry the game")
	}
	if err := client.refresh(); err != nil || !client.alive || client.lifetime.created != warm.created {
		return errors.New("Steam changed during inspection; retry the game")
	}
	if onStatus != nil {
		onStatus(SteamLaunchStatus{Stage: "steam", Message: "Reconnecting to Steam…", Progress: -1, NeedsUpdate: game.SteamNeedsUpdate})
	}
	lgx.Infof("Steam stale registration recovery appID=%d stalePID=%d clientPID=%d; requesting graceful shutdown", game.SteamAppID, stalePID, warm.pid)
	cmd := exec.Command(game.Executable, "-shutdown")
	cmd.Dir = filepath.Dir(game.Executable)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Could not request Steam restart: %w", err)
	}
	exit := make(chan error, 1)
	go func() { exit <- cmd.Wait() }()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	commandDone := false
	for {
		clients, _, err = tracker.sample()
		if err != nil {
			return fmt.Errorf("Could not verify Steam shutdown: %w", err)
		}
		if commandDone && len(clients) == 0 {
			break
		}
		select {
		case err := <-exit:
			exit = nil
			commandDone = true
			lgx.Infof("Steam stale registration shutdown command exited appID=%d err=%v", game.SteamAppID, err)
			if err != nil {
				return fmt.Errorf("Steam declined the restart: %w", err)
			}
		case <-ticker.C:
		case <-timer.C:
			return errors.New("Steam did not exit normally; fully exit it using Steam's menu, then retry")
		}
	}
	cmd = exec.Command(game.Executable, "-silent")
	cmd.Dir = filepath.Dir(game.Executable)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Could not reopen Steam: %w", err)
	}
	restarted := cmd
	go func() {
		lgx.Infof("Steam stale registration restart command exited appID=%d err=%v", game.SteamAppID, restarted.Wait())
	}()
	lgx.Infof("Steam stale registration restart requested appID=%d requestPID=%d; waiting for fresh registration", game.SteamAppID, cmd.Process.Pid)
	return nil
}

// Check all configured libraries, not just the selected card. Manifests cover
// downloads/updates. Native process inspection is limited to the verified
// Steam tree; unrelated processes cannot veto recovery because of their DACL.
// A broker outside that tree without a manifest flag can escape this check.
func steamRecoveryLibraryIdle(tracker *steamTracker) error {
	root := filepath.Dir(tracker.exe)
	libraries := map[string]bool{root: true}
	foundInventory := false
	for _, path := range []string{filepath.Join(root, "steamapps", "libraryfolders.vdf"), filepath.Join(root, "config", "libraryfolders.vdf")} {
		vdf, err := readSteamVDF(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || vdf.objects["libraryfolders"] == nil {
			return fmt.Errorf("library inventory unavailable: %s", path)
		}
		foundInventory = true
		for _, library := range steamLibraryPaths(vdf.objects["libraryfolders"]) {
			libraries[filepath.Clean(library)] = true
		}
	}
	if !foundInventory {
		return errors.New("library inventory unavailable")
	}
	for library := range libraries {
		apps := filepath.Join(library, "steamapps")
		entries, err := os.ReadDir(apps)
		if err != nil {
			return fmt.Errorf("library unavailable: %s", apps)
		}
		for _, entry := range entries {
			name := strings.ToLower(entry.Name())
			if entry.IsDir() || !strings.HasPrefix(name, "appmanifest_") || !strings.HasSuffix(name, ".acf") {
				continue
			}
			vdf, err := readSteamVDF(filepath.Join(apps, entry.Name()))
			if err != nil || vdf.objects["appstate"] == nil {
				return fmt.Errorf("manifest unavailable: %s", entry.Name())
			}
			flags, err := strconv.ParseUint(vdf.objects["appstate"].values["stateflags"], 10, 32)
			if err != nil || uint32(flags)&(steamUpdateActivityFlags|64|512|2048) != 0 {
				return fmt.Errorf("Steam app busy or state unknown: %s", entry.Name())
			}
		}
	}
	return steamRecoverySteamIdle(tracker)
}

func steamRecoverySteamIdle(tracker *steamTracker) error {
	entries, err := steamRecoveryProcessEntries(tracker.sessionID)
	if err != nil {
		return err
	}
	candidates := steamRecoveryCandidates(tracker, entries)
	lgx.Debugf("Steam recovery process snapshot session=%d total=%d steamCandidates=%d; inspecting Steam activity only", tracker.sessionID, len(entries), len(candidates))
	var inspectionError error
	var busyPID uint32
	verifiedClient := false
	for _, entry := range candidates {
		process, err := steamRecoveryProcess(tracker, entry)
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_FOUND) {
			continue
		}
		if err != nil {
			if inspectionError == nil {
				inspectionError = fmt.Errorf("Steam process inspection unavailable pid=%d exe=%q: %w", entry.pid, entry.exeName, err)
			}
			continue // Inspect the remaining Steam candidates before deciding.
		}
		if process == nil {
			continue // Another Windows session.
		}
		if process.alive && tracker.clientExecutable(process.path) {
			verifiedClient = true
		} else if process.alive && !steamBackgroundHelper(process.path) {
			busyPID = entry.pid
		}
	}
	if busyPID != 0 {
		return fmt.Errorf("another Steam app is running pid=%d", busyPID)
	}
	if inspectionError != nil {
		return inspectionError
	}
	if !verifiedClient {
		return errors.New("Steam client changed during recovery inspection; retry the game")
	}
	return nil
}

type steamRecoveryProcessInfo struct {
	path    string
	alive   bool
	created uint64
}

func steamRecoveryProcess(tracker *steamTracker, entry steamRecoveryEntry) (*steamRecoveryProcessInfo, error) {
	// Session metadata comes from the snapshot, without a per-process query.
	// Keep the fixed system PID exclusions as a second line of defense.
	if entry.sessionID != tracker.sessionID || entry.pid == 0 || entry.pid == 4 {
		return nil, nil
	}
	// This is an idle check, not a retained lifecycle tracker. The native
	// snapshot already supplies the session; ProcessIdToSessionId requires
	// broader PROCESS_QUERY_INFORMATION rights, and waiting adds SYNCHRONIZE.
	// Neither is needed to query an image path, creation time or exit code.
	handle, err := windows.OpenProcess(processQueryLimitedInformation, false, entry.pid)
	if err != nil {
		return nil, fmt.Errorf("open for limited query: %w", err)
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return nil, fmt.Errorf("query creation time: %w", err)
	}
	creationTime := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	if entry.created == 0 || creationTime != entry.created {
		return nil, errors.New("process identity changed during recovery inspection; retry the game")
	}
	var path [32768]uint16
	size := uint32(len(path))
	if err := windows.QueryFullProcessImageName(handle, 0, &path[0], &size); err != nil {
		return nil, fmt.Errorf("query executable path: %w", err)
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return nil, fmt.Errorf("query exit status: %w", err)
	}
	// Treat STILL_ACTIVE (259) conservatively as running. An executable that
	// exits with that code can cause a refusal, never an unsafe idle result.
	return &steamRecoveryProcessInfo{path: filepath.Clean(windows.UTF16ToString(path[:size])), alive: exitCode == 259, created: creationTime}, nil
}
