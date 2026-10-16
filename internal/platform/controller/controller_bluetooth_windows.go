//go:build windows

package controller

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	controllerCfgMgr            = windows.NewLazySystemDLL("cfgmgr32.dll")
	controllerInterfaceProperty = controllerCfgMgr.NewProc("CM_Get_Device_Interface_PropertyW")
	controllerLocateNode        = controllerCfgMgr.NewProc("CM_Locate_DevNodeW")
	controllerParentNode        = controllerCfgMgr.NewProc("CM_Get_Parent")
	controllerNodeID            = controllerCfgMgr.NewProc("CM_Get_Device_IDW")
)

// Read-only association from the HID interface to its exact Classic/LE Bluetooth
// remote-device ancestor. Names, model IDs and USB adapter pairings are never
// used to guess which Windows pairing owns a controller.
func controllerWindowsTransport(path string) (connection, address, reason string) {
	if !strings.HasPrefix(strings.ToLower(path), `\\?\`) {
		return "", "", "no Windows interface path"
	}
	for _, proc := range []*windows.LazyProc{controllerInterfaceProperty, controllerLocateNode, controllerParentNode, controllerNodeID} {
		if proc.Find() != nil {
			return "", "", "Configuration Manager API unavailable"
		}
	}
	iface, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", "", "invalid interface path"
	}
	// DEVPKEY_Device_InstanceId, DEVPROP_TYPE_STRING (WinSDK devpkey.h).
	key := windows.DEVPROPKEY{FmtID: windows.DEVPROPGUID{Data1: 0x78c34fc8, Data2: 0x104a, Data3: 0x4aca, Data4: [8]byte{0x9e, 0xa4, 0x52, 0x4d, 0x52, 0x99, 0x6e, 0x57}}, PID: 256}
	var buffer [1024]uint16
	size, kind := uint32(len(buffer)*2), uint32(0)
	result, _, _ := controllerInterfaceProperty.Call(uintptr(unsafe.Pointer(iface)), uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0)
	if result != 0 || kind != 0x12 || size < 2 || size > uint32(len(buffer)*2) {
		return "", "", fmt.Sprintf("interface instance property CR=%#x type=%#x bytes=%d", result, kind, size)
	}
	var node uint32
	result, _, _ = controllerLocateNode.Call(uintptr(unsafe.Pointer(&node)), uintptr(unsafe.Pointer(&buffer[0])), 0)
	if result != 0 {
		return "", "", fmt.Sprintf("locate interface devnode CR=%#x", result)
	}
	var deviceIDs []string
	for depth := 0; depth < 16; depth++ {
		result, _, _ = controllerNodeID.Call(uintptr(node), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0)
		if result != 0 {
			reason = fmt.Sprintf("device ID CR=%#x", result)
			break
		}
		deviceID := windows.UTF16ToString(buffer[:])
		deviceIDs = append(deviceIDs, deviceID)
		if bluetoothAddressFromDeviceID(deviceID) != "" {
			break
		}
		var parent uint32
		result, _, _ = controllerParentNode.Call(uintptr(unsafe.Pointer(&parent)), uintptr(node), 0)
		if result != 0 || parent == node {
			break
		}
		node = parent
	}
	connection, address = controllerTransportFromDeviceIDs(deviceIDs)
	if reason == "" {
		reason = strings.Join(deviceIDs, " <- ")
	}
	return connection, address, reason
}
