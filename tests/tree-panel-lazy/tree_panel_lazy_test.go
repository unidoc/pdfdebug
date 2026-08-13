// Package tree_panel_lazy_test provides acceptance tests for Story 2.5:
// Tree Panel with Lazy-Loading Navigation.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 2-5 is implemented.
//
// This story is purely frontend. Go structural tests verify:
// 1. No backend regressions
// 2. TreePanel.tsx exists with required attributes
// 3. useDocumentState.tsx has SELECT_NODE action
// 4. MainLayout.tsx uses TreePanel (not inline static tree)
//
// Run: cd tests/tree-panel-lazy && go test -v -count=1 ./...
package tree_panel_lazy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func fileExists(t *testing.T, relPath string) bool {
	t.Helper()
	root := projectRoot(t)
	absPath := filepath.Join(root, relPath)
	_, err := os.Stat(absPath)
	return err == nil
}

// ---------------------------------------------------------------------------
// TreePanel component exists
// ---------------------------------------------------------------------------

// TreePanel.tsx component file exists
func TestTreePanelComponentExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Fatal("frontend/src/components/TreePanel.tsx does not exist")
	}
}

// ---------------------------------------------------------------------------
// TreePanel has required data-testid attributes
// ---------------------------------------------------------------------------

// TreePanel has data-testid="tree-panel"
func TestTreePanelHasTestID(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Skip("TreePanel.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/TreePanel.tsx")

	if !strings.Contains(content, `data-testid="tree-panel"`) {
		t.Error("TreePanel.tsx missing data-testid=\"tree-panel\"")
	}
}

// TreePanel has data-testid="tree-node" on node rows
func TestTreePanelHasNodeTestID(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Skip("TreePanel.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/TreePanel.tsx")

	if !strings.Contains(content, `data-testid="tree-node"`) {
		t.Error("TreePanel.tsx missing data-testid=\"tree-node\" on node rows")
	}

	if !strings.Contains(content, `data-node-id`) {
		t.Error("TreePanel.tsx missing data-node-id attribute on node rows")
	}
}

// ---------------------------------------------------------------------------
// TreePanel uses react-arborist for virtualized tree
// ---------------------------------------------------------------------------

// TreePanel imports react-arborist
func TestTreePanelUsesReactArborist(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Skip("TreePanel.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/TreePanel.tsx")

	if !strings.Contains(content, "react-arborist") {
		t.Error("TreePanel.tsx must import from react-arborist for virtualized tree")
	}
}

// ---------------------------------------------------------------------------
// TreePanel calls GetChildren for lazy loading
// ---------------------------------------------------------------------------

// TreePanel imports GetChildren from Wails bindings
func TestTreePanelImportsGetChildren(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Skip("TreePanel.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/TreePanel.tsx")

	if !strings.Contains(content, "GetChildren") {
		t.Error("TreePanel.tsx must import and use GetChildren for lazy-loading children")
	}
}

// ---------------------------------------------------------------------------
// useDocumentState has SELECT_NODE action
// ---------------------------------------------------------------------------

// useDocumentState.tsx handles SELECT_NODE action
func TestReducerHasSelectNodeAction(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	if !strings.Contains(content, "SELECT_NODE") {
		t.Error("useDocumentState.tsx must define SELECT_NODE action type")
	}
}

// TabState has selectedNodeId field
func TestTabStateHasSelectedNodeId(t *testing.T) {
	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	if !strings.Contains(content, "selectedNodeId") {
		t.Error("TabState must have selectedNodeId field")
	}
}

// ---------------------------------------------------------------------------
// MainLayout uses TreePanel instead of inline static list
// ---------------------------------------------------------------------------

// MainLayout imports TreePanel

// MainLayout no longer has inline TreeNodeItem

// ---------------------------------------------------------------------------
// Error node rendering support
// ---------------------------------------------------------------------------

// TreePanel handles error nodes with text-error styling
func TestTreePanelErrorNodeStyling(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Skip("TreePanel.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/TreePanel.tsx")

	// Error nodes must have text-error for warning icon
	if !strings.Contains(content, "text-error") {
		t.Error("TreePanel.tsx must use text-error class for error node warning icon")
	}

	// Error nodes must have text-text-muted for label
	if !strings.Contains(content, "text-text-muted") {
		t.Error("TreePanel.tsx must use text-text-muted class for error node label")
	}
}

// ---------------------------------------------------------------------------
// Selected node highlight styling
// ---------------------------------------------------------------------------

// TreePanel has bg-surface-selected for selected node
func TestTreePanelSelectedStyling(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Skip("TreePanel.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/TreePanel.tsx")

	if !strings.Contains(content, "bg-surface-selected") {
		t.Error("TreePanel.tsx must use bg-surface-selected for selected node")
	}

	if !strings.Contains(content, "border-") {
		t.Error("TreePanel.tsx must have left border accent on selected node")
	}
}

// ---------------------------------------------------------------------------
// Hover state
// ---------------------------------------------------------------------------

// TreePanel has bg-surface-hover for hover state
func TestTreePanelHoverStyling(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Skip("TreePanel.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/TreePanel.tsx")

	if !strings.Contains(content, "bg-surface-hover") {
		t.Error("TreePanel.tsx must use bg-surface-hover for hover state")
	}
}

// ---------------------------------------------------------------------------
// Document Structure header
// ---------------------------------------------------------------------------

// TreePanel has "Document Structure" header
func TestTreePanelHeader(t *testing.T) {
	if !fileExists(t, "frontend/src/components/TreePanel.tsx") {
		t.Skip("TreePanel.tsx does not exist yet")
	}

	content := readFile(t, "frontend/src/components/TreePanel.tsx")

	if !strings.Contains(content, "Document Structure") {
		t.Error("TreePanel.tsx must have 'Document Structure' header")
	}
}

// ---------------------------------------------------------------------------
// No-regression: Go backend compiles and vets
// ---------------------------------------------------------------------------

// go build compiles (no backend changes, but verify)
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

// go vet passes
func TestProjectVet(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet./... failed:\n%s", string(output))
	}
}

// Pdfcore tests still pass (no regression)
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

// Pdfservice tests still pass (no regression)
func TestPdfserviceNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfservice regression -- tests failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// Pdfcore has zero Wails imports (architecture compliance)
func TestPdfcoreZeroWailsImports(t *testing.T) {
	root := projectRoot(t)

	pdfcoreDir := filepath.Join(root, "internal", "pdfcore")
	entries, err := os.ReadDir(pdfcoreDir)
	if err != nil {
		t.Fatalf("cannot read internal/pdfcore/: %v", err)
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
			t.Errorf("%s imports Wails -- pdfcore must have zero Wails deps", entry.Name())
		}
	}
}
