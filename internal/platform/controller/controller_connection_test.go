package controller

import "testing"

func TestBluetoothIndicatorUsesLiveTransportRatherThanNameOrSavedPairing(t *testing.T) {
	for _, sample := range []struct {
		name    string
		devices []controllerDevice
		want    bool
	}{
		{"none", nil, false},
		{"Ultimate 2C LE", []controllerDevice{{info: ControllerInfo{Name: "8BitDo Ultimate 2C", Connection: "Bluetooth", Connected: true}}}, true},
		{"Classic pad", []controllerDevice{{info: ControllerInfo{Connection: "Bluetooth", Connected: true}}}, true},
		{"paired but disconnected", []controllerDevice{{info: ControllerInfo{Connection: "Bluetooth", Known: true, BluetoothAddress: "e417d834565b"}}}, false},
		{"same model USB receiver", []controllerDevice{{info: ControllerInfo{Name: "8BitDo Ultimate 2C", Connection: "USB / wireless receiver", Connected: true}}}, false},
		{"wireless transport unverified", []controllerDevice{{info: ControllerInfo{Connection: "Wireless", Connected: true}}}, false},
		{"mixed USB and BLE", []controllerDevice{{info: ControllerInfo{Connection: "USB", Connected: true}}, {info: ControllerInfo{Connection: "Bluetooth", Connected: true}}}, true},
		{"only USB remains", []controllerDevice{{info: ControllerInfo{Connection: "USB", Connected: true}}, {info: ControllerInfo{Connection: "Bluetooth", Connected: false}}}, false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			if got := hasConnectedBluetoothController(sample.devices); got != sample.want {
				t.Fatalf("got %t want %t", got, sample.want)
			}
		})
	}
}

func TestControllerTransportUsesAncestryRatherThanModel(t *testing.T) {
	for _, sample := range []struct {
		ids                 []string
		connection, address string
	}{
		{[]string{`HID\VID_2DC8&PID_3012\instance`, `BTHENUM\service\instance`, `BTHENUM\Dev_AABBCCDDEEFF\instance`}, "Bluetooth", "aabbccddeeff"},
		{[]string{`HID\VID_2DC8&PID_3012\instance`, `BTHLEDEVICE\service\instance`, `USB\radio\instance`}, "Bluetooth", ""},
		{[]string{`HID\VID_2DC8&PID_301B\instance`, `BTHLEDEVICE\service\instance`, `BTHLE\DEV_E417D834565B\7&357F956&0&E417D834565B`, `USB\radio\instance`}, "Bluetooth", "e417d834565b"},
		{[]string{`HID\VID_2DC8&PID_3012\instance`, `USB\VID_2DC8&PID_3012\receiver`}, "USB", ""},
		{[]string{`HID\VID_2DC8&PID_3012\instance`}, "", ""},
	} {
		connection, address := controllerTransportFromDeviceIDs(sample.ids)
		if connection != sample.connection || address != sample.address {
			t.Fatalf("%v: got %q/%q", sample.ids, connection, address)
		}
	}
}

func TestControllerConnectionLabelNeverAssumesUSBFromUnknownPath(t *testing.T) {
	for _, sample := range []struct {
		path, transport string
		state           int32
		want            string
	}{
		{`\\?\HID#VID_2DC8&PID_3012#instance`, "Bluetooth", 1, "Bluetooth"},
		{`\\?\HID#VID_2DC8&PID_3012#instance`, "", 0, "Unknown"},
		{"XInput#0", "", 2, "Wireless"},
		{"XInput#0", "", 1, "Wired"},
		{"", "", -1, "Unknown"},
		{"receiver", "USB", 2, "USB / wireless receiver"},
		{"receiver", "USB", 0, "USB"},
		{`\\?\HID#{00001124-0000-1000-8000-00805F9B34FB}_VID&00022DC8_PID&3012#instance`, "", 0, "Bluetooth"},
	} {
		if got := controllerConnectionLabel(sample.path, sample.transport, sample.state); got != sample.want {
			t.Fatalf("%+v: got %q", sample, got)
		}
	}
}
