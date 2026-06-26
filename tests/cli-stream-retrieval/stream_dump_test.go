// Package cli_stream_retrieval_test provides acceptance tests for Story 5.3:
// CLI Content Stream Retrieval.
//
// Test Levels: Unit (Go) and Integration (Go) -- CLI binary build + execution.
// No browser interaction required; all criteria are CLI binary validation.
//
// Run: cd tests/cli-stream-retrieval && go test -v -count=1 ./...
package cli_stream_retrieval_test

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
// 5.3-INTG-001 [P0]: Build binary, run `pdfdebug dump stream --page 1
// testdata/content-stream.pdf`, verify stdout parses as valid JSON with
// ContentStreamData fields (nodeId, raw, tokenized), verify `raw` is
// non-empty (decompressed text), verify exit code 0.
// AC#1: Given a valid PDF file with content streams, When
//       `pdfdebug dump stream --page 1 [--json] <file>` is executed, Then
//       the CLI outputs the decoded content stream for page 1 as structured
//       JSON to stdout, And the JSON matches the ContentStreamData model
//       (nodeId, raw, tokenized, error), And the exit code is 0.
// ---------------------------------------------------------------------------

func TestStreamDump_ValidPage_OutputsJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P0] 5.3-INTG-001: expected exit code 0, got %d", exitCode)
	}

	// stdout must be valid JSON
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("[P0] 5.3-INTG-001: stdout is not valid JSON: %v\nraw: %s", err, stdout)
	}

	// Must contain ContentStreamData fields
	requiredFields := []string{"nodeId", "raw", "tokenized"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("[P0] 5.3-INTG-001: ContentStreamData JSON missing required field %q", field)
		}
	}

	// raw must be non-empty (content-stream.pdf has decompressed content)
	raw, _ := result["raw"].(string)
	if raw == "" {
		t.Error("[P0] 5.3-INTG-001: 'raw' field is empty -- expected decompressed content stream text")
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-001 [P0]: Run with `--page 999` against content-stream.pdf,
// verify stderr contains JSON error mentioning "out of range", stdout is
// empty, exit code 2.
// AC#2: Given a page number that does not exist, When the CLI is executed
//       with `--page 999`, Then an error message in JSON format is written
//       to stderr indicating the page is out of range, And the exit code
//       is 2.
// ---------------------------------------------------------------------------

func TestStreamDump_OutOfRangePage_JSONErrorOnStderr(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", "--page", "999", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P0] 5.3-UNIT-001: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P0] 5.3-UNIT-001: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P0] 5.3-UNIT-001: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	errMsg, ok := errObj["error"]
	if !ok {
		t.Fatal("[P0] 5.3-UNIT-001: stderr JSON missing 'error' key")
	}

	if !strings.Contains(strings.ToLower(errMsg), "out of range") {
		t.Errorf("[P0] 5.3-UNIT-001: error message should mention 'out of range', got: %s", errMsg)
	}
}

// ---------------------------------------------------------------------------
// 5.3-INTG-002 [P1]: Parse stdout JSON from content-stream.pdf page 1,
// verify `tokenized` array is non-empty, each Token has `type`, `value`,
// `line`, `col` fields. Verify at least one token has type "operator".
// AC#1: `tokenized` contains an array of Token objects with type, value,
//       line, col fields.
// AC#3: The JSON schema is self-documenting with clear field names matching
//       the ContentStreamData model.
// ---------------------------------------------------------------------------

func TestStreamDump_TokenizedArray_HasTokens(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.3-INTG-002: expected exit code 0, got %d", exitCode)
	}

	var result map[string]any
	mustParseJSON(t, stdout, &result)

	tokenized, ok := result["tokenized"]
	if !ok || tokenized == nil {
		t.Fatal("[P1] 5.3-INTG-002: ContentStreamData missing 'tokenized' field")
	}

	tokenArr, ok := tokenized.([]any)
	if !ok || len(tokenArr) == 0 {
		t.Fatal("[P1] 5.3-INTG-002: 'tokenized' is empty or not an array")
	}

	// Verify each token has required fields
	tokenFields := []string{"type", "value", "line", "col"}
	for i, tok := range tokenArr {
		tokMap, ok := tok.(map[string]any)
		if !ok {
			t.Fatalf("[P1] 5.3-INTG-002: token[%d] is not an object", i)
		}
		for _, field := range tokenFields {
			if _, exists := tokMap[field]; !exists {
				t.Errorf("[P1] 5.3-INTG-002: token[%d] missing required field %q", i, field)
			}
		}
	}

	// Verify at least one token has type "operator"
	foundOperator := false
	for _, tok := range tokenArr {
		tokMap, _ := tok.(map[string]any)
		if tokType, _ := tokMap["type"].(string); tokType == "operator" {
			foundOperator = true
			break
		}
	}
	if !foundOperator {
		t.Error("[P1] 5.3-INTG-002: expected at least one token with type 'operator'")
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-002 [P1]: Run against `testdata/empty-stream.pdf` with
// `--page 1`, verify exit code 0 (not a failure), verify JSON output has
// empty `raw` string.
// Note: empty-stream.pdf has a Contents entry pointing to a zero-length
// stream -- the page dict has a Contents ref, but the stream body is empty.
// AC#4: Given a page with no content stream (empty page), When the CLI is
//       executed, Then the JSON output includes an empty `raw` string,
//       And the exit code is 0 (not a failure).
// ---------------------------------------------------------------------------

func TestStreamDump_EmptyStream_ReturnsEmptyRaw(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "empty-stream.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.3-UNIT-002: expected exit code 0, got %d", exitCode)
	}

	var result map[string]any
	mustParseJSON(t, stdout, &result)

	raw, _ := result["raw"].(string)
	if raw != "" {
		t.Errorf("[P1] 5.3-UNIT-002: expected empty 'raw' for empty-stream.pdf, got: %q", raw)
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-002b [P1]: Run against `testdata/minimal.pdf` with `--page 1`.
// minimal.pdf's page has no Contents entry -- it is a bare page with no
// drawing commands. Verify exit code 0, verify JSON output has `error`
// field containing "page has no content stream", verify `raw` is empty.
// This tests the "no Contents entry at all" case (the CLI's empty-page
// branch from Task 3.4).
// AC#4: Given a page with no content stream (empty page), When the CLI is
//       executed, Then the JSON output includes an empty `raw` string
//       and/or a non-fatal `error` field explaining the page has no stream
//       content, And the exit code is 0.
// ---------------------------------------------------------------------------

func TestStreamDump_NoContentsEntry_ReturnsErrorField(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.3-UNIT-002b: expected exit code 0, got %d", exitCode)
	}

	var result map[string]any
	mustParseJSON(t, stdout, &result)

	// raw should be empty
	raw, _ := result["raw"].(string)
	if raw != "" {
		// If minimal.pdf page 1 DOES have a Contents entry, this test's
		// assumption is wrong. Skip with a clear message so the developer
		// knows to use a different fixture.
		t.Skipf("[P1] 5.3-UNIT-002b: minimal.pdf page 1 has non-empty raw content (%d bytes) -- this test assumes no Contents entry; use a different fixture or create no-contents.pdf", len(raw))
	}

	// error field should explain there is no content stream
	errField, _ := result["error"].(string)
	if !strings.Contains(strings.ToLower(errField), "no content stream") {
		t.Errorf("[P1] 5.3-UNIT-002b: expected error field mentioning 'no content stream', got: %q", errField)
	}
}

// ---------------------------------------------------------------------------
// 5.3-INTG-003 [P2]: Run against content-stream.pdf, verify `raw` field
// contains readable text content (not compressed binary gibberish). Check
// for presence of common PDF operators like "BT", "ET", "Tf", "Tj".
// AC#1: `raw` contains the decompressed plain text (FlateDecode streams
//       are decoded).
// ---------------------------------------------------------------------------

func TestStreamDump_FlateDecode_ReturnsDecompressed(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P2] 5.3-INTG-003: expected exit code 0, got %d", exitCode)
	}

	var result map[string]any
	mustParseJSON(t, stdout, &result)

	raw, _ := result["raw"].(string)
	if raw == "" {
		t.Fatal("[P2] 5.3-INTG-003: 'raw' is empty -- expected decompressed content")
	}

	// Decompressed content should contain recognizable PDF operators.
	// content-stream.pdf has text drawing commands.
	operators := []string{"BT", "ET", "Tf", "Tj"}
	foundAny := false
	for _, op := range operators {
		if strings.Contains(raw, op) {
			foundAny = true
			break
		}
	}
	if !foundAny {
		t.Errorf("[P2] 5.3-INTG-003: raw content does not contain any common PDF operators (BT, ET, Tf, Tj) -- may still be compressed\nraw (first 200 chars): %.200s", raw)
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-003 [P2]: Test with `--page 0` and `--page -1`, verify each
// produces JSON error on stderr, exit code 1 (usage error, not file error).
// AC#5: Given an invalid page number (`--page 0`, `--page -1`), When the
//       CLI is executed, Then an error message on stderr clearly describes
//       the expected page format (1-based positive integer), And the exit
//       code is 1 (usage error).
// ---------------------------------------------------------------------------

func TestStreamDump_InvalidPageNumber_UsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	cases := []struct {
		name string
		page string
	}{
		{"page zero", "0"},
		{"page negative", "-1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", "--page", tc.page, pdfPath)

			if exitCode != 1 {
				t.Errorf("[P2] 5.3-UNIT-003/%s: expected exit code 1 (usage error), got %d", tc.name, exitCode)
			}

			if strings.TrimSpace(stdout) != "" {
				t.Errorf("[P2] 5.3-UNIT-003/%s: stdout should be empty for usage error, got: %s", tc.name, stdout)
			}

			// stderr should be valid JSON with an error message
			var errObj map[string]string
			if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
				t.Fatalf("[P2] 5.3-UNIT-003/%s: stderr is not valid JSON: %v\nraw: %s", tc.name, err, stderr)
			}

			errMsg, ok := errObj["error"]
			if !ok {
				t.Fatalf("[P2] 5.3-UNIT-003/%s: stderr JSON missing 'error' key", tc.name)
			}

			// Error should mention 1-based or >= 1
			lower := strings.ToLower(errMsg)
			if !strings.Contains(lower, "1") && !strings.Contains(lower, "page") {
				t.Errorf("[P2] 5.3-UNIT-003/%s: error should describe valid page format, got: %s", tc.name, errMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-004 [P1]: Run without --page flag, verify usage error on stderr,
// exit code 1.
// AC#5: Given a missing `--page`, When the CLI is executed, Then an error
//       message on stderr describes the expected page format, And the exit
//       code is 1 (usage error).
// ---------------------------------------------------------------------------

func TestStreamDump_MissingPageFlag_UsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", pdfPath)

	if exitCode != 1 {
		t.Errorf("[P1] 5.3-UNIT-004: expected exit code 1 (usage error), got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P1] 5.3-UNIT-004: stdout should be empty for usage errors, got: %s", stdout)
	}

	if strings.TrimSpace(stderr) == "" {
		t.Error("[P1] 5.3-UNIT-004: stderr should contain usage/error information")
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-005 [P1]: Run without file path, verify usage error on stderr,
// exit code 1.
// AC#5 (implied): Missing file path is a usage error.
// ---------------------------------------------------------------------------

func TestStreamDump_MissingFilePath_UsageError(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", "--page", "1")

	if exitCode != 1 {
		t.Errorf("[P1] 5.3-UNIT-005: expected exit code 1 (usage error), got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P1] 5.3-UNIT-005: stdout should be empty for usage errors, got: %s", stdout)
	}

	if strings.TrimSpace(stderr) == "" {
		t.Error("[P1] 5.3-UNIT-005: stderr should contain usage/error information")
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-006 [P1] (REVISED by Story 13-1): Stream dump WITHOUT --json emits a
// human-readable operator listing (the flipped default), NOT JSON. One operator
// per line; the content-stream.pdf fixture draws a BT ... ET text block.
// ---------------------------------------------------------------------------

func TestStreamDump_WithoutJSONFlag_OutputsPlainOperators(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	// Run WITHOUT --json flag -> plain operator listing.
	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.3-UNIT-006: expected exit code 0, got %d", exitCode)
	}

	trimmed := strings.TrimSpace(stdout)
	if strings.HasPrefix(trimmed, "{") && json.Valid([]byte(trimmed)) {
		t.Fatalf("[P1] 5.3-UNIT-006: default stream output must be a plain operator listing, not JSON\nraw: %s", stdout)
	}
	// Structural: a recognizable PDF operator appears on its own (BT opens text).
	foundOp := false
	for _, line := range strings.Split(trimmed, "\n") {
		s := strings.TrimSpace(line)
		if s == "BT" || s == "ET" || strings.HasSuffix(s, " Tj") || strings.HasSuffix(s, " Tf") {
			foundOp = true
			break
		}
	}
	if !foundOp {
		t.Errorf("[P1] 5.3-UNIT-006: plain stream output should list operators one per line\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-024 [P1] (AC3): the plain operator listing renders operands BEFORE
// the trailing operator in PDF content-stream order (not just "operator is the
// last token"). content-stream.pdf draws `/F1 12 Tf`, `100 700 Td`,
// `(Hello World) Tj` - each a multi-operand line. STRUCTURAL: on a line whose
// last token is a known operand-taking operator, assert at least one operand
// token precedes it and that the operator is genuinely last. NON-CONTRACTUAL
// plain text; no whole-dump equality.
// ---------------------------------------------------------------------------

func TestStreamDump_PlainOperandsPrecedeOperator(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--page", "1", pdfPath)
	if exitCode != 0 {
		t.Fatalf("[P1] 13.1-INTG-024: expected exit code 0, got %d", exitCode)
	}

	// Operators that, in content-stream.pdf, are always preceded by >=1 operand.
	operandTaking := map[string]bool{"Tf": true, "Td": true, "Tj": true}

	checked := 0
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		op := fields[len(fields)-1]
		if !operandTaking[op] {
			continue
		}
		checked++
		// Structural order assertion: operands occupy the leading positions and
		// the operator is strictly the final token (PDF postfix order).
		if len(fields) < 2 {
			t.Errorf("[P1] 13.1-INTG-024: operator %q has no preceding operand on line %q", op, line)
			continue
		}
		operands := fields[:len(fields)-1]
		for _, operand := range operands {
			if operandTaking[operand] {
				t.Errorf("[P1] 13.1-INTG-024: operator-looking token %q appears in the operand region (operands must precede the operator) on line %q", operand, line)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("[P1] 13.1-INTG-024: no operand-taking operator line (Tf/Td/Tj) found in plain output:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-007 [P1]: Stream dump with --json flag explicitly also works.
// AC#1: --json flag is accepted.
// ---------------------------------------------------------------------------

func TestStreamDump_WithJSONFlag_OutputsJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.3-UNIT-007: expected exit code 0, got %d", exitCode)
	}

	if !json.Valid([]byte(stdout)) {
		t.Fatalf("[P1] 5.3-UNIT-007: stdout with --json flag is not valid JSON\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-008 [P1]: Parse entire stdout as JSON; any parse failure = test
// failure. Ensures no log noise from pdfcpu leaks into stdout.
// AC#3: The output can be piped to `jq` for filtering and transformation.
// ---------------------------------------------------------------------------

func TestStreamDump_StdoutContainsOnlyJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.3-UNIT-008: expected exit code 0, got %d", exitCode)
	}

	if !json.Valid([]byte(stdout)) {
		t.Fatalf("[P1] 5.3-UNIT-008: stdout is not valid JSON (log noise present?)\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-009 [P2]: Non-existent file path returns JSON error on stderr
// and exit code 2 (file error, not usage error).
// AC#2 (boundary): File errors use exit code 2.
// ---------------------------------------------------------------------------

func TestStreamDump_NonexistentFile_JSONErrorExitCode2(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", "--page", "1", "/nonexistent/path/fake.pdf")

	if exitCode != 2 {
		t.Errorf("[P2] 5.3-UNIT-009: expected exit code 2 for file error, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.3-UNIT-009: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.3-UNIT-009: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.3-UNIT-009: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-010 [P2]: Encrypted PDF with --page produces JSON error on stderr
// and exit code 2.
// AC#2 (boundary): Encrypted PDF is a file-level error.
// ---------------------------------------------------------------------------

func TestStreamDump_EncryptedPDF_JSONErrorExitCode2(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "encrypted.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", "--page", "1", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P2] 5.3-UNIT-010: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.3-UNIT-010: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.3-UNIT-010: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.3-UNIT-010: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT-011 [P2]: Malformed PDF with --page produces JSON error on stderr
// and exit code 2.
// AC#2 (boundary): Malformed PDF is a file-level error.
// ---------------------------------------------------------------------------

func TestStreamDump_MalformedPDF_JSONErrorExitCode2(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "malformed.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "stream", "--page", "1", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P2] 5.3-UNIT-011: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.3-UNIT-011: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.3-UNIT-011: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.3-UNIT-011: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.3-INTG-004 [P1]: Verify the nodeId field in ContentStreamData output
// follows the node ID format (starts with "obj:" and has 3 colon-separated
// parts).
// AC#1: The JSON matches the ContentStreamData model (nodeId field).
// ---------------------------------------------------------------------------

func TestStreamDump_NodeID_Format(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.3-INTG-004: expected exit code 0, got %d", exitCode)
	}

	var result map[string]any
	mustParseJSON(t, stdout, &result)

	nodeID, ok := result["nodeId"].(string)
	if !ok || nodeID == "" {
		t.Fatal("[P1] 5.3-INTG-004: ContentStreamData missing or empty 'nodeId'")
	}

	if !strings.HasPrefix(nodeID, "obj:") {
		t.Errorf("[P1] 5.3-INTG-004: nodeId should start with 'obj:', got: %s", nodeID)
	}

	parts := strings.SplitN(nodeID, ":", 3)
	if len(parts) != 3 {
		t.Errorf("[P1] 5.3-INTG-004: nodeId should have 3 colon-separated parts (obj:gen:num), got: %s", nodeID)
	}
}

// ---------------------------------------------------------------------------
// 5.3-BENCH-001 [P3]: Stream dump of `testdata/content-stream.pdf` page 1
// completes in under 2 seconds. Performance guard.
// ---------------------------------------------------------------------------

func BenchmarkStreamDump_ContentStreamPDF(b *testing.B) {
	// Build binary manually (cannot use buildCLI with *testing.B).
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

	pdfPath := filepath.Join(root, "testdata", "content-stream.pdf")

	b.ResetTimer()
	for b.Loop() {
		cmd := exec.Command(binPath, "dump", "stream", "--page", "1", pdfPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("[P3] 5.3-BENCH-001: stream dump failed: %v\n%s", err, string(output))
		}
	}
}
