//go:build windows

package games

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Helpers run only when explicitly launched by the integration test below.
func TestJobTrackingHelper(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-3] != "--stagebox-job-helper" {
		return
	}
	role, path := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	if role == "child" {
		if err := os.WriteFile(path+".tmp", []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Second)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestJobTrackingHelper$", "--", "--stagebox-job-helper", "child", path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Process.Release()
}

func TestJobTracksChildAfterRootExit(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "child.pid")
	process, err := startAdvancedGameProcess(&GameData{
		Executable: exe, Name: "Job tracking test", AdvancedTracking: true,
		Args: []string{"-test.run=^TestJobTrackingHelper$", "--", "--stagebox-job-helper", "root", path},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := process.(*jobGameProcess)
	t.Cleanup(func() { _ = job.Kill(); job.Close() })
	if state, err := windows.WaitForSingleObject(job.root, 10000); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("root did not exit: state=%d err=%v", state, err)
	}
	var childPID int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			childPID, err = strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("child did not report its PID")
	}
	pids, err := jobProcessIDs(job.job)
	if err != nil {
		t.Fatal(err)
	}
	if len(pids) != 1 || pids[0] != uint32(childPID) {
		t.Fatalf("job lost its child after root exit: pids=%v child=%d", pids, childPID)
	}
	if !launchLockActive(lockState{valid: true, meta: launchLockMetadata{PID: job.PID(), JobName: job.JobName()}}) {
		t.Fatal("launch lock must remain active while the child is running")
	}
	if err := job.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pids, err = jobProcessIDs(job.job)
		if err != nil {
			t.Fatal(err)
		}
		if len(pids) == 0 {
			if advancedJobActive(job.JobName()) {
				t.Fatal("empty job must not hold the launch lock")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job termination did not stop descendants: %v", pids)
}
