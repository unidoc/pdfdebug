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
// AC#1: Centered empty state with "UniDOC PDF Debugger" title and
//       "Inspect PDF internal structure" subtitle
// AC#2: Dashed-border drop zone with "Drop a PDF file here" text
// AC#3: "or" divider + "Open File..." primary button (blue bg, white text)
// AC#5: Design system fonts and colors used
// ---------------------------------------------------------------------------

func TestEmptyStateComponentRendersRequiredElements(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P0] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// AC#1: App title text
	if !strings.Contains(content, "UniDOC PDF Debugger") {
		t.Error("[P0] EmptyState.tsx missing app title text: 'UniDOC PDF Debugger'")
	}

	// AC#1: Subtitle text
	if !strings.Contains(content, "Inspect PDF internal structure") {
		t.Error("[P0] EmptyState.tsx missing subtitle text: 'Inspect PDF internal structure'")
	}

	// AC#2: Drop zone text
	if !strings.Contains(content, "Drop a PDF file here") {
		t.Error("[P0] EmptyState.tsx missing drop zone text: 'Drop a PDF file here'")
	}

	// AC#3: Open File button text
	if !strings.Contains(content, "Open File...") {
		t.Error("[P0] EmptyState.tsx missing button text: 'Open File...'")
	}

	// AC#3: "or" divider between drop zone and button
	// Check for an element containing just "or" as text content
	orDividerRe := regexp.MustCompile(`>\s*or\s*<`)
	if !orDividerRe.MatchString(content) {
		t.Error("[P0] EmptyState.tsx missing 'or' divider text between drop zone and button")
	}

	// AC#2: Dashed border on drop zone (Tailwind class)
	if !strings.Contains(content, "border-dashed") {
		t.Error("[P0] EmptyState.tsx missing dashed border class on drop zone (expected 'border-dashed')")
	}

	// AC#3: Primary button uses blue background (bg-border-focus reuses Blue 500)
	if !strings.Contains(content, "bg-border-focus") {
		t.Error("[P0] EmptyState.tsx missing primary button blue background class (expected 'bg-border-focus')")
	}

	// AC#3: Primary button uses white text
	if !strings.Contains(content, "text-white") {
		t.Error("[P0] EmptyState.tsx missing primary button white text class (expected 'text-white')")
	}
}

// ---------------------------------------------------------------------------
// 1.3-UNIT-001b (P0): EmptyState data-testid attributes present
// AC#1-5: All required data-testid attributes for test automation
// ---------------------------------------------------------------------------

func TestEmptyStateDataTestIds(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P0] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	requiredTestIds := []string{
		"empty-state",
		"empty-state-title",
		"empty-state-subtitle",
		"drop-zone",
		"open-file-button",
		"shortcut-hint",
	}

	for _, testId := range requiredTestIds {
		// Match data-testid="..." or data-testid={'...'}
		pattern := `data-testid=["']` + regexp.QuoteMeta(testId) + `["']`
		testIdRe := regexp.MustCompile(pattern)
		if !testIdRe.MatchString(content) {
			t.Errorf("[P0] EmptyState.tsx missing data-testid attribute: %q", testId)
		}
	}
}

// ---------------------------------------------------------------------------
// 1.3-UNIT-002 (P1): Drop zone accessibility attributes
// AC#2: Drop zone has role="region" and aria-label="File drop zone"
// AC#6: Drag hint has aria-live="polite"
// ---------------------------------------------------------------------------

func TestEmptyStateAccessibilityAttributes(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P1] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// AC#2: Drop zone role and aria-label
	if !strings.Contains(content, `role="region"`) {
		t.Error("[P1] EmptyState.tsx missing role='region' on drop zone")
	}
	if !strings.Contains(content, `aria-label="File drop zone"`) {
		t.Error("[P1] EmptyState.tsx missing aria-label='File drop zone' on drop zone")
	}

	// AC#6: aria-live on drag hint text
	if !strings.Contains(content, `aria-live="polite"`) {
		t.Error("[P1] EmptyState.tsx missing aria-live='polite' on drag hint element")
	}

	// AC#3: Open File button must be a <button> element
	buttonRe := regexp.MustCompile(`<button[^>]*data-testid=["']open-file-button["']`)
	if !buttonRe.MatchString(content) {
		// Also check reverse order (data-testid before other attrs)
		buttonRe2 := regexp.MustCompile(`<button[^>]*open-file-button`)
		if !buttonRe2.MatchString(content) {
			t.Error("[P1] EmptyState.tsx 'Open File...' must be a <button> element (not a styled <div>)")
		}
	}

	// AC#3: Focus indicator on button
	if !strings.Contains(content, "focus-visible:ring-2") {
		t.Error("[P1] EmptyState.tsx missing focus-visible:ring-2 on Open File button")
	}
	if !strings.Contains(content, "focus-visible:ring-border-focus") {
		t.Error("[P1] EmptyState.tsx missing focus-visible:ring-border-focus on Open File button")
	}
}

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

func TestShortcutHintRenderedInEmptyState(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P1] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must import from platform utility
	platformImportRe := regexp.MustCompile(`import\s+.*from\s+['"].*lib/platform['"]`)
	if !platformImportRe.MatchString(content) {
		t.Error("[P1] EmptyState.tsx must import from '../lib/platform' (or similar path)")
	}

	// Must use getShortcutHint function
	if !strings.Contains(content, "getShortcutHint") {
		t.Error("[P1] EmptyState.tsx must call getShortcutHint to render platform-aware shortcut")
	}

	// Must render with text-xs text-text-muted (per spec)
	if !strings.Contains(content, "text-xs") || !strings.Contains(content, "text-text-muted") {
		t.Error("[P1] EmptyState.tsx shortcut hint must use text-xs text-text-muted styling")
	}
}

// ---------------------------------------------------------------------------
// 1.3-UNIT-004 (P2): hasDocument=true hides empty state
// AC#9: Component not rendered when hasDocument is true
// ---------------------------------------------------------------------------

func TestHasDocumentPropConditionalRendering(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P2] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must define EmptyStateProps interface with hasDocument
	if !strings.Contains(content, "EmptyStateProps") {
		t.Error("[P2] EmptyState.tsx missing EmptyStateProps interface definition")
	}

	if !strings.Contains(content, "hasDocument") {
		t.Error("[P2] EmptyState.tsx missing hasDocument prop in EmptyStateProps")
	}

	// Must return null when hasDocument is true
	nullReturnRe := regexp.MustCompile(`hasDocument.*return\s+null|if\s*\(\s*hasDocument\s*\)\s*\{?\s*return\s+null`)
	if !nullReturnRe.MatchString(content) {
		// Also check for a more concise pattern
		earlyReturnRe := regexp.MustCompile(`hasDocument.*\breturn\s+null\b`)
		if !earlyReturnRe.MatchString(content) {
			t.Error("[P2] EmptyState.tsx must return null when hasDocument is true")
		}
	}
}

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

func TestDragEventHandlersOnOuterContainer(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P1] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must have all four drag event handlers
	requiredHandlers := []string{
		"onDragEnter",
		"onDragOver",
		"onDragLeave",
		"onDrop",
	}

	for _, handler := range requiredHandlers {
		if !strings.Contains(content, handler) {
			t.Errorf("[P1] EmptyState.tsx missing drag event handler: %s", handler)
		}
	}

	// Must use dragCounter ref pattern (not state) to prevent flickering
	dragCounterRe := regexp.MustCompile(`useRef.*0|dragCounter`)
	if !dragCounterRe.MatchString(content) {
		t.Error("[P1] EmptyState.tsx must use useRef for drag counter (not useState) to prevent re-renders")
	}

	// Must track isDragOver state
	if !strings.Contains(content, "isDragOver") {
		t.Error("[P1] EmptyState.tsx must track isDragOver state")
	}

	// Must track isInvalidFile state
	if !strings.Contains(content, "isInvalidFile") {
		t.Error("[P1] EmptyState.tsx must track isInvalidFile state")
	}

	// Must call preventDefault on events (required for drop to work)
	if !strings.Contains(content, "preventDefault") {
		t.Error("[P1] EmptyState.tsx must call e.preventDefault() on drag events")
	}

	// Must call stopPropagation on events
	if !strings.Contains(content, "stopPropagation") {
		t.Error("[P1] EmptyState.tsx must call e.stopPropagation() on drag events")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-005 (P1): Drop zone visual feedback classes
// AC#6: Blue border and highlight background on valid drag
// AC#7: "PDF files only" error text for invalid files
// ---------------------------------------------------------------------------

func TestDropZoneVisualFeedbackClasses(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P1] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// AC#6: Must use border-border-focus for active drag state (blue border)
	if !strings.Contains(content, "border-border-focus") {
		t.Error("[P1] EmptyState.tsx missing border-border-focus class for drag-over blue highlight")
	}

	// AC#6: Must use bg-surface-selected for active drag background
	if !strings.Contains(content, "bg-surface-selected") {
		t.Error("[P1] EmptyState.tsx missing bg-surface-selected class for drag-over background highlight")
	}

	// AC#7: "PDF files only" error text for invalid file types
	if !strings.Contains(content, "PDF files only") {
		t.Error("[P1] EmptyState.tsx missing 'PDF files only' error text for invalid file drag/drop")
	}

	// AC#7: Must use text-error for invalid file hint color
	if !strings.Contains(content, "text-error") {
		t.Error("[P1] EmptyState.tsx missing text-error class for invalid file hint")
	}

	// AC#6: transition-colors for smooth state changes
	if !strings.Contains(content, "transition-colors") {
		t.Error("[P1] EmptyState.tsx missing transition-colors class on drop zone")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-006 (P1): Non-PDF drop rejection with 2-second timeout
// AC#7: After non-PDF drop, show error for 2 seconds then reset
// ---------------------------------------------------------------------------

func TestNonPdfDropRejectionTimeout(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P1] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must use setTimeout for 2-second reset
	if !strings.Contains(content, "setTimeout") {
		t.Error("[P1] EmptyState.tsx must use setTimeout to reset invalid file hint after 2 seconds")
	}

	// Must check for 2000ms timeout value
	// Use (?s) so .* spans newlines inside the setTimeout callback
	timeoutRe := regexp.MustCompile(`(?s)setTimeout\s*\(.*?,\s*2000\s*\)`)
	if !timeoutRe.MatchString(content) {
		t.Error("[P1] EmptyState.tsx setTimeout must use 2000ms (2 seconds) for invalid file hint reset")
	}

	// Must validate dropped file by extension (.pdf check)
	pdfCheckRe := regexp.MustCompile(`\.pdf['"]?\s*\)|endsWith.*\.pdf`)
	if !pdfCheckRe.MatchString(content) {
		t.Error("[P1] EmptyState.tsx must validate dropped files by .pdf extension")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-007 (P1): EmptyState centering layout
// AC#5: Layout vertically and horizontally centered using design system
// ---------------------------------------------------------------------------

func TestEmptyStateCenteringLayout(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P1] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must use flex centering layout (Tailwind classes)
	if !strings.Contains(content, "flex") {
		t.Error("[P1] EmptyState.tsx missing 'flex' class for centering layout")
	}
	if !strings.Contains(content, "items-center") {
		t.Error("[P1] EmptyState.tsx missing 'items-center' class for vertical centering")
	}
	if !strings.Contains(content, "justify-center") {
		t.Error("[P1] EmptyState.tsx missing 'justify-center' class for horizontal centering")
	}
	if !strings.Contains(content, "h-full") {
		t.Error("[P1] EmptyState.tsx missing 'h-full' class for full-height layout")
	}

	// AC#1: Title must use design system classes
	if !strings.Contains(content, "text-xl") {
		t.Error("[P1] EmptyState.tsx title missing 'text-xl' class")
	}
	if !strings.Contains(content, "font-semibold") {
		t.Error("[P1] EmptyState.tsx title missing 'font-semibold' class")
	}
	if !strings.Contains(content, "text-text") {
		t.Error("[P1] EmptyState.tsx title missing 'text-text' class for design system color")
	}

	// AC#1: Subtitle must use design system classes
	if !strings.Contains(content, "text-text-secondary") {
		t.Error("[P1] EmptyState.tsx subtitle missing 'text-text-secondary' class")
	}
}

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

func TestEmptyStateVerticalStacking(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P1] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	if !strings.Contains(content, "flex-col") {
		t.Error("[P1] EmptyState.tsx missing 'flex-col' class for vertical element stacking")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-011 (P1): drop-zone-hint data-testid
// AC#6: data-testid on hint text inside drop zone
// ---------------------------------------------------------------------------

func TestDropZoneHintTestId(t *testing.T) {
	if !fileExists(t, "frontend/src/components/EmptyState.tsx") {
		t.Fatal("[P1] frontend/src/components/EmptyState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	testIdRe := regexp.MustCompile(`data-testid=["']drop-zone-hint["']`)
	if !testIdRe.MatchString(content) {
		t.Error("[P1] EmptyState.tsx missing data-testid='drop-zone-hint' on hint text element inside drop zone")
	}
}

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

func TestTimeoutCleanupOnUnmount(t *testing.T) {
	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must have useEffect with cleanup that calls clearTimeout
	if !strings.Contains(content, "useEffect") {
		t.Error("[P2] EmptyState.tsx must use useEffect for cleanup")
	}

	// Must have clearTimeout in the component (used for both unmount and timer management)
	if !strings.Contains(content, "clearTimeout") {
		t.Error("[P2] EmptyState.tsx must call clearTimeout to clean up pending timers")
	}

	// Must store timer ref for cleanup (useRef pattern)
	timerRefRe := regexp.MustCompile(`useRef.*(?:null|setTimeout|timer)`)
	if !timerRefRe.MatchString(content) {
		// Also check for invalidTimerRef pattern
		if !strings.Contains(content, "invalidTimerRef") && !strings.Contains(content, "timerRef") {
			t.Error("[P2] EmptyState.tsx must use a ref to store the timeout ID for cleanup")
		}
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-013 (P2): Drag counter negative guard
// Edge case: dragCounter must not go below 0 from unpaired dragleave events
// ---------------------------------------------------------------------------

func TestDragCounterNegativeGuard(t *testing.T) {
	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must guard against negative counter
	negativeGuardRe := regexp.MustCompile(`dragCounter\.current\s*<\s*0`)
	if !negativeGuardRe.MatchString(content) {
		t.Error("[P2] EmptyState.tsx must guard against negative dragCounter (unpaired dragleave events)")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-014 (P2): Case-insensitive PDF extension validation on drop
// Edge case: dropped file with .PDF or .Pdf extension should be accepted
// ---------------------------------------------------------------------------

func TestCaseInsensitivePdfValidation(t *testing.T) {
	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must call toLowerCase() before endsWith('.pdf') check
	lowerCaseRe := regexp.MustCompile(`toLowerCase\(\)\.endsWith\(['"]\.pdf['"]\)`)
	if !lowerCaseRe.MatchString(content) {
		t.Error("[P2] EmptyState.tsx must use case-insensitive PDF extension check (toLowerCase().endsWith('.pdf'))")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-015 (P1): dragover handler must NOT update state or counter
// Edge case: dragover fires continuously; incrementing counter on it would
// break the dragleave counter-to-zero pattern
// ---------------------------------------------------------------------------

func TestDragOverHandlerMinimal(t *testing.T) {
	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Extract the handleDragOver function body
	// It should contain only preventDefault and stopPropagation, nothing else
	dragOverRe := regexp.MustCompile(`(?s)handleDragOver\s*=\s*useCallback\s*\(\s*\(e[^)]*\)\s*=>\s*\{([^}]*)\}`)
	matches := dragOverRe.FindStringSubmatch(content)
	if matches == nil {
		t.Error("[P1] EmptyState.tsx must define handleDragOver as a useCallback")
		return
	}

	body := matches[1]
	// Should NOT contain dragCounter increment
	if strings.Contains(body, "dragCounter") {
		t.Error("[P1] handleDragOver must NOT reference dragCounter (fires continuously, would break counter pattern)")
	}
	// Should NOT contain setIsDragOver
	if strings.Contains(body, "setIsDragOver") {
		t.Error("[P1] handleDragOver must NOT call setIsDragOver (fires continuously, would cause re-renders)")
	}
	// Should NOT contain setIsInvalidFile
	if strings.Contains(body, "setIsInvalidFile") {
		t.Error("[P1] handleDragOver must NOT call setIsInvalidFile (fires continuously)")
	}
}

// ---------------------------------------------------------------------------
// 1.3-INTG-016 (P1): New drag resets stale invalid state from prior drop
// Edge case: dragging a new file within 2s of a rejected drop should not
// show error state from the previous rejection
// ---------------------------------------------------------------------------

func TestNewDragResetsStaleInvalidState(t *testing.T) {
	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// The handleDragEnter must reset isInvalidFile to false
	dragEnterRe := regexp.MustCompile(`(?s)handleDragEnter\s*=\s*useCallback\s*\([^{]*\{(.*?)\}\s*,\s*\[`)
	matches := dragEnterRe.FindStringSubmatch(content)
	if matches == nil {
		t.Error("[P1] EmptyState.tsx must define handleDragEnter as a useCallback")
		return
	}

	body := matches[1]
	// Must reset isInvalidFile in dragEnter
	if !strings.Contains(body, "setIsInvalidFile(false)") {
		t.Error("[P1] handleDragEnter must reset isInvalidFile to false to clear stale error state from prior drop rejection")
	}
	// Must clear pending timer in dragEnter
	if !strings.Contains(body, "clearTimeout") {
		t.Error("[P1] handleDragEnter must clearTimeout to cancel pending 2s reset timer from prior drop rejection")
	}
}
