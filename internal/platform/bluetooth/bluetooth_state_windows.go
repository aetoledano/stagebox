//go:build windows

package bluetooth

import (
	"launcher/internal/logx"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// ---- Win32 Bluetooth (bthprops.cpl) interop ----

type bluetoothFindRadioParams struct {
	DwSize uint32
}

// ---- Win32 Bluetooth (bthprops.cpl) interop ----

// Prefer bthprops.cpl; on some systems functions also exist in BluetoothApis.dll
var (
	bthMainDLL = windows.NewLazySystemDLL("bthprops.cpl")
	bthAltDLL  = windows.NewLazySystemDLL("BluetoothApis.dll")

	// All procs are declared here but bound inside initBluetoothProcs()
	procFindFirstRadio *windows.LazyProc
	procFindNextRadio  *windows.LazyProc
	procFindRadioClose *windows.LazyProc
	procIsConnectable  *windows.LazyProc
	procIsDiscoverable *windows.LazyProc // optional

	procFindFirstDevice *windows.LazyProc
	procFindNextDevice  *windows.LazyProc
	procFindDeviceClose *windows.LazyProc

	bluetoothProcsInitialized bool
	bluetoothProcsOnce        sync.Once
)

func initBluetoothProcs() bool {
	bluetoothProcsOnce.Do(func() { loadBluetoothProcs() })
	return bluetoothProcsInitialized
}

func loadBluetoothProcs() bool {
	if bluetoothProcsInitialized {
		return true
	}
	bind := func(d *windows.LazyDLL) bool {
		if err := d.Load(); err != nil {
			return false
		}
		ff := d.NewProc("BluetoothFindFirstRadio")
		fn := d.NewProc("BluetoothFindNextRadio")
		fc := d.NewProc("BluetoothFindRadioClose")
		ic := d.NewProc("BluetoothIsConnectable")
		// Optional on some builds:
		id := d.NewProc("BluetoothIsDiscoverable")

		fd1 := d.NewProc("BluetoothFindFirstDevice")
		fd2 := d.NewProc("BluetoothFindNextDevice")
		fdC := d.NewProc("BluetoothFindDeviceClose")

		if ff.Find() == nil && fn.Find() == nil && fc.Find() == nil &&
			ic.Find() == nil &&
			fd1.Find() == nil && fd2.Find() == nil && fdC.Find() == nil {

			procFindFirstRadio, procFindNextRadio, procFindRadioClose = ff, fn, fc
			procIsConnectable = ic
			// id may fail Find(); that’s OK—treat as optional
			if id.Find() == nil {
				procIsDiscoverable = id
			} else {
				procIsDiscoverable = nil
			}

			procFindFirstDevice, procFindNextDevice, procFindDeviceClose = fd1, fd2, fdC

			bluetoothProcsInitialized = true
			logx.Debug("[bt] Win32 Bluetooth enumeration APIs loaded")
			return true
		}
		return false
	}
	// try main, then alt
	if bind(bthMainDLL) {
		return true
	}

	return bind(bthAltDLL)
}

// ---- Service fallback ----

func btServiceState() (running bool, disabled bool) {
	m, err := mgr.Connect()
	if err != nil {
		return false, false
	}
	defer m.Disconnect()
	s, err := m.OpenService("bthserv") // Bluetooth Support Service
	if err != nil {
		return false, false
	}
	defer s.Close()

	st, err := s.Query()
	if err != nil {
		return false, false
	}
	cfg, err := s.Config()
	if err != nil {
		return st.State == svc.Running, false
	}
	// SERVICE_DISABLED = 0x4
	return st.State == svc.Running, cfg.StartType == 0x4
}

// ---- Public API ----

// State returns "on" if any radio is connectable,
// "off" if radios exist but none are connectable (or service running but no connectable radio),
// "disabled" if the service is disabled or no radios & service stopped,
// "unknown" on errors.
func State() string {
	loaded := initBluetoothProcs()
	logx.Debugf("[bt] DLL loaded: %v", loaded)

	// Prefer Win32 Bluetooth APIs (most accurate)
	if loaded {
		var params bluetoothFindRadioParams
		params.DwSize = uint32(unsafe.Sizeof(params))

		var hRadio windows.Handle
		hFind, _, _ := procFindFirstRadio.Call(
			uintptr(unsafe.Pointer(&params)),
			uintptr(unsafe.Pointer(&hRadio)),
		)
		if hFind != 0 {
			defer procFindRadioClose.Call(hFind)

			anyRadio := false
			connectable := false

			for {
				anyRadio = true
				r, _, _ := procIsConnectable.Call(uintptr(hRadio))
				if r != 0 {
					connectable = true
				}
				_ = windows.CloseHandle(hRadio)

				r2, _, _ := procFindNextRadio.Call(
					hFind,
					uintptr(unsafe.Pointer(&hRadio)),
				)
				if r2 == 0 {
					break
				}
			}

			logx.Debugf("[bt] radios=%t connectable=%t", anyRadio, connectable)

			// If any radio is connectable → ON
			if connectable {
				return "on"
			}
			// If radios exist but aren't connectable, don't hard-fail yet:
			// some stacks report non-connectable while the service is up.
			if anyRadio {
				// fall through to service check before declaring "off"
			}
		} else {
			logx.Debug("[bt] no radios enumerated via Win32 API")
		}
	}

	// Fallback/heuristic via service state
	running, disabled := btServiceState()
	logx.Debugf("[bt] service running=%v disabled=%v", running, disabled)

	// More forgiving heuristic:
	// - running service → "on" (even if Win32 reported not connectable)
	// - disabled → "disabled"
	// - else → "off"
	switch {
	case running:
		return "on"
	case disabled:
		return "disabled"
	default:
		return "off"
	}
}

// Start core service: bthserv
func startBthServ() (running bool) {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService("bthserv")
	if err != nil {
		return false
	}
	defer s.Close()

	st, err := s.Query()
	if err == nil && st.State == svc.Running {
		return true
	}

	_ = s.Start() // best-effort
	st, _ = s.Query()
	return st.State == svc.Running
}

// Kick per-user service & try to power ON radio via WinRT (PowerShell)
func startUserBTAndTurnOn() {
	ps := `
$ErrorActionPreference="SilentlyContinue"
# Start per-user BT service instance(s)
Get-Service BthUserService* | Start-Service
# WinRT: turn Bluetooth radio ON if present
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$r=[Windows.Devices.Radios.Radio]::GetRadiosAsync().GetAwaiter().GetResult() |
    Where-Object {$_.Kind -eq [Windows.Devices.Radios.RadioKind]::Bluetooth}
if ($r) { $null = $r[0].SetStateAsync([Windows.Devices.Radios.RadioState]::On).GetAwaiter().GetResult() }
`
	_ = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps).Start()
}

// EnsureUp tries to ensure BT is usable, quickly.
func EnsureUp() {
	ok := startBthServ()
	_ = ok // even if false, still try user service
	startUserBTAndTurnOn()
}
