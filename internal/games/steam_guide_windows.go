//go:build windows

package games

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Only the foreground client/webhelper UI of this verified installation is
// eligible. Keep tracker handles alive across samples to reject PID reuse.
type SteamGuideMonitor struct{ tracker *steamTracker }
type SteamGuideUI struct {
	PID       uint32
	Window    uintptr
	Title     string
	Minimized bool
}

func NewSteamGuideMonitor(steamExe string) (*SteamGuideMonitor, error) {
	var session uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &session); err != nil {
		return nil, err
	}
	return &SteamGuideMonitor{tracker: &steamTracker{exe: filepath.Clean(steamExe), sessionID: session, processes: map[uint32]*steamTrackedProcess{}}}, nil
}

func (m *SteamGuideMonitor) Close() { m.tracker.close() }

func (m *SteamGuideMonitor) Poll() (SteamGuideUI, error) {
	if _, _, err := m.tracker.sample(); err != nil {
		return SteamGuideUI{}, err
	}
	hwnd, _, _ := procSteamGetForegroundWindow.Call()
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	process := m.tracker.processes[pid]
	if process == nil || (!process.client && !strings.EqualFold(filepath.Base(process.path), "steamwebhelper.exe")) {
		return SteamGuideUI{}, nil
	}
	if err := process.refresh(); err != nil {
		return SteamGuideUI{}, err
	}
	if !process.alive {
		return SteamGuideUI{}, nil
	}
	var title [512]uint16
	n, _, _ := procSteamGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
	iconic, _, _ := procSteamIsIconic.Call(hwnd)
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if n == 0 && iconic == 0 && visible != 0 {
		return SteamGuideUI{}, nil // Untitled utility windows are not prompt targets.
	}
	return SteamGuideUI{PID: pid, Window: hwnd, Title: windows.UTF16ToString(title[:]), Minimized: iconic != 0 || visible == 0}, nil
}

// Hide only an iconic foreground window, never dismiss a visible firmware,
// sign-in or update prompt. Refresh owner and focus immediately before acting.
func (m *SteamGuideMonitor) HideMinimized(ui SteamGuideUI) error {
	if err := m.validateForeground(ui); err != nil {
		return err
	}
	iconic, _, _ := procSteamIsIconic.Call(ui.Window)
	if iconic != 0 {
		procSteamShowWindowAsync.Call(ui.Window, 0) // SW_HIDE; no close message or process termination.
	}
	return nil
}

// During an explicit controller capture, background the verified Steam client
// if its own Guide handler steals focus. Do not close it or accept any prompt.
func (m *SteamGuideMonitor) BackgroundForControllerTest(ui SteamGuideUI) error {
	if err := m.validateForeground(ui); err != nil {
		return err
	}
	procSteamShowWindowAsync.Call(ui.Window, 6) // SW_MINIMIZE; the user can reopen Steam normally.
	return nil
}

func (m *SteamGuideMonitor) validateForeground(ui SteamGuideUI) error {
	process := m.tracker.processes[ui.PID]
	if process == nil || process.refresh() != nil || !process.alive {
		return fmt.Errorf("Steam guide window owner changed")
	}
	foreground, _, _ := procSteamGetForegroundWindow.Call()
	var pid uint32
	procGetWindowThreadProcessID.Call(ui.Window, uintptr(unsafe.Pointer(&pid)))
	if foreground != ui.Window || pid != ui.PID {
		return fmt.Errorf("Steam guide foreground changed")
	}
	return nil
}
