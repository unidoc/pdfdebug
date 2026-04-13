// Package open_pdf_dialog_dnd_test provides acceptance tests for Story 2.4:
// Open PDF via File Dialog and Drag-and-Drop.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 2-4 is implemented.
//
// Test Levels: Integration (Go) -- source file content parsing, structural validation.
// Frontend state/UI tests are Vitest (2.4-UNIT-001/002/003) or E2E (2.4-E2E-001).
//
// Run: cd tests/open-pdf-dialog-dnd && go test -v -count=1 ./...
package open_pdf_dialog_dnd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot returns the absolute path to the project root directory.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// readFile reads a file relative to the project root and returns its content.
func readFile(t *testing.T, relPath string) string {
	t.Helper()
	root := projectRoot(t)
	absPath := filepath.Join(root, relPath)
	content, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", relPath, err)
	}
	return string(content)
}

// fileExists checks if a file exists relative to the project root.
func fileExists(t *testing.T, relPath string) bool {
	t.Helper()
	root := projectRoot(t)
	absPath := filepath.Join(root, relPath)
	_, err := os.Stat(absPath)
	return err == nil
}

// ---------------------------------------------------------------------------
// AC#1: File dialog -- PDFService.OpenFileDialog method + SetApp pattern
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-001 [P0]: PDFService has OpenFileDialog method
// AC#1: Given PDFService, When reviewed, Then it has an exported
//       OpenFileDialog() method that returns (string, error).
// ---------------------------------------------------------------------------

func TestPDFServiceHasOpenFileDialog(t *testing.T) {
	content := readFile(t, "internal/pdfservice/service.go")

	if !strings.Contains(content, "func (s *PDFService) OpenFileDialog(") {
		t.Fatal("[P0] 2.4-INTG-001: service.go missing OpenFileDialog method on PDFService")
	}

	// Return type should include string and error
	// Look for the method signature line
	lines := strings.Split(content, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "func (s *PDFService) OpenFileDialog(") {
			if strings.Contains(line, "string") && strings.Contains(line, "error") {
				found = true
			}
			break
		}
	}
	if !found {
		t.Error("[P0] 2.4-INTG-001: OpenFileDialog should return (string, error)")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-002 [P0]: PDFService has SetApp method and app field
// AC#1: PDFService needs access to *application.App for dialog calls.
//       SetApp(app *application.App) method and app field must exist.
// ---------------------------------------------------------------------------

func TestPDFServiceHasSetAppAndAppField(t *testing.T) {
	content := readFile(t, "internal/pdfservice/service.go")

	// Must have app field of type *application.App
	if !strings.Contains(content, "app ") || !strings.Contains(content, "*application.App") {
		t.Error("[P0] 2.4-INTG-002: service.go missing app *application.App field in PDFService struct")
	}

	// Must accept *application.App via constructor (not SetApp, to avoid exposing it as a Wails binding)
	if !strings.Contains(content, "func NewPDFService(app *application.App)") {
		t.Error("[P0] 2.4-INTG-002: service.go NewPDFService must accept *application.App parameter")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-003 [P0]: PDFService imports wails application package
// AC#1: pdfservice is the Wails adapter layer -- it MAY import Wails packages
//       (unlike pdfcore which must have zero Wails deps). After story 2-4,
//       service.go must import application for *application.App type.
// ---------------------------------------------------------------------------

func TestPDFServiceImportsWailsApplication(t *testing.T) {
	content := readFile(t, "internal/pdfservice/service.go")

	if !strings.Contains(content, `"github.com/wailsapp/wails/v3/pkg/application"`) {
		t.Error("[P0] 2.4-INTG-003: service.go must import wails application package for dialog API")
	}
}

// ---------------------------------------------------------------------------
// AC#1: main.go creates PDFService with app after application.New()
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-004 [P0]: main.go passes app to NewPDFService after app creation
// AC#1: PDFService must receive *application.App via constructor, created
//       after application.New() but before app.Run().
// ---------------------------------------------------------------------------

func TestMainGoCallsSetApp(t *testing.T) {
	content := readFile(t, "main.go")

	// Must create PDFService with app argument (constructor injection)
	if !strings.Contains(content, "pdfservice.NewPDFService(app)") {
		t.Error("[P0] 2.4-INTG-004: main.go must call pdfservice.NewPDFService(app)")
	}

	// NewPDFService must come after application.New and before app.Run
	newIdx := strings.Index(content, "application.New(")
	newSvcIdx := strings.Index(content, "pdfservice.NewPDFService(app)")
	runIdx := strings.Index(content, "app.Run()")

	if newIdx < 0 || newSvcIdx < 0 || runIdx < 0 {
		t.Fatal("[P0] 2.4-INTG-004: main.go missing application.New, NewPDFService(app), or app.Run")
	}

	if newSvcIdx < newIdx {
		t.Error("[P0] 2.4-INTG-004: NewPDFService(app) must come after application.New()")
	}
	if newSvcIdx > runIdx {
		t.Error("[P0] 2.4-INTG-004: NewPDFService(app) must come before app.Run()")
	}
}

// ---------------------------------------------------------------------------
// AC#2: Drag-and-drop -- EnableFileDrop + window event handler
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-005 [P0]: main.go enables file drop on window
// AC#2: WebviewWindowOptions must include EnableFileDrop: true.
// ---------------------------------------------------------------------------

func TestMainGoEnableFileDrop(t *testing.T) {
	content := readFile(t, "main.go")

	if !strings.Contains(content, "EnableFileDrop") {
		t.Fatal("[P0] 2.4-INTG-005: main.go missing EnableFileDrop in WebviewWindowOptions")
	}

	// Must be set to true
	// Find the line containing EnableFileDrop and check it has "true"
	lines := strings.Split(content, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "EnableFileDrop") {
			if strings.Contains(line, "true") {
				found = true
			}
			break
		}
	}
	if !found {
		t.Error("[P0] 2.4-INTG-005: EnableFileDrop must be set to true")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-006 [P0]: main.go captures window reference
// AC#2: Window return value from NewWithOptions must be captured (not discarded)
//       to register OnWindowEvent handler for file drop.
// ---------------------------------------------------------------------------

func TestMainGoCapturesWindowReference(t *testing.T) {
	content := readFile(t, "main.go")

	// The window reference must be captured, not discarded.
	// Current code: app.Window.NewWithOptions(...) -- return value discarded.
	// After: window := app.Window.NewWithOptions(...) or similar.
	if strings.Contains(content, "app.Window.NewWithOptions(") {
		// Check it's assigned to a variable (not just called)
		lines := strings.Split(content, "\n")
		captured := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "NewWithOptions(") || strings.Contains(trimmed, "NewWithOptions(application.WebviewWindowOptions") {
				// Check that the line has an assignment (= or :=)
				if strings.Contains(trimmed, ":=") || (strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "//")) {
					captured = true
				}
				break
			}
		}
		if !captured {
			t.Error("[P0] 2.4-INTG-006: main.go must capture window return value from NewWithOptions (e.g., window := app.Window.NewWithOptions(...))")
		}
	} else {
		t.Error("[P0] 2.4-INTG-006: main.go missing app.Window.NewWithOptions call")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-007 [P0]: main.go registers OnWindowEvent for file drop
// AC#2: Go-side window event handler for WindowFilesDropped must exist.
// ---------------------------------------------------------------------------

func TestMainGoRegistersFileDropHandler(t *testing.T) {
	content := readFile(t, "main.go")

	// Must have OnWindowEvent call
	if !strings.Contains(content, "OnWindowEvent(") {
		t.Fatal("[P0] 2.4-INTG-007: main.go missing OnWindowEvent registration for file drop")
	}

	// Must reference WindowFilesDropped event
	if !strings.Contains(content, "WindowFilesDropped") {
		t.Error("[P0] 2.4-INTG-007: main.go must handle events.Common.WindowFilesDropped")
	}

	// Must import events package
	if !strings.Contains(content, `"github.com/wailsapp/wails/v3/pkg/events"`) {
		t.Error("[P0] 2.4-INTG-007: main.go must import wails events package")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-008 [P0]: File drop handler filters for .pdf extension
// AC#2: The drop handler must filter dropped files for .pdf extension.
// ---------------------------------------------------------------------------

func TestMainGoFileDropFiltersPDF(t *testing.T) {
	content := readFile(t, "main.go")

	// Must reference DroppedFiles to get file list from event
	if !strings.Contains(content, "DroppedFiles()") {
		t.Error("[P0] 2.4-INTG-008: main.go file drop handler must call DroppedFiles()")
	}

	// Must check for .pdf extension
	if !strings.Contains(content, ".pdf") {
		t.Error("[P0] 2.4-INTG-008: main.go file drop handler must filter for .pdf extension")
	}

	// Must import path/filepath for extension check
	if !strings.Contains(content, `"path/filepath"`) {
		t.Error("[P0] 2.4-INTG-008: main.go must import path/filepath for extension filtering")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-009 [P0]: File drop handler emits document:opened or document:error
// AC#2: After processing a dropped file, the Go handler must emit events
//       to the frontend since WindowFilesDropped is Go-side only.
// ---------------------------------------------------------------------------

func TestMainGoFileDropEmitsEvents(t *testing.T) {
	content := readFile(t, "main.go")

	if !strings.Contains(content, `"document:opened"`) {
		t.Error("[P0] 2.4-INTG-009: main.go must emit 'document:opened' event")
	}

	if !strings.Contains(content, `"document:error"`) {
		t.Error("[P0] 2.4-INTG-009: main.go must emit 'document:error' event")
	}
}

// ---------------------------------------------------------------------------
// AC#1: Menu File > Open wired to actual dialog
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-010 [P0]: File > Open menu handler wired to dialog (not just log)
// AC#1: The TODO placeholder in the menu handler must be replaced with
//       actual file dialog logic.
// ---------------------------------------------------------------------------

func TestMenuFileOpenWired(t *testing.T) {
	content := readFile(t, "main.go")

	// The stub "File > Open clicked" log should be gone or the dialog should be present
	if strings.Contains(content, `log.Println("File > Open clicked")`) {
		t.Error("[P0] 2.4-INTG-010: File > Open menu still has TODO stub -- must be wired to dialog")
	}

	// Must have dialog call in the menu handler area
	if !strings.Contains(content, "Dialog.OpenFile()") {
		t.Error("[P0] 2.4-INTG-010: main.go must call Dialog.OpenFile() for File > Open menu")
	}

	// Menu handler must emit document:opened on success
	// (already checked by INTG-009, but verify dialog + emit coexist)
	hasDialog := strings.Contains(content, "Dialog.OpenFile()")
	hasEmit := strings.Contains(content, `"document:opened"`)
	if !hasDialog || !hasEmit {
		t.Error("[P0] 2.4-INTG-010: main.go must wire File > Open to dialog and emit document:opened")
	}
}

// ---------------------------------------------------------------------------
// AC#3: Document display -- root node visible after open
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-011 [P1]: useDocumentState.tsx has OPEN_DOCUMENT reducer implementation
// AC#3: The reducer must handle OPEN_DOCUMENT to transition from empty to loaded.
//       Current stub returns state unchanged -- must be implemented.
// ---------------------------------------------------------------------------

func TestReducerHandlesOpenDocument(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// The stub comment should be gone
	if strings.Contains(content, "Stub reducer") {
		t.Error("[P1] 2.4-INTG-011: useDocumentState.tsx still has stub reducer -- OPEN_DOCUMENT must be implemented")
	}

	// Must handle OPEN_DOCUMENT action type
	if !strings.Contains(content, "'OPEN_DOCUMENT'") && !strings.Contains(content, `"OPEN_DOCUMENT"`) {
		t.Error("[P1] 2.4-INTG-011: reducer must handle OPEN_DOCUMENT action type")
	}

	// AppState must have documentError field
	if !strings.Contains(content, "documentError") {
		t.Error("[P1] 2.4-INTG-011: AppState must have documentError field")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-012 [P1]: TabState has rootNode and rootChildren fields
// AC#3: TabState must include rootNode and rootChildren to hold the Catalog
//       root node and its immediate children after file open.
// ---------------------------------------------------------------------------

func TestTabStateHasRootFields(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	if !strings.Contains(content, "rootNode") {
		t.Error("[P1] 2.4-INTG-012: TabState must have rootNode field")
	}

	if !strings.Contains(content, "rootChildren") {
		t.Error("[P1] 2.4-INTG-012: TabState must have rootChildren field")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-013 [P1]: Reducer handles SET_DOCUMENT_ERROR and DISMISS_ERROR
// AC#4: Error actions must exist for error banner flow.
// ---------------------------------------------------------------------------

func TestReducerHandlesErrorActions(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	if !strings.Contains(content, "SET_DOCUMENT_ERROR") {
		t.Error("[P1] 2.4-INTG-013: reducer must handle SET_DOCUMENT_ERROR action")
	}

	if !strings.Contains(content, "DISMISS_ERROR") {
		t.Error("[P1] 2.4-INTG-013: reducer must handle DISMISS_ERROR action")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-014 [P1]: Reducer handles CLOSE_DOCUMENT action
// AC#1: CLOSE_DOCUMENT must remove the tab and update activeTabId.
// ---------------------------------------------------------------------------

func TestReducerHandlesCloseDocument(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must have non-stub handling for CLOSE_DOCUMENT
	// Check for tab removal logic (filter or splice)
	if !strings.Contains(content, "CLOSE_DOCUMENT") {
		t.Error("[P1] 2.4-INTG-014: reducer must handle CLOSE_DOCUMENT action type")
	}

	// The reducer should modify state for CLOSE_DOCUMENT (not just return state)
	// Look for filter or some mutation logic near CLOSE_DOCUMENT
	if strings.Contains(content, "Stub reducer") {
		t.Error("[P1] 2.4-INTG-014: reducer still has stub -- CLOSE_DOCUMENT must be implemented")
	}
}

// ---------------------------------------------------------------------------
// AC#4: Error handling -- ErrorBanner component
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-015 [P0]: ErrorBanner component exists
// AC#4: frontend/src/components/ErrorBanner.tsx must exist.
// ---------------------------------------------------------------------------

func TestErrorBannerComponentExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/ErrorBanner.tsx") {
		t.Fatal("[P0] 2.4-INTG-015: frontend/src/components/ErrorBanner.tsx does not exist")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-016 [P0]: ErrorBanner has required data-testid attributes
// AC#4: ErrorBanner must have data-testid="error-banner",
//       data-testid="error-banner-message", data-testid="error-banner-dismiss".
// ---------------------------------------------------------------------------

func TestErrorBannerHasTestIDs(t *testing.T) {
	if !fileExists(t, "frontend/src/components/ErrorBanner.tsx") {
		t.Skip("ErrorBanner.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/ErrorBanner.tsx")

	// Story 2-9 made the root testid severity-aware (error-banner / warning-banner)
	// so check for the dynamic pattern and the static child testids.
	testIDs := []string{
		`data-testid={testId}`,
		`data-testid="error-banner-message"`,
		`data-testid="error-banner-dismiss"`,
	}

	for _, id := range testIDs {
		if !strings.Contains(content, id) {
			t.Errorf("[P0] 2.4-INTG-016: ErrorBanner.tsx missing %s", id)
		}
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-017 [P0]: ErrorBanner has role="alert" for accessibility
// AC#4: Screen reader announcement requires role="alert" on the container.
// ---------------------------------------------------------------------------

func TestErrorBannerHasAlertRole(t *testing.T) {
	if !fileExists(t, "frontend/src/components/ErrorBanner.tsx") {
		t.Skip("ErrorBanner.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/ErrorBanner.tsx")

	if !strings.Contains(content, `role="alert"`) {
		t.Error("[P0] 2.4-INTG-017: ErrorBanner must have role=\"alert\" for accessibility")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-018 [P1]: ErrorBanner accepts severity and onDismiss props
// AC#4: Component must accept message, severity, and onDismiss props.
// ---------------------------------------------------------------------------

func TestErrorBannerProps(t *testing.T) {
	if !fileExists(t, "frontend/src/components/ErrorBanner.tsx") {
		t.Skip("ErrorBanner.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/ErrorBanner.tsx")

	// Must define props with message, severity, onDismiss
	if !strings.Contains(content, "message") {
		t.Error("[P1] 2.4-INTG-018: ErrorBanner must accept message prop")
	}
	if !strings.Contains(content, "severity") {
		t.Error("[P1] 2.4-INTG-018: ErrorBanner must accept severity prop")
	}
	if !strings.Contains(content, "onDismiss") {
		t.Error("[P1] 2.4-INTG-018: ErrorBanner must accept onDismiss prop")
	}
}

// ---------------------------------------------------------------------------
// AC#2: App container has data-file-drop-target for window-wide Wails drop
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-019 [P0]: App container has data-file-drop-target attribute
// AC#2: Wails requires this attribute to intercept OS file drops. Placed on
// the root app container so the entire window is a valid drop surface.
// ---------------------------------------------------------------------------

func TestAppHasFileDropTarget(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	if !strings.Contains(content, "data-file-drop-target") {
		t.Error("[P0] 2.4-INTG-019: App.jsx must have data-file-drop-target on root container")
	}
}

// ---------------------------------------------------------------------------
// AC#1, #2, #3: App.jsx listens for backend events
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-020 [P0]: App.jsx listens for document:opened event
// AC#1, #2: Frontend must listen for document:opened events from Go backend
//           (used by both menu File > Open and drag-and-drop paths).
// ---------------------------------------------------------------------------

func TestAppListensForDocumentOpened(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	if !strings.Contains(content, "document:opened") {
		t.Error("[P0] 2.4-INTG-020: App.jsx must listen for 'document:opened' event from backend")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-021 [P0]: App.jsx listens for document:error event
// AC#4: Frontend must listen for document:error events from Go backend.
// ---------------------------------------------------------------------------

func TestAppListensForDocumentError(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	if !strings.Contains(content, "document:error") {
		t.Error("[P0] 2.4-INTG-021: App.jsx must listen for 'document:error' event from backend")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-022 [P0]: App.jsx renders ErrorBanner when documentError is present
// AC#4: ErrorBanner must be conditionally rendered based on state.
// ---------------------------------------------------------------------------

func TestAppRendersErrorBanner(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	if !strings.Contains(content, "ErrorBanner") {
		t.Error("[P0] 2.4-INTG-022: App.jsx must import and render ErrorBanner component")
	}

	if !strings.Contains(content, "documentError") {
		t.Error("[P0] 2.4-INTG-022: App.jsx must check documentError state for ErrorBanner rendering")
	}
}

// ---------------------------------------------------------------------------
// AC#3: MainLayout displays root node after document open
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-023 [P1]: MainLayout shows root node and children in left panel
// AC#3: After file open, the left panel must display the root Catalog node
//       and its immediate children (not just placeholder text).
// ---------------------------------------------------------------------------

func TestMainLayoutDisplaysTreeContent(t *testing.T) {
	content := readFile(t, "frontend/src/components/MainLayout.tsx")

	// MainLayout delegates tree rendering to TreePanel (story 2-5+)
	if !strings.Contains(content, "TreePanel") {
		t.Error("[P1] 2.4-INTG-023: MainLayout must include TreePanel in the left panel")
	}
}

// ---------------------------------------------------------------------------
// AC#1: usePDFService hook exists
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-024 [P1]: usePDFService.ts hook exists
// AC#1: Frontend needs a wrapper to call Wails-generated PDFService bindings.
// ---------------------------------------------------------------------------

func TestUsePDFServiceHookExists(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/usePDFService.ts") {
		t.Fatal("[P1] 2.4-INTG-024: frontend/src/hooks/usePDFService.ts does not exist")
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-025 [P1]: usePDFService exports openPDFFile function
// AC#1: The hook must export an openPDFFile function for the button click path.
// ---------------------------------------------------------------------------

func TestUsePDFServiceExportsOpenPDFFile(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/usePDFService.ts") {
		t.Skip("usePDFService.ts does not exist yet")
	}

	content := readFile(t, "frontend/src/hooks/usePDFService.ts")

	if !strings.Contains(content, "openPDFFile") {
		t.Error("[P1] 2.4-INTG-025: usePDFService.ts must export openPDFFile function")
	}
}

// ---------------------------------------------------------------------------
// AC#4: EmptyState wires open-file button to backend
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-026 [P1]: EmptyState imports and uses dispatch/service
// AC#1, #4: EmptyState button click must call PDFService.OpenFileDialog
//           then openPDFFile, then dispatch OPEN_DOCUMENT or SET_DOCUMENT_ERROR.
// ---------------------------------------------------------------------------

func TestEmptyStateWiresOpenFile(t *testing.T) {
	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must import dispatch hook
	if !strings.Contains(content, "useAppDispatch") {
		t.Error("[P1] 2.4-INTG-026: EmptyState must import useAppDispatch")
	}

	// Must call OpenFileDialog (from Wails bindings) or openPDFFile (from hook)
	if !strings.Contains(content, "OpenFileDialog") && !strings.Contains(content, "openPDFFile") {
		t.Error("[P1] 2.4-INTG-026: EmptyState must call OpenFileDialog or openPDFFile for button click")
	}
}

// ---------------------------------------------------------------------------
// Verification: project compiles and vets
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 2.4-INTG-027 [P0]: go build compiles the full project
// AC#9.1: Full project compiles with story 2-4 changes.
// ---------------------------------------------------------------------------

func TestProjectCompiles(t *testing.T) {
	root := projectRoot(t)

	packages := []string{
		"./internal/...",
		".",
	}
	for _, pkg := range packages {
		cmd := exec.Command("go", "build", pkg)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("[P0] 2.4-INTG-027: go build %s failed:\n%s", pkg, string(output))
		}
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-028 [P0]: go vet passes
// AC#9.2: No vet warnings.
// ---------------------------------------------------------------------------

func TestProjectVet(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P0] 2.4-INTG-028: go vet ./... failed:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-029 [P1]: Existing pdfservice tests still pass
// AC#9.3: No regression in pdfservice.
// ---------------------------------------------------------------------------

func TestPdfserviceNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 2.4-INTG-029: pdfservice test suite failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 2.4-INTG-029: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-030 [P1]: Existing pdfcore tests still pass
// AC#9.4: No regression in pdfcore.
// ---------------------------------------------------------------------------

func TestPdfcoreNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 2.4-INTG-030: pdfcore regression -- tests failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 2.4-INTG-030: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-031 [P1]: TypeScript compiles clean
// AC#9.5: tsc --noEmit from frontend/ passes.
// ---------------------------------------------------------------------------

func TestTypeScriptCompiles(t *testing.T) {
	root := projectRoot(t)
	frontendDir := filepath.Join(root, "frontend")

	cmd := exec.Command("npx", "tsc", "--noEmit")
	cmd.Dir = frontendDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 2.4-INTG-031: tsc --noEmit failed:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-032 [P1]: pdfcore still has zero Wails imports (regression check)
// AC: Architecture compliance carried forward.
// ---------------------------------------------------------------------------

func TestPdfcoreZeroWailsImports(t *testing.T) {
	root := projectRoot(t)

	pdfcoreDir := filepath.Join(root, "internal", "pdfcore")
	entries, err := os.ReadDir(pdfcoreDir)
	if err != nil {
		t.Fatalf("[P1] 2.4-INTG-032: cannot read internal/pdfcore/ directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filePath := filepath.Join(pdfcoreDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("cannot read %s: %v", entry.Name(), err)
			continue
		}
		if strings.Contains(string(content), "wailsapp") {
			t.Errorf("[P1] 2.4-INTG-032: %s imports Wails -- pdfcore must have zero Wails dependencies", entry.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// 2.4-INTG-033 [P1]: App-shell acceptance tests still pass
// AC: Existing app-shell tests must not regress.
// ---------------------------------------------------------------------------

func TestAppShellNoRegression(t *testing.T) {
	root := projectRoot(t)

	appShellDir := filepath.Join(root, "tests", "app-shell")
	cmd := exec.Command("go", "test", "-v", "-count=1", "./...")
	cmd.Dir = appShellDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 2.4-INTG-033: app-shell tests regressed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 2.4-INTG-033: expected PASS in output but got:\n%s", string(output))
	}
}
