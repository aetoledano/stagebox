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
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Validate the registry discovery data used by Steam API clients. Neither a
// bootstrapper PID nor a responsive Chromium window proves API readiness.
// DLLs are inspected as files only; StageBox never loads or modifies them.
// The third result identifies a dead PID agreed upon by both registry views
// with valid DLL paths; other rejection reasons cannot authorize recovery.
func registeredSteamClient(tracker *steamTracker) (steamClientRegistration, string, uint32) {
	var rejected []string
	var deadPIDs []uint32
	for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
		key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam\ActiveProcess`, registry.QUERY_VALUE|view)
		if err != nil {
			rejected = append(rejected, fmt.Sprintf("view=%d open=%v", view, err))
			continue
		}
		pid, _, pidErr := key.GetIntegerValue("pid")
		dll32, dll32Err := steamRegisteredDLL(key, "SteamClientDll")
		dll64, dll64Err := steamRegisteredDLL(key, "SteamClientDll64")
		_ = key.Close()
		if pidErr != nil || pid == 0 || pid > uint64(^uint32(0)) {
			rejected = append(rejected, fmt.Sprintf("view=%d pid=%d read=%v", view, pid, pidErr))
			continue
		}
		process, err := tracker.open(uint32(pid))
		if err != nil || process == nil {
			rejected = append(rejected, fmt.Sprintf("view=%d pid=%d handle=%v sameSession=%v dll32=%q dll32Err=%v dll64=%q dll64Err=%v", view, pid, err, process != nil, dll32, dll32Err, dll64, dll64Err))
			if dll32Err == nil && dll64Err == nil && (errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_FOUND)) {
				deadPIDs = append(deadPIDs, uint32(pid))
			}
			continue
		}
		matches := process.alive && tracker.clientExecutable(process.path)
		registration := steamClientRegistration{pid: uint32(pid), created: process.lifetime.created, dll32: dll32, dll64: dll64}
		_ = windows.CloseHandle(process.handle)
		if !matches {
			rejected = append(rejected, fmt.Sprintf("view=%d pid=%d alive=%v path=%q", view, pid, process.alive, process.path))
			if !process.alive && dll32Err == nil && dll64Err == nil {
				deadPIDs = append(deadPIDs, uint32(pid))
			}
			continue
		}
		if dll32Err != nil || dll64Err != nil {
			rejected = append(rejected, fmt.Sprintf("view=%d pid=%d dll32=%v dll64=%v", view, pid, dll32Err, dll64Err))
			continue
		}
		return registration, "", 0
	}
	var stalePID uint32
	if len(deadPIDs) == 2 && deadPIDs[0] == deadPIDs[1] {
		stalePID = deadPIDs[0]
	}
	return steamClientRegistration{}, strings.Join(rejected, "; "), stalePID
}

func steamRegisteredDLL(key registry.Key, name string) (string, error) {
	path, kind, err := key.GetStringValue(name)
	if err != nil {
		return "", fmt.Errorf("%s registration: %w", name, err)
	}
	if kind == registry.EXPAND_SZ {
		path, err = registry.ExpandString(path)
		if err != nil {
			return "", fmt.Errorf("%s expand: %w", name, err)
		}
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s registration is empty", name)
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s path=%q: %w", name, path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s path=%q is not a regular file", name, path)
	}
	return path, nil
}

func ensureSteamClientReady(game *GameData, onStatus func(SteamLaunchStatus)) error {
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		return err
	}
	tracker := steamTracker{exe: filepath.Clean(game.Executable), sessionID: sessionID, processes: map[uint32]*steamTrackedProcess{}}
	defer tracker.close()
	cleanupFailed := false
	cleanUI := func() {
		if err := hideMinimizedSteamClientUI(&tracker); err != nil && !cleanupFailed {
			lgx.Warnf("Steam minimized-window cleanup during client startup: %v", err)
			cleanupFailed = true
		}
	}
	var activationExit chan error
	activationIssued := false
	coldStart := false
	status, registryReason := "", ""
	started, nextSnapshot := time.Now(), time.Now()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(steamGameStartGrace)
	defer timeout.Stop()
	var activationErr error
	var readiness steamClientReadiness
	var stale steamStaleClientRecovery
	warmClients := map[uint32]uint64{}
	capturedWarmClients, recoveryAttempted := false, false
	for {
		cleanUI()
		if !capturedWarmClients {
			for pid, process := range tracker.processes {
				if process.client && process.alive {
					warmClients[pid] = process.lifetime.created
				}
			}
			capturedWarmClients = true
		}
		registration, reason, stalePID := registeredSteamClient(&tracker)
		now := time.Now()
		if reason != registryReason {
			lgx.Infof("Steam game client registration pending appID=%d reason=%s", game.SteamAppID, reason)
			registryReason = reason
		}
		if !now.Before(nextSnapshot) {
			logSteamClientSnapshot(&tracker, game.SteamAppID, "waiting", now.Sub(started), registration, reason)
			nextSnapshot = now.Add(10 * time.Second)
		}
		if readiness.observe(now, registration) {
			logSteamClientSnapshot(&tracker, game.SteamAppID, "ready", now.Sub(started), registration, "")
			lgx.Infof("Steam game client ready appID=%d clientPID=%d stableFor=%s; requesting game", game.SteamAppID, registration.pid, now.Sub(readiness.stableSince).Round(time.Millisecond))
			return nil
		}
		clientAlive := false
		var warm steamClientRegistration
		clientCount := 0
		for _, process := range tracker.processes {
			clientAlive = clientAlive || (process.client && process.alive)
		}
		for pid, process := range tracker.processes {
			if !process.client || !process.alive {
				continue
			}
			clientCount++
			if created, ok := warmClients[pid]; ok && created == process.lifetime.created {
				filetime := windows.Filetime{HighDateTime: uint32(created >> 32), LowDateTime: uint32(created)}
				if now.Sub(time.Unix(0, filetime.Nanoseconds())) >= 10*time.Second {
					warm = steamClientRegistration{pid: pid, created: created}
				}
			}
		}
		if clientCount != 1 {
			warm = steamClientRegistration{}
		}
		if !recoveryAttempted && stale.observe(now, stalePID, warm) {
			recoveryAttempted = true
			if err := recoverStaleSteamClient(&tracker, game, stalePID, warm, onStatus); err != nil {
				lgx.Warnf("Steam stale registration recovery refused/failed appID=%d stalePID=%d clientPID=%d err=%v", game.SteamAppID, stalePID, warm.pid, err)
				return fmt.Errorf("Steam's game connection is stale. %w", err)
			}
			// Recovery either restarted Steam once or observed self-repair.
			// Neither case needs another activation before stabilization.
			activationIssued = true
			coldStart = !tracker.processes[warm.pid].alive
			readiness = steamClientReadiness{}
			status = ""
			continue
		}
		// One silent singleton activation only. An unregistered warm client
		// gets a chance to initialize first; reactivation never bypasses the
		// registration gate and never directly starts a game.
		if !activationIssued && registration.pid == 0 && (!clientAlive || now.Sub(started) >= 2*time.Second) {
			cmd := exec.Command(game.Executable, "-silent")
			cmd.Dir = filepath.Dir(game.Executable)
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("activate Steam before launching game: %w", err)
			}
			activationIssued, coldStart = true, !clientAlive
			activationExit = make(chan error, 1)
			exit := activationExit
			go func() { exit <- cmd.Wait() }()
			lgx.Infof("Steam game client activation appID=%d cold=%v requestPID=%d; waiting for stable registration", game.SteamAppID, coldStart, cmd.Process.Pid)
		}
		message := "Waiting for Steam…"
		if coldStart {
			message = "Starting Steam…"
		} else if registration.pid != 0 {
			message = "Connecting to Steam…"
		}
		if onStatus != nil && status != message {
			onStatus(SteamLaunchStatus{Stage: "steam", Message: message, Progress: -1, NeedsUpdate: game.SteamNeedsUpdate})
			status = message
		}
		select {
		case err := <-activationExit:
			activationErr = err
			activationExit = nil
			lgx.Infof("Steam client activation exited appID=%d err=%v; awaiting stable registration", game.SteamAppID, err)
		case <-ticker.C:
		case <-timeout.C:
			logSteamClientSnapshot(&tracker, game.SteamAppID, "timeout", time.Since(started), registration, registryReason)
			lgx.Warnf("Steam client registration timed out appID=%d activationErr=%v reason=%s", game.SteamAppID, activationErr, registryReason)
			return fmt.Errorf("Steam's game connection did not become ready. Open Steam using the topbar, finish any sign-in or client update, then try again")
		}
	}
}

// Snapshots deliberately omit account names, Steam IDs, credentials and
// environment contents. Access-level/user-context differences can explain API
// failures, but they are diagnostic evidence, not a reason to restart Steam.
func logSteamClientSnapshot(tracker *steamTracker, appID uint32, stage string, elapsed time.Duration, registration steamClientRegistration, reason string) {
	clients := make([]string, 0)
	for pid, process := range tracker.processes {
		if !process.client || !process.alive {
			continue
		}
		created := windows.Filetime{HighDateTime: uint32(process.lifetime.created >> 32), LowDateTime: uint32(process.lifetime.created)}
		clients = append(clients, fmt.Sprintf("pid=%d age=%s %s", pid, time.Since(time.Unix(0, created.Nanoseconds())).Round(time.Millisecond), steamClientAccessContext(process.handle)))
	}
	slices.Sort(clients)
	lgx.Infof("Steam readiness snapshot policy=registered-stable-v3 appID=%d stage=%s elapsed=%s session=%d exe=%q clients=%v registeredPID=%d dll32=%q dll64=%q reason=%q", appID, stage, elapsed.Round(time.Millisecond), tracker.sessionID, tracker.exe, clients, registration.pid, registration.dll32, registration.dll64, reason)
}

func steamClientAccessContext(handle windows.Handle) string {
	sameUser, clientElevated, launcherElevated, err := querySteamClientAccessContext(handle)
	if err != nil {
		return fmt.Sprintf("contextQuery=%v", err)
	}
	return fmt.Sprintf("sameUser=%v clientElevated=%v launcherElevated=%v", sameUser, clientElevated, launcherElevated)
}

func querySteamClientAccessContext(handle windows.Handle) (bool, bool, bool, error) {
	var client, launcher windows.Token
	if err := windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &client); err != nil {
		return false, false, false, err
	}
	defer client.Close()
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &launcher); err != nil {
		return false, false, false, err
	}
	defer launcher.Close()
	clientUser, clientErr := client.GetTokenUser()
	launcherUser, launcherErr := launcher.GetTokenUser()
	var clientElevated, launcherElevated, length uint32
	elevationErr := windows.GetTokenInformation(client, windows.TokenElevation, (*byte)(unsafe.Pointer(&clientElevated)), uint32(unsafe.Sizeof(clientElevated)), &length)
	launcherElevationErr := windows.GetTokenInformation(launcher, windows.TokenElevation, (*byte)(unsafe.Pointer(&launcherElevated)), uint32(unsafe.Sizeof(launcherElevated)), &length)
	if clientErr != nil || launcherErr != nil || elevationErr != nil || launcherElevationErr != nil {
		return false, false, false, errors.Join(clientErr, launcherErr, elevationErr, launcherElevationErr)
	}
	return clientUser.User.Sid.Equals(launcherUser.User.Sid), clientElevated != 0, launcherElevated != 0, nil
}
