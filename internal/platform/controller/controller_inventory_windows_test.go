//go:build windows

package controller

import "testing"

func TestMissingControllerReleasesGrabWithoutCapturingReplacement(t *testing.T) {
	bridge := &ControllerBridge{}
	grab := &controllerTestRequest{id: "tested"}
	bridge.testing.Store(grab)
	bridge.rumbleTarget.Store(grab)
	devices := []controllerDevice{{info: ControllerInfo{ID: "replacement", Connected: true}}}
	if got := bridge.releaseMissingTest(devices); got != grab {
		t.Fatalf("released = %v, want exact missing grab", got)
	}
	if bridge.IsTesting() || !bridge.resetInput.Load() || bridge.rumbleTarget.Load() != nil {
		t.Fatal("missing source retained input/rumble or failed to rearm")
	}
	devices = append(devices, controllerDevice{info: ControllerInfo{ID: "tested", Connected: true}})
	if bridge.releaseMissingTest(devices) != nil || bridge.IsTesting() {
		t.Fatal("reconnection silently captured input")
	}
}

func TestPresentTestControllerRetainsGrab(t *testing.T) {
	bridge := &ControllerBridge{}
	grab := &controllerTestRequest{id: "tested"}
	bridge.testing.Store(grab)
	if bridge.releaseMissingTest([]controllerDevice{{info: ControllerInfo{ID: "tested", Connected: true}}}) != nil || bridge.testing.Load() != grab {
		t.Fatal("connected test source lost its grab")
	}
	if bridge.releaseMissingTest([]controllerDevice{{info: ControllerInfo{ID: "tested", Connected: false}}}) != grab {
		t.Fatal("disconnected source retained its grab")
	}
}
