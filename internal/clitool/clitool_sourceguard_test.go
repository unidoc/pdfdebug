// Source-guard test for Story 11.2 AC #5(b): the install path's security
// guarantee is "no shell exists" -- which cannot be asserted by inspecting a
// (non-existent) shell string. So this test asserts, at AST + content level,
// that the production source of package clitool imports neither `os/exec` nor
// references the `osascript` / `brew --prefix` / `sh -c` escalation idioms.
//
// This is a TDD RED-PHASE test: it scans the package's own non-test .go files.
// Until the Dev step creates package clitool's production source, the package
// fails to compile (the rest of the suite references undefined symbols), which
// is the red state. Once implemented, this guard stays green only if the
// implementation honors the no-shell / no-root design.
//
// Scope note: only NON-test files are scanned. Test files legitimately mention
// these tokens (e.g. this file names them as forbidden patterns); scanning
// tests would create a self-trip. The forbidden tokens below are assembled from
// fragments at runtime so this guard file's own bytes never contain the literal
// patterns it forbids.
//
// Run: cd internal/clitool && go test -count=1 -run SourceGuard ./...
package clitool

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// productionGoFiles returns the package's non-test .go files in the current
// (package) directory.
func productionGoFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}
	if len(files) == 0 {
		t.Fatalf("no production .go files found in package clitool: the install package must exist with non-test source to scan")
	}
	return files
}

// TestSourceGuardNoOsExecImport asserts no production file imports os/exec.
// (AC #5b -- the no-shell guarantee forbids spawning processes for the install
// path.)
func TestSourceGuardNoOsExecImport(t *testing.T) {
	forbidden := "os" + "/" + "exec"
	fset := token.NewFileSet()
	for _, f := range productionGoFiles(t) {
		file, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == forbidden {
				t.Errorf("%s imports %q -- the install path must not spawn processes (\"no shell exists\")", f, forbidden)
			}
		}
	}
}

// TestSourceGuardNoShellEscalationTokens asserts no production file contains the
// shell-escalation idioms. Tokens are assembled from fragments so this guard
// file's own source never carries the literal patterns it forbids.
// (AC #5b -- no `osascript ... with administrator privileges`, no `brew --prefix`
// exec, no `sh -c`.)
func TestSourceGuardNoShellEscalationTokens(t *testing.T) {
	forbidden := []string{
		"osa" + "script",
		"brew " + "--prefix",
		"sh " + "-c",
		"with admin" + "istrator privileges",
		"do shell " + "script",
	}
	for _, f := range productionGoFiles(t) {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(data)
		for _, tok := range forbidden {
			if strings.Contains(src, tok) {
				t.Errorf("%s contains forbidden shell-escalation token %q (no root escalation, no shell)", f, tok)
			}
		}
	}
}

// pkgDirSanity guards the helper itself: the package directory must resolve and
// contain at least one .go production file path that is relative (cwd-anchored),
// matching the go test working-directory convention.
func TestSourceGuardScansPackageDir(t *testing.T) {
	files := productionGoFiles(t)
	for _, f := range files {
		if filepath.IsAbs(f) {
			t.Errorf("productionGoFiles returned an absolute path %q; expected cwd-relative names", f)
		}
	}
}
