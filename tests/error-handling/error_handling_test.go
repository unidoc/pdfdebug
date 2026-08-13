// Package error_handling_test provides acceptance tests for Story 2.9:
// Error Handling and Graceful Degradation.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 2-9 is implemented.
//
// Test Levels:
//   - Structural (Go): file content checks for backend and frontend artifacts
//   - Unit delegation via runPdfcoreTest: verifies pdfcore unit tests exist and pass
//
// Run: go test ./tests/error-handling/... -v -count=1
package error_handling_test

import (
	"errors"
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

// testdataDir returns the absolute path to the testdata/ directory at project root.
func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata")
}

// runPdfcoreTest runs a named test pattern in internal/pdfcore/... and fails if
// the test does not pass or does not exist.
func runPdfcoreTest(t *testing.T, runPattern string) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-v", "-run", runPattern, "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	outStr := string(output)
	if err != nil {
		t.Fatalf("pdfcore test failed:\n%s", outStr)
	}
	if strings.Contains(outStr, "no tests to run") {
		t.Fatalf("no matching test found for pattern %q -- unit test not implemented yet:\n%s", runPattern, outStr)
	}
	if !strings.Contains(outStr, "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", outStr)
	}
}

// ---------------------------------------------------------------------------
// Partial open with warning -- Open() resilient to EnsurePageCount failure
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Open() with malformed PDF stores document and populates DocumentInfo.Error
// with a warning (not a fatal error) when ReadContextFile succeeds but
// EnsurePageCount fails. Delegates to pdfcore unit test.
// ---------------------------------------------------------------------------

func TestOpenPartialSuccessWithWarning(t *testing.T) {
	malformedPDF := filepath.Join(testdataDir(t), "malformed.pdf")
	if _, err := os.Stat(malformedPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/malformed.pdf does not exist -- prerequisite missing")
	}

	runPdfcoreTest(t, "TestOpenPartialSuccess")
}

// ---------------------------------------------------------------------------
// Open() partial success still stores document so
// GetTreeRoot/GetChildren can access it afterward. Delegates to
// pdfcore unit test.
// ---------------------------------------------------------------------------

func TestOpenPartialSuccessDocumentStored(t *testing.T) {
	malformedPDF := filepath.Join(testdataDir(t), "malformed.pdf")
	if _, err := os.Stat(malformedPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/malformed.pdf does not exist -- prerequisite missing")
	}

	runPdfcoreTest(t, "TestOpenPartialSuccessDocStored")
}

// ---------------------------------------------------------------------------
// Error node detail resolution -- GetObjectDetail handles "error:" prefix
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// GetObjectDetail returns ObjectDetail for error-prefixed node IDs (from
// buildChildrenDepth error nodes) instead of returning "unknown node ID
// kind" error. Delegates to pdfcore unit test.
// ---------------------------------------------------------------------------

func TestGetObjectDetailErrorNodePrefix(t *testing.T) {
	runPdfcoreTest(t, "TestGetObjectDetailErrorNode")
}

// ---------------------------------------------------------------------------
// inspector.go has strings.HasPrefix(nodeID, "error:") check in
// GetObjectDetail.
// Error-prefixed nodes must be handled before resolveNodeObject.
// ---------------------------------------------------------------------------

func TestGetObjectDetailErrorPrefixCheck(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read inspector.go: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, `strings.HasPrefix(nodeID, "error:")`) {
		t.Fatalf("inspector.go GetObjectDetail must check for error: prefix")
	}
}

// ---------------------------------------------------------------------------
// Warning propagation in main.go openFileAndEmit
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// main.go propagates docInfo.Error as "warning" field in the
// document:opened event payload.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Frontend warning state in useDocumentState
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// useDocumentState.tsx has documentWarning in AppState.
// ---------------------------------------------------------------------------

func TestStateHasDocumentWarning(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "documentWarning") {
		t.Fatalf("useDocumentState.tsx AppState must have documentWarning field")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState.tsx has SET_DOCUMENT_WARNING action.
// ---------------------------------------------------------------------------

func TestStateHasSetDocumentWarningAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "SET_DOCUMENT_WARNING") {
		t.Fatalf("useDocumentState.tsx must have SET_DOCUMENT_WARNING action")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState.tsx has DISMISS_WARNING action.
// ---------------------------------------------------------------------------

func TestStateHasDismissWarningAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "DISMISS_WARNING") {
		t.Fatalf("useDocumentState.tsx must have DISMISS_WARNING action")
	}
}

// ---------------------------------------------------------------------------
// ErrorBanner severity-specific enhancements
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// ErrorBanner.tsx uses severity-aware data-testid: "warning-banner" for
// warning, "error-banner" for error.
// ---------------------------------------------------------------------------

func TestErrorBannerSeverityTestId(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ErrorBanner.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read ErrorBanner.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "warning-banner") {
		t.Fatalf("ErrorBanner.tsx must use data-testid='warning-banner' for warning severity")
	}
}

// ---------------------------------------------------------------------------
// ErrorBanner.tsx has severity icon -- (!) for warning and (x) for error.
// ---------------------------------------------------------------------------

func TestErrorBannerSeverityIcon(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ErrorBanner.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read ErrorBanner.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "(!)") {
		t.Fatalf("ErrorBanner.tsx must have (!) icon for warning severity")
	}
	if !strings.Contains(src, "(x)") {
		t.Fatalf("ErrorBanner.tsx must have (x) icon for error severity")
	}
}

// ---------------------------------------------------------------------------
// ErrorBanner.tsx has severity-aware aria-label on
// dismiss button ("Dismiss warning" / "Dismiss error").
// ---------------------------------------------------------------------------

func TestErrorBannerDismissAriaLabel(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ErrorBanner.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read ErrorBanner.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "Dismiss warning") {
		t.Fatalf("ErrorBanner.tsx dismiss button must have aria-label 'Dismiss warning' for warning severity")
	}
}

// ---------------------------------------------------------------------------
// ErrorBanner.tsx uses tinted-light backgrounds (no dark: variants by
// design). See in-file rationale below.
// ---------------------------------------------------------------------------

func TestErrorBannerHasTintedBackgrounds(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ErrorBanner.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read ErrorBanner.tsx: %v", err)
	}
	src := string(content)
	// Re-pinned 2026-05-22 (Epic 9 retro): the original dark:bg-* pin was stale.
	// ErrorBanner.tsx deliberately drops `dark:` Tailwind variants because the
	// app shell uses design-token CSS that does not flip on prefers-color-scheme;
	// a tinted banner with dark: variants would mismatch surrounding chrome.
	// Contract now asserts the documented decision is preserved.
	if !strings.Contains(src, "bg-red-100") {
		t.Fatalf("ErrorBanner.tsx must use bg-red-100 for error background")
	}
	if !strings.Contains(src, "bg-amber-100") {
		t.Fatalf("ErrorBanner.tsx must use bg-amber-100 for warning background")
	}
	if strings.Contains(src, "dark:bg-red") || strings.Contains(src, "dark:bg-amber") {
		t.Fatalf("ErrorBanner.tsx must NOT reintroduce dark:bg-* variants (mismatches app shell tokens; see in-file rationale)")
	}
}

// ---------------------------------------------------------------------------
// App.jsx warning banner rendering
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// App.jsx reads documentWarning from state and renders a warning
// ErrorBanner with severity="warning".
// ---------------------------------------------------------------------------

func TestAppJsxWarningBanner(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "App.jsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read App.jsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "documentWarning") {
		t.Fatalf("App.jsx must read documentWarning from state")
	}
	if !strings.Contains(src, "DISMISS_WARNING") {
		t.Fatalf("App.jsx must dispatch DISMISS_WARNING on warning banner dismiss")
	}
}

// ---------------------------------------------------------------------------
// App.jsx document:opened event listener reads data.warning and
// dispatches SET_DOCUMENT_WARNING.
// ---------------------------------------------------------------------------

func TestAppJsxEventWarningPropagation(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "App.jsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read App.jsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "SET_DOCUMENT_WARNING") {
		t.Fatalf("App.jsx must dispatch SET_DOCUMENT_WARNING from document:opened event")
	}
}

// ---------------------------------------------------------------------------
// Warning propagation via usePDFService.ts (EmptyState code path)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// usePDFService.ts OpenPDFResult has warning field.
// ---------------------------------------------------------------------------

func TestUsePDFServiceWarningField(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "usePDFService.ts")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read usePDFService.ts: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "warning") {
		t.Fatalf("usePDFService.ts OpenPDFResult must have warning field")
	}
}

// ---------------------------------------------------------------------------
// Panic recovery coverage -- all pdfcpu calls wrapped in safeCall
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// safeCall catches panic during tree traversal and returns error node.
// Delegates to existing pdfcore test.
// ---------------------------------------------------------------------------

func TestSafeCallPanicRecovery(t *testing.T) {
	runPdfcoreTest(t, "TestSafeCallCatches")
}

// ---------------------------------------------------------------------------
// Error nodes from buildChildrenDepth have correct "error:"-prefixed
// IDs. Delegates to existing pdfcore test.
// ---------------------------------------------------------------------------

func TestErrorNodeCreation(t *testing.T) {
	runPdfcoreTest(t, "TestErrorNodeCreation")
}

// ---------------------------------------------------------------------------
// Error node siblings remain navigable when one child fails. Delegates
// to existing pdfcore test.
// ---------------------------------------------------------------------------

func TestErrorNodeSiblingSurvival(t *testing.T) {
	runPdfcoreTest(t, "TestErrorNodeSiblingSurvival")
}
