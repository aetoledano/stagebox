//go:build windows

package games

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestSteamRecoveryIgnoresUnrelatedDeniedProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	start := func(name string) *exec.Cmd {
		path := filepath.Join(root, name)
		writeSteamFixture(t, path, data)
		cmd := exec.Command(path, "-test.run=^TestSteamTrackingHelper$", "--", "--stagebox-steam-helper")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	client := start("Steam.exe")
	unrelated := start("NVDisplay.Container.exe")
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessionID); err != nil {
		t.Fatal(err)
	}
	tracker := steamTracker{exe: filepath.Join(root, "Steam.exe"), sessionID: sessionID, processes: map[uint32]*steamTrackedProcess{}}
	defer tracker.close()
	process, err := tracker.open(uint32(client.Process.Pid))
	if err != nil || process == nil {
		t.Fatalf("cannot verify fixture Steam client: %v", err)
	}
	process.client = true
	tracker.add(uint32(client.Process.Pid), process)
	pid := uint32(unrelated.Process.Pid)
	retained, err := tracker.open(pid)
	if err != nil || retained == nil {
		t.Fatalf("cannot retain fixture identity before denying access: %v", err)
	}
	defer windows.CloseHandle(retained.handle)
	handle, err := windows.OpenProcess(windows.WRITE_DAC, false, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	sd, err := windows.SecurityDescriptorFromString("D:(D;;0x00101400;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetSecurityInfo(handle, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if h, err := windows.OpenProcess(processQueryLimitedInformation, false, pid); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		if err == nil {
			windows.CloseHandle(h)
		}
		t.Fatalf("fixture did not deny limited process inspection: %v", err)
	}
	if err := steamRecoverySteamIdle(&tracker); err != nil {
		t.Fatalf("unrelated protected process blocked idle Steam: %v", err)
	}
	// The same denial must block once the identity belongs to Steam's
	// retained tracking set, even though the client can still be inspected.
	tracker.processes[pid] = retained
	defer delete(tracker.processes, pid) // The separate defer owns this handle.
	if err := steamRecoverySteamIdle(&tracker); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("successful client query hid an unknown Steam process: %v", err)
	}
}

func TestSteamRecoveryCandidatesFollowVerifiedLifetimes(t *testing.T) {
	tracker := steamTracker{processes: map[uint32]*steamTrackedProcess{
		100: {client: true, lifetime: steamProcessLifetime{created: 10}},
		300: {client: true, lifetime: steamProcessLifetime{created: 10, exited: 15}},
	}}
	entries := []steamRecoveryEntry{
		{processEntry: processEntry{pid: 100, exeName: "Steam.exe"}, created: 10},
		{processEntry: processEntry{pid: 101, parentPID: 100, exeName: "steamwebhelper.exe"}, created: 11},
		{processEntry: processEntry{pid: 102, parentPID: 101, exeName: "Game.exe"}, created: 12},
		{processEntry: processEntry{pid: 200, parentPID: 500, exeName: "NVDisplay.Container.exe"}, created: 13},
		{processEntry: processEntry{pid: 201, parentPID: 100, exeName: "BeforeSteam.exe"}, created: 5},
		{processEntry: processEntry{pid: 301, parentPID: 300, exeName: "RetainedHandoff.exe"}, created: 12},
		{processEntry: processEntry{pid: 302, parentPID: 300, exeName: "ReusedParent.exe"}, created: 16},
	}
	var pids []uint32
	for _, entry := range steamRecoveryCandidates(&tracker, entries) {
		pids = append(pids, entry.pid)
	}
	slices.Sort(pids)
	if !slices.Equal(pids, []uint32{100, 101, 102, 301}) {
		t.Fatalf("lost Steam handoffs or inspected unrelated/reused PIDs: %v", pids)
	}
}

func TestSteamRecoverySnapshotFiltersSessionsAndVerifiesIdentity(t *testing.T) {
	pid := windows.GetCurrentProcessId()
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(pid, &sessionID); err != nil {
		t.Fatal(err)
	}
	tracker := steamTracker{sessionID: sessionID}
	entries, err := steamRecoveryProcessEntries(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var current steamRecoveryEntry
	for _, entry := range entries {
		if entry.sessionID != sessionID || entry.pid == 0 || entry.pid == 4 {
			t.Fatalf("snapshot retained another session or a system PID: %+v", entry)
		}
		if entry.pid == pid {
			current = entry
		}
	}
	if current.pid == 0 || current.created == 0 || current.exeName == "" {
		t.Fatal("native snapshot did not identify the current process")
	}
	for _, systemPID := range []uint32{0, 4} {
		process, err := steamRecoveryProcess(&tracker, steamRecoveryEntry{processEntry: processEntry{pid: systemPID, exeName: "System"}, sessionID: sessionID})
		if err != nil || process != nil {
			t.Fatalf("system PID %d blocked recovery: process=%v err=%v", systemPID, process, err)
		}
	}
	foreign := current
	foreign.sessionID++
	foreign.exeName = "Registry"
	if process, err := steamRecoveryProcess(&tracker, foreign); process != nil || err != nil {
		t.Fatalf("other-session process was queried: process=%v err=%v", process, err)
	}
	// A system-looking basename does not grant the same exception to an
	// ordinary PID: this must open and verify the actual native process.
	current.exeName = "System"
	process, err := steamRecoveryProcess(&tracker, current)
	if err != nil || process == nil {
		t.Fatalf("ordinary process was skipped or not queried: process=%v err=%v", process, err)
	}
	if !process.alive || process.created != current.created || process.path == "" {
		t.Fatal("ordinary process did not retain native identity/liveness checks")
	}
	current.created++ // Same PID, different native lifetime must fail closed.
	if process, err := steamRecoveryProcess(&tracker, current); process != nil || err == nil {
		t.Fatalf("changed process identity was accepted: process=%v err=%v", process, err)
	}
}

func TestSteamRecoveryInspectsProcessWithOnlyLimitedQueryAccess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe, "-test.run=^TestSteamTrackingHelper$", "--", "--stagebox-steam-helper")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	pid := uint32(child.Process.Pid)
	// Modify only this disposable child's DACL. Retained handles keep their
	// original rights, so cleanup can still terminate/reap it on any failure.
	handle, err := windows.OpenProcess(windows.WRITE_DAC, false, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	sd, err := windows.SecurityDescriptorFromString("D:(D;;0x00100400;;;WD)(A;;0x00001000;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetSecurityInfo(handle, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessionID); err != nil {
		t.Fatal(err)
	}
	tracker := steamTracker{sessionID: sessionID}
	if process, err := tracker.open(pid); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		if process != nil {
			windows.CloseHandle(process.handle)
		}
		t.Fatalf("fixture did not deny the previous inspection path: %v", err)
	}
	entries, err := steamRecoveryProcessEntries(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.pid != pid {
			continue
		}
		process, err := steamRecoveryProcess(&tracker, entry)
		if err != nil || process == nil {
			t.Fatalf("limited-query process blocked recovery: process=%v err=%v", process, err)
		}
		if !process.alive || process.created != entry.created || !strings.EqualFold(process.path, filepath.Clean(exe)) {
			t.Fatalf("limited inspection lost activity/identity/path: %+v", process)
		}
		return
	}
	t.Fatal("child absent from native snapshot")
}

func TestSteamRecoverySnapshotRejectsMalformedRecords(t *testing.T) {
	headerSize := int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}))
	for _, test := range []struct {
		name string
		edit func([]byte, *windows.SYSTEM_PROCESS_INFORMATION)
	}{
		{name: "next record outside buffer", edit: func(buffer []byte, info *windows.SYSTEM_PROCESS_INFORMATION) {
			info.NextEntryOffset = uint32(len(buffer) + 1)
		}},
		{name: "overlapping next record", edit: func(_ []byte, info *windows.SYSTEM_PROCESS_INFORMATION) {
			info.NextEntryOffset = 1
		}},
		{name: "unaligned next record", edit: func(_ []byte, info *windows.SYSTEM_PROCESS_INFORMATION) {
			info.NextEntryOffset = uint32(headerSize + 1)
		}},
		{name: "name outside buffer", edit: func(buffer []byte, info *windows.SYSTEM_PROCESS_INFORMATION) {
			info.ImageName.Length, info.ImageName.MaximumLength = 4, 4
			info.ImageName.Buffer = (*uint16)(unsafe.Pointer(&buffer[len(buffer)-2]))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			buffer := make([]byte, headerSize*2+8)
			info := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buffer[0]))
			info.UniqueProcessID, info.SessionID, info.CreateTime = 123, 7, 1
			test.edit(buffer, info)
			if entries, err := parseSteamRecoveryProcessEntries(buffer, 7); err == nil || entries != nil {
				t.Fatalf("malformed snapshot accepted: entries=%v err=%v", entries, err)
			}
		})
	}
	if _, err := parseSteamRecoveryProcessEntries(make([]byte, headerSize-1), 7); err == nil {
		t.Fatal("truncated snapshot accepted")
	}
}

func TestSteamRecoveryRefusesBusyOrUncertainLibraries(t *testing.T) {
	for _, test := range []struct {
		name, flags, want string
		missingInventory  bool
	}{
		{name: "running app", flags: "68", want: "busy"},
		{name: "download", flags: "1048580", want: "busy"},
		{name: "staging", flags: "2097156", want: "busy"},
		{name: "paused download", flags: "516", want: "busy"},
		{name: "unknown flags", flags: "invalid", want: "unknown"},
		{name: "missing inventory", flags: "4", missingInventory: true, want: "inventory unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if !test.missingInventory {
				inventory := fmt.Sprintf(`"libraryfolders" { "0" { "path" %s } }`, strconv.Quote(root))
				writeSteamFixture(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), []byte(inventory))
			}
			manifest := fmt.Sprintf(`"AppState" { "appid" "42" "StateFlags" "%s" }`, test.flags)
			writeSteamFixture(t, filepath.Join(root, "steamapps", "appmanifest_42.acf"), []byte(manifest))
			tracker := steamTracker{exe: filepath.Join(root, "Steam.exe")}
			// These checks must refuse recovery before any native process
			// scan or shutdown request. No real Steam client is used.
			if err := steamRecoveryLibraryIdle(&tracker); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unsafe recovery was not refused: err=%v", err)
			}
		})
	}
}
