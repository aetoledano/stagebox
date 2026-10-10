package games

import (
	"fmt"
	"strconv"
)

const steamUpdateActivityFlags = 256 | 1024 | 131072 | 262144 | 524288 | 1048576 | 2097152 | 4194304

type steamGameState struct {
	flags                                         uint32
	downloaded, downloadTotal, staged, stageTotal uint64
}

func readSteamGameState(game *GameData) (steamGameState, error) {
	file, err := readSteamVDF(game.steamManifest)
	if err != nil {
		return steamGameState{}, err
	}
	state := file.objects["appstate"]
	if state == nil || state.values["appid"] != strconv.FormatUint(uint64(game.SteamAppID), 10) {
		return steamGameState{}, fmt.Errorf("Steam manifest identity changed")
	}
	flags, err := strconv.ParseUint(state.values["stateflags"], 10, 32)
	if err != nil {
		return steamGameState{}, fmt.Errorf("invalid Steam state flags: %w", err)
	}
	value := func(key string) uint64 { number, _ := strconv.ParseUint(state.values[key], 10, 64); return number }
	return steamGameState{flags: uint32(flags), downloaded: value("bytesdownloaded"), downloadTotal: value("bytestodownload"), staged: value("bytesstaged"), stageTotal: value("bytestostage")}, nil
}

func (s steamGameState) needsUpdate() bool { return s.flags&(2|steamUpdateActivityFlags|512) != 0 }

func SteamUpdateNeeded(game *GameData) (bool, error) {
	state, err := readSteamGameState(game)
	return state.needsUpdate(), err
}

type SteamLaunchStatus struct {
	Stage          string `json:"stage"`
	Message        string `json:"message"`
	Progress       int    `json:"progress"` // Current phase percentage; -1 when no measurable phase is reported.
	NeedsUpdate    bool   `json:"needsUpdate"`
	CanLeaveUpdate bool   `json:"canLeaveUpdate"`
}

func (s steamGameState) launchStatus() SteamLaunchStatus {
	status := SteamLaunchStatus{Stage: "starting", Message: "Starting game…", Progress: -1}
	if !s.needsUpdate() {
		return status
	}
	status.Stage, status.NeedsUpdate = "updating", true
	status.Message = "Steam needs to update this game before it starts."
	if s.flags&512 != 0 {
		status.Message = "Steam update is paused. Resume the download in Steam."
	} else if s.flags&(131072|262144|524288|2097152|4194304) != 0 {
		status.Message = "Steam is preparing or installing the update…"
		status.Progress = steamByteProgress(s.staged, s.stageTotal)
	} else if s.flags&1048576 != 0 {
		status.Message = "Steam is downloading the update…"
		status.Progress = steamByteProgress(s.downloaded, s.downloadTotal)
	} else if s.flags&(256|1024) != 0 {
		// UpdateRunning/UpdateStarted also cover work before any bytes arrive.
		// Some manifests expose only these generic flags during a transfer;
		// show reported partial bytes without claiming a specific active phase.
		status.Message = "Steam is preparing the update…"
		if s.downloaded > 0 && s.downloaded < s.downloadTotal {
			status.Message = "Steam update download progress…"
			status.Progress = steamByteProgress(s.downloaded, s.downloadTotal)
		} else if s.downloadTotal > 0 && s.downloaded >= s.downloadTotal && s.staged > 0 && s.staged < s.stageTotal {
			// Generic flags can persist into staging too. Completed download
			// bytes do not describe installation; expose its partial snapshot.
			status.Message = "Steam update installation progress…"
			status.Progress = steamByteProgress(s.staged, s.stageTotal)
		}
	}
	return status
}

func steamByteProgress(done, total uint64) int {
	if total == 0 {
		return -1
	}
	if done >= total {
		return 100
	}
	return int(float64(done) / float64(total) * 100)
}
