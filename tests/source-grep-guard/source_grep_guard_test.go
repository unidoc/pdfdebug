// Package source_grep_guard_test enforces the anti-recurrence clause
// from Story 4-5 ("Replace Source-Grep Tests with Behavioral Coverage").
//
// The rule: future contributors MUST NOT add NEW `strings.Contains` /
// `regexp.Match` / file-content assertions in tests under `tests/` that read
// `main.go`, `frontend/src/components/MainLayout.tsx`, or
// `frontend/src/components/EmptyState.tsx`. Pre-existing structural greps are
// grandfathered; this test snapshots them as an allowlist and fails when a NEW
// occurrence appears outside the allowlist.
//
// Detection: a Test* function under `tests/**/*_test.go` is a "source-grep" of
// a guarded file if its body contains BOTH a string literal whose path
// basename equals one of the guarded files AND a call to a content-reading
// function (`readFile`, `os.ReadFile`, `ReadFile`). Pure existence checks
// (e.g. `fileExists(t, "main.go")`) are NOT source-greps and are NOT flagged;
// the rule forbids file-content assertions specifically.
//
// Maintenance: when a grandfathered test is deleted or refactored to no longer
// source-grep, remove its entry from `grandfatheredAllowlist`. The test fails
// if the allowlist contains a stale entry, so pruning is mechanical.
//
// This test partially fulfills the deferred CI-lint follow-up tracked in
// docs/_bmad-output/implementation-artifacts/deferred-work.md under
// "Deferred from: story 4-5".
//
// Run: cd tests/source-grep-guard && go test -count=1 ./...
package source_grep_guard_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// guardedBasenames are the production source files future tests must not grep.
var guardedBasenames = map[string]bool{
	"main.go":         true,
	"MainLayout.tsx":  true,
	"EmptyState.tsx":  true,
}

// readFunctionNames are the content-reading helpers / stdlib calls that mark
// a function as a content-grep when paired with a guarded path literal.
// `readFile` is the per-suite helper convention; `ReadFile` matches both
// `os.ReadFile` (selector) and bare `ReadFile` (rare).
var readFunctionNames = map[string]bool{
	"readFile": true,
	"ReadFile": true,
}

// grandfatheredAllowlist tracks the small set of source-grep tests the rule
// admits as legitimate. Two narrow categories qualify:
//
//  1. Structural guarantees about main.go's window-creation ORDER (e.g.
//     the splash must precede the main window). This is a property of
//     the production-source layout, not of runtime behavior, and is not
//     reproducible via Vitest, Playwright, or boot-smoke because the
//     splash WebView is a frameless OS-native window outside Playwright's
//     reach. See story 9-13.
//
//  2. Structural regression guards on reentrant callbacks (e.g. the
//     splash MUST NOT be created inside OnSecondInstanceLaunch). Same
//     justification: behavioral tests cannot reach a Wails callback
//     body's lexical contents.
//
// New entries require a story-spec justification. If a behavioral
// alternative is feasible, prefer it.
//
// Story 9-2 (2026-05-07) deleted every pre-existing source-grep test from
// the suite. Story 9-13 (2026-05-20) added the two splash entries below.
var grandfatheredAllowlist = []string{
	// Story 9-13 -- splash window must be created before the main
	// WebviewWindow so the user sees branding during WebView2 cold init.
	"tests/startup-splash-screen/startup_splash_screen_test.go::TestSplashWindowCreatedBeforeMainWindow",
	// Story 9-13 -- structural regression guard: splash creation
	// must NOT appear inside OnSecondInstanceLaunch or
	// ApplicationOpenedWithFile callback bodies.
	"tests/startup-splash-screen/startup_splash_screen_test.go::TestSplashNotCreatedInsideSecondInstanceCallback",
}

// projectRoot walks upward from the test working directory to find the repo
// root identified by go.mod's `module unidoc-pdf-debugger`.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(data), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

// findOffenders walks tests/**/*_test.go via Go AST and returns the set of
// "<relpath>::<TestFunc>" identifiers whose body source-greps a guarded file.
func findOffenders(t *testing.T, root string) []string {
	t.Helper()
	testsDir := filepath.Join(root, "tests")
	var offenders []string
	err := filepath.WalkDir(testsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		// Normalise to forward slashes so allowlist entries are portable
		// across Linux/macOS/Windows runners.
		rel = filepath.ToSlash(rel)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil {
				continue
			}
			if !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			if functionSourceGreps(fn) {
				offenders = append(offenders, rel+"::"+fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", testsDir, err)
	}
	sort.Strings(offenders)
	return offenders
}

// functionSourceGreps returns true if fn's body contains BOTH a string
// literal whose path basename is a guarded production source file AND a
// call to a content-reading function. Pure existence checks (no read call)
// are not flagged.
func functionSourceGreps(fn *ast.FuncDecl) bool {
	hasGuardedLiteral := false
	hasReadCall := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING {
				// Strip the surrounding quotes; v.Value preserves them.
				lit := strings.Trim(v.Value, "`\"")
				base := lit
				if i := strings.LastIndex(lit, "/"); i >= 0 {
					base = lit[i+1:]
				}
				if guardedBasenames[base] {
					hasGuardedLiteral = true
				}
			}
		case *ast.CallExpr:
			switch fnExpr := v.Fun.(type) {
			case *ast.Ident:
				if readFunctionNames[fnExpr.Name] {
					hasReadCall = true
				}
			case *ast.SelectorExpr:
				if readFunctionNames[fnExpr.Sel.Name] {
					hasReadCall = true
				}
			}
		}
		return true
	})
	return hasGuardedLiteral && hasReadCall
}

// TestDetectorFlagsKnownPattern is a self-check: parses two synthetic Test
// functions in-memory and confirms the detector flags the source-grep one
// while leaving the existence-only one alone. Guards against silent
// false-negatives in functionSourceGreps.
func TestDetectorFlagsKnownPattern(t *testing.T) {
	src := `package x
import "testing"
func readFile(t *testing.T, p string) string { return "" }
func fileExists(t *testing.T, p string) bool { return true }
func TestGrepsMainGo(t *testing.T) {
	c := readFile(t, "main.go")
	_ = c
}
func TestExistenceOnly(t *testing.T) {
	if !fileExists(t, "main.go") { t.Fatal("missing") }
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	results := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		results[fn.Name.Name] = functionSourceGreps(fn)
	}
	if !results["TestGrepsMainGo"] {
		t.Error("detector failed to flag a function that calls readFile on main.go")
	}
	if results["TestExistenceOnly"] {
		t.Error("detector wrongly flagged a fileExists-only function (no content read)")
	}
}

// TestNoNewSourceGrepTests enforces the rule by scanning every test file under
// tests/ and asserting that no Test* function reads `main.go`,
// `MainLayout.tsx`, or `EmptyState.tsx` outside the grandfathered allowlist.
//
// Two failure modes:
//
//  1. NEW SOURCE-GREP: a function not in the allowlist matches the detection.
//     The contributor must convert it to a behavioural test (Vitest, Playwright,
//     or boot-smoke) per CONTRIBUTING.md "Tests Must Not Grep Production Source".
//  2. STALE ALLOWLIST ENTRY: an allowlist entry no longer matches (test was
//     deleted or refactored). Remove it from `grandfatheredAllowlist`.
func TestNoNewSourceGrepTests(t *testing.T) {
	root := projectRoot(t)
	offenders := findOffenders(t, root)

	allowed := make(map[string]bool, len(grandfatheredAllowlist))
	for _, k := range grandfatheredAllowlist {
		allowed[k] = true
	}
	seen := make(map[string]bool, len(offenders))
	for _, k := range offenders {
		seen[k] = true
	}

	var newGreps []string
	for _, k := range offenders {
		if !allowed[k] {
			newGreps = append(newGreps, k)
		}
	}
	var staleEntries []string
	for _, k := range grandfatheredAllowlist {
		if !seen[k] {
			staleEntries = append(staleEntries, k)
		}
	}

	if len(newGreps) > 0 {
		t.Errorf("%d NEW source-grep test(s) detected.\n"+
			"These tests read main.go / MainLayout.tsx / EmptyState.tsx with a content-reading helper.\n"+
			"Convert each to a behavioural test (Vitest, Playwright, or boot-smoke). See CONTRIBUTING.md.\n\n%s",
			len(newGreps), strings.Join(newGreps, "\n"))
	}
	if len(staleEntries) > 0 {
		t.Errorf("Stale grandfatheredAllowlist entries (test no longer source-greps; prune them): %d\n%s",
			len(staleEntries), strings.Join(staleEntries, "\n"))
	}
}
