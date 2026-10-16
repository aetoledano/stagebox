package controller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSavedControllerRemovalRequiresDisconnectedAndUnpaired(t *testing.T) {
	for _, sample := range []struct {
		name    string
		info    ControllerInfo
		paired  []string
		allowed bool
	}{
		{"connected", ControllerInfo{Known: true, Connected: true}, nil, false},
		{"unknown", ControllerInfo{}, nil, false},
		{"paired but disconnected", ControllerInfo{Known: true, Connection: "Bluetooth", BluetoothAddress: "aabbccddeeff"}, []string{"aabbccddeeff"}, false},
		{"unpaired with another pairing", ControllerInfo{Known: true, Connection: "Bluetooth", BluetoothAddress: "aabbccddeeff"}, []string{"112233445566"}, true},
		{"legacy uncertain", ControllerInfo{Known: true, Connection: "Bluetooth"}, []string{"112233445566"}, false},
		{"legacy no remaining pairings", ControllerInfo{Known: true, Connection: "Bluetooth"}, nil, true},
		{"verified USB profile", ControllerInfo{Known: true, Connection: "USB"}, []string{"112233445566"}, true},
		{"legacy USB fallback could be Bluetooth", ControllerInfo{Known: true, Connection: "USB / other"}, []string{"112233445566"}, false},
		{"wireless unknown pairing", ControllerInfo{Known: true, Connection: "Wireless"}, []string{"112233445566"}, false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			if err := CheckSavedControllerRemoval(sample.info, sample.paired); (err == nil) != sample.allowed {
				t.Fatalf("allowed=%t err=%v", sample.allowed, err)
			}
		})
	}
}

func TestControllerBluetoothRemoteAddressOnly(t *testing.T) {
	for _, sample := range []struct{ id, want string }{
		{`BTHENUM\Dev_AABBCCDDEEFF\8&1234&0&BluetoothDevice_AABBCCDDEEFF`, "aabbccddeeff"},
		{`BTHLE\DEV_E417D834565B\7&357F956&0&E417D834565B`, "e417d834565b"},
		{`BTHLEDEVICE\service_DEV_E417D834565B\instance`, ""},
		{`HID\VID_2DC8&PID_3012\AABBCCDDEEFF`, ""},
		{`USB\Dev_AABBCCDDEEFF\adapter`, ""},
		{`BTHENUM\Dev_AABBCCDDEEFFextra\instance`, ""},
		{`BTHENUM\Dev_000000000000\instance`, ""},
		{`BTHENUM\Dev_GGBBCCDDEEFF\instance`, ""},
	} {
		if got := bluetoothAddressFromDeviceID(sample.id); got != sample.want {
			t.Fatalf("%q: got %q want %q", sample.id, got, sample.want)
		}
	}
}

func TestControllerBluetoothProfileUpgradeAndExactRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controllers.json")
	store, err := controllerPreferenceStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	first := ControllerInfo{ID: controllerIdentity("old-path", "", 0x2dc8, 0x3012, "Bluetooth"), Name: "Same model", Connection: "Bluetooth"}
	second := ControllerInfo{ID: controllerIdentity("other-path", "", 0x2dc8, 0x3012, "Bluetooth"), Name: "Same model", Connection: "Bluetooth", BluetoothAddress: "112233445566"}
	if err := store.save(first, ControllerVirtual); err != nil {
		t.Fatal(err)
	}
	if err := store.save(second, ControllerNative); err != nil {
		t.Fatal(err)
	}
	first.BluetoothAddress = "aabbccddeeff"
	unknown := ControllerInfo{ID: controllerIdentity("unsaved-path", "", 1, 2, "Bluetooth"), BluetoothAddress: "aabbccddeeff"}
	if err := store.rememberBluetooth([]controllerDevice{{info: first}, {info: unknown}}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := controllerPreferenceStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	list := reloaded.list(nil)
	if len(list) != 2 {
		t.Fatalf("upgrade created unknown profiles: %+v", list)
	}
	var matching []string
	for _, info := range list {
		if info.BluetoothAddress == first.BluetoothAddress {
			matching = append(matching, info.ID)
		}
	}
	if len(matching) != 1 || matching[0] != first.ID {
		t.Fatalf("wrong pairing association: %+v", list)
	}
	if err := reloaded.remove(matching); err != nil {
		t.Fatal(err)
	}
	reloaded, err = controllerPreferenceStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	list = reloaded.list(nil)
	if len(list) != 1 || list[0].ID != second.ID || list[0].Mode != ControllerNative || list[0].BluetoothAddress != second.BluetoothAddress {
		t.Fatalf("another same-name controller changed: %+v", list)
	}
}

func TestControllerProfileRemovalWriteFailurePreservesMemory(t *testing.T) {
	root := t.TempDir()
	store, err := controllerPreferenceStoreAt(filepath.Join(root, "controllers.json"))
	if err != nil {
		t.Fatal(err)
	}
	info := ControllerInfo{ID: controllerIdentity("pad", "", 1, 2, "Bluetooth"), Name: "Pad", Connection: "Bluetooth"}
	if err := store.save(info, ControllerVirtual); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	store.path = filepath.Join(blocker, "controllers.json")
	if err := store.remove([]string{info.ID}); err == nil {
		t.Fatal("removal ignored write failure")
	}
	if !store.apply(info).Known {
		t.Fatal("failed write removed in-memory profile")
	}
}
