// Package cli_tree_dump_test provides acceptance tests for Story 5.1:
// CLI Binary Setup and Tree Dump Command.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 5-1 is implemented.
//
// Test Levels: Unit (Go) and Integration (Go) -- CLI binary build + execution.
// No browser interaction required; all criteria are CLI binary validation.
//
// Run: cd tests/cli-tree-dump && go test -v -count=1 ./...
package cli_tree_dump_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Build binary, run `pdfdebug dump tree --json testdata/minimal.pdf`,
// verify stdout parses as valid JSON, exit code 0.
// Given a valid PDF file, When `pdfdebug dump tree [--json] <file>` is
//       executed, Then the CLI outputs the PDF object tree as structured JSON
//       to stdout, And the exit code is 0.
// ---------------------------------------------------------------------------

func TestTreeDump_ValidPDF_OutputsJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// stdout must be valid JSON
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nraw: %s", err, stdout)
	}
}

// ---------------------------------------------------------------------------
// Parse stdout JSON, verify root object has fields: id, label,
// nodeType, hasChildren, childCount.
// JSON includes tree node objects matching the TreeNode model.
// ---------------------------------------------------------------------------

func TestTreeDump_JSONContainsTreeNodeFields(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var root map[string]any
	mustParseJSON(t, stdout, &root)

	requiredFields := []string{"id", "label", "nodeType", "hasChildren", "childCount"}
	for _, field := range requiredFields {
		if _, ok := root[field]; !ok {
			t.Errorf("root node missing required field %q", field)
		}
	}
}

// ---------------------------------------------------------------------------
// Verify JSON has nested `children` array with at least one level of depth
// (not just root).
// Children are recursively included.
// ---------------------------------------------------------------------------

func TestTreeDump_IncludesChildren(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var root map[string]any
	mustParseJSON(t, stdout, &root)

	children, ok := root["children"]
	if !ok {
		t.Fatal("root node has no 'children' field")
	}

	childArr, ok := children.([]any)
	if !ok || len(childArr) == 0 {
		t.Fatal("root 'children' is empty or not an array -- tree must include at least one level of children")
	}
}

// ---------------------------------------------------------------------------
// Run with non-existent path, verify stderr contains JSON {"error":
// "..."}, stdout is empty, exit code 2.
// Given an invalid file path, Then an error message in JSON format is
//       written to stderr, And the exit code is non-zero (2 for file error),
//       And stdout remains empty.
// ---------------------------------------------------------------------------

func TestTreeDump_InvalidFilePath_JSONErrorOnStderr(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--json", "/nonexistent/path/fake.pdf")

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// Run with testdata/malformed.pdf, verify stderr has JSON error, exit code
// 2, no panic.
// Given an unparseable PDF, Then an error message in JSON format is
//       written to stderr, And the exit code is non-zero (2 for file error).
// ---------------------------------------------------------------------------

func TestTreeDump_MalformedPDF_JSONErrorOnStderr(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "malformed.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// Run `pdfdebug --help`, verify output contains command names, exit code
// 0.
// `pdfdebug --help` shows clear usage information.
// ---------------------------------------------------------------------------

func TestHelp_PrintsUsage(t *testing.T) {
	bin := buildCLI(t)

	// --help text may go to stdout or stderr; check both
	stdout, stderr, exitCode := runCLI(t, bin, "--help")

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for --help, got %d", exitCode)
	}

	combined := stdout + stderr
	if !strings.Contains(combined, "dump") {
		t.Error("--help output does not mention 'dump' command")
	}
	if !strings.Contains(combined, "tree") {
		t.Error("--help output does not mention 'tree' subcommand")
	}
}

// ---------------------------------------------------------------------------
// Run `pdfdebug --version`, verify output contains version, exit code
// 0.
// `pdfdebug --version` shows the version number.
// ---------------------------------------------------------------------------

func TestVersion_PrintsVersionString(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "--version")

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for --version, got %d", exitCode)
	}

	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "version") {
		t.Error("--version output does not contain 'version'")
	}
}

// ---------------------------------------------------------------------------
// Parse entire stdout as JSON; any parse failure = test failure. Ensures
// no log noise.
// Output is well-formed JSON suitable for piping to jq.
// ---------------------------------------------------------------------------

func TestTreeDump_StdoutContainsOnlyJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// The entire stdout must be parseable as a single JSON value.
	// If there are any log lines, fmt.Println noise, or pdfcpu diagnostics
	// mixed in, this will fail.
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("stdout is not valid JSON (log noise present?)\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Verify error output schema is {"error": "..."} across invalid path,
// malformed, and encrypted scenarios.
// Error message in JSON format written to stderr.
// ---------------------------------------------------------------------------

func TestTreeDump_ErrorJSON_ConsistentStructure(t *testing.T) {
	bin := buildCLI(t)

	cases := []struct {
		name string
		args []string
	}{
		{"invalid path", []string{"dump", "tree", "--json", "/nonexistent/fake.pdf"}},
		{"malformed PDF", []string{"dump", "tree", "--json", filepath.Join(testdataDir(t), "malformed.pdf")}},
		{"encrypted PDF", []string{"dump", "tree", "--json", filepath.Join(testdataDir(t), "encrypted.pdf")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, exitCode := runCLI(t, bin, tc.args...)

			if exitCode == 0 {
				t.Errorf("%s: expected non-zero exit code", tc.name)
			}

			trimmed := strings.TrimSpace(stderr)
			var errObj map[string]string
			if err := json.Unmarshal([]byte(trimmed), &errObj); err != nil {
				t.Fatalf("%s: stderr not valid JSON: %v\nraw: %s", tc.name, err, stderr)
			}

			if _, ok := errObj["error"]; !ok {
				t.Errorf("%s: stderr JSON missing 'error' key", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Run `pdfdebug dump tree` with no file path, verify usage on stderr,
// exit code 1. Distinguishes usage error (1) from file error (2).
// Exit code 1 for usage error.
// ---------------------------------------------------------------------------

func TestTreeDump_MissingFilePath_UsageOnStderr_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree")

	if exitCode != 1 {
		t.Errorf("expected exit code 1 (usage error), got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty for usage errors, got: %s", stdout)
	}

	if strings.TrimSpace(stderr) == "" {
		t.Error("stderr should contain usage information")
	}
}

// ---------------------------------------------------------------------------
// Run with testdata/encrypted.pdf, verify error message contains "encrypt"
// (case-insensitive), exit code 2.
// Given an encrypted PDF, Then the error message on stderr specifically
//       mentions encryption, And the exit code is 2.
// ---------------------------------------------------------------------------

func TestTreeDump_EncryptedPDF_MentionsEncryption(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "encrypted.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty for error cases, got: %s", stdout)
	}

	if !strings.Contains(strings.ToLower(stderr), "encrypt") {
		t.Errorf("error message should mention encryption\nstderr: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// Run with testdata/multipage.pdf, verify tree dump labels contain
// page-related entries.
// Tree dump includes page structure from multi-page PDFs.
// ---------------------------------------------------------------------------

func TestTreeDump_MultipagePDF_IncludesPageLabels(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// The tree dump JSON should contain page-related labels somewhere
	// in the recursive structure. Check for common page indicators.
	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "page") {
		t.Error("tree dump of multipage.pdf does not contain any 'page' label")
	}
}

// TestTreeDump_MultipagePDF_PlainIncludesPageLabels is the plain-text sibling
// (Story 13-1): the default (no --json) output is human-readable plain text and
// still names the page structure (label spine), but must NOT be JSON.
func TestTreeDump_MultipagePDF_PlainIncludesPageLabels(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[13.1] tree plain: expected exit code 0, got %d", exitCode)
	}
	if json.Valid([]byte(strings.TrimSpace(stdout))) && strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Errorf("[13.1] tree plain: default output must be plain text, not JSON:\n%.200s", stdout)
	}
	if !strings.Contains(strings.ToLower(stdout), "page") {
		t.Error("[13.1] tree plain: default output does not name any page label")
	}
}

// ---------------------------------------------------------------------------
// Run `go list -deps ./cmd/cli/` from project root, verify no
// dependency contains "wails" in the import path.
// CLI binary includes only pdfcore and CLI logic (no Wails dependency).
// ---------------------------------------------------------------------------

func TestCLI_NoWailsImports(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "list", "-deps", "./cmd/cli/")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, string(output))
	}

	for line := range strings.SplitSeq(string(output), "\n") {
		if strings.Contains(line, "wails") {
			t.Errorf("CLI binary depends on Wails package: %s", line)
		}
	}
}

// ---------------------------------------------------------------------------
// Tree dump of testdata/multipage.pdf completes in under 5 seconds.
// Performance guard for recursive tree traversal (risk E5-R-001).
// Recursive tree traversal completes in reasonable time.
// ---------------------------------------------------------------------------

func BenchmarkTreeDump_MultipagePDF(b *testing.B) {
	// We cannot use buildCLI with *testing.B directly, so build manually.
	dir, err := os.Getwd()
	if err != nil {
		b.Fatalf("failed to get working directory: %v", err)
	}
	var root string
	for d := dir; ; d = filepath.Dir(d) {
		goMod := filepath.Join(d, "go.mod")
		if content, err := os.ReadFile(goMod); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				root = d
				break
			}
		}
		if filepath.Dir(d) == d {
			b.Fatal("could not find project root")
		}
	}

	tmpDir := b.TempDir()
	binName := "pdfdebug"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmpDir, binName)
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
	buildCmd.Dir = root
	if output, err := buildCmd.CombinedOutput(); err != nil {
		b.Fatalf("failed to build CLI: %s\n%s", err, output)
	}

	pdfPath := filepath.Join(root, "testdata", "multipage.pdf")

	b.ResetTimer()
	for b.Loop() {
		cmd := exec.Command(binPath, "dump", "tree", "--json", pdfPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("tree dump failed: %v\n%s", err, string(output))
		}
	}
}

// ---------------------------------------------------------------------------
// Build CLI binary, verify file size < 25MB.
// Originally targets 10MB, but pdfcpu alone pulls ~19MB of Go code. The
// realistic budget is 25MB until pdfcpu is replaced or trimmed.
// ---------------------------------------------------------------------------

func TestCLI_BinarySizeUnder25MB(t *testing.T) {
	bin := buildCLI(t)

	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("cannot stat binary: %v", err)
	}

	const maxSize = 25 * 1024 * 1024 // 25MB -- pdfcpu pulls in ~19MB of Go code
	if info.Size() > maxSize {
		t.Errorf("binary size %d bytes exceeds 25MB limit", info.Size())
	}
}

// ---------------------------------------------------------------------------
// Run `pdfdebug` with no subcommand, verify usage on stderr, exit code
// 1. Friendly error for bare invocation.
// --help shows clear usage; no-args should also guide the user.
// ---------------------------------------------------------------------------

func TestNoSubcommand_PrintsUsage_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin)

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for no subcommand, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty for usage errors, got: %s", stdout)
	}

	combined := stdout + stderr
	if !strings.Contains(combined, "dump") {
		t.Error("usage output does not mention 'dump' command")
	}
}

// ---------------------------------------------------------------------------
// Negative --depth value produces JSON error on stderr, exit code 1 (usage
// error).
// --depth semantics. Negative values are rejected as invalid input.
// ---------------------------------------------------------------------------

func TestTreeDump_NegativeDepth_Error(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--depth", "-1", pdfPath)

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for negative depth, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty for error cases, got: %s", stdout)
	}

	// stderr should contain a JSON error mentioning depth
	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	if msg, ok := errObj["error"]; !ok {
		t.Error("stderr JSON missing 'error' key")
	} else if !strings.Contains(strings.ToLower(msg), "depth") {
		t.Errorf("error message should mention depth, got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// `pdfdebug dump object` is implemented (story 5-2). Verify it produces
// a JSON error for a non-existent file (exit code 2).
// ---------------------------------------------------------------------------

func TestObjectDump_NonexistentFile_JSONError(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "1 0 R", "dummy.pdf")

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for file error, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty for error, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	if _, ok := errObj["error"]; !ok {
		t.Error("stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// `pdfdebug dump stream` with non-existent file returns JSON error on
// stderr and exit code 2 (file error). Originally tested the stub "not
// implemented" response; updated after story 5-3 implementation.
// ---------------------------------------------------------------------------

func TestStreamDump_NonexistentFile_JSONError(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", "--page", "1", "dummy.pdf")

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for file error, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty for error, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	if _, ok := errObj["error"]; !ok {
		t.Error("stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// `pdfdebug dump` with no resource prints usage to stderr, exit code 1.
// Missing subcommand arguments produce usage error.
// ---------------------------------------------------------------------------

func TestDumpNoResource_PrintsUsage_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump")

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for missing resource, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty, got: %s", stdout)
	}

	if strings.TrimSpace(stderr) == "" {
		t.Error("stderr should contain usage information")
	}
}

// ---------------------------------------------------------------------------
// `pdfdebug dump unknown` prints error + usage to stderr, exit code 1.
// Unknown resource names produce usage error.
// ---------------------------------------------------------------------------

func TestDumpUnknownResource_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "unknown")

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for unknown resource, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty, got: %s", stdout)
	}

	if !strings.Contains(stderr, "Unknown resource") {
		t.Errorf("stderr should mention unknown resource\nstderr: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// `pdfdebug garbage` (unknown top-level command) prints error + usage to
// stderr, exit code 1.
// Unknown commands guide the user with usage information.
// ---------------------------------------------------------------------------

func TestUnknownCommand_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "garbage")

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for unknown command, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty, got: %s", stdout)
	}

	if !strings.Contains(stderr, "Unknown command") {
		t.Errorf("stderr should mention unknown command\nstderr: %s", stderr)
	}

	// Should still show usage guidance
	if !strings.Contains(stderr, "dump") {
		t.Errorf("usage text should mention 'dump'\nstderr: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// REVISED by Story 13-1: Tree dump WITHOUT --json now emits human-readable
// PLAIN TEXT (the flipped default), NOT JSON. The plain output is structural:
// indented node lines with the catalog spine. --json opts into the JSON
// contract (covered by the other JSON cases).
// ---------------------------------------------------------------------------

func TestTreeDump_WithoutJSONFlag_OutputsPlainText(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// Run WITHOUT --json flag -> plain text default.
	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	trimmed := strings.TrimSpace(stdout)
	// Plain text must NOT be a JSON object/array.
	if strings.HasPrefix(trimmed, "{") && json.Valid([]byte(trimmed)) {
		t.Fatalf("default tree output must be plain text, not JSON\nraw: %s", stdout)
	}
	// Structural shape: the catalog node is the spine root; indented children follow.
	if !strings.Contains(stdout, "Catalog") {
		t.Errorf("plain tree output should name the Catalog root\nraw: %s", stdout)
	}
	if !strings.Contains(stdout, "\n  ") {
		t.Errorf("plain tree output should be indented (two-space child indent)\nraw: %s", stdout)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("plain output must end with a trailing newline")
	}
}

// ---------------------------------------------------------------------------
// Tree dump with --depth flag limits traversal.
// Given a large PDF, When --depth N is passed, Then recursive tree
//       traversal stops at depth N.
// ---------------------------------------------------------------------------

func TestTreeDump_DepthFlag_LimitsTraversal(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// Run with --depth 1: should have root + immediate children, but children
	// should NOT have their own children expanded.
	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", "--depth", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var root map[string]any
	mustParseJSON(t, stdout, &root)

	// Root should have children (depth 1 = one level below root)
	children, ok := root["children"]
	if !ok {
		t.Fatal("root has no 'children' at depth 1")
	}

	childArr, ok := children.([]any)
	if !ok || len(childArr) == 0 {
		t.Fatal("root children empty at depth 1")
	}

	// At depth 1, children should NOT have their own children expanded
	// (nodes with hasChildren=true should not have a populated children array)
	for _, child := range childArr {
		childMap, ok := child.(map[string]any)
		if !ok {
			continue
		}
		if grandchildren, exists := childMap["children"]; exists {
			if gcArr, ok := grandchildren.([]any); ok && len(gcArr) > 0 {
				t.Error("--depth 1 should not expand grandchildren, but found nested children")
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Tree dump with --depth 0 (unlimited) includes full recursive tree
// (same as omitting --depth).
// When omitted, traversal is unlimited (depth 0 = unlimited).
// ---------------------------------------------------------------------------

func TestTreeDump_DepthZero_UnlimitedTraversal(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// --depth 0 should behave same as no --depth flag (unlimited)
	stdoutDepth0, _, exitCode0 := runCLI(t, bin, "dump", "tree", "--json", "--depth", "0", pdfPath)
	if exitCode0 != 0 {
		t.Fatalf("expected exit code 0 with --depth 0, got %d", exitCode0)
	}

	stdoutNoDepth, _, exitCodeND := runCLI(t, bin, "dump", "tree", "--json", pdfPath)
	if exitCodeND != 0 {
		t.Fatalf("expected exit code 0 without --depth, got %d", exitCodeND)
	}

	// Both should produce the same output
	if stdoutDepth0 != stdoutNoDepth {
		t.Error("--depth 0 output differs from no-depth output (expected identical)")
	}
}
