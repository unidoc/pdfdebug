// A file rewritten after the CLI parsed it is reported as a view error
// (stderr JSON, exit 2) instead of mixing the old parse with the new bytes.
// afterCLIOpen runs right after openForCLI succeeds and rewrites the file.
//
// Run: cd code && go test -run TestCLIFileChanged ./cmd/cli/

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// signedCopy copies testdata/signed.pdf into a temp dir.
func signedCopy(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "signed.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "signed.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// rewriteAfterOpen installs an afterCLIOpen hook that overwrites the opened
// file with minimal.pdf and moves its mtime ahead.
func rewriteAfterOpen(t *testing.T) *bool {
	t.Helper()
	called := new(bool)
	afterCLIOpen = func(path string) {
		*called = true
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "minimal.pdf"))
		if err != nil {
			t.Errorf("read minimal.pdf: %v", err)
			return
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Errorf("rewrite: %v", err)
		}
		later := time.Now().Add(time.Hour)
		_ = os.Chtimes(path, later, later)
	}
	t.Cleanup(func() { afterCLIOpen = nil })
	return called
}

func TestCLIFileChangedBytesJSON(t *testing.T) {
	path := signedCopy(t)
	called := rewriteAfterOpen(t)
	var code int
	var stderr string
	captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = execBytesDump(path, docViewFlags{json: true})
		})
	})
	if !*called {
		t.Fatalf("afterCLIOpen was not called by dump bytes --json")
	}
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr, "file changed on disk") {
		t.Errorf("stderr %q must contain %q", stderr, "file changed on disk")
	}
}

func TestCLIFileChangedSignatures(t *testing.T) {
	path := signedCopy(t)
	called := rewriteAfterOpen(t)
	var code int
	var stderr string
	captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = execSignaturesDump(path, docViewFlags{})
		})
	})
	if !*called {
		t.Fatalf("afterCLIOpen was not called by dump signatures")
	}
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr, "file changed on disk") {
		t.Errorf("stderr %q must contain %q", stderr, "file changed on disk")
	}
}

func TestCLIFileChangedHookUnsetLeavesRunsUnaffected(t *testing.T) {
	path := signedCopy(t)
	if afterCLIOpen != nil {
		t.Fatalf("afterCLIOpen must be nil outside tests")
	}
	var code int
	var stderr string
	captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = execSignaturesDump(path, docViewFlags{json: true})
		})
	})
	if code != 0 || stderr != "" {
		t.Errorf("unchanged file: exit %d stderr %q, want 0 and nothing", code, stderr)
	}
}
