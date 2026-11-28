package pdf_parsing_correctness_fixes_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the pdfdebug CLI once per run and returns its path.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		name := "pdfdebug"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		dir, err := os.MkdirTemp("", "pdfdebug-correctness-")
		if err != nil {
			cliBuildErr = err.Error()
			return
		}
		bin := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/cli/")
		cmd.Dir = projectRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "go build ./cmd/cli/: " + err.Error() + "\n" + string(out)
			return
		}
		cliBinPath = bin
	})
	if cliBuildErr != "" {
		t.Fatal(cliBuildErr)
	}
	return cliBinPath
}

// runCLI runs the CLI from the project root and returns stdout, stderr and
// the exit code.
func runCLI(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = projectRoot(t)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return stdout.String(), stderr.String(), 0
	case errors.As(err, &ee):
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	t.Fatalf("run %v: %v", args, err)
	return "", "", 0
}

// runGoTest runs `go test` with args from the project root. A -run pattern
// that matches no test is an error, so a renamed test cannot pass silently.
func runGoTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	if err == nil && strings.Contains(string(out), "no tests to run") {
		err = errors.New("the -run pattern matched no test")
	}
	return string(out), err
}
