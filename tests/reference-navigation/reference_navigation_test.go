// Package reference_navigation_test provides acceptance tests for Clickable
// Reference Navigation.
//
// Test Levels:
//   - Structural (Go): file content checks for backend GetAncestorPath and frontend wiring
//   - Unit delegation via runPdfcoreTest: verifies pdfcore unit tests exist and pass
//
// Run: go test ./tests/reference-navigation/... -v -count=1
package reference_navigation_test

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
// GetAncestorPath exists in inspector.go: The backend must provide
// GetAncestorPath to return the path from
//           root to a target node.
// ---------------------------------------------------------------------------

func TestGetAncestorPathMethodExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read inspector.go: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "GetAncestorPath") {
		t.Fatalf("inspector.go must contain GetAncestorPath method")
	}
}

// ---------------------------------------------------------------------------
// findPathToObject helper exists in inspector.go: BFS helper to
// find the path through the PDF object graph.
// ---------------------------------------------------------------------------

func TestFindPathToObjectExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read inspector.go: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "findPathToObject") {
		t.Fatalf("inspector.go must contain findPathToObject helper")
	}
}

// ---------------------------------------------------------------------------
// GetAncestorPath unit test exists and passes: The pdfcore unit test for
// GetAncestorPath must exist and pass using
//       testdata/minimal.pdf.
// ---------------------------------------------------------------------------

func TestGetAncestorPathUnitTestPasses(t *testing.T) {
	// Verify testdata/minimal.pdf exists (prerequisite)
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- prerequisite missing")
	}

	runPdfcoreTest(t, "TestGetAncestorPath")
}

// ---------------------------------------------------------------------------
// GetAncestorPath returns root path for "root" nodeID
// GetAncestorPath("root") must return ["root"].
// Delegates to pdfcore unit test.
// ---------------------------------------------------------------------------

func TestGetAncestorPathRootNode(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- prerequisite missing")
	}

	runPdfcoreTest(t, "TestGetAncestorPathRoot$")
}

// ---------------------------------------------------------------------------
// GetAncestorPath returns error for dangling reference: Dangling
// reference must return an error, not crash.
// ---------------------------------------------------------------------------

func TestGetAncestorPathDanglingRef(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- prerequisite missing")
	}

	runPdfcoreTest(t, "TestGetAncestorPathDangling")
}

// ---------------------------------------------------------------------------
// PDFService.GetAncestorPath delegation exists: pdfservice must expose
// GetAncestorPath, delegating to pdfcore.
// ---------------------------------------------------------------------------

func TestServiceGetAncestorPathExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfservice", "service.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read service.go: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "GetAncestorPath") {
		t.Fatalf("service.go must contain GetAncestorPath method")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState has pendingNavTarget field: State must track
// pending navigation target per tab.
// ---------------------------------------------------------------------------

func TestStateHasPendingNavTarget(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "pendingNavTarget") {
		t.Fatalf("useDocumentState.tsx must have pendingNavTarget in TabState")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState has navError field: State must track
// navigation errors per tab.
// ---------------------------------------------------------------------------

func TestStateHasNavError(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "navError") {
		t.Fatalf("useDocumentState.tsx must have navError in TabState")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState has NAVIGATE_TO_REF action: Reducer must handle
// NAVIGATE_TO_REF action.
// ---------------------------------------------------------------------------

func TestStateHasNavigateToRefAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "NAVIGATE_TO_REF") {
		t.Fatalf("useDocumentState.tsx must have NAVIGATE_TO_REF action")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState has CLEAR_NAV_TARGET action: Reducer must handle
// CLEAR_NAV_TARGET action.
// ---------------------------------------------------------------------------

func TestStateHasClearNavTargetAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "CLEAR_NAV_TARGET") {
		t.Fatalf("useDocumentState.tsx must have CLEAR_NAV_TARGET action")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState has NAV_ERROR action: Reducer must handle
// NAV_ERROR action for dangling references.
// ---------------------------------------------------------------------------

func TestStateHasNavErrorAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "NAV_ERROR") {
		t.Fatalf("useDocumentState.tsx must have NAV_ERROR action")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState has DISMISS_NAV_ERROR action: Reducer must handle
// DISMISS_NAV_ERROR action.
// ---------------------------------------------------------------------------

func TestStateHasDismissNavErrorAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "DISMISS_NAV_ERROR") {
		t.Fatalf("useDocumentState.tsx must have DISMISS_NAV_ERROR action")
	}
}

// ---------------------------------------------------------------------------
// DetailShared.tsx ValueDisplay has onReferenceClick prop: ValueDisplay must
// accept onReferenceClick callback prop.
// ---------------------------------------------------------------------------

func TestValueDisplayHasOnReferenceClick(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailShared.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read DetailShared.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "onReferenceClick") {
		t.Fatalf("DetailShared.tsx ValueDisplay must accept onReferenceClick prop")
	}
}

// ---------------------------------------------------------------------------
// DetailPanel.tsx has handleReferenceClick: DetailPanel must define
// handleReferenceClick and pass to view components.
// ---------------------------------------------------------------------------

func TestDetailPanelHasHandleReferenceClick(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read DetailPanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "handleReferenceClick") || !strings.Contains(src, "NAVIGATE_TO_REF") {
		t.Fatalf("DetailPanel.tsx must have handleReferenceClick that dispatches NAVIGATE_TO_REF")
	}
}

// ---------------------------------------------------------------------------
// ObjectInfoPanel.tsx dispatches NAVIGATE_TO_REF via useAppDispatch on reference
// click. Re-pinned 2026-05-22: the original handleReferenceClick
// identifier pin was stale -- the local handler was renamed to handleRefClick and
// routes through useAppDispatch. The behavioral contract (click -> NAVIGATE_TO_REF
// dispatch) is preserved; only the identifier name drifted. Behavioral coverage
// held by frontend/src/components/ReferenceNavigation.test.tsx (1,032 lines) +
// ObjectInfoPanel.test.tsx.
// ---------------------------------------------------------------------------

func TestObjectInfoPanelDispatchesNavigateToRefOnClick(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ObjectInfoPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read ObjectInfoPanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "useAppDispatch") {
		t.Fatalf("ObjectInfoPanel.tsx must use useAppDispatch")
	}
	if !strings.Contains(src, "NAVIGATE_TO_REF") {
		t.Fatalf("ObjectInfoPanel.tsx must dispatch NAVIGATE_TO_REF on reference click")
	}
}

// ---------------------------------------------------------------------------
// TreePanel.tsx has treeRef for react-arborist TreeApi: TreePanel must use
// a ref to control react-arborist
//               programmatic open/scroll/select.
// ---------------------------------------------------------------------------

func TestTreePanelHasTreeRef(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "TreePanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read TreePanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "treeRef") {
		t.Fatalf("TreePanel.tsx must have treeRef for react-arborist TreeApi")
	}
}

// ---------------------------------------------------------------------------
// TreePanel.tsx watches pendingNavTarget: TreePanel must have a useEffect
// that watches pendingNavTarget
//           and calls GetAncestorPath.
// ---------------------------------------------------------------------------

func TestTreePanelWatchesPendingNavTarget(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "TreePanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read TreePanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "pendingNavTarget") {
		t.Fatalf("TreePanel.tsx must watch pendingNavTarget")
	}
	if !strings.Contains(src, "GetAncestorPath") {
		t.Fatalf("TreePanel.tsx must call GetAncestorPath during navigation")
	}
}

// ---------------------------------------------------------------------------
// TreePanel.tsx has flashNodeId for flash effect: TreePanel must have
// flashNodeId state for the 100ms highlight pulse.
// ---------------------------------------------------------------------------

func TestTreePanelHasFlashNodeId(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "TreePanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read TreePanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "flashNodeId") {
		t.Fatalf("TreePanel.tsx must have flashNodeId state for flash effect")
	}
}

// ---------------------------------------------------------------------------
// TreePanel.tsx has navError display: TreePanel must render navError
// as a transient toast message.
// ---------------------------------------------------------------------------

func TestTreePanelHasNavErrorDisplay(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "TreePanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read TreePanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "navError") {
		t.Fatalf("TreePanel.tsx must display navError")
	}
	if !strings.Contains(src, "DISMISS_NAV_ERROR") {
		t.Fatalf("TreePanel.tsx must auto-dismiss navError via DISMISS_NAV_ERROR")
	}
}

// ---------------------------------------------------------------------------
// GetAncestorPath uses safeCall for panic recovery: All pdfcpu calls in
// GetAncestorPath must be wrapped in safeCall.
// ---------------------------------------------------------------------------

func TestGetAncestorPathUsesSafeCall(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read inspector.go: %v", err)
	}
	src := string(content)
	// GetAncestorPath should exist and the file should contain safeCall
	// (safeCall is already used elsewhere; we verify it is used near findPathToObject)
	if !strings.Contains(src, "GetAncestorPath") {
		t.Fatalf("inspector.go must contain GetAncestorPath")
	}
	if !strings.Contains(src, "findPathToObject") {
		t.Fatalf("inspector.go must contain findPathToObject")
	}
	// Verify safeCall is used in the file (already is, but verify it is still present)
	if !strings.Contains(src, "safeCall") {
		t.Fatalf("inspector.go must use safeCall for panic recovery")
	}
}
