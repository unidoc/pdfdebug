// A file rewritten after the CLI parsed it is reported instead of mixing the
// old parse with the new bytes: dump bytes (raw and --json) exits 2 with a
// stderr JSON error; dump signatures keeps the parsed details and prints the
// hole check as not checked. afterCLIOpen runs right after openForCLI
// succeeds and rewrites the file.
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

func TestCLIFileChangedBytesRaw(t *testing.T) {
	path := signedCopy(t)
	called := rewriteAfterOpen(t)
	var code int
	var stdout, stderr string
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = execBytesDump(path, docViewFlags{})
		})
	})
	if !*called {
		t.Fatalf("afterCLIOpen was not called by dump bytes")
	}
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr, "file changed on disk") {
		t.Errorf("stderr %q must contain %q", stderr, "file changed on disk")
	}
	if stdout != "" {
		t.Errorf("no bytes of the rewritten file may reach stdout, got %d bytes", len(stdout))
	}
}

func TestCLIFileChangedSignatures(t *testing.T) {
	path := signedCopy(t)
	called := rewriteAfterOpen(t)
	var code int
	stdout := captureStdout(t, func() {
		captureStderr(t, func() {
			code = execSignaturesDump(path, docViewFlags{})
		})
	})
	if !*called {
		t.Fatalf("afterCLIOpen was not called by dump signatures")
	}
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	if !strings.Contains(stdout, "not checked: file changed on disk") {
		t.Errorf("stdout must note the hole as not checked, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "ByteRange") {
		t.Errorf("stdout must keep the parsed signature details, got:\n%s", stdout)
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
