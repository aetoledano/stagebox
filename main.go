package main

import (
	"embed"
	cacheassets "launcher/internal/assets"
	"launcher/internal/jsonx"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"launcher/internal/config"
	lgx "launcher/internal/logx"
)

//go:embed all:frontend/dist
var assets embed.FS

var appVersion = "0.25"

func main() {
	defer lgx.RecoverPanic("startup")

	// load config
	config := config.NewConfig()
	jsonx.LoadJSONFile("config.json", &config)
	thumbnailDir := cacheassets.ThumbnailCacheDir()
	if err := os.MkdirAll(thumbnailDir, 0o755); err != nil {
		lgx.Errorf("failed to create thumbnail cache: %v", err)
	}
	// Create an instance of the app structure
	app := NewApp(&config, thumbnailDir, cacheassets.ThumbnailURLPrefix)

	// Enable debug mode if specified in config
	logFileName := "launcher.log"
	if config.Debug {
		logFileName = "stagebox-debug.log"
	}
	if err := lgx.Init(config.Debug, logFileName, true); err != nil {
		lgx.Errorf("failed to open log file at default path: %v", err)
	}
	lgx.SetLevel("debug")
	lgx.Infof("launcher starting version=%s pid=%d", appVersion, os.Getpid())

	// Create application with options
	err := wails.Run(&options.App{
		Title: "launcher",
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: cacheassets.ThumbnailHandler(thumbnailDir),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []any{
			app,
		},
	})

	if err != nil {
		lgx.Error(err)
	}
}
