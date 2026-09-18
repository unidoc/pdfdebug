// Package update_check_test is the acceptance suite for the in-app update
// notification feature.
//
// Cross-module mechanics: the logic under test lives in
// unidoc-pdf-debugger/internal/updatecheck. Go forbids importing an internal
// package from a standalone test module (the import path does not share the
// unidoc-pdf-debugger/ prefix), so this suite writes a harness package into a
// throwaway dot-prefixed directory INSIDE the main module and runs `go test` on
// it as a subprocess. A harness rooted in the main module may import the
// internal package, and dot-prefixed dirs are invisible to `go test ./...`. The
// subprocess passing is the assertion.
//
// Behavior-only: zero source greps, zero doc-content assertions.
//
// Run: cd tests/update-check && go test -count=1 ./...
package update_check_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot walks up from cwd to the main module go.mod.
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
			t.Fatalf("could not find project root")
		}
		dir = parent
	}
}

// runHarness writes pkgFiles into a fresh dot-prefixed dir inside the main
// module and runs `go test -count=1` on it, returning combined output and the
// run error. A clean compile + pass yields a nil error.
func runHarness(t *testing.T, pkgFiles map[string]string) (string, error) {
	t.Helper()
	root := projectRoot(t)

	dir, err := os.MkdirTemp(root, ".acc-update-check-")
	if err != nil {
		t.Fatalf("mkdir harness: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	for name, src := range pkgFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cmd := exec.Command("go", "test", "-count=1", ".")
	cmd.Dir = dir
	raw, runErr := cmd.CombinedOutput()
	return string(raw), runErr
}
