package games

import (
	"launcher/tools"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const DefaultLoadWorkers = 8

type LoadOptions struct {
	ThumbnailDir       string
	ThumbnailURLPrefix string
	Workers            int
	OnGame             func(*GameData)
	OnProgress         func(loaded int, total int)
	OnError            func(gamePath string, err error)
}

func LoadGames(rootPath string) map[uuid.UUID]*GameData {
	return LoadGamesWithOptions(rootPath, LoadOptions{})
}

func LoadGamesWithOptions(rootPath string, opts LoadOptions) map[uuid.UUID]*GameData {
	rootEntries, err := os.ReadDir(rootPath)
	if err != nil {
		log.Printf("Error reading game root %s: %v", rootPath, err)
		if opts.OnError != nil {
			opts.OnError(rootPath, err)
		}
		if opts.OnProgress != nil {
			opts.OnProgress(0, 0)
		}
		return map[uuid.UUID]*GameData{}
	}

	gameDirs := make([]string, 0, len(rootEntries))
	for _, e := range rootEntries {
		if e.IsDir() {
			gameDirs = append(gameDirs, filepath.Join(rootPath, e.Name()))
		}
	}

	total := len(gameDirs)
	if opts.OnProgress != nil {
		opts.OnProgress(0, total)
	}

	workers := opts.Workers
	if workers <= 0 || workers > DefaultLoadWorkers {
		workers = DefaultLoadWorkers
	}
	if total > 0 && workers > total {
		workers = total
	}

	games := map[uuid.UUID]*GameData{}
	var gamesMu sync.Mutex
	var progressMu sync.Mutex
	loaded := 0
	jobs := make(chan string)
	var wg sync.WaitGroup

	reportDone := func() {
		progressMu.Lock()
		loaded++
		cur := loaded
		progressMu.Unlock()
		if opts.OnProgress != nil {
			opts.OnProgress(cur, total)
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for gamePath := range jobs {
				gd, err := loadGame(gamePath, opts)
				if err != nil {
					log.Printf("Error loading game from %s: %v", gamePath, err)
					if opts.OnError != nil {
						opts.OnError(gamePath, err)
					}
					reportDone()
					continue
				}

				gamesMu.Lock()
				games[gd.ID] = gd
				gamesMu.Unlock()

				if opts.OnGame != nil {
					opts.OnGame(gd)
				}
				reportDone()
			}
		}()
	}

	for _, gamePath := range gameDirs {
		jobs <- gamePath
	}
	close(jobs)
	wg.Wait()

	return games
}

func loadGame(gamePath string, opts LoadOptions) (*GameData, error) {
	lcPath := filepath.Join(gamePath, "lc.json")
	gd := NewGameData()
	if err := tools.LoadJSONFile(lcPath, &gd); err != nil {
		return nil, err
	}
	if gd.ID == uuid.Nil {
		gd.ID = deterministicGameID(gamePath, gd.Name)
	}

	gd.Image = filepath.Join(gamePath, gd.Image)
	gd.Executable = filepath.Join(gamePath, gd.Executable)
	gd.StartsOn = gamePath

	if opts.ThumbnailDir != "" && opts.ThumbnailURLPrefix != "" && gd.Image != "" {
		thumb, err := EnsureThumbnail(gd.ID, gd.Image, opts.ThumbnailDir, opts.ThumbnailURLPrefix)
		if err != nil {
			log.Printf("Error generating thumbnail for %s: %v", gd.Image, err)
		} else {
			gd.ImageHash = thumb.Hash
			gd.ThumbnailURL = thumb.URL
		}
	}

	return &gd, nil
}

func deterministicGameID(gamePath string, name string) uuid.UUID {
	key := strings.TrimSpace(strings.ToLower(name))
	if key == "" {
		key = strings.TrimSpace(strings.ToLower(filepath.Base(gamePath)))
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("launcher/game/"+key))
}
