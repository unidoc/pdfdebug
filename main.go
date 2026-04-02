package main

import (
	"embed"
	"log"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"unipdf-debugger/internal/pdfservice"
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

	pdfService := pdfservice.NewPDFService(app)

	app.RegisterService(application.NewService(&pdfService))

	// openFileAndEmit handles the shared logic for opening a PDF and emitting
	// the result to the frontend. Used by both menu and file drop handlers.
	openFileAndEmit := func(path string) {
		docInfo, err := pdfService.OpenFile(path)
		if err != nil {
			app.Event.Emit("document:error", map[string]any{
				"message": err.Error(),
			})
			return
		}
		root, err := pdfService.GetTreeRoot(docInfo.TabID)
		if err != nil {
			_ = pdfService.CloseDocument(docInfo.TabID)
			app.Event.Emit("document:error", map[string]any{
				"message": err.Error(),
			})
			return
		}
		children, err := pdfService.GetChildren(docInfo.TabID, "root")
		if err != nil {
			log.Printf("warning: failed to get root children for tab %s: %v", docInfo.TabID, err)
		}
		app.Event.Emit("document:opened", map[string]any{
			"tabId":        docInfo.TabID,
			"fileName":     docInfo.FileName,
			"filePath":     docInfo.FilePath,
			"pageCount":    docInfo.PageCount,
			"fileSize":     docInfo.FileSize,
			"rootNode":     root,
			"rootChildren": children,
		})
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
			path, err := app.Dialog.OpenFile().
				SetTitle("Open PDF").
				AddFilter("PDF Files", "*.pdf").
				AddFilter("All Files", "*.*").
				PromptForSingleSelection()
			if err != nil || path == "" {
				return
			}
			openFileAndEmit(path)
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
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "UniPDF Debugger",
		Width:            1024,
		Height:           768,
		MinWidth:         800,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(248, 250, 252),
		URL:              "/",
		EnableFileDrop:   true,
	})

	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		var pdfPath string
		for _, f := range files {
			if strings.EqualFold(filepath.Ext(f), ".pdf") {
				pdfPath = f
				break
			}
		}
		if pdfPath == "" {
			return
		}
		openFileAndEmit(pdfPath)
	})

	// Run the application. This blocks until the application has been exited.
	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
