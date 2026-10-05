package images_navigator_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// cliTimeout bounds one CLI run, so a walk that never terminates on a form
// cycle fails the case instead of hanging the suite.
const cliTimeout = 2 * time.Minute

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
// The root is resolved outside the Once, because a t.Fatalf inside it would
// mark the Once done with no binary built.
func buildCLI(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-images-")
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

// runCLI executes the CLI binary with args and returns stdout, stderr and the
// exit code. A run past cliTimeout fails the test.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildCLI(t), args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("pdfdebug %s did not finish within %s", strings.Join(args, " "), cliTimeout)
	}
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

// imageEntry mirrors one element of the `dump images --json` array.
type imageEntry struct {
	ObjNum               int       `json:"objNum"`
	Gen                  int       `json:"gen"`
	NodeID               string    `json:"nodeId"`
	Width                int       `json:"width"`
	Height               int       `json:"height"`
	BitsPerComponent     int       `json:"bitsPerComponent"`
	ColorSpace           string    `json:"colorSpace"`
	Filters              []string  `json:"filters"`
	ImageMask            bool      `json:"imageMask"`
	SMask                *string   `json:"smask"`
	Decode               []float64 `json:"decode"`
	DecodeNonDefault     bool      `json:"decodeNonDefault"`
	SampleInterpretation string    `json:"sampleInterpretation"`
	AdobeMarker          string    `json:"adobeMarker"`
	AdobeTransform       *int      `json:"adobeTransform"`
	EstimatedBytes       int64     `json:"estimatedBytes"`
	FirstPage            int       `json:"firstPage"`
	PageCount            int       `json:"pageCount"`
	FirstPages           []int     `json:"firstPages"`
	Warning              string    `json:"warning"`
	Err                  string    `json:"error"`
}

// imageEntryKeys is every key a `dump images --json` element carries.
var imageEntryKeys = []string{
	"objNum", "gen", "nodeId", "width", "height", "bitsPerComponent",
	"colorSpace", "filters", "imageMask", "smask", "decode",
	"decodeNonDefault", "sampleInterpretation", "adobeMarker",
	"adobeTransform", "estimatedBytes", "firstPage", "pageCount",
	"firstPages", "warning", "error",
}

// imageFacts mirrors the fields of `dump image --json --metadata` that the
// index projects.
type imageFacts struct {
	Width                int       `json:"width"`
	Height               int       `json:"height"`
	BitsPerComponent     int       `json:"bitsPerComponent"`
	ColorSpace           string    `json:"colorSpace"`
	Filter               string    `json:"filter"`
	ImageMask            bool      `json:"imageMask"`
	SMask                *string   `json:"smask"`
	Decode               []float64 `json:"decode"`
	AdobeMarker          string    `json:"adobeMarker"`
	AdobeTransform       *int      `json:"adobeTransform"`
	SampleInterpretation string    `json:"sampleInterpretation"`
	DecodedBytes         int64     `json:"decodedBytes"`
}

// dumpImagesJSON runs `dump images --json` on path, requires exit 0 and a JSON
// array on stdout, and returns the parsed entries plus stderr.
func dumpImagesJSON(t *testing.T, path string) ([]imageEntry, string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "images", "--json", path)
	if code != 0 {
		t.Fatalf("dump images --json exited %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var entries []imageEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("stdout is not a JSON array of image entries: %v\nraw: %s", err, stdout)
	}
	return entries, stderr
}

// dumpImagesRaw runs `dump images --json` on path, requires exit 0, and returns
// each element as raw JSON keyed by field name.
func dumpImagesRaw(t *testing.T, path string) []map[string]json.RawMessage {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "images", "--json", path)
	if code != 0 {
		t.Fatalf("dump images --json exited %d, want 0\nstderr: %s", code, stderr)
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("stdout is not a JSON array of objects: %v\nraw: %s", err, stdout)
	}
	return raw
}

// dumpImagesPlain runs `dump images` in its plain-text default and requires exit 0.
func dumpImagesPlain(t *testing.T, path string) string {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "images", path)
	if code != 0 {
		t.Fatalf("dump images exited %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	return stdout
}

// imageFactsFor runs `dump image --json --metadata` on one reference.
func imageFactsFor(t *testing.T, path, ref string) imageFacts {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "image", "--json", "--metadata", "--ref", ref, path)
	if code != 0 {
		t.Fatalf("dump image --ref %q exited %d\nstderr: %s", ref, code, stderr)
	}
	var f imageFacts
	if err := json.Unmarshal([]byte(stdout), &f); err != nil {
		t.Fatalf("dump image --ref %q stdout is not an image object: %v\nraw: %s", ref, err, stdout)
	}
	return f
}

// images returns the entries that are images (a node id), in order.
func images(entries []imageEntry) []imageEntry {
	var out []imageEntry
	for _, e := range entries {
		if e.NodeID != "" {
			out = append(out, e)
		}
	}
	return out
}

// errorRows returns the entries with no node id, in order.
func errorRows(entries []imageEntry) []imageEntry {
	var out []imageEntry
	for _, e := range entries {
		if e.NodeID == "" {
			out = append(out, e)
		}
	}
	return out
}

// objNums lists the object numbers of entries, in order.
func objNums(entries []imageEntry) []int {
	out := make([]int, len(entries))
	for i, e := range entries {
		out[i] = e.ObjNum
	}
	return out
}

// entryFor returns the image entry for objNum, failing when there is none.
func entryFor(t *testing.T, entries []imageEntry, objNum int) imageEntry {
	t.Helper()
	for _, e := range entries {
		if e.NodeID != "" && e.ObjNum == objNum {
			return e
		}
	}
	t.Fatalf("no entry for object %d in %+v", objNum, entries)
	return imageEntry{}
}

// parseTable splits a plain-text table into rows keyed by header name, using
// the header's column starts; cells are padded to those columns.
func parseTable(t *testing.T, out string) []map[string]string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "REF") {
		t.Fatalf("output does not start with a REF header:\n%s", out)
	}
	header := lines[0]
	names := strings.Fields(header)
	starts := make([]int, len(names))
	from := 0
	for i, n := range names {
		idx := strings.Index(header[from:], n)
		starts[i] = from + idx
		from = starts[i] + len(n)
	}
	var rows []map[string]string
	for _, line := range lines[1:] {
		r := map[string]string{}
		for i, n := range names {
			if starts[i] >= len(line) {
				r[n] = ""
				continue
			}
			end := len(line)
			if i+1 < len(names) && starts[i+1] < end {
				end = starts[i+1]
			}
			r[n] = strings.TrimSpace(line[starts[i]:end])
		}
		rows = append(rows, r)
	}
	return rows
}

// rowFor returns the parsed table row whose REF cell is ref.
func rowFor(t *testing.T, rows []map[string]string, ref string) map[string]string {
	t.Helper()
	for _, r := range rows {
		if r["REF"] == ref {
			return r
		}
	}
	t.Fatalf("no row with REF %q in %+v", ref, rows)
	return nil
}
