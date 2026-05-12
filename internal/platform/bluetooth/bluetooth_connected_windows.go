//go:build windows

package bluetooth

import (
	"strings"
	"unsafe"

	lgx "launcher/internal/logx"

	"golang.org/x/sys/windows"
)

type BLUETOOTH_FIND_RADIO_PARAMS struct{ DwSize uint32 }

type BLUETOOTH_DEVICE_SEARCH_PARAMS struct {
	DwSize               uint32
	FReturnAuthenticated int32
	FReturnRemembered    int32
	FReturnUnknown       int32
	FReturnConnected     int32
	FIssueInquiry        int32
	CTimeoutMultiplier   byte
	_                    [3]byte
	HRadio               windows.Handle
}

type SYSTEMTIME struct {
	Year, Month, DayOfWeek, Day        uint16
	Hour, Minute, Second, Milliseconds uint16
}

type BLUETOOTH_ADDRESS struct{ Addr uint64 }

type BLUETOOTH_DEVICE_INFO struct {
	DwSize          uint32
	Address         BLUETOOTH_ADDRESS
	UlClassofDevice uint32
	FConnected      int32
	FRemembered     int32
	FAuthenticated  int32
	StLastSeen      SYSTEMTIME
	StLastUsed      SYSTEMTIME
	SzName          [248]uint16
}

func AnyConnectedDevice() bool {
	return ConnectedControllerCount() > 0
}

func ConnectedControllerCount() int {
	seen := map[string]struct{}{}

	if !initBluetoothProcs() {
		return 0
	}
	radios := enumerateRadios()
	defer closeRadioHandles(radios)

	for _, hRadio := range radios {
		radioConnectedControllers(hRadio, seen)
	}
	return len(seen)
}

func closeRadioHandles(radios []windows.Handle) {
	for _, h := range radios {
		_ = windows.CloseHandle(h)
	}
}

func radioConnectedControllers(hRadio windows.Handle, seen map[string]struct{}) {
	var sp BLUETOOTH_DEVICE_SEARCH_PARAMS
	sp.DwSize = uint32(unsafe.Sizeof(sp))
	sp.FReturnAuthenticated = 1
	sp.FReturnRemembered = 1
	sp.FReturnUnknown = 0
	sp.FReturnConnected = 1
	sp.FIssueInquiry = 0
	sp.CTimeoutMultiplier = 1
	sp.HRadio = hRadio

	var di BLUETOOTH_DEVICE_INFO
	di.DwSize = uint32(unsafe.Sizeof(di))

	hFind, _, _ := procFindFirstDevice.Call(uintptr(unsafe.Pointer(&sp)), uintptr(unsafe.Pointer(&di)))
	if hFind == 0 {
		return
	}
	hFindDev := windows.Handle(hFind)
	defer procFindDeviceClose.Call(uintptr(hFindDev))

	for {
		if di.FConnected != 0 && isControllerDevice(di) {
			key := bluetoothAddressKey(di.Address.Addr)
			seen[key] = struct{}{}
		}
		di.DwSize = uint32(unsafe.Sizeof(di))
		rn, _, _ := procFindNextDevice.Call(uintptr(hFindDev), uintptr(unsafe.Pointer(&di)))
		if rn == 0 {
			break
		}
	}
}

func isControllerDevice(di BLUETOOTH_DEVICE_INFO) bool {
	name := strings.ToLower(windows.UTF16ToString(di.SzName[:]))
	if isDeniedControllerText(name) {
		return false
	}
	if isAllowedControllerText(name) {
		return true
	}

	major := (di.UlClassofDevice >> 8) & 0x1f
	minorPeripheral := (di.UlClassofDevice >> 2) & 0x3f
	return major == 0x05 && (minorPeripheral == 0x01 || minorPeripheral == 0x02)
}

func pnpConnectedControllerCount(seen map[string]struct{}) int {
	for _, enumerator := range []string{"BTHENUM", "BTHLEDEVICE"} {
		devs, err := windows.SetupDiGetClassDevsEx(nil, enumerator, 0, windows.DIGCF_PRESENT|windows.DIGCF_ALLCLASSES, 0, "")
		if err != nil {
			continue
		}

		for i := 0; ; i++ {
			dev, err := windows.SetupDiEnumDeviceInfo(devs, i)
			if err != nil {
				break
			}
			if !pnpDeviceStarted(dev) {
				continue
			}

			instanceID, _ := windows.SetupDiGetDeviceInstanceId(devs, dev)
			text := strings.ToLower(instanceID)
			for _, prop := range []windows.SPDRP{
				windows.SPDRP_FRIENDLYNAME,
				windows.SPDRP_DEVICEDESC,
				windows.SPDRP_HARDWAREID,
				windows.SPDRP_COMPATIBLEIDS,
				windows.SPDRP_CLASS,
				windows.SPDRP_ENUMERATOR_NAME,
			} {
				text += " " + registryPropertyText(devs, dev, prop)
			}

			if isDeniedControllerText(text) || !isAllowedControllerText(text) {
				continue
			}
			if instanceID == "" {
				instanceID = text
			}
			seen["pnp:"+strings.ToLower(instanceID)] = struct{}{}
		}

		_ = windows.SetupDiDestroyDeviceInfoList(devs)
	}

	return len(seen)
}

func pnpDeviceStarted(dev *windows.DevInfoData) bool {
	var status uint32
	var problem uint32
	if err := windows.CM_Get_DevNode_Status(&status, &problem, dev.DevInst, 0); err != nil {
		return false
	}
	return status&windows.DN_STARTED != 0 && status&windows.DN_HAS_PROBLEM == 0
}

func registryPropertyText(devs windows.DevInfo, dev *windows.DevInfoData, prop windows.SPDRP) string {
	value, err := windows.SetupDiGetDeviceRegistryProperty(devs, dev, prop)
	if err != nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.ToLower(v)
	case []string:
		return strings.ToLower(strings.Join(v, " "))
	default:
		return ""
	}
}

func isDeniedControllerText(text string) bool {
	for _, denied := range []string{
		"adapter",
		"[tv]",
		" tv",
		"television",
		"samsung",
		"headphone",
		"headset",
		"speaker",
		"audio",
		"keyboard",
		"mouse",
	} {
		if strings.Contains(text, denied) {
			return true
		}
	}
	return false
}

func isAllowedControllerText(text string) bool {
	for _, allowed := range []string{
		"8bitdo",
		"xbox",
		"dualshock",
		"dualsense",
		"gamepad",
		"controller",
		"game controller",
		"joystick",
		"pro controller",
		"wireless controller",
		"hid_device_system_game",
	} {
		if strings.Contains(text, allowed) {
			return true
		}
	}
	return false
}

func bluetoothAddressKey(addr uint64) string {
	return "bt:" + formatBluetoothAddress(addr)
}

func formatBluetoothAddress(addr uint64) string {
	const hex = "0123456789abcdef"
	var out [12]byte
	for i := 11; i >= 0; i-- {
		out[i] = hex[addr&0x0f]
		addr >>= 4
	}
	return string(out[:])
}

func enumerateRadios() []windows.Handle {
	found := make([]windows.Handle, 0, 2)

	var frp BLUETOOTH_FIND_RADIO_PARAMS
	frp.DwSize = uint32(unsafe.Sizeof(frp))

	var hRadio windows.Handle
	r0, _, _ := procFindFirstRadio.Call(uintptr(unsafe.Pointer(&frp)), uintptr(unsafe.Pointer(&hRadio)))
	if r0 == 0 {
		lgx.Debug("[bt] connected-device scan found no radios")
		return found
	}
	hFind := windows.Handle(r0)
	defer procFindRadioClose.Call(uintptr(hFind))

	if hRadio != 0 {
		found = append(found, hRadio)
	}

	for {
		var next windows.Handle
		rn, _, _ := procFindNextRadio.Call(uintptr(hFind), uintptr(unsafe.Pointer(&next)))
		if rn == 0 {
			break
		}
		if next != 0 {
			found = append(found, next)
		}
	}
	return found
}
