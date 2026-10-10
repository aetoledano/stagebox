package games

import (
	"context"
	"fmt"
	"image"
	"io"
	lgx "launcher/internal/logx"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const steamArtworkMaxBytes = 8 << 20

func prepareSteamArtwork(game *GameData, opts LoadOptions) {
	if opts.ThumbnailDir == "" || opts.ThumbnailURLPrefix == "" {
		return
	}
	if game.Image != "" {
		if err := setSteamThumbnail(game, game.Image, opts); err == nil {
			return
		} else {
			lgx.Warnf("Steam custom artwork unavailable path=%q: %v; using Steam artwork", game.Image, err)
		}
	}
	cache := filepath.Join(filepath.Dir(game.Executable), "appcache", "librarycache")
	for _, path := range steamArtworkCandidates(cache, game.SteamAppID) {
		if setSteamThumbnail(game, path, opts) == nil {
			return
		}
	}
	path, err := ensureSteamArtwork(game.SteamAppID, filepath.Join(opts.ThumbnailDir, "steam-artwork"))
	if err == nil {
		err = setSteamThumbnail(game, path, opts)
	}
	if err != nil {
		// A missing image must never remove an installed game from the library.
		lgx.Warnf("Steam artwork unavailable appID=%d name=%q err=%v", game.SteamAppID, game.Name, err)
	}
}

func setSteamThumbnail(game *GameData, path string, opts LoadOptions) error {
	if err := validateSteamArtwork(path); err != nil {
		return err
	}
	thumb, err := EnsureThumbnail(game.ID, path, opts.ThumbnailDir, opts.ThumbnailURLPrefix)
	if err != nil {
		return err
	}
	game.Image, game.ImageHash, game.ThumbnailURL = path, thumb.Hash, thumb.URL
	return nil
}

func steamArtworkCandidates(cache string, appID uint32) []string {
	id := strconv.FormatUint(uint64(appID), 10)
	candidates := []string{}
	for _, suffix := range []string{"_library_header.jpg", "_header.jpg", "_library_hero.jpg", "_library_600x900.jpg", "_library_600x900_2x.jpg"} {
		path := filepath.Join(cache, id+suffix)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			candidates = append(candidates, path)
		}
	}
	// New clients also cache artwork as <appID>/<content-hash>/<asset>.jpg.
	root := filepath.Join(cache, id)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			if strings.Count(relative, string(filepath.Separator)) >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		if steamArtworkPriority(path) < 100 {
			candidates = append(candidates, path)
		}
		return nil
	})
	sort.SliceStable(candidates, func(i, j int) bool { return steamArtworkPriority(candidates[i]) < steamArtworkPriority(candidates[j]) })
	return candidates
}

func steamArtworkPriority(path string) int {
	name := strings.ToLower(filepath.Base(path))
	for index, suffix := range []string{"library_header.jpg", "header.jpg", "library_hero.jpg", "library_600x900_2x.jpg", "library_600x900.jpg", "library_capsule.jpg"} {
		if name == suffix || strings.HasSuffix(name, "_"+suffix) {
			return index
		}
	}
	return 100
}

func validateSteamArtwork(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > steamArtworkMaxBytes {
		return fmt.Errorf("Steam artwork exceeds size limit")
	}
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 16384 || config.Height > 16384 || int64(config.Width)*int64(config.Height) > 32<<20 {
		return fmt.Errorf("invalid Steam artwork dimensions")
	}
	return nil
}

func ensureSteamArtwork(appID uint32, cache string) (string, error) {
	path := filepath.Join(cache, fmt.Sprintf("%d-header.jpg", appID))
	if validateSteamArtwork(path) == nil {
		return path, nil
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/header.jpg", appID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Steam artwork download: %s", response.Status)
	}
	out, err := os.CreateTemp(cache, ".steam-artwork-*.jpg")
	if err != nil {
		return "", err
	}
	temporary := out.Name()
	defer os.Remove(temporary)
	size, copyErr := io.Copy(out, io.LimitReader(response.Body, steamArtworkMaxBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if size > steamArtworkMaxBytes {
		return "", fmt.Errorf("Steam artwork download exceeds size limit")
	}
	if err := validateSteamArtwork(temporary); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(temporary, path); err != nil {
		return "", err
	}
	return path, nil
}
