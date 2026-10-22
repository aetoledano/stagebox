package controller

import (
	"testing"
	"time"
)

func TestMappingShortcutRequiresNeutralThenStablePhysicalHold(t *testing.T) {
	s := controllerMappingShortcut{button: -1}
	now := time.Unix(100, 0)
	for _, delta := range []time.Duration{0, 2 * time.Second} {
		if fire, _ := s.update([]bool{true}, now.Add(delta)); fire {
			t.Fatal("button held before settings armed triggered remapping")
		}
	}
	s.update([]bool{false}, now.Add(3*time.Second))
	s.update([]bool{true}, now.Add(4*time.Second))
	if fire, _ := s.update([]bool{true}, now.Add(5400*time.Millisecond)); fire {
		t.Fatal("triggered too early")
	}
	if fire, released := s.update([]bool{true}, now.Add(5500*time.Millisecond)); !fire || released {
		t.Fatal("physical hold did not trigger exactly once")
	}
	if fire, released := s.update([]bool{false}, now.Add(6*time.Second)); fire || released {
		t.Fatal("long hold became a short menu tap")
	}
}

func TestMappingShortcutShortTapAndButtonSwitch(t *testing.T) {
	s := controllerMappingShortcut{button: -1}
	now := time.Unix(100, 0)
	s.update([]bool{false, false}, now)
	s.update([]bool{true, false}, now.Add(time.Second))
	if fire, released := s.update([]bool{false, false}, now.Add(1100*time.Millisecond)); fire || !released {
		t.Fatal("short tap not released normally")
	}
	s.update([]bool{true, false}, now.Add(2*time.Second))
	s.update([]bool{false, true}, now.Add(3*time.Second))
	if fire, _ := s.update([]bool{false, true}, now.Add(3600*time.Millisecond)); fire {
		t.Fatal("hold time merged different raw buttons")
	}
}
