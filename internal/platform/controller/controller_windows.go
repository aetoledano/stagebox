//go:build windows

package controller

import (
	"context"
	lgx "launcher/internal/logx"
	"math"
	"os"
	goRuntime "runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

var (
	xinput                       *windows.LazyDLL
	xinputGetState               *windows.LazyProc
	legacyJoystickReady          atomic.Bool
	connectedControllers         atomic.Int32
	winmm                        = windows.NewLazySystemDLL("winmm.dll")
	procJoyGetNumDevs            = winmm.NewProc("joyGetNumDevs")
	procJoyGetPosEx              = winmm.NewProc("joyGetPosEx")
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetCursor                = user32.NewProc("GetCursorPos")
	procSetCursor                = user32.NewProc("SetCursorPos")
	procMouseEvent               = user32.NewProc("mouse_event")
	procKeybdEvent               = user32.NewProc("keybd_event")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procSendMessage              = user32.NewProc("SendMessageW")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
)

type xinputState struct {
	dwPacketNumber uint32
	Gamepad        xinputGamepad
}

type joyInfoEx struct {
	dwSize         uint32
	dwFlags        uint32
	dwXpos         uint32
	dwYpos         uint32
	dwZpos         uint32
	dwRpos         uint32
	dwUpos         uint32
	dwVpos         uint32
	dwButtons      uint32
	dwButtonNumber uint32
	dwPOV          uint32
	dwReserved1    uint32
	dwReserved2    uint32
}

const (
	ERROR_DEVICE_NOT_CONNECTED = 1167
	joyReturnAll               = 0x000000FF
	joyPovCentered             = 0x0000FFFF

	mouseEventLeftDown   = 0x0002
	mouseEventLeftUp     = 0x0004
	mouseEventRightDown  = 0x0008
	mouseEventRightUp    = 0x0010
	mouseEventMiddleDown = 0x0020
	mouseEventMiddleUp   = 0x0040
	keyEventKeyUp        = 0x0002
	vkShift              = 0x10
	vkTab                = 0x09
	vkReturn             = 0x0D
	vkEscape             = 0x1B
	vkSpace              = 0x20
	vkLeft               = 0x25
	vkUp                 = 0x26
	vkRight              = 0x27
	vkDown               = 0x28
	vkDelete             = 0x2E
	vk0                  = 0x30
	vk9                  = 0x39
	vkA                  = 0x41
	vkZ                  = 0x5A
	vkNumpad0            = 0x60
	vkF1                 = 0x70
	vkF12                = 0x7B
	vkLeftShift          = 0xA0
	vkRightShift         = 0xA1
	vkLeftControl        = 0xA2
	vkRightControl       = 0xA3
	vkLeftAlt            = 0xA4
	vkRightAlt           = 0xA5
	wmNextDlgCtl         = 0x0028
	swShow               = 5
	swRestore            = 9
)

const (
	triggerThreshold = 30
)

type point struct {
	X int32
	Y int32
}

type GameKeyMap struct {
	Buttons          map[string][]string
	LeftStick        map[string][]string
	RightStick       map[string][]string
	Mouse            string
	MouseSensitivity int
}

type gameKeyBinding struct {
	source    gameInputSource
	button    uint16
	direction string
	vk        byte
	mouse     string
}

type gameInputSource int

const (
	gameInputButton gameInputSource = iota
	gameInputLeftStick
	gameInputRightStick
	gameInputLeftTrigger
	gameInputRightTrigger
)

type gameStickKeyState struct {
	up    bool
	down  bool
	left  bool
	right bool
}

func loadXInput() error {
	var err error
	for _, name := range []string{"xinput1_4.dll", "xinput9_1_0.dll", "xinput1_3.dll"} {
		d := windows.NewLazySystemDLL(name)
		if e := d.Load(); e != nil {
			err = e
			continue
		}
		p := d.NewProc("XInputGetState")
		if e := p.Find(); e != nil {
			err = e
			continue
		}
		xinput, xinputGetState = d, p
		return nil
	}
	return err
}

func loadLegacyJoystick() error {
	if legacyJoystickReady.Load() {
		return nil
	}
	if err := winmm.Load(); err != nil {
		return err
	}
	if err := procJoyGetNumDevs.Find(); err != nil {
		return err
	}
	if err := procJoyGetPosEx.Find(); err != nil {
		return err
	}
	legacyJoystickReady.Store(true)
	return nil
}

type ControllerBridge struct {
	ctx                               context.Context
	cancel                            context.CancelFunc
	paused                            atomic.Int32 // 0 = running, 1 = paused
	resetInput                        atomic.Bool
	quit                              chan struct{}
	stopOnce                          sync.Once
	mouse                             atomic.Int32 // 0 = launcher events, 1 = OS cursor mode
	keyboard                          atomic.Int32 // 0 = launcher events, 1 = pairing keyboard mode
	gameKeys                          atomic.Int32 // 0 = launcher events, 1 = game key-map mode
	inputTargetPID                    atomic.Uint32
	inputTargetWindow                 atomic.Uintptr
	volumeAction                      func(string)
	guideAction                       func(string)
	virtualInput                      atomic.Pointer[virtualInputSession]
	preferences                       *controllerPreferenceStore
	inventory                         atomic.Pointer[controllerInventory]
	testing                           atomic.Pointer[controllerTestRequest]
	selectController                  atomic.Pointer[controllerTestRequest]
	nextInventoryEvent, nextTestEvent time.Time // SDL owner only
	aPressed                          bool      // physical A edge, owned by poller
	keyBindings                       []gameKeyBinding
	keyBindingsMu                     sync.Mutex
	activeGameKeys                    map[byte]bool
	mouseStick                        string
	mouseStepX                        int
	mouseStepY                        int
	// Edge/hold state is owned by the polling loop.
	prevButtons uint16
	holdDir     string
	nextRepeat  time.Time
	prevGamepad xinputGamepad
	activeIndex uint32
	legacyIndex uint32
	killCombo   time.Time
	killArmed   bool
}

func StartControllerBridge(ctx context.Context, volumeAction func(string), downloadDatabase bool, guideAction ...func(string)) *ControllerBridge {
	ctx, cancel := context.WithCancel(ctx)
	b := &ControllerBridge{
		ctx:          ctx,
		cancel:       cancel,
		quit:         make(chan struct{}),
		volumeAction: volumeAction,
	}
	if len(guideAction) > 0 {
		b.guideAction = guideAction[0]
	}
	var err error
	b.preferences, err = newControllerPreferenceStore()
	if err != nil {
		lgx.Warnf("controller preferences unavailable: %v", err)
	}
	go ensureControllerDatabase(ctx, downloadDatabase)
	go func() {
		recoverVirtualInputState(ctx)
		b.loop()
	}()
	return b
}

func (b *ControllerBridge) Pause() {
	b.paused.Store(1)
	b.resetInput.Store(true)
}
func (b *ControllerBridge) Resume() {
	b.resetInput.Store(true)
	b.paused.Store(0)
}
func (b *ControllerBridge) EnableMouseMode() { b.EnableMouseModeForPID(0) }
func (b *ControllerBridge) EnableMouseModeForPID(pid uint32) {
	b.EnableMouseModeForWindow(pid, 0)
}
func (b *ControllerBridge) EnableMouseModeForWindow(pid uint32, window uintptr) {
	b.inputTargetPID.Store(pid)
	b.inputTargetWindow.Store(window)
	b.gameKeys.Store(0)
	b.keyboard.Store(0)
	b.mouse.Store(1)
	b.resetInput.Store(true)
}
func (b *ControllerBridge) DisableMouseMode() {
	b.mouse.Store(0)
	b.keyboard.Store(0)
	b.gameKeys.Store(0)
	b.inputTargetPID.Store(0)
	b.inputTargetWindow.Store(0)
	b.clearGameKeyMap()
	b.resetInput.Store(true)
}
func (b *ControllerBridge) EnablePairingKeyboardModeForPID(pid uint32) {
	b.inputTargetPID.Store(pid)
	b.mouse.Store(0)
	b.gameKeys.Store(0)
	b.clearGameKeyMap()
	b.keyboard.Store(1)
	b.resetInput.Store(true)
}
func (b *ControllerBridge) DisablePairingKeyboardMode() {
	b.keyboard.Store(0)
	b.mouse.Store(0)
	b.gameKeys.Store(0)
	b.inputTargetPID.Store(0)
	b.resetInput.Store(true)
}
func (b *ControllerBridge) EnableGameKeyMap(keyMap GameKeyMap) bool {
	bindings := buildGameKeyBindings(keyMap)
	mouseStick := normalizeInputName(keyMap.Mouse)
	if mouseStick != "leftstick" && mouseStick != "rightstick" {
		mouseStick = ""
	}
	if len(bindings) == 0 && mouseStick == "" {
		return false
	}
	b.clearGameKeyMap()
	b.keyBindingsMu.Lock()
	b.keyBindings = bindings
	b.activeGameKeys = map[byte]bool{}
	b.mouseStick = mouseStick

	base := keyMap.MouseSensitivity
	if base <= 0 {
		base = 14
	}
	if base > 40 {
		base = 40
	}

	b.mouseStepX = base
	b.mouseStepY = base / 2
	if b.mouseStepY < 1 {
		b.mouseStepY = 1
	}
	b.keyBindingsMu.Unlock()

	b.inputTargetPID.Store(0)
	b.mouse.Store(0)
	b.keyboard.Store(0)
	b.gameKeys.Store(1)
	b.resetInput.Store(true)
	return true
}
func (b *ControllerBridge) SetGameInputTargetPID(pid uint32) {
	b.inputTargetPID.Store(pid)
}
func (b *ControllerBridge) DisableGameKeyMap() {
	b.gameKeys.Store(0)
	b.clearGameKeyMap()
	b.resetInput.Store(true)
}

func (b *ControllerBridge) clearGameKeyMap() {
	b.keyBindingsMu.Lock()
	defer b.keyBindingsMu.Unlock()
	b.releaseGameInputsLocked()
	b.keyBindings = nil
	b.activeGameKeys = nil
	b.mouseStick = ""
	b.mouseStepX = 0
	b.mouseStepY = 0
}

func (b *ControllerBridge) releaseGameInputs() {
	b.keyBindingsMu.Lock()
	defer b.keyBindingsMu.Unlock()
	b.releaseGameInputsLocked()
}

func (b *ControllerBridge) releaseGameInputsLocked() {
	for vk := range b.activeGameKeys {
		switch vk {
		case 0xF0:
			mouseButtonUp("left")
		case 0xF1:
			mouseButtonUp("right")
		case 0xF2:
			mouseButtonUp("middle")
		default:
			keyUp(vk)
		}
		delete(b.activeGameKeys, vk)
	}
}

func FocusProcessWindow(pid uint32) bool {
	return focusProcessWindow(pid)
}

func HasVisibleWindowForPID(pid uint32) bool {
	return findProcessWindow(pid) != 0
}

func MoveCursorTo(x int, y int) {
	procSetCursor.Call(uintptr(int32(x)), uintptr(int32(y)))
}

func SendKeyStrokeToPID(pid uint32, key string) bool {
	vk, ok := virtualKey(key)
	if !ok {
		return false
	}
	if pid != 0 {
		FocusProcessWindow(pid)
	}
	pressKey(vk)
	return true
}

func ConnectedCount() int {
	return int(connectedControllers.Load())
}

func nativeConnectedCount() int {

	count := 0
	if xinputGetState != nil {
		var st xinputState
		for i := uint32(0); i < 4; i++ {
			r1, _, _ := xinputGetState.Call(uintptr(i), uintptr(unsafe.Pointer(&st)))
			if r1 == 0 {
				count++
			}
		}
	}
	if count > 0 {
		return count
	}
	if loadLegacyJoystick() != nil {
		return 0
	}
	return legacyConnectedCount()
}

func legacyConnectedCount() int {
	if loadLegacyJoystick() != nil {
		return 0
	}
	n, _, _ := procJoyGetNumDevs.Call()
	if n > 16 {
		n = 16
	}

	count := 0
	for i := uintptr(0); i < n; i++ {
		if _, ok := pollLegacyIndex(uint32(i)); ok {
			count++
		}
	}
	return count
}

func (b *ControllerBridge) pollXInputGamepad(lastConn time.Time) (xinputGamepad, bool) {
	if xinputGetState == nil {
		return xinputGamepad{}, false
	}

	var st xinputState
	r1, _, _ := xinputGetState.Call(uintptr(b.activeIndex), uintptr(unsafe.Pointer(&st)))
	if r1 == 0 {
		return st.Gamepad, true
	}

	if r1 != ERROR_DEVICE_NOT_CONNECTED && time.Since(lastConn) <= 2*time.Second {
		return xinputGamepad{}, false
	}

	for i := uint32(0); i < 4; i++ {
		r1, _, _ = xinputGetState.Call(uintptr(i), uintptr(unsafe.Pointer(&st)))
		if r1 == 0 {
			b.activeIndex = i
			return st.Gamepad, true
		}
	}
	return xinputGamepad{}, false
}

func (b *ControllerBridge) pollLegacyGamepad() (xinputGamepad, bool) {
	if loadLegacyJoystick() != nil {
		return xinputGamepad{}, false
	}
	if gp, ok := pollLegacyIndex(b.legacyIndex); ok {
		return gp, true
	}

	n, _, _ := procJoyGetNumDevs.Call()
	if n > 16 {
		n = 16
	}
	for i := uint32(0); i < uint32(n); i++ {
		gp, ok := pollLegacyIndex(i)
		if ok {
			b.legacyIndex = i
			return gp, true
		}
	}
	return xinputGamepad{}, false
}

func pollLegacyIndex(index uint32) (xinputGamepad, bool) {
	var joy joyInfoEx
	joy.dwSize = uint32(unsafe.Sizeof(joy))
	joy.dwFlags = joyReturnAll

	r1, _, _ := procJoyGetPosEx.Call(uintptr(index), uintptr(unsafe.Pointer(&joy)))
	if r1 != 0 {
		return xinputGamepad{}, false
	}
	return legacyJoyToGamepad(joy), true
}

func legacyJoyToGamepad(joy joyInfoEx) xinputGamepad {
	buttons := uint16(0)
	if joy.dwButtons&(1<<0) != 0 {
		buttons |= XINPUT_GAMEPAD_A
	}
	if joy.dwButtons&(1<<1) != 0 {
		buttons |= XINPUT_GAMEPAD_B
	}
	if joy.dwButtons&(1<<2) != 0 {
		buttons |= XINPUT_GAMEPAD_X
	}
	if joy.dwButtons&(1<<3) != 0 {
		buttons |= XINPUT_GAMEPAD_Y
	}
	if joy.dwButtons&(1<<4) != 0 {
		buttons |= XINPUT_GAMEPAD_LEFT_SHOULDER
	}
	if joy.dwButtons&(1<<5) != 0 {
		buttons |= XINPUT_GAMEPAD_RIGHT_SHOULDER
	}
	if joy.dwButtons&(1<<6) != 0 {
		buttons |= XINPUT_GAMEPAD_BACK
	}
	if joy.dwButtons&(1<<7) != 0 {
		buttons |= XINPUT_GAMEPAD_START
	}
	if joy.dwButtons&(1<<8) != 0 {
		buttons |= XINPUT_GAMEPAD_LEFT_THUMB
	}
	if joy.dwButtons&(1<<9) != 0 {
		buttons |= XINPUT_GAMEPAD_RIGHT_THUMB
	}

	if joy.dwPOV != joyPovCentered {
		switch {
		case joy.dwPOV >= 31500 || joy.dwPOV <= 4500:
			buttons |= XINPUT_GAMEPAD_DPAD_UP
		case joy.dwPOV >= 4500 && joy.dwPOV <= 13500:
			buttons |= XINPUT_GAMEPAD_DPAD_RIGHT
		case joy.dwPOV >= 13500 && joy.dwPOV <= 22500:
			buttons |= XINPUT_GAMEPAD_DPAD_DOWN
		case joy.dwPOV >= 22500 && joy.dwPOV <= 31500:
			buttons |= XINPUT_GAMEPAD_DPAD_LEFT
		}
	}

	return xinputGamepad{
		wButtons: buttons,
		sThumbLX: legacyAxis(joy.dwXpos),
		sThumbLY: legacyAxisInverted(joy.dwYpos),
		sThumbRX: legacyAxis(joy.dwZpos),
		sThumbRY: legacyAxisInverted(joy.dwRpos),
	}
}

func legacyAxis(value uint32) int16 {
	if value > 65535 {
		value = 65535
	}
	return int16(int32(value) - 32768)
}

func legacyAxisInverted(value uint32) int16 {
	if value > 65535 {
		value = 65535
	}
	v := int32(32768) - int32(value)
	if v > 32767 {
		v = 32767
	}
	if v < -32768 {
		v = -32768
	}
	return int16(v)
}

func (b *ControllerBridge) Stop() {
	b.stopOnce.Do(func() {
		b.cancel()
		close(b.quit)
	})
}

func (b *ControllerBridge) loop() {
	goRuntime.LockOSThread()
	defer goRuntime.UnlockOSThread()
	defer lgx.RecoverPanic("controller bridge")
	var sdl *sdlBackend
	if strings.EqualFold(strings.TrimSpace(os.Getenv("STAGEBOX_CONTROLLER_BACKEND")), "legacy") {
		lgx.Info("controller backend forced to legacy XInput/WinMM")
	} else {
		var err error
		sdl, err = openSDLBackend()
		if err != nil {
			lgx.Warnf("SDL3 controller backend unavailable; using legacy XInput/WinMM: %v", err)
		}
	}
	// Initialize native fallback on this same owner thread, never in status reads.
	if err := loadXInput(); err != nil {
		lgx.Warnf("XInput fallback unavailable: %v", err)
	}
	if err := loadLegacyJoystick(); err != nil {
		lgx.Warnf("WinMM joystick fallback unavailable: %v", err)
	}
	defer func() {
		if err := b.DisableVirtualInput(); err != nil {
			lgx.Warnf("virtual input shutdown: %v", err)
		}
		b.releaseGameInputs()
		connectedControllers.Store(0)
		if sdl != nil {
			sdl.shutdown()
		}
	}()
	stickDZ := int16(8500)
	var lastConn time.Time
	var launcherGate launcherInputGate
	var deviceGate launcherInputGate
	var volumeStick volumeStickState
	var deviceID uint64
	var mappingRevision uint64
	var lastLauncherFocus time.Time
	var nextNativeCount time.Time
	nativeCount := 0
	resetDevice := func() {
		b.cancelPhysicalA()
		b.releaseGameInputs()
		b.prevGamepad = xinputGamepad{}
		b.prevButtons, b.holdDir = 0, ""
		b.nextRepeat, b.killCombo = time.Time{}, time.Time{}
		b.killArmed = false
		launcherGate.reset()
		deviceGate.reset()
		volumeStick.reset()
	}

	t := time.NewTicker(8 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-b.quit:
			return
		case <-b.ctx.Done():
			return
		case <-t.C:
		}
		if b.resetInput.Swap(false) {
			b.cancelPhysicalA()
			volumeStick.reset()
			b.prevButtons = 0
			b.holdDir = ""
			b.nextRepeat = time.Time{}
			b.killCombo = time.Time{}
			b.killArmed = false
			launcherGate.reset()
			deviceGate.reset()
		}

		now := time.Now()
		guideAllowed := b.paused.Load() == 0 && b.mouse.Load() == 0 && b.keyboard.Load() == 0 && b.gameKeys.Load() == 0 && b.testing.Load() == nil
		if guideAllowed && LauncherHasFocus() {
			lastLauncherFocus = now
		}
		var gp xinputGamepad
		var currentID uint64
		ok := false
		virtual := b.virtualInput.Load()
		if sdl != nil {
			if selected := b.selectController.Swap(nil); selected != nil {
				for id, info := range sdl.info {
					if info.ID == selected.id {
						sdl.activeID = id
						break
					}
				}
			}
			if virtual != nil {
				sdl.setVirtualSources(virtual.sourcePaths)
				virtual.pinOnce.Do(func() { close(virtual.pinned) })
			} else {
				sdl.setVirtualSources(nil)
			}
			state, id, count, err := sdl.poll(now)
			if err == nil && sdl.guidePressed != 0 && guideAllowed && now.Sub(lastLauncherFocus) <= 150*time.Millisecond && b.guideAction != nil {
				b.guideAction(sdl.info[sdl.guidePressed].ID) // Callback only queues work; no Steam/window IO on SDL's owner.
			}
			if sdl.mappingRevision != mappingRevision {
				resetDevice()
				if virtual != nil {
					virtual.resetGates()
				}
				mappingRevision = sdl.mappingRevision
			}
			if err == nil {
				devices := sdl.inventory()
				b.publishInventory(devices, now)
				if virtual != nil {
					virtual.publishDevices(devices)
				}
			}
			if virtual != nil {
				connectedControllers.Store(int32(count))
			}
			if err != nil {
				lgx.Warnf("SDL3 input failed; switching to legacy XInput/WinMM: %v", err)
				sdl.shutdown()
				sdl = nil
			} else if id != 0 {
				gp, ok, currentID = state, true, uint64(3)<<32|uint64(id)
				connectedControllers.Store(int32(count))
			}
		}
		if sdl == nil {
			b.publishInventory(nil, now)
			if virtual != nil {
				virtual.publishDevices(nil)
			}
		}
		if !ok && virtual == nil {
			gp, ok = b.pollXInputGamepad(lastConn)
			if ok {
				lastConn = now
				currentID = uint64(1)<<32 | uint64(b.activeIndex)
			} else if gp, ok = b.pollLegacyGamepad(); ok {
				currentID = uint64(2)<<32 | uint64(b.legacyIndex)
			}
			if !now.Before(nextNativeCount) || currentID != deviceID {
				nativeCount = nativeConnectedCount()
				nextNativeCount = now.Add(250 * time.Millisecond)
			}
			connectedControllers.Store(int32(nativeCount))
		}
		if currentID != deviceID {
			resetDevice()
			deviceID = currentID
			backend := "none"
			switch currentID >> 32 {
			case 1:
				backend = "xinput"
			case 2:
				backend = "winmm"
			case 3:
				backend = "sdl3"
			}
			lgx.Infof("controller input device changed backend=%s id=%d", backend, uint32(currentID))
		}
		if b.testing.Load() != nil {
			b.cancelPhysicalA()
			b.prevButtons = 0
			continue
		}
		if !ok {
			if virtual != nil && sdl == nil {
				connectedControllers.Store(0)
			}
			b.cancelPhysicalA()
			continue
		}
		// Never inherit another pad's edges, held kill combo or mapped keys.
		if !deviceGate.allow(true, gamepadNeutral(gp)) {
			b.cancelPhysicalA()
			continue
		}

		buttons := gp.wButtons
		just := func(mask uint16) bool { return (buttons&mask) != 0 && (b.prevButtons&mask) == 0 }

		both := (buttons&XINPUT_GAMEPAD_START) != 0 && (buttons&XINPUT_GAMEPAD_BACK) != 0
		if both {
			b.cancelPhysicalA()
			volumeStick.reset()
			launcherGate.reset()
			if !b.killArmed {
				if b.killCombo.IsZero() {
					b.killCombo = time.Now()
				} else if time.Since(b.killCombo) > 700*time.Millisecond {
					runtime.EventsEmit(b.ctx, "pad:press", "KILL")
					b.killArmed = true
				}
			}
			b.prevGamepad = gp
			b.prevButtons = buttons
			continue
		}
		b.killCombo = time.Time{}
		b.killArmed = false

		if b.mouse.Load() == 1 {
			b.cancelPhysicalA()
			volumeStick.reset()
			launcherGate.reset()
			b.handleMouseMode(gp, buttons, just)
			b.prevGamepad = gp
			b.prevButtons = buttons
			continue
		}
		if b.keyboard.Load() == 1 {
			b.cancelPhysicalA()
			volumeStick.reset()
			launcherGate.reset()
			b.handlePairingKeyboardMode(gp, buttons, just)
			b.prevGamepad = gp
			b.prevButtons = buttons
			continue
		}
		if b.gameKeys.Load() == 1 {
			b.cancelPhysicalA()
			volumeStick.reset()
			launcherGate.reset()
			b.handleGameKeyMap(gp, buttons, just)
			b.prevGamepad = gp
			b.prevButtons = buttons
			continue
		}

		lx, ly := int(gp.sThumbLX), int(gp.sThumbLY)
		dz := int(stickDZ)
		enabled := b.paused.Load() == 0 && LauncherHasFocus()
		if action := volumeStick.update(time.Now(), int(gp.sThumbRX), int(gp.sThumbRY), enabled); action != "" && b.volumeAction != nil {
			b.volumeAction(action)
		}
		neutral := buttons == 0 && lx*lx+ly*ly < dz*dz
		if !launcherGate.allow(enabled, neutral) {
			b.cancelPhysicalA()
			b.holdDir = ""
			b.nextRepeat = time.Time{}
			b.prevGamepad = gp
			b.prevButtons = buttons
			continue
		}

		// D-pad edges
		if just(XINPUT_GAMEPAD_DPAD_LEFT) {
			runtime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": "left"})
		}
		if just(XINPUT_GAMEPAD_DPAD_RIGHT) {
			runtime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": "right"})
		}
		if just(XINPUT_GAMEPAD_DPAD_UP) {
			runtime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": "up"})
		}
		if just(XINPUT_GAMEPAD_DPAD_DOWN) {
			runtime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": "down"})
		}

		held := ""
		if lx*lx+ly*ly >= dz*dz { // outside radial deadzone
			if math.Abs(float64(lx)) >= math.Abs(float64(ly)) {
				if lx < 0 {
					held = "left"
				} else {
					held = "right"
				}
			} else {
				if ly > 0 {
					held = "up"
				} else {
					held = "down"
				}
			}
		}

		if held == "" {
			b.holdDir = ""
		} else if b.holdDir == "" {
			b.holdDir = held
			b.nextRepeat = time.Now().Add(300 * time.Millisecond)
			runtime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": held})
		} else if time.Now().After(b.nextRepeat) {
			b.nextRepeat = time.Now().Add(180 * time.Millisecond)
			runtime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": b.holdDir})
		}

		// Buttons
		if just(XINPUT_GAMEPAD_A) {
			b.aPressed = true
			runtime.EventsEmit(b.ctx, "pad:button", map[string]any{"name": "A", "down": true})
		}
		if b.aPressed && buttons&XINPUT_GAMEPAD_A == 0 {
			b.aPressed = false
			runtime.EventsEmit(b.ctx, "pad:button", map[string]any{"name": "A", "down": false})
		}
		if just(XINPUT_GAMEPAD_B) {
			runtime.EventsEmit(b.ctx, "pad:press", "B")
		}
		if just(XINPUT_GAMEPAD_START) && (buttons&XINPUT_GAMEPAD_BACK) == 0 {
			runtime.EventsEmit(b.ctx, "pad:press", "START")
		}
		if just(XINPUT_GAMEPAD_LEFT_SHOULDER) {
			runtime.EventsEmit(b.ctx, "pad:press", "LB")
		}
		if just(XINPUT_GAMEPAD_RIGHT_SHOULDER) {
			runtime.EventsEmit(b.ctx, "pad:press", "RB")
		}

		b.prevGamepad = gp
		b.prevButtons = buttons
	}
}

func (b *ControllerBridge) cancelPhysicalA() {
	if b.aPressed {
		b.aPressed = false
		runtime.EventsEmit(b.ctx, "pad:button", map[string]any{"name": "A", "down": false, "cancelled": true})
	}
}

func (b *ControllerBridge) handleMouseMode(gp xinputGamepad, buttons uint16, just func(uint16) bool) {
	if pid := b.inputTargetPID.Load(); pid != 0 {
		hwnd, _, _ := procKeyboardGetForegroundWindow.Call()
		if window := b.inputTargetWindow.Load(); window != 0 && hwnd != window {
			return
		}
		var foregroundPID uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&foregroundPID)))
		if foregroundPID != pid {
			return
		}
	}
	const (
		stickDeadzone = 6500
		maxStep       = 14
		dpadStep      = 9
	)

	dx, dy := 0, 0
	lx, ly := int(gp.sThumbLX), int(gp.sThumbLY)
	if lx*lx+ly*ly >= stickDeadzone*stickDeadzone {
		dx += lx * maxStep / 32767
		dy -= ly * maxStep / 32767
	}
	if buttons&XINPUT_GAMEPAD_DPAD_LEFT != 0 {
		dx -= dpadStep
	}
	if buttons&XINPUT_GAMEPAD_DPAD_RIGHT != 0 {
		dx += dpadStep
	}
	if buttons&XINPUT_GAMEPAD_DPAD_UP != 0 {
		dy -= dpadStep
	}
	if buttons&XINPUT_GAMEPAD_DPAD_DOWN != 0 {
		dy += dpadStep
	}
	if dx != 0 || dy != 0 {
		moveCursor(dx, dy)
	}
	if just(XINPUT_GAMEPAD_A) {
		leftClick()
	}
	if just(XINPUT_GAMEPAD_B) {
		if b.inputTargetWindow.Load() == 0 {
			FocusProcessWindow(b.inputTargetPID.Load())
		}
		pressEscape()
	}
}

func (b *ControllerBridge) handlePairingKeyboardMode(gp xinputGamepad, buttons uint16, just func(uint16) bool) {
	const stickDeadzone = 8500

	pid := b.inputTargetPID.Load()
	if just(XINPUT_GAMEPAD_A) {
		pressPairingKey(pid, vkReturn)
	}
	if just(XINPUT_GAMEPAD_B) {
		pressPairingKey(pid, vkEscape)
	}

	held := ""
	if buttons&XINPUT_GAMEPAD_DPAD_UP != 0 {
		held = "arrow-up"
	} else if buttons&XINPUT_GAMEPAD_DPAD_DOWN != 0 {
		held = "arrow-down"
	} else if buttons&XINPUT_GAMEPAD_DPAD_LEFT != 0 {
		held = "arrow-left"
	} else if buttons&XINPUT_GAMEPAD_DPAD_RIGHT != 0 {
		held = "arrow-right"
	} else {
		lx := int(gp.sThumbLX)
		if lx <= -stickDeadzone {
			held = "tab-left"
		} else if lx >= stickDeadzone {
			held = "tab-right"
		}
	}

	if held == "" {
		b.holdDir = ""
		return
	}
	if b.holdDir != held {
		b.holdDir = held
		b.nextRepeat = time.Now().Add(300 * time.Millisecond)
		sendPairingDirection(pid, held)
		return
	}
	if time.Now().After(b.nextRepeat) {
		b.nextRepeat = time.Now().Add(160 * time.Millisecond)
		sendPairingDirection(pid, held)
	}
}

func (b *ControllerBridge) handleGameKeyMap(gp xinputGamepad, buttons uint16, just func(uint16) bool) {
	b.keyBindingsMu.Lock()
	defer b.keyBindingsMu.Unlock()
	b.handleGameMouse(gp)
	if b.activeGameKeys == nil {
		b.activeGameKeys = map[byte]bool{}
	}

	for _, binding := range b.keyBindings {
		pressed, wasPressed := b.gameBindingState(binding, gp, buttons)
		if binding.mouse != "" {
			if pressed && !b.activeGameKeys[binding.activeKey()] {
				mouseButtonDown(binding.mouse)
				b.activeGameKeys[binding.activeKey()] = true
			} else if !pressed && wasPressed && !b.mappedMouseStillPressed(binding.mouse, gp, buttons) {
				mouseButtonUp(binding.mouse)
				delete(b.activeGameKeys, binding.activeKey())
			}
			continue
		}
		if pressed && !b.activeGameKeys[binding.activeKey()] {
			keyDown(binding.vk)
			b.activeGameKeys[binding.activeKey()] = true
		} else if !pressed && wasPressed && !b.mappedVKStillPressed(binding.vk, gp, buttons) {
			keyUp(binding.vk)
			delete(b.activeGameKeys, binding.activeKey())
		}
	}
}

func (b *ControllerBridge) handleGameMouse(gp xinputGamepad) {
	if b.mouseStick == "" {
		return
	}

	dx, dy := 0, 0
	if b.mouseStick == "leftstick" {
		dx, dy = stickMouseDelta(gp.sThumbLX, gp.sThumbLY, b.mouseStepX, b.mouseStepY)
	} else {
		dx, dy = stickMouseDelta(gp.sThumbRX, gp.sThumbRY, b.mouseStepX, b.mouseStepY)
	}

	if dx != 0 || dy != 0 {
		moveCursor(dx, dy)
	}
}

func stickMouseDelta(x int16, y int16, maxStepX int, maxStepY int) (int, int) {
	const stickDeadzone = 6500

	lx, ly := int(x), int(y)
	if lx*lx+ly*ly < stickDeadzone*stickDeadzone {
		return 0, 0
	}

	dx := lx * maxStepX / 32767
	dy := -ly * maxStepY / 32767

	return dx, dy
}

func (b *ControllerBridge) mappedVKStillPressed(vk byte, gp xinputGamepad, buttons uint16) bool {
	for _, binding := range b.keyBindings {
		pressed, _ := b.gameBindingState(binding, gp, buttons)
		if binding.mouse == "" && binding.vk == vk && pressed {
			return true
		}
	}
	return false
}

func (b *ControllerBridge) mappedMouseStillPressed(mouse string, gp xinputGamepad, buttons uint16) bool {
	for _, binding := range b.keyBindings {
		pressed, _ := b.gameBindingState(binding, gp, buttons)
		if binding.mouse == mouse && pressed {
			return true
		}
	}
	return false
}

func (binding gameKeyBinding) activeKey() byte {
	if binding.mouse == "" {
		return binding.vk
	}
	switch binding.mouse {
	case "left":
		return 0xF0
	case "right":
		return 0xF1
	case "middle":
		return 0xF2
	default:
		return 0
	}
}

func (b *ControllerBridge) gameBindingState(binding gameKeyBinding, gp xinputGamepad, buttons uint16) (bool, bool) {
	switch binding.source {
	case gameInputButton:
		return buttons&binding.button != 0, b.prevButtons&binding.button != 0
	case gameInputLeftStick:
		cur := stickDirections(gp.sThumbLX, gp.sThumbLY)
		prev := stickDirections(b.prevGamepad.sThumbLX, b.prevGamepad.sThumbLY)
		return directionPressed(binding.direction, cur), directionPressed(binding.direction, prev)
	case gameInputRightStick:
		cur := stickDirections(gp.sThumbRX, gp.sThumbRY)
		prev := stickDirections(b.prevGamepad.sThumbRX, b.prevGamepad.sThumbRY)
		return directionPressed(binding.direction, cur), directionPressed(binding.direction, prev)

	case gameInputLeftTrigger:
		return gp.bLeftTrigger > triggerThreshold, b.prevGamepad.bLeftTrigger > triggerThreshold

	case gameInputRightTrigger:
		return gp.bRightTrigger > triggerThreshold, b.prevGamepad.bRightTrigger > triggerThreshold

	default:
		return false, false
	}
}

func buildGameKeyBindings(keyMap GameKeyMap) []gameKeyBinding {
	total := countGameBindings(keyMap)
	if total == 0 {
		return nil
	}

	bindings := make([]gameKeyBinding, 0, total)

	for buttonName, keys := range keyMap.Buttons {
		source, button, ok := gameButtonSource(buttonName)
		if !ok {
			continue
		}

		for _, keyName := range keys {
			if mouse, ok := mouseButtonName(keyName); ok {
				bindings = append(bindings, gameKeyBinding{
					source: source,
					button: button,
					mouse:  mouse,
				})
				continue
			}

			vk, ok := virtualKey(keyName)
			if !ok {
				continue
			}

			bindings = append(bindings, gameKeyBinding{
				source: source,
				button: button,
				vk:     vk,
			})
		}
	}

	bindings = append(bindings, buildStickKeyBindings(gameInputLeftStick, keyMap.LeftStick)...)
	bindings = append(bindings, buildStickKeyBindings(gameInputRightStick, keyMap.RightStick)...)

	return bindings
}

func gameButtonSource(name string) (gameInputSource, uint16, bool) {
	switch normalizeInputName(name) {
	case "lt", "l2", "lefttrigger":
		return gameInputLeftTrigger, 0, true

	case "rt", "r2", "righttrigger":
		return gameInputRightTrigger, 0, true
	}

	button, ok := gameButtonMask(name)
	if !ok {
		return gameInputButton, 0, false
	}

	return gameInputButton, button, true
}

func buildStickKeyBindings(source gameInputSource, stickMap map[string][]string) []gameKeyBinding {
	bindings := make([]gameKeyBinding, 0, len(stickMap))
	for dirName, keys := range stickMap {
		for _, keyName := range keys {
			dir, ok := gameStickDirection(dirName)
			if !ok {
				continue
			}
			if mouse, ok := mouseButtonName(keyName); ok {
				bindings = append(bindings, gameKeyBinding{source: source, direction: dir, mouse: mouse})
				continue
			}
			vk, ok := virtualKey(keyName)
			if !ok {
				continue
			}
			bindings = append(bindings, gameKeyBinding{source: source, direction: dir, vk: vk})
		}
	}
	return bindings
}

func gameStickDirection(name string) (string, bool) {
	switch normalizeInputName(name) {
	case "up":
		return "up", true
	case "down":
		return "down", true
	case "left":
		return "left", true
	case "right":
		return "right", true
	default:
		return "", false
	}
}

func stickDirections(x int16, y int16) gameStickKeyState {
	const stickDeadzone = 12000
	lx, ly := int(x), int(y)
	if lx*lx+ly*ly < stickDeadzone*stickDeadzone {
		return gameStickKeyState{}
	}
	if math.Abs(float64(lx)) >= math.Abs(float64(ly)) {
		if lx < 0 {
			return gameStickKeyState{left: true}
		}
		return gameStickKeyState{right: true}
	}
	if ly > 0 {
		return gameStickKeyState{up: true}
	}
	return gameStickKeyState{down: true}
}

func directionPressed(direction string, state gameStickKeyState) bool {
	switch direction {
	case "up":
		return state.up
	case "down":
		return state.down
	case "left":
		return state.left
	case "right":
		return state.right
	default:
		return false
	}
}

func gameButtonMask(name string) (uint16, bool) {
	switch normalizeInputName(name) {
	case "a":
		return XINPUT_GAMEPAD_A, true
	case "b":
		return XINPUT_GAMEPAD_B, true
	case "x":
		return XINPUT_GAMEPAD_X, true
	case "y":
		return XINPUT_GAMEPAD_Y, true
	case "l3", "leftstick", "leftthumb", "leftstickpress", "leftthumbpress":
		return XINPUT_GAMEPAD_LEFT_THUMB, true
	case "r3", "rightstick", "rightthumb", "rightstickpress", "rightthumbpress":
		return XINPUT_GAMEPAD_RIGHT_THUMB, true
	case "start":
		return XINPUT_GAMEPAD_START, true
	case "back", "select":
		return XINPUT_GAMEPAD_BACK, true
	case "lb", "leftbumper", "leftshoulder":
		return XINPUT_GAMEPAD_LEFT_SHOULDER, true
	case "rb", "rightbumper", "rightshoulder":
		return XINPUT_GAMEPAD_RIGHT_SHOULDER, true
	case "dpadup", "up":
		return XINPUT_GAMEPAD_DPAD_UP, true
	case "dpaddown", "down":
		return XINPUT_GAMEPAD_DPAD_DOWN, true
	case "dpadleft", "left":
		return XINPUT_GAMEPAD_DPAD_LEFT, true
	case "dpadright", "right":
		return XINPUT_GAMEPAD_DPAD_RIGHT, true
	default:
		return 0, false
	}
}

func virtualKey(name string) (byte, bool) {
	n := normalizeInputName(name)
	switch n {
	case "space":
		return vkSpace, true
	case "enter", "return":
		return vkReturn, true
	case "escape", "esc":
		return vkEscape, true
	case "tab":
		return vkTab, true
	case "shift", "leftshift":
		return vkLeftShift, true
	case "rightshift":
		return vkRightShift, true
	case "control", "ctrl", "leftcontrol", "leftctrl":
		return vkLeftControl, true
	case "rightcontrol", "rightctrl":
		return vkRightControl, true
	case "alt", "leftalt":
		return vkLeftAlt, true
	case "rightalt":
		return vkRightAlt, true
	case "arrowup", "up":
		return vkUp, true
	case "arrowdown", "down":
		return vkDown, true
	case "arrowleft", "left":
		return vkLeft, true
	case "arrowright", "right":
		return vkRight, true
	case "delete", "del":
		return vkDelete, true
	}

	if len(n) == 1 {
		c := n[0]
		switch {
		case c >= 'a' && c <= 'z':
			return vkA + (c - 'a'), true
		case c >= '0' && c <= '9':
			return vk0 + (c - '0'), true
		}
	}
	if strings.HasPrefix(n, "f") && len(n) <= 3 {
		num := 0
		for _, ch := range n[1:] {
			if ch < '0' || ch > '9' {
				return 0, false
			}
			num = num*10 + int(ch-'0')
		}
		if num >= 1 && num <= 12 {
			return vkF1 + byte(num-1), true
		}
	}
	if strings.HasPrefix(n, "numpad") && len(n) == len("numpad0") {
		c := n[len(n)-1]
		if c >= '0' && c <= '9' {
			return vkNumpad0 + (c - '0'), true
		}
	}

	return 0, false
}

func mouseButtonName(name string) (string, bool) {
	switch normalizeInputName(name) {
	case "mouseleft", "leftmouse", "btnleft", "leftclick":
		return "left", true
	case "mouseright", "rightmouse", "btnright", "rightclick":
		return "right", true
	case "mousemiddle", "middlemouse", "btnmiddle", "middleclick":
		return "middle", true
	default:
		return "", false
	}
}

func normalizeInputName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, "_", "")
	n = strings.ReplaceAll(n, "-", "")
	n = strings.ReplaceAll(n, " ", "")
	return n
}

func sendPairingDirection(pid uint32, dir string) {
	switch dir {
	case "arrow-up":
		pressPairingKey(pid, vkUp)
	case "arrow-down":
		pressPairingKey(pid, vkDown)
	case "arrow-left":
		pressPairingKey(pid, vkLeft)
	case "arrow-right":
		pressPairingKey(pid, vkRight)
	case "tab-left":
		stepPairingDialogFocus(pid, true)
	case "tab-right":
		stepPairingDialogFocus(pid, false)
	}
}

func pressPairingKey(pid uint32, vk byte) {
	FocusProcessWindow(pid)
	pressKey(vk)
}

func pressPairingShiftTab(pid uint32) {
	FocusProcessWindow(pid)
	procKeybdEvent.Call(vkShift, 0, 0, 0)
	pressKey(vkTab)
	procKeybdEvent.Call(vkShift, 0, keyEventKeyUp, 0)
}

func stepPairingDialogFocus(pid uint32, previous bool) {
	hwnd := findProcessWindow(pid)
	if hwnd == 0 {
		if previous {
			pressPairingShiftTab(pid)
			return
		}
		pressPairingKey(pid, vkTab)
		return
	}

	procShowWindow.Call(hwnd, swShow)
	procSetForegroundWindow.Call(hwnd)
	direction := uintptr(0)
	if previous {
		direction = 1
	}
	procSendMessage.Call(hwnd, wmNextDlgCtl, direction, 0)
}

type enumWindowSearch struct {
	pid  uint32
	hwnd uintptr
}

func focusProcessWindow(pid uint32) bool {
	if pid == 0 {
		return false
	}
	hwnd := findProcessWindow(pid)
	if hwnd == 0 {
		return false
	}
	minimized, _, _ := procIsIconic.Call(hwnd)
	if minimized != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	procSetForegroundWindow.Call(hwnd)
	return true
}

func findProcessWindow(pid uint32) uintptr {
	if pid == 0 {
		return 0
	}
	search := &enumWindowSearch{pid: pid}
	procEnumWindows.Call(windows.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		s := (*enumWindowSearch)(unsafe.Pointer(lparam))
		var windowPID uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if windowPID == s.pid {
			visible, _, _ := procIsWindowVisible.Call(hwnd)
			if visible != 0 {
				s.hwnd = hwnd
				return 0
			}
		}
		return 1
	}), uintptr(unsafe.Pointer(search)))
	return search.hwnd
}

func moveCursor(dx int, dy int) {
	var p point
	r, _, _ := procGetCursor.Call(uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		return
	}
	procSetCursor.Call(uintptr(int32(p.X)+int32(dx)), uintptr(int32(p.Y)+int32(dy)))
}

func leftClick() {
	procMouseEvent.Call(mouseEventLeftDown, 0, 0, 0, 0)
	procMouseEvent.Call(mouseEventLeftUp, 0, 0, 0, 0)
}

func mouseButtonDown(button string) {
	switch button {
	case "left":
		procMouseEvent.Call(mouseEventLeftDown, 0, 0, 0, 0)
	case "right":
		procMouseEvent.Call(mouseEventRightDown, 0, 0, 0, 0)
	case "middle":
		procMouseEvent.Call(mouseEventMiddleDown, 0, 0, 0, 0)
	}
}

func mouseButtonUp(button string) {
	switch button {
	case "left":
		procMouseEvent.Call(mouseEventLeftUp, 0, 0, 0, 0)
	case "right":
		procMouseEvent.Call(mouseEventRightUp, 0, 0, 0, 0)
	case "middle":
		procMouseEvent.Call(mouseEventMiddleUp, 0, 0, 0, 0)
	}
}

func pressEscape() {
	pressKey(vkEscape)
}

func pressKey(vk byte) {
	keyDown(vk)
	keyUp(vk)
}

func keyDown(vk byte) {
	procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
}

func keyUp(vk byte) {
	procKeybdEvent.Call(uintptr(vk), 0, keyEventKeyUp, 0)
}

func countGameBindings(keyMap GameKeyMap) int {
	total := 0

	for _, keys := range keyMap.Buttons {
		total += len(keys)
	}
	for _, keys := range keyMap.LeftStick {
		total += len(keys)
	}
	for _, keys := range keyMap.RightStick {
		total += len(keys)
	}

	return total
}
