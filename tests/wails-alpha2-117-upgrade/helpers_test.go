// Package wails_alpha2_117_upgrade_test provides behavioral regression nets that
// stand across a Wails v3 bump + binding regen: a consumer-driven binding-presence
// contract and a wire-shape guard for the JSON struct-tag risk, plus the
// documented deferred human/hardware gates.
//
// Test pyramid for this suite (per the directive to favour API/integration over
// E2E, unit only where there is business logic):
//
//   - No new business logic to unit-test: these are structural / CLI-integration
//     acceptance checks in an independent module (mirrors
//     tests/wails-alpha2-103-upgrade/).
//   - The decisive coverage (the live bindings round-trip inside the platform
//     WebView, multi-WebView desktop smoke) is the native runtime layer. It needs
//     real macOS + Windows hardware and a human observer and is a DEFERRED
//     HUMAN/HARDWARE gate recorded in the Dev Agent Record (see
//     deferred_gates_test.go). Playwright cannot drive it; no red E2E is authored.
//
// This module is an independent go.mod (the project convention: no `replace`
// link into the main module, which would drag the whole Wails tree into a test
// module). It reads the repo's static files and exercises the CLI, the
// executable expression of the SAME internal/pdfcore model.go structs.
//
// Run: cd tests/wails-alpha2-117-upgrade && go test -v -count=1 ./...
package wails_alpha2_117_upgrade_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// projectRoot walks up from the working directory until it finds the project
// go.mod (module unidoc-pdf-debugger), and returns its absolute path.
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

// fileExists returns true when relPath exists under projectRoot.
func fileExists(t *testing.T, relPath string) bool {
	t.Helper()
	root := projectRoot(t)
	_, err := os.Stat(filepath.Join(root, relPath))
	return err == nil
}

// buildCLI compiles the pdfdebug CLI into a temp dir and returns its path. The
// CLI is the executable expression of the internal/pdfcore model.go structs, so
// its JSON output is the real-Marshal wire shape the wire-shape guard inspects
// without this module having to import (and thus replace-link) the main module.
func buildCLI(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	tmpDir := t.TempDir()
	binName := "pdfdebug"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmpDir, binName)
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build CLI binary: %s\n%s", err, output)
	}
	return binPath
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

// mustParseJSONObject parses s as a single JSON object, failing on error.
func mustParseJSONObject(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &m); err != nil {
		t.Fatalf("failed to parse JSON object: %v\nraw: %s", err, s)
	}
	return m
}
