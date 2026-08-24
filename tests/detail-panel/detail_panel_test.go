// Package detail_panel_test provides acceptance tests for Detail Panel --
// Context-Sensitive Content Display.
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
// DetailPanel.tsx exists -#7: The DetailPanel
// component file must exist.
// ---------------------------------------------------------------------------

func TestDetailPanelFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tsx")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("frontend/src/components/DetailPanel.tsx does not exist")
	}
}

// ---------------------------------------------------------------------------
// DetailShared.tsx exists -#5: Shared rendering components extracted
// to DetailShared.tsx.
// ---------------------------------------------------------------------------

func TestDetailSharedFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailShared.tsx")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("frontend/src/components/DetailShared.tsx does not exist")
	}
}

// ---------------------------------------------------------------------------
// DetailPanel.tsx exports DetailPanel -#7: The component
// must be a named export.
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
// DetailPanel.tsx wraps its export in React.memo, an architecture requirement.
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
// DetailPanel.tsx has data-testid attributes -#6: Required
// data-testid attributes for testing.
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
// DetailPanel.tsx has aria-live="polite": Screen reader
// support via aria-live.
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
// MainLayout.tsx imports DetailPanel: DetailPanel wired
// into the right panel.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// ObjectInfoPanel.tsx dispatches NAVIGATE_TO_REF through useAppDispatch when a
// reference value is clicked. Re-pinned 2026-05-22 -- the
// original DetailShared import assertion was stale (the component was
// refactored to consume context via useAppState/useAppDispatch directly and to
// fetch source via GetObjectSource). Behavioral coverage for the
// click-to-navigate contract is held by
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
// useDocumentState.tsx has selectedNodeLabel in TabState: Tab state must
// track selected node label for header context.
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
// useDocumentState.tsx has selectedNodeRawKey in TabState: Tab state must
// track selected node rawKey for header context.
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
// DetailShared.tsx exports shared types and components -#5: Shared
// rendering logic available for both panels.
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
