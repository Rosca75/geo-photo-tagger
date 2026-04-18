package main

// main.go — GeoPhotoTagger entry point
// This file bootstraps the Wails v2 desktop application.
// It embeds the static/ directory and binds the App struct to the JS frontend.

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Embed the entire static/ directory into the binary at compile time.
// Wails serves these files as the frontend — no external HTTP server needed.
//
//go:embed all:static
var assets embed.FS

func main() {
	// Create a new App instance. This struct holds all application state
	// and its public methods become callable from JavaScript.
	app := NewApp()

	// Start the Wails application.
	// - Assets: embedded static/ directory
	// - Window: 1400×900 default, 1000×700 minimum
	// - OnStartup: called when the app window is ready
	// - Bind: exposes App methods to the JS frontend via window.go.main.App.*
	err := wails.Run(&options.App{
		Title:     "GeoPhotoTagger",
		Width:     1400,
		Height:    900,
		MinWidth:  1000,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal("Error starting GeoPhotoTagger:", err)
	}
}
