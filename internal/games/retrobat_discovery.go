package games

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrRetroBatNotFound = errors.New("RetroBat is not installed at the selected path")

// Explicit settings are authoritative: a typo must not launch a different installation.
func FindRetroBatExecutable(configuredPath string) (string, error) {
	environmentPath := os.Getenv("RETROBAT_PATH")
	if strings.TrimSpace(configuredPath) != "" || strings.TrimSpace(environmentPath) != "" {
		return findRetroBatExecutable(configuredPath, environmentPath, nil)
	}
	candidates := retroBatPlatformPaths()
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	candidates = append(candidates, filepath.Join(drive+`\`, "RetroBat"))
	for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if dir := os.Getenv(variable); dir != "" {
			candidates = append(candidates, filepath.Join(dir, "RetroBat"))
		}
	}
	if launcher, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(launcher), "RetroBat"))
	}
	if path, err := exec.LookPath("RetroBat.exe"); err == nil {
		candidates = append(candidates, path)
	}
	return findRetroBatExecutable(configuredPath, environmentPath, candidates)
}

func findRetroBatExecutable(configuredPath, environmentPath string, candidates []string) (string, error) {
	if path := strings.TrimSpace(configuredPath); path != "" {
		return retroBatExecutableAt(path)
	}
	if path := strings.TrimSpace(environmentPath); path != "" {
		return retroBatExecutableAt(path)
	}
	seen := make(map[string]bool)
	for _, path := range candidates {
		if strings.TrimSpace(path) == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] {
			continue
		}
		seen[key] = true
		if exe, err := retroBatExecutableAt(path); err == nil {
			return exe, nil
		}
	}
	return "", ErrRetroBatNotFound
}

// Registry icon/uninstall values can contain a quoted executable followed by
// an icon index or command arguments. Use its directory, never execute it.
func retroBatRegistryExecutablePath(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) {
		if end := strings.Index(value[1:], `"`); end >= 0 {
			return value[1 : end+1]
		}
		return ""
	}
	if end := strings.Index(strings.ToLower(value), ".exe"); end >= 0 {
		return value[:end+4]
	}
	return ""
}

func retroBatExecutableAt(path string) (string, error) {
	path = os.ExpandEnv(strings.Trim(strings.TrimSpace(path), `"`))
	if info, err := os.Stat(path); err == nil && info.IsDir() || os.IsNotExist(err) && !strings.EqualFold(filepath.Ext(path), ".exe") {
		path = filepath.Join(path, "RetroBat.exe")
	}
	exe, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve RetroBat path: %w", err)
	}
	if !strings.EqualFold(filepath.Base(exe), "RetroBat.exe") {
		return "", fmt.Errorf("retrobatPath must point to a RetroBat installation folder or RetroBat.exe: %s", exe)
	}
	if info, err := os.Stat(exe); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrRetroBatNotFound, exe)
		}
		return "", fmt.Errorf("RetroBat executable is unavailable at %s: %w", exe, err)
	} else if !info.Mode().IsRegular() {
		return "", fmt.Errorf("RetroBat executable is not a regular file: %s", exe)
	}
	return exe, nil
}
