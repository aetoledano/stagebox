//go:build windows

package games

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestSteamGameSpawnHelper(t *testing.T) {
	if len(os.Args) < 5 || os.Args[len(os.Args)-4] != "--stagebox-steam-game-spawn" {
		return
	}
	trigger, childExe, report := os.Args[len(os.Args)-3], os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(trigger); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("parent was never released")
		}
		time.Sleep(20 * time.Millisecond)
	}
	child := exec.Command(childExe, "-test.run=^TestSteamTrackingHelper$", "--", "--stagebox-steam-helper")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	writeSteamFixture(t, report, []byte(strconv.Itoa(child.Process.Pid)))
	_ = child.Process.Release()
}

func TestSteamGameJobTracksChildOutsideInstallDirectoryAfterParentExit(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	install := filepath.Join(root, "Selected")
	parentExe, childExe := filepath.Join(install, "Launcher.exe"), filepath.Join(root, "Broker", "Child.exe")
	trigger, report := filepath.Join(root, "start"), filepath.Join(root, "child.pid")
	writeSteamFixture(t, parentExe, data)
	writeSteamFixture(t, childExe, data)
	parent := exec.Command(parentExe, "-test.run=^TestSteamGameSpawnHelper$", "--", "--stagebox-steam-game-spawn", trigger, childExe, report)
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Process.Kill(); _ = parent.Wait() })
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		t.Fatal(err)
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &steamGameProcess{
		game: &GameData{SteamAppID: 42, StartsOn: install}, job: job, startedPID: make(chan int, 1),
		tracker: steamTracker{exe: filepath.Join(root, "Steam.exe"), sessionID: sessionID, processes: map[uint32]*steamTrackedProcess{}},
	}
	defer func() { _ = windows.TerminateJobObject(job, 1); p.Close() }()
	if _, _, err := p.sampleLocked(); err != nil {
		t.Fatal(err)
	}
	writeSteamFixture(t, trigger, []byte("start"))
	if err := parent.Wait(); err != nil {
		t.Fatal(err)
	}
	pidData, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(string(pidData))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if process, err := os.FindProcess(childPID); err == nil {
			_ = process.Kill()
			_ = process.Release()
		}
	})
	pids, _, err := p.sampleLocked()
	if err != nil || !slices.Equal(pids, []uint32{uint32(childPID)}) {
		t.Fatalf("parent exit lost external child: pids=%v err=%v", pids, err)
	}
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
}

func TestSteamGameJobAndKillLeaveClientAndOtherGamesRunning(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	install := filepath.Join(root, "steamapps", "common", "Selected")
	steamExe := filepath.Join(root, "Steam.exe")
	gameExe := filepath.Join(install, "Game.exe")
	otherExe := filepath.Join(root, "steamapps", "common", "Other", "Game.exe")
	start := func(path string) *exec.Cmd {
		writeSteamFixture(t, path, data)
		cmd := exec.Command(path, "-test.run=^TestSteamTrackingHelper$", "--", "--stagebox-steam-helper")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	client, game, other := start(steamExe), start(gameExe), start(otherExe)
	reporter := start(filepath.Join(install, "Engine", "CrashReportClient.exe"))
	crashpad := start(filepath.Join(install, "Crashpad", "crashpad_handler.exe"))
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		t.Fatal(err)
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &steamGameProcess{
		game: &GameData{SteamAppID: 42, StartsOn: install, Executable: steamExe}, job: job,
		startedPID: make(chan int, 1),
		tracker:    steamTracker{exe: steamExe, sessionID: sessionID, processes: map[uint32]*steamTrackedProcess{}},
	}
	defer p.Close()
	pids, _, err := p.sampleLocked()
	expected := []uint32{uint32(game.Process.Pid), uint32(reporter.Process.Pid), uint32(crashpad.Process.Pid)}
	slices.Sort(expected)
	if err != nil || !slices.Equal(pids, expected) {
		t.Fatalf("wrong game processes: %v err=%v", pids, err)
	}
	if active := p.activityPIDs(pids); !slices.Equal(active, []uint32{uint32(game.Process.Pid)}) {
		t.Fatalf("reporting helpers counted as game activity: %v", active)
	}
	jobPIDs, err := jobProcessIDs(job)
	if err != nil || !slices.Equal(jobPIDs, pids) {
		t.Fatalf("game was not attached to advanced tracking job: %v err=%v", jobPIDs, err)
	}
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := game.Wait(); err == nil {
		t.Fatal("game was not forcibly stopped")
	}
	for _, helper := range []*exec.Cmd{reporter, crashpad} {
		if err := helper.Wait(); err == nil {
			t.Fatalf("reporting helper was not included in game cleanup: pid=%d", helper.Process.Pid)
		}
	}
	for _, cmd := range []*exec.Cmd{client, other} {
		if !processExists(cmd.Process.Pid) {
			t.Fatalf("Steam game kill stopped unrelated pid=%d", cmd.Process.Pid)
		}
	}
}
