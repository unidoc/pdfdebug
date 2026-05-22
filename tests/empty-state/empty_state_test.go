// 4-5: deleted TestOpenFileButtonOnClickBehavior (source-grep on EmptyState.tsx
//      console.log fallback; real behaviour covered by Playwright E2E).

// Package empty_state_test provides acceptance tests for Story 1.3:
// Empty State with Drag-and-Drop Zone.
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
// 1.3-UNIT-001 (P0): EmptyState component renders title, subtitle, drop zone,
//                     and Open File button
// AC#1: Centered empty state with "UniDoc PDF Debugger" title and
//       "Inspect PDF internal structure" subtitle
// AC#2: Dashed-border drop zone with "Drop a PDF file here" text
// AC#3: "or" divider + "Open File..." primary button (blue bg, white text)
// AC#5: Design system fonts and colors used
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-UNIT-001b (P0): EmptyState data-testid attributes present
// AC#1-5: All required data-testid attributes for test automation
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-UNIT-002 (P1): Drop zone accessibility attributes
// AC#2: Drop zone has role="region" and aria-label="File drop zone"
// AC#6: Drag hint has aria-live="polite"
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-UNIT-003 (P1): Platform detection utility
// AC#4: Platform-aware keyboard shortcut hint
// ---------------------------------------------------------------------------

func TestPlatformDetectionUtilityExists(t *testing.T) {
	if !fileExists(t, "frontend/src/lib/platform.ts") {
		t.Fatal("[P1] frontend/src/lib/platform.ts does not exist")
	}

	content := readFile(t, "frontend/src/lib/platform.ts")

	// Must export getPlatformModifier function
	if !strings.Contains(content, "getPlatformModifier") {
		t.Error("[P1] platform.ts missing getPlatformModifier function")
	}

	// Must export getShortcutHint function
	if !strings.Contains(content, "getShortcutHint") {
		t.Error("[P1] platform.ts missing getShortcutHint function")
	}

	// Must detect macOS via navigator.platform
	if !strings.Contains(content, "navigator.platform") {
		t.Error("[P1] platform.ts must use navigator.platform for primary detection")
	}

	// Must return "Cmd" for macOS
	if !strings.Contains(content, "Cmd") {
		t.Error("[P1] platform.ts must return 'Cmd' for macOS platform")
	}

	// Must return "Ctrl" for non-macOS
	if !strings.Contains(content, "Ctrl") {
		t.Error("[P1] platform.ts must return 'Ctrl' for non-macOS platforms")
	}

	// Must check for Mac/iPhone/iPad patterns
	macPatternRe := regexp.MustCompile(`Mac|iPhone|iPad`)
	if !macPatternRe.MatchString(content) {
		t.Error("[P1] platform.ts must check for Mac/iPhone/iPad in navigator.platform")
	}
}

// ---------------------------------------------------------------------------
// 1.3-UNIT-003b (P1): Shortcut hint rendered in EmptyState
// AC#4: Keyboard shortcut hint shows "Cmd+O" or "Ctrl+O"
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-UNIT-004 (P2): hasDocument=true hides empty state
// AC#9: Component not rendered when hasDocument is true
// ---------------------------------------------------------------------------


// 1.3-INTG-001 (Story 4-5): TestOpenFileButtonOnClickBehavior was a source-grep
// asserting EmptyState.tsx still calls `console.log` as a fallback when no
// onOpenFile prop is supplied. The console.log was placeholder scaffolding;
// real file-open behaviour is covered by tests/e2e/open-pdf-dialog-dnd.spec.ts.
// Delete-only, no replacement.

// ---------------------------------------------------------------------------
// 1.3-INTG-002 (P1): App.jsx integrates EmptyState component
// AC#1, AC#5, AC#9: App renders EmptyState, Wails template removed
// ---------------------------------------------------------------------------

func TestAppJsxIntegratesEmptyState(t *testing.T) {
	if !fileExists(t, "frontend/src/App.jsx") {
		t.Fatal("[P1] frontend/src/App.jsx does not exist")
	}

	content := readFile(t, "frontend/src/App.jsx")

	// Must import EmptyState component
	emptyStateImportRe := regexp.MustCompile(`import\s+.*EmptyState.*from`)
	if !emptyStateImportRe.MatchString(content) {
		t.Error("[P1] App.jsx must import EmptyState component")
	}

	// Must render <EmptyState /> (or <EmptyState ...)
	emptyStateRenderRe := regexp.MustCompile(`<EmptyState\s*/?>|<EmptyState\s+`)
	if !emptyStateRenderRe.MatchString(content) {
		t.Error("[P1] App.jsx must render <EmptyState /> component")
	}

	// Must NOT contain GreetService import (Wails template removed)
	if strings.Contains(content, "GreetService") {
		t.Error("[P1] App.jsx still contains GreetService import -- Wails template code must be removed")
	}

	// Must NOT import WML or Events from Wails runtime
	if strings.Contains(content, "WML") {
		t.Error("[P1] App.jsx still contains WML import -- Wails template code must be removed")
	}

	// Must NOT contain wails.png or react.svg references
	if strings.Contains(content, "wails.png") {
		t.Error("[P1] App.jsx still references wails.png -- Wails template code must be removed")
	}
	if strings.Contains(content, "react.svg") {
		t.Error("[P1] App.jsx still references react.svg -- Wails template code must be removed")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-003 (P0): App.jsx keeps .jsx extension (not renamed to .tsx)
// Per project convention: Wails-generated files keep .jsx extension
// ---------------------------------------------------------------------------

func TestAppJsxExtensionPreserved(t *testing.T) {
	if !fileExists(t, "frontend/src/App.jsx") {
		t.Fatal("[P0] frontend/src/App.jsx does not exist -- must keep .jsx extension per project convention")
	}

	// Must NOT have been renamed to .tsx
	if fileExists(t, "frontend/src/App.tsx") {
		t.Error("[P0] frontend/src/App.tsx exists -- App should remain .jsx per project convention (Wails-generated files keep original extension)")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-004 (P1): Drag-and-drop event handlers on outermost container
// AC#6: Drag handlers on entire empty state wrapper, not just drop zone
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-005 (P1): Drop zone visual feedback classes
// AC#6: Blue border and highlight background on valid drag
// AC#7: "PDF files only" error text for invalid files
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-006 (P1): Non-PDF drop rejection with 2-second timeout
// AC#7: After non-PDF drop, show error for 2 seconds then reset
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-007 (P1): EmptyState centering layout
// AC#5: Layout vertically and horizontally centered using design system
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-008 (P1): CSS height rule for full-height centering
// AC#5: html, body, #root must have height: 100% for h-full to work
// ---------------------------------------------------------------------------

func TestCSSHeightRuleForCentering(t *testing.T) {
	css := readFile(t, "frontend/src/style.css")

	// Must have height: 100% rule for html, body, #root
	// This is required for h-full to fill the Wails WebView correctly
	heightRuleRe := regexp.MustCompile(`(?s)(html|body|#root)[^{]*\{[^}]*height\s*:\s*100%`)
	if !heightRuleRe.MatchString(css) {
		t.Error("[P1] style.css missing 'height: 100%' rule for html, body, or #root (required for EmptyState h-full centering in Wails WebView)")
	}

	// Check specifically for #root having height: 100%
	rootHeightRe := regexp.MustCompile(`#root[^{]*\{[^}]*height\s*:\s*100%`)
	if !rootHeightRe.MatchString(css) {
		// Also check combined selector (html, body, #root { height: 100% })
		combinedRe := regexp.MustCompile(`(?s)(html\s*,\s*body\s*,\s*#root|#root)[^{]*\{[^}]*height\s*:\s*100%`)
		if !combinedRe.MatchString(css) {
			t.Error("[P1] style.css missing 'height: 100%' on #root element")
		}
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-009 (P0): No barrel files created
// Project rule: barrel files (index.ts) are forbidden
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
			t.Errorf("[P0] %s/index.ts exists -- barrel files are forbidden by project rules", dir)
		}
		indexTsxPath := filepath.Join(root, dir, "index.tsx")
		if _, err := os.Stat(indexTsxPath); err == nil {
			t.Errorf("[P0] %s/index.tsx exists -- barrel files are forbidden by project rules", dir)
		}
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-010 (P1): EmptyState uses flex-col for vertical stacking
// AC#5: Elements stacked vertically (title, subtitle, drop zone, divider, button, hint)
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-011 (P1): drop-zone-hint data-testid
// AC#6: data-testid on hint text inside drop zone
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-UNIT-005 (P2): Platform detection SSR/non-browser guard
// Edge case: navigator undefined returns "Ctrl" default
// ---------------------------------------------------------------------------

func TestPlatformDetectionSSRGuard(t *testing.T) {
	content := readFile(t, "frontend/src/lib/platform.ts")

	// Must guard against undefined navigator for SSR/test environments
	if !strings.Contains(content, "typeof navigator") {
		t.Error("[P2] platform.ts must guard against undefined navigator (SSR/non-browser environments)")
	}

	// Must return 'Ctrl' as default when navigator is unavailable
	undefinedGuardRe := regexp.MustCompile(`(?s)typeof navigator\s*===?\s*['"]undefined['"].*return\s+['"]Ctrl['"]`)
	if !undefinedGuardRe.MatchString(content) {
		t.Error("[P2] platform.ts must return 'Ctrl' when navigator is undefined")
	}
}

// ---------------------------------------------------------------------------
// 1.3-UNIT-006 (P2): Platform detection secondary userAgentData check
// Edge case: userAgentData used as fallback when navigator.platform misses
// ---------------------------------------------------------------------------

func TestPlatformDetectionUserAgentDataFallback(t *testing.T) {
	content := readFile(t, "frontend/src/lib/platform.ts")

	// Must check userAgentData as secondary detection
	if !strings.Contains(content, "userAgentData") {
		t.Error("[P2] platform.ts must include userAgentData as secondary detection method")
	}

	// Must check for "macOS" string in userAgentData (single or double quotes)
	if !strings.Contains(content, `"macOS"`) && !strings.Contains(content, `'macOS'`) {
		t.Error("[P2] platform.ts must check for 'macOS' in userAgentData.platform")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-012 (P2): Timeout cleanup on unmount
// Edge case: clearTimeout in useEffect cleanup prevents state update after unmount
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-013 (P2): Drag counter negative guard
// Edge case: dragCounter must not go below 0 from unpaired dragleave events
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-014 (P2): Case-insensitive PDF extension validation on drop
// Edge case: dropped file with .PDF or .Pdf extension should be accepted
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-015 (P1): dragover handler must NOT update state or counter
// Edge case: dragover fires continuously; incrementing counter on it would
// break the dragleave counter-to-zero pattern
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.3-INTG-016 (P1): New drag resets stale invalid state from prior drop
// Edge case: dragging a new file within 2s of a rejected drop should not
// show error state from the previous rejection
// ---------------------------------------------------------------------------

