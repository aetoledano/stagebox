//go:build windows

package controller

import (
	"embed"
	cacheassets "launcher/internal/assets"
)

// Keep StageBox additions separate from SDL's built-in database. User databases
// load afterward, with the assets database taking final precedence.
//
//go:embed sdl/gamecontrollerdb.txt sdl/gamecontrollerdb-LICENSE.txt
var sdlMappingAssets embed.FS

func bundledSDLMappingPath() (string, error) {
	path, err := cacheassets.ExtractEmbeddedFile(sdlMappingAssets, "sdl/gamecontrollerdb.txt", "stagebox-gamecontrollerdb-v1.txt")
	if err != nil {
		return "", err
	}
	if _, err := cacheassets.ExtractEmbeddedFile(sdlMappingAssets, "sdl/gamecontrollerdb-LICENSE.txt", "stagebox-gamecontrollerdb-v1-LICENSE.txt"); err != nil {
		return "", err
	}
	return path, nil
}
