package bluetooth

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIncompleteBluetoothListPreservesAvailableDevicesButCannotAuthorizeProfileRemoval(t *testing.T) {
	classic := []PairedDevice{{ID: "112233445566", Name: "Classic pad", Transport: "Classic"}}
	le := []PairedDevice{{ID: bleDeviceID("endpoint"), Name: "LE pad", Transport: "LE", profileAddress: "e417d834565b"}}
	for _, sample := range []struct {
		name              string
		classicErr, leErr error
		count             int
		complete          bool
	}{
		{"both backends", nil, nil, 2, true},
		{"LE failure", nil, errors.New("WinRT list failed"), 1, false},
		{"Classic failure", errors.New("Classic list failed"), nil, 1, false},
		{"both fail", errors.New("Classic list failed"), errors.New("WinRT list failed"), 0, false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			result := pairedDeviceList(classic, sample.classicErr, le, sample.leErr)
			if len(result.Devices) != sample.count || result.Complete != sample.complete {
				t.Fatalf("wrong display result: %+v", result)
			}
			_, err := result.verifiedDevices()
			if (err == nil) != sample.complete {
				t.Fatalf("incomplete list authorized profile removal: %+v err=%v", result, err)
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "list failed") {
				t.Fatal("backend detail leaked into UI payload")
			}
		})
	}
}

func TestPairedDeviceAddress(t *testing.T) {
	if got, err := pairedDeviceAddress("012345abcdef"); err != nil || got != 0x012345abcdef {
		t.Fatalf("address=%x error=%v", got, err)
	}
	for _, id := range []string{"", "000000000000", "11223344556677", "01:23:45:ab:cd:ef", "012345ABCDEf", "zzzzzzzzzzzz", "../device-id"} {
		if _, err := pairedDeviceAddress(id); err == nil {
			t.Errorf("invalid device ID accepted: %q", id)
		}
	}
}

func TestBLEDeviceIDsPinEndpointWithoutRequiringAddress(t *testing.T) {
	id := bleDeviceID(`BluetoothLE#local-remote`)
	if err := validatePairedDeviceID(id); err != nil {
		t.Fatal(err)
	}
	if bleDeviceID(`BluetoothLE#other-endpoint`) == id {
		t.Fatal("different endpoint reused a pairing ID")
	}
	if err := validatePairedDeviceID("012345abcdef"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"le:", id + "extra", strings.ToUpper(id), "le:" + strings.Repeat("z", 32)} {
		if err := validatePairedDeviceID(invalid); err == nil {
			t.Fatalf("invalid ID accepted: %q", invalid)
		}
	}
}

func TestOfflineBLEWithoutAddressStillListsAndHasStableEndpointID(t *testing.T) {
	offline, err := decodeBLEDevices([]byte(`{"devices":[{"deviceID":"BluetoothLE#2C","address":"","name":"8BitDo Ultimate 2C","connected":false}]}`))
	if err != nil || len(offline) != 1 {
		t.Fatalf("offline pairing broke listing: %+v %v", offline, err)
	}
	online, err := decodeBLEDevices([]byte(`{"devices":[{"deviceID":"BluetoothLE#2C","address":"e417d834565b","name":"8BitDo Ultimate 2C","connected":true}]}`))
	if err != nil || len(online) != 1 || online[0].ID != offline[0].ID {
		t.Fatalf("address availability changed selection identity: %+v %v", online, err)
	}
	classic := []PairedDevice{{ID: "112233445566", Name: "SN30 Pro", Transport: "Classic", Connected: true}}
	result := pairedDeviceList(classic, nil, offline, nil)
	if !result.Complete || len(result.Devices) != 2 {
		t.Fatalf("offline LE hid Classic: %+v", result)
	}
	if _, err := result.verifiedDevices(); err == nil {
		t.Fatal("missing address authorized saved-profile removal")
	}
	if offline[0].ControllerAddress() != "" || online[0].ControllerAddress() != "e417d834565b" {
		t.Fatal("invented/lost profile correlation address")
	}
}

func TestBLEListDecodeAndClassicMergePreserveSameNameDevices(t *testing.T) {
	le, err := decodeBLEDevices([]byte(`{"devices":[{"deviceID":"BluetoothLE#1","address":"e417d834565b","name":"8BitDo Ultimate 2C","connected":true},{"deviceID":"BluetoothLE#1","address":"e417d834565b","name":"8BitDo Ultimate 2C","connected":false},{"deviceID":"BluetoothLE#2","address":"112233445566","name":"8BitDo Ultimate 2C","connected":false}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(le) != 2 {
		t.Fatalf("duplicate endpoint not collapsed: %+v", le)
	}
	classic := PairedDevice{ID: "e417d834565b", Name: "8BitDo Ultimate 2C", Address: "E4:17:D8:34:56:5B", Transport: "Classic"}
	merged := mergePairedDevices([]PairedDevice{classic}, le)
	if len(merged) != 3 {
		t.Fatalf("same names or cross-protocol endpoints merged: %+v", merged)
	}
	found := false
	for _, device := range merged {
		if device.deviceID == "BluetoothLE#1" {
			found = true
			if !device.Connected || device.Transport != "LE" || device.Address != "E4:17:D8:34:56:5B" {
				t.Fatalf("BLE data lost: %+v", device)
			}
		}
	}
	if !found {
		t.Fatal("LE endpoint missing")
	}
	data, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "BluetoothLE#") {
		t.Fatal("private native endpoint leaked to UI")
	}
}

func TestBLEListRejectsIncompleteOrInvalidOutput(t *testing.T) {
	for _, invalid := range []string{`{}`, `{"devices":null}`, `{"devices":[]} garbage`, `{"devices":[{"deviceID":"x","address":"not-an-address"}]}`, `{"devices":[{"deviceID":"","address":"e417d834565b"}]}`, `{"devices":[],"unexpected":true}`} {
		if _, err := decodeBLEDevices([]byte(invalid)); err == nil {
			t.Fatalf("invalid list accepted: %s", invalid)
		}
	}
	if devices, err := decodeBLEDevices([]byte(`{"devices":[]}`)); err != nil || len(devices) != 0 {
		t.Fatalf("empty valid list rejected: %v %v", devices, err)
	}
}
