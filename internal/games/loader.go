package games

import (
	"launcher/internal/jsonx"
	lgx "launcher/internal/logx"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const DefaultLoadWorkers = 8

type LoadOptions struct {
	SteamExecutable    string
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
	gameDirs := []string{}
	if rootPath != "" {
		rootEntries, err := os.ReadDir(rootPath)
		if err != nil {
			lgx.Errorf("error reading game root %q: %v", rootPath, err)
			if opts.OnError != nil {
				opts.OnError(rootPath, err)
			}
		} else {
			for _, e := range rootEntries {
				if e.IsDir() && !strings.EqualFold(e.Name(), "SteamGames") {
					gameDirs = append(gameDirs, filepath.Join(rootPath, e.Name()))
				}
			}
		}
	}
	steamLibrary := discoverSteamLibrary(opts.SteamExecutable, opts.OnError)
	syncSteamConfigs(rootPath, steamLibrary, opts.OnError)
	steamGames := steamLibrary.games
	total := len(gameDirs) + len(steamGames)
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
	type loadJob struct {
		path  string
		steam *GameData
	}
	jobs := make(chan loadJob)
	var wg sync.WaitGroup

	reportDone := func() {
		progressMu.Lock()
		loaded++
		cur := loaded
		if opts.OnProgress != nil {
			opts.OnProgress(cur, total)
		}
		progressMu.Unlock()
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				gamePath := job.path
				var gd *GameData
				var err error
				if job.steam != nil {
					gd = job.steam
					prepareSteamArtwork(gd, opts)
				} else {
					gd, err = loadGame(gamePath, opts)
				}
				if err != nil {
					lgx.Errorf("error loading game from %q: %v", gamePath, err)
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
		jobs <- loadJob{path: gamePath}
	}
	for _, game := range steamGames {
		jobs <- loadJob{steam: game}
	}
	close(jobs)
	wg.Wait()

	return games
}

func loadGame(gamePath string, opts LoadOptions) (*GameData, error) {
	lcPath := filepath.Join(gamePath, "lc.json")
	gd := NewGameData()
	if err := jsonx.LoadJSONFile(lcPath, &gd); err != nil {
		return nil, err
	}
	gd.nativeConfigPath = lcPath
	if gd.ID == uuid.Nil {
		gd.ID = deterministicGameID(gamePath, gd.Name)
	}

	gd.Image = filepath.Join(gamePath, gd.Image)
	gd.Executable = filepath.Join(gamePath, gd.Executable)
	if gd.StartsOn == "" {
		gd.StartsOn = gamePath
	} else if !filepath.IsAbs(gd.StartsOn) {
		gd.StartsOn = filepath.Join(gamePath, gd.StartsOn)
	}

	if opts.ThumbnailDir != "" && opts.ThumbnailURLPrefix != "" && gd.Image != "" {
		thumb, err := EnsureThumbnail(gd.ID, gd.Image, opts.ThumbnailDir, opts.ThumbnailURLPrefix)
		if err != nil {
			lgx.Errorf("error generating thumbnail for %q: %v", gd.Image, err)
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
