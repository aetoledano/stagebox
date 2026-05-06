//go:build windows

package bluetooth

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	lgx "launcher/internal/logx" // adjust if needed

	"golang.org/x/sys/windows"
)

// Uses shared procs from bluetooth_state_windows.go via initBluetoothProcs()

type BLUETOOTH_FIND_RADIO_PARAMS struct{ DwSize uint32 }

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

const (
	authMITMNotRequired = 0x00

	rcOK               = 0
	rcNotAuthenticated = 1244 // ERROR_NOT_AUTHENTICATED
	rcGenFailure       = 31   // ERROR_GEN_FAILURE
)

func utf16ToString(buf []uint16) string {
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return windows.UTF16ToString(buf[:n])
}

func bdaddrString(a BLUETOOTH_ADDRESS) string {
	v := a.Addr & 0xFFFFFFFFFFFF
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// ===== Supervisor =====

var (
	apMu     sync.Mutex
	apOn     bool
	apStopCh chan struct{}
	apWlist  []string
	apStatus = "off"
)

func AutoPairRunning() bool  { apMu.Lock(); defer apMu.Unlock(); return apOn }
func AutoPairStatus() string { apMu.Lock(); defer apMu.Unlock(); return apStatus }

func AutoPairStart(whitelist string) error {
	apMu.Lock()
	defer apMu.Unlock()
	if apOn {
		return nil
	}
	if !initBluetoothProcs() {
		apStatus = "error"
		return fmt.Errorf("bluetooth support not available")
	}
	apWlist = nil
	for _, s := range strings.Split(strings.ToLower(whitelist), ",") {
		if t := strings.TrimSpace(s); t != "" {
			apWlist = append(apWlist, t)
		}
	}
	apStopCh = make(chan struct{})
	apOn = true
	apStatus = "running"
	lgx.Infof("[btpair] start: whitelist=%v", apWlist)
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
	lgx.Infof("[btpair] stop")
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
	radios := enumerateRadios()
	if len(radios) == 0 {
		lgx.Warnf("[btpair] no radios found; scanning anyway (stack may come up later)")
	}

	inqTimeout := byte(8) // ~10.2s inquiry
	tickerGap := 1000 * time.Millisecond

	for {
		select {
		case <-stop:
			for _, h := range radios {
				windows.CloseHandle(h)
			}
			return
		default:
		}

		for _, hRadio := range append([]windows.Handle{}, radios...) {
			scanOnce(stop, hRadio, inqTimeout)
			select {
			case <-stop:
				for _, h := range radios {
					windows.CloseHandle(h)
				}
				return
			default:
			}
		}

		time.Sleep(tickerGap)
	}
}

func enumerateRadios() []windows.Handle {
	found := make([]windows.Handle, 0, 2)

	var frp BLUETOOTH_FIND_RADIO_PARAMS
	frp.DwSize = uint32(unsafe.Sizeof(frp))

	var hRadio windows.Handle
	r0, _, _ := procFindFirstRadio.Call(uintptr(unsafe.Pointer(&frp)), uintptr(unsafe.Pointer(&hRadio)))
	if r0 == 0 {
		lgx.Debugf("[btpair] radios=0 (FindFirstRadio returned 0)")
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
	lgx.Debugf("[btpair] radios=%d", len(found))
	return found
}

func scanOnce(stop <-chan struct{}, hRadio windows.Handle, timeout byte) {
	var sp BLUETOOTH_DEVICE_SEARCH_PARAMS
	sp.DwSize = uint32(unsafe.Sizeof(sp))
	sp.FReturnAuthenticated = 0
	sp.FReturnRemembered = 1
	sp.FReturnUnknown = 1
	sp.FReturnConnected = 0
	sp.FIssueInquiry = 1
	sp.CTimeoutMultiplier = timeout
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
		select {
		case <-stop:
			return
		default:
		}

		name := utf16ToString(di.SzName[:])
		if name != "" && nameMatches(name) {
			addr := bdaddrString(di.Address)
			lgx.Debugf("[btpair] seen name=%q addr=%s remembered=%d authed=%d", name, addr, di.FRemembered, di.FAuthenticated)

			if di.FRemembered != 0 && di.FAuthenticated == 0 {
				if removeDevice(&di.Address) {
					lgx.Infof("[btpair] removed stale remember for %q (%s); will re-scan next loop", name, addr)
				} else {
					lgx.Warnf("[btpair] failed to remove stale device %q (%s)", name, addr)
				}
			} else if di.FAuthenticated == 0 {
				ok := false

				// 1) AuthEx (retry once if 1244-ish race)
				rc := pairAuthExRC(&di, hRadio)
				if rc == rcOK {
					ok = true
				} else {
					lgx.Debugf("[btpair] AuthEx failed rc=%d (retry)", rc)
					time.Sleep(500 * time.Millisecond)
					rc2 := pairAuthExRC(&di, hRadio)
					if rc2 == rcOK {
						ok = true
					} else {
						// 2) PINs (0000 -> 1234)
						lgx.Debugf("[btpair] trying PINs after AuthEx rc=%d", rc2)
						ok = pairPINRC(&di, hRadio, "0000") == rcOK ||
							pairPINRC(&di, hRadio, "1234") == rcOK
					}
				}

				if ok {
					lgx.Infof("[btpair] paired %q (%s)", name, addr)
					procUpdateDevice.Call(uintptr(unsafe.Pointer(&di)))
					time.Sleep(750 * time.Millisecond)
				} else {
					lgx.Warnf("[btpair] pair failed for %q (%s)", name, addr)
				}
			}
		}

		di.DwSize = uint32(unsafe.Sizeof(di))
		rn, _, _ := procFindNextDevice.Call(uintptr(hFindDev), uintptr(unsafe.Pointer(&di)))
		if rn == 0 {
			break
		}
	}
}

func removeDevice(addr *BLUETOOTH_ADDRESS) bool {
	r, _, _ := procRemoveDevice.Call(uintptr(unsafe.Pointer(addr)))
	return r == 0
}

// Helpers that return Win32 rc so we can branch on 1244/31 explicitly.

func pairAuthExRC(di *BLUETOOTH_DEVICE_INFO, hRadio windows.Handle) int {
	r, _, _ := procAuthEx.Call(
		0,
		uintptr(hRadio),
		uintptr(unsafe.Pointer(di)),
		0,
		uintptr(uint32(authMITMNotRequired)),
	)
	return int(r)
}

func pairPINRC(di *BLUETOOTH_DEVICE_INFO, hRadio windows.Handle, pin string) int {
	pin16, err := windows.UTF16FromString(pin)
	if err != nil || len(pin16) == 0 {
		return rcGenFailure
	}
	r, _, _ := procAuthPIN.Call(
		0,
		uintptr(hRadio),
		uintptr(unsafe.Pointer(di)),
		uintptr(unsafe.Pointer(&pin16[0])),
		uintptr(uint32(len(pin))),
	)
	return int(r)
}
