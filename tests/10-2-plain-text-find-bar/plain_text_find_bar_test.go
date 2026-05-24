// Package plain_text_find_bar_test provides acceptance tests for Story 10.2:
// Find Bar in Plain Text View.
//
// TDD RED PHASE: these tests MUST fail until Story 10-2 is implemented.
//
// Test pyramid for this story (per the user's "favour API/integration over
// E2E" directive + Task 7.3 of the story spec: no Playwright in v1):
//
//   - Frontend pure-function logic (findMatches.ts): unit tested in
//     findMatches.test.ts. AC4, AC12, AC19 -- match algorithm, non-Latin-1
//     codepoint detection, perf budget.
//   - Frontend hook logic (useFindBar.ts): hook tested in useFindBar.test.ts.
//     AC1, AC2, AC3, AC7, AC8, AC9, AC10, AC11, AC13, AC15, AC22 -- keystroke
//     handlers, navigation, wrap-status, openedOnce flag, focus-guard.
//   - Frontend component contract (FindBar.tsx): component tested in
//     FindBar.test.tsx. AC1 testids, AC10 aria, AC12 hint, AC16 keyboard
//     wiring, AC18 disabled state, AC20 tab order, AC21 aria-live.
//   - Frontend integration (PlainTextView.find.test.tsx): full FindBar mounted
//     over PlainTextView. AC5, AC6, AC7 auto-scroll, AC11 inner-tab vs
//     document-tab persistence, AC13 cmd+F gate on data===null, AC17 viewport
//     re-render on scroll.
//   - Frontend reducer (useDocumentState extension): AC10/AC11/AC14 per-tab
//     findCaseSensitive.
//   - Project docs: project-context.md gains the Plain Text Find Bar Rules
//     section; deferred-work.md marks the 9-12 "Search within Plain Text
//     payload" entry as RESOLVED and records new deferrals.
//
// This Go harness pins structural invariants only: file existence, required
// exports, data-testid strings, Tailwind token registration, project-context
// section heading, deferred-work bookkeeping. Behavioral assertions live in
// the colocated Vitest suites. Mirrors the convention established by
// tests/10-1-async-plain-text-load/.
//
// Run: cd tests/10-2-plain-text-find-bar && go test -v -count=1 ./...
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
// AC#4, AC#12, AC#19 -- findMatches pure-function module exists with the
// documented exports. Behavioral tests live in findMatches.test.ts.
// ---------------------------------------------------------------------------

// 10-2-STRUCT-001 [P0] Task 2.1: frontend/src/lib/findMatches.ts exists.
func Test_10_2_STRUCT_001_FindMatchesModuleExists(t *testing.T) {
	if !fileExists(t, "frontend/src/lib/findMatches.ts") {
		t.Fatalf("[P0] 10-2-STRUCT-001: frontend/src/lib/findMatches.ts must exist (Task 2.1)")
	}
}

// 10-2-STRUCT-002 [P0] Task 2.1, AC#4: findMatches.ts exports findMatches,
// buildLineStartOffsets, and the Match interface. The Match shape carries
// start, end, line per AC4.
func Test_10_2_STRUCT_002_FindMatchesExports(t *testing.T) {
	src := readSource(t, "frontend/src/lib/findMatches.ts")
	required := []string{
		"export function findMatches",
		"export function buildLineStartOffsets",
		"export interface Match",
	}
	for _, sym := range required {
		if !strings.Contains(src, sym) {
			t.Errorf("[P0] 10-2-STRUCT-002: findMatches.ts must contain %q (Task 2.1)", sym)
		}
	}
	// Match shape from AC4: { start, end, line }.
	for _, field := range []string{"start", "end", "line"} {
		if !strings.Contains(src, field) {
			t.Errorf("[P0] 10-2-STRUCT-002: findMatches.ts Match interface must declare %q field (AC4)", field)
		}
	}
}

// 10-2-STRUCT-003 [P0] Task 2.3: co-located findMatches.test.ts exists.
func Test_10_2_STRUCT_003_FindMatchesTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/lib/findMatches.test.ts") {
		t.Fatalf("[P0] 10-2-STRUCT-003: frontend/src/lib/findMatches.test.ts must exist (Task 2.3)")
	}
}

// ---------------------------------------------------------------------------
// AC#1..AC#3, AC#7..AC#11, AC#13, AC#15, AC#22 -- useFindBar hook surface.
// ---------------------------------------------------------------------------

// 10-2-STRUCT-010 [P0] Task 3.1: frontend/src/hooks/useFindBar.ts exists.
func Test_10_2_STRUCT_010_UseFindBarHookExists(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useFindBar.ts") {
		t.Fatalf("[P0] 10-2-STRUCT-010: frontend/src/hooks/useFindBar.ts must exist (Task 3.1)")
	}
}

// 10-2-STRUCT-011 [P0] Task 3.1: useFindBar.ts exports useFindBar.
func Test_10_2_STRUCT_011_UseFindBarExport(t *testing.T) {
	src := readSource(t, "frontend/src/hooks/useFindBar.ts")
	if !strings.Contains(src, "export function useFindBar") &&
		!strings.Contains(src, "export const useFindBar") {
		t.Errorf("[P0] 10-2-STRUCT-011: useFindBar.ts must export useFindBar (Task 3.1)")
	}
}

// 10-2-STRUCT-012 [P0] Task 3.7: co-located useFindBar.test.ts exists.
func Test_10_2_STRUCT_012_UseFindBarTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useFindBar.test.ts") {
		t.Fatalf("[P0] 10-2-STRUCT-012: frontend/src/hooks/useFindBar.test.ts must exist (Task 3.7)")
	}
}

// ---------------------------------------------------------------------------
// AC#1, AC#5, AC#6, AC#10, AC#12, AC#16, AC#18, AC#20, AC#21 -- FindBar
// component surface.
// ---------------------------------------------------------------------------

// 10-2-STRUCT-020 [P0] Task 4.1: frontend/src/components/FindBar.tsx exists.
func Test_10_2_STRUCT_020_FindBarComponentExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/FindBar.tsx") {
		t.Fatalf("[P0] 10-2-STRUCT-020: frontend/src/components/FindBar.tsx must exist (Task 4.1)")
	}
}

// 10-2-STRUCT-021 [P0] Task 4.1: FindBar.tsx exports FindBar.
func Test_10_2_STRUCT_021_FindBarExport(t *testing.T) {
	src := readSource(t, "frontend/src/components/FindBar.tsx")
	if !strings.Contains(src, "export function FindBar") &&
		!strings.Contains(src, "export const FindBar") {
		t.Errorf("[P0] 10-2-STRUCT-021: FindBar.tsx must export FindBar (Task 4.1)")
	}
}

// 10-2-STRUCT-022 [P0] Task 4.5: co-located FindBar.test.tsx exists.
func Test_10_2_STRUCT_022_FindBarTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/FindBar.test.tsx") {
		t.Fatalf("[P0] 10-2-STRUCT-022: frontend/src/components/FindBar.test.tsx must exist (Task 4.5)")
	}
}

// 10-2-STRUCT-023 [P0] AC#1: FindBar.tsx renders the documented data-testids
// for the static structure (input, count, case toggle, prev, next, close).
// Conditional testids (wrap-status, non-Latin-1 hint, gutter marker) are
// asserted in their dedicated component / integration tests.
func Test_10_2_STRUCT_023_FindBarTestIds(t *testing.T) {
	src := readSource(t, "frontend/src/components/FindBar.tsx")
	requiredTestIds := []string{
		"plain-text-find-bar",          // AC1 root
		"plain-text-find-input",        // AC1 input
		"plain-text-find-count",        // AC1 count
		"plain-text-find-case-toggle",  // AC1 case toggle
		"plain-text-find-prev",         // AC1 prev
		"plain-text-find-next",         // AC1 next
		"plain-text-find-close",        // AC1 close
		"plain-text-find-wrap-status",  // AC7 / AC8 wrap status
	}
	for _, tid := range requiredTestIds {
		if !strings.Contains(src, tid) {
			t.Errorf("[P0] 10-2-STRUCT-023: FindBar.tsx missing data-testid=%q (AC1 / AC7 / AC8)", tid)
		}
	}
}

// 10-2-STRUCT-024 [P0] AC#1, AC#21: FindBar.tsx wires the documented aria
// attributes (role=search, aria-label="Find in plain text", aria-live="polite"
// on count + wrap-status, aria-pressed on case toggle).
func Test_10_2_STRUCT_024_FindBarAriaContract(t *testing.T) {
	src := readSource(t, "frontend/src/components/FindBar.tsx")
	requiredAria := []string{
		`role="search"`,                  // AC1 root role
		`Find in plain text`,             // AC1 aria-label root
		`Find query`,                     // AC1 aria-label input
		`Match case`,                     // AC1 aria-label case toggle
		`Previous match`,                 // AC1 aria-label prev
		`Next match`,                     // AC1 aria-label next
		`Close find`,                     // AC1 aria-label close
		`aria-live="polite"`,             // AC21 count + wrap-status live region
		`aria-pressed`,                   // AC1 / AC10 case toggle pressed state
	}
	for _, frag := range requiredAria {
		if !strings.Contains(src, frag) {
			t.Errorf("[P0] 10-2-STRUCT-024: FindBar.tsx must contain %q (AC1 / AC21)", frag)
		}
	}
}

// 10-2-STRUCT-025 [P0] AC#12: FindBar.tsx wires the non-Latin-1 hint testid
// and the documented copy. The aria-describedby linkage from the input to
// the hint id is part of the same contract.
func Test_10_2_STRUCT_025_FindBarNonLatin1Hint(t *testing.T) {
	src := readSource(t, "frontend/src/components/FindBar.tsx")
	requiredFragments := []string{
		"plain-text-find-non-latin1-hint",            // AC12 testid + id
		`Non-Latin-1 characters won't match`,         // AC12 exact copy
		`aria-describedby`,                           // AC12 linkage
	}
	for _, frag := range requiredFragments {
		if !strings.Contains(src, frag) {
			t.Errorf("[P0] 10-2-STRUCT-025: FindBar.tsx must contain %q (AC12)", frag)
		}
	}
}

// ---------------------------------------------------------------------------
// AC#5, AC#6, AC#7, AC#11, AC#13, AC#17 -- PlainTextView integration.
// ---------------------------------------------------------------------------

// 10-2-STRUCT-030 [P0] Task 5.1: PlainTextView.tsx imports useFindBar and
// FindBar.
func Test_10_2_STRUCT_030_PlainTextViewImportsFindBar(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	required := []string{
		"useFindBar",
		"FindBar",
	}
	for _, sym := range required {
		if !strings.Contains(src, sym) {
			t.Errorf("[P0] 10-2-STRUCT-030: PlainTextView.tsx must reference %q (Task 5.1)", sym)
		}
	}
}

// 10-2-STRUCT-031 [P0] AC#5: PlainTextView.tsx wires the active-match and
// non-active-match testids for the per-row <mark> overlay (Task 5.3).
func Test_10_2_STRUCT_031_PlainTextViewMarkTestIds(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	required := []string{
		"plain-text-find-active-match",  // AC5 active match
		"plain-text-find-match",         // AC5 inactive match
	}
	for _, tid := range required {
		if !strings.Contains(src, tid) {
			t.Errorf("[P0] 10-2-STRUCT-031: PlainTextView.tsx must reference data-testid=%q (AC5 / Task 5.3)", tid)
		}
	}
}

// 10-2-STRUCT-032 [P0] AC#6: PlainTextView.tsx wires the gutter density
// marker testid prefix (Task 5.4). The {lineNo} suffix is asserted in the
// integration Vitest.
func Test_10_2_STRUCT_032_PlainTextViewGutterMarker(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	if !strings.Contains(src, "plain-text-find-gutter-marker") {
		t.Errorf("[P0] 10-2-STRUCT-032: PlainTextView.tsx must reference data-testid=plain-text-find-gutter-marker-{lineNo} (AC6 / Task 5.4)")
	}
}

// 10-2-STRUCT-033 [P0] Task 5.6: PlainTextView.find.test.tsx exists for the
// integration assertions (per-row <mark> + gutter marker + auto-scroll +
// inner-tab persistence + tabId reset + Esc scope + Cmd+F gate on data===null).
func Test_10_2_STRUCT_033_PlainTextFindTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/components/PlainTextView.find.test.tsx") {
		t.Fatalf("[P0] 10-2-STRUCT-033: frontend/src/components/PlainTextView.find.test.tsx must exist (Task 5.6)")
	}
}

// ---------------------------------------------------------------------------
// AC#10, AC#11, AC#14 -- TabState.findCaseSensitive + SET_FIND_CASE_SENSITIVE.
// ---------------------------------------------------------------------------

// 10-2-STRUCT-040 [P0] Task 1.1: TabState declares findCaseSensitive: boolean.
func Test_10_2_STRUCT_040_TabStateCarriesFindCaseSensitive(t *testing.T) {
	src := readSource(t, "frontend/src/hooks/useDocumentState.tsx")
	if !strings.Contains(src, "findCaseSensitive") {
		t.Fatalf("[P0] 10-2-STRUCT-040: useDocumentState.tsx must declare findCaseSensitive on TabState (Task 1.1 / AC10)")
	}
	// Pinned type: boolean. Catches accidental migration to a tri-state.
	if !strings.Contains(src, "findCaseSensitive: boolean") {
		t.Errorf("[P0] 10-2-STRUCT-040: TabState.findCaseSensitive must be typed boolean (Task 1.1)")
	}
}

// 10-2-STRUCT-041 [P0] Task 1.2: AppAction union declares
// SET_FIND_CASE_SENSITIVE with payload { tabId, value }.
func Test_10_2_STRUCT_041_SetFindCaseSensitiveAction(t *testing.T) {
	src := readSource(t, "frontend/src/hooks/useDocumentState.tsx")
	if !strings.Contains(src, "SET_FIND_CASE_SENSITIVE") {
		t.Fatalf("[P0] 10-2-STRUCT-041: useDocumentState.tsx must declare the SET_FIND_CASE_SENSITIVE action (Task 1.2 / AC10)")
	}
	// Payload contract carries tabId + value (boolean).
	if !strings.Contains(src, "tabId") || !strings.Contains(src, "value") {
		t.Errorf("[P0] 10-2-STRUCT-041: SET_FIND_CASE_SENSITIVE payload must contain { tabId, value } (Task 1.2)")
	}
}

// 10-2-STRUCT-042 [P0] Task 1.3: useDocumentState.find.test.tsx exists with
// reducer coverage for SET_FIND_CASE_SENSITIVE + OPEN_DOCUMENT defaulting +
// CLOSE_DOCUMENT cleanup.
func Test_10_2_STRUCT_042_ReducerFindTestExists(t *testing.T) {
	if !fileExists(t, "frontend/src/hooks/useDocumentState.find.test.tsx") {
		t.Fatalf("[P0] 10-2-STRUCT-042: frontend/src/hooks/useDocumentState.find.test.tsx must exist (Task 1.3)")
	}
}

// ---------------------------------------------------------------------------
// AC#5, AC#6 -- Tailwind tokens registered in style.css.
// ---------------------------------------------------------------------------

// 10-2-STRUCT-050 [P0] Task 6.1: style.css registers the five find-color CSS
// custom properties and exposes them via @theme inline. The exact RGB values
// are not pinned (designer-tunable); we pin only the variable names and the
// @theme registration.
func Test_10_2_STRUCT_050_FindColorTokens(t *testing.T) {
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
			t.Errorf("[P0] 10-2-STRUCT-050: style.css must declare %q (Task 6.1 / AC5 / AC6)", v)
		}
	}
	if !strings.Contains(src, "@theme inline") {
		t.Errorf("[P0] 10-2-STRUCT-050: style.css must keep the @theme inline block for color token registration (Task 6.1)")
	}
}

// ---------------------------------------------------------------------------
// Out-of-scope guards: no Cmd+G rebinding (AC#9 -- F3 only).
// ---------------------------------------------------------------------------

// 10-2-STRUCT-060 [P0] AC#9: useFindBar.ts must NOT bind Cmd+G / Ctrl+G for
// Find-Next. App.jsx owns that combo for Open Go to Page (Story 9-4). The
// only Find-Next keystroke is F3 / Shift+F3.
func Test_10_2_STRUCT_060_NoCmdGRebinding(t *testing.T) {
	src := readSource(t, "frontend/src/hooks/useFindBar.ts")
	// Use a literal lowercase-and-uppercase form check; either is a regression.
	for _, forbidden := range []string{`e.key === 'g'`, `e.key === 'G'`, `key: 'g'`, `key: 'G'`} {
		if strings.Contains(src, forbidden) {
			t.Errorf("[P0] 10-2-STRUCT-060: useFindBar.ts must NOT bind %q (AC9: Cmd+G is owned by App.jsx for Open Go to Page; Find-Next is F3 only)", forbidden)
		}
	}
	// F3 binding must be present.
	if !strings.Contains(src, "F3") {
		t.Errorf("[P0] 10-2-STRUCT-060: useFindBar.ts must bind F3 for Find-Next (AC9)")
	}
}

// ---------------------------------------------------------------------------
// AC contract: documentation surfaces -- project-context.md section + deferred-work.md.
// ---------------------------------------------------------------------------

// 10-2-STRUCT-070 [P0] Task 8.1: _bmad-output/project-context.md gains a
// section heading for the Plain Text Find Bar Rules. The docs symlink points
// to ../docs/_bmad-output -- we read through the project-relative path so the
// symlink is exercised end-to-end.
func Test_10_2_STRUCT_070_ProjectContextSection(t *testing.T) {
	src := readSource(t, "_bmad-output/project-context.md")
	if !strings.Contains(src, "Plain Text Find Bar Rules") {
		t.Errorf("[P0] 10-2-STRUCT-070: project-context.md must add a 'Plain Text Find Bar Rules' section (Task 8.1)")
	}
	// Latin-1 invariant cross-reference must be present so the codepoint > 0xFF
	// rule (AC12) is documented next to the corpus backend invariant.
	if !strings.Contains(src, "latin1Decode") {
		t.Errorf("[P0] 10-2-STRUCT-070: Plain Text Find Bar Rules section must reference internal/pdfcore/plaintext.go latin1Decode (AC12 docs trail)")
	}
}

// 10-2-STRUCT-071 [P0] Task 8.2: _bmad-output/implementation-artifacts/deferred-work.md
// marks the 9-12 "Search within Plain Text payload" entry as RESOLVED by 10-2
// and adds the new deferral entries (regex, copy-with-context, find across
// Object / XREF tabs, find non-Latin-1 query via transcoding).
func Test_10_2_STRUCT_071_DeferredWorkUpdated(t *testing.T) {
	src := readSource(t, "_bmad-output/implementation-artifacts/deferred-work.md")
	if !strings.Contains(src, "Search within Plain Text payload") {
		t.Errorf("[P0] 10-2-STRUCT-071: deferred-work.md must keep the 9-12 'Search within Plain Text payload' entry (Task 8.2)")
	}
	if !strings.Contains(src, "10-2") {
		t.Errorf("[P0] 10-2-STRUCT-071: deferred-work.md must reference Story 10-2 in the search-related entries (Task 8.2)")
	}
	if !strings.Contains(src, "RESOLVED") {
		t.Errorf("[P0] 10-2-STRUCT-071: deferred-work.md must mark the search entry RESOLVED by Story 10-2 (Task 8.2)")
	}
}
