//go:build windows

package controller

import (
	"runtime"
	"testing"
	"time"
)

// This validates the shipped DLL/Go ABI and event-loop lifecycle without
// launching Wails or injecting controller input. A physical pad is optional.
func TestSDL3RuntimeLoadPollAndClose(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("bundled SDL3 runtime is Windows amd64")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	backend, err := openSDLBackend()
	if err != nil {
		t.Fatalf("initialize bundled SDL3: %v", err)
	}
	defer backend.shutdown()
	for sample := 0; sample < 2; sample++ {
		_, id, count, err := backend.poll(time.Now())
		if err != nil {
			t.Fatalf("poll SDL3: %v", err)
		}
		if (id == 0) != (count == 0) {
			t.Fatalf("inventory and selected pad disagree: id=%d count=%d", id, count)
		}
		if id != 0 && backend.gamepads[id] == 0 {
			t.Fatalf("selected pad has no native handle: id=%d", id)
		}
	}
	// Exercise the real SDL quit/reinitialize/memory-IO path used by live
	// edits and deletions. SDL input remains private to this test process.
	for _, files := range [][]controllerMappingFile{
		{{path: "synthetic controller database", data: []byte(testControllerMapping)}},
		nil,
	} {
		if err := backend.rebuildMappings(files); err != nil {
			t.Fatalf("rebuild SDL3 mappings: %v", err)
		}
		if !backend.initialized {
			t.Fatal("SDL input not initialized after rebuild")
		}
		if _, _, _, err := backend.poll(time.Now()); err != nil {
			t.Fatalf("poll after mapping rebuild: %v", err)
		}
	}
}
