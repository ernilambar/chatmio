package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            readAppDetails().Name,
		Width:            1200,
		Height:           800,
		MinWidth:         900,
		MinHeight:        600,
		WindowStartState: options.Maximised,
		BackgroundColour: &options.RGBA{R: 15, G: 17, B: 21, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Menu:       applicationMenu(app),
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal("wails: ", err)
	}
}
