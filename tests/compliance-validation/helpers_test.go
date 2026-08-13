// Story 13-5 acceptance test harness for the top-level CLI
// command `validate` (structural PDF/A-1b and PDF/UA-1-structural conformance
// checks with jump-to-object).
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess. Failures
// surface at RUNTIME (unknown command -> exit 1 / wrong output shape / wrong
// exit code), not at compile time, so the main `unidoc-pdf-debugger` module
// keeps building green (mirrors 13-2 / 13-3 / 13-4).
//
// Test pyramid: every case here is a Go integration-level black-box test
// against the built CLI binary -- the project's established acceptance level
// for the CLI surface (10-x, 13-1..13-4). The Validate GUI panel and its
// jump-to-object wiring are covered at the component (Vitest) level in
// frontend/src/components/ValidateView.test.tsx; the veraPDF oracle
// cross-check lives in oracle_verapdf_test.go (skipped when veraPDF is
// absent). No browser/E2E layer is warranted.
//
// JSON wire contract pinned by this suite (camelCase per the IPC rules,
// matching the 13-2/13-3/13-4 precedent):
//
//	validate --json => top-level OBJECT:
//	  profile   string                selected profile ("pdfa-1b" | "pdfua-1-structural")
//	  summary   { errors:int, warnings:int }
//	  problems  array of Problem, each:
//	    ruleId    string   stable rule id
//	    profile   string   emitting profile (redundant per-run; self-describing)
//	    severity  string   "error" | "warning" | "info" (registry-declared)
//	    message   string   human-readable defect description
//	    objRef    string   "N G R"        (optional; "" for document-level)
//	    objNodeId string   "obj:{gen}:{num}" (present whenever objRef is; "" otherwise)
//	    specRef   string   ISO 32000 / PDF/A / PDF/UA clause (never empty)
//
// Exit codes (a hard three-way contract -- `dump` only uses 0/2):
//	0  ran successfully, ZERO error-severity problems (warnings/info allowed)
//	1  ran successfully AND found >=1 error-severity problem (the CI gate signal)
//	2  operational error (missing/unreadable file, unknown profile, view failure)
//
// Naming: [Px] per the story Testing Requirements.
//
// Run: cd tests/compliance-validation && go test -v -count=1 ./...
package compliance_validation_test

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

// --- CLI harness (mirrors the embedded-data, font-cmap and signature suites) --

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

// testdataDir returns the absolute path to the testdata/ directory at project root.
func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata")
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

// validateResult parses `validate --json` stdout as the contract's top-level
// object and returns it.
func validateResult(t *testing.T, stdout string) map[string]any {
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

// problemsOf extracts res["problems"] as a slice of problem objects.
func problemsOf(t *testing.T, res map[string]any) []map[string]any {
	t.Helper()
	raw, ok := res["problems"].([]any)
	if !ok {
		t.Fatalf("result has no \"problems\" array: %v", res)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, p := range raw {
		m, ok := p.(map[string]any)
		if !ok {
			t.Fatalf("problems entry is not an object: %v", p)
		}
		out = append(out, m)
	}
	return out
}

// summaryOf extracts res["summary"] and returns (errors, warnings).
func summaryOf(t *testing.T, res map[string]any) (errs, warns int) {
	t.Helper()
	m, ok := res["summary"].(map[string]any)
	if !ok {
		t.Fatalf("result has no \"summary\" object: %v", res)
	}
	return jsonInt(m["errors"]), jsonInt(m["warnings"])
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

// countBySeverity counts problems whose severity equals sev.
func countBySeverity(ps []map[string]any, sev string) int {
	n := 0
	for _, p := range ps {
		if getStr(p, "severity") == sev {
			n++
		}
	}
	return n
}

// findByMessageContains returns the first problem whose message contains sub
// (case-insensitive), or nil.
func findByMessageContains(ps []map[string]any, sub string) map[string]any {
	sub = strings.ToLower(sub)
	for _, p := range ps {
		if strings.Contains(strings.ToLower(getStr(p, "message")), sub) {
			return p
		}
	}
	return nil
}

// --- honesty guardrail -------------------------------------------------

// forbiddenVerdictPhrases are authoritative conformance verdicts the tool must
// NEVER emit. The command name "validate"/"validation" and the negated forms
// "not compliant"/"invalid"/"not valid" are deliberately NOT in this list;
// only standalone authoritative claims are.
var forbiddenVerdictPhrases = []string{
	"pdf/a compliant",
	"pdf/a-compliant",
	"pdf/ua compliant",
	"is compliant",
	"fully compliant",
	"conformant",
	"is valid",
	"valid pdf/a",
	"pdf/a valid",
	"passed validation",
	"validation passed",
	"compliance: pass",
}

// assertNoComplianceVerdict fails when out makes an authoritative conformance
// claim. The not-authoritative disclaimer must also be present.
func assertNoComplianceVerdict(t *testing.T, id, out string) {
	t.Helper()
	lower := strings.ToLower(out)
	for _, p := range forbiddenVerdictPhrases {
		if strings.Contains(lower, p) {
			t.Errorf("[%s] authoritative conformance verdict %q present:\n%s", id, p, out)
		}
	}
	if !strings.Contains(lower, "structural checks only") {
		t.Errorf("[%s] output must always carry the \"structural checks only\" disclaimer:\n%s", id, out)
	}
}

// --- fixture builders (validated during ATDD authoring) ----------------------
//
// Every fixture is a minimal hand-assembled PDF built to PARSE through the
// existing Inspector.Open path while deliberately tripping (or
// satisfying) specific structural rules. They are not in testdata/; this suite
// is self-contained.

func pad10(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assemblePDF stitches a header, object bodies (object i+1), an xref table, and
// a trailer with /Root 1 0 R. Mirrors the 13-4 fixture assembler.
func assemblePDF(objs []string) []byte {
	body := "%PDF-1.7\n"
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(body)
		body += o
	}
	xrefOff := len(body)
	size := len(objs) + 1
	xref := "xref\n0 " + strconv.Itoa(size) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + strconv.Itoa(size) + " /Root 1 0 R >>\nstartxref\n" +
		strconv.Itoa(xrefOff) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

// streamObj renders object number n as a content-stream object with a correct
// /Length for payload.
func streamObj(n int, payload string) string {
	return strconv.Itoa(n) + " 0 obj\n<< /Length " + strconv.Itoa(len(payload)) +
		" >>\nstream\n" + payload + "\nendstream\nendobj\n"
}

// nonEmbeddedFontPDF builds a one-page PDF that references a Type1 font with NO
// /FontDescriptor / /FontFile* (PDF/A-1b 6.3.4 forbids non-embedded fonts). The
// offending font is object 4 -> node id obj:0:4.
func nonEmbeddedFontPDF() []byte {
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
		streamObj(5, "BT /F1 24 Tf 72 720 Td (Hi) Tj ET"),
	})
}

// untaggedPDF builds a one-page PDF with NO /MarkInfo, NO /StructTreeRoot, and
// NO /Lang -- the PDF/UA-1 structural warnings (all non-gating). The missing
// /Lang is the story's canonical document-level (no object ref) problem.
func untaggedPDF() []byte {
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	})
}

// taggedPDF builds a one-page PDF that SATISFIES the PDF/UA-1 structural subset:
// /MarkInfo << /Marked true >>, a /StructTreeRoot, and a document /Lang. Under
// the pdfua-1-structural profile it yields zero problems.
func taggedPDF() []byte {
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 4 0 R /Lang (en-US) >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /StructTreeRoot >>\nendobj\n",
	})
}
