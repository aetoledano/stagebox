package controller

import "testing"

func TestLauncherInputGate(t *testing.T) {
	var gate launcherInputGate
	steps := []struct {
		name    string
		enabled bool
		neutral bool
		want    bool
	}{
		{"startup held", true, false, false},
		{"startup neutral", true, true, false},
		{"fresh press", true, false, true},
		{"background held", false, false, false},
		{"background neutral", false, true, false},
		{"return held", true, false, false},
		{"return neutral", true, true, false},
		{"new press", true, false, true},
		{"normal repeat", true, false, true},
	}
	for _, step := range steps {
		if got := gate.allow(step.enabled, step.neutral); got != step.want {
			t.Errorf("%s: allow = %v, want %v", step.name, got, step.want)
		}
	}
	gate.reset()
	if gate.allow(true, false) {
		t.Error("mode reset must require neutral input again")
	}
}

func TestLauncherKeyState(t *testing.T) {
	const enter = 0x0D
	var keys launcherKeyState
	steps := []struct {
		name        string
		down        bool
		enabled     bool
		wantEmit    bool
		wantConsume bool
	}{
		{"background keydown", true, false, false, false},
		{"foreground held repeat", true, true, false, true},
		{"release", false, true, false, false},
		{"fresh keydown", true, true, true, true},
		{"normal repeat", true, true, true, true},
		{"background repeat", true, false, false, false},
		{"return repeat", true, true, false, true},
		{"background release", false, false, false, false},
		{"fresh foreground keydown", true, true, true, true},
	}
	for _, step := range steps {
		emit, consume := keys.update(enter, step.down, step.enabled)
		if emit != step.wantEmit || consume != step.wantConsume {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", step.name, emit, consume, step.wantEmit, step.wantConsume)
		}
	}
	keys.reset()
	if emit, consume := keys.update(enter, true, true); emit || !consume {
		t.Error("pause/resume reset must suppress a held key until release")
	}
	keys.update(enter, false, true)
	if emit, consume := keys.update(enter, true, true); !emit || !consume {
		t.Error("fresh key after release must be accepted")
	}
	if emit, consume := keys.update(256, true, true); emit || consume {
		t.Error("out-of-range keys must pass through")
	}
}
