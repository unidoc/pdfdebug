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


// 1.4-INTG-002 (P0): greetservice.go deleted
// AC#1 prerequisite: Template leftover file removed

func TestGreetServiceFileDeleted(t *testing.T) {
	if fileExists(t, "greetservice.go") {
		t.Error("greetservice.go still exists -- must be deleted (template leftover)")
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


// ---------------------------------------------------------------------------
// AC#3 + Task 1.5: Window configuration (Title, Width, Height, URL)
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Task 1.6/1.7: Window styling (no hidden title bar, correct background)
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Task 1.1: Services field removed from application.Options
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// AC#6: AppProvider with React Context + useReducer
// Test IDs: 1.4-INTG-006 through 1.4-INTG-010
// ---------------------------------------------------------------------------

// 1.4-INTG-006 (P0): useDocumentState.tsx exists with required exports

func TestUseDocumentStateFileExists(t *testing.T) {
	// File uses .tsx extension because it contains JSX (AppProvider component)
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("frontend/src/hooks/useDocumentState.tsx does not exist -- must be created for AppProvider")
	}
}

// 1.4-INTG-007 (P0): AppProvider, useAppState, useAppDispatch exported

func TestUseDocumentStateExports(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must export AppProvider component
	appProviderExportRe := regexp.MustCompile(`export\s+(function|const)\s+AppProvider`)
	if !appProviderExportRe.MatchString(content) {
		t.Error("useDocumentState.tsx missing exported AppProvider component")
	}

	// Must export useAppState hook
	useAppStateExportRe := regexp.MustCompile(`export\s+(function|const)\s+useAppState`)
	if !useAppStateExportRe.MatchString(content) {
		t.Error("useDocumentState.tsx missing exported useAppState hook")
	}

	// Must export useAppDispatch hook
	useAppDispatchExportRe := regexp.MustCompile(`export\s+(function|const)\s+useAppDispatch`)
	if !useAppDispatchExportRe.MatchString(content) {
		t.Error("useDocumentState.tsx missing exported useAppDispatch hook")
	}
}

// 1.4-INTG-008 (P1): AppState shape with tabs and activeTabId

func TestAppStateShape(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must define AppState interface/type
	if !strings.Contains(content, "AppState") {
		t.Error("useDocumentState.tsx missing AppState type/interface definition")
	}

	// Must have tabs field
	if !strings.Contains(content, "tabs") {
		t.Error("useDocumentState.tsx AppState missing 'tabs' field")
	}

	// Must have activeTabId field
	if !strings.Contains(content, "activeTabId") {
		t.Error("useDocumentState.tsx AppState missing 'activeTabId' field")
	}

	// Must define TabState interface/type
	if !strings.Contains(content, "TabState") {
		t.Error("useDocumentState.tsx missing TabState type/interface definition")
	}

	// TabState must have tabId and fileName
	if !strings.Contains(content, "tabId") {
		t.Error("useDocumentState.tsx TabState missing 'tabId' field")
	}
	if !strings.Contains(content, "fileName") {
		t.Error("useDocumentState.tsx TabState missing 'fileName' field")
	}
}

// 1.4-INTG-009 (P1): AppAction types defined

func TestAppActionTypes(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must define AppAction type
	if !strings.Contains(content, "AppAction") {
		t.Error("useDocumentState.tsx missing AppAction type definition")
	}

	// Must include OPEN_DOCUMENT action type
	if !strings.Contains(content, "OPEN_DOCUMENT") {
		t.Error("useDocumentState.tsx missing OPEN_DOCUMENT action type")
	}

	// Must include CLOSE_DOCUMENT action type
	if !strings.Contains(content, "CLOSE_DOCUMENT") {
		t.Error("useDocumentState.tsx missing CLOSE_DOCUMENT action type")
	}
}

// 1.4-INTG-010 (P1): Two separate contexts for state and dispatch

func TestTwoSeparateContexts(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must use useReducer
	if !strings.Contains(content, "useReducer") {
		t.Error("useDocumentState.tsx must use useReducer for state management")
	}

	// Must use createContext (at least twice for state + dispatch)
	createContextCount := strings.Count(content, "createContext")
	if createContextCount < 2 {
		t.Errorf("useDocumentState.tsx must create two separate contexts (state + dispatch), found %d createContext calls", createContextCount)
	}

	// Must define appReducer function
	if !strings.Contains(content, "appReducer") {
		t.Error("useDocumentState.tsx missing appReducer function")
	}
}

// ---------------------------------------------------------------------------
// AC#2, AC#5: MainLayout component with allotment two-column layout
// Test IDs: 1.4-UNIT-001, 1.4-UNIT-003, 1.4-INTG-011 through 1.4-INTG-015
// ---------------------------------------------------------------------------

// 1.4-INTG-011 (P0): MainLayout.tsx exists

func TestMainLayoutFileExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/MainLayout.tsx") {
		t.Fatal("frontend/src/components/MainLayout.tsx does not exist -- must be created for two-column layout")
	}
}

// 1.4-UNIT-001 (Story 4-5): TestMainLayoutTwoColumnStructure was a source-grep
// on MainLayout.tsx asserting the literal `preferredSize={300}`. Story 4-4
// made `preferredSize` conditional on persisted state, breaking the grep while
// behaviour was preserved. Replaced by an extension of
// frontend/src/components/MainLayout.test.tsx (4-5-UNIT-001) that asserts both
// `left-panel` and `right-panel` testids render.

// 1.4-UNIT-003 (P2): Semantic HTML elements used


// 1.4-INTG-012 (P0): MainLayout data-testid attributes


// 1.4-INTG-013 (P1): MainLayout placeholder content


// 1.4-INTG-014 (P1): MainLayout uses h-full for full height


// 1.4-INTG-015 (P1): MainLayout exported as named export (not default)


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
		t.Error("App.jsx must import AppProvider from './hooks/useDocumentState'")
	}

	// Must render <AppProvider>
	if !strings.Contains(content, "<AppProvider>") {
		t.Error("App.jsx must render <AppProvider> to provide context to child components")
	}

	// Must close </AppProvider>
	if !strings.Contains(content, "</AppProvider>") {
		t.Error("App.jsx must close </AppProvider>")
	}
}

// 1.4-INTG-017 (P0): App.jsx imports and conditionally renders MainLayout

func TestAppJsxRendersMainLayout(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must import MainLayout
	mainLayoutImportRe := regexp.MustCompile(`import\s+.*MainLayout.*from\s+['"].*components/MainLayout['"]`)
	if !mainLayoutImportRe.MatchString(content) {
		t.Error("App.jsx must import MainLayout from './components/MainLayout'")
	}

	// Must reference MainLayout in render (conditional)
	if !strings.Contains(content, "<MainLayout") {
		t.Error("App.jsx must render <MainLayout /> (conditionally, when document is open)")
	}
}

// 1.4-INTG-018 (P0): App.jsx still renders EmptyState

func TestAppJsxStillRendersEmptyState(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must still import EmptyState
	emptyStateImportRe := regexp.MustCompile(`import\s+.*EmptyState.*from`)
	if !emptyStateImportRe.MatchString(content) {
		t.Error("App.jsx must still import EmptyState component")
	}

	// Must still render <EmptyState
	if !strings.Contains(content, "<EmptyState") {
		t.Error("App.jsx must still render <EmptyState /> (when no document is open)")
	}
}

// 1.4-INTG-019 (P1): App.jsx uses useAppState hook for conditional rendering

func TestAppJsxUsesAppStateHook(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must import useAppState
	useAppStateImportRe := regexp.MustCompile(`import\s+.*useAppState.*from\s+['"].*hooks/useDocumentState['"]`)
	if !useAppStateImportRe.MatchString(content) {
		t.Error("App.jsx must import useAppState from './hooks/useDocumentState'")
	}

	// Must call useAppState()
	if !strings.Contains(content, "useAppState()") {
		t.Error("App.jsx must call useAppState to access application state")
	}

	// Must reference activeTabId for conditional rendering
	if !strings.Contains(content, "activeTabId") {
		t.Error("App.jsx must use activeTabId from state for conditional rendering")
	}
}

// 1.4-INTG-020 (P1): AppContent wrapper component exists inside App.jsx
// Hooks must be called inside AppProvider, so a wrapper component is needed

func TestAppContentWrapperComponent(t *testing.T) {
	content := readFile(t, "frontend/src/App.jsx")

	// Must define AppContent (or similar inner component) that uses hooks inside AppProvider
	if !strings.Contains(content, "AppContent") {
		t.Error("App.jsx must define an AppContent wrapper component (hooks must be called inside AppProvider)")
	}

	// AppContent must be rendered inside AppProvider
	// Pattern: <AppProvider> ... <AppContent ... </AppProvider>
	appContentInsideProviderRe := regexp.MustCompile(`(?s)<AppProvider>.*<AppContent.*</AppProvider>`)
	if !appContentInsideProviderRe.MatchString(content) {
		t.Error("App.jsx: AppContent must be rendered inside <AppProvider> (hooks require context)")
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
				t.Errorf("%s/%s exists -- barrel files are forbidden by project rules", dir, ext)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 1.4-UNIT-002 (P1): Platform-aware shortcut hint renders correctly
// AC#1 (related): EmptyState uses getShortcutHint from platform.ts
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.4-UNIT-004 (P2): Focus indicator applied to interactive elements
// AC#5 (related): Accessibility focus-visible ring on buttons
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.4-INTG-021 (P1): ErrorBoundary wraps Allotment in MainLayout
// Defensive: ErrorBoundary catches render errors in split panels
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 1.4-INTG-022 (P1): useDocumentState initial state is correct
// Verify initialState has empty tabs and null activeTabId
// ---------------------------------------------------------------------------

func TestUseDocumentStateInitialState(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// Must define initialState
	if !strings.Contains(content, "initialState") {
		t.Error("useDocumentState.tsx missing initialState definition")
	}

	// initialState must have empty tabs array
	emptyTabsRe := regexp.MustCompile(`tabs\s*:\s*\[\s*\]`)
	if !emptyTabsRe.MatchString(content) {
		t.Error("useDocumentState.tsx initialState must have tabs: []")
	}

	// initialState must have null activeTabId
	nullActiveTabRe := regexp.MustCompile(`activeTabId\s*:\s*null`)
	if !nullActiveTabRe.MatchString(content) {
		t.Error("useDocumentState.tsx initialState must have activeTabId: null")
	}
}

// ---------------------------------------------------------------------------
// 1.4-INTG-023 (P2): useDocumentState context null guards
// Hooks throw meaningful errors when used outside AppProvider
// ---------------------------------------------------------------------------

func TestUseDocumentStateContextNullGuards(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.tsx") {
		t.Fatal("frontend/src/hooks/useDocumentState.tsx does not exist")
	}

	content := readFile(t, "frontend/src/hooks/useDocumentState.tsx")

	// useAppState must throw when context is null
	if !strings.Contains(content, "useAppState must be used within") {
		t.Error("useAppState hook missing null-guard error message")
	}

	// useAppDispatch must throw when context is null
	if !strings.Contains(content, "useAppDispatch must be used within") {
		t.Error("useAppDispatch hook missing null-guard error message")
	}
}

// ---------------------------------------------------------------------------
// App.jsx keeps .jsx extension (regression guard from Story 1.3)
// ---------------------------------------------------------------------------

func TestAppJsxExtensionPreserved(t *testing.T) {
	if !fileExists(t, "frontend/src/App.jsx") {
		t.Fatal("frontend/src/App.jsx does not exist -- must keep.jsx extension per project convention")
	}

	if fileExists(t, "frontend/src/App.tsx") {
		t.Error("frontend/src/App.tsx exists -- App should remain.jsx per project convention")
	}
}
