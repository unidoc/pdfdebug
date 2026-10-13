package pages_navigator_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
		if content, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
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

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI binary once per test package and returns its path.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-pages-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = projectRoot(t)
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

// runCLI executes the CLI binary with args and returns stdout, stderr and the
// exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// runGoTest runs the root module's in-package tests matching runPattern and
// fails when none match or any fails.
func runGoTest(t *testing.T, runPattern, pkgPath string, extra ...string) {
	t.Helper()
	args := append([]string{"test", "-v", "-count=1"}, extra...)
	args = append(args, "-run", runPattern, pkgPath)
	cmd := exec.Command("go", args...)
	cmd.Dir = projectRoot(t)
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil {
		t.Fatalf("delegated test failed:\n%s", out)
	}
	if strings.Contains(out, "no tests to run") || !strings.Contains(out, "--- PASS") {
		t.Fatalf("no unit tests matched pattern %q in %s -- unit test does not exist yet:\n%s", runPattern, pkgPath, out)
	}
}

// writeFixture writes content to name inside a fresh temp dir and returns the path.
func writeFixture(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write fixture %s: %v", name, err)
	}
	return path
}

// testdataFile returns the absolute path of a committed testdata/ file.
func testdataFile(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata", name)
}

// pageEntry mirrors one element of the `dump pages --json` array.
type pageEntry struct {
	PageNum       int        `json:"pageNum"`
	ObjNum        int        `json:"objNum"`
	Gen           int        `json:"gen"`
	NodeID        string     `json:"nodeId"`
	ContentNodeID string     `json:"contentNodeId"`
	MediaBox      [4]float64 `json:"mediaBox"`
	Rotate        int        `json:"rotate"`
	Inherited     uint8      `json:"inherited"`
	AnnotCount    int        `json:"annotCount"`
	ContentLen    int64      `json:"contentLen"`
	Err           string     `json:"error"`
}

// pageEntryKeys is every key a `dump pages --json` element carries.
var pageEntryKeys = []string{
	"pageNum", "objNum", "gen", "nodeId", "contentNodeId", "mediaBox",
	"rotate", "inherited", "annotCount", "contentLen", "error",
}

// dumpPagesJSON runs `dump pages --json` on path, requires exit 0 and a JSON
// array on stdout, and returns the parsed entries plus stderr.
func dumpPagesJSON(t *testing.T, path string) ([]pageEntry, string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "pages", "--json", path)
	if code != 0 {
		t.Fatalf("dump pages --json exited %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var entries []pageEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("stdout is not a JSON array of page entries: %v\nraw: %s", err, stdout)
	}
	return entries, stderr
}

// dumpPagesPlain runs `dump pages` in its plain-text default and requires exit 0.
func dumpPagesPlain(t *testing.T, path string) (string, string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "pages", path)
	if code != 0 {
		t.Fatalf("dump pages exited %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	return stdout, stderr
}

// numbered returns the entries that are pages, in order.
func numbered(entries []pageEntry) []pageEntry {
	var out []pageEntry
	for _, e := range entries {
		if e.PageNum > 0 {
			out = append(out, e)
		}
	}
	return out
}

// stderrWarnings parses every {"warning": ...} JSON object on stderr.
func stderrWarnings(t *testing.T, stderr string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]string
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		if w, ok := obj["warning"]; ok {
			out = append(out, w)
		}
	}
	return out
}

// pageNodeIDFromTree returns the node id `dump tree --page n` resolves, which is
// what GetPageNode returns for page n.
func pageNodeIDFromTree(t *testing.T, path string, n int) string {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "tree", "--json", "--depth", "0", "--page", strconv.Itoa(n), path)
	if code != 0 {
		t.Fatalf("dump tree --page %d exited %d\nstderr: %s", n, code, stderr)
	}
	var node struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &node); err != nil {
		t.Fatalf("dump tree --page %d stdout is not a node: %v\nraw: %s", n, err, stdout)
	}
	return node.ID
}
