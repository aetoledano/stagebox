//go:build windows

package tools

import "golang.org/x/sys/windows"

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdiplus  = windows.NewLazySystemDLL("gdiplus.dll")
)
