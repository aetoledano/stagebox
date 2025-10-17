package main

import (
	"embed"
	"fmt"
	"launcher/tools"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"launcher/internal/config"
	lgx "launcher/internal/logx"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	defer lgx.RecoverPanic("startup")

	// load config
	config := config.NewConfig()
	tools.LoadJSONFile("config.json", &config)
	// Create an instance of the app structure
	app := NewApp(&config)

	// Enable debug mode if specified in config
	ts := time.Now().UnixMilli()
	logFileName := fmt.Sprintf("debug-%v.log", ts)
	if err := lgx.Init(config.Debug, logFileName, true); err != nil {
		lgx.Errorf("failed to open log file at default path: %v", err)
	}
	lgx.SetLevel("debug")
	lgx.Info("launcher starting…")

	// Create application with options
	err := wails.Run(&options.App{
		Title: "launcher",
		AssetServer: &assetserver.Options{
			Assets: assets,
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
