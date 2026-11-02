// Some switches (the Aruba 6100 among them) ship a self-signed certificate
// with a negative serial number, which Go refuses by default since 1.23.
// The certificate is never trusted anyway; this only lets it be read.
//
//go:debug x509negativeserial=1
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// Version is set at build time with -ldflags "-X main.Version=..."
var Version = "1.0.1"

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "SwitchDesk",
		Width:            1360,
		Height:           860,
		MinWidth:         1024,
		MinHeight:        640,
		WindowStartState: options.Maximised,
		BackgroundColour: &options.RGBA{R: 14, G: 17, B: 22, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			About:                &mac.AboutInfo{Title: "SwitchDesk", Message: "Aruba switch manager\nVersion " + Version + "\n\nBuilt by Jaba Macharashvili\nclihero.com"},
		},
		Windows: &windows.Options{Theme: windows.Dark},
	})
	if err != nil {
		log.Fatal(err)
	}
}
