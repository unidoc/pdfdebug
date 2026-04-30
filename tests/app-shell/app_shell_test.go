// 4-5: deleted TestPlatformConditionalQuit (source-grep; macOS Quit-in-AppMenu
//      duplicate-prevention not separately covered, accept regression risk).

// Package app_shell_test provides acceptance tests for Story 1.4:
// Native Menu Bar and Application Shell.
//
// These tests verify that:
//   - main.go is cleaned up (template boilerplate removed) and contains
//     native menu bar setup, window config with minimum size, and correct
//     background colour
//   - greetservice.go is deleted
//   - useDocumentState.tsx provides AppProvider, AppState, AppAction types,
//     and useAppState/useAppDispatch hooks
//   - MainLayout.tsx renders a two-column resizable layout with allotment,
//     semantic HTML (<aside>, <main>), and required data-testid attributes
//   - App.jsx wraps content in AppProvider and conditionally renders
//     EmptyState vs MainLayout
//
// Test Levels: Integration (Go) -- source file content parsing.
// Layout rendering requiring real browser interaction is covered
// by Playwright E2E tests in tests/e2e/app-shell.spec.ts.
//
// Run: cd tests/app-shell && go test -v -count=1 ./...
package app_shell_test

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
// AC#1: Native menu bar with File menu (Open, Close, Quit)
// Test IDs: 1.4-INTG-001 through 1.4-INTG-005
// ---------------------------------------------------------------------------

// 1.4-INTG-001 (P0): main.go template boilerplate removed
// AC#1 prerequisite: GreetService, time ticker, init() removed

func TestMainGoTemplateBoilerplateRemoved(t *testing.T) {
	content := readFile(t, "main.go")

	// Must NOT contain GreetService registration
	if strings.Contains(content, "GreetService") {
		t.Error("[P0] main.go still contains GreetService -- template boilerplate must be removed")
	}

	// Must NOT contain time ticker goroutine
	if strings.Contains(content, "time.NewTicker") {
		t.Error("[P0] main.go still contains time.NewTicker -- template ticker goroutine must be removed")
	}

	// Must NOT contain done channel pattern
	doneChannelRe := regexp.MustCompile(`done\s*:=\s*make\s*\(\s*chan\s+struct`)
	if doneChannelRe.MatchString(content) {
		t.Error("[P0] main.go still contains done channel -- template goroutine pattern must be removed")
	}

	// Must NOT contain init() function with event registration
	if strings.Contains(content, "application.RegisterEvent") {
		t.Error("[P0] main.go still contains application.RegisterEvent -- init() function must be removed")
	}

	// Must NOT import "time" package (no longer needed)
	timeImportRe := regexp.MustCompile(`"time"`)
	if timeImportRe.MatchString(content) {
		t.Error("[P0] main.go still imports \"time\" package -- no longer needed after template cleanup")
	}

	// Must still import "embed" and "log"
	if !strings.Contains(content, `"embed"`) {
		t.Error("[P0] main.go missing \"embed\" import -- required for asset embedding")
	}
	if !strings.Contains(content, `"log"`) {
		t.Error("[P0] main.go missing \"log\" import -- required for menu click logging")
	}
}

// 1.4-INTG-002 (P0): greetservice.go deleted
// AC#1 prerequisite: Template leftover file removed

func TestGreetServiceFileDeleted(t *testing.T) {
	if fileExists(t, "greetservice.go") {
		t.Error("[P0] greetservice.go still exists -- must be deleted (template leftover)")
	}
}

// 1.4-INTG-003, 1.4-INTG-004, 1.4-INTG-005 (Story 4-5):
// TestNativeMenuBarCreated, TestPlatformConditionalQuit, and TestMainGoSetupOrdering
// were source-grep assertions on main.go. TestNativeMenuBarCreated and
// TestMainGoSetupOrdering are replaced by tests/boot-smoke (boot path runs to
// the event loop without panic). TestPlatformConditionalQuit is delete-only;
// macOS Quit-in-AppMenu duplicate-prevention is not separately covered after
// deletion (regression risk accepted; structural breakage would surface in the
// build matrix and at first manual smoke).

// ---------------------------------------------------------------------------
// AC#3: Window minimum size 800x600
// Test ID: 1.4-UNIT-005 (P2)
// ---------------------------------------------------------------------------

func TestWindowConfigMinimumSize(t *testing.T) {
	content := readFile(t, "main.go")

	// Must set MinWidth: 800
	minWidthRe := regexp.MustCompile(`MinWidth\s*:\s*800`)
	if !minWidthRe.MatchString(content) {
		t.Error("[P2] main.go missing MinWidth: 800 in window options")
	}

	// Must set MinHeight: 600
	minHeightRe := regexp.MustCompile(`MinHeight\s*:\s*600`)
	if !minHeightRe.MatchString(content) {
		t.Error("[P2] main.go missing MinHeight: 600 in window options")
	}
}

// ---------------------------------------------------------------------------
// AC#3 + Task 1.5: Window configuration (Title, Width, Height, URL)
// ---------------------------------------------------------------------------

func TestWindowConfigOptions(t *testing.T) {
	content := readFile(t, "main.go")

	// Must set Title to "UniDoc PDF Debugger"
	titleRe := regexp.MustCompile(`Title\s*:\s*"UniDoc PDF Debugger"`)
	if !titleRe.MatchString(content) {
		t.Error("[P1] main.go missing Title: \"UniDoc PDF Debugger\" in window options")
	}

	// Must set Width: 1024
	widthRe := regexp.MustCompile(`Width\s*:\s*1024`)
	if !widthRe.MatchString(content) {
		t.Error("[P1] main.go missing Width: 1024 in window options")
	}

	// Must set Height: 768
	heightRe := regexp.MustCompile(`Height\s*:\s*768`)
	if !heightRe.MatchString(content) {
		t.Error("[P1] main.go missing Height: 768 in window options")
	}

	// Must set URL: "/"
	urlRe := regexp.MustCompile(`URL\s*:\s*"/"`)
	if !urlRe.MatchString(content) {
		t.Error("[P1] main.go missing URL: \"/\" in window options")
	}
}

// ---------------------------------------------------------------------------
// Task 1.6/1.7: Window styling (no hidden title bar, correct background)
// ---------------------------------------------------------------------------

func TestWindowStylingOptions(t *testing.T) {
	content := readFile(t, "main.go")

	// Must NOT use MacTitleBarHiddenInset (hides native menu bar)
	if strings.Contains(content, "MacTitleBarHiddenInset") {
		t.Error("[P1] main.go still uses MacTitleBarHiddenInset -- must use standard title bar for menu bar visibility")
	}

	// Must NOT use MacBackdropTranslucent
	if strings.Contains(content, "MacBackdropTranslucent") {
		t.Error("[P1] main.go still uses MacBackdropTranslucent -- must be removed")
	}

	// Must set BackgroundColour to match light theme (#f8fafc = RGB 248, 250, 252)
	bgColourRe := regexp.MustCompile(`NewRGB\s*\(\s*248\s*,\s*250\s*,\s*252\s*\)`)
	if !bgColourRe.MatchString(content) {
		t.Error("[P1] main.go missing BackgroundColour: NewRGB(248, 250, 252) -- must match light theme --color-bg (#f8fafc)")
	}
}

// ---------------------------------------------------------------------------
// Task 1.1: Services field removed from application.Options
// ---------------------------------------------------------------------------

func TestServicesFieldRemoved(t *testing.T) {
	content := readFile(t, "main.go")

	// Must NOT have Services field in application.Options
	if strings.Contains(content, "Services:") && strings.Contains(content, "application.NewService") {
		t.Error("[P0] main.go still has Services field with NewService -- GreetService registration must be removed entirely")
	}
}

// ---------------------------------------------------------------------------
// AC#6: AppProvider with React Context + useReducer
// Test IDs: 1.4-INTG-006 through 1.4-INTG-010
// ---------------------------------------------------------------------------

// 1.4-INTG-006 (P0): useDocumentState.tsx exists with required exports

func TestUseDocumentStateFileExists(t *testing.T) {
	// File uses .tsx extension because it contains JSX (AppProvider component)
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("[P0] frontend/src/hooks/useDocumentState.tsx does not exist -- must be created for AppProvider")
	}
}

// 1.4-INTG-007 (P0): AppProvider, useAppState, useAppDispatch exported

func TestUseDocumentStateExports(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("[P0] frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must export AppProvider component
	appProviderExportRe := regexp.MustCompile(`export\s+(function|const)\s+AppProvider`)
	if !appProviderExportRe.MatchString(content) {
		t.Error("[P0] useDocumentState.tsx missing exported AppProvider component")
	}

	// Must export useAppState hook
	useAppStateExportRe := regexp.MustCompile(`export\s+(function|const)\s+useAppState`)
	if !useAppStateExportRe.MatchString(content) {
		t.Error("[P0] useDocumentState.tsx missing exported useAppState hook")
	}

	// Must export useAppDispatch hook
	useAppDispatchExportRe := regexp.MustCompile(`export\s+(function|const)\s+useAppDispatch`)
	if !useAppDispatchExportRe.MatchString(content) {
		t.Error("[P0] useDocumentState.tsx missing exported useAppDispatch hook")
	}
}

// 1.4-INTG-008 (P1): AppState shape with tabs and activeTabId

func TestAppStateShape(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("[P1] frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must define AppState interface/type
	if !strings.Contains(content, "AppState") {
		t.Error("[P1] useDocumentState.tsx missing AppState type/interface definition")
	}

	// Must have tabs field
	if !strings.Contains(content, "tabs") {
		t.Error("[P1] useDocumentState.tsx AppState missing 'tabs' field")
	}

	// Must have activeTabId field
	if !strings.Contains(content, "activeTabId") {
		t.Error("[P1] useDocumentState.tsx AppState missing 'activeTabId' field")
	}

	// Must define TabState interface/type
	if !strings.Contains(content, "TabState") {
		t.Error("[P1] useDocumentState.tsx missing TabState type/interface definition")
	}

	// TabState must have tabId and fileName
	if !strings.Contains(content, "tabId") {
		t.Error("[P1] useDocumentState.tsx TabState missing 'tabId' field")
	}
	if !strings.Contains(content, "fileName") {
		t.Error("[P1] useDocumentState.tsx TabState missing 'fileName' field")
	}
}

// 1.4-INTG-009 (P1): AppAction types defined

func TestAppActionTypes(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("[P1] frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must define AppAction type
	if !strings.Contains(content, "AppAction") {
		t.Error("[P1] useDocumentState.tsx missing AppAction type definition")
	}

	// Must include OPEN_DOCUMENT action type
	if !strings.Contains(content, "OPEN_DOCUMENT") {
		t.Error("[P1] useDocumentState.tsx missing OPEN_DOCUMENT action type")
	}

	// Must include CLOSE_DOCUMENT action type
	if !strings.Contains(content, "CLOSE_DOCUMENT") {
		t.Error("[P1] useDocumentState.tsx missing CLOSE_DOCUMENT action type")
	}
}

// 1.4-INTG-010 (P1): Two separate contexts for state and dispatch

func TestTwoSeparateContexts(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("[P1] frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must use useReducer
	if !strings.Contains(content, "useReducer") {
		t.Error("[P1] useDocumentState.tsx must use useReducer for state management")
	}

	// Must use createContext (at least twice for state + dispatch)
	createContextCount := strings.Count(content, "createContext")
	if createContextCount < 2 {
		t.Errorf("[P1] useDocumentState.tsx must create two separate contexts (state + dispatch), found %d createContext calls", createContextCount)
	}

	// Must define appReducer function
	if !strings.Contains(content, "appReducer") {
		t.Error("[P1] useDocumentState.tsx missing appReducer function")
	}
}

// ---------------------------------------------------------------------------
// AC#2, AC#5: MainLayout component with allotment two-column layout
// Test IDs: 1.4-UNIT-001, 1.4-UNIT-003, 1.4-INTG-011 through 1.4-INTG-015
// ---------------------------------------------------------------------------

// 1.4-INTG-011 (P0): MainLayout.tsx exists

func TestMainLayoutFileExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/MainLayout.tsx") {
		t.Fatal("[P0] frontend/src/components/MainLayout.tsx does not exist -- must be created for two-column layout")
	}
}

// 1.4-UNIT-001 (Story 4-5): TestMainLayoutTwoColumnStructure was a source-grep
// on MainLayout.tsx asserting the literal `preferredSize={300}`. Story 4-4
// made `preferredSize` conditional on persisted state, breaking the grep while
// behaviour was preserved. Replaced by an extension of
// frontend/src/components/MainLayout.test.tsx (4-5-UNIT-001) that asserts both
// `left-panel` and `right-panel` testids render.

// 1.4-UNIT-003 (P2): Semantic HTML elements used

func TestMainLayoutSemanticHTML(t *testing.T) {
	if !fileExists(t, "frontend/src/components/MainLayout.tsx") {
		t.Fatal("[P2] frontend/src/components/MainLayout.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/MainLayout.tsx")

	// AC#5: Left panel must use <aside> element
	asideRe := regexp.MustCompile(`<aside[^>]*data-testid=["']left-panel["']`)
	if !asideRe.MatchString(content) {
		// Check reverse order
		asideRe2 := regexp.MustCompile(`<aside[^>]*left-panel`)
		if !asideRe2.MatchString(content) {
			t.Error("[P2] MainLayout.tsx left panel must use <aside> element with data-testid='left-panel'")
		}
	}

	// AC#5: Right panel must use <main> element
	mainRe := regexp.MustCompile(`<main[^>]*data-testid=["']right-panel["']`)
	if !mainRe.MatchString(content) {
		// Check reverse order
		mainRe2 := regexp.MustCompile(`<main[^>]*right-panel`)
		if !mainRe2.MatchString(content) {
			t.Error("[P2] MainLayout.tsx right panel must use <main> element with data-testid='right-panel'")
		}
	}
}

// 1.4-INTG-012 (P0): MainLayout data-testid attributes

func TestMainLayoutDataTestIds(t *testing.T) {
	if !fileExists(t, "frontend/src/components/MainLayout.tsx") {
		t.Fatal("[P0] frontend/src/components/MainLayout.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/MainLayout.tsx")

	requiredTestIds := []string{
		"main-layout",
		"left-panel",
		"right-panel",
	}

	for _, testId := range requiredTestIds {
		pattern := `data-testid=["']` + regexp.QuoteMeta(testId) + `["']`
		testIdRe := regexp.MustCompile(pattern)
		if !testIdRe.MatchString(content) {
			t.Errorf("[P0] MainLayout.tsx missing data-testid attribute: %q", testId)
		}
	}
}

// 1.4-INTG-013 (P1): MainLayout placeholder content

func TestMainLayoutPlaceholderContent(t *testing.T) {
	if !fileExists(t, "frontend/src/components/MainLayout.tsx") {
		t.Fatal("[P1] frontend/src/components/MainLayout.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/MainLayout.tsx")

	// Epic 2 replaced placeholders with real components (TreePanel, DetailPanel)
	if !strings.Contains(content, "TreePanel") {
		t.Error("[P1] MainLayout.tsx left panel missing TreePanel component")
	}
	if !strings.Contains(content, "DetailPanel") {
		t.Error("[P1] MainLayout.tsx right panel missing DetailPanel component")
	}
}

// 1.4-INTG-014 (P1): MainLayout uses h-full for full height

func TestMainLayoutFullHeight(t *testing.T) {
	if !fileExists(t, "frontend/src/components/MainLayout.tsx") {
		t.Fatal("[P1] frontend/src/components/MainLayout.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/MainLayout.tsx")

	if !strings.Contains(content, "h-full") {
		t.Error("[P1] MainLayout.tsx missing 'h-full' class for full-height layout (required for allotment to size correctly)")
	}
}

// 1.4-INTG-015 (P1): MainLayout exported as named export (not default)

func TestMainLayoutExported(t *testing.T) {
	if !fileExists(t, "frontend/src/components/MainLayout.tsx") {
		t.Fatal("[P1] frontend/src/components/MainLayout.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/MainLayout.tsx")

	exportRe := regexp.MustCompile(`export\s+function\s+MainLayout`)
	if !exportRe.MatchString(content) {
		t.Error("[P1] MainLayout.tsx must export MainLayout as a named export (export function MainLayout)")
	}
}

// ---------------------------------------------------------------------------
// AC#6: App.jsx integrates AppProvider, MainLayout, and EmptyState
// Test IDs: 1.4-INTG-016 through 1.4-INTG-020
// ---------------------------------------------------------------------------

// 1.4-INTG-016 (P0): App.jsx wraps content in AppProvider

func TestAppJsxWrapsInAppProvider(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must import AppProvider from useDocumentState
	appProviderImportRe := regexp.MustCompile(`import\s+.*AppProvider.*from\s+['"].*hooks/useDocumentState['"]`)
	if !appProviderImportRe.MatchString(content) {
		t.Error("[P0] App.jsx must import AppProvider from './hooks/useDocumentState'")
	}

	// Must render <AppProvider>
	if !strings.Contains(content, "<AppProvider>") {
		t.Error("[P0] App.jsx must render <AppProvider> to provide context to child components")
	}

	// Must close </AppProvider>
	if !strings.Contains(content, "</AppProvider>") {
		t.Error("[P0] App.jsx must close </AppProvider>")
	}
}

// 1.4-INTG-017 (P0): App.jsx imports and conditionally renders MainLayout

func TestAppJsxRendersMainLayout(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must import MainLayout
	mainLayoutImportRe := regexp.MustCompile(`import\s+.*MainLayout.*from\s+['"].*components/MainLayout['"]`)
	if !mainLayoutImportRe.MatchString(content) {
		t.Error("[P0] App.jsx must import MainLayout from './components/MainLayout'")
	}

	// Must reference MainLayout in render (conditional)
	if !strings.Contains(content, "<MainLayout") {
		t.Error("[P0] App.jsx must render <MainLayout /> (conditionally, when document is open)")
	}
}

// 1.4-INTG-018 (P0): App.jsx still renders EmptyState

func TestAppJsxStillRendersEmptyState(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must still import EmptyState
	emptyStateImportRe := regexp.MustCompile(`import\s+.*EmptyState.*from`)
	if !emptyStateImportRe.MatchString(content) {
		t.Error("[P0] App.jsx must still import EmptyState component")
	}

	// Must still render <EmptyState
	if !strings.Contains(content, "<EmptyState") {
		t.Error("[P0] App.jsx must still render <EmptyState /> (when no document is open)")
	}
}

// 1.4-INTG-019 (P1): App.jsx uses useAppState hook for conditional rendering

func TestAppJsxUsesAppStateHook(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must import useAppState
	useAppStateImportRe := regexp.MustCompile(`import\s+.*useAppState.*from\s+['"].*hooks/useDocumentState['"]`)
	if !useAppStateImportRe.MatchString(content) {
		t.Error("[P1] App.jsx must import useAppState from './hooks/useDocumentState'")
	}

	// Must call useAppState()
	if !strings.Contains(content, "useAppState()") {
		t.Error("[P1] App.jsx must call useAppState() to access application state")
	}

	// Must reference activeTabId for conditional rendering
	if !strings.Contains(content, "activeTabId") {
		t.Error("[P1] App.jsx must use activeTabId from state for conditional rendering")
	}
}

// 1.4-INTG-020 (P1): AppContent wrapper component exists inside App.jsx
// Hooks must be called inside AppProvider, so a wrapper component is needed

func TestAppContentWrapperComponent(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must define AppContent (or similar inner component) that uses hooks inside AppProvider
	if !strings.Contains(content, "AppContent") {
		t.Error("[P1] App.jsx must define an AppContent wrapper component (hooks must be called inside AppProvider)")
	}

	// AppContent must be rendered inside AppProvider
	// Pattern: <AppProvider> ... <AppContent ... </AppProvider>
	appContentInsideProviderRe := regexp.MustCompile(`(?s)<AppProvider>.*<AppContent.*</AppProvider>`)
	if !appContentInsideProviderRe.MatchString(content) {
		t.Error("[P1] App.jsx: AppContent must be rendered inside <AppProvider> (hooks require context)")
	}
}

// ---------------------------------------------------------------------------
// Project rules: No barrel files in new directories
// ---------------------------------------------------------------------------

func TestNoBarrelFilesInNewDirectories(t *testing.T) {
	root := projectRoot(t)

	// Check hooks directory (newly created)
	dirsToCheck := []string{
		"frontend/src/hooks",
	}

	for _, dir := range dirsToCheck {
		for _, ext := range []string{"index.ts", "index.tsx"} {
			indexPath := filepath.Join(root, dir, ext)
			if _, err := os.Stat(indexPath); err == nil {
				t.Errorf("[P0] %s/%s exists -- barrel files are forbidden by project rules", dir, ext)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 1.4-UNIT-002 (P1): Platform-aware shortcut hint renders correctly
// AC#1 (related): EmptyState uses getShortcutHint from platform.ts
// ---------------------------------------------------------------------------

func TestPlatformShortcutHintInEmptyState(t *testing.T) {
	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must import getShortcutHint from platform.ts
	if !strings.Contains(content, "getShortcutHint") {
		t.Error("[P1] EmptyState.tsx missing getShortcutHint import -- required for platform-aware shortcut hints")
	}

	// Must render shortcut hint with data-testid
	shortcutHintRe := regexp.MustCompile(`data-testid=["']shortcut-hint["']`)
	if !shortcutHintRe.MatchString(content) {
		t.Error("[P1] EmptyState.tsx missing data-testid='shortcut-hint' element for keyboard shortcut display")
	}

	// Must call getShortcutHint with 'O' key for Open shortcut
	if !strings.Contains(content, `getShortcutHint('O')`) && !strings.Contains(content, `getShortcutHint("O")`) {
		t.Error("[P1] EmptyState.tsx must call getShortcutHint('O') for the Open shortcut hint")
	}

	// Verify platform.ts has getPlatformModifier logic
	platformContent := readFile(t, "frontend/src/lib/platform.ts")

	// Must export getPlatformModifier
	if !strings.Contains(platformContent, "getPlatformModifier") {
		t.Error("[P1] platform.ts missing getPlatformModifier function")
	}

	// Must check for Mac/macOS platform
	if !strings.Contains(platformContent, "Mac") {
		t.Error("[P1] platform.ts must detect macOS platform for Cmd modifier")
	}

	// Must return 'Cmd' for macOS
	if !strings.Contains(platformContent, `"Cmd"`) && !strings.Contains(platformContent, `'Cmd'`) {
		t.Error("[P1] platform.ts must return 'Cmd' for macOS platform")
	}

	// Must return 'Ctrl' for non-macOS
	if !strings.Contains(platformContent, `"Ctrl"`) && !strings.Contains(platformContent, `'Ctrl'`) {
		t.Error("[P1] platform.ts must return 'Ctrl' for non-macOS platform")
	}
}

// ---------------------------------------------------------------------------
// 1.4-UNIT-004 (P2): Focus indicator applied to interactive elements
// AC#5 (related): Accessibility focus-visible ring on buttons
// ---------------------------------------------------------------------------

func TestFocusIndicatorOnInteractiveElements(t *testing.T) {
	// Check EmptyState's Open File button has focus-visible ring
	content := readFile(t, "frontend/src/components/EmptyState.tsx")

	// Must have focus-visible:ring-2 class for 2px ring
	if !strings.Contains(content, "focus-visible:ring-2") {
		t.Error("[P2] EmptyState.tsx Open File button missing focus-visible:ring-2 class for focus indicator")
	}

	// Must use border-focus color for the ring (Blue 500)
	if !strings.Contains(content, "focus-visible:ring-border-focus") {
		t.Error("[P2] EmptyState.tsx Open File button missing focus-visible:ring-border-focus class (Blue 500 ring color)")
	}

	// Must remove default outline to avoid double focus indicator
	if !strings.Contains(content, "focus-visible:outline-none") {
		t.Error("[P2] EmptyState.tsx Open File button missing focus-visible:outline-none to prevent double focus indicator")
	}
}

// ---------------------------------------------------------------------------
// 1.4-INTG-021 (P1): ErrorBoundary wraps Allotment in MainLayout
// Defensive: ErrorBoundary catches render errors in split panels
// ---------------------------------------------------------------------------

func TestMainLayoutErrorBoundary(t *testing.T) {
	if !fileExists(t, "frontend/src/components/MainLayout.tsx") {
		t.Fatal("[P1] frontend/src/components/MainLayout.tsx does not exist")
	}

	content := readFile(t, "frontend/src/components/MainLayout.tsx")

	// Must import ErrorBoundary
	if !strings.Contains(content, "ErrorBoundary") {
		t.Skip("[P1] MainLayout.tsx does not use ErrorBoundary -- defensive addition, not required by story spec")
	}

	// If ErrorBoundary is imported, verify it's from the correct location
	errorBoundaryImportRe := regexp.MustCompile(`import\s+.*ErrorBoundary.*from\s+['"].*ErrorBoundary['"]`)
	if !errorBoundaryImportRe.MatchString(content) {
		t.Error("[P1] MainLayout.tsx ErrorBoundary import not from expected module")
	}
}

// ---------------------------------------------------------------------------
// 1.4-INTG-022 (P1): useDocumentState initial state is correct
// Verify initialState has empty tabs and null activeTabId
// ---------------------------------------------------------------------------

func TestUseDocumentStateInitialState(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("[P1] frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must define initialState
	if !strings.Contains(content, "initialState") {
		t.Error("[P1] useDocumentState.tsx missing initialState definition")
	}

	// initialState must have empty tabs array
	emptyTabsRe := regexp.MustCompile(`tabs\s*:\s*\[\s*\]`)
	if !emptyTabsRe.MatchString(content) {
		t.Error("[P1] useDocumentState.tsx initialState must have tabs: []")
	}

	// initialState must have null activeTabId
	nullActiveTabRe := regexp.MustCompile(`activeTabId\s*:\s*null`)
	if !nullActiveTabRe.MatchString(content) {
		t.Error("[P1] useDocumentState.tsx initialState must have activeTabId: null")
	}
}

// ---------------------------------------------------------------------------
// 1.4-INTG-023 (P2): useDocumentState context null guards
// Hooks throw meaningful errors when used outside AppProvider
// ---------------------------------------------------------------------------

func TestUseDocumentStateContextNullGuards(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("[P2] frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// useAppState must throw when context is null
	if !strings.Contains(content, "useAppState must be used within") {
		t.Error("[P2] useAppState hook missing null-guard error message")
	}

	// useAppDispatch must throw when context is null
	if !strings.Contains(content, "useAppDispatch must be used within") {
		t.Error("[P2] useAppDispatch hook missing null-guard error message")
	}
}

// ---------------------------------------------------------------------------
// App.jsx keeps .jsx extension (regression guard from Story 1.3)
// ---------------------------------------------------------------------------

func TestAppJsxExtensionPreserved(t *testing.T) {
	if !fileExists(t, "frontend/src/App.jsx") {
		t.Fatal("[P0] frontend/src/App.jsx does not exist -- must keep .jsx extension per project convention")
	}

	if fileExists(t, "frontend/src/App.tsx") {
		t.Error("[P0] frontend/src/App.tsx exists -- App should remain .jsx per project convention")
	}
}
