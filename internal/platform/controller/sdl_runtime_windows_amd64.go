//go:build windows && amd64

package controller

import (
	"embed"
	cacheassets "launcher/internal/assets"
)

//go:embed sdl/windows_amd64/SDL3.dll sdl/windows_amd64/LICENSE.txt
var sdlRuntimeAssets embed.FS

func sdlRuntimePath() (string, error) {
	path, err := cacheassets.ExtractEmbeddedFile(sdlRuntimeAssets, "sdl/windows_amd64/SDL3.dll", "SDL3-3.4.18-amd64.dll")
	if err != nil {
		return "", err
	}
	if _, err := cacheassets.ExtractEmbeddedFile(sdlRuntimeAssets, "sdl/windows_amd64/LICENSE.txt", "SDL3-3.4.18-LICENSE.txt"); err != nil {
		return "", err
	}
	return path, nil
}
