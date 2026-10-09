//go:build windows

package controller

import (
	"reflect"
	"testing"
)

func TestKeyboardVolumeIndependentOfNavigationPause(t *testing.T) {
	var actions []string
	bridge := &KeyboardBridge{volumeAction: func(action string) { actions = append(actions, action) }}
	bridge.Pause()
	for _, event := range []struct {
		key  uint32
		down bool
	}{
		{0xAF, true}, {0xAF, true}, {0xAF, false},
		{0xAE, true}, {0xAE, false},
		{0xAD, true}, {0xAD, true}, {0xAD, false}, {0xAD, true}, {0xAD, false},
	} {
		if !bridge.handleKeyEvent(event.key, event.down) {
			t.Fatal("multimedia keys must be consumed to prevent duplicate OS handling")
		}
	}
	want := []string{"media-up", "media-up", "media-down", "media-mute", "media-mute"}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("actions = %v, want %v", actions, want)
	}
}
