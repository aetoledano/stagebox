//go:build windows

package tools

import (
	"errors"
	"math"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
StartWallpaper(imagePath, mode) starts a bottom-most native window that paints `imagePath`.
mode: "cover" (default) or "contain"
Returns a stop() you should call on exit.
*/
func StartWallpaper(imagePath string, mode string) (func(), error) {
	if imagePath == "" {
		return nil, errors.New("imagePath required")
	}
	abs, _ := filepath.Abs(imagePath)
	wimg, _ := windows.UTF16PtrFromString(abs)
	if mode != "contain" {
		mode = "cover"
	}

	var (
		procRegisterClassEx  = user32.NewProc("RegisterClassExW")
		procCreateWindowEx   = user32.NewProc("CreateWindowExW")
		procDefWindowProc    = user32.NewProc("DefWindowProcW")
		procShowWindow       = user32.NewProc("ShowWindow")
		procUpdateWindow     = user32.NewProc("UpdateWindow")
		procTranslateMessage = user32.NewProc("TranslateMessage")
		procDispatchMessage  = user32.NewProc("DispatchMessageW")
		procBeginPaint       = user32.NewProc("BeginPaint")
		procEndPaint         = user32.NewProc("EndPaint")
		procGetClientRect    = user32.NewProc("GetClientRect")
		procSetWindowPos     = user32.NewProc("SetWindowPos")
		procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
		procPostQuitMessage  = user32.NewProc("PostQuitMessage")
		procPeekMessage      = user32.NewProc("PeekMessageW")

		procGdiplusStartup     = gdiplus.NewProc("GdiplusStartup")
		procGdiplusShutdown    = gdiplus.NewProc("GdiplusShutdown")
		procGdipCreateFromHDC  = gdiplus.NewProc("GdipCreateFromHDC")
		procGdipDeleteGraphics = gdiplus.NewProc("GdipDeleteGraphics")
		procGdipLoadImage      = gdiplus.NewProc("GdipLoadImageFromFile")
		procGdipDisposeImage   = gdiplus.NewProc("GdipDisposeImage")
		procGdipGetWidth       = gdiplus.NewProc("GdipGetImageWidth")
		procGdipGetHeight      = gdiplus.NewProc("GdipGetImageHeight")
		procGdipDrawImageRect  = gdiplus.NewProc("GdipDrawImageRectI")

		procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
		procDestroyWindow   = user32.NewProc("DestroyWindow")
	)

	// —— constants
	const (
		WS_EX_TOOLWINDOW = 0x00000080
		WS_EX_NOACTIVATE = 0x08000000
		WS_POPUP         = 0x80000000
		WS_VISIBLE       = 0x10000000

		SW_SHOW     = 5
		SM_CXSCREEN = 0
		SM_CYSCREEN = 1

		WM_DESTROY           = 0x0002
		WM_PAINT             = 0x000F
		WM_WINDOWPOSCHANGING = 0x0046

		SWP_NOSIZE     = 0x0001
		SWP_NOMOVE     = 0x0002
		SWP_NOACTIVATE = 0x0010
		SWP_SHOWWINDOW = 0x0040

		PM_REMOVE = 0x0001
	)

	type (
		HWND = uintptr
		HDC  = uintptr
	)

	// WNDCLASSEX
	type wndClassEx struct {
		cbSize        uint32
		style         uint32
		lpfnWndProc   uintptr
		cbClsExtra    int32
		cbWndExtra    int32
		hInstance     windows.Handle
		hIcon         windows.Handle
		hCursor       windows.Handle
		hbrBackground windows.Handle
		lpszMenuName  *uint16
		lpszClassName *uint16
		hIconSm       windows.Handle
	}

	type paintStruct struct {
		hdc         HDC
		fErase      int32
		rcPaint     struct{ left, top, right, bottom int32 }
		fRestore    int32
		fIncUpdate  int32
		rgbReserved [32]byte
	}

	type rect struct{ left, top, right, bottom int32 }

	type windowPos struct {
		hwnd            HWND
		hwndInsertAfter HWND
		x, y, cx, cy    int32
		flags           uint32
	}

	// GDI+ structs
	type gdiplusStartupInput struct {
		GdiplusVersion           uint32
		DebugEventCallback       uintptr
		SuppressBackgroundThread bool
		SuppressExternalCodecs   bool
	}
	type (
		GpImage    = uintptr
		GpGraphics = uintptr
	)

	var (
		hwnd       HWND
		token      uintptr
		img        GpImage
		imgW, imgH uint32
		wg         sync.WaitGroup
		stopCh     = make(chan struct{})
	)

	// init GDI+
	var si gdiplusStartupInput
	si.GdiplusVersion = 1
	if r, _, _ := procGdiplusStartup.Call(uintptr(unsafe.Pointer(&token)), uintptr(unsafe.Pointer(&si)), 0); r != 0 {
		return nil, errors.New("GdiplusStartup failed")
	}
	// load image
	if r, _, _ := procGdipLoadImage.Call(uintptr(unsafe.Pointer(wimg)), uintptr(unsafe.Pointer(&img))); r != 0 || img == 0 {
		procGdiplusShutdown.Call(token)
		return nil, errors.New("GdipLoadImageFromFile failed")
	}
	procGdipGetWidth.Call(img, uintptr(unsafe.Pointer(&imgW)))
	procGdipGetHeight.Call(img, uintptr(unsafe.Pointer(&imgH)))

	// wndproc
	wndProc := windows.NewCallback(func(h HWND, msg uint32, wparam, lparam uintptr) uintptr {
		switch msg {
		case WM_WINDOWPOSCHANGING:
			// ensure we stay at bottom
			wp := (*windowPos)(unsafe.Pointer(lparam))
			wp.hwndInsertAfter = 1 // HWND_BOTTOM == 1
			wp.flags |= SWP_NOACTIVATE
			return 0
		case WM_PAINT:
			var ps paintStruct
			r1, _, _ := procBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			hdc := HDC(r1)
			if hdc != 0 && img != 0 {
				var cr rect
				procGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
				cw := int(cr.right - cr.left)
				ch := int(cr.bottom - cr.top)

				// compute dest rect (cover/contain)
				srcW := int(imgW)
				srcH := int(imgH)
				var dw, dh int
				if mode == "contain" {
					scale := math.Min(float64(cw)/float64(srcW), float64(ch)/float64(srcH))
					dw = int(float64(srcW) * scale)
					dh = int(float64(srcH) * scale)
				} else { // cover
					scale := math.Max(float64(cw)/float64(srcW), float64(ch)/float64(srcH))
					dw = int(float64(srcW) * scale)
					dh = int(float64(srcH) * scale)
				}
				dx := (cw - dw) / 2
				dy := (ch - dh) / 2

				var g GpGraphics
				procGdipCreateFromHDC.Call(uintptr(hdc), uintptr(unsafe.Pointer(&g)))
				if g != 0 {
					procGdipDrawImageRect.Call(uintptr(g), img, uintptr(int32(dx)), uintptr(int32(dy)), uintptr(int32(dw)), uintptr(int32(dh)))
					procGdipDeleteGraphics.Call(uintptr(g))
				}
			}
			procEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			return 0
		case WM_DESTROY:
			procPostQuitMessage.Call(0)
			return 0
		}
		r, _, _ := procDefWindowProc.Call(h, uintptr(msg), wparam, lparam)
		return r
	})

	clsName, _ := windows.UTF16PtrFromString("LauncherWallpaperWnd")

	// register class
	var hInst windows.Handle
	if r1, _, _ := procGetModuleHandle.Call(0); r1 != 0 {
		hInst = windows.Handle(r1)
	}

	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   wndProc,
		hInstance:     hInst,
		lpszClassName: clsName,
	}
	if r, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		procGdipDisposeImage.Call(img)
		procGdiplusShutdown.Call(token)
		return nil, errors.New("RegisterClassEx failed")
	}

	// screen size
	cx, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	cy, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)

	// create bottom-most popup window (no taskbar, no focus)
	const exStyle = WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE
	const style = WS_POPUP | WS_VISIBLE
	h, _, _ := procCreateWindowEx.Call(
		exStyle,
		uintptr(unsafe.Pointer(clsName)),
		0,
		style,
		0, 0, cx, cy,
		0, 0,
		uintptr(hInst), 0,
	)
	if h == 0 {
		procGdipDisposeImage.Call(img)
		procGdiplusShutdown.Call(token)
		return nil, errors.New("CreateWindowEx failed")
	}
	hwnd = h

	// shove to bottom, show
	procSetWindowPos.Call(hwnd, 1 /*HWND_BOTTOM*/, 0, 0, uintptr(cx), uintptr(cy), SWP_NOACTIVATE|SWP_SHOWWINDOW)
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

	// message loop in a dedicated OS thread
	wg.Add(1)
	go func() {
		runtime.LockOSThread()
		defer wg.Done()

		var msg struct {
			hwnd   HWND
			msg    uint32
			wparam uintptr
			lparam uintptr
			time   uint32
			pt     struct{ x, y int32 }
		}
		for {
			select {
			case <-stopCh:
				// destroy window -> WM_DESTROY will quit loop
				procDestroyWindow.Call(hwnd)
				return
			default:
			}
			r, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, PM_REMOVE)
			if r != 0 {
				if msg.msg == 0 { // WM_NULL after quit
					break
				}
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
				procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
			} else {
				// idle: small sleep avoids burning CPU
				windows.SleepEx(8, false)
			}
		}
	}()

	// return stop()
	stop := func() {
		close(stopCh)
		wg.Wait()
		if img != 0 {
			procGdipDisposeImage.Call(img)
			img = 0
		}
		if token != 0 {
			procGdiplusShutdown.Call(token)
			token = 0
		}
	}
	return stop, nil
}
