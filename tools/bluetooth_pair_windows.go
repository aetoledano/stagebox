//go:build windows

package tools

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ===== Win32 Bluetooth interop (bthprops.cpl) =====

var (
	bthDLL                  = windows.NewLazySystemDLL("bthprops.cpl")
	procFindFirstRadio      = bthDLL.NewProc("BluetoothFindFirstRadio")
	procFindRadioClose      = bthDLL.NewProc("BluetoothFindRadioClose")
	procFindFirstDevice     = bthDLL.NewProc("BluetoothFindFirstDevice")
	procFindNextDevice      = bthDLL.NewProc("BluetoothFindNextDevice")
	procFindDeviceClose     = bthDLL.NewProc("BluetoothFindDeviceClose")
	procAuthEx              = bthDLL.NewProc("BluetoothAuthenticateDeviceEx")
	procAuthPIN             = bthDLL.NewProc("BluetoothAuthenticateDevice")
	procUpdateDevice        = bthDLL.NewProc("BluetoothUpdateDeviceRecord")
	procIsConnectable       = bthDLL.NewProc("BluetoothIsConnectable")
	procFindFirstRadioReady bool
)

func loadBth() bool {
	if err := bthDLL.Load(); err != nil {
		return false
	}
	// All procs are lazy; just check one to confirm presence.
	if err := procFindFirstRadio.Find(); err == nil {
		procFindFirstRadioReady = true
		return true
	}
	return false
}

type BLUETOOTH_FIND_RADIO_PARAMS struct {
	DwSize uint32
}

// BOOL is 4 bytes; HANDLE 8 bytes on x64
type BLUETOOTH_DEVICE_SEARCH_PARAMS struct {
	DwSize               uint32
	FReturnAuthenticated int32
	FReturnRemembered    int32
	FReturnUnknown       int32
	FReturnConnected     int32
	FIssueInquiry        int32
	CTimeoutMultiplier   byte // *1.28s
	_                    [3]byte
	HRadio               windows.Handle
}

type SYSTEMTIME struct {
	Year         uint16
	Month        uint16
	DayOfWeek    uint16
	Day          uint16
	Hour         uint16
	Minute       uint16
	Second       uint16
	Milliseconds uint16
}

type BLUETOOTH_ADDRESS struct {
	Addr uint64 // lower 48 bits used (BDADDR in LE)
}

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

const (
	authMITMNotRequired = 0x00
)

func utf16ToString(buf []uint16) string {
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return windows.UTF16ToString(buf[:n])
}

// ===== Auto-pair supervisor =====

var (
	apMu     sync.Mutex
	apOn     bool
	apStopCh chan struct{}
	apWlist  []string
	apStatus = "off" // "off" | "running" | "error"
)

func AutoPairRunning() bool {
	apMu.Lock()
	defer apMu.Unlock()
	return apOn
}

func AutoPairStatus() string {
	apMu.Lock()
	defer apMu.Unlock()
	return apStatus
}

// StartAutoPair starts a background scan/pair loop.
// whitelist: comma-separated substrings (lowercased) to match device names; empty = match all.
func AutoPairStart(whitelist string) error {
	apMu.Lock()
	defer apMu.Unlock()
	if apOn {
		return nil
	}
	if !loadBth() {
		apStatus = "error"
		return fmt.Errorf("bluetooth support not available")
	}

	apWlist = nil
	for _, s := range strings.Split(strings.ToLower(whitelist), ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			apWlist = append(apWlist, s)
		}
	}
	apStopCh = make(chan struct{})
	apOn = true
	apStatus = "running"
	go apLoop(apStopCh)
	return nil
}

func AutoPairStop() {
	apMu.Lock()
	if !apOn {
		apMu.Unlock()
		return
	}
	close(apStopCh)
	apOn = false
	apStatus = "off"
	apMu.Unlock()
}

func nameMatches(name string) bool {
	if len(apWlist) == 0 {
		return true
	}
	n := strings.ToLower(name)
	for _, w := range apWlist {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

func apLoop(stop <-chan struct{}) {
	// Pick first radio (primary)
	var hRadio windows.Handle
	hFindRadio := windows.Handle(0)
	{
		var frp BLUETOOTH_FIND_RADIO_PARAMS
		frp.DwSize = uint32(unsafe.Sizeof(frp))
		r0, _, _ := procFindFirstRadio.Call(uintptr(unsafe.Pointer(&frp)), uintptr(unsafe.Pointer(&hRadio)))
		if r0 == 0 {
			// No radio → nothing to do; leave status as running but loop will do nothing.
		} else {
			hFindRadio = windows.Handle(r0)
			defer procFindRadioClose.Call(uintptr(hFindRadio))
		}
	}

	// Simple outer loop: keep issuing short inquiries until stopped.
	for {
		select {
		case <-stop:
			return
		default:
		}

		// If we have no radio, just sleep a bit via a short inquiry that will bail
		// when procFindFirstDevice fails with HANDLE==0.
		var sp BLUETOOTH_DEVICE_SEARCH_PARAMS
		sp.DwSize = uint32(unsafe.Sizeof(sp))
		sp.FReturnAuthenticated = 0
		sp.FReturnRemembered = 0
		sp.FReturnUnknown = 1 // we want new/unpaired
		sp.FReturnConnected = 0
		sp.FIssueInquiry = 1      // actively scan
		sp.CTimeoutMultiplier = 4 // ~5.1s
		sp.HRadio = hRadio

		var di BLUETOOTH_DEVICE_INFO
		di.DwSize = uint32(unsafe.Sizeof(di))

		hFind, _, _ := procFindFirstDevice.Call(
			uintptr(unsafe.Pointer(&sp)),
			uintptr(unsafe.Pointer(&di)),
		)
		if hFind != 0 {
			hFindDev := windows.Handle(hFind)
			// iterate all results for this inquiry
			for {
				// Stop ASAP if requested
				select {
				case <-stop:
					procFindDeviceClose.Call(uintptr(hFindDev))
					return
				default:
				}

				name := utf16ToString(di.SzName[:])

				if name != "" && nameMatches(name) && di.FAuthenticated == 0 {
					// Try to pair: 1) SSP/no-PIN, 2) fallback to PIN "0000"
					ok := pairTryAuthEx(hRadio, &di) || pairTryPIN(hRadio, &di, "0000")
					if ok {
						// Update record (best effort)
						procUpdateDevice.Call(uintptr(unsafe.Pointer(&di)))
					}
				}

				// Next device
				rn, _, _ := procFindNextDevice.Call(uintptr(hFindDev), uintptr(unsafe.Pointer(&di)))
				if rn == 0 {
					break
				}
			}
			procFindDeviceClose.Call(uintptr(hFindDev))
		}

		// Small gap between scans; ~1s
		select {
		case <-stop:
			return
		default:
		}
		windows.SleepEx(1000, false)
	}
}

func pairTryAuthEx(hRadio windows.Handle, di *BLUETOOTH_DEVICE_INFO) bool {
	// BluetoothAuthenticateDeviceEx(hwndParent=NULL, hRadio, di, pOobData=NULL, MITMNotRequired)
	r, _, _ := procAuthEx.Call(
		0,
		uintptr(hRadio),
		uintptr(unsafe.Pointer(di)),
		0,
		uintptr(uint32(authMITMNotRequired)),
	)
	return r == 0 // ERROR_SUCCESS
}

func pairTryPIN(hRadio windows.Handle, di *BLUETOOTH_DEVICE_INFO, pin string) bool {
	pin16, err := windows.UTF16FromString(pin)
	if err != nil {
		return false
	}
	r, _, _ := procAuthPIN.Call(
		0,
		uintptr(hRadio),
		uintptr(unsafe.Pointer(di)),
		uintptr(unsafe.Pointer(&pin16[0])),
		uintptr(uint32(len(pin))),
	)
	return r == 0
}
