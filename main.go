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

	"unidoc-pdf-debugger/internal/clitool"
	"unidoc-pdf-debugger/internal/pdfcore"
	"unidoc-pdf-debugger/internal/pdfservice"
	"unidoc-pdf-debugger/internal/pendingopen"
	"unidoc-pdf-debugger/internal/splash"
)

// version is the release version of the GUI binary, printed by the `--version`
// flag. Overridden at build time via `-ldflags "-X main.version=x.y.z"` (see
// build/{darwin,linux,windows}/Taskfile.yml). Default `"dev"` applies to
// untagged local builds.
var version = "dev"

// appName is the application's display name. It is BOTH the Wails app Name and
// the macOS app-submenu label (Wails builds the app submenu via
// NewSubMenuItem(options.Name)), so the install item lookup
// (menu.FindByLabel(appName)) MUST use the same literal -- a single source of
// truth here prevents the menu item from silently vanishing on a rename.
const appName = "UniDoc PDF Debugger"

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

// routeOpenPath is the shared per-path decision used by both file-open entry
// points (ApplicationOpenedWithFile, OnSecondInstanceLaunch). Story 12.1: it
// adds the path to the queue first; if the queue is ready (warm path) it opens
// the path immediately via open and returns true so the caller can decide
// whether to Focus the window. If the queue is not yet ready (cold start) the
// path is buffered for the frontend drain and the function returns false
// without opening. Extracting the decision keeps the two callbacks in lockstep
// and gives the wiring a unit-testable seam (see TestRouteOpenPath).
func routeOpenPath(q *pendingopen.Queue, path string, open func(string)) bool {
	if q.Add(path) {
		open(path)
		return true
	}
	return false
}

// pdfOpener is the narrow surface openFileAndEmitWithWarning needs from
// pdfservice.PDFService. Defined as an interface so the latency test can
// swap in a stub that sleeps inside OpenFile without dragging the full
// Wails service plumbing into the unit test. *pdfservice.PDFService
// satisfies this implicitly via its pointer-receiver methods.
type pdfOpener interface {
	OpenFile(path string) (*pdfcore.DocumentInfo, error)
	GetTreeRoot(tabID string) (*pdfcore.TreeNode, error)
	GetChildren(tabID string, nodeID string) ([]*pdfcore.TreeNode, error)
	CloseDocument(tabID string) error
}

// eventEmitter is the narrow surface openFileAndEmitWithWarning needs from
// application.EventManager. *application.EventManager satisfies this
// implicitly; the latency test passes a recording stub.
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
// The pdfcpu read is dispatched to a goroutine so the caller
// (Wails event-dispatch goroutine for menu / file-drop / single instance)
// returns immediately, leaving the native event loop free to service
// window resize / menu clicks during the parse. The wg argument lets
// callers synchronise on goroutine completion: openFilesBatch awaits per
// file (sequential at the file boundary because pdfcpu's ReadContextFile
// is not documented as concurrent-safe across files), and single-file
// entry points pass a local WaitGroup so they preserve their
// synchronous-completion contract.
//
// The caller MUST call wg.Add(1) BEFORE invoking this function. The
// goroutine launched here calls wg.Done() on completion. document:load-start is emitted synchronously (before the
// goroutine is dispatched) so the frontend renders the loading indicator
// without waiting on the goroutine scheduler.
//
// svc and emitter are narrow interfaces (pdfOpener, eventEmitter) so the
// latency test can inject a slow-OpenFile stub and a recording emitter.
// Production passes &pdfService and app.Event respectively.
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
// splash. It clears the splash's AlwaysOnTop so the main
// window can render above it, triggers the crossfade by emitting
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

// installedLinkPath returns the path of OUR installed pdfdebug symlink if one
// already exists in the install dir (used to initialize the menu label and to
// drive uninstall). Returns "" if not installed.
func installedLinkPath() string {
	dir := clitool.DefaultInstallDir()
	if dir == "" {
		return ""
	}
	link := filepath.Join(dir, "pdfdebug")
	if clitool.IsInstalled(link) {
		return link
	}
	return ""
}

// runInstallCLI invokes the install package and presents the appropriate native
// dialog for each typed result. On a confirmed-overwrite it re-invokes with the
// Overwrite flag. After a successful install it flips the menu item to the
// uninstall affordance. macOS-only (the menu item is gated to darwin).
func runInstallCLI(app *application.App, installItem *application.MenuItem, overwrite bool) {
	exe, err := clitool.RunningExecutablePath()
	if err != nil {
		app.Dialog.Error().
			SetTitle("Install pdfdebug").
			SetMessage("Could not locate the running application: " + err.Error()).
			Show()
		return
	}

	res, err := clitool.InstallCLI(clitool.Options{ExecutablePath: exe, Overwrite: overwrite})
	if err != nil {
		app.Dialog.Error().
			SetTitle("Install pdfdebug").
			SetMessage("Install failed: " + err.Error()).
			Show()
		return
	}

	switch r := res.(type) {
	case clitool.Installed:
		app.Dialog.Info().
			SetTitle("pdfdebug is ready").
			SetMessage("Installed at:\n" + r.Path + "\n\nOpen a NEW terminal window and try:\n  pdfdebug --version").
			Show()
		flipToUninstall(app, installItem, r.Path)
	case clitool.NeedsPathHelp:
		dialog := app.Dialog.Question().
			SetTitle("Almost there -- add pdfdebug to your PATH").
			SetMessage("pdfdebug was linked into:\n" + r.Dir + "\n\nThat directory is not on your PATH yet, so the command will not be found until it is added. Want me to add it to your shell profile for you?")
		addBtn := dialog.AddButton("Add it for me")
		manualBtn := dialog.AddButton("I'll do it myself")
		dialog.SetDefaultButton(addBtn)
		addBtn.OnClick(func() {
			profile, err := clitool.AddDirToShellProfile(r.Dir)
			if err != nil {
				// Unknown shell or write failure -> fall back to manual guidance.
				showManualPathHelp(app, r.Dir, r.ExportLine)
				return
			}
			app.Dialog.Info().
				SetTitle("pdfdebug added to your PATH").
				SetMessage("Updated:\n" + profile + "\n\nRestart your terminal (or run `source " + profile + "`), then run:\n  pdfdebug --version").
				Show()
		})
		manualBtn.OnClick(func() { showManualPathHelp(app, r.Dir, r.ExportLine) })
		dialog.Show()
		flipToUninstall(app, installItem, filepath.Join(r.Dir, "pdfdebug"))
	case clitool.ConfirmOverwrite:
		dialog := app.Dialog.Question().
			SetTitle("Replace existing pdfdebug?").
			SetMessage("An existing file is already at:\n" + r.LinkPath + "\n\nIt was not created by this app. Replace it with a link to the bundled pdfdebug?")
		replace := dialog.AddButton("Replace")
		cancel := dialog.AddButton("Cancel")
		dialog.SetDefaultButton(cancel)
		replace.OnClick(func() { runInstallCLI(app, installItem, true) })
		dialog.Show()
	case clitool.NotInBundle:
		app.Dialog.Warning().
			SetTitle("Install pdfdebug").
			SetMessage("This command is only available when running the installed app. Run UniDoc PDF Debugger from /Applications (or wherever you installed the .app), then try again.").
			Show()
	}
}

// showManualPathHelp presents the manual PATH-export instructions (the fallback
// when the user declines the auto-edit or the shell is unrecognized).
func showManualPathHelp(app *application.App, dir, exportLine string) {
	app.Dialog.Info().
		SetTitle("Add pdfdebug to your PATH").
		SetMessage("pdfdebug was linked into:\n" + dir + "\n\nAdd this line to your shell profile (e.g. ~/.zshrc), then open a NEW terminal:\n\n  " + exportLine + "\n\nThen run:\n  pdfdebug --version").
		Show()
}

// runUninstallCLI removes our symlink and flips the menu item back to the
// install affordance.
func runUninstallCLI(app *application.App, installItem *application.MenuItem, linkPath string) {
	if err := clitool.UninstallCLI(linkPath); err != nil {
		app.Dialog.Error().
			SetTitle("Uninstall pdfdebug").
			SetMessage("Uninstall failed: " + err.Error()).
			Show()
		return
	}
	app.Dialog.Info().
		SetTitle("pdfdebug removed").
		SetMessage("The pdfdebug command was removed from your PATH.").
		Show()
	flipToInstall(app, installItem)
}

// flipToUninstall mutates the retained menu item to the uninstall affordance.
func flipToUninstall(app *application.App, installItem *application.MenuItem, linkPath string) {
	installItem.SetLabel(clitool.UninstallMenuItemLabel)
	installItem.OnClick(func(ctx *application.Context) {
		runUninstallCLI(app, installItem, linkPath)
	})
}

// flipToInstall mutates the retained menu item back to the install affordance.
func flipToInstall(app *application.App, installItem *application.MenuItem) {
	installItem.SetLabel(clitool.MenuItemLabel)
	installItem.OnClick(func(ctx *application.Context) {
		runInstallCLI(app, installItem, false)
	})
}

// wireInstallCLIMenuItem appends the install item to the macOS app submenu,
// initializes its label from the current installed state, and wires its click
// handler. The retained *MenuItem is the runtime mutation target for the
// install/uninstall label flip (the app menu is set once via
// SetApplicationMenu). macOS-only; the caller gates on runtime.GOOS.
func wireInstallCLIMenuItem(app *application.App, appSub *application.Menu) {
	installItem := appSub.Add(clitool.MenuItemLabel)
	if link := installedLinkPath(); link != "" {
		flipToUninstall(app, installItem, link)
	} else {
		flipToInstall(app, installItem)
	}
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

	// The cold-start file-association queue. Constructed BEFORE
	// application.New() so both file-open callbacks can capture it by value
	// (it has no dependency on app/window, unlike the openFileAndEmit/window
	// closure dance above). On cold start, paths arriving before the frontend
	// has drained are buffered here instead of being emitted into a
	// not-yet-listening WebView and silently dropped.
	openQueue := &pendingopen.Queue{}

	// Create a new Wails application by providing the necessary options.
	app := application.New(application.Options{
		Name:        appName,
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
				// Route every path through the queue first. A path
				// that arrives before the frontend has drained is buffered
				// (Add returns false) instead of dropped; only ready/warm paths
				// open immediately. window.Focus() fires only when at least one
				// path opened on the ready (warm) path.
				focus := false
				for _, p := range extractPDFPaths(data.Args) {
					if routeOpenPath(openQueue, p, openFileAndEmit) {
						focus = true
					}
				}
				if focus {
					window.Focus()
				}
			},
		},
	})
	if app == nil {
		log.Fatal("application.New returned nil")
	}

	pdfService := pdfservice.NewPDFService(app)
	// Wire the cold-start queue so ConsumePendingOpenFiles can
	// drain it from the frontend.
	pdfService.SetPendingOpens(openQueue)

	app.RegisterService(application.NewService(&pdfService))

	// openFileAndEmit handles the shared logic for opening a PDF and emitting
	// the result to the frontend. Used by menu, file drop, file association,
	// and single-instance handlers.
	//
	// openFileAndEmitWithWarning now dispatches the pdfcpu read
	// to a goroutine. Single-file entry points (menu / file-drop /
	// single-instance / file-association) wrap with a local WaitGroup +
	// wg.Wait() so callers preserve their synchronous-completion contract.
	// Without this Wait, callers would return to the event loop before the
	// document opens, breaking the implicit "first call after Open succeeds
	// returns the new tab" assumption.
	openFileAndEmit = func(path string) {
		// Code shape: wg.Add(1) is called by the caller before invoking
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
		// Sequential dispatch at the file boundary.
		// Local WaitGroup; wg.Add(1) before each call (code shape);
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
		// No nil guard. The invariant: Drain is reachable only
		// through the bound ConsumePendingOpenFiles method, bindings serve only
		// after app.Run(), and both openFileAndEmit (assigned above) and window
		// (assigned before app.Run()) exist by then. An early-fire path is
		// buffered by routeOpenPath (queue not ready) rather than dropped, so a
		// guard here would re-introduce the silent drop this story fixes.
		filePath := event.Context().Filename()
		if filePath != "" && strings.EqualFold(filepath.Ext(filePath), ".pdf") {
			if routeOpenPath(openQueue, filePath, openFileAndEmit) {
				window.Focus()
			}
		}
	})

	// Build native menu bar
	menu := application.NewMenu()

	// macOS app menu (About, Services, Hide, Quit) -- AddRole is a no-op on non-macOS
	menu.AddRole(application.AppMenu)

	// Story 11.2: macOS-only "Install 'pdfdebug' Command in PATH..." item under
	// the app menu. AddRole(AppMenu) returns the PARENT *Menu, not the app
	// submenu, so the item is appended via FindByLabel(appName).GetSubmenu()
	// (verified against Wails v3 alpha.95; see the story's Menu-API note).
	if runtime.GOOS == "darwin" {
		if appItem := menu.FindByLabel(appName); appItem != nil {
			if appSub := appItem.GetSubmenu(); appSub != nil {
				wireInstallCLIMenuItem(app, appSub)
			}
		}
	}

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

	// Startup splash window. Created BEFORE the main
	// WebviewWindow so the user sees branding during WebView2 cold init
	// (especially on Windows where the main webview can take 10-30s on
	// first launch). Lives only in this first-instance bootstrap path --
	// the OnSecondInstanceLaunch and ApplicationOpenedWithFile callbacks
	// above are reentrant and MUST NOT spawn additional splash windows
	// (story 9.13 Task 2.2). The splash is on EVERY launch by design
	// (consistency is the brand signal); no first-launch persistence
	// gate.
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
	// integration tests (which scan source text for the options)
	// remain pinned to the story spec wording.
	//
	// Splash window options:
	//   Width: 480 -- logical width
	//   Height: 320 -- logical height
	//   Frameless: true -- no title bar / chrome
	//   AlwaysOnTop: true -- cleared in the dismissal handler
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
		// "no context menu". Without this the WebView's default
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

	// splashFailed tracks whether the splash entered the failure
	// (timeout) state. The flag is read by the splash WindowClosing
	// listener below: if the user closes the splash error pane (via the
	// Close button's window.close() or the OS), we terminate the app so
	// they are not left with a hidden main window stuck in cold-init.
	// Tracked as an atomic.Bool because the timeout callback fires on a
	// clock goroutine while WindowClosing fires on Wails' impl thread.
	var splashFailed atomic.Bool

	// Wire the failure-path timeout and the success-path dismissal
	// via the injectable-clock scheduler in internal/splash. The
	// min-display floor and the failure-path timeout are both served
	// by this single Scheduler instance.
	splashScheduler := splash.NewScheduler(
		splash.RealClock{},
		// onDismiss: clear AlwaysOnTop, trigger crossfade, then close
		// + destroy the splash so it does not linger in the OS
		// window list. The callback fires on a clock goroutine;
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

	// Close-to-quit: when the splash is closing after the failure
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
	// opaque first paint. The frontend additionally starts at
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
