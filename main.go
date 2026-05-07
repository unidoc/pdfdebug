package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"unidoc-pdf-debugger/internal/pdfservice"
)

// version is the release version of the GUI binary, printed by the `--version`
// flag. Overridden at build time via `-ldflags "-X main.version=x.y.z"` (see
// build/{darwin,linux,windows}/Taskfile.yml). Default `"dev"` applies to
// untagged local builds.
var version = "dev"

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

// extractPDFPaths returns all arguments (after the first, which is the binary
// name) that end in .pdf (case-insensitive). Returns nil if args has fewer
// than 2 elements.
func extractPDFPaths(args []string) []string {
	if len(args) < 2 {
		return nil
	}
	var paths []string
	for _, arg := range args[1:] {
		if strings.EqualFold(filepath.Ext(arg), ".pdf") {
			paths = append(paths, arg)
		}
	}
	return paths
}

func main() {
	// --version short-circuit: must run BEFORE application.New so that
	// `unidoc-pdf-debugger --version` does not spin up a Wails webview/window
	// on headless runners.
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)
		os.Exit(0)
	}

	// Declare variables before application.New() so the SingleInstance callback
	// can capture them by reference. They are assigned after app/window creation
	// but before app.Run(), so they are safe to use in the async callback.
	var openFileAndEmit func(string)
	var window *application.WebviewWindow

	// Create a new Wails application by providing the necessary options.
	app := application.New(application.Options{
		Name:        "UniDoc PDF Debugger",
		Description: "PDF structure inspector and debugger\n\nUniDoc ehf. -- https://unidoc.io",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		FileAssociations: []string{".pdf"},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.unidoc.unidoc-pdf-debugger",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// Guard: openFileAndEmit and window are assigned after app
				// creation but before app.Run(). If this fires unexpectedly
				// early, skip rather than panic.
				if openFileAndEmit == nil || window == nil {
					return
				}
				pdfPaths := extractPDFPaths(data.Args)
				for _, p := range pdfPaths {
					openFileAndEmit(p)
				}
				window.Focus()
			},
		},
	})
	if app == nil {
		log.Fatal("application.New returned nil")
	}

	pdfService := pdfservice.NewPDFService(app)

	app.RegisterService(application.NewService(&pdfService))

	// openFileAndEmit handles the shared logic for opening a PDF and emitting
	// the result to the frontend. Used by menu, file drop, file association,
	// and single-instance handlers.
	openFileAndEmit = func(path string) {
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
		payload := map[string]any{
			"tabId":        docInfo.TabID,
			"fileName":     docInfo.FileName,
			"filePath":     docInfo.FilePath,
			"pageCount":    docInfo.PageCount,
			"fileSize":     docInfo.FileSize,
			"rootNode":     root,
			"rootChildren": children,
		}
		if docInfo.Error != "" {
			payload["warning"] = docInfo.Error
		}
		app.Event.Emit("document:opened", payload)
	}

	// openFilesBatch opens a slice of PDF paths sequentially and emits
	// document:batch-* progress events when more than one file is in flight.
	// Optionally emits a soft document:warning for unsupported files in a
	// mixed batch. Used by both the file-drop handler and the menu Open...
	// item so the multi-file UX is identical regardless of how the user
	// selected the batch.
	openFilesBatch := func(pdfPaths []string, unsupportedCount int) {
		if len(pdfPaths) == 0 {
			return
		}
		if len(pdfPaths) > 1 {
			app.Event.Emit("document:batch-start", map[string]any{
				"total": len(pdfPaths),
			})
		}
		for i, p := range pdfPaths {
			openFileAndEmit(p)
			if len(pdfPaths) > 1 {
				app.Event.Emit("document:batch-progress", map[string]any{
					"completed": i + 1,
					"total":     len(pdfPaths),
				})
			}
		}
		if len(pdfPaths) > 1 {
			app.Event.Emit("document:batch-complete", nil)
		}
		if unsupportedCount > 0 {
			noun := "files"
			if unsupportedCount == 1 {
				noun = "file"
			}
			app.Event.Emit("document:warning", map[string]any{
				"message": fmt.Sprintf("%d unsupported %s could not be opened.", unsupportedCount, noun),
			})
		}
	}

	// Handle files opened via OS file association (right-click > "Open with").
	// On macOS this fires for both cold and warm starts. On Windows/Linux cold
	// start only -- warm start is handled by OnSecondInstanceLaunch.
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(event *application.ApplicationEvent) {
		if openFileAndEmit == nil || window == nil {
			return
		}
		filePath := event.Context().Filename()
		if filePath != "" && strings.EqualFold(filepath.Ext(filePath), ".pdf") {
			openFileAndEmit(filePath)
			window.Focus()
		}
	})

	// Build native menu bar
	menu := application.NewMenu()

	// macOS app menu (About, Services, Hide, Quit) -- AddRole is a no-op on non-macOS
	menu.AddRole(application.AppMenu)

	// File menu
	fileMenu := menu.AddSubmenu("File")
	fileMenu.Add("Open...").
		SetAccelerator("CmdOrCtrl+o").
		OnClick(func(ctx *application.Context) {
			paths, err := app.Dialog.OpenFile().
				SetTitle("Open PDF").
				AddFilter("PDF Files", "*.pdf").
				AddFilter("All Files", "*.*").
				PromptForMultipleSelection()
			if err != nil || len(paths) == 0 {
				return
			}
			// Filter on extension defensively in case the user picked
			// non-PDFs via the "All Files" filter.
			var pdfPaths []string
			for _, p := range paths {
				if strings.EqualFold(filepath.Ext(p), ".pdf") {
					pdfPaths = append(pdfPaths, p)
				}
			}
			unsupported := len(paths) - len(pdfPaths)
			if len(pdfPaths) == 0 {
				app.Event.Emit("document:error", map[string]any{
					"message": "Only PDF files can be opened.",
				})
				return
			}
			openFilesBatch(pdfPaths, unsupported)
		})
	fileMenu.Add("Close Document").
		SetAccelerator("CmdOrCtrl+w").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("document:close-active", nil)
		})
	if runtime.GOOS != "darwin" {
		fileMenu.AddSeparator()
		fileMenu.Add("Quit").
			SetAccelerator("Ctrl+q").
			OnClick(func(ctx *application.Context) {
				app.Quit()
			})
	}

	// Edit menu (standard roles -- Cut, Copy, Paste, Select All, etc.)
	menu.AddRole(application.EditMenu)

	// Navigate menu
	navMenu := menu.AddSubmenu("Navigate")
	navBackItem := navMenu.Add("Back").
		SetAccelerator("CmdOrCtrl+[").
		SetEnabled(false).
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("navigate:back", nil)
		})
	navForwardItem := navMenu.Add("Forward").
		SetAccelerator("CmdOrCtrl+]").
		SetEnabled(false).
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("navigate:forward", nil)
		})

	navMenu.AddSeparator()
	navMenu.Add("Go to Page...").
		SetAccelerator("CmdOrCtrl+G").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("navigate:goToPage", nil)
		})

	navMenu.AddSeparator()
	navMenu.Add("Next Tab").
		SetAccelerator("CmdOrCtrl+Right").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("tab:next", nil)
		})
	navMenu.Add("Previous Tab").
		SetAccelerator("CmdOrCtrl+Left").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("tab:prev", nil)
		})

	// Frontend sends navigation state changes to sync menu enabled state
	app.Event.On("navigate:state-changed", func(event *application.CustomEvent) {
		data, ok := event.Data.(map[string]any)
		if !ok {
			return
		}
		if canBack, ok := data["canGoBack"].(bool); ok {
			navBackItem.SetEnabled(canBack)
		}
		if canFwd, ok := data["canGoForward"].(bool); ok {
			navForwardItem.SetEnabled(canFwd)
		}
	})

	// macOS uses the screen-top system menu bar via SetApplicationMenu.
	// Windows requires UseApplicationMenu: true on each window to opt in
	// (Wails alpha.74 windowsWebviewWindow only honors the app menu when
	// UseApplicationMenu is set; Windows.Menu would also work but is more
	// verbose). Linux falls back to the app menu unconditionally.
	app.Menu.SetApplicationMenu(menu)

	// Create main window
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:              "UniDoc PDF Debugger",
		Width:              1024,
		Height:             768,
		MinWidth:           800,
		MinHeight:          600,
		BackgroundColour:   application.NewRGB(248, 250, 252),
		URL:                "/",
		EnableFileDrop:     true,
		UseApplicationMenu: true,
	})

	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		var pdfPaths []string
		for _, f := range files {
			if strings.EqualFold(filepath.Ext(f), ".pdf") {
				pdfPaths = append(pdfPaths, f)
			}
		}
		unsupported := len(files) - len(pdfPaths)
		if len(pdfPaths) == 0 {
			app.Event.Emit("document:error", map[string]any{
				"message": "Only PDF files can be opened.",
			})
			return
		}
		openFilesBatch(pdfPaths, unsupported)
	})

	// Run the application. This blocks until the application has been exited.
	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
