package mcp_gui_automation_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// runGateScript runs scripts/test-all.sh with args and a short deadline. Every
// call here starts with --no-gui and ends with an unknown flag, so the script
// stops at argument parsing whether or not it knows --no-gui.
func runGateScript(t *testing.T, args ...string) (string, int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("scripts/test-all.sh runs under bash on macOS and Linux")
	}
	requireTool(t, "bash")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{filepath.Join(projectRoot(t), "scripts", "test-all.sh")}, args...)...)
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("scripts/test-all.sh %s ran past argument parsing (still running after 30s):\n%s", strings.Join(args, " "), out)
	}
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run scripts/test-all.sh: %v", err)
	}
	return string(out), code
}

func TestGateScriptAcceptsNoGuiAlongsideOtherFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--no-gui", "--bogus-flag"},
		{"--no-gui", "--go", "--bogus-flag"},
		{"--no-gui", "--quick", "--bogus-flag"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, code := runGateScript(t, args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2 for the unknown flag:\n%s", code, out)
			}
			if !strings.Contains(out, "--bogus-flag") {
				t.Errorf("output does not name the unknown flag --bogus-flag:\n%s", out)
			}
			if strings.Contains(out, "unknown flag: --no-gui") || strings.Contains(out, "unknown flag: --go") || strings.Contains(out, "unknown flag: --quick") {
				t.Errorf("a supported flag was reported as unknown:\n%s", out)
			}
		})
	}
}

func TestGateScriptRunsTheTaggedChecksAndTheGUISuite(t *testing.T) {
	script := readRepoFile(t, "scripts/test-all.sh")
	for _, want := range []string{
		"go vet -tags mcp,guimcp ./...",
		"go test -tags mcp -run 'DialogQueue|InstanceIdentity' ./internal/pdfservice/ .",
		"wails3 task build:mcp",
		"-tags guimcp",
		"./tests/gui-mcp/",
		"xvfb-run -a dbus-run-session --",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("scripts/test-all.sh does not run %q", want)
		}
	}
}
