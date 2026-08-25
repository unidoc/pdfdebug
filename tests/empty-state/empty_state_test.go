// Deleted TestOpenFileButtonOnClickBehavior (source-grep on EmptyState.tsx
// console.log fallback; real behaviour covered by Playwright E2E).

// Package empty_state_test provides acceptance tests for Empty State with
// Drag-and-Drop Zone.
//
// These tests verify that the EmptyState component, platform detection
// utility, App.jsx integration, and supporting CSS rules are correctly
// implemented by parsing source files.
//
// Test Levels: Integration (Go) -- source file content parsing.
// Drag-and-drop behavior requiring real browser interaction is covered
// by Playwright E2E tests in tests/e2e/empty-state.spec.ts.
//
// Run: cd tests/empty-state && go test -v -count=1 ./...
package empty_state_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// projectRoot returns the absolute path to the project root directory.
// It walks upward from the test file location to find the project root,
// identified by the presence of a go.mod whose module name is "unidoc-pdf-debugger".
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
// EmptyState renders title, subtitle, drop zone and Open File button:
//   - centered, with the "UniDoc PDF Debugger" title and the
//     "Inspect PDF internal structure" subtitle
//   - a dashed-border drop zone reading "Drop a PDF file here"
//   - an "or" divider plus an "Open File..." primary button (blue bg,
//     white text)
//   - design system fonts and colors throughout
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// EmptyState carries every data-testid attribute the automation needs
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Drop zone accessibility attributes: the drop zone has role="region" and
// aria-label="File drop zone", and the drag hint has aria-live="polite"
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Platform detection utility: Platform-aware
// keyboard shortcut hint
// ---------------------------------------------------------------------------

func TestPlatformDetectionUtilityExists(t *testing.T) {
	if !fileExists(t, "frontend/src/lib/platform.ts") {
		t.Fatal("frontend/src/lib/platform.ts does not exist")
	}

	content := readFile(t, "frontend/src/lib/platform.ts")

	// Must export getPlatformModifier function
	if !strings.Contains(content, "getPlatformModifier") {
		t.Error("platform.ts missing getPlatformModifier function")
	}

	// Must export getShortcutHint function
	if !strings.Contains(content, "getShortcutHint") {
		t.Error("platform.ts missing getShortcutHint function")
	}

	// Must detect macOS via navigator.platform
	if !strings.Contains(content, "navigator.platform") {
		t.Error("platform.ts must use navigator.platform for primary detection")
	}

	// Must return "Cmd" for macOS
	if !strings.Contains(content, "Cmd") {
		t.Error("platform.ts must return 'Cmd' for macOS platform")
	}

	// Must return "Ctrl" for non-macOS
	if !strings.Contains(content, "Ctrl") {
		t.Error("platform.ts must return 'Ctrl' for non-macOS platforms")
	}

	// Must check for Mac/iPhone/iPad patterns
	macPatternRe := regexp.MustCompile(`Mac|iPhone|iPad`)
	if !macPatternRe.MatchString(content) {
		t.Error("platform.ts must check for Mac/iPhone/iPad in navigator.platform")
	}
}

// ---------------------------------------------------------------------------
// Shortcut hint rendered in EmptyState: it shows "Cmd+O" or "Ctrl+O".
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// hasDocument=true hides the empty state: the component is not rendered.
// ---------------------------------------------------------------------------


// TestOpenFileButtonOnClickBehavior was a source-grep asserting
// EmptyState.tsx still calls `console.log` as a fallback when no
// onOpenFile prop is supplied. The console.log was placeholder scaffolding;
// real file-open behaviour is covered by tests/e2e/open-pdf-dialog-dnd.spec.ts.
// Delete-only, no replacement.

// ---------------------------------------------------------------------------
// App.jsx integrates EmptyState component: App renders EmptyState,
// Wails template removed
// ---------------------------------------------------------------------------

func TestAppJsxIntegratesEmptyState(t *testing.T) {
	if !fileExists(t, "frontend/src/App.jsx") {
		t.Fatal("frontend/src/App.jsx does not exist")
	}

	content := readFile(t, "frontend/src/App.jsx")

	// Must import EmptyState component
	emptyStateImportRe := regexp.MustCompile(`import\s+.*EmptyState.*from`)
	if !emptyStateImportRe.MatchString(content) {
		t.Error("App.jsx must import EmptyState component")
	}

	// Must render <EmptyState /> (or <EmptyState ...)
	emptyStateRenderRe := regexp.MustCompile(`<EmptyState\s*/?>|<EmptyState\s+`)
	if !emptyStateRenderRe.MatchString(content) {
		t.Error("App.jsx must render <EmptyState /> component")
	}

	// Must NOT contain GreetService import (Wails template removed)
	if strings.Contains(content, "GreetService") {
		t.Error("App.jsx still contains GreetService import -- Wails template code must be removed")
	}

	// Must NOT import WML or Events from Wails runtime
	if strings.Contains(content, "WML") {
		t.Error("App.jsx still contains WML import -- Wails template code must be removed")
	}

	// Must NOT contain wails.png or react.svg references
	if strings.Contains(content, "wails.png") {
		t.Error("App.jsx still references wails.png -- Wails template code must be removed")
	}
	if strings.Contains(content, "react.svg") {
		t.Error("App.jsx still references react.svg -- Wails template code must be removed")
	}
}

// ---------------------------------------------------------------------------
// App.jsx keeps its .jsx extension and is not renamed to .tsx: Wails-generated
// files keep .jsx by project convention.
// ---------------------------------------------------------------------------

func TestAppJsxExtensionPreserved(t *testing.T) {
	if !fileExists(t, "frontend/src/App.jsx") {
		t.Fatal("frontend/src/App.jsx does not exist -- must keep .jsx extension per project convention")
	}

	// Must NOT have been renamed to .tsx
	if fileExists(t, "frontend/src/App.tsx") {
		t.Error("frontend/src/App.tsx exists -- App should remain .jsx per project convention (Wails-generated files keep original extension)")
	}
}

// ---------------------------------------------------------------------------
// Drag-and-drop event handlers sit on the outermost container: the whole
// empty-state wrapper, not just the drop zone.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Drop zone visual feedback classes: a blue border and highlight background on
// a valid drag, and "PDF files only" error text for invalid files.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Non-PDF drop rejection with a 2-second timeout: after a non-PDF drop the
// error shows for 2 seconds, then resets.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// EmptyState centering layout: vertically and horizontally centered using the
// design system.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// CSS height rule for full-height centering: html, body and #root must have
// height: 100% for h-full to work.
// ---------------------------------------------------------------------------

func TestCSSHeightRuleForCentering(t *testing.T) {
	css := readFile(t, "frontend/src/style.css")

	// Must have height: 100% rule for html, body, #root
	// This is required for h-full to fill the Wails WebView correctly
	heightRuleRe := regexp.MustCompile(`(?s)(html|body|#root)[^{]*\{[^}]*height\s*:\s*100%`)
	if !heightRuleRe.MatchString(css) {
		t.Error("style.css missing 'height: 100%' rule for html, body, or #root (required for EmptyState h-full centering in Wails WebView)")
	}

	// Check specifically for #root having height: 100%
	rootHeightRe := regexp.MustCompile(`#root[^{]*\{[^}]*height\s*:\s*100%`)
	if !rootHeightRe.MatchString(css) {
		// Also check combined selector (html, body, #root { height: 100% })
		combinedRe := regexp.MustCompile(`(?s)(html\s*,\s*body\s*,\s*#root|#root)[^{]*\{[^}]*height\s*:\s*100%`)
		if !combinedRe.MatchString(css) {
			t.Error("style.css missing 'height: 100%' on #root element")
		}
	}
}

// ---------------------------------------------------------------------------
// No barrel files: index.ts barrel files are forbidden by project rule.
// ---------------------------------------------------------------------------

func TestNoBarrelFilesCreated(t *testing.T) {
	root := projectRoot(t)

	// Check that no barrel index.ts files exist in new directories
	dirsToCheck := []string{
		"frontend/src/components",
		"frontend/src/lib",
	}

	for _, dir := range dirsToCheck {
		indexPath := filepath.Join(root, dir, "index.ts")
		if _, err := os.Stat(indexPath); err == nil {
			t.Errorf("%s/index.ts exists -- barrel files are forbidden by project rules", dir)
		}
		indexTsxPath := filepath.Join(root, dir, "index.tsx")
		if _, err := os.Stat(indexTsxPath); err == nil {
			t.Errorf("%s/index.tsx exists -- barrel files are forbidden by project rules", dir)
		}
	}
}

// ---------------------------------------------------------------------------
// EmptyState uses flex-col so elements stack vertically: title, subtitle, drop
// zone, divider, button, hint.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// drop-zone-hint data-testid sits on the hint text inside the drop zone.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Platform detection SSR/non-browser guard.
// Edge case: navigator undefined returns "Ctrl" default
// ---------------------------------------------------------------------------

func TestPlatformDetectionSSRGuard(t *testing.T) {
	content := readFile(t, "frontend/src/lib/platform.ts")

	// Must guard against undefined navigator for SSR/test environments
	if !strings.Contains(content, "typeof navigator") {
		t.Error("platform.ts must guard against undefined navigator (SSR/non-browser environments)")
	}

	// Must return 'Ctrl' as default when navigator is unavailable
	undefinedGuardRe := regexp.MustCompile(`(?s)typeof navigator\s*===?\s*['"]undefined['"].*return\s+['"]Ctrl['"]`)
	if !undefinedGuardRe.MatchString(content) {
		t.Error("platform.ts must return 'Ctrl' when navigator is undefined")
	}
}

// ---------------------------------------------------------------------------
// Platform detection secondary userAgentData check.
// Edge case: userAgentData used as fallback when navigator.platform misses
// ---------------------------------------------------------------------------

func TestPlatformDetectionUserAgentDataFallback(t *testing.T) {
	content := readFile(t, "frontend/src/lib/platform.ts")

	// Must check userAgentData as secondary detection
	if !strings.Contains(content, "userAgentData") {
		t.Error("platform.ts must include userAgentData as secondary detection method")
	}

	// Must check for "macOS" string in userAgentData (single or double quotes)
	if !strings.Contains(content, `"macOS"`) && !strings.Contains(content, `'macOS'`) {
		t.Error("platform.ts must check for 'macOS' in userAgentData.platform")
	}
}

// ---------------------------------------------------------------------------
// Timeout cleanup on unmount.
// Edge case: clearTimeout in useEffect cleanup prevents state update after
// unmount
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Drag counter negative guard.
// Edge case: dragCounter must not go below 0 from unpaired dragleave events
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Case-insensitive PDF extension validation on drop.
// Edge case: dropped file with .PDF or .Pdf extension should be accepted
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Dragover handler must NOT update state or counter.
// Edge case: dragover fires continuously; incrementing counter on it would
// break the dragleave counter-to-zero pattern
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// New drag resets stale invalid state from prior drop.
// Edge case: dragging a new file within 2s of a rejected drop should not
// show error state from the previous rejection
// ---------------------------------------------------------------------------

