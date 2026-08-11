// Package detail_panel_test provides acceptance tests for Story 2.7:
// Detail Panel -- Context-Sensitive Content Display.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 2-7 is implemented.
//
// Test Levels: Structural (Go) -- file existence checks for frontend artifacts.
// This story is frontend-only. No Go logic is added. These tests verify that
// the required frontend files exist after implementation.
//
// Run: go test ./tests/detail-panel/... -v -count=1
package detail_panel_test

import (
	"os"
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

// ---------------------------------------------------------------------------
// 2.7-STRUCT-001 [P1]: DetailPanel.tsx exists
// AC#1-#7: The DetailPanel component file must exist.
// ---------------------------------------------------------------------------

func TestDetailPanelFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tsx")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("frontend/src/components/DetailPanel.tsx does not exist")
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-002 [P1]: DetailShared.tsx exists
// AC#2-#5: Shared rendering components extracted to DetailShared.tsx.
// ---------------------------------------------------------------------------

func TestDetailSharedFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailShared.tsx")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("frontend/src/components/DetailShared.tsx does not exist")
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-003 [P1]: DetailPanel.tsx exports DetailPanel
// AC#1-#7: The component must be a named export.
// ---------------------------------------------------------------------------

func TestDetailPanelExport(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read DetailPanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "export") || !strings.Contains(src, "DetailPanel") {
		t.Fatalf("DetailPanel.tsx must export DetailPanel")
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-004 [P1]: DetailPanel.tsx uses React.memo
// AC: Architecture requirement -- wrapped in React.memo.
// ---------------------------------------------------------------------------

func TestDetailPanelUsesMemo(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read DetailPanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "memo") {
		t.Fatalf("DetailPanel.tsx must use React.memo")
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-005 [P1]: DetailPanel.tsx has data-testid attributes
// AC#1-#6: Required data-testid attributes for testing.
// ---------------------------------------------------------------------------

func TestDetailPanelTestIds(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read DetailPanel.tsx: %v", err)
	}
	src := string(content)

	requiredTestIds := []string{
		"detail-panel",
		"detail-panel-empty",
		"detail-panel-error",
		"detail-panel-header",
		"detail-panel-content",
	}

	for _, tid := range requiredTestIds {
		if !strings.Contains(src, tid) {
			t.Errorf("DetailPanel.tsx missing data-testid=%q", tid)
		}
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-006 [P1]: DetailPanel.tsx has aria-live="polite"
// AC#7: Screen reader support via aria-live.
// ---------------------------------------------------------------------------

func TestDetailPanelAriaLive(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read DetailPanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "aria-live") {
		t.Fatalf("DetailPanel.tsx must have aria-live attribute")
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-007 [P1]: MainLayout.tsx imports DetailPanel
// AC#1: DetailPanel wired into the right panel.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 2.7-STRUCT-008 [P1]: ObjectInfoPanel.tsx dispatches NAVIGATE_TO_REF through
// useAppDispatch when a reference value is clicked.
// Re-pinned 2026-05-22 (Epic 9 retro) -- the original DetailShared import
// assertion was stale (the component was refactored to consume context via
// useAppState/useAppDispatch directly and to fetch source via GetObjectSource).
// Behavioral coverage for the click-to-navigate contract is held by
// frontend/src/components/ObjectInfoPanel.test.tsx (430 lines, 20 cases).
// ---------------------------------------------------------------------------

func TestObjectInfoPanelDispatchesNavigateToRef(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ObjectInfoPanel.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read ObjectInfoPanel.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "useAppDispatch") {
		t.Fatalf("ObjectInfoPanel.tsx must use useAppDispatch for reference navigation")
	}
	if !strings.Contains(src, "NAVIGATE_TO_REF") {
		t.Fatalf("ObjectInfoPanel.tsx must dispatch NAVIGATE_TO_REF on reference click")
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-009 [P1]: useDocumentState.tsx has selectedNodeLabel in TabState
// AC#2: Tab state must track selected node label for header context.
// ---------------------------------------------------------------------------

func TestTabStateHasSelectedNodeLabel(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "selectedNodeLabel") {
		t.Fatalf("useDocumentState.tsx TabState must have selectedNodeLabel field")
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-010 [P1]: useDocumentState.tsx has selectedNodeRawKey in TabState
// AC#2: Tab state must track selected node rawKey for header context.
// ---------------------------------------------------------------------------

func TestTabStateHasSelectedNodeRawKey(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "hooks", "useDocumentState.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read useDocumentState.tsx: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "selectedNodeRawKey") {
		t.Fatalf("useDocumentState.tsx TabState must have selectedNodeRawKey field")
	}
}

// ---------------------------------------------------------------------------
// 2.7-STRUCT-011 [P1]: DetailShared.tsx exports shared types and components
// AC#2-#5: Shared rendering logic available for both panels.
// ---------------------------------------------------------------------------

func TestDetailSharedExports(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailShared.tsx")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read DetailShared.tsx: %v", err)
	}
	src := string(content)

	requiredExports := []string{
		"ValueDisplay",
		"DictView",
		"ArrayView",
		"ScalarView",
		"StreamMetadata",
		"TYPE_CLASS_MAP",
		"ObjectDetailData",
	}

	for _, name := range requiredExports {
		if !strings.Contains(src, name) {
			t.Errorf("DetailShared.tsx must contain %q", name)
		}
	}
}
