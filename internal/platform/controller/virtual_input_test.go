package controller

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestVirtualReportPreservesXboxButtonsAndAnalogEndpoints(t *testing.T) {
	state := xinputGamepad{wButtons: XINPUT_GAMEPAD_START | XINPUT_GAMEPAD_BACK | XINPUT_GAMEPAD_A | XINPUT_GAMEPAD_DPAD_UP, bLeftTrigger: 255, bRightTrigger: 17, sThumbLX: -32768, sThumbLY: 32767, sThumbRX: 12500, sThumbRY: -6500}
	data := virtualReport(state)
	if len(data) != 12 || binary.LittleEndian.Uint16(data[:]) != state.wButtons || data[2] != 255 || data[3] != 17 {
		t.Fatalf("button/trigger payload corrupted: %v", data)
	}
	for i, want := range []int16{state.sThumbLX, state.sThumbLY, state.sThumbRX, state.sThumbRY} {
		if got := int16(binary.LittleEndian.Uint16(data[4+i*2:])); got != want {
			t.Fatalf("axis %d=%d, want %d", i, got, want)
		}
	}
}

func TestVirtualSessionFeedsAndStopsWithoutDriver(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	session := newVirtualInputSession(context.Background(), "physical-fixture")
	var closed atomic.Int32
	if err := session.attach(left, func() error { closed.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	defer session.close()
	// Arm neutral before feeding held input. The producer is the SDL owner.
	session.publish(xinputGamepad{})
	var frame [12]byte
	right.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.ReadFull(right, frame[:]); err != nil {
		t.Fatal(err)
	}
	session.publish(xinputGamepad{wButtons: XINPUT_GAMEPAD_A})
	for {
		if _, err := io.ReadFull(right, frame[:]); err != nil {
			t.Fatal(err)
		}
		if binary.LittleEndian.Uint16(frame[:]) == XINPUT_GAMEPAD_A {
			break
		}
	}
	if err := session.close(); err != nil {
		t.Fatal(err)
	}
	if err := session.close(); err != nil || closed.Load() != 1 {
		t.Fatalf("cleanup repeated: count=%d err=%v", closed.Load(), err)
	}
}

func TestVirtualSessionRejectsAttachAfterStop(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	session := newVirtualInputSession(context.Background(), "fixture")
	session.close()
	called := false
	if err := session.attach(left, func() error { called = true; return nil }); err == nil {
		t.Fatal("stopped session attached a helper")
	}
	if !called {
		t.Fatal("late helper not cleaned up")
	}
}

func TestVirtualSessionKeepsPlayersSeparateAndRearmsReconnectedSource(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	session := newVirtualInputSession(context.Background(), "player-one", "player-two")
	if err := session.attach(left, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	defer session.close()
	readUntil := func(first, second uint16) {
		t.Helper()
		right.SetReadDeadline(time.Now().Add(time.Second))
		var frame [24]byte
		for {
			if _, err := io.ReadFull(right, frame[:]); err != nil {
				t.Fatalf("waiting for independent player states %04x/%04x: %v", first, second, err)
			}
			if binary.LittleEndian.Uint16(frame[:]) == first && binary.LittleEndian.Uint16(frame[12:]) == second {
				return
			}
		}
	}
	first := controllerDevice{path: "player-one", instance: 1}
	second := controllerDevice{path: "player-two", instance: 2}
	session.publishDevices([]controllerDevice{first, second})
	readUntil(0, 0)
	first.state.wButtons, second.state.wButtons = XINPUT_GAMEPAD_A, XINPUT_GAMEPAD_B
	session.publishDevices([]controllerDevice{first, second})
	readUntil(XINPUT_GAMEPAD_A, XINPUT_GAMEPAD_B)
	session.publishDevices([]controllerDevice{second})
	readUntil(0, XINPUT_GAMEPAD_B)
	first.instance, first.state.wButtons = 3, XINPUT_GAMEPAD_X
	session.publishDevices([]controllerDevice{first, second})
	readUntil(0, XINPUT_GAMEPAD_B)
	first.state.wButtons = 0
	session.publishDevices([]controllerDevice{first, second})
	readUntil(0, XINPUT_GAMEPAD_B)
	first.state.wButtons = XINPUT_GAMEPAD_X
	session.publishDevices([]controllerDevice{first, second})
	readUntil(XINPUT_GAMEPAD_X, XINPUT_GAMEPAD_B)
}
