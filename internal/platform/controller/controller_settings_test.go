package controller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestControllerDefaultAndGameSwitchPolicy(t *testing.T) {
	for _, sample := range []struct {
		mode                   string
		nativeXbox, game, want bool
	}{
		{ControllerNative, false, false, false},
		{ControllerNative, false, true, true},
		{ControllerVirtual, false, false, true},
		{ControllerVirtual, false, true, true},
		{ControllerNative, true, false, false},
		{ControllerNative, true, true, false},
		{ControllerVirtual, true, true, false},
	} {
		if got := controllerUsesVirtual(ControllerInfo{Mode: sample.mode, NativeXInput: sample.nativeXbox}, sample.game); got != sample.want {
			t.Fatalf("mode=%s nativeXbox=%t game=%t got=%t want=%t", sample.mode, sample.nativeXbox, sample.game, got, sample.want)
		}
	}
}

func TestControllerPreferencesPreserveIndependentDevicesAndReconnect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets", "controllers.json")
	store, err := controllerPreferenceStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	sony := ControllerInfo{ID: controllerIdentity("sony-path", "sony-serial", 0x054c, 0x0ce6, "Bluetooth"), Name: "DualSense", Connection: "Bluetooth", Connected: true}
	generic := ControllerInfo{ID: controllerIdentity("generic-path", "", 0x2dc8, 0x3012, "Bluetooth"), Name: "Generic", Connection: "Bluetooth", Connected: true}
	if store.apply(sony).Known {
		t.Fatal("unknown controller already classified")
	}
	if err := store.save(sony, ControllerNative); err != nil {
		t.Fatal(err)
	}
	if err := store.save(generic, ControllerVirtual); err != nil {
		t.Fatal(err)
	}
	reloaded, err := controllerPreferenceStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual := reloaded.apply(sony); !actual.Known || actual.Mode != ControllerNative {
		t.Fatalf("native default lost: %+v", actual)
	}
	if actual := reloaded.apply(generic); !actual.Known || actual.Mode != ControllerVirtual {
		t.Fatalf("virtual default lost: %+v", actual)
	}
	list := reloaded.list([]controllerDevice{{info: sony}})
	if len(list) != 2 || !list[0].Connected || list[1].Connected || list[1].Mode != ControllerVirtual {
		t.Fatalf("disconnected preference lost: %+v", list)
	}
	if controllerIdentity("different-port", "sony-serial", 0x054c, 0x0ce6, "Bluetooth") != sony.ID {
		t.Fatal("serial identity changed with transient path")
	}
	if controllerIdentity("SONY-PATH", "sony-serial", 0x054c, 0x0ce6, "USB / other") == sony.ID {
		t.Fatal("different connection mode inherited a preference")
	}
}

func TestInvalidControllerPreferencesAreNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controllers.json")
	original := []byte(`{"version":1,"controllers":{"bad-id":{"name":"Pad","mode":"virtual"}}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := controllerPreferenceStoreAt(path); err == nil {
		t.Fatal("invalid settings accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatal("invalid settings overwritten")
	}
}
