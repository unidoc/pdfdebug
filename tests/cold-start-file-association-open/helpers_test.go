// Package story_12_1_test holds the per-story acceptance suite for Story 12.1:
// Cold-Start File Association Open.
//
// Cross-module mechanics: the production logic under test lives in
// unidoc-pdf-debugger/internal/pendingopen and internal/pdfservice. Go forbids
// importing an `internal/...` package from a standalone test module (the import
// path does not share the `unidoc-pdf-debugger/` prefix), so these tests CANNOT
// import the packages directly. Instead each test writes a small harness test
// package into a throwaway dot-prefixed directory INSIDE the main module
// (`.atdd-12-1-*`) and runs `go test -race` on it as a subprocess. A harness
// package rooted inside the main module is allowed to import the internal
// packages, and dot-prefixed dirs are invisible to `go test ./...`, so a leaked
// dir can never break the main sweep. The subprocess result is the assertion:
// the harness compiling + passing == the production contract holding.
//
// This keeps the suite BEHAVIOR-ONLY: zero source greps (the strict
// source-grep guard forbids new grep tests), zero doc-content assertions.
//
// Red phase: every harness fails to BUILD today because
// internal/pendingopen does not exist yet (and the pdfservice / main-package
// symbols are not implemented). Dev's job turns each red harness green.
//
// Run: cd tests/12-1-cold-start-file-association-open && go test -count=1 ./...
package story_12_1_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot walks up from cwd until it finds the project go.mod (module
// unidoc-pdf-debugger). Mirrors the sibling acceptance suites.
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

// runHarness writes pkgFiles into a fresh dot-prefixed directory inside the
// main module and runs `go test -race -count=1` on it. The map key is the
// file name (e.g. "harness_test.go"); the value is the Go source. The harness
// package name is the caller's choice inside the source.
//
// It returns combined stdout+stderr and the run error. On a clean compile +
// pass, err is nil. A build failure or a test failure both surface as a
// non-nil err with the tool output in `out` for diagnosis.
//
// The directory is removed on cleanup regardless of outcome.
func runHarness(t *testing.T, pkgFiles map[string]string) (out string, err error) {
	t.Helper()
	root := projectRoot(t)

	// Dot prefix keeps the dir out of `go test ./...` and `go build ./...`.
	dir, mkErr := os.MkdirTemp(root, ".atdd-12-1-")
	if mkErr != nil {
		t.Fatalf("mkdir harness: %v", mkErr)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	for name, src := range pkgFiles {
		if writeErr := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); writeErr != nil {
			t.Fatalf("write harness file %s: %v", name, writeErr)
		}
	}

	// Run from the harness dir so it resolves the main module's go.mod.
	cmd := exec.Command("go", "test", "-race", "-count=1", ".")
	cmd.Dir = dir
	raw, runErr := cmd.CombinedOutput()
	return string(raw), runErr
}

// runMainPackageTest runs a single named test in the MAIN package (root
// directory of the module) via `go test -run`. Used to delegate the AC7
// routing-helper verdict to the main-package table test (Task 2.4), which is
// the only sanctioned automated pin on the main.go wiring (source-grep tests
// are forbidden). Returns combined output and the run error.
func runMainPackageTest(t *testing.T, runRegexp string) (out string, err error) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-run", runRegexp, "-count=1", ".")
	cmd.Dir = root
	raw, runErr := cmd.CombinedOutput()
	return string(raw), runErr
}
