package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"gitvela/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

var windowsOptions application.WindowsOptions

func main() {
	desktop := application.New(application.Options{
		Name:        "GitVela",
		Icon:        appIcon,
		Description: "Browse local Git history and compare changes",
		Services:    []application.Service{application.NewService(&app.RepositoryService{})},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Windows:     windowsOptions,
	})
	desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "GitVela",
		Width:            1340,
		Height:           880,
		MinWidth:         780,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(255, 255, 255),
		URL:              "/",
	})
	if err := desktop.Run(); err != nil {
		log.Fatal(err)
	}
}
