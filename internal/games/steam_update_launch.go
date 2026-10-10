package games

import (
	"context"
	"fmt"
	lgx "launcher/internal/logx"
	"time"
)

func steamGameUpdateArgs(appID uint32) []string {
	return []string{"-silent", fmt.Sprintf("steam://install/%d", appID)}
}

// WaitForSteamUpdate requests only content installation, never a game launch.
// Cancellation abandons our wait; Steam owns and retains the download.
func WaitForSteamUpdate(ctx context.Context, game *GameData, onStatus func(SteamLaunchStatus)) error {
	if game == nil || game.SteamAppID == 0 {
		return fmt.Errorf("invalid Steam game data")
	}
	if err := validateSteamGameForUpdate(game, true); err != nil {
		return err
	}
	initial, err := readSteamGameState(game)
	if err != nil {
		return err
	}
	if !initial.needsUpdate() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Do not reactivate the install UI for a download already in progress.
	if initial.flags&steamUpdateActivityFlags == 0 || initial.flags&512 != 0 {
		if err := requestSteamGameUpdate(game); err != nil {
			return err
		}
	} else {
		lgx.Infof("Steam update wait reused appID=%d flags=%d; game launch deferred", game.SteamAppID, initial.flags)
	}
	return waitForSteamUpdate(ctx, game, initial, onStatus)
}

func waitForSteamUpdate(ctx context.Context, game *GameData, initial steamGameState, onStatus func(SteamLaunchStatus)) error {
	state := steamGameSessionState{started: time.Now()}
	state.observeUpdate(state.started, initial)
	last := initial
	var lastStatus SteamLaunchStatus
	var lastLog, completeSince time.Time
	var readError string
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			lgx.Infof("Steam update wait left appID=%d; no game launch requested, download retained", game.SteamAppID)
			return err
		}
		now := time.Now()
		update, err := readSteamGameState(game)
		if err != nil {
			completeSince = time.Time{}
			if err.Error() != readError {
				lgx.Warnf("Steam update manifest unavailable appID=%d err=%v; retaining last known state", game.SteamAppID, err)
				readError = err.Error()
			}
		} else {
			if readError != "" {
				lgx.Infof("Steam update manifest readable again appID=%d", game.SteamAppID)
				readError = ""
			}
			last = update
			state.observeUpdate(now, update)
			if !update.needsUpdate() {
				if err := validateSteamGame(game); err != nil {
					return err
				}
				if completeSince.IsZero() {
					completeSince = now
				} else if now.Sub(completeSince) >= time.Second {
					lgx.Infof("Steam update complete appID=%d; returning to launch preflight", game.SteamAppID)
					return ctx.Err()
				}
			} else {
				completeSince = time.Time{}
			}
		}
		status := last.launchStatus()
		status.CanLeaveUpdate = true
		if !last.needsUpdate() {
			status.Stage, status.Message = "updating", "Steam update finished. Preparing to launch…"
		}
		if status != lastStatus || now.Sub(lastLog) >= 30*time.Second {
			var idle time.Duration
			if state.updateWait {
				idle = now.Sub(state.updateAt).Round(time.Second)
			}
			lgx.Infof("Steam update-only status appID=%d flags=%d downloaded=%d/%d staged=%d/%d updateIdle=%s message=%q", game.SteamAppID, last.flags, last.downloaded, last.downloadTotal, last.staged, last.stageTotal, idle, status.Message)
			lastLog = now
		}
		if status != lastStatus {
			if onStatus != nil {
				onStatus(status)
			}
			lastStatus = status
		}
		if finished, _ := state.finished(now, false, 0); finished {
			if last.flags&512 != 0 {
				return fmt.Errorf("Steam update is paused; resume the download in Steam, then retry")
			}
			return fmt.Errorf("Steam has not reported update progress for %s; check its download queue or update dialogs, then retry", steamGameUpdateGrace)
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}
