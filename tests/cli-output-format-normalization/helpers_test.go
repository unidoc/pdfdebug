// Story 13-1 RED-PHASE acceptance test harness for CLI output format
// normalization: plain text is the default for every `dump` command, JSON is
// emitted only behind --json, --pretty applies only to JSON, --raw/--ops stay
// an orthogonal payload axis, and `dump page --info --json` carries an in-band
// "_stability":"experimental" marker.
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess. These
// tests assert the EXPECTED post-implementation behavior. They MUST FAIL
// against the current binary (which emits JSON by default and treats --json as
// a no-op on most commands) until Story 13-1 is implemented. They fail at
// RUNTIME (wrong output shape / wrong exit code), not at compile time, so the
// main `unidoc-pdf-debugger` module keeps building green (mirrors the Story
// 11-5 / 11-6 convention).
//
// Test pyramid: every case here is a Go integration-level black-box test
// against the built CLI binary -- the project's established level for CLI
// acceptance. No browser/E2E layer is warranted; Story 13-1 touches only
// cmd/cli output presentation and has zero UI surface.
//
// Naming: 13.1-INTG-NNN [Px] per the story Testing Requirements.
//
// Run: cd tests/cli-output-format-normalization && go test -v -count=1 ./...
package cli_output_format_normalization_test

import (
	"encoding/json"
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

// testdataDir returns the absolute path to the testdata/ directory at project root.
func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata")
}

// fixture returns the absolute path to a testdata fixture by relative name
// (e.g. "minimal.pdf" or "page-render/render-info.pdf").
func fixture(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(testdataDir(t), filepath.FromSlash(rel))
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI binary once per test package and returns its path.
// The build is cached via sync.Once: the binary is identical for every test in
// the module, so the expensive `go build` runs a single time instead of once
// per test. The build dir is a plain os.MkdirTemp (not t.TempDir) because it is
// shared across tests and must outlive any single one; the OS reclaims it.
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

// mustParseJSON parses s as a single JSON value into target, failing on error.
func mustParseJSON(t *testing.T, s string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), target); err != nil {
		t.Fatalf("failed to parse JSON: %v\nraw: %s", err, s)
	}
}

// parsesAsJSON reports whether s (trimmed) is a single well-formed JSON value.
// Used to assert the plain-text default is NOT JSON. We require both: it parses
// AND the first non-whitespace byte is a JSON structural opener so a bare plain
// word that happens to be a JSON literal (e.g. a number) is still treated as
// "not a JSON document" only via the structural-opener check in assertNotJSON.
func parsesAsJSON(s string) bool {
	var v any
	return json.Unmarshal([]byte(strings.TrimSpace(s)), &v) == nil
}

// assertNotJSON fails when out is a JSON object or array document. Plain-text
// output may incidentally contain braces, so this checks the document-level
// shape: a plain dump must NOT parse as a top-level JSON object/array. This is
// the structural "default is not JSON" guard (FORMAT-001), deliberately NOT a
// whole-dump string-equality assertion (brittle-struct-grep history).
func assertNotJSON(t *testing.T, out string) {
	t.Helper()
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return // empty output is trivially not a JSON document
	}
	if (trimmed[0] == '{' || trimmed[0] == '[') && parsesAsJSON(trimmed) {
		t.Fatalf("default output parsed as a JSON document; expected plain text:\n%s", out)
	}
}

// assertTrailingNewline fails when out does not end in a newline (AC2: plain
// text output ends with a trailing newline). Empty output is exempt.
func assertTrailingNewline(t *testing.T, out string) {
	t.Helper()
	if out == "" {
		return
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("plain-text output does not end with a trailing newline")
	}
}

// assertASCII fails when out contains a non-ASCII byte (AC2: ASCII-only).
func assertASCII(t *testing.T, out string) {
	t.Helper()
	for i := 0; i < len(out); i++ {
		if out[i] > 0x7f {
			t.Errorf("plain-text output contains non-ASCII byte 0x%02x at offset %d", out[i], i)
			return
		}
	}
}

// nonEmptyLines returns the output split into lines with empty/whitespace-only
// lines removed. Used for structural row/section counts.
func nonEmptyLines(out string) []string {
	var lines []string
	for _, ln := range strings.Split(out, "\n") {
		if strings.TrimSpace(ln) != "" {
			lines = append(lines, ln)
		}
	}
	return lines
}

// containsLineWith reports whether any line contains every one of the given
// substrings (order-independent within the line). Structural label presence
// check -- NOT whole-dump equality.
func containsLineWith(out string, subs ...string) bool {
	for _, ln := range strings.Split(out, "\n") {
		all := true
		for _, s := range subs {
			if !strings.Contains(ln, s) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// leadingSpaces returns the count of leading space characters on a line (indent
// width). Used to assert two-space indent shape on hierarchical presenters.
func leadingSpaces(line string) int {
	n := 0
	for _, r := range line {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}
