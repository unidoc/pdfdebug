package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create a new Wails application by providing the necessary options.
	app := application.New(application.Options{
		Name:        "unipdf-debugger",
		Description: "PDF structure inspector and debugger",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	if app == nil {
		log.Fatal("application.New returned nil")
	}

	// Build native menu bar
	menu := application.NewMenu()

	// macOS app menu (About, Services, Hide, Quit) -- AddRole is a no-op on non-macOS
	menu.AddRole(application.AppMenu)

	// File menu
	fileMenu := menu.AddSubmenu("File")
	fileMenu.Add("Open...").
		SetAccelerator("CmdOrCtrl+o").
		OnClick(func(ctx *application.Context) {
			// TODO: Wire to file dialog in Story 2.4
			log.Println("File > Open clicked")
		})
	fileMenu.AddSeparator()
	fileMenu.AddRole(application.CloseWindow)
	if runtime.GOOS != "darwin" {
		fileMenu.AddSeparator()
		fileMenu.AddRole(application.Quit)
	}

	// Edit menu (standard roles -- Cut, Copy, Paste, Select All, etc.)
	menu.AddRole(application.EditMenu)

	app.Menu.SetApplicationMenu(menu)

	// Create main window
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "UniPDF Debugger",
		Width:            1024,
		Height:           768,
		MinWidth:         800,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(248, 250, 252),
		URL:              "/",
	})

	// Run the application. This blocks until the application has been exited.
	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
