// Story 13-6 RED-PHASE acceptance harness for the new top-level CLI command
// `diff` (path-aligned structural diff of two PDFs).
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess. These
// tests assert the EXPECTED post-implementation behavior of the NEW `diff`
// command. They MUST FAIL against the current binary (which has no `diff`
// command) until Story 13-6 is implemented. They fail at RUNTIME (unknown
// command -> exit 1 / wrong output shape / wrong exit code), not at compile
// time, so the main `unidoc-pdf-debugger` module keeps building green (mirrors
// 13-2 / 13-3 / 13-4 / 13-5). This module has its own go.mod and is not part of
// the main build.
//
// Test pyramid: every case here is a Go integration-level black-box test
// against the built CLI binary -- the project's established acceptance level
// for the CLI surface (10-x, 13-1..13-5). The path-alignment ENGINE is unit-
// tested co-located in internal/pdfcore/diff_test.go; the GUI side-by-side view
// is covered at the component (Vitest) level in
// frontend/src/components/DiffView.test.tsx. No browser/E2E layer is warranted.
//
// JSON wire contract pinned by this suite (camelCase per the IPC rules,
// matching the 13-2..13-5 precedent):
//
//	diff --json  =>  top-level OBJECT:
//	  summary  { added:int, removed:int, changed:int,
//	             pageCountLeft:int, pageCountRight:int,
//	             versionChanged:bool, encryptionChanged:bool,
//	             infoChanged:bool, xmpChanged:bool }
//	  root     DiffNode, recursively:
//	    path         string   catalog-rooted structural path
//	    status       string   "added" | "removed" | "changed" | "unchanged"
//	    kind         string   "dict" | "array" | "stream" | "scalar" | "ref"
//	    changedKeys  []string (optional)
//	    leftSummary  string
//	    rightSummary string
//	    children     []DiffNode (optional)
//
// Exit codes (AC4, a hard three-way contract -- distinct so scripts can tell
// "differ" from "broken file"; `dump` only uses 0/2):
//	0  ran successfully, the two documents are structurally IDENTICAL
//	1  ran successfully AND the documents DIFFER (the scriptable signal)
//	2  operational error (missing/unreadable file, bad args, parse failure)
//
// Naming: 13.6-INTG-NNN [Px] per the story Testing Requirements (AC7).
//
// Run: cd tests/structural-diff && go test -v -count=1 ./...
package structural_diff_test

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

// --- CLI harness (mirrors the embedded-data through compliance suites) --------

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

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI binary once per test package and returns its path.
// The build is cached via sync.Once: the binary is identical for every test in
// the module, so the expensive `go build` runs a single time instead of once
// per test. The build dir is a plain os.MkdirTemp (not t.TempDir) because it is
// shared across tests and must outlive any single one; the OS reclaims it.
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

// writeTempPDF writes content to a temp file and returns its path.
func writeTempPDF(t *testing.T, name string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}
	return path
}

// --- JSON helpers ------------------------------------------------------------

// parsesAsJSON reports whether s (trimmed) is a single well-formed JSON value.
func parsesAsJSON(s string) bool {
	var v any
	return json.Unmarshal([]byte(strings.TrimSpace(s)), &v) == nil
}

// assertNotJSON fails when out is a top-level JSON object/array document. The
// plain-text default must NOT parse as a JSON document (13-1 contract).
func assertNotJSON(t *testing.T, out string) {
	t.Helper()
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return
	}
	if (trimmed[0] == '{' || trimmed[0] == '[') && parsesAsJSON(trimmed) {
		t.Fatalf("default output parsed as a JSON document; expected plain text:\n%s", out)
	}
}

// assertASCII fails when out contains a non-ASCII byte (13-1 plain-text contract).
func assertASCII(t *testing.T, out string) {
	t.Helper()
	for i := 0; i < len(out); i++ {
		if out[i] > 0x7f {
			t.Errorf("plain-text output contains non-ASCII byte 0x%02x at offset %d", out[i], i)
			return
		}
	}
}

// assertTrailingNewline fails when out does not end in a newline (13-1 contract).
func assertTrailingNewline(t *testing.T, out string) {
	t.Helper()
	if out == "" {
		return
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("plain-text output does not end with a trailing newline")
	}
}

// diffResult parses `diff --json` stdout as the contract's top-level object.
func diffResult(t *testing.T, stdout string) map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" || trimmed[0] != '{' {
		t.Fatalf("--json must emit a top-level object, got:\n%s", stdout)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("failed to parse --json output: %v\nraw: %s", err, stdout)
	}
	return res
}

// summaryOf extracts res["summary"] and returns (added, removed, changed).
func summaryOf(t *testing.T, res map[string]any) (added, removed, changed int) {
	t.Helper()
	m, ok := res["summary"].(map[string]any)
	if !ok {
		t.Fatalf("result has no \"summary\" object: %v", res)
	}
	return jsonInt(m["added"]), jsonInt(m["removed"]), jsonInt(m["changed"])
}

// jsonInt coerces a JSON number (float64) to int; non-numbers yield 0.
func jsonInt(v any) int {
	f, _ := v.(float64)
	return int(f)
}

// getStr returns m[key] as a string ("" when absent or not a string).
func getStr(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// --- fixture builders (self-contained; assembled to parse via Inspector.Open) -
//
// Every fixture is a minimal hand-assembled PDF built to parse through the
// existing open path (guarded by 13.6-INTG-000) while exercising a specific
// structural relationship. None exist in testdata/ today; the red-phase suite
// is self-contained (13-4 / 13-5 precedent).

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func pad10(n int) string {
	s := itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assemblePDF stitches a 1.7 header, object bodies (object i+1 at position i),
// an xref table, and a trailer whose /Root points at rootNum.
func assemblePDF(rootNum int, objs ...string) []byte {
	header := "%PDF-1.7\n"
	body := header
	offsets := make([]int, len(objs))
	cur := len(header)
	for i, o := range objs {
		offsets[i] = cur
		body += o
		cur += len(o)
	}
	xrefOffset := len(body)
	xref := "xref\n0 " + itoa(len(objs)+1) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + itoa(len(objs)+1) + " /Root " + itoa(rootNum) +
		" 0 R >>\nstartxref\n" + itoa(xrefOffset) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

// assemblePDFWithTrailer is assemblePDF with extra trailer entries injected
// (e.g. "/Info 4 0 R ") so a fixture can carry a trailer /Info dict that lives
// OFF the catalog-rooted walk.
func assemblePDFWithTrailer(rootNum int, trailerExtra string, objs ...string) []byte {
	header := "%PDF-1.7\n"
	body := header
	offsets := make([]int, len(objs))
	cur := len(header)
	for i, o := range objs {
		offsets[i] = cur
		body += o
		cur += len(o)
	}
	xrefOffset := len(body)
	xref := "xref\n0 " + itoa(len(objs)+1) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + itoa(len(objs)+1) + " /Root " + itoa(rootNum) +
		" 0 R " + trailerExtra + ">>\nstartxref\n" + itoa(xrefOffset) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

// infoProducerPDF is onePagePDF plus a trailer /Info dict (obj 4) with the given
// /Producer. The catalog graph is IDENTICAL across producers; only the trailer
// /Info differs. Two of these are structurally identical in the object walk yet
// must diff as "differ" (exit 1) because /Info is a document-level fact.
func infoProducerPDF(producer string) []byte {
	return assemblePDFWithTrailer(1, "/Info 4 0 R ",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Producer ("+producer+") >>\nendobj\n",
	)
}

// onePagePDF is the baseline single-page document (natural numbering).
func onePagePDF() []byte {
	return assemblePDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
}

// onePageRenumberedPDF is STRUCTURALLY IDENTICAL to onePagePDF but with the
// object numbers permuted and /Root 3 0 R -- the CLI-level alignment guardrail.
func onePageRenumberedPDF() []byte {
	return assemblePDF(3,
		"1 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [1 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
	)
}

// twoPagePDF is a two-page document (page-count delta vs onePagePDF).
func twoPagePDF() []byte {
	return assemblePDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
}

// changedMediaBoxPDF is onePagePDF with the page /MediaBox modified in place.
func changedMediaBoxPDF() []byte {
	return assemblePDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>\nendobj\n",
	)
}
