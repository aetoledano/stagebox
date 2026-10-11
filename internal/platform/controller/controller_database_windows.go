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
	path string
	data []byte
}

func controllerDatabasePaths() ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(exe)
	// The requested assets file is authoritative. Preserve support for the
	// old executable-adjacent override, loading it before the assets file.
	return []string{filepath.Join(dir, "gamecontrollerdb.txt"), filepath.Join(dir, "assets", "gamecontrollerdb.txt")}, nil
}

func ensureControllerDatabase(ctx context.Context) {
	paths, err := controllerDatabasePaths()
	if err != nil {
		lgx.Warnf("resolve controller database download path: %v", err)
		return
	}
	path := paths[len(paths)-1]
	lgx.Infof("controller database download enabled path=%q", path)
	if err := downloadControllerDatabase(ctx, path, controllerDatabaseURL, &http.Client{Timeout: 30 * time.Second}); err != nil {
		lgx.Warnf("controller database download failed path=%q err=%v; keeping local/bundled mappings", path, err)
		return
	}
	if err := writeControllerDatabaseLicense(filepath.Dir(path)); err != nil {
		lgx.Warnf("write controller database license: %v", err)
	}
}

func writeControllerDatabaseLicense(dir string) error {
	data, err := sdlMappingAssets.ReadFile("sdl/gamecontrollerdb-LICENSE.txt")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, "gamecontrollerdb-LICENSE.txt"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(file.Name())
		return writeErr
	}
	return closeErr
}

// Download only a missing database. Moving the complete temporary file without
// REPLACE_EXISTING also protects a file created manually during the request.
func downloadControllerDatabase(ctx context.Context, path, url string, client *http.Client) error {
	if _, err := os.Stat(path); err == nil {
		lgx.Infof("controller database already exists; preserving edits path=%q", path)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
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
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := windows.UTF16PtrFromString(file.Name())
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(src, dst, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		if _, existing := os.Stat(path); existing == nil {
			lgx.Infof("controller database appeared during download; preserving edits path=%q", path)
			return nil
		}
		return err
	}
	lgx.Infof("controller database downloaded path=%q windowsMappings=%d bytes=%d source=%s", path, count, len(data), url)
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
	for _, path := range paths {
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
		files = append(files, controllerMappingFile{path: path, data: data})
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return files, digest, nil
}
