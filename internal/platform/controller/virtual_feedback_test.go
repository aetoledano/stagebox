package controller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestVirtualFeedbackRetainsPrefetchedFramesBeyondSetupLimit(t *testing.T) {
	frame := []byte{0xff, 0xff, 0, 0}
	const frames = 10000 // Runtime traffic must not inherit the 32 KiB setup bound.
	data := append([]byte("ready\n"), bytes.Repeat(frame, frames)...)
	conn := &virtualFeedbackConnection{reader: bufio.NewReaderSize(bytes.NewReader(data), 32768)}
	if line, err := conn.reader.ReadSlice('\n'); err != nil || string(line) != "ready\n" {
		t.Fatalf("setup framing failed: %q %v", line, err)
	}
	var received [4]byte
	for range frames {
		if _, err := io.ReadFull(conn, received[:]); err != nil || !bytes.Equal(received[:], frame) {
			t.Fatalf("runtime feedback was lost/truncated: %v %v", received, err)
		}
	}
}

func TestVirtualRumblePreservesIndependentMotorRanges(t *testing.T) {
	data := []byte{0xff, 0xff, 0, 0, 0x34, 0x12, 0xcd, 0xab}
	levels, err := decodeVirtualRumble(data, 2)
	if err != nil || levels[0] != (virtualRumble{65535, 0}) || levels[1] != (virtualRumble{0x1234, 0xabcd}) {
		t.Fatalf("players or motor ranges changed: %+v %v", levels, err)
	}
	for _, sample := range []struct{ bytes, controllers int }{{0, 0}, {20, 5}, {7, 2}, {9, 2}} {
		if _, err := decodeVirtualRumble(make([]byte, sample.bytes), sample.controllers); err == nil {
			t.Fatalf("accepted invalid feedback batch: %+v", sample)
		}
	}
}

func TestVirtualRumbleStopsOnStaleFeedbackOrSessionEnd(t *testing.T) {
	now := time.Now()
	session := newVirtualInputSession(context.Background(), "physical")
	defer session.close()
	session.active.Store(true)
	session.feedback.Store(&virtualRumbleSnapshot{levels: []virtualRumble{{65535, 128}}, at: now})
	if got := session.rumbleLevels(now); len(got) != 1 || got[0].low != 65535 {
		t.Fatal("fresh feedback was not routed")
	}
	if session.rumbleLevels(now.Add(virtualRumbleFreshness+time.Nanosecond)) != nil {
		t.Fatal("stale feedback retained rumble")
	}
	session.cancel()
	if session.rumbleLevels(now) != nil || (*virtualInputSession)(nil).rumbleLevels(now) != nil {
		t.Fatal("ended/absent session retained rumble")
	}
}

func TestVirtualSessionReceivesFragmentedFeedbackWhileFeedingInput(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	session := newVirtualInputSession(context.Background(), "one", "two")
	if err := session.attach(left, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	defer session.close()
	go io.Copy(io.Discard, right) // Consumer drains independent input direction.
	feedback := []byte{0xff, 0xff, 0, 0, 0, 0, 0x34, 0x12}
	right.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := right.Write(feedback[:3]); err != nil {
		t.Fatal(err)
	}
	if _, err := right.Write(feedback[3:]); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for session.feedback.Load() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	levels := session.rumbleLevels(time.Now())
	if len(levels) != 2 || levels[0] != (virtualRumble{65535, 0}) || levels[1] != (virtualRumble{0, 0x1234}) {
		t.Fatalf("fragmented/duplex feedback corrupted: %+v", levels)
	}
}

func TestSDLTriggerRangeReachesEveryXboxLevelWithoutThresholding(t *testing.T) {
	seen := [256]bool{}
	previous := byte(0)
	for value := 0; value <= 32767; value++ {
		state := normalizedSDLGamepad(0, [6]int16{0, 0, 0, 0, int16(value), int16(32767 - value)})
		data := virtualReport(state)
		if data[2] < previous || data[2] != state.bLeftTrigger || data[3] != state.bRightTrigger || binary.LittleEndian.Uint16(data[:]) != 0 {
			t.Fatalf("analog triggers became thresholded/corrupted at SDL value %d", value)
		}
		seen[data[2]], previous = true, data[2]
	}
	for value, present := range seen {
		if !present {
			t.Fatalf("Xbox trigger value %d is unreachable", value)
		}
	}
}
