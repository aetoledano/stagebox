//go:build windows

package games

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestSteamTrackingHelper(t *testing.T) {
	if os.Args[len(os.Args)-1] == "--stagebox-steam-helper" {
		time.Sleep(time.Minute)
	}
}

func TestSteamTrackerFollowsIndependentRestart(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	steamExe := filepath.Join(t.TempDir(), "Steam.exe")
	otherExe := filepath.Join(t.TempDir(), "Steam.exe")
	for _, path := range []string{steamExe, otherExe} {
		if err := os.WriteFile(path, data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		t.Fatal(err)
	}
	tracker := steamTracker{exe: steamExe, sessionID: sessionID, processes: make(map[uint32]*steamTrackedProcess)}
	defer tracker.close()
	start := func(path string) *exec.Cmd {
		cmd := exec.Command(path, "-test.run=^TestSteamTrackingHelper$", "--", "--stagebox-steam-helper")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		})
		return cmd
	}
	first := start(steamExe)
	start(otherExe)
	clients, _, err := tracker.sample()
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 || clients[0] != uint32(first.Process.Pid) {
		t.Fatalf("did not attach to the correct existing client: %v", clients)
	}
	otherSession := steamTracker{exe: steamExe, sessionID: ^uint32(0), processes: make(map[uint32]*steamTrackedProcess)}
	defer otherSession.close()
	clients, _, err = otherSession.sample()
	if err != nil || len(clients) != 0 {
		t.Fatalf("client from another Windows session was accepted: clients=%v err=%v", clients, err)
	}
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = first.Wait() // Forced termination is expected to report an exit error.
	clients, _, err = tracker.sample()
	if err != nil || len(clients) != 0 {
		t.Fatalf("exited client was kept active: clients=%v err=%v", clients, err)
	}
	// This process is launched by the test, not the old Steam client or its children.
	replacement := start(steamExe)
	clients, _, err = tracker.sample()
	if err != nil || len(clients) != 1 || clients[0] != uint32(replacement.Process.Pid) {
		t.Fatalf("independent replacement was not found: clients=%v err=%v", clients, err)
	}
}

func TestSteamBackgroundHelpers(t *testing.T) {
	for _, name := range []string{"steamwebhelper.exe", "SteamService.exe", "GameOverlayUI.exe", "crashhandler64.exe"} {
		if !steamBackgroundHelper(filepath.Join(`C:\Steam`, name)) {
			t.Fatalf("helper was counted as a game: %s", name)
		}
	}
	if steamBackgroundHelper(`C:\Steam\steamapps\common\Game\Game.exe`) {
		t.Fatal("a game in the Steam installation must not be excluded")
	}
}
