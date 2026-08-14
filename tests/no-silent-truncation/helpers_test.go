// Acceptance harness for the "no silent truncation" rule
// across two machine-contract surfaces: `diff` (depth-cap under-report) and
// `dump stream` (multi-stream /Contents).
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess against
// the committed correctness-corpus fixtures deep-change-{a,b}.pdf and
// multi-content-stream.pdf. Failures surface at RUNTIME (a truncated diff
// reporting "identical" at exit 0; a multi-stream page showing only stream 1
// with no marker), NOT at compile time, so the main `unidoc-pdf-debugger`
// module keeps building green.
// This module has its own go.mod and is not part of the main build (mirrors
// tests/trustworthy-stream-op-output and tests/structural-diff).
//
// Test pyramid: every case here is a Go integration-level black-box test against
// the built CLI binary -- the project's established acceptance level for the CLI
// machine contract (10-x, 13-1..13-6, 14-1). The diff depth-cap count (the intent)
// is asserted through the `diff --json` summary.truncatedSubtrees field rather than
// a co-located internal/pdfcore unit test: that field/`DiffNode.Truncated` do not
// exist yet, so a co-located test would break the main module's compile (and `go
// vet`/gate), violating the runtime-red convention. The thin GUI display branch is
// covered at the component (Vitest) level in
// frontend/src/components/DiffView.truncation.test.tsx.
//
// Naming: [Px] per the story Testing Requirements.
//
// Run: cd tests/no-silent-truncation && go test -v -count=1 ./...
package no_silent_truncation_test

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

// --- JSON helpers ------------------------------------------------------------

// parseObject parses stdout as a single top-level JSON object.
func parseObject(t *testing.T, stdout string) map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" || trimmed[0] != '{' {
		t.Fatalf("expected a top-level JSON object, got:\n%s", stdout)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw: %s", err, stdout)
	}
	return res
}

// jsonInt coerces a JSON number (float64) to int; non-numbers yield 0.
func jsonInt(v any) int {
	f, _ := v.(float64)
	return int(f)
}

// anyNodeTruncated reports whether the DiffNode tree rooted at node carries a
// node with "truncated": true anywhere. This is the depth-cap marker the
// implementation adds to DiffNode; today the field is absent, so this is false.
func anyNodeTruncated(node map[string]any) bool {
	if node == nil {
		return false
	}
	if b, ok := node["truncated"].(bool); ok && b {
		return true
	}
	children, ok := node["children"].([]any)
	if !ok {
		return false
	}
	for _, c := range children {
		if cm, ok := c.(map[string]any); ok && anyNodeTruncated(cm) {
			return true
		}
	}
	return false
}

// formattedOperators extracts the operator strings from a ContentStreamData
// `formatted` array (the `dump stream --json` shape). Empty-operator lines
// (comments / dangling operand runs) are skipped.
func formattedOperators(res map[string]any) []string {
	var ops []string
	formatted, ok := res["formatted"].([]any)
	if !ok {
		return ops
	}
	for _, fl := range formatted {
		flm, ok := fl.(map[string]any)
		if !ok {
			continue
		}
		if op, ok := flm["operator"].(string); ok && op != "" {
			ops = append(ops, op)
		}
	}
	return ops
}

// contains reports whether s is in xs.
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
