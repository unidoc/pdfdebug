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
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 5.1-INTG-001 [P0]: Build binary, run `pdfdebug dump tree --json
// testdata/minimal.pdf`, verify stdout parses as valid JSON, exit code 0.
// AC#1: Given a valid PDF file, When `pdfdebug dump tree [--json] <file>` is
//       executed, Then the CLI outputs the PDF object tree as structured JSON
//       to stdout, And the exit code is 0.
// ---------------------------------------------------------------------------

func TestTreeDump_ValidPDF_OutputsJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P0] 5.1-INTG-001: expected exit code 0, got %d", exitCode)
	}

	// stdout must be valid JSON
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("[P0] 5.1-INTG-001: stdout is not valid JSON: %v\nraw: %s", err, stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.1-INTG-002 [P0]: Parse stdout JSON, verify root object has fields:
// id, label, nodeType, hasChildren, childCount.
// AC#1: JSON includes tree node objects matching the TreeNode model.
// ---------------------------------------------------------------------------

func TestTreeDump_JSONContainsTreeNodeFields(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P0] 5.1-INTG-002: expected exit code 0, got %d", exitCode)
	}

	var root map[string]any
	mustParseJSON(t, stdout, &root)

	requiredFields := []string{"id", "label", "nodeType", "hasChildren", "childCount"}
	for _, field := range requiredFields {
		if _, ok := root[field]; !ok {
			t.Errorf("[P0] 5.1-INTG-002: root node missing required field %q", field)
		}
	}
}

// ---------------------------------------------------------------------------
// 5.1-INTG-003 [P0]: Verify JSON has nested `children` array with at least
// one level of depth (not just root).
// AC#1: Children are recursively included.
// ---------------------------------------------------------------------------

func TestTreeDump_IncludesChildren(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P0] 5.1-INTG-003: expected exit code 0, got %d", exitCode)
	}

	var root map[string]any
	mustParseJSON(t, stdout, &root)

	children, ok := root["children"]
	if !ok {
		t.Fatal("[P0] 5.1-INTG-003: root node has no 'children' field")
	}

	childArr, ok := children.([]any)
	if !ok || len(childArr) == 0 {
		t.Fatal("[P0] 5.1-INTG-003: root 'children' is empty or not an array -- tree must include at least one level of children")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-001 [P0]: Run with non-existent path, verify stderr contains
// JSON {"error": "..."}, stdout is empty, exit code 2.
// AC#3: Given an invalid file path, Then an error message in JSON format is
//       written to stderr, And the exit code is non-zero (2 for file error),
//       And stdout remains empty.
// ---------------------------------------------------------------------------

func TestTreeDump_InvalidFilePath_JSONErrorOnStderr(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--json", "/nonexistent/path/fake.pdf")

	if exitCode != 2 {
		t.Errorf("[P0] 5.1-UNIT-001: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P0] 5.1-UNIT-001: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P0] 5.1-UNIT-001: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P0] 5.1-UNIT-001: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-002 [P0]: Run with testdata/malformed.pdf, verify stderr has JSON
// error, exit code 2, no panic.
// AC#3: Given an unparseable PDF, Then an error message in JSON format is
//       written to stderr, And the exit code is non-zero (2 for file error).
// ---------------------------------------------------------------------------

func TestTreeDump_MalformedPDF_JSONErrorOnStderr(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "malformed.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P0] 5.1-UNIT-002: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P0] 5.1-UNIT-002: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P0] 5.1-UNIT-002: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P0] 5.1-UNIT-002: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-003 [P0]: Run `pdfdebug --help`, verify output contains command
// names, exit code 0.
// AC#2: `pdfdebug --help` shows clear usage information.
// ---------------------------------------------------------------------------

func TestHelp_PrintsUsage(t *testing.T) {
	bin := buildCLI(t)

	// --help text may go to stdout or stderr; check both
	stdout, stderr, exitCode := runCLI(t, bin, "--help")

	if exitCode != 0 {
		t.Errorf("[P0] 5.1-UNIT-003: expected exit code 0 for --help, got %d", exitCode)
	}

	combined := stdout + stderr
	if !strings.Contains(combined, "dump") {
		t.Error("[P0] 5.1-UNIT-003: --help output does not mention 'dump' command")
	}
	if !strings.Contains(combined, "tree") {
		t.Error("[P0] 5.1-UNIT-003: --help output does not mention 'tree' subcommand")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-004 [P0]: Run `pdfdebug --version`, verify output contains
// version, exit code 0.
// AC#2: `pdfdebug --version` shows the version number.
// ---------------------------------------------------------------------------

func TestVersion_PrintsVersionString(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "--version")

	if exitCode != 0 {
		t.Errorf("[P0] 5.1-UNIT-004: expected exit code 0 for --version, got %d", exitCode)
	}

	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "version") {
		t.Error("[P0] 5.1-UNIT-004: --version output does not contain 'version'")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-005 [P1]: Parse entire stdout as JSON; any parse failure = test
// failure. Ensures no log noise.
// AC#1: Output is well-formed JSON suitable for piping to jq.
// ---------------------------------------------------------------------------

func TestTreeDump_StdoutContainsOnlyJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.1-UNIT-005: expected exit code 0, got %d", exitCode)
	}

	// The entire stdout must be parseable as a single JSON value.
	// If there are any log lines, fmt.Println noise, or pdfcpu diagnostics
	// mixed in, this will fail.
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("[P1] 5.1-UNIT-005: stdout is not valid JSON (log noise present?)\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-006 [P1]: Verify error output schema is {"error": "..."} across
// invalid path, malformed, and encrypted scenarios.
// AC#3: Error message in JSON format written to stderr.
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
				t.Errorf("[P1] 5.1-UNIT-006/%s: expected non-zero exit code", tc.name)
			}

			trimmed := strings.TrimSpace(stderr)
			var errObj map[string]string
			if err := json.Unmarshal([]byte(trimmed), &errObj); err != nil {
				t.Fatalf("[P1] 5.1-UNIT-006/%s: stderr not valid JSON: %v\nraw: %s", tc.name, err, stderr)
			}

			if _, ok := errObj["error"]; !ok {
				t.Errorf("[P1] 5.1-UNIT-006/%s: stderr JSON missing 'error' key", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-007 [P1]: Run `pdfdebug dump tree` with no file path, verify
// usage on stderr, exit code 1. Distinguishes usage error (1) from file
// error (2).
// AC#3: Exit code 1 for usage error.
// ---------------------------------------------------------------------------

func TestTreeDump_MissingFilePath_UsageOnStderr_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree")

	if exitCode != 1 {
		t.Errorf("[P1] 5.1-UNIT-007: expected exit code 1 (usage error), got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P1] 5.1-UNIT-007: stdout should be empty for usage errors, got: %s", stdout)
	}

	if strings.TrimSpace(stderr) == "" {
		t.Error("[P1] 5.1-UNIT-007: stderr should contain usage information")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-008 [P1]: Run with testdata/encrypted.pdf, verify error message
// contains "encrypt" (case-insensitive), exit code 2.
// AC#4: Given an encrypted PDF, Then the error message on stderr specifically
//       mentions encryption, And the exit code is 2.
// ---------------------------------------------------------------------------

func TestTreeDump_EncryptedPDF_MentionsEncryption(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "encrypted.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P1] 5.1-UNIT-008: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P1] 5.1-UNIT-008: stdout should be empty for error cases, got: %s", stdout)
	}

	if !strings.Contains(strings.ToLower(stderr), "encrypt") {
		t.Errorf("[P1] 5.1-UNIT-008: error message should mention encryption\nstderr: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// 5.1-INTG-004 [P2]: Run with testdata/multipage.pdf, verify tree dump
// labels contain page-related entries.
// AC#1: Tree dump includes page structure from multi-page PDFs.
// ---------------------------------------------------------------------------

func TestTreeDump_MultipagePDF_IncludesPageLabels(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P2] 5.1-INTG-004: expected exit code 0, got %d", exitCode)
	}

	// The tree dump JSON should contain page-related labels somewhere
	// in the recursive structure. Check for common page indicators.
	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "page") {
		t.Error("[P2] 5.1-INTG-004: tree dump of multipage.pdf does not contain any 'page' label")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-009 [P2]: Run `go list -deps ./cmd/cli/` from project root,
// verify no dependency contains "wails" in the import path.
// AC#2: CLI binary includes only pdfcore and CLI logic (no Wails dependency).
// ---------------------------------------------------------------------------

func TestCLI_NoWailsImports(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "list", "-deps", "./cmd/cli/")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P2] 5.1-UNIT-009: go list -deps failed: %v\n%s", err, string(output))
	}

	for line := range strings.SplitSeq(string(output), "\n") {
		if strings.Contains(line, "wails") {
			t.Errorf("[P2] 5.1-UNIT-009: CLI binary depends on Wails package: %s", line)
		}
	}
}

// ---------------------------------------------------------------------------
// 5.1-BENCH-001 [P2]: Tree dump of testdata/multipage.pdf completes in
// under 5 seconds. Performance guard for recursive tree traversal
// (risk E5-R-001).
// AC#5: Recursive tree traversal completes in reasonable time.
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
	binPath := filepath.Join(tmpDir, "pdfdebug")
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
			b.Fatalf("[P2] 5.1-BENCH-001: tree dump failed: %v\n%s", err, string(output))
		}
	}
}

// ---------------------------------------------------------------------------
// 5.1-BUILD-001 [P3]: Build CLI binary, verify file size < 25MB.
// AC#2 originally targets 10MB, but pdfcpu alone pulls ~19MB of Go code.
// The realistic budget is 25MB until pdfcpu is replaced or trimmed.
// ---------------------------------------------------------------------------

func TestCLI_BinarySizeUnder25MB(t *testing.T) {
	bin := buildCLI(t)

	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("[P3] 5.1-BUILD-001: cannot stat binary: %v", err)
	}

	const maxSize = 25 * 1024 * 1024 // 25MB -- pdfcpu pulls in ~19MB of Go code
	if info.Size() > maxSize {
		t.Errorf("[P3] 5.1-BUILD-001: binary size %d bytes exceeds 25MB limit", info.Size())
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-010 [P3]: Run `pdfdebug` with no subcommand, verify usage on
// stderr, exit code 1. Friendly error for bare invocation.
// AC#2: --help shows clear usage; no-args should also guide the user.
// ---------------------------------------------------------------------------

func TestNoSubcommand_PrintsUsage_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin)

	if exitCode != 1 {
		t.Errorf("[P3] 5.1-UNIT-010: expected exit code 1 for no subcommand, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P3] 5.1-UNIT-010: stdout should be empty for usage errors, got: %s", stdout)
	}

	combined := stdout + stderr
	if !strings.Contains(combined, "dump") {
		t.Error("[P3] 5.1-UNIT-010: usage output does not mention 'dump' command")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-011 [P2]: Negative --depth value produces JSON error on stderr,
// exit code 1 (usage error).
// AC#5: --depth semantics. Negative values are rejected as invalid input.
// ---------------------------------------------------------------------------

func TestTreeDump_NegativeDepth_Error(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "tree", "--depth", "-1", pdfPath)

	if exitCode != 1 {
		t.Errorf("[P2] 5.1-UNIT-011: expected exit code 1 for negative depth, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.1-UNIT-011: stdout should be empty for error cases, got: %s", stdout)
	}

	// stderr should contain a JSON error mentioning depth
	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.1-UNIT-011: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	if msg, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.1-UNIT-011: stderr JSON missing 'error' key")
	} else if !strings.Contains(strings.ToLower(msg), "depth") {
		t.Errorf("[P2] 5.1-UNIT-011: error message should mention depth, got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-012 [P2]: `pdfdebug dump object` stub returns JSON error on stderr
// and exit code 1.
// AC#2: Stubs for unimplemented commands report "not implemented".
// ---------------------------------------------------------------------------

func TestObjectDumpStub_ReturnsNotImplemented(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "1 0 R", "dummy.pdf")

	if exitCode != 1 {
		t.Errorf("[P2] 5.1-UNIT-012: expected exit code 1 for stub, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.1-UNIT-012: stdout should be empty for stub, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.1-UNIT-012: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	if msg, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.1-UNIT-012: stderr JSON missing 'error' key")
	} else if !strings.Contains(strings.ToLower(msg), "not implemented") {
		t.Errorf("[P2] 5.1-UNIT-012: expected 'not implemented' error, got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-013 [P2]: `pdfdebug dump stream` stub returns JSON error on stderr
// and exit code 1.
// AC#2: Stubs for unimplemented commands report "not implemented".
// ---------------------------------------------------------------------------

func TestStreamDumpStub_ReturnsNotImplemented(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", "--page", "1", "dummy.pdf")

	if exitCode != 1 {
		t.Errorf("[P2] 5.1-UNIT-013: expected exit code 1 for stub, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.1-UNIT-013: stdout should be empty for stub, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.1-UNIT-013: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	if msg, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.1-UNIT-013: stderr JSON missing 'error' key")
	} else if !strings.Contains(strings.ToLower(msg), "not implemented") {
		t.Errorf("[P2] 5.1-UNIT-013: expected 'not implemented' error, got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-014 [P2]: `pdfdebug dump` with no resource prints usage to stderr,
// exit code 1.
// AC#3: Missing subcommand arguments produce usage error.
// ---------------------------------------------------------------------------

func TestDumpNoResource_PrintsUsage_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump")

	if exitCode != 1 {
		t.Errorf("[P2] 5.1-UNIT-014: expected exit code 1 for missing resource, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.1-UNIT-014: stdout should be empty, got: %s", stdout)
	}

	if strings.TrimSpace(stderr) == "" {
		t.Error("[P2] 5.1-UNIT-014: stderr should contain usage information")
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-015 [P2]: `pdfdebug dump unknown` prints error + usage to stderr,
// exit code 1.
// AC#3: Unknown resource names produce usage error.
// ---------------------------------------------------------------------------

func TestDumpUnknownResource_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "unknown")

	if exitCode != 1 {
		t.Errorf("[P2] 5.1-UNIT-015: expected exit code 1 for unknown resource, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.1-UNIT-015: stdout should be empty, got: %s", stdout)
	}

	if !strings.Contains(stderr, "Unknown resource") {
		t.Errorf("[P2] 5.1-UNIT-015: stderr should mention unknown resource\nstderr: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// 5.1-UNIT-016 [P3]: `pdfdebug garbage` (unknown top-level command) prints
// error + usage to stderr, exit code 1.
// AC#2: Unknown commands guide the user with usage information.
// ---------------------------------------------------------------------------

func TestUnknownCommand_ExitCode1(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "garbage")

	if exitCode != 1 {
		t.Errorf("[P3] 5.1-UNIT-016: expected exit code 1 for unknown command, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P3] 5.1-UNIT-016: stdout should be empty, got: %s", stdout)
	}

	if !strings.Contains(stderr, "Unknown command") {
		t.Errorf("[P3] 5.1-UNIT-016: stderr should mention unknown command\nstderr: %s", stderr)
	}

	// Should still show usage guidance
	if !strings.Contains(stderr, "dump") {
		t.Errorf("[P3] 5.1-UNIT-016: usage text should mention 'dump'\nstderr: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// 5.1-INTG-005 [P1]: Tree dump without --json flag still outputs JSON.
// AC#1: --json is accepted for explicitness but JSON is always the output
//       format. Omitting --json still produces JSON output.
// ---------------------------------------------------------------------------

func TestTreeDump_WithoutJSONFlag_StillOutputsJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// Run WITHOUT --json flag
	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.1-INTG-005: expected exit code 0, got %d", exitCode)
	}

	if !json.Valid([]byte(stdout)) {
		t.Fatalf("[P1] 5.1-INTG-005: stdout without --json flag is not valid JSON\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.1-INTG-006 [P1]: Tree dump with --depth flag limits traversal.
// AC#5: Given a large PDF, When --depth N is passed, Then recursive tree
//       traversal stops at depth N.
// ---------------------------------------------------------------------------

func TestTreeDump_DepthFlag_LimitsTraversal(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// Run with --depth 1: should have root + immediate children, but children
	// should NOT have their own children expanded.
	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--depth", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.1-INTG-006: expected exit code 0, got %d", exitCode)
	}

	var root map[string]any
	mustParseJSON(t, stdout, &root)

	// Root should have children (depth 1 = one level below root)
	children, ok := root["children"]
	if !ok {
		t.Fatal("[P1] 5.1-INTG-006: root has no 'children' at depth 1")
	}

	childArr, ok := children.([]any)
	if !ok || len(childArr) == 0 {
		t.Fatal("[P1] 5.1-INTG-006: root children empty at depth 1")
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
				t.Error("[P1] 5.1-INTG-006: --depth 1 should not expand grandchildren, but found nested children")
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 5.1-INTG-007 [P1]: Tree dump with --depth 0 (unlimited) includes full
// recursive tree (same as omitting --depth).
// AC#5: When omitted, traversal is unlimited (depth 0 = unlimited).
// ---------------------------------------------------------------------------

func TestTreeDump_DepthZero_UnlimitedTraversal(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// --depth 0 should behave same as no --depth flag (unlimited)
	stdoutDepth0, _, exitCode0 := runCLI(t, bin, "dump", "tree", "--depth", "0", pdfPath)
	if exitCode0 != 0 {
		t.Fatalf("[P1] 5.1-INTG-007: expected exit code 0 with --depth 0, got %d", exitCode0)
	}

	stdoutNoDepth, _, exitCodeND := runCLI(t, bin, "dump", "tree", pdfPath)
	if exitCodeND != 0 {
		t.Fatalf("[P1] 5.1-INTG-007: expected exit code 0 without --depth, got %d", exitCodeND)
	}

	// Both should produce the same output
	if stdoutDepth0 != stdoutNoDepth {
		t.Error("[P1] 5.1-INTG-007: --depth 0 output differs from no-depth output (expected identical)")
	}
}
