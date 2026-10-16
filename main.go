package main

import (
	"embed"
	cacheassets "launcher/internal/assets"
	"launcher/internal/jsonx"
	"launcher/internal/platform/controller"
	"launcher/internal/platform/display"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"launcher/internal/config"
	lgx "launcher/internal/logx"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/loading-space.png
var runtimeAssets embed.FS

var appVersion = "0.33"

func main() {
	defer lgx.RecoverPanic("startup")

	// load config
	config := config.NewConfig()
	configErr := jsonx.LoadJSONFile("config.json", &config)
	thumbnailDir := cacheassets.ThumbnailCacheDir()
	if err := os.MkdirAll(thumbnailDir, 0o755); err != nil {
		lgx.Errorf("failed to create thumbnail cache: %v", err)
	}

	if err := lgx.Init(true, "stagebox.log", true); err != nil {
		lgx.Errorf("failed to open log file at default path: %v", err)
	}
	lgx.SetLevel("debug")
	lgx.Infof("launcher starting version=%s pid=%d", appVersion, os.Getpid())
	lgx.Infof("logging level=%s; config.debug controls exit behavior, not logging verbosity", lgx.GetLevel())
	if configErr != nil {
		lgx.Errorf("failed to load config.json: %v", configErr)
	}
	lgx.Infof("config gamesRoot=%q debug=%v", config.GamesRoot, config.Debug)

	if changed, err := display.UsePrimaryMaxResolutionChanged(); err != nil {
		lgx.Warnf("display max-resolution setup failed before Wails start: %v", err)
	} else if changed {
		lgx.Info("display mode changed before Wails start; waiting for monitor settle")
		time.Sleep(3 * time.Second)
	} else {
		lgx.Debug("display already at max resolution before Wails start")
	}

	loadingWallpaperPath := "loading-space.png"
	if path, err := cacheassets.ExtractEmbeddedFile(runtimeAssets, "assets/loading-space.png", "loading-space.png"); err != nil {
		lgx.Errorf("failed to extract embedded loading wallpaper: %v", err)
	} else {
		loadingWallpaperPath = path
	}

	// Create an instance of the app structure
	app := NewApp(&config, thumbnailDir, cacheassets.ThumbnailURLPrefix, loadingWallpaperPath)

	// Create application with options
	err := wails.Run(&options.App{
		Title: "stagebox",
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: cacheassets.ThumbnailHandler(thumbnailDir),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		Windows: &windows.Options{
			WindowClassName: controller.LauncherWindowClass,
		},
		Bind: []any{
			app,
		},
	})

	if err != nil {
		lgx.Error(err)
	}
}
