package controller

import (
	"testing"
	"time"
)

func TestVolumeKeyAction(t *testing.T) {
	for _, tc := range []struct {
		vk   uint32
		want string
	}{{0xAF, "up"}, {0xAE, "down"}, {0xAD, "mute"}, {0x0D, ""}} {
		if got := volumeKeyAction(tc.vk); got != tc.want {
			t.Errorf("key 0x%X: got %q, want %q", tc.vk, got, tc.want)
		}
	}
}

func TestVolumeStick(t *testing.T) {
	start := time.Unix(1000, 0)
	var stick volumeStickState
	steps := []struct {
		name    string
		elapsed time.Duration
		x, y    int
		enabled bool
		want    string
	}{
		{"held at startup", 0, 0, 24000, true, ""},
		{"released at startup", time.Millisecond, 0, 0, true, ""},
		{"fresh up", 2 * time.Millisecond, 0, 24000, true, "up"},
		{"wait before repeat", 300 * time.Millisecond, 0, 24000, true, ""},
		{"first repeat", 352 * time.Millisecond, 0, 24000, true, "up"},
		{"repeat cadence", 472 * time.Millisecond, 0, 24000, true, "up"},
		{"reverse direction", 480 * time.Millisecond, 0, -24000, true, "down"},
		{"horizontal", 490 * time.Millisecond, 24000, 15000, true, ""},
		{"background", time.Second, 0, 24000, false, ""},
		{"held on return", 2 * time.Second, 0, 24000, true, ""},
		{"release on return", 3 * time.Second, 0, 0, true, ""},
		{"fresh down", 4 * time.Second, 0, -24000, true, "down"},
	}
	for _, step := range steps {
		if got := stick.update(start.Add(step.elapsed), step.x, step.y, step.enabled); got != step.want {
			t.Errorf("%s: got %q, want %q", step.name, got, step.want)
		}
	}
	stick.reset()
	if action := stick.update(start.Add(5*time.Second), 0, 24000, true); action != "" {
		t.Fatal("mode changes must require a neutral right stick")
	}
}
