//go:build windows

package controller

import (
	"context"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

const (
	whKeyboardLL = 13
	hcAction     = 0
	wmKeydown    = 0x0100
	wmSysKeydown = 0x0104
	wmQuit       = 0x0012
)

var (
	keyboardUser32                  = windows.NewLazySystemDLL("user32.dll")
	keyboardKernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procSetWindowsHookEx            = keyboardUser32.NewProc("SetWindowsHookExW")
	procCallNextHookEx              = keyboardUser32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx         = keyboardUser32.NewProc("UnhookWindowsHookEx")
	procGetMessage                  = keyboardUser32.NewProc("GetMessageW")
	procTranslateMessage            = keyboardUser32.NewProc("TranslateMessage")
	procDispatchMessage             = keyboardUser32.NewProc("DispatchMessageW")
	procPostThreadMessage           = keyboardUser32.NewProc("PostThreadMessageW")
	procKeyboardGetForegroundWindow = keyboardUser32.NewProc("GetForegroundWindow")
	procKeyboardGetCurrentThreadID  = keyboardKernel32.NewProc("GetCurrentThreadId")
)

type KeyboardBridge struct {
	ctx      context.Context
	paused   atomic.Int32
	threadID atomic.Uint32
	quit     chan struct{}
	stopOnce sync.Once
	hook     windows.Handle
	callback uintptr
}

type keyboardHookEvent struct {
	VKCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type keyboardMessage struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

func StartKeyboardBridge(ctx context.Context) *KeyboardBridge {
	b := &KeyboardBridge{
		ctx:  ctx,
		quit: make(chan struct{}),
	}
	go b.loop()
	go func() {
		<-ctx.Done()
		b.Stop()
	}()
	return b
}

func (b *KeyboardBridge) Pause() {
	if b == nil {
		return
	}
	b.paused.Store(1)
}

func (b *KeyboardBridge) Resume() {
	if b == nil {
		return
	}
	b.paused.Store(0)
}

func (b *KeyboardBridge) Stop() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() {
		close(b.quit)
		if tid := b.threadID.Load(); tid != 0 {
			procPostThreadMessage.Call(uintptr(tid), wmQuit, 0, 0)
		}
	})
}

func (b *KeyboardBridge) loop() {
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	tid, _, _ := procKeyboardGetCurrentThreadID.Call()
	b.threadID.Store(uint32(tid))
	select {
	case <-b.quit:
		return
	default:
	}
	b.callback = windows.NewCallback(b.hookProc)

	hook, _, _ := procSetWindowsHookEx.Call(whKeyboardLL, b.callback, 0, 0)
	if hook == 0 {
		return
	}
	b.hook = windows.Handle(hook)
	defer procUnhookWindowsHookEx.Call(uintptr(b.hook))

	var msg keyboardMessage
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (b *KeyboardBridge) hookProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode == hcAction && (wParam == wmKeydown || wParam == wmSysKeydown) {
		event := (*keyboardHookEvent)(unsafe.Pointer(lParam))
		if b.handleKey(event.VKCode) {
			return 1
		}
	}

	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func (b *KeyboardBridge) handleKey(vk uint32) bool {
	if b.paused.Load() == 1 || !foregroundBelongsToCurrentProcess() {
		return false
	}

	switch vk {
	case vkLeft:
		wailsruntime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": "left"})
	case vkRight:
		wailsruntime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": "right"})
	case vkUp:
		wailsruntime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": "up"})
	case vkDown:
		wailsruntime.EventsEmit(b.ctx, "pad:nav", map[string]any{"dir": "down"})
	case vkReturn:
		wailsruntime.EventsEmit(b.ctx, "pad:press", "A")
	case vkEscape:
		wailsruntime.EventsEmit(b.ctx, "pad:press", "B")
	case vkSpace:
		wailsruntime.EventsEmit(b.ctx, "pad:press", "START")
	default:
		return false
	}

	return true
}

func foregroundBelongsToCurrentProcess() bool {
	hwnd, _, _ := procKeyboardGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}

	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid == windows.GetCurrentProcessId()
}
