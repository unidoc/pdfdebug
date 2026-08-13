// Story 11.4: Expose existing pdfcore views as CLI commands -- RED PHASE.
//
// Document-level presenters (no --ref): xref, objects, plaintext. These MUST
// FAIL against the current binary until Story 11-4 wires the new resources
// into the dispatch switch. Black-box: build the CLI, run as a subprocess.
//
// Test level: Integration (Go) -- CLI binary build + execution. No browser.
//
// Covers: (xref + objects JSON), (plaintext raw byte-exact + --json wrapper
// without the tabId leak).
//
// Run: cd tests/cli-views && go test -v -count=1 ./...
package cli_views_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// `dump xref minimal.pdf` emits the XRefTable JSON (tabId + entries[]) with
// exit 0. Each entry carries objNum/gen/status, and the nodeID JSON tag is
// capital-D "nodeID" (NOT "nodeId") -- verbatim from the XRefEntry struct.
// ---------------------------------------------------------------------------

func TestXRefDump_ValidPDF_OutputsXRefTableJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "xref", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var table struct {
		TabID   string                     `json:"tabId"`
		Entries []map[string]json.RawMessage `json:"entries"`
	}
	mustParseJSON(t, stdout, &table)

	if len(table.Entries) == 0 {
		t.Fatalf("expected at least one xref entry")
	}
	for _, key := range []string{"objNum", "gen", "status", "offset", "nodeID"} {
		if _, ok := table.Entries[0][key]; !ok {
			t.Errorf("XRefEntry missing key %q (note: nodeID is capital-D)", key)
		}
	}
}

// ---------------------------------------------------------------------------
// `dump objects minimal.pdf` emits the []ObjectIndexEntry JSON array (one
// entry per object) with exit 0. Each entry carries
// objNum/gen/typeName/free/reachable/nodeId. Distinct from the singular `dump
// object` (which requires --ref).
// ---------------------------------------------------------------------------

func TestObjectsDump_ValidPDF_OutputsObjectIndexArray(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "objects", "--json", pdfPath)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var index []map[string]json.RawMessage
	mustParseJSON(t, stdout, &index)

	if len(index) == 0 {
		t.Fatalf("expected at least one object index entry")
	}
	for _, key := range []string{"objNum", "gen", "typeName", "free", "reachable", "nodeId"} {
		if _, ok := index[0][key]; !ok {
			t.Errorf("ObjectIndexEntry missing key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// `dump objects` (plural -> index, no --ref) and `dump object` (singular
// -> single object, requires --ref) are distinct
// commands with distinct output. dump object WITHOUT --ref is a usage error;
// dump objects WITHOUT --ref succeeds. Guards the one-character naming trap.
// ---------------------------------------------------------------------------

func TestObjectsVsObject_DistinctCommands(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// Plural: succeeds without --ref, returns a JSON array (--json).
	pluralOut, _, ecPlural := runCLI(t, bin, "dump", "objects", "--json", pdfPath)
	if ecPlural != 0 {
		t.Fatalf("`dump objects` expected exit 0, got %d", ecPlural)
	}
	var arr []any
	mustParseJSON(t, pluralOut, &arr)

	// Singular: without --ref is a usage error (exit 1).
	_, _, ecSingular := runCLI(t, bin, "dump", "object", pdfPath)
	if ecSingular != 1 {
		t.Errorf("`dump object` without --ref expected exit 1 (usage), got %d", ecSingular)
	}
}

// ---------------------------------------------------------------------------
// `dump plaintext <file>` (default) writes the document text to stdout, NOT
// JSON-wrapped, and Latin-1-re-encoded so the stdout bytes equal the source
// file bytes byte-for-byte. A naive UTF-8 string write would corrupt every
// byte >= 0x80, so byte-exact equality is the gate.
//
// Fixture choice: image-xobject.pdf embeds DCTDecode (JPEG) stream bytes that
// include bytes >= 0x80, which is exactly what surfaces the re-encoding trap.
// ---------------------------------------------------------------------------

func TestPlaintextDump_Default_ByteExactSource(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "image-xobject.pdf")

	stdout, _, exitCode := runCLIRaw(t, bin, "dump", "plaintext", pdfPath)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	want, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("failed to read source fixture: %v", err)
	}

	if !bytes.Equal(stdout, want) {
		t.Errorf("plaintext stdout does not equal source bytes byte-for-byte (Latin-1 re-encode trap?)\n got %d bytes, want %d bytes", len(stdout), len(want))
	}
}

// ---------------------------------------------------------------------------
// `dump plaintext --json <file>` wraps the document as EXACTLY
// {"totalBytes","content"} -- the tabId field is NOT included (it is a
// CLI-internal artifact). totalBytes equals the on-disk file size.
// ---------------------------------------------------------------------------

func TestPlaintextDump_JSON_WrapsWithoutTabID(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "plaintext", "--json", pdfPath)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var raw map[string]json.RawMessage
	mustParseJSON(t, stdout, &raw)

	if _, leaked := raw["tabId"]; leaked {
		t.Errorf("--json wrapper must NOT include the tabId field")
	}
	for _, key := range []string{"totalBytes", "content"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("--json wrapper missing required key %q", key)
		}
	}
	if len(raw) != 2 {
		t.Errorf("--json wrapper should have exactly 2 keys {totalBytes, content}, got %d", len(raw))
	}

	// totalBytes must equal the on-disk size.
	info, err := os.Stat(pdfPath)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	var totalBytes int64
	_ = json.Unmarshal(raw["totalBytes"], &totalBytes)
	if totalBytes != info.Size() {
		t.Errorf("totalBytes %d != file size %d", totalBytes, info.Size())
	}
}

// ---------------------------------------------------------------------------
// document-level presenters surface genuine Go errors as JSON on stderr
// with exit 2 (nonexistent file), empty stdout.
// ---------------------------------------------------------------------------

func TestDocumentViews_NonexistentFile_JSONErrorExit2(t *testing.T) {
	bin := buildCLI(t)

	for _, resource := range []string{"xref", "objects", "plaintext"} {
		t.Run(resource, func(t *testing.T) {
			stdout, stderr, ec := runCLI(t, bin, "dump", resource, "/nonexistent/path/fake.pdf")
			if ec != 2 {
				t.Errorf("%s: expected exit code 2, got %d", resource, ec)
			}
			if len(stdout) != 0 {
				t.Errorf("%s: stdout should be empty on error, got: %s", resource, stdout)
			}
			var errObj map[string]string
			if err := json.Unmarshal([]byte(trimSpace(stderr)), &errObj); err != nil {
				t.Fatalf("%s: stderr is not valid JSON: %v\nraw: %s", resource, err, stderr)
			}
			if _, ok := errObj["error"]; !ok {
				t.Errorf("%s: stderr JSON missing 'error' key", resource)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 13.1: plain-text default siblings. WITHOUT --json, dump xref / dump objects
// emit a human-readable aligned table (header row + data rows), NOT JSON.
// Assertions are STRUCTURAL (header tokens + row count >= 1), never whole-dump
// equality.
// ---------------------------------------------------------------------------

func TestXRefDump_PlainTextDefault(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "xref", pdfPath)
	if exitCode != 0 {
		t.Fatalf("[13.1] xref plain: expected exit code 0, got %d", exitCode)
	}
	if json.Valid([]byte(trimSpace(stdout))) && (len(stdout) > 0 && (stdout[0] == '{' || stdout[0] == '[')) {
		t.Fatalf("[13.1] xref plain: default output must be plain text, not JSON:\n%.200s", stdout)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) < 2 {
		t.Fatalf("[13.1] xref plain: expected a header row + at least one data row, got %d lines:\n%s", len(lines), stdout)
	}
	for _, col := range []string{"OBJ", "GEN", "TYPE"} {
		if !contains(lines[0], col) {
			t.Errorf("[13.1] xref plain: header row missing column %q\nheader: %s", col, lines[0])
		}
	}
}

func TestObjectsDump_PlainTextDefault(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "objects", pdfPath)
	if exitCode != 0 {
		t.Fatalf("[13.1] objects plain: expected exit code 0, got %d", exitCode)
	}
	if json.Valid([]byte(trimSpace(stdout))) && (len(stdout) > 0 && (stdout[0] == '{' || stdout[0] == '[')) {
		t.Fatalf("[13.1] objects plain: default output must be plain text, not JSON:\n%.200s", stdout)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) < 2 {
		t.Fatalf("[13.1] objects plain: expected a header row + at least one data row, got %d lines:\n%s", len(lines), stdout)
	}
	for _, col := range []string{"OBJ", "GEN", "TYPE"} {
		if !contains(lines[0], col) {
			t.Errorf("[13.1] objects plain: header row missing column %q\nheader: %s", col, lines[0])
		}
	}
}

// nonEmptyLines splits s into its non-blank lines (small local helper).
func nonEmptyLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			if trimSpace(line) != "" {
				out = append(out, line)
			}
			start = i + 1
		}
	}
	return out
}

// contains reports whether sub appears in s (avoids importing strings for one use).
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// trimSpace is a tiny local helper to avoid importing strings just for one use.
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\n' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\n' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
