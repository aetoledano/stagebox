//go:build windows

package games

import (
	"fmt"
	lgx "launcher/internal/logx"
	"os/exec"
	"path/filepath"
	"time"
)

func (p *steamGameProcess) requestUI(args []string) error {
	stopSteamUIBackgrounding()
	cmd := exec.Command(p.game.Executable, args...)
	cmd.Dir = filepath.Dir(p.game.Executable)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open Steam game interface: %w", err)
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			lgx.Warnf("Steam interactive activation exited appID=%d err=%v", p.game.SteamAppID, err)
		}
	}()
	return nil
}

// Caller owns p.mu. UI processes live in a separate tracker and can never
// enter the selected game's job or kill set.
func (p *steamGameProcess) sampleInteractiveUI(gameProcesses int) (bool, error) {
	if p.clientUI == nil {
		p.clientUI = &steamTracker{exe: p.tracker.exe, sessionID: p.tracker.sessionID, processes: map[uint32]*steamTrackedProcess{}}
	}
	clients, _, err := p.clientUI.sample()
	if err != nil {
		return false, err
	}
	ui, err := p.clientUI.clientUIActivity()
	if err != nil {
		return false, err
	}
	if ui.bigPicture && p.updateOnly && !p.detailsRequested && !p.killed.Load() {
		if err := p.requestUI(steamGameDetailsArgs(p.game.SteamAppID)); err != nil {
			return false, err
		}
		p.detailsRequested = true
		lgx.Infof("Steam interactive game page requested appID=%d; choose Update or Play in Big Picture", p.game.SteamAppID)
	}
	if (ui.bigPicture || ui.dialogPID != 0) && !p.uiReady {
		p.uiReady = true
		if p.onUIReady != nil {
			p.onUIReady()
		}
	}
	if p.onDialog != nil {
		p.onDialog(ui.dialogPID, ui.dialogWindow)
	}
	setupDialog := p.uiReady && ui.dialog
	finished := p.setup.finished(time.Now(), len(clients), ui.bigPicture || setupDialog, ui.bigPicture || (p.uiReady && ui.minimizedBigPicture), setupDialog, gameProcesses)
	if finished && !p.setup.sawActivity {
		return false, fmt.Errorf("Steam showed no setup interface; open Steam from the top bar and finish sign-in or client updates")
	}
	return finished, nil
}
