//go:build windows

package controller

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestControllerRumbleRejectsDisconnectedOrCancelledTarget(t *testing.T) {
	bridge := &ControllerBridge{ctx: context.Background()}
	if err := bridge.TestControllerRumble("pad"); err == nil {
		t.Fatal("rumble accepted for a disconnected controller")
	}
	old := &controllerTestRequest{id: "pad"}
	bridge.rumbleTarget.Store(old)
	request := &controllerRumbleRequest{target: old, ctx: context.Background(), result: make(chan error, 1)}
	bridge.testRumble.Store(request)
	if err := bridge.TestControllerRumble(""); err != nil {
		t.Fatal(err)
	}
	bridge.pollTestRumble(nil, time.Now())
	if err := <-request.result; err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("request after cancellation = %v", err)
	}
}

func TestControllerRumbleRequestWithoutSDL(t *testing.T) {
	test := &controllerTestRequest{id: "pad"}
	bridge := &ControllerBridge{ctx: context.Background()}
	bridge.rumbleTarget.Store(test)
	request := &controllerRumbleRequest{target: test, ctx: context.Background(), result: make(chan error, 1)}
	bridge.testRumble.Store(request)
	bridge.pollTestRumble(nil, time.Now())
	if err := <-request.result; err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("rumble without SDL = %v", err)
	}
	if bridge.testRumble.Load() != nil {
		t.Fatal("pending request not consumed")
	}
}
