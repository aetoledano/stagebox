package assets

import (
	"net/http"
	"path/filepath"
)

const ThumbnailURLPrefix = "/game-thumbs"

func ThumbnailCacheDir() string {
	dir := filepath.Join("cache", "thumbnails")
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

func ThumbnailHandler(thumbnailDir string) http.Handler {
	return http.StripPrefix(ThumbnailURLPrefix+"/", http.FileServer(http.Dir(thumbnailDir)))
}
