//go:build windows

package winapi

import "golang.org/x/sys/windows"

var (
	User32   = windows.NewLazySystemDLL("user32.dll")
	Kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	GDIPlus  = windows.NewLazySystemDLL("gdiplus.dll")
)
