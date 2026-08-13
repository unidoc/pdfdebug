// Story 14-1 RED-PHASE acceptance harness for the machine-contract surface of
// `dump stream --json` / `--ops`.
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess against
// committed fixtures in testdata/correctness/. These tests assert the EXPECTED
// post-implementation contract behavior and MUST FAIL against the current
// binary (leading-sign operands mislabeled as operators; --ops emitting
// empty-operator records) until Story 14-1 is implemented. They fail at
// RUNTIME (wrong token type / phantom NDJSON record), not at compile time, so
// the main module keeps building green. This module has its own go.mod and is
// not part of the main build (mirrors tests/structural-diff).
//
// Naming: [Px] per the story Testing Requirements.
//
// Run: cd tests/trustworthy-stream-op-output && go test -v -count=1 ./...
package trustworthy_stream_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// projectRoot walks up from the test directory to find the main module's go.mod.
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

// fixturePath returns the absolute path to a committed correctness-corpus fixture.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata", "correctness", name)
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI binary once per test package and returns its path.
// Cached via sync.Once: the binary is identical for every test in the module.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		root := projectRoot(t)
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-cli-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(output)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	return cliBinPath
}

// runCLI executes the CLI binary with args and returns stdout, stderr, exit code.
func runCLI(t *testing.T, binPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}
