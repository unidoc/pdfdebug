// Acceptance harness for scalar values on the PDF inspection surfaces: the
// object tree (plain text and JSON), the object detail view, the structural
// diff and the reserialized object source.
//
// Black-box: builds the pdfdebug binary and runs it against PDFs this module
// assembles itself (see fixtures_test.go). The module has its own go.mod, so it
// runs via the per-suite tests/*/ loop rather than `go test ./...`.
//
// Run: cd tests/object-tree-scalar-values && go test -v -count=1 ./...
package object_tree_scalar_values_test

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

// --- typed JSON shapes (parse-then-assert, never grep) ----------------------

// treeNodeJSON mirrors one node of `dump tree --json`. Value and ValueRaw are
// pointers so an absent key is distinguishable from an empty one: both are
// omitempty on the wire and their absence is part of the contract.
type treeNodeJSON struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	RawKey      string          `json:"rawKey"`
	NodeType    string          `json:"nodeType"`
	ValueType   string          `json:"valueType"`
	HasChildren bool            `json:"hasChildren"`
	ChildCount  int             `json:"childCount"`
	PdfRef      string          `json:"pdfRef"`
	TypeName    string          `json:"typeName"`
	Value       *string         `json:"value"`
	ValueRaw    *string         `json:"valueRaw"`
	Children    []*treeNodeJSON `json:"children"`
}

// valueEntryJSON mirrors a ValueEntry in `dump object --json`.
type valueEntryJSON struct {
	Type      string `json:"type"`
	Display   string `json:"display"`
	Raw       string `json:"raw"`
	RefTarget string `json:"refTarget"`
}

// objectDetailJSON mirrors `dump object --json`.
type objectDetailJSON struct {
	NodeID     string `json:"nodeId"`
	ObjectRef  string `json:"objectRef"`
	Type       string `json:"type"`
	Properties []struct {
		Key   string         `json:"key"`
		Value valueEntryJSON `json:"value"`
	} `json:"properties"`
	Elements    []valueEntryJSON `json:"elements"`
	ScalarValue *valueEntryJSON  `json:"scalarValue"`
}

// diffNodeJSON mirrors one node of `diff --json`.
type diffNodeJSON struct {
	Path         string          `json:"path"`
	Status       string          `json:"status"`
	Kind         string          `json:"kind"`
	LeftSummary  string          `json:"leftSummary"`
	RightSummary string          `json:"rightSummary"`
	Children     []*diffNodeJSON `json:"children"`
}

// diffResultJSON mirrors `diff --json`.
type diffResultJSON struct {
	Root *diffNodeJSON `json:"root"`
}

// --- harness ----------------------------------------------------------------

// TestMain removes the temp dirs holding the built CLI binary and the generated
// fixtures. A per-test t.Cleanup cannot: both are shared across the module.
func TestMain(m *testing.M) {
	code := m.Run()
	if cliTmpDir != "" {
		_ = os.RemoveAll(cliTmpDir)
	}
	if fixtureDir != "" {
		_ = os.RemoveAll(fixtureDir)
	}
	os.Exit(code)
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
	// Tracked separately from cliBinPath so a failed build still cleans up.
	cliTmpDir string

	projectRootOnce sync.Once
	projectRootDir  string
)

// projectRoot walks up to the main module's go.mod. Memoized.
func projectRoot(t *testing.T) string {
	t.Helper()
	projectRootOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			return
		}
		for {
			if content, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
				if strings.Contains(string(content), "module unidoc-pdf-debugger") {
					projectRootDir = dir
					return
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return
			}
			dir = parent
		}
	})
	if projectRootDir == "" {
		t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
	}
	return projectRootDir
}

// committedFixture returns the absolute path of a PDF under testdata/.
func committedFixture(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(projectRoot(t), "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s missing: %v", name, err)
	}
	return p
}

// buildCLI compiles the CLI binary once per test package and returns its path.
func buildCLI(t *testing.T) string {
	t.Helper()
	// Resolved before Do: projectRoot ends in t.Fatalf (runtime.Goexit), which
	// inside Do would mark the Once complete with both globals empty.
	root := projectRoot(t)
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-cli-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		cliTmpDir = tmpDir
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
			t.Fatalf("failed to run CLI %v: %v", args, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// treePlain runs `dump tree` on a fixture and returns the plain-text output,
// failing the test on a non-zero exit.
func treePlain(t *testing.T, pdf string) string {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "tree", "--depth", "20", pdf)
	if code != 0 {
		t.Fatalf("dump tree exited %d: %s", code, stderr)
	}
	return stdout
}

// treeJSON runs `dump tree --json` on a fixture and returns the parsed root.
func treeJSON(t *testing.T, pdf string) *treeNodeJSON {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "tree", "--json", "--depth", "20", pdf)
	if code != 0 {
		t.Fatalf("dump tree --json exited %d: %s", code, stderr)
	}
	var root treeNodeJSON
	if err := json.Unmarshal([]byte(stdout), &root); err != nil {
		t.Fatalf("dump tree --json is not valid JSON: %v\nraw: %s", err, stdout)
	}
	return &root
}

// objectJSON runs `dump object --json --ref` and returns the parsed detail.
func objectJSON(t *testing.T, pdf, ref string) *objectDetailJSON {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "object", "--json", "--ref", ref, pdf)
	if code != 0 {
		t.Fatalf("dump object --json --ref %s exited %d: %s", ref, code, stderr)
	}
	var detail objectDetailJSON
	if err := json.Unmarshal([]byte(stdout), &detail); err != nil {
		t.Fatalf("dump object --json is not valid JSON: %v\nraw: %s", err, stdout)
	}
	return &detail
}

// diffJSON runs `diff --json` over two fixtures and returns the parsed result.
// Exit code 1 means "documents differ", which every caller here expects.
func diffJSON(t *testing.T, left, right string) *diffResultJSON {
	t.Helper()
	stdout, stderr, code := runCLI(t, "diff", "--json", left, right)
	if code > 1 {
		t.Fatalf("diff --json exited %d: %s", code, stderr)
	}
	var result diffResultJSON
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("diff --json is not valid JSON: %v\nraw: %s", err, stdout)
	}
	return &result
}

// --- tree navigation --------------------------------------------------------

// nodeAt walks the tree by rawKey, one step per path element ("/Alt" for a dict
// entry, "[0]" for an array element), and fails the test if a step is missing.
func nodeAt(t *testing.T, root *treeNodeJSON, path ...string) *treeNodeJSON {
	t.Helper()
	node := root
	for i, step := range path {
		var next *treeNodeJSON
		for _, child := range node.Children {
			if child.RawKey == step {
				next = child
				break
			}
		}
		if next == nil {
			t.Fatalf("no node at %v: %q has no child with rawKey %q (children: %s)",
				path[:i+1], node.RawKey, step, childRawKeys(node))
		}
		node = next
	}
	return node
}

// childRawKeys lists a node's children by rawKey, for failure messages.
func childRawKeys(n *treeNodeJSON) string {
	keys := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		keys = append(keys, c.RawKey)
	}
	return strings.Join(keys, ", ")
}

// value returns a node's value and whether the key was emitted at all.
func value(n *treeNodeJSON) (string, bool) {
	if n.Value == nil {
		return "", false
	}
	return *n.Value, true
}

// valueRaw returns a node's raw counterpart and whether the key was emitted.
func valueRaw(n *treeNodeJSON) (string, bool) {
	if n.ValueRaw == nil {
		return "", false
	}
	return *n.ValueRaw, true
}

// mustValue fails unless the node carries a value key, then returns it.
func mustValue(t *testing.T, n *treeNodeJSON) string {
	t.Helper()
	v, ok := value(n)
	if !ok {
		t.Fatalf("node %s (%s) carries no value key", n.ID, n.RawKey)
	}
	return v
}

// --- plain-text helpers -----------------------------------------------------

// trimmedLines splits plain-text output into lines with the indentation
// removed, dropping blank lines.
func trimmedLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}
	return lines
}

// hasLine reports whether any line, with indentation removed, equals want.
func hasLine(out, want string) bool {
	for _, l := range trimmedLines(out) {
		if l == want {
			return true
		}
	}
	return false
}

// linesStartingWith returns every trimmed line whose text starts with prefix.
func linesStartingWith(out, prefix string) []string {
	var found []string
	for _, l := range trimmedLines(out) {
		if strings.HasPrefix(l, prefix) {
			found = append(found, l)
		}
	}
	return found
}

// oneLineStartingWith returns the single trimmed line starting with prefix,
// failing the test when there is not exactly one.
func oneLineStartingWith(t *testing.T, out, prefix string) string {
	t.Helper()
	found := linesStartingWith(out, prefix)
	if len(found) != 1 {
		t.Fatalf("want exactly one line starting with %q, got %d: %q", prefix, len(found), found)
	}
	return found[0]
}

// propertyValue returns the ValueEntry for key in a parsed object detail.
func propertyValue(t *testing.T, d *objectDetailJSON, key string) valueEntryJSON {
	t.Helper()
	for _, p := range d.Properties {
		if p.Key == key {
			return p.Value
		}
	}
	t.Fatalf("object %s has no property %q", d.ObjectRef, key)
	return valueEntryJSON{}
}

// diffNodeAt finds the diff node with the given structural path.
func diffNodeAt(t *testing.T, result *diffResultJSON, path string) *diffNodeJSON {
	t.Helper()
	var walk func(*diffNodeJSON) *diffNodeJSON
	walk = func(n *diffNodeJSON) *diffNodeJSON {
		if n == nil {
			return nil
		}
		if n.Path == path {
			return n
		}
		for _, c := range n.Children {
			if hit := walk(c); hit != nil {
				return hit
			}
		}
		return nil
	}
	if hit := walk(result.Root); hit != nil {
		return hit
	}
	t.Fatalf("diff has no node at path %q", path)
	return nil
}
