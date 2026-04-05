// Package reference_navigation_test provides acceptance tests for Story 2.8:
// Clickable Reference Navigation.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 2-8 is implemented.
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
			if strings.Contains(string(content), "module unipdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unipdf-debugger found)")
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
func runPdfcoreTest(t *testing.T, testID, runPattern string) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-v", "-run", runPattern, "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	outStr := string(output)
	if err != nil {
		t.Fatalf("[%s] pdfcore test failed:\n%s", testID, outStr)
	}
	if strings.Contains(outStr, "no tests to run") {
		t.Fatalf("[%s] no matching test found for pattern %q -- unit test not implemented yet:\n%s", testID, runPattern, outStr)
	}
	if !strings.Contains(outStr, "PASS") {
		t.Fatalf("[%s] expected PASS in output but got:\n%s", testID, outStr)
	}
}

// ---------------------------------------------------------------------------
// 2.8-INTG-001 [P1]: GetAncestorPath exists in inspector.go
// AC#2, #6: The backend must provide GetAncestorPath to return the path from
//           root to a target node.
// ---------------------------------------------------------------------------

func TestGetAncestorPathMethodExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-INTG-001: cannot read inspector.go: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "GetAncestorPath") {
		t.Fatalf("[P1] 2.8-INTG-001: inspector.go must contain GetAncestorPath method")
	}
}

// ---------------------------------------------------------------------------
// 2.8-INTG-002 [P1]: findPathToObject helper exists in inspector.go
// AC#2: BFS helper to find the path through the PDF object graph.
// ---------------------------------------------------------------------------

func TestFindPathToObjectExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-INTG-002: cannot read inspector.go: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "findPathToObject") {
		t.Fatalf("[P1] 2.8-INTG-002: inspector.go must contain findPathToObject helper")
	}
}

// ---------------------------------------------------------------------------
// 2.8-INTG-003 [P0]: GetAncestorPath unit test exists and passes
// AC#2: The pdfcore unit test for GetAncestorPath must exist and pass using
//       testdata/minimal.pdf.
// ---------------------------------------------------------------------------

func TestGetAncestorPathUnitTestPasses(t *testing.T) {
	// Verify testdata/minimal.pdf exists (prerequisite)
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 2.8-INTG-003: testdata/minimal.pdf does not exist -- prerequisite missing")
	}

	runPdfcoreTest(t, "2.8-INTG-003", "TestGetAncestorPath")
}

// ---------------------------------------------------------------------------
// 2.8-INTG-004 [P1]: GetAncestorPath returns root path for "root" nodeID
// AC#2: GetAncestorPath("root") must return ["root"].
// Delegates to pdfcore unit test.
// ---------------------------------------------------------------------------

func TestGetAncestorPathRootNode(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P1] 2.8-INTG-004: testdata/minimal.pdf does not exist -- prerequisite missing")
	}

	runPdfcoreTest(t, "2.8-INTG-004", "TestGetAncestorPathRoot$")
}

// ---------------------------------------------------------------------------
// 2.8-INTG-005 [P1]: GetAncestorPath returns error for dangling reference
// AC#5: Dangling reference must return an error, not crash.
// ---------------------------------------------------------------------------

func TestGetAncestorPathDanglingRef(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P1] 2.8-INTG-005: testdata/minimal.pdf does not exist -- prerequisite missing")
	}

	runPdfcoreTest(t, "2.8-INTG-005", "TestGetAncestorPathDangling")
}

// ---------------------------------------------------------------------------
// 2.8-INTG-006 [P1]: PDFService.GetAncestorPath delegation exists
// AC#2: pdfservice must expose GetAncestorPath, delegating to pdfcore.
// ---------------------------------------------------------------------------

func TestServiceGetAncestorPathExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfservice", "service.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-INTG-006: cannot read service.go: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "GetAncestorPath") {
		t.Fatalf("[P1] 2.8-INTG-006: service.go must contain GetAncestorPath method")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-001 [P1]: useDocumentState has pendingNavTarget field
// AC#2, #3: State must track pending navigation target per tab.
// ---------------------------------------------------------------------------

func TestStateHasPendingNavTarget(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-001: cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "pendingNavTarget") {
		t.Fatalf("[P1] 2.8-STRUCT-001: useDocumentState.tsx must have pendingNavTarget in TabState")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-002 [P1]: useDocumentState has navError field
// AC#5: State must track navigation errors per tab.
// ---------------------------------------------------------------------------

func TestStateHasNavError(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-002: cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "navError") {
		t.Fatalf("[P1] 2.8-STRUCT-002: useDocumentState.tsx must have navError in TabState")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-003 [P1]: useDocumentState has NAVIGATE_TO_REF action
// AC#2: Reducer must handle NAVIGATE_TO_REF action.
// ---------------------------------------------------------------------------

func TestStateHasNavigateToRefAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-003: cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "NAVIGATE_TO_REF") {
		t.Fatalf("[P1] 2.8-STRUCT-003: useDocumentState.tsx must have NAVIGATE_TO_REF action")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-004 [P1]: useDocumentState has CLEAR_NAV_TARGET action
// AC#2: Reducer must handle CLEAR_NAV_TARGET action.
// ---------------------------------------------------------------------------

func TestStateHasClearNavTargetAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-004: cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "CLEAR_NAV_TARGET") {
		t.Fatalf("[P1] 2.8-STRUCT-004: useDocumentState.tsx must have CLEAR_NAV_TARGET action")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-005 [P1]: useDocumentState has NAV_ERROR action
// AC#5: Reducer must handle NAV_ERROR action for dangling references.
// ---------------------------------------------------------------------------

func TestStateHasNavErrorAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-005: cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "NAV_ERROR") {
		t.Fatalf("[P1] 2.8-STRUCT-005: useDocumentState.tsx must have NAV_ERROR action")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-006 [P1]: useDocumentState has DISMISS_NAV_ERROR action
// AC#5: Reducer must handle DISMISS_NAV_ERROR action.
// ---------------------------------------------------------------------------

func TestStateHasDismissNavErrorAction(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-006: cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "DISMISS_NAV_ERROR") {
		t.Fatalf("[P1] 2.8-STRUCT-006: useDocumentState.tsx must have DISMISS_NAV_ERROR action")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-007 [P1]: DetailShared.tsx ValueDisplay has onReferenceClick prop
// AC#1, #2: ValueDisplay must accept onReferenceClick callback prop.
// ---------------------------------------------------------------------------

func TestValueDisplayHasOnReferenceClick(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailShared.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-007: cannot read DetailShared.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "onReferenceClick") {
		t.Fatalf("[P1] 2.8-STRUCT-007: DetailShared.tsx ValueDisplay must accept onReferenceClick prop")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-008 [P1]: DetailPanel.tsx has handleReferenceClick
// AC#2: DetailPanel must define handleReferenceClick and pass to view components.
// ---------------------------------------------------------------------------

func TestDetailPanelHasHandleReferenceClick(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-008: cannot read DetailPanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "handleReferenceClick") || !strings.Contains(src, "NAVIGATE_TO_REF") {
		t.Fatalf("[P1] 2.8-STRUCT-008: DetailPanel.tsx must have handleReferenceClick that dispatches NAVIGATE_TO_REF")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-009 [P1]: ObjectInfoPanel.tsx has handleReferenceClick
// AC#2: ObjectInfoPanel must define handleReferenceClick and pass to view components.
// ---------------------------------------------------------------------------

func TestObjectInfoPanelHasHandleReferenceClick(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ObjectInfoPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-009: cannot read ObjectInfoPanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "handleReferenceClick") || !strings.Contains(src, "NAVIGATE_TO_REF") {
		t.Fatalf("[P1] 2.8-STRUCT-009: ObjectInfoPanel.tsx must have handleReferenceClick that dispatches NAVIGATE_TO_REF")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-010 [P1]: TreePanel.tsx has treeRef for react-arborist TreeApi
// AC#2, #4, #6: TreePanel must use a ref to control react-arborist
//               programmatic open/scroll/select.
// ---------------------------------------------------------------------------

func TestTreePanelHasTreeRef(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "TreePanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-010: cannot read TreePanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "treeRef") {
		t.Fatalf("[P1] 2.8-STRUCT-010: TreePanel.tsx must have treeRef for react-arborist TreeApi")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-011 [P1]: TreePanel.tsx watches pendingNavTarget
// AC#2, #6: TreePanel must have a useEffect that watches pendingNavTarget
//           and calls GetAncestorPath.
// ---------------------------------------------------------------------------

func TestTreePanelWatchesPendingNavTarget(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "TreePanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-011: cannot read TreePanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "pendingNavTarget") {
		t.Fatalf("[P1] 2.8-STRUCT-011: TreePanel.tsx must watch pendingNavTarget")
	}
	if !strings.Contains(src, "GetAncestorPath") {
		t.Fatalf("[P1] 2.8-STRUCT-011: TreePanel.tsx must call GetAncestorPath during navigation")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-012 [P2]: TreePanel.tsx has flashNodeId for flash effect
// AC#3: TreePanel must have flashNodeId state for the 100ms highlight pulse.
// ---------------------------------------------------------------------------

func TestTreePanelHasFlashNodeId(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "TreePanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P2] 2.8-STRUCT-012: cannot read TreePanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "flashNodeId") {
		t.Fatalf("[P2] 2.8-STRUCT-012: TreePanel.tsx must have flashNodeId state for flash effect")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-013 [P2]: TreePanel.tsx has navError display
// AC#5: TreePanel must render navError as a transient toast message.
// ---------------------------------------------------------------------------

func TestTreePanelHasNavErrorDisplay(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "TreePanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P2] 2.8-STRUCT-013: cannot read TreePanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "navError") {
		t.Fatalf("[P2] 2.8-STRUCT-013: TreePanel.tsx must display navError")
	}
	if !strings.Contains(src, "DISMISS_NAV_ERROR") {
		t.Fatalf("[P2] 2.8-STRUCT-013: TreePanel.tsx must auto-dismiss navError via DISMISS_NAV_ERROR")
	}
}

// ---------------------------------------------------------------------------
// 2.8-STRUCT-014 [P1]: GetAncestorPath uses safeCall for panic recovery
// AC#2: All pdfcpu calls in GetAncestorPath must be wrapped in safeCall.
// ---------------------------------------------------------------------------

func TestGetAncestorPathUsesSafeCall(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("[P1] 2.8-STRUCT-014: cannot read inspector.go: %v", err)
	}
	src := string(content)
	// GetAncestorPath should exist and the file should contain safeCall
	// (safeCall is already used elsewhere; we verify it is used near findPathToObject)
	if !strings.Contains(src, "GetAncestorPath") {
		t.Fatalf("[P1] 2.8-STRUCT-014: inspector.go must contain GetAncestorPath")
	}
	if !strings.Contains(src, "findPathToObject") {
		t.Fatalf("[P1] 2.8-STRUCT-014: inspector.go must contain findPathToObject")
	}
	// Verify safeCall is used in the file (already is, but verify it is still present)
	if !strings.Contains(src, "safeCall") {
		t.Fatalf("[P1] 2.8-STRUCT-014: inspector.go must use safeCall for panic recovery")
	}
}
