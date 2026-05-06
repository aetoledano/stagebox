package games

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gen2brain/avif"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	thumbnailMaxDimension = 512
	thumbnailJPEGQuality  = 90
)

type ThumbnailResult struct {
	Hash string
	URL  string
}

func EnsureThumbnail(gameID uuid.UUID, imagePath, cacheDir, urlPrefix string) (ThumbnailResult, error) {
	if imagePath == "" {
		return ThumbnailResult{}, nil
	}

	hash, err := fileHash(imagePath)
	if err != nil {
		return ThumbnailResult{}, err
	}

	shortHash := hash[:16]
	filename := fmt.Sprintf("%s-%s.jpg", gameID.String(), shortHash)
	targetPath := filepath.Join(cacheDir, filename)
	result := ThumbnailResult{
		Hash: shortHash,
		URL:  strings.TrimRight(urlPrefix, "/") + "/" + filename,
	}

	if _, err := os.Stat(targetPath); err == nil {
		return result, nil
	} else if !os.IsNotExist(err) {
		return ThumbnailResult{}, err
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return ThumbnailResult{}, err
	}

	img, err := decodeImage(imagePath)
	if err != nil {
		return ThumbnailResult{}, err
	}

	thumb := resizeToMax(img, thumbnailMaxDimension)
	tmp, err := os.CreateTemp(cacheDir, filename+".*.tmp")
	if err != nil {
		return ThumbnailResult{}, err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := jpeg.Encode(tmp, thumb, &jpeg.Options{Quality: thumbnailJPEGQuality}); err != nil {
		_ = tmp.Close()
		return ThumbnailResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return ThumbnailResult{}, err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return ThumbnailResult{}, err
	}
	ok = true

	_ = removeStaleThumbnails(cacheDir, gameID, filename)
	return result, nil
}

func fileHash(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func decodeImage(filename string) (image.Image, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	switch strings.ToLower(filepath.Ext(filename)) {
	case ".webp":
		return webp.Decode(f)
	case ".avif", ".avifs":
		return avif.Decode(f)
	default:
		img, _, err := image.Decode(f)
		return img, err
	}
}

func resizeToMax(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	if w <= maxDim && h <= maxDim {
		return src
	}

	scale := float64(maxDim) / float64(w)
	if h > w {
		scale = float64(maxDim) / float64(h)
	}
	nw := max(1, int(float64(w)*scale+0.5))
	nh := max(1, int(float64(h)*scale+0.5))

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

func removeStaleThumbnails(cacheDir string, gameID uuid.UUID, keep string) error {
	matches, err := filepath.Glob(filepath.Join(cacheDir, gameID.String()+"-*.jpg"))
	if err != nil {
		return err
	}
	keepPath := filepath.Join(cacheDir, keep)
	for _, match := range matches {
		if match == keepPath {
			continue
		}
		_ = os.Remove(match)
	}
	return nil
}
