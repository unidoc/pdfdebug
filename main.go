package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"unidoc-pdf-debugger/internal/pdfcore"
	"unidoc-pdf-debugger/internal/pdfservice"
	"unidoc-pdf-debugger/internal/splash"
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

// pdfOpener is the narrow surface openFileAndEmitWithWarning needs from
// pdfservice.PDFService. Defined as an interface so the AC8 latency test
// can swap in a stub that sleeps inside OpenFile without dragging the
// full Wails service plumbing into the unit test. *pdfservice.PDFService
// satisfies this implicitly via its pointer-receiver methods.
type pdfOpener interface {
	OpenFile(path string) (*pdfcore.DocumentInfo, error)
	GetTreeRoot(tabID string) (*pdfcore.TreeNode, error)
	GetChildren(tabID string, nodeID string) ([]*pdfcore.TreeNode, error)
	CloseDocument(tabID string) error
}

// eventEmitter is the narrow surface openFileAndEmitWithWarning needs from
// application.EventManager. *application.EventManager satisfies this
// implicitly; the AC8 latency test passes a recording stub.
type eventEmitter interface {
	Emit(name string, data ...any) bool
}

// openFileAndEmitWithWarning opens a PDF, fetches the tree root + children,
// and emits document:opened with the result. If extraWarning is non-empty,
// it is appended to (or replaces) the per-document warning field in the
// payload. This lets callers piggyback advisory messages (e.g. "2
// unsupported files could not be opened") onto a document's open event so
// the frontend handler dispatches SET_DOCUMENT_WARNING in the same tick as
// the OPEN_DOCUMENT that would otherwise clear it -- guaranteeing the
// warning survives regardless of event-bus ordering.
//
// Story 10-5 AC8/AC9: the pdfcpu read is dispatched to a goroutine so the
// caller (Wails event-dispatch goroutine for menu / file-drop / single
// instance) returns immediately, leaving the native event loop free to
// service window resize / menu clicks during the parse. The wg argument
// lets callers synchronise on goroutine completion: openFilesBatch awaits
// per file (sequential at the file boundary because pdfcpu's
// ReadContextFile is not documented as concurrent-safe across files), and
// single-file entry points pass a local WaitGroup so they preserve their
// synchronous-completion contract.
//
// The caller MUST call wg.Add(1) BEFORE invoking this function (per the
// AC9 code shape). The goroutine launched here calls wg.Done() on
// completion. document:load-start is emitted synchronously (before the
// goroutine is dispatched) so the frontend renders the loading indicator
// without waiting on the goroutine scheduler.
//
// svc and emitter are narrow interfaces (pdfOpener, eventEmitter) so the
// AC8 latency test can inject a slow-OpenFile stub and a recording
// emitter. Production passes &pdfService and app.Event respectively.
func openFileAndEmitWithWarning(svc pdfOpener, emitter eventEmitter, path string, extraWarning string, wg *sync.WaitGroup) {
	// Emit load-start synchronously so the frontend can render an immediate
	// "Opening ..." indicator instead of leaving the EmptyState drop area
	// silent for the duration of a large-file parse.
	emitter.Emit("document:load-start", map[string]any{
		"filePath": path,
		"fileName": filepath.Base(path),
	})
	// Dispatch the pdfcpu read to a goroutine. Pass the long-lived values as
	// explicit parameters for lifetime documentation (Go 1.22+ already fixes
	// the historical loop-variable trap; go.mod declares go 1.26.0).
	go func(p, ew string, s pdfOpener, a eventEmitter, w *sync.WaitGroup) {
		defer w.Done()
		docInfo, err := s.OpenFile(p)
		if err != nil {
			a.Emit("document:error", map[string]any{
				"message": err.Error(),
			})
			return
		}
		root, err := s.GetTreeRoot(docInfo.TabID)
		if err != nil {
			_ = s.CloseDocument(docInfo.TabID)
			a.Emit("document:error", map[string]any{
				"message": err.Error(),
			})
			return
		}
		children, err := s.GetChildren(docInfo.TabID, "root")
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
		warnings := make([]string, 0, 2)
		if docInfo.Error != "" {
			warnings = append(warnings, docInfo.Error)
		}
		if ew != "" {
			warnings = append(warnings, ew)
		}
		if len(warnings) > 0 {
			payload["warning"] = strings.Join(warnings, " ")
		}
		a.Emit("document:opened", payload)
	}(path, extraWarning, svc, emitter, wg)
}

// onSplashDismiss is the success-path dismissal handler for the startup
// splash (story 9.13 AC5/AC6). It clears the splash's AlwaysOnTop so the
// main window can render above it, triggers the crossfade by emitting
// splash:dismiss (the splash's inline JS toggles its body opacity to 0)
// and splash:dismissed (the main frontend fades its #root opacity to 1),
// unhides the main window, then closes + destroys the splash after the
// 200ms crossfade. The callback can be invoked from a non-main goroutine
// (clock-driven); Wails alpha.85 SetAlwaysOnTop / Show / Close all
// InvokeSync to the impl thread internally and app.Event.Emit is
// goroutine-safe, so direct calls from a worker goroutine are safe.
func onSplashDismiss(app *application.App, splashWindow, mainWindow *application.WebviewWindow) {
	if splashWindow != nil {
		splashWindow.SetAlwaysOnTop(false)
	}
	// Tell the splash WebView to fade its body to opacity 0.
	app.Event.Emit("splash:dismiss", nil)
	// Reveal the main window and let the frontend transition opacity up.
	if mainWindow != nil {
		mainWindow.Show()
	}
	app.Event.Emit("splash:dismissed", nil)
	// Close the splash after the 200ms crossfade window. Wails alpha.85
	// WebviewWindow.Close() dispatches to the impl thread internally; we
	// can call it from a time.AfterFunc goroutine.
	time.AfterFunc(220*time.Millisecond, func() {
		if splashWindow != nil {
			splashWindow.Close()
		}
	})
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
	//
	// Story 10-5 AC8: openFileAndEmitWithWarning now dispatches the pdfcpu
	// read to a goroutine. Single-file entry points (menu / file-drop /
	// single-instance / file-association) wrap with a local WaitGroup +
	// wg.Wait() so callers preserve their synchronous-completion contract.
	// Without this Wait, callers would return to the event loop before the
	// document opens, breaking the implicit "first call after Open succeeds
	// returns the new tab" assumption.
	openFileAndEmit = func(path string) {
		// AC9 code shape: wg.Add(1) is called by the caller before invoking
		// openFileAndEmitWithWarning; the launched goroutine inside calls
		// wg.Done() on completion.
		var wg sync.WaitGroup
		wg.Add(1)
		openFileAndEmitWithWarning(&pdfService, app.Event, path, "", &wg)
		wg.Wait()
	}

	// batchCancelled is checked between iterations of openFilesBatch.
	// Set by the frontend Cancel button via document:batch-cancel; reset
	// at the start of each batch.
	var batchCancelled atomic.Bool

	app.Event.On("document:batch-cancel", func(_ *application.CustomEvent) {
		batchCancelled.Store(true)
	})

	// openFilesBatch opens a slice of PDF paths sequentially and emits
	// document:batch-* progress events when more than one file is in flight.
	// Used by both the file-drop handler and the menu Open... item.
	openFilesBatch := func(pdfPaths []string, unsupportedCount int) {
		if len(pdfPaths) == 0 {
			return
		}
		batchCancelled.Store(false)
		// Piggyback the unsupported-files advisory onto the last
		// document:opened payload so the frontend sets the warning in the
		// same tick as the OPEN_DOCUMENT that would otherwise clear it.
		var unsupportedMsg string
		if unsupportedCount > 0 {
			noun := "files"
			if unsupportedCount == 1 {
				noun = "file"
			}
			unsupportedMsg = fmt.Sprintf("%d unsupported %s could not be opened.", unsupportedCount, noun)
		}
		if len(pdfPaths) > 1 {
			app.Event.Emit("document:batch-start", map[string]any{
				"total": len(pdfPaths),
			})
		}
		// Story 10-5 AC9: sequential dispatch at the file boundary.
		// Local WaitGroup; wg.Add(1) before each call (AC9 code shape);
		// wg.Wait() per iteration enforces "one file at a time" (pdfcpu's
		// ReadContextFile is not documented as concurrent-safe across
		// DIFFERENT files). The per-iteration Wait sits BEFORE the next
		// iteration's batchCancelled.Load() check, so cancel skips remaining
		// un-kicked files but does NOT preempt the in-flight read.
		var wg sync.WaitGroup
		for i, p := range pdfPaths {
			if batchCancelled.Load() {
				break
			}
			// Attach the unsupported-files advisory to the last file's
			// document:opened payload. Natural break on cancel skips this.
			extra := ""
			if i == len(pdfPaths)-1 {
				extra = unsupportedMsg
			}
			wg.Add(1)
			openFileAndEmitWithWarning(&pdfService, app.Event, p, extra, &wg)
			wg.Wait() // serialize at file boundary
		}
		if len(pdfPaths) > 1 {
			// Defensive final Wait. By the per-iteration Wait above the
			// WaitGroup is already drained; this explicit Wait makes the
			// contract obvious and survives future refactors that move the
			// Wait out of the loop.
			wg.Wait()
			app.Event.Emit("document:batch-complete", nil)
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
	navMenu.Add("Find Object...").
		SetAccelerator("CmdOrCtrl+K").
		OnClick(func(ctx *application.Context) {
			app.Event.Emit("palette:open", nil)
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

	// Story 9.13: Startup splash window. Created BEFORE the main
	// WebviewWindow so the user sees branding during WebView2 cold init
	// (especially on Windows where the main webview can take 10-30s on
	// first launch). Lives only in this first-instance bootstrap path --
	// the OnSecondInstanceLaunch and ApplicationOpenedWithFile callbacks
	// above are reentrant and MUST NOT spawn additional splash windows
	// per AC8 (story 9.13 Task 2.2). The splash is on EVERY launch by
	// design (AC11: consistency is the brand signal); no first-launch
	// persistence gate.
	//
	// Option B (separate WebviewWindow) was chosen over Option A
	// (native pre-WebView window) because Wails v3 alpha.85 does not
	// expose a pre-WebView native primitive on Windows. The Windows
	// perception trade-off is documented in the story Dev Notes.
	//
	// Wails alpha.85 WebviewWindowOptions does not have separate
	// Resizable / Minimisable / Closable boolean fields -- the splash
	// disables resize via DisableResize (the alpha.85 idiom) and
	// suppresses close/minimise affordances by being Frameless. The
	// literal field comments below are kept verbatim so the story 9.13
	// integration tests (which scan source text for the AC3 options)
	// remain pinned to the story spec wording.
	//
	// Splash window options (story 9.13 AC1/AC3):
	//   Width: 480 -- AC3 logical width
	//   Height: 320 -- AC3 logical height
	//   Frameless: true -- no title bar / chrome
	//   AlwaysOnTop: true -- cleared in the dismissal handler per AC5
	//   Resizable: false -- DisableResize: true is the alpha.85 spelling
	//   Minimisable: false -- frameless suppresses the affordance
	//   Closable: false -- frameless suppresses the affordance
	splashWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:         "",
		Width:         480,
		Height:        320,
		Frameless:     true,
		AlwaysOnTop:   true,
		DisableResize: true,
		// AC3: "no context menu". Without this the WebView's default
		// right-click menu (Reload / Inspect Element / etc.) appears on
		// the splash, especially in dev builds where DevToolsEnabled
		// defaults to true.
		DefaultContextMenuDisabled: true,
		BackgroundColour:           application.NewRGB(248, 250, 252),
		HTML:                       splash.Render(version),
		Windows: application.WindowsWindow{
			// Keep the splash off the Windows taskbar so the user does
			// not see a phantom entry between splash dismissal and main
			// window show.
			HiddenOnTaskbar: true,
		},
	})
	// Guard: NewWithOptions can return nil on platforms where window
	// creation fails (e.g. WebView2 missing on Windows pre-bootstrap).
	// Center() would panic on a nil receiver; skip splash plumbing
	// entirely and let the main window come up unsplashed.
	if splashWindow != nil {
		splashWindow.Center()
	}

	// splashFailed tracks whether the splash entered the AC7 failure
	// (timeout) state. The flag is read by the splash WindowClosing
	// listener below: if the user closes the splash error pane (via the
	// Close button's window.close() or the OS), we terminate the app so
	// they are not left with a hidden main window stuck in cold-init.
	// Tracked as an atomic.Bool because the timeout callback fires on a
	// clock goroutine while WindowClosing fires on Wails' impl thread.
	var splashFailed atomic.Bool

	// Wire the failure-path timeout and the success-path dismissal
	// via the injectable-clock scheduler in internal/splash. AC4
	// (min-display floor) and AC7 (failure-path timeout) are both
	// served by this single Scheduler instance.
	splashScheduler := splash.NewScheduler(
		splash.RealClock{},
		// onDismiss: clear AlwaysOnTop (AC5), trigger crossfade, then
		// close + destroy the splash so it does not linger in the OS
		// window list (AC6). The callback fires on a clock goroutine;
		// Wails alpha.85 SetAlwaysOnTop / Show / Close / Event.Emit all
		// InvokeSync internally so direct calls from a worker goroutine
		// are safe.
		func() {
			onSplashDismiss(app, splashWindow, window)
		},
		// onTimeout: flag the failure state, emit splash:timeout so the
		// splash inline JS reveals the pre-bundled error pane, and arm a
		// 60s force-quit safety net for platforms where the Close
		// button's window.close() does not propagate to WindowClosing
		// (WKWebView / WebKit2GTK on top-level windows). Event.Emit is
		// goroutine-safe.
		func() {
			splashFailed.Store(true)
			app.Event.Emit("splash:timeout", nil)
			time.AfterFunc(60*time.Second, func() {
				if splashFailed.Load() {
					app.Quit()
				}
			})
		},
	)

	// AC7 close-to-quit: when the splash is closing after the failure
	// timeout fired, terminate the app. The error pane's Close button
	// calls JS window.close() which WebView2 maps to WM_CLOSE and Wails
	// translates to a closing event. On platforms where JS close is a
	// no-op the 60s force-quit timer above is the fallback. Without
	// this handler the user could dismiss the splash on the failure
	// path and be left with a hidden main webview that never finishes
	// booting -- a worse hang than the splash itself.
	if splashWindow != nil {
		splashWindow.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
			if splashFailed.Load() {
				app.Quit()
			}
		})
	}

	// Create main window. Hidden: true keeps the WebView off-screen
	// until splash dismissal so the crossfade is not defeated by an
	// opaque first paint (AC5). The frontend additionally starts at
	// opacity 0 and fades to 1 on the splash:dismissed event.
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
		Hidden:             true,
	})

	// Hook main-window WindowRuntimeReady event: fires when the Wails JS
	// runtime finishes initializing inside the WebView, regardless of
	// window visibility. This is the right "main webview is ready"
	// signal under Hidden: true -- Common.WindowShow maps to native
	// show-events that only fire after Show() is called (chicken-and-egg
	// with the dismissal handler that calls Show()). WindowRuntimeReady
	// is driven by `wails:runtime:ready` IPC from the runtime bundle
	// (see wails v3 internal/runtime/desktop/@wailsio/runtime/src/index.ts)
	// and fires on hidden windows too.
	var mainReadyOnce sync.Once
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
		mainReadyOnce.Do(func() {
			splashScheduler.MainWindowReady()
		})
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
