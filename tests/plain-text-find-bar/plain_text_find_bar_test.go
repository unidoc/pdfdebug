// Package plain_text_find_bar_test provides acceptance tests for Find Bar in
// Plain Text View.
//
// Test pyramid for this story (per the user's "favour API/integration over
// E2E" directive + Task 7.3 of the story spec: no Playwright in v1):
//
//   - Frontend pure-function logic (findMatches.ts): unit tested in
//     findMatches.test.ts. -- match algorithm, non-Latin-1
//     codepoint detection, perf budget.
//   - Frontend hook logic (useFindBar.ts): hook tested in useFindBar.test.ts.
//     keystroke
//     handlers, navigation, wrap-status, openedOnce flag, focus-guard.
//   - Frontend component contract (FindBar.tsx): component tested in
//     FindBar.test.tsx. testids, aria, hint, keyboard
//     wiring, disabled state, tab order, aria-live.
//   - Frontend integration (PlainTextView.find.test.tsx): full FindBar mounted
//     over PlainTextView. auto-scroll, inner-tab vs
//     document-tab persistence, cmd+F gate on data===null, viewport
//     re-render on scroll.
//   - Frontend reducer (useDocumentState extension): per-tab
//     findCaseSensitive.
//   - Project docs: project-context.md gains the Plain Text Find Bar Rules
//     section; deferred-work.md marks the 9-12 "Search within Plain Text
//     payload" entry as RESOLVED and records new deferrals.
//
// This Go harness pins structural invariants only: file existence, required
// exports, data-testid strings, Tailwind token registration, project-context
// section heading, deferred-work bookkeeping. Behavioral assertions live in
// the colocated Vitest suites. Mirrors the convention established by
// tests/async-plain-text-load/.
//
// Run: cd tests/plain-text-find-bar && go test -v -count=1 ./...
package plain_text_find_bar_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot walks up from the working directory until it finds the project
// go.mod (module unidoc-pdf-debugger), and returns its absolute path.
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

// readSource reads a file relative to the project root.
func readSource(t *testing.T, relPath string) string {
	t.Helper()
	root := projectRoot(t)
	content, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		t.Fatalf("cannot read %s: %v", relPath, err)
	}
	return string(content)
}

// fileExists returns true when the project-relative path resolves to an
// existing regular file.
func fileExists(t *testing.T, relPath string) bool {
	t.Helper()
	root := projectRoot(t)
	info, err := os.Stat(filepath.Join(root, relPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		t.Fatalf("stat %s: %v", relPath, err)
	}
	return !info.IsDir()
}

// ---------------------------------------------------------------------------
// findMatches pure-function module exists with the documented exports.
// Behavioral tests live in findMatches.test.ts.
// ---------------------------------------------------------------------------

// TestFindMatchesModuleExists asserts frontend/src/lib/findMatches.ts exists.
func TestFindMatchesModuleExists(t *testing.T) {
	if !fileExists(t, "frontend/src/lib/findMatches.ts") {
		t.Fatalf("frontend/src/lib/findMatches.ts must exist (Task 2.1)")
	}
}

// TestFindMatchesExports asserts findMatches.ts exports findMatches,
// buildLineStartOffsets and the Match interface, and that the Match shape carries
// start, end and line.
func TestFindMatchesExports(t *testing.T) {
	src := readSource(t, "frontend/src/lib/findMatches.ts")
	required := []string{
		"export function findMatches",
		"export function buildLineStartOffsets",
		"export interface Match",
	}
	for _, sym := range required {
		if !strings.Contains(src, sym) {
			t.Errorf("findMatches.ts must contain %q (Task 2.1)", sym)
		}
	}
	// Match shape: { start, end, line }.
	for _, field := range []string{"start", "end", "line"} {
		if !strings.Contains(src, field) {
			t.Errorf("findMatches.ts Match interface must declare %q field", field)
		}
	}
}

// TestFindMatchesTestExists asserts the co-located findMatches.test.ts exists.
func TestFindMatchesTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/lib/findMatches.test.ts") {
		t.Fatalf("frontend/src/lib/findMatches.test.ts must exist (Task 2.3)")
	}
}

// ---------------------------------------------------------------------------
// .. -- useFindBar hook surface.
// ---------------------------------------------------------------------------

// TestUseFindBarHookExists asserts frontend/src/hooks/useFindBar.ts exists.
func TestUseFindBarHookExists(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useFindBar.ts") {
		t.Fatalf("frontend/src/hooks/useFindBar.ts must exist (Task 3.1)")
	}
}

// TestUseFindBarExport asserts useFindBar.ts exports useFindBar.
func TestUseFindBarExport(t *testing.T) {
	src := readSource(t, "frontend/src/hooks/useFindBar.ts")
	if !strings.Contains(src, "export function useFindBar") &&
		!strings.Contains(src, "export const useFindBar") {
		t.Errorf("useFindBar.ts must export useFindBar (Task 3.1)")
	}
}

// TestUseFindBarTestExists asserts the co-located useFindBar.test.ts exists.
func TestUseFindBarTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useFindBar.test.ts") {
		t.Fatalf("frontend/src/hooks/useFindBar.test.ts must exist (Task 3.7)")
	}
}

// ---------------------------------------------------------------------------
// FindBar component surface.
// ---------------------------------------------------------------------------

// TestFindBarComponentExists asserts frontend/src/components/FindBar.tsx exists.
func TestFindBarComponentExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/FindBar.tsx") {
		t.Fatalf("frontend/src/components/FindBar.tsx must exist (Task 4.1)")
	}
}

// TestFindBarExport asserts FindBar.tsx exports FindBar.
func TestFindBarExport(t *testing.T) {
	src := readSource(t, "frontend/src/components/FindBar.tsx")
	if !strings.Contains(src, "export function FindBar") &&
		!strings.Contains(src, "export const FindBar") {
		t.Errorf("FindBar.tsx must export FindBar (Task 4.1)")
	}
}

// TestFindBarTestExists asserts the co-located FindBar.test.tsx exists.
func TestFindBarTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/FindBar.test.tsx") {
		t.Fatalf("frontend/src/components/FindBar.test.tsx must exist (Task 4.5)")
	}
}

// TestFindBarTestIds asserts FindBar.tsx renders the documented data-testids for
// its static structure (input, count, case toggle, prev, next, close). The
// conditional testids -- wrap-status, non-Latin-1 hint, gutter marker -- are
// asserted in their dedicated component and integration tests.
func TestFindBarTestIds(t *testing.T) {
	src := readSource(t, "frontend/src/components/FindBar.tsx")
	requiredTestIds := []string{
		"plain-text-find-bar",          // root
		"plain-text-find-input",        // input
		"plain-text-find-count",        // count
		"plain-text-find-case-toggle",  // case toggle
		"plain-text-find-prev",         // prev
		"plain-text-find-next",         // next
		"plain-text-find-close",        // close
		"plain-text-find-wrap-status",  // wrap status
	}
	for _, tid := range requiredTestIds {
		if !strings.Contains(src, tid) {
			t.Errorf("FindBar.tsx missing data-testid=%q", tid)
		}
	}
}

// TestFindBarAriaContract asserts FindBar.tsx wires the documented aria attributes:
// role=search, aria-label="Find in plain text", aria-live="polite" on count and
// wrap-status, aria-pressed on the case toggle.
func TestFindBarAriaContract(t *testing.T) {
	src := readSource(t, "frontend/src/components/FindBar.tsx")
	requiredAria := []string{
		`role="search"`,                  // root role
		`Find in plain text`,             // aria-label root
		`Find query`,                     // aria-label input
		`Match case`,                     // aria-label case toggle
		`Previous match`,                 // aria-label prev
		`Next match`,                     // aria-label next
		`Close find`,                     // aria-label close
		`aria-live="polite"`,             // count + wrap-status live region
		`aria-pressed`,                   // case toggle pressed state
	}
	for _, frag := range requiredAria {
		if !strings.Contains(src, frag) {
			t.Errorf("FindBar.tsx must contain %q", frag)
		}
	}
}

// TestFindBarNonLatin1Hint asserts FindBar.tsx wires the non-Latin-1 hint testid
// and its documented copy, plus the aria-describedby linkage from the input to the
// hint id.
func TestFindBarNonLatin1Hint(t *testing.T) {
	src := readSource(t, "frontend/src/components/FindBar.tsx")
	requiredFragments := []string{
		"plain-text-find-non-latin1-hint", // testid + id
		`Non-Latin-1 characters won't match`, // exact copy
		`aria-describedby`, // linkage
	}
	for _, frag := range requiredFragments {
		if !strings.Contains(src, frag) {
			t.Errorf("FindBar.tsx must contain %q", frag)
		}
	}
}

// ---------------------------------------------------------------------------
// PlainTextView integration.
// ---------------------------------------------------------------------------

// TestPlainTextViewImportsFindBar asserts PlainTextView.tsx imports useFindBar and
// FindBar.
func TestPlainTextViewImportsFindBar(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	required := []string{
		"useFindBar",
		"FindBar",
	}
	for _, sym := range required {
		if !strings.Contains(src, sym) {
			t.Errorf("PlainTextView.tsx must reference %q (Task 5.1)", sym)
		}
	}
}

// TestPlainTextViewMarkTestIds asserts PlainTextView.tsx wires the active-match and
// non-active-match testids for the per-row <mark> overlay.
func TestPlainTextViewMarkTestIds(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	required := []string{
		"plain-text-find-active-match",  // active match
		"plain-text-find-match",         // inactive match
	}
	for _, tid := range required {
		if !strings.Contains(src, tid) {
			t.Errorf("PlainTextView.tsx must reference data-testid=%q (/ Task 5.3)", tid)
		}
	}
}

// TestPlainTextViewGutterMarker asserts PlainTextView.tsx wires the gutter density
// marker testid prefix. The {lineNo} suffix is asserted in the integration Vitest.
func TestPlainTextViewGutterMarker(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	if !strings.Contains(src, "plain-text-find-gutter-marker") {
		t.Errorf("PlainTextView.tsx must reference data-testid=plain-text-find-gutter-marker-{lineNo} (/ Task 5.4)")
	}
}

// TestPlainTextFindTestExists asserts PlainTextView.find.test.tsx exists for the
// integration assertions: per-row <mark>, gutter marker, auto-scroll, inner-tab
// persistence, tabId reset, Esc scope and the Cmd+F gate on data === null.
func TestPlainTextFindTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/PlainTextView.find.test.tsx") {
		t.Fatalf("frontend/src/components/PlainTextView.find.test.tsx must exist (Task 5.6)")
	}
}

// ---------------------------------------------------------------------------
// TabState.findCaseSensitive + SET_FIND_CASE_SENSITIVE.
// ---------------------------------------------------------------------------

// TestTabStateCarriesFindCaseSensitive asserts TabState declares
// findCaseSensitive: boolean.
func TestTabStateCarriesFindCaseSensitive(t *testing.T) {
	src := readSource(t, "frontend/src/hooks/useDocumentState.tsx")
	if !strings.Contains(src, "findCaseSensitive") {
		t.Fatalf("useDocumentState.tsx must declare findCaseSensitive on TabState")
	}
	// Pinned type: boolean. Catches accidental migration to a tri-state.
	if !strings.Contains(src, "findCaseSensitive: boolean") {
		t.Errorf("TabState.findCaseSensitive must be typed boolean (Task 1.1)")
	}
}

// TestSetFindCaseSensitiveAction asserts the AppAction union declares
// SET_FIND_CASE_SENSITIVE with payload { tabId, value }.
func TestSetFindCaseSensitiveAction(t *testing.T) {
	src := readSource(t, "frontend/src/hooks/useDocumentState.tsx")
	if !strings.Contains(src, "SET_FIND_CASE_SENSITIVE") {
		t.Fatalf("useDocumentState.tsx must declare the SET_FIND_CASE_SENSITIVE action")
	}
	// Payload contract carries tabId + value (boolean).
	if !strings.Contains(src, "tabId") || !strings.Contains(src, "value") {
		t.Errorf("SET_FIND_CASE_SENSITIVE payload must contain { tabId, value } (Task 1.2)")
	}
}

// TestReducerFindTestExists asserts useDocumentState.find.test.tsx exists with
// reducer coverage for SET_FIND_CASE_SENSITIVE, OPEN_DOCUMENT defaulting and
// CLOSE_DOCUMENT cleanup.
func TestReducerFindTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.find.test.tsx") {
		t.Fatalf("frontend/src/hooks/useDocumentState.find.test.tsx must exist (Task 1.3)")
	}
}

// ---------------------------------------------------------------------------
// Tailwind tokens registered in style.css.
// ---------------------------------------------------------------------------

// TestFindColorTokens asserts style.css registers the five find-color CSS custom
// properties and exposes them via @theme inline. Only the variable names and the
// @theme registration are pinned; the RGB values stay designer-tunable.
func TestFindColorTokens(t *testing.T) {
	src := readSource(t, "frontend/src/style.css")
	requiredVars := []string{
		"--color-find-match",
		"--color-find-match-fg",
		"--color-find-active",
		"--color-find-active-fg",
		"--color-find-gutter",
	}
	for _, v := range requiredVars {
		if !strings.Contains(src, v) {
			t.Errorf("style.css must declare %q", v)
		}
	}
	if !strings.Contains(src, "@theme inline") {
		t.Errorf("style.css must keep the @theme inline block for color token registration (Task 6.1)")
	}
}

// ---------------------------------------------------------------------------
// Out-of-scope guards: no Cmd+G rebinding (F3 only).
// ---------------------------------------------------------------------------

// TestNoCmdGRebinding asserts useFindBar.ts does not bind Cmd+G / Ctrl+G for
// Find-Next. App.jsx owns that combo for Open Go to Page, so the only Find-Next
// keystroke is F3 / Shift+F3.
func TestNoCmdGRebinding(t *testing.T) {
	src := readSource(t, "frontend/src/hooks/useFindBar.ts")
	// Use a literal lowercase-and-uppercase form check; either is a regression.
	for _, forbidden := range []string{`e.key === 'g'`, `e.key === 'G'`, `key: 'g'`, `key: 'G'`} {
		if strings.Contains(src, forbidden) {
			t.Errorf("useFindBar.ts must NOT bind %q (Cmd+G is owned by App.jsx for Open Go to Page; Find-Next is F3 only)", forbidden)
		}
	}
	// F3 binding must be present.
	if !strings.Contains(src, "F3") {
		t.Errorf("useFindBar.ts must bind F3 for Find-Next")
	}
}

