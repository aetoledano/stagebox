//go:build !windows

package games

import "fmt"

type SteamGuideMonitor struct{}
type SteamGuideUI struct {
	PID       uint32
	Window    uintptr
	Title     string
	Minimized bool
}

func NewSteamGuideMonitor(string) (*SteamGuideMonitor, error) {
	return nil, fmt.Errorf("Steam guide UI requires Windows")
}
func (*SteamGuideMonitor) Close()                           {}
func (*SteamGuideMonitor) Poll() (SteamGuideUI, error)      { return SteamGuideUI{}, nil }
func (*SteamGuideMonitor) HideMinimized(SteamGuideUI) error { return nil }
