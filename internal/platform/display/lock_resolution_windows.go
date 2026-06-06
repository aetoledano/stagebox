//go:build windows

package display

import (
	"fmt"
	"launcher/internal/platform/winapi"
	"unsafe"
)

type devmode struct {
	DeviceName         [32]uint16
	SpecVersion        uint16
	DriverVersion      uint16
	Size               uint16
	DriverExtra        uint16
	Fields             uint32
	PositionX          int32
	PositionY          int32
	DisplayOrientation uint32
	DisplayFixedOutput uint32
	Color              uint16
	Duplex             uint16
	YResolution        uint16
	TTOption           uint16
	Collate            uint16
	FormName           [32]uint16
	LogPixels          uint16
	BitsPerPel         uint32
	PelsWidth          uint32
	PelsHeight         uint32
	DisplayFlags       uint32
	DisplayFrequency   uint32
	ICMMethod          uint32
	ICMIntent          uint32
	MediaType          uint32
	DitherType         uint32
	Reserved1          uint32
	Reserved2          uint32
	PanningWidth       uint32
	PanningHeight      uint32
}

type displayDeviceW struct {
	Cb           uint32
	DeviceName   [32]uint16  // e.g. "\\.\DISPLAY1"
	DeviceString [128]uint16 // adapter name
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

const (
	ENUM_CURRENT_SETTINGS  = ^uintptr(0) // -1
	EDS_ROTATEDMODE        = 0x00000004
	DM_PELSWIDTH           = 0x00080000
	DM_PELSHEIGHT          = 0x00100000
	DM_DISPLAYFREQUENCY    = 0x00400000
	CDS_UPDATEREGISTRY     = 0x00000001
	CDS_GLOBAL             = 0x00000008
	DISP_CHANGE_SUCCESSFUL = 0

	DISPLAY_DEVICE_ACTIVE         = 0x00000001
	DISPLAY_DEVICE_PRIMARY_DEVICE = 0x00000004
)

var (
	procEnumDisplaySettingsExW = winapi.User32.NewProc("EnumDisplaySettingsExW")
	procChangeDisplaySettings  = winapi.User32.NewProc("ChangeDisplaySettingsExW")
	procEnumDisplayDevices     = winapi.User32.NewProc("EnumDisplayDevicesW")
)

type displayMode struct{ w, h, hz uint32 }

func currentMode(dev *uint16) (displayMode, error) {
	var dm devmode
	dm.Size = uint16(unsafe.Sizeof(dm))
	_, _, _ = procEnumDisplaySettingsExW.Call(uintptr(unsafe.Pointer(dev)), ENUM_CURRENT_SETTINGS, uintptr(unsafe.Pointer(&dm)), EDS_ROTATEDMODE)
	if dm.PelsWidth == 0 || dm.PelsHeight == 0 {
		return displayMode{}, fmt.Errorf("no current mode")
	}
	return displayMode{w: dm.PelsWidth, h: dm.PelsHeight, hz: dm.DisplayFrequency}, nil
}

func listModes(dev *uint16) []displayMode {
	var out []displayMode
	for i := uintptr(0); ; i++ {
		var dm devmode
		dm.Size = uint16(unsafe.Sizeof(dm))
		r, _, _ := procEnumDisplaySettingsExW.Call(uintptr(unsafe.Pointer(dev)), i, uintptr(unsafe.Pointer(&dm)), EDS_ROTATEDMODE)
		if r == 0 {
			break
		}
		if dm.PelsWidth > 0 && dm.PelsHeight > 0 {
			out = append(out, displayMode{w: dm.PelsWidth, h: dm.PelsHeight, hz: dm.DisplayFrequency})
		}
	}
	return out
}

func setMode(dev *uint16, target displayMode) error {
	var dm devmode
	dm.Size = uint16(unsafe.Sizeof(dm))
	_, _, _ = procEnumDisplaySettingsExW.Call(uintptr(unsafe.Pointer(dev)), ENUM_CURRENT_SETTINGS, uintptr(unsafe.Pointer(&dm)), EDS_ROTATEDMODE)
	dm.Fields = DM_PELSWIDTH | DM_PELSHEIGHT
	dm.PelsWidth, dm.PelsHeight = target.w, target.h
	if target.hz != 0 {
		dm.Fields |= DM_DISPLAYFREQUENCY
		dm.DisplayFrequency = target.hz
	}
	r, _, _ := procChangeDisplaySettings.Call(uintptr(unsafe.Pointer(dev)), uintptr(unsafe.Pointer(&dm)), 0, CDS_UPDATEREGISTRY|CDS_GLOBAL, 0)
	if int32(r) != DISP_CHANGE_SUCCESSFUL {
		return fmt.Errorf("ChangeDisplaySettingsEx failed: %d", r)
	}
	return nil
}

// Return a heap-allocated pointer to the primary display device name (\\.\DISPLAYx) or nil (let Windows pick).
func deviceNameForPrimary() *uint16 {
	var dd displayDeviceW
	dd.Cb = uint32(unsafe.Sizeof(dd))
	for i := uint32(0); ; i++ {
		r, _, _ := procEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0)
		if r == 0 {
			break
		}
		if (dd.StateFlags & (DISPLAY_DEVICE_ACTIVE | DISPLAY_DEVICE_PRIMARY_DEVICE)) == (DISPLAY_DEVICE_ACTIVE | DISPLAY_DEVICE_PRIMARY_DEVICE) {
			name := new([32]uint16)         // heap
			copy(name[:], dd.DeviceName[:]) // keep it alive after return
			return &name[0]
		}
	}
	return nil // fallback → primary/current
}

// Try 1920x1080 exactly; if unsupported, pick the best mode <=1080p (max area, then highest Hz).
func ForcePreferredResolution(dev *uint16, prefW, prefH uint32) (applied displayMode, err error) {
	if dev == nil {
		dev = deviceNameForPrimary()
	}
	cur, _ := currentMode(dev)
	modes := listModes(dev)
	if len(modes) == 0 {
		return cur, fmt.Errorf("no modes enumerated")
	}

	var exact *displayMode
	for i := range modes {
		m := modes[i]
		if m.w == prefW && m.h == prefH {
			if exact == nil || m.hz > exact.hz {
				exact = &m
			}
		}
	}
	if exact != nil {
		if err := setMode(dev, *exact); err == nil {
			return *exact, nil
		}
	}

	var best *displayMode
	bestArea := uint64(0)
	for i := range modes {
		m := modes[i]
		if m.w <= prefW && m.h <= prefH {
			area := uint64(m.w) * uint64(m.h)
			if area > bestArea || (area == bestArea && best != nil && m.hz > best.hz) {
				cp := m
				best = &cp
				bestArea = area
			}
		}
	}
	if best != nil {
		if err := setMode(dev, *best); err == nil {
			return *best, nil
		}
	}

	return cur, fmt.Errorf("no suitable mode applied")
}

func ForceMaxResolution(dev *uint16) (applied displayMode, err error) {
	if dev == nil {
		dev = deviceNameForPrimary()
	}
	cur, _ := currentMode(dev)
	modes := listModes(dev)
	if len(modes) == 0 {
		return cur, fmt.Errorf("no modes enumerated")
	}

	best := modes[0]
	bestArea := uint64(best.w) * uint64(best.h)
	for _, m := range modes[1:] {
		area := uint64(m.w) * uint64(m.h)
		if area > bestArea ||
			(area == bestArea && m.hz > best.hz) ||
			(area == bestArea && m.hz == best.hz && m.w > best.w) {
			best = m
			bestArea = area
		}
	}

	if cur.w == best.w && cur.h == best.h && (best.hz == 0 || cur.hz == best.hz) {
		return cur, nil
	}
	if err := setMode(dev, best); err != nil {
		return cur, err
	}
	return best, nil
}

// Convenience: force primary to 1080p or best-below.
func ForcePrimary1080pOrBest() error {
	_, err := ForcePreferredResolution(nil, 1920, 1080)
	return err
}

func UsePrimaryMaxResolution() error {
	_, err := ForceMaxResolution(nil)
	return err
}
