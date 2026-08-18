// 4-5: deleted TestAppShellNoRegression (meta-test that subprocess-ran the
//      app-shell suite; CI already runs that suite -- pure overhead).

// Package open_pdf_dialog_dnd_test provides acceptance tests for Open PDF via
// File Dialog and Drag-and-Drop.
//
// Test Levels: Integration (Go) -- source file content parsing, structural validation.
// Frontend state/UI tests are Vitest (/002/003) or E2E.
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
// File dialog -- PDFService.OpenFileDialog method + SetApp pattern
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// PDFService has OpenFileDialog method: Given PDFService, When
// reviewed, Then it has an exported
//       OpenFileDialog() method that returns (string, error).
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// PDFService has SetApp method and app field: PDFService needs access
// to *application.App for dialog calls.
//       SetApp(app *application.App) method and app field must exist.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// PDFService imports wails application package: pdfservice is the Wails
// adapter layer -- it MAY import Wails packages
//       (unlike pdfcore which must have zero Wails deps). Here,
//       service.go must import application for *application.App type.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// main.go creates PDFService with app after application.New()
// ---------------------------------------------------------------------------

// TestMainGoCallsSetApp was a source-grep asserting
// `pdfservice.NewPDFService(app)` appears between `application.New(` and
// `app.Run()`. Replaced by tests/boot-smoke (boot path runs to event loop
// without panic; the registration crashing or being misordered would surface
// as a panic).

// ---------------------------------------------------------------------------
// Drag-and-drop -- EnableFileDrop + window event handler
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// main.go enables file drop on window: WebviewWindowOptions
// must include EnableFileDrop: true.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// main.go captures window reference: Window return value from NewWithOptions
// must be captured (not discarded)
//       to register OnWindowEvent handler for file drop.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// main.go registers OnWindowEvent for file drop: Go-side window event
// handler for WindowFilesDropped must exist.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// File drop handler filters for .pdf extension: The drop handler must
// filter dropped files for .pdf extension.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// File drop handler emits document:opened or document:error: After processing
// a dropped file, the Go handler must emit events
//       to the frontend since WindowFilesDropped is Go-side only.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Menu File > Open wired to actual dialog
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// File > Open menu handler wired to dialog (not just log): The TODO
// placeholder in the menu handler must be replaced with
//       actual file dialog logic.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Document display -- root node visible after open
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// useDocumentState.tsx has OPEN_DOCUMENT reducer implementation: The reducer must
// handle OPEN_DOCUMENT to transition from empty to loaded.
//       Current stub returns state unchanged -- must be implemented.
// ---------------------------------------------------------------------------

func TestReducerHandlesOpenDocument(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// The stub comment should be gone
	if strings.Contains(content, "Stub reducer") {
		t.Error("useDocumentState.tsx still has stub reducer -- OPEN_DOCUMENT must be implemented")
	}

	// Must handle OPEN_DOCUMENT action type
	if !strings.Contains(content, "'OPEN_DOCUMENT'") && !strings.Contains(content, `"OPEN_DOCUMENT"`) {
		t.Error("reducer must handle OPEN_DOCUMENT action type")
	}

	// AppState must have documentError field
	if !strings.Contains(content, "documentError") {
		t.Error("AppState must have documentError field")
	}
}

// ---------------------------------------------------------------------------
// TabState has rootNode and rootChildren fields: TabState must include
// rootNode and rootChildren to hold the Catalog
//       root node and its immediate children after file open.
// ---------------------------------------------------------------------------

func TestTabStateHasRootFields(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	if !strings.Contains(content, "rootNode") {
		t.Error("TabState must have rootNode field")
	}

	if !strings.Contains(content, "rootChildren") {
		t.Error("TabState must have rootChildren field")
	}
}

// ---------------------------------------------------------------------------
// Reducer handles SET_DOCUMENT_ERROR and DISMISS_ERROR: Error actions
// must exist for error banner flow.
// ---------------------------------------------------------------------------

func TestReducerHandlesErrorActions(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	if !strings.Contains(content, "SET_DOCUMENT_ERROR") {
		t.Error("reducer must handle SET_DOCUMENT_ERROR action")
	}

	if !strings.Contains(content, "DISMISS_ERROR") {
		t.Error("reducer must handle DISMISS_ERROR action")
	}
}

// ---------------------------------------------------------------------------
// Reducer handles CLOSE_DOCUMENT action: CLOSE_DOCUMENT must
// remove the tab and update activeTabId.
// ---------------------------------------------------------------------------

func TestReducerHandlesCloseDocument(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must have non-stub handling for CLOSE_DOCUMENT
	// Check for tab removal logic (filter or splice)
	if !strings.Contains(content, "CLOSE_DOCUMENT") {
		t.Error("reducer must handle CLOSE_DOCUMENT action type")
	}

	// The reducer should modify state for CLOSE_DOCUMENT (not just return state)
	// Look for filter or some mutation logic near CLOSE_DOCUMENT
	if strings.Contains(content, "Stub reducer") {
		t.Error("reducer still has stub -- CLOSE_DOCUMENT must be implemented")
	}
}

// ---------------------------------------------------------------------------
// Error handling -- ErrorBanner component
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// ErrorBanner component exists:
// frontend/src/components/ErrorBanner.tsx must exist.
// ---------------------------------------------------------------------------

func TestErrorBannerComponentExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/ErrorBanner.tsx") {
		t.Fatal("frontend/src/components/ErrorBanner.tsx does not exist")
	}
}

// ---------------------------------------------------------------------------
// ErrorBanner has required data-testid attributes: ErrorBanner must
// have data-testid="error-banner",
//       data-testid="error-banner-message", data-testid="error-banner-dismiss".
// ---------------------------------------------------------------------------

func TestErrorBannerHasTestIDs(t *testing.T) {
	if !fileExists(t, "frontend/src/components/ErrorBanner.tsx") {
		t.Skip("ErrorBanner.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/ErrorBanner.tsx")

	// Made the root testid severity-aware (error-banner / warning-banner)
	// so check for the dynamic pattern and the static child testids.
	testIDs := []string{
		`data-testid={testId}`,
		`data-testid="error-banner-message"`,
		`data-testid="error-banner-dismiss"`,
	}

	for _, id := range testIDs {
		if !strings.Contains(content, id) {
			t.Errorf("ErrorBanner.tsx missing %s", id)
		}
	}
}

// ---------------------------------------------------------------------------
// ErrorBanner has role="alert" for accessibility: Screen reader
// announcement requires role="alert" on the container.
// ---------------------------------------------------------------------------

func TestErrorBannerHasAlertRole(t *testing.T) {
	if !fileExists(t, "frontend/src/components/ErrorBanner.tsx") {
		t.Skip("ErrorBanner.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/ErrorBanner.tsx")

	if !strings.Contains(content, `role="alert"`) {
		t.Error("ErrorBanner must have role=\"alert\" for accessibility")
	}
}

// ---------------------------------------------------------------------------
// ErrorBanner accepts severity and onDismiss props: Component must
// accept message, severity, and onDismiss props.
// ---------------------------------------------------------------------------

func TestErrorBannerProps(t *testing.T) {
	if !fileExists(t, "frontend/src/components/ErrorBanner.tsx") {
		t.Skip("ErrorBanner.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/ErrorBanner.tsx")

	// Must define props with message, severity, onDismiss
	if !strings.Contains(content, "message") {
		t.Error("ErrorBanner must accept message prop")
	}
	if !strings.Contains(content, "severity") {
		t.Error("ErrorBanner must accept severity prop")
	}
	if !strings.Contains(content, "onDismiss") {
		t.Error("ErrorBanner must accept onDismiss prop")
	}
}

// ---------------------------------------------------------------------------
// App container has data-file-drop-target for window-wide Wails drop
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// App container has data-file-drop-target attribute: Wails requires this
// attribute to intercept OS file drops. Placed on the root app container so
// the entire window is a valid drop surface.
// ---------------------------------------------------------------------------

func TestAppHasFileDropTarget(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	if !strings.Contains(content, "data-file-drop-target") {
		t.Error("App.jsx must have data-file-drop-target on root container")
	}
}

// ---------------------------------------------------------------------------
// App.jsx listens for backend events
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// App.jsx listens for document:opened event: Frontend must listen for
// document:opened events from Go backend
//           (used by both menu File > Open and drag-and-drop paths).
// ---------------------------------------------------------------------------

func TestAppListensForDocumentOpened(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	if !strings.Contains(content, "document:opened") {
		t.Error("App.jsx must listen for 'document:opened' event from backend")
	}
}

// ---------------------------------------------------------------------------
// App.jsx listens for document:error event: Frontend must listen for
// document:error events from Go backend.
// ---------------------------------------------------------------------------

func TestAppListensForDocumentError(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	if !strings.Contains(content, "document:error") {
		t.Error("App.jsx must listen for 'document:error' event from backend")
	}
}

// ---------------------------------------------------------------------------
// App.jsx renders ErrorBanner when documentError is present: ErrorBanner must
// be conditionally rendered based on state.
// ---------------------------------------------------------------------------

func TestAppRendersErrorBanner(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	if !strings.Contains(content, "ErrorBanner") {
		t.Error("App.jsx must import and render ErrorBanner component")
	}

	if !strings.Contains(content, "documentError") {
		t.Error("App.jsx must check documentError state for ErrorBanner rendering")
	}
}

// ---------------------------------------------------------------------------
// MainLayout displays root node after document open
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// MainLayout shows root node and children in left panel: After file open,
// the left panel must display the root Catalog node
//       and its immediate children (not just placeholder text).
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// usePDFService hook exists
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// usePDFService.ts hook exists: Frontend needs a wrapper to call
// Wails-generated PDFService bindings.
// ---------------------------------------------------------------------------

func TestUsePDFServiceHookExists(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/usePDFService.ts") {
		t.Fatal("frontend/src/hooks/usePDFService.ts does not exist")
	}
}

// ---------------------------------------------------------------------------
// usePDFService exports openPDFFile function: The hook must export an
// openPDFFile function for the button click path.
// ---------------------------------------------------------------------------

func TestUsePDFServiceExportsOpenPDFFile(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/usePDFService.ts") {
		t.Skip("usePDFService.ts does not exist yet")
	}

	content := readFile(t, "frontend/src/hooks/usePDFService.ts")

	if !strings.Contains(content, "openPDFFile") {
		t.Error("usePDFService.ts must export openPDFFile function")
	}
}

// ---------------------------------------------------------------------------
// EmptyState wires open-file button to backend
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// EmptyState imports and uses dispatch/service: EmptyState button click
// must call PDFService.OpenFileDialog
//           then openPDFFile, then dispatch OPEN_DOCUMENT or SET_DOCUMENT_ERROR.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Verification: project compiles and vets
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// go build compiles the full project: Full project
// compiles.
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
			t.Fatalf("go build %s failed:\n%s", pkg, string(output))
		}
	}
}

// ---------------------------------------------------------------------------
// go vet passes: No vet warnings.
// ---------------------------------------------------------------------------

func TestProjectVet(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet ./... failed:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Existing pdfservice tests still pass: No regression in
// pdfservice.
// ---------------------------------------------------------------------------

func TestPdfserviceNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfservice test suite failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Existing pdfcore tests still pass: No regression in
// pdfcore.
// ---------------------------------------------------------------------------

func TestPdfcoreNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfcore regression -- tests failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// TypeScript compiles clean: tsc --noEmit from
// frontend/ passes.
// ---------------------------------------------------------------------------

func TestTypeScriptCompiles(t *testing.T) {
	root := projectRoot(t)
	frontendDir := filepath.Join(root, "frontend")

	cmd := exec.Command("npx", "tsc", "--noEmit")
	cmd.Dir = frontendDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tsc --noEmit failed:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Pdfcore still has zero Wails imports (regression check) AC: Architecture
// compliance carried forward.
// ---------------------------------------------------------------------------

func TestPdfcoreZeroWailsImports(t *testing.T) {
	root := projectRoot(t)

	pdfcoreDir := filepath.Join(root, "internal", "pdfcore")
	entries, err := os.ReadDir(pdfcoreDir)
	if err != nil {
		t.Fatalf("cannot read internal/pdfcore/ directory: %v", err)
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
			t.Errorf("%s imports Wails -- pdfcore must have zero Wails dependencies", entry.Name())
		}
	}
}

// TestAppShellNoRegression was a meta-test that subprocess-ran
// the app-shell suite. CI already runs that suite, so this is pure overhead
// and a flake-risk amplifier. Delete-only, no replacement.
