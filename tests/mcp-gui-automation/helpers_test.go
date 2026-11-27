// Package mcp_gui_automation_test checks the agent build of the desktop app:
// the build:mcp target, the guard that keeps the mcp tag out of production
// builds, the instance identity of the agent build, the dialog stub, the MCP
// client and guictl, the removal of the browser test setup and the developer
// docs for GUI automation.
//
// The Go unit tests for the new code live next to it (internal/mcpclient,
// tools/guictl, internal/pdfservice and the main package). This suite runs
// them with the tags they need, so their result shows up here.
//
// Run: cd tests/mcp-gui-automation && go test -count=1 ./...
package mcp_gui_automation_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// projectRoot walks up from cwd to the directory holding the main go.mod.
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

// readRepoFile returns the content of a file given relative to the repo root.
func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// runInRoot runs a command from the repo root with extra environment entries
// and returns its combined output.
func runInRoot(t *testing.T, timeout time.Duration, env []string, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = projectRoot(t)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("%s %s did not finish within %v:\n%s", name, strings.Join(args, " "), timeout, out)
	}
	return string(out), err
}

// runGo runs the go tool from the repo root.
func runGo(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	return runInRoot(t, 10*time.Minute, env, "go", args...)
}

// requireTool skips the test when a command is not on PATH.
func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not on PATH", name)
	}
}
