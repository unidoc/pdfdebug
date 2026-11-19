// Package dependency_updates_test checks the root module and frontend
// dependency set: the Go directive and module versions in go.mod, the frontend
// lockfile clear of the known advisories, the open path that parses without
// pdfcpu's validator, broken-checksum streams that still decode, and the docs
// and comments that describe pdfcpu's behaviour.
//
// The Go unit tests for the pdfcore changes live next to the code and are run
// again from here (colocated_unit_test.go).
//
// Run: cd tests/dependency-updates && go test -count=1 ./...
package dependency_updates_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

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

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// requirement is one go.mod require entry.
type requirement struct {
	version  string
	indirect bool
}

var requireLine = regexp.MustCompile(`^\s*(?:require\s+)?([A-Za-z0-9./_~-]+)\s+(v\S+)(\s*//\s*indirect)?\s*$`)

// goModRequires parses the require entries of the root go.mod.
func goModRequires(t *testing.T) map[string]requirement {
	t.Helper()
	reqs := map[string]requirement{}
	inBlock := false
	for line := range strings.SplitSeq(readRepoFile(t, "go.mod"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "require (":
			inBlock = true
			continue
		case inBlock && trimmed == ")":
			inBlock = false
			continue
		case !inBlock && !strings.HasPrefix(trimmed, "require "):
			continue
		}
		if m := requireLine.FindStringSubmatch(trimmed); m != nil {
			reqs[m[1]] = requirement{version: m[2], indirect: m[3] != ""}
		}
	}
	return reqs
}

// compareVersions compares dotted numeric versions, with or without a leading
// "v", ignoring any pre-release suffix. It returns -1, 0 or 1.
func compareVersions(t *testing.T, a, b string) int {
	t.Helper()
	pa, pb := versionParts(t, a), versionParts(t, b)
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(t *testing.T, v string) []int {
	t.Helper()
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var nums []int
	for p := range strings.SplitSeq(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("version %q has a non-numeric part %q", v, p)
		}
		nums = append(nums, n)
	}
	return nums
}

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
		dir, err := os.MkdirTemp("", "pdfdebug-deps-")
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

// runCLI runs the CLI and returns stdout, stderr and the exit code.
func runCLI(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	return stdout.String(), stderr.String(), 0
}

// runGoTest runs `go test` with args from the project root.
func runGoTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// classicPDF lays out object bodies as a classic-xref PDF. Body i becomes
// object i+1, and object 1 is the catalog.
func classicPDF(bodies ...string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(bodies))
	for i, body := range bodies {
		offsets[i] = b.Len()
		b.WriteString(strconv.Itoa(i+1) + " 0 obj\n" + body + "\nendobj\n")
	}
	xref := b.Len()
	b.WriteString("xref\n0 " + strconv.Itoa(len(bodies)+1) + "\n0000000000 65535 f \n")
	for _, off := range offsets {
		b.WriteString(leftPad10(off) + " 00000 n \n")
	}
	b.WriteString("trailer\n<< /Size " + strconv.Itoa(len(bodies)+1) + " /Root 1 0 R >>\nstartxref\n" +
		strconv.Itoa(xref) + "\n%%EOF\n")
	return []byte(b.String())
}

func leftPad10(n int) string {
	s := strconv.Itoa(n)
	return strings.Repeat("0", 10-len(s)) + s
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
