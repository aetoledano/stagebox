package assets

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
)

func RuntimeAssetCacheDir() string {
	dir := filepath.Join("cache", "assets")
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

func ExtractEmbeddedFile(fsys fs.FS, sourcePath string, targetName string) (string, error) {
	data, err := fs.ReadFile(fsys, sourcePath)
	if err != nil {
		return "", err
	}

	dir := RuntimeAssetCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	target := filepath.Join(dir, filepath.Base(targetName))
	if current, err := os.ReadFile(target); err == nil && bytes.Equal(current, data) {
		return target, nil
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", err
	}
	return target, nil
}
