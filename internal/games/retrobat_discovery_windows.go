//go:build windows

package games

import (
	lgx "launcher/internal/logx"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Read fresh installation records on every activation: installing RetroBat
// while StageBox is open must not require a restart or another download.
func retroBatPlatformPaths() []string {
	paths := retroBatRegistryPaths()
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		lgx.Warnf("enumerate RetroBat drive locations: %v", err)
		return paths
	}
	for index := uint32(0); index < 26; index++ {
		if mask&(1<<index) == 0 {
			continue
		}
		root := string(rune('A'+index)) + `:\`
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		driveType := windows.GetDriveType(rootPtr)
		if driveType == windows.DRIVE_FIXED || driveType == windows.DRIVE_REMOVABLE {
			paths = append(paths, filepath.Join(root, "RetroBat"))
		}
	}
	return paths
}

func retroBatRegistryPaths() []string {
	var paths []string
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			access := registry.READ | view
			if key, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\App Paths\RetroBat.exe`, access); err == nil {
				if path := retroBatRegistryString(key, ""); path != "" {
					paths = append(paths, path)
				}
				_ = key.Close()
			}
			uninstall, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, access)
			if err != nil {
				continue
			}
			names, err := uninstall.ReadSubKeyNames(-1)
			if err != nil {
				_ = uninstall.Close()
				continue
			}
			slices.Sort(names)
			for _, name := range names {
				key, err := registry.OpenKey(uninstall, name, access)
				if err != nil {
					continue
				}
				displayName := strings.ToLower(strings.TrimSpace(retroBatRegistryString(key, "DisplayName")))
				if displayName == "retrobat" || strings.HasPrefix(displayName, "retrobat ") || strings.HasPrefix(displayName, "retrobat-") {
					if path := retroBatRegistryString(key, "InstallLocation"); path != "" {
						paths = append(paths, path)
					}
					for _, field := range []string{"DisplayIcon", "UninstallString"} {
						if exe := retroBatRegistryExecutablePath(retroBatRegistryString(key, field)); exe != "" {
							paths = append(paths, filepath.Dir(exe))
						}
					}
				}
				_ = key.Close()
			}
			_ = uninstall.Close()
		}
	}
	return paths
}

func retroBatRegistryString(key registry.Key, name string) string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	if expanded, err := registry.ExpandString(value); err == nil {
		value = expanded
	}
	return strings.TrimSpace(value)
}
