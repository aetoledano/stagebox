//go:build windows

package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	lgx "launcher/internal/logx"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

const (
	controllerDatabaseURL   = "https://raw.githubusercontent.com/mdqinc/SDL_GameControllerDB/master/gamecontrollerdb.txt"
	controllerDatabaseLimit = 8 << 20
)

type controllerMappingFile struct {
	path   string
	data   []byte
	source string
}

func controllerDatabasePaths() ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(exe)
	return controllerDatabasePathsIn(dir), nil
}

func controllerDatabasePathsIn(dir string) []string {
	return []string{
		filepath.Join(dir, "assets", "gamecontrollerdb.txt"),
		filepath.Join(dir, "gamecontrollerdb.txt"), // Existing legacy overrides.
		filepath.Join(dir, "assets", "gamecontrollerdb-overrides.txt"),
	}
}

func ensureControllerDatabase(ctx context.Context, download bool) {
	paths, err := controllerDatabasePaths()
	if err != nil {
		lgx.Warnf("resolve controller database download path: %v", err)
		return
	}
	path := paths[0]
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		lgx.Warnf("prepare controller database assets: %v", err)
		return
	}
	if err := migrateControllerDatabaseBackup(dir); err != nil {
		lgx.Warnf("rename controller database backup: %v", err)
	}
	overrides := []byte("# StageBox manual controller overrides. This file is never downloaded or overwritten.\n# Add SDL mapping rows here with platform:Windows; valid edits reload automatically.\n# The complete upstream database is kept separately in gamecontrollerdb.txt.\n")
	if err := publishControllerMappingFile(paths[2], overrides, false); err != nil {
		lgx.Warnf("prepare controller overrides: %v", err)
	}
	if err := writeControllerDatabaseLicense(dir); err != nil {
		lgx.Warnf("write controller database license: %v", err)
	}
	if !download {
		lgx.Info("controller database startup refresh disabled; using cached database and overrides")
		return
	}
	lgx.Infof("controller database background refresh started path=%q", path)
	if err := downloadControllerDatabase(ctx, path, controllerDatabaseURL, &http.Client{Timeout: 30 * time.Second}); err != nil {
		lgx.Warnf("controller database download failed path=%q err=%v; keeping local/bundled mappings", path, err)
		return
	}
}

func writeControllerDatabaseLicense(dir string) error {
	file, err := os.OpenFile(filepath.Join(dir, "gamecontrollerdb-LICENSE.txt"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(sdlDatabaseLicense)
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(file.Name())
		return writeErr
	}
	return closeErr
}

// Refresh the managed base database on every startup; user corrections belong
// to the separately watched overrides file and are never downloaded over.
func downloadControllerDatabase(ctx context.Context, path, url string, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "StageBox-controller-database")
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := readControllerMappingData(response.Body)
	if err != nil {
		return err
	}
	if response.ContentLength >= 0 && int64(len(data)) != response.ContentLength {
		return fmt.Errorf("incomplete controller database download")
	}
	count, err := validateControllerMappingData(data)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("download contains no Windows controller mappings")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if current, err := os.Open(path); err == nil {
		oldData, readErr := readControllerMappingData(current)
		current.Close()
		if readErr != nil {
			return fmt.Errorf("read previous controller database: %w", readErr)
		}
		if bytes.Equal(oldData, data) {
			lgx.Infof("controller database already current path=%q windowsMappings=%d", path, count)
			return nil
		}
		// Retain the first replaced local file so edits made under the previous
		// shared-base policy can be moved into the dedicated overrides file.
		backup := filepath.Join(filepath.Dir(path), "gamecontrollerdb-backup.txt")
		if err := publishControllerMappingFile(backup, oldData, false); err != nil {
			return fmt.Errorf("back up previous controller database: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishControllerMappingFile(path, data, true); err != nil {
		return err
	}
	lgx.Infof("controller database downloaded path=%q windowsMappings=%d bytes=%d source=%s", path, count, len(data), url)
	return nil
}

func migrateControllerDatabaseBackup(dir string) error {
	path := filepath.Join(dir, "gamecontrollerdb-backup.txt")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	legacy := filepath.Join(dir, "gamecontrollerdb-before-refresh.txt")
	file, err := os.Open(legacy)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := readControllerMappingData(file)
	file.Close()
	if err != nil {
		return err
	}
	if err := publishControllerMappingFile(path, data, false); err != nil {
		return err
	}
	// Another process may have created the new name meanwhile. Remove the old
	// file only when the published backup is byte-for-byte the same snapshot.
	current, err := os.Open(path)
	if err != nil {
		return err
	}
	published, err := readControllerMappingData(current)
	current.Close()
	if err != nil {
		return err
	}
	if bytes.Equal(published, data) {
		return os.Remove(legacy)
	}
	return nil
}

func publishControllerMappingFile(path string, data []byte, replace bool) error {
	if !replace {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".gamecontrollerdb-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	src, err := windows.UTF16PtrFromString(file.Name())
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if replace {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	if err := windows.MoveFileEx(src, dst, flags); err != nil {
		if !replace {
			if _, existing := os.Stat(path); existing == nil {
				return nil
			}
		}
		return err
	}
	return nil
}

func readControllerMappingData(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, controllerDatabaseLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > controllerDatabaseLimit {
		return nil, fmt.Errorf("controller database exceeds %d bytes", controllerDatabaseLimit)
	}
	return data, nil
}

// Validate text/row structure before passing any changed file to SDL. Blank
// files are valid local overrides (clear all custom entries), not downloads.
func validateControllerMappingData(data []byte) (int, error) {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return 0, fmt.Errorf("controller database must be UTF-8 text without NUL bytes")
	}
	count := 0
	for i, raw := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 4 || strings.TrimSpace(fields[1]) == "" {
			return 0, fmt.Errorf("invalid controller mapping row %d", i+1)
		}
		// Upstream includes SDL's special "xinput" GUID for generic XInput
		// devices in addition to normal 32-digit hardware GUIDs.
		if fields[0] != "xinput" {
			guid, err := hex.DecodeString(fields[0])
			if err != nil || len(guid) != 16 {
				return 0, fmt.Errorf("invalid controller GUID at row %d", i+1)
			}
		}
		platform := false
		for _, field := range fields[2:] {
			if field == "platform:Windows" {
				count++
			}
			if strings.HasPrefix(field, "platform:") {
				platform = true
			}
			if field != "" && !strings.Contains(field, ":") {
				return 0, fmt.Errorf("invalid controller binding at row %d", i+1)
			}
		}
		if !platform {
			return 0, fmt.Errorf("controller mapping row %d requires platform:Windows (or its source platform)", i+1)
		}
	}
	return count, nil
}

func readControllerMappingFiles(paths []string) ([]controllerMappingFile, [32]byte, error) {
	files := make([]controllerMappingFile, 0, len(paths))
	hash := sha256.New()
	for index, path := range paths {
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			hash.Write([]byte{0})
			continue
		}
		if err != nil {
			return nil, [32]byte{}, fmt.Errorf("read mappings %q: %w", path, err)
		}
		data, err := readControllerMappingData(file)
		file.Close()
		if err != nil {
			return nil, [32]byte{}, fmt.Errorf("read mappings %q: %w", path, err)
		}
		if _, err := validateControllerMappingData(data); err != nil {
			return nil, [32]byte{}, fmt.Errorf("validate mappings %q: %w", path, err)
		}
		hash.Write([]byte{1})
		sum := sha256.Sum256(data)
		hash.Write(sum[:])
		source := "overrides"
		if index == 0 {
			source = "database"
		}
		files = append(files, controllerMappingFile{path: path, data: data, source: source})
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return files, digest, nil
}
