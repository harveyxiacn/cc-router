package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cliPath, err := resolveCLIPath()
	var app *App
	if err != nil {
		app = &App{initializationError: err.Error()}
	} else {
		app = NewApp(cliPath)
	}
	defer app.shutdown()
	err = wails.Run(&options.App{
		Title: "CC Router · 工作台", Width: 1280, Height: 820, MinWidth: 980, MinHeight: 680,
		BackgroundColour: &options.RGBA{R: 246, G: 245, B: 239, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets}, OnStartup: app.startup, Bind: []interface{}{app},
		EnableDefaultContextMenu: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "CC Router 桌面应用无法启动：", err)
		os.Exit(1)
	}
}
