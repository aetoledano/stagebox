//go:build windows

package controller

import (
	"context"
	"math"
	"time"
	"unsafe"

	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

var (
	xinput         *windows.LazyDLL
	xinputGetState *windows.LazyProc
)

type xinputGamepad struct {
	wButtons      uint16
	bLeftTrigger  byte
	bRightTrigger byte
	sThumbLX      int16
	sThumbLY      int16
	sThumbRX      int16
	sThumbRY      int16
}
type xinputState struct {
	dwPacketNumber uint32
	Gamepad        xinputGamepad
}

const (
	XINPUT_GAMEPAD_DPAD_UP        = 0x0001
	XINPUT_GAMEPAD_DPAD_DOWN      = 0x0002
	XINPUT_GAMEPAD_DPAD_LEFT      = 0x0004
	XINPUT_GAMEPAD_DPAD_RIGHT     = 0x0008
	XINPUT_GAMEPAD_LEFT_SHOULDER  = 0x0100
	XINPUT_GAMEPAD_RIGHT_SHOULDER = 0x0200
	XINPUT_GAMEPAD_A              = 0x1000
	XINPUT_GAMEPAD_B              = 0x2000
	XINPUT_GAMEPAD_START          = 0x0010
	XINPUT_GAMEPAD_BACK           = 0x0020
	ERROR_DEVICE_NOT_CONNECTED    = 1167
)

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

type XInputBridge struct {
	ctx    context.Context
	paused atomic.Int32 // 0 = running, 1 = paused
	quit   chan struct{}
	// edge/hold state (cleared when paused)
	prevButtons uint16
	holdDir     string
	nextRepeat  time.Time
	activeIndex uint32
	killCombo   time.Time
	killArmed   bool
}

func StartXInputBridge(ctx context.Context) *XInputBridge {
	if err := loadXInput(); err != nil {
		return nil // no XInput available
	}
	b := &XInputBridge{
		ctx:  ctx,
		quit: make(chan struct{}),
	}
	go b.loop()
	return b
}

func (b *XInputBridge) Pause() {
	b.paused.Store(1)
	// clear edges so resuming doesn't emit stale presses
	b.prevButtons = 0
	b.holdDir = ""
	b.nextRepeat = time.Time{}
}
func (b *XInputBridge) Resume() { b.paused.Store(0) }
func (b *XInputBridge) Stop() {
	select {
	case <-b.quit:
	default:
		close(b.quit)
	}
}

func (b *XInputBridge) loop() {
	defer func() { _ = recover() }()
	stickDZ := int16(8500)
	var lastConn time.Time

	t := time.NewTicker(8 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-b.quit:
			return
		case <-t.C:
		}

		// poll current controller
		var st xinputState
		r1, _, _ := xinputGetState.Call(uintptr(b.activeIndex), uintptr(unsafe.Pointer(&st)))
		if r1 != 0 {
			// rescan 0..3 if disconnected or every 2s
			if r1 == ERROR_DEVICE_NOT_CONNECTED || time.Since(lastConn) > 2*time.Second {
				found := false
				for i := uint32(0); i < 4; i++ {
					r1, _, _ = xinputGetState.Call(uintptr(i), uintptr(unsafe.Pointer(&st)))
					if r1 == 0 {
						b.activeIndex = i
						found = true
						break
					}
				}
				if !found {
					b.prevButtons, b.holdDir = 0, ""
					continue
				}
			}
		} else {
			lastConn = time.Now()
		}

		gp := st.Gamepad
		buttons := gp.wButtons
		just := func(mask uint16) bool { return (buttons&mask) != 0 && (b.prevButtons&mask) == 0 }

		both := (buttons&XINPUT_GAMEPAD_START) != 0 && (buttons&XINPUT_GAMEPAD_BACK) != 0
		if both {
			if !b.killArmed {
				if b.killCombo.IsZero() {
					b.killCombo = time.Now()
				} else if time.Since(b.killCombo) > 700*time.Millisecond {
					runtime.EventsEmit(b.ctx, "pad:press", "KILL")
					b.killArmed = true // <— require release before next kill
				}
			}
		} else {
			b.killCombo = time.Time{}
			b.killArmed = false
		}

		if b.paused.Load() == 1 {
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
		lx, ly := int(gp.sThumbLX), int(gp.sThumbLY)
		dz := int(stickDZ)
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
			runtime.EventsEmit(b.ctx, "pad:press", "A")
		}
		if just(XINPUT_GAMEPAD_B) {
			runtime.EventsEmit(b.ctx, "pad:press", "B")
		}
		if just(XINPUT_GAMEPAD_LEFT_SHOULDER) {
			runtime.EventsEmit(b.ctx, "pad:press", "LB")
		}
		if just(XINPUT_GAMEPAD_RIGHT_SHOULDER) {
			runtime.EventsEmit(b.ctx, "pad:press", "RB")
		}

		b.prevButtons = buttons
	}
}
