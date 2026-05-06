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

var defaultPairDenylist = []string{
	"adapter",
	"audio",
	"buds",
	"dongle",
	"headphone",
	"headset",
	"keyboard",
	"mouse",
	"phone",
	"receiver",
	"samsung",
	"speaker",
	"television",
	"[tv]",
	"tv",
	"usb adapter",
	"wireless adapter",
}

func AutoPairRunning() bool  { apMu.Lock(); defer apMu.Unlock(); return apOn }
func AutoPairStatus() string { apMu.Lock(); defer apMu.Unlock(); return apStatus }

func setAutoPairStatus(status string) {
	apMu.Lock()
	apStatus = status
	apMu.Unlock()
}

func AutoPairStart(whitelist string) error {
	apMu.Lock()
	defer apMu.Unlock()
	if apOn {
		return nil
	}
	apStatus = "starting"
	go EnsureUp()
	if !initBluetoothProcs() {
		apStatus = "error"
		return fmt.Errorf("bluetooth support not available")
	}
	apWlist = nil
	for _, s := range strings.Split(strings.ToLower(whitelist), ",") {
		if t := strings.TrimSpace(s); t != "" {
			apWlist = append(apWlist, normalizeWhitelistTerm(t))
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

func normalizeWhitelistTerm(term string) string {
	switch term {
	case "pro":
		return "pro controller"
	case "ps5":
		return "dualsense"
	case "ps4":
		return "dualshock"
	default:
		return term
	}
}

func nameDenied(name string) (string, bool) {
	n := strings.ToLower(name)
	for _, denied := range defaultPairDenylist {
		if denied == "tv" {
			if hasNameToken(n, denied) {
				return denied, true
			}
			continue
		}
		if strings.Contains(n, denied) {
			return denied, true
		}
	}
	return "", false
}

func majorDeviceClass(cod uint32) uint32 {
	return (cod >> 8) & 0x1F
}

func minorDeviceClass(cod uint32) uint32 {
	return (cod >> 2) & 0x3F
}

func className(cod uint32) string {
	switch majorDeviceClass(cod) {
	case 0x04:
		return "audio-video"
	case 0x05:
		return "peripheral"
	default:
		return fmt.Sprintf("major-0x%02X", majorDeviceClass(cod))
	}
}

func classLooksPairable(cod uint32) bool {
	if majorDeviceClass(cod) != 0x05 {
		return false
	}
	minor := minorDeviceClass(cod)
	return minor == 0x01 || minor == 0x02 || minor == 0x08 || minor == 0x09
}

func hasNameToken(name string, token string) bool {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, part := range parts {
		if part == token {
			return true
		}
	}
	return false
}

func apLoop(stop <-chan struct{}) {
	inqTimeout := byte(8) // ~10.2s inquiry
	tickerGap := 1000 * time.Millisecond

	for {
		select {
		case <-stop:
			return
		default:
		}

		radios := enumerateRadios()
		if len(radios) == 0 {
			setAutoPairStatus("no-radio")
			lgx.Warnf("[btpair] no radios found; will retry")
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Second):
				continue
			}
		}
		setAutoPairStatus("scanning")

		for _, hRadio := range radios {
			scanOnce(stop, hRadio, inqTimeout)
			select {
			case <-stop:
				closeRadioHandles(radios)
				return
			default:
			}
		}
		closeRadioHandles(radios)

		select {
		case <-stop:
			return
		case <-time.After(tickerGap):
		}
	}
}

func closeRadioHandles(radios []windows.Handle) {
	for _, h := range radios {
		_ = windows.CloseHandle(h)
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
	lgx.Debugf("[btpair] radio handles=%d", len(found))
	return found
}

func scanOnce(stop <-chan struct{}, hRadio windows.Handle, timeout byte) {
	var sp BLUETOOTH_DEVICE_SEARCH_PARAMS
	sp.DwSize = uint32(unsafe.Sizeof(sp))
	sp.FReturnAuthenticated = 0
	sp.FReturnRemembered = 1
	sp.FReturnUnknown = 1
	sp.FReturnConnected = 1
	sp.FIssueInquiry = 1
	sp.CTimeoutMultiplier = timeout
	sp.HRadio = hRadio

	var di BLUETOOTH_DEVICE_INFO
	di.DwSize = uint32(unsafe.Sizeof(di))

	hFind, _, _ := procFindFirstDevice.Call(uintptr(unsafe.Pointer(&sp)), uintptr(unsafe.Pointer(&di)))
	if hFind == 0 {
		lgx.Debugf("[btpair] scan found no devices for radio=%v", hRadio)
		return
	}
	hFindDev := windows.Handle(hFind)
	defer procFindDeviceClose.Call(uintptr(hFindDev))

	seen := 0
	unnamed := 0
	deniedCount := 0
	skippedCount := 0
	candidateCount := 0
	defer func() {
		lgx.Debugf(
			"[btpair] scan summary radio=%v seen=%d candidates=%d denied=%d skipped=%d unnamed=%d",
			hRadio,
			seen,
			candidateCount,
			deniedCount,
			skippedCount,
			unnamed,
		)
	}()

	for {
		select {
		case <-stop:
			return
		default:
		}

		seen++
		name := utf16ToString(di.SzName[:])
		addr := bdaddrString(di.Address)
		class := className(di.UlClassofDevice)
		pairableClass := classLooksPairable(di.UlClassofDevice)
		if name == "" {
			unnamed++
			if pairableClass {
				lgx.Debugf("[btpair] unnamed pairable addr=%s class=0x%06X kind=%s remembered=%d authed=%d connected=%d", addr, di.UlClassofDevice, class, di.FRemembered, di.FAuthenticated, di.FConnected)
			} else {
				lgx.Debugf("[btpair] skipped unnamed addr=%s class=0x%06X kind=%s remembered=%d authed=%d connected=%d", addr, di.UlClassofDevice, class, di.FRemembered, di.FAuthenticated, di.FConnected)
			}
		}
		_, denied := nameDenied(name)
		if reason, ok := nameDenied(name); ok {
			deniedCount++
			lgx.Debugf("[btpair] skipped denied name=%q addr=%s class=0x%06X kind=%s reason=%q", name, addr, di.UlClassofDevice, class, reason)
		} else if name != "" && !nameMatches(name) {
			skippedCount++
			lgx.Debugf("[btpair] skipped name=%q addr=%s class=0x%06X kind=%s whitelist=%v", name, addr, di.UlClassofDevice, class, apWlist)
		}
		if !denied && (nameMatches(name) || (name == "" && pairableClass)) {
			candidateCount++
			lgx.Debugf("[btpair] seen name=%q addr=%s class=0x%06X kind=%s remembered=%d authed=%d connected=%d", name, addr, di.UlClassofDevice, class, di.FRemembered, di.FAuthenticated, di.FConnected)

			if di.FRemembered != 0 && di.FAuthenticated == 0 {
				if removeDevice(&di.Address) {
					lgx.Infof("[btpair] removed stale remember for %q (%s); will re-scan next loop", name, addr)
				} else {
					lgx.Warnf("[btpair] failed to remove stale device %q (%s)", name, addr)
				}
			} else if di.FAuthenticated == 0 {
				ok := false

				// 1) AuthEx (retry once if 1244-ish race)
				if bluetoothHaveAuthEx {
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
							lgx.Debugf("[btpair] AuthEx retry failed rc=%d", rc2)
						}
					}
				} else {
					lgx.Debug("[btpair] AuthEx unavailable; skipping")
				}

				if !ok {
					if bluetoothHaveAuthPIN {
						lgx.Debugf("[btpair] trying PINs for %q", name)
						ok = pairPINRC(&di, hRadio, "0000") == rcOK ||
							pairPINRC(&di, hRadio, "1234") == rcOK
					} else {
						lgx.Debug("[btpair] PIN auth unavailable; skipping")
					}
				}

				if ok {
					setAutoPairStatus("paired")
					lgx.Infof("[btpair] paired %q (%s)", name, addr)
					procUpdateDevice.Call(uintptr(unsafe.Pointer(&di)))
					time.Sleep(750 * time.Millisecond)
				} else {
					setAutoPairStatus("pair-failed")
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
