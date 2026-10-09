//go:build windows && cgo

package audio

/*
#cgo CXXFLAGS: -std=c++11
#cgo LDFLAGS: -static -lole32 -luuid -lstdc++ -lwinpthread
#include <stdlib.h>
#include "audio_windows.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unsafe"
)

func nativeError(operation string, code C.uint32_t) error {
	if int32(code) >= 0 {
		return nil
	}
	return fmt.Errorf("%s failed (Windows audio HRESULT 0x%08X)", operation, uint32(code))
}

func GetSettings() (Settings, error) {
	settings := Settings{Devices: []Device{}}
	var devices *C.sb_audio_device
	var count C.uint
	code := C.sb_audio_list(&devices, &count)
	if err := nativeError("list sound outputs", code); err != nil {
		return settings, err
	}
	defer C.sb_audio_free_devices(devices, count)
	for _, device := range unsafe.Slice(devices, int(count)) {
		settings.Devices = append(settings.Devices, Device{ID: C.GoString(device.id), Name: C.GoString(device.name), Current: device.current != 0})
	}
	sort.Slice(settings.Devices, func(i, j int) bool {
		return strings.ToLower(settings.Devices[i].Name) < strings.ToLower(settings.Devices[j].Name)
	})
	status, err := CurrentStatus()
	settings.Status = status
	// Use the newest default lookup if the device changed during enumeration.
	for i := range settings.Devices {
		settings.Devices[i].Current = settings.Status.Available && settings.Devices[i].ID == settings.Status.DeviceID
	}
	return settings, err
}

func CurrentStatus() (Status, error) { return volumeOperation(0, 0) }

func SetOutput(id string) error {
	if id == "" || strings.ContainsRune(id, '\x00') {
		return errors.New("invalid sound output ID")
	}
	value := C.CString(id)
	defer C.free(unsafe.Pointer(value))
	return nativeError("select default sound output", C.sb_audio_set_output(value))
}

func ChangeVolume(direction int) (Status, error) {
	if direction != -1 && direction != 1 {
		return Status{}, errors.New("volume direction must be -1 or 1")
	}
	return volumeOperation(1, direction*VolumeStep)
}

func SetVolume(level int) (Status, error) {
	if level < 0 || level > 100 {
		return Status{}, errors.New("volume must be between 0 and 100")
	}
	return volumeOperation(2, level)
}

func ToggleMute() (Status, error) { return volumeOperation(3, 0) }

func volumeOperation(operation, value int) (Status, error) {
	var raw C.sb_audio_status
	code := C.sb_audio_volume(C.int(operation), C.int(value), &raw)
	defer C.sb_audio_free_status(&raw)
	if err := nativeError("read/change sound volume", code); err != nil {
		return Status{}, err
	}
	return Status{Available: raw.available != 0, DeviceID: C.GoString(raw.id), DeviceName: C.GoString(raw.name), Volume: int(raw.volume), Muted: raw.muted != 0}, nil
}
