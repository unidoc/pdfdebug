// Acceptance harness for `pdfdebug dump structure`.
//
// Black-box: the CLI is built once and run as a subprocess over committed
// fixtures in testdata/ (structure.pdf, structure-broken.pdf, untagged.pdf).
// Covers output shape, exit codes and usage errors only; walk edge cases,
// caps and the marked-content scan live in the pdfcore unit tests.
//
// Run: cd tests/dump-structure && go test -count=1 ./...
package dump_structure_test

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

// projectRoot walks up from the test directory to the main module's go.mod.
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

// fixture returns the absolute path of a file in testdata/, failing when it
// is missing.
func fixture(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(projectRoot(t), "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s missing: %v", name, err)
	}
	return p
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI once per test package and returns the binary path.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-dump-structure-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = projectRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(out)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	return cliBinPath
}

// runCLI runs the CLI with args and returns stdout, stderr and the exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// structElem is the subset of an element object this suite reads.
type structElem struct {
	Ref    string       `json:"ref"`
	S      string       `json:"s"`
	Page   int          `json:"page"`
	Kids   []*structKid `json:"kids"`
	Errmsg string       `json:"error"`
}

// structKid is one /K entry.
type structKid struct {
	Kind    string      `json:"kind"`
	Element *structElem `json:"element"`
	MCID    *int        `json:"mcid"`
	Page    int         `json:"page"`
	ObjType string      `json:"objType"`
	Errmsg  string      `json:"error"`
}

// alignment is one per-MCID row of a page.
type alignment struct {
	MCID              int      `json:"mcid"`
	ContentCount      int      `json:"contentCount"`
	ParentTreeElement string   `json:"parentTreeElement"`
	TreeElements      []string `json:"treeElements"`
	Consistent        bool     `json:"consistent"`
}

// structPage is one page row.
type structPage struct {
	PageNum       int         `json:"pageNum"`
	StructParents *int        `json:"structParents"`
	ParentTreeErr string      `json:"parentTreeError"`
	Alignment     []alignment `json:"alignment"`
}

// parentTreeEntry is one ParentTree key no page names.
type parentTreeEntry struct {
	Key        int    `json:"key"`
	ElementRef string `json:"elementRef"`
	S          string `json:"s"`
}

// problem is one top-level finding.
type problem struct {
	Where   string `json:"where"`
	Message string `json:"message"`
}

// structureReport is the subset of `dump structure --json` this suite reads.
type structureReport struct {
	HasStructTree   bool              `json:"hasStructTree"`
	HasParentTree   bool              `json:"hasParentTree"`
	Elements        []*structKid      `json:"elements"`
	Pages           []*structPage     `json:"pages"`
	OtherParentTree []parentTreeEntry `json:"otherParentTree"`
	Problems        []problem         `json:"problems"`
}

// reportKeys is every top-level key of the --json object.
var reportKeys = []string{
	"hasStructTree", "rootRef", "hasParentTree", "parentTreeRef", "marked",
	"docLang", "parentTreeNextKey", "roleMap", "elements", "pages",
	"otherParentTree", "problems", "counts",
}

// dumpStructureJSON runs `dump structure --json` on a fixture, requires exit
// 0 and a JSON object on stdout, and returns it with the raw output.
func dumpStructureJSON(t *testing.T, name string) (structureReport, string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "structure", "--json", fixture(t, name))
	if code != 0 {
		t.Fatalf("dump structure --json %s exit = %d, want 0\nstderr: %s", name, code, stderr)
	}
	var r structureReport
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("--json output is not a structure report object: %v\n%s", err, stdout)
	}
	return r, stdout
}

// dumpStructurePlain runs `dump structure` in plain text and requires exit 0.
func dumpStructurePlain(t *testing.T, name string) string {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "structure", fixture(t, name))
	if code != 0 {
		t.Fatalf("dump structure %s exit = %d, want 0\nstderr: %s", name, code, stderr)
	}
	return stdout
}

// eachKid calls fn on every kid of kids and of their elements, depth first.
func eachKid(kids []*structKid, fn func(k *structKid)) {
	for _, k := range kids {
		fn(k)
		if k.Element != nil {
			eachKid(k.Element.Kids, fn)
		}
	}
}

// kidS returns the /S names of the element kids in kids, in order.
func kidS(kids []*structKid) []string {
	var out []string
	for _, k := range kids {
		if k.Element != nil {
			out = append(out, k.Element.S)
		}
	}
	return out
}

// page returns page n of the report, failing when it is missing.
func page(t *testing.T, r structureReport, n int) *structPage {
	t.Helper()
	for _, p := range r.Pages {
		if p.PageNum == n {
			return p
		}
	}
	t.Fatalf("page %d missing from the report", n)
	return nil
}

// hasLinePrefix reports whether any line of out, with leading spaces removed,
// starts with prefix.
func hasLinePrefix(out, prefix string) bool {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), prefix) {
			return true
		}
	}
	return false
}
