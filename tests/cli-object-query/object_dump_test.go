// Package cli_object_query_test provides acceptance tests for Story 5.2:
// CLI Object Query by Reference.
//
// Test Levels: Unit (Go) and Integration (Go) -- CLI binary build + execution.
// No browser interaction required; all criteria are CLI binary validation.
//
// Run: cd tests/cli-object-query && go test -v -count=1 ./...
package cli_object_query_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper: discover a valid object ref from the tree dump output.
// Runs `pdfdebug dump tree <file>`, walks the JSON tree to find an obj: node,
// and converts it back to "N G R" format.
// ---------------------------------------------------------------------------

// treeNode is a minimal representation of the TreeNode JSON for discovery.
type treeNode struct {
	ID       string     `json:"id"`
	Children []treeNode `json:"children"`
}

// findObjNodeID walks a tree looking for a node whose ID starts with "obj:".
// Returns the first obj: node ID found, or "" if none.
func findObjNodeID(node treeNode) string {
	if strings.HasPrefix(node.ID, "obj:") {
		return node.ID
	}
	for _, child := range node.Children {
		if id := findObjNodeID(child); id != "" {
			return id
		}
	}
	return ""
}

// nodeIDToRef converts "obj:{gen}:{num}" to "{num} {gen} R".
func nodeIDToRef(nodeID string) (string, error) {
	// Format: obj:G:N
	parts := strings.SplitN(nodeID, ":", 3)
	if len(parts) != 3 || parts[0] != "obj" {
		return "", fmt.Errorf("unexpected node ID format: %s", nodeID)
	}
	gen := parts[1]
	num := parts[2]
	return fmt.Sprintf("%s %s R", num, gen), nil
}

// discoverValidRef runs tree dump and finds a valid object reference.
// Returns the ref string (e.g., "3 0 R") and the corresponding node ID.
func discoverValidRef(t *testing.T, bin, pdfPath string) (ref, nodeID string) {
	t.Helper()
	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)
	if exitCode != 0 {
		t.Fatalf("tree dump failed with exit code %d (needed for discovery)", exitCode)
	}

	var root treeNode
	mustParseJSON(t, stdout, &root)

	nodeID = findObjNodeID(root)
	if nodeID == "" {
		t.Fatal("no obj: node found in tree dump output")
	}

	ref, err := nodeIDToRef(nodeID)
	if err != nil {
		t.Fatalf("failed to convert node ID to ref: %v", err)
	}
	return ref, nodeID
}

// ---------------------------------------------------------------------------
// 5.2-INTG-001 [P0]: Build binary, discover a valid object reference from
// minimal.pdf via tree dump, run `pdfdebug dump object --ref "N G R"
// testdata/minimal.pdf`, verify stdout parses as valid JSON with
// ObjectDetail fields (nodeId, objectRef, type), exit code 0.
// AC#1: Given a valid PDF file and an object reference, When
//       `pdfdebug dump object --ref "5 0 R" <file>` is executed, Then the
//       CLI outputs the ObjectDetail for that object as structured JSON to
//       stdout, And the exit code is 0.
// ---------------------------------------------------------------------------

func TestObjectDump_ValidRef_OutputsJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	ref, _ := discoverValidRef(t, bin, pdfPath)

	stdout, _, exitCode := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P0] 5.2-INTG-001: expected exit code 0, got %d", exitCode)
	}

	// stdout must be valid JSON
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("[P0] 5.2-INTG-001: stdout is not valid JSON: %v\nraw: %s", err, stdout)
	}

	// Must contain ObjectDetail fields
	requiredFields := []string{"nodeId", "objectRef", "type"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("[P0] 5.2-INTG-001: ObjectDetail JSON missing required field %q", field)
		}
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-001 [P0]: Run with `--ref "999 0 R"` against minimal.pdf,
// verify stderr contains JSON error, stdout is empty, exit code 2.
// AC#2: Given an invalid or non-existent reference, When the CLI is
//       executed with `--ref "999 0 R"`, Then an error message in JSON
//       format is written to stderr indicating the object was not found,
//       And the exit code is 2.
// ---------------------------------------------------------------------------

func TestObjectDump_InvalidRef_JSONErrorOnStderr(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "999 0 R", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P0] 5.2-UNIT-001: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P0] 5.2-UNIT-001: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P0] 5.2-UNIT-001: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P0] 5.2-UNIT-001: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.2-INTG-002 [P1]: Query a known dict object (the catalog root object
// from minimal.pdf), verify the JSON has `properties` array with at least
// one entry containing `key` and `value` fields.
// AC#1: The JSON includes the object's properties (for dicts).
// ---------------------------------------------------------------------------

func TestObjectDump_DictObject_HasProperties(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// Discover a dict object via tree dump. The root's first obj: child
	// in minimal.pdf is typically the catalog (a dict).
	ref, _ := discoverValidRef(t, bin, pdfPath)

	stdout, _, exitCode := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.2-INTG-002: expected exit code 0, got %d", exitCode)
	}

	var detail map[string]any
	mustParseJSON(t, stdout, &detail)

	objType, _ := detail["type"].(string)
	if objType != "dict" {
		t.Skipf("[P1] 5.2-INTG-002: discovered object is type %q, not dict -- skipping (test needs a dict object)", objType)
	}

	props, ok := detail["properties"]
	if !ok {
		t.Fatal("[P1] 5.2-INTG-002: dict ObjectDetail missing 'properties' field")
	}

	propArr, ok := props.([]any)
	if !ok || len(propArr) == 0 {
		t.Fatal("[P1] 5.2-INTG-002: 'properties' is empty or not an array")
	}

	// Check first property has key and value
	firstProp, ok := propArr[0].(map[string]any)
	if !ok {
		t.Fatal("[P1] 5.2-INTG-002: first property is not an object")
	}

	if _, ok := firstProp["key"]; !ok {
		t.Error("[P1] 5.2-INTG-002: property entry missing 'key' field")
	}
	if _, ok := firstProp["value"]; !ok {
		t.Error("[P1] 5.2-INTG-002: property entry missing 'value' field")
	}
}

// ---------------------------------------------------------------------------
// 5.2-INTG-003 [P1]: Query a known array object from multipage.pdf
// (Pages/Kids array), verify `elements` array is populated.
// AC#1: The JSON includes elements (for arrays).
//
// Discovery strategy: dump tree of multipage.pdf, walk to find an obj:
// node, query it, check if type is "array". If not, walk deeper to find
// an array. This test may skip if no array object is readily discoverable.
// ---------------------------------------------------------------------------

func TestObjectDump_ArrayObject_HasElements(t *testing.T) {
	bin := buildCLI(t)
	// array-object.pdf has object 4 as an indirect array ([3 0 R]).
	pdfPath := filepath.Join(testdataDir(t), "array-object.pdf")

	stdout, _, exitCode := runCLI(t, bin, "dump", "object", "--json", "--ref", "4 0 R", pdfPath)
	if exitCode != 0 {
		t.Fatalf("[P1] 5.2-INTG-003: expected exit code 0, got %d", exitCode)
	}

	var detail map[string]any
	mustParseJSON(t, stdout, &detail)

	objType, _ := detail["type"].(string)
	if objType != "array" {
		t.Fatalf("[P1] 5.2-INTG-003: expected type 'array', got %q", objType)
	}

	elements, ok := detail["elements"]
	if !ok {
		t.Fatal("[P1] 5.2-INTG-003: array ObjectDetail missing 'elements' field")
	}

	elemArr, ok := elements.([]any)
	if !ok || len(elemArr) == 0 {
		t.Fatal("[P1] 5.2-INTG-003: 'elements' is empty or not an array")
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-002 [P1]: Test valid ref format handling by running the binary
// with refs discovered from the tree dump. Since parseObjectRef is
// unexported, validate indirectly: run `pdfdebug dump object --ref "N 0 R"
// <file>` for each, verify exit code 0 and valid JSON output.
// AC#1: Valid references are accepted and produce correct output.
// ---------------------------------------------------------------------------

func TestObjectDump_ValidRefFormats(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// Dump tree and collect multiple obj: node IDs
	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)
	if exitCode != 0 {
		t.Fatalf("[P1] 5.2-UNIT-002: tree dump failed with exit code %d", exitCode)
	}

	var root treeNode
	mustParseJSON(t, stdout, &root)

	var objNodeIDs []string
	var collectObjNodes func(n treeNode)
	collectObjNodes = func(n treeNode) {
		if strings.HasPrefix(n.ID, "obj:") {
			objNodeIDs = append(objNodeIDs, n.ID)
		}
		for _, child := range n.Children {
			collectObjNodes(child)
		}
	}
	collectObjNodes(root)

	if len(objNodeIDs) == 0 {
		t.Fatal("[P1] 5.2-UNIT-002: no obj: nodes found in tree dump")
	}

	// Test at least 2 different refs (or as many as available)
	limit := min(2, len(objNodeIDs))

	for i := range limit {
		nodeID := objNodeIDs[i]
		ref, err := nodeIDToRef(nodeID)
		if err != nil {
			t.Fatalf("[P1] 5.2-UNIT-002: failed to convert node ID %q: %v", nodeID, err)
		}

		t.Run(ref, func(t *testing.T) {
			objStdout, _, objExitCode := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)

			if objExitCode != 0 {
				t.Errorf("[P1] 5.2-UNIT-002: expected exit code 0 for ref %q, got %d", ref, objExitCode)
			}

			if !json.Valid([]byte(objStdout)) {
				t.Errorf("[P1] 5.2-UNIT-002: stdout is not valid JSON for ref %q\nraw: %s", ref, objStdout)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-003 [P1]: Test with malformed refs: "abc", "5 0", "5 0 r"
// (lowercase), "5", "". Verify each produces a clear error message
// mentioning the expected format, exit code 1.
// AC#3: Given a malformed reference string, When the CLI is executed,
//       Then the error message on stderr clearly describes the expected
//       reference format, And the exit code is 1 (usage error).
// ---------------------------------------------------------------------------

func TestObjectDump_MalformedRef_ClearError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	cases := []struct {
		name string
		ref  string
	}{
		{"alphabetic", "abc"},
		{"missing R", "5 0"},
		{"lowercase r", "5 0 r"},
		{"number only", "5"},
		{"empty string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", tc.ref, pdfPath)

			if exitCode != 1 {
				t.Errorf("[P1] 5.2-UNIT-003/%s: expected exit code 1 (usage error), got %d", tc.name, exitCode)
			}

			if strings.TrimSpace(stdout) != "" {
				t.Errorf("[P1] 5.2-UNIT-003/%s: stdout should be empty for malformed ref, got: %s", tc.name, stdout)
			}

			// stderr should contain an error mentioning the expected format
			stderrLower := strings.ToLower(stderr)
			if !strings.Contains(stderrLower, "n g r") && !strings.Contains(stderrLower, "format") && !strings.Contains(stderrLower, "reference") {
				t.Errorf("[P1] 5.2-UNIT-003/%s: error message should describe expected ref format\nstderr: %s", tc.name, stderr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5.2-INTG-004 [P2]: Query a known stream object from content-stream.pdf,
// verify `streamInfo` field is present with `length` and `filters`.
// AC#4: Given a stream object reference, When the CLI is executed, Then
//       the ObjectDetail JSON includes `streamInfo` with length and filters.
// ---------------------------------------------------------------------------

func TestObjectDump_StreamObject_HasStreamInfo(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	// Dump tree and collect all obj: node IDs
	stdout, _, exitCode := runCLI(t, bin, "dump", "tree", "--json", pdfPath)
	if exitCode != 0 {
		t.Fatalf("[P2] 5.2-INTG-004: tree dump failed with exit code %d", exitCode)
	}

	var root treeNode
	mustParseJSON(t, stdout, &root)

	var objNodeIDs []string
	var collectObjNodes func(n treeNode)
	collectObjNodes = func(n treeNode) {
		if strings.HasPrefix(n.ID, "obj:") {
			objNodeIDs = append(objNodeIDs, n.ID)
		}
		for _, child := range n.Children {
			collectObjNodes(child)
		}
	}
	collectObjNodes(root)

	if len(objNodeIDs) == 0 {
		t.Fatal("[P2] 5.2-INTG-004: no obj: nodes found in tree dump")
	}

	// Try each obj: node until we find a stream
	var foundStream bool
	for _, nodeID := range objNodeIDs {
		ref, err := nodeIDToRef(nodeID)
		if err != nil {
			continue
		}

		objStdout, _, objExitCode := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)
		if objExitCode != 0 {
			continue
		}

		var detail map[string]any
		if err := json.Unmarshal([]byte(objStdout), &detail); err != nil {
			continue
		}

		objType, _ := detail["type"].(string)
		if objType != "stream" {
			continue
		}

		foundStream = true

		streamInfo, ok := detail["streamInfo"]
		if !ok || streamInfo == nil {
			t.Fatal("[P2] 5.2-INTG-004: stream ObjectDetail missing 'streamInfo' field")
		}

		infoMap, ok := streamInfo.(map[string]any)
		if !ok {
			t.Fatal("[P2] 5.2-INTG-004: 'streamInfo' is not an object")
		}

		if _, ok := infoMap["length"]; !ok {
			t.Error("[P2] 5.2-INTG-004: streamInfo missing 'length' field")
		}
		if _, ok := infoMap["filters"]; !ok {
			t.Error("[P2] 5.2-INTG-004: streamInfo missing 'filters' field")
		}
		break
	}

	if !foundStream {
		t.Fatal("[P2] 5.2-INTG-004: no stream object found in content-stream.pdf tree -- expected at least one stream")
	}
}

// ---------------------------------------------------------------------------
// 5.2-INTG-005 [P1]: Verify ObjectDetail includes null-valued fields for
// inapplicable sections. A dict object should have elements: null, a
// scalar should have properties: null, etc.
// AC#1: The output matches the ObjectDetail model structure (including
//       null-valued fields for inapplicable sections).
// ---------------------------------------------------------------------------

func TestObjectDump_NullFieldsPresent(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	ref, _ := discoverValidRef(t, bin, pdfPath)

	stdout, _, exitCode := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.2-INTG-005: expected exit code 0, got %d", exitCode)
	}

	// Parse as raw JSON to check for presence of null fields
	var raw map[string]json.RawMessage
	mustParseJSON(t, stdout, &raw)

	// ObjectDetail should include all fields, even if null.
	// For a dict object: properties is populated, elements/scalarValue/streamInfo are null.
	// For any type: all fields should be present in the JSON.
	expectedFields := []string{"nodeId", "objectRef", "type", "properties", "elements", "scalarValue", "streamInfo"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("[P1] 5.2-INTG-005: ObjectDetail JSON missing field %q (should be present, even if null)", field)
		}
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-004 [P1]: Run without --ref flag, verify usage error on stderr,
// exit code 1.
// AC#3 (implied): Missing required --ref flag is a usage error.
// ---------------------------------------------------------------------------

func TestObjectDump_MissingRefFlag_UsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", pdfPath)

	if exitCode != 1 {
		t.Errorf("[P1] 5.2-UNIT-004: expected exit code 1 (usage error), got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P1] 5.2-UNIT-004: stdout should be empty for usage errors, got: %s", stdout)
	}

	if strings.TrimSpace(stderr) == "" {
		t.Error("[P1] 5.2-UNIT-004: stderr should contain usage/error information")
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-005 [P1]: Run without file path, verify usage error on stderr,
// exit code 1.
// AC#3 (implied): Missing file path is a usage error.
// ---------------------------------------------------------------------------

func TestObjectDump_MissingFilePath_UsageError(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "1 0 R")

	if exitCode != 1 {
		t.Errorf("[P1] 5.2-UNIT-005: expected exit code 1 (usage error), got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P1] 5.2-UNIT-005: stdout should be empty for usage errors, got: %s", stdout)
	}

	if strings.TrimSpace(stderr) == "" {
		t.Error("[P1] 5.2-UNIT-005: stderr should contain usage/error information")
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-006 [P1] (REVISED by Story 13-1): Object dump WITHOUT --json emits
// human-readable PLAIN TEXT (the flipped default), NOT JSON. The plain output
// is an aligned single record (Object/Type header + Properties block).
// ---------------------------------------------------------------------------

func TestObjectDump_WithoutJSONFlag_OutputsPlainText(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	ref, _ := discoverValidRef(t, bin, pdfPath)

	// Run WITHOUT --json flag -> plain text default.
	stdout, _, exitCode := runCLI(t, bin, "dump", "object", "--ref", ref, pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.2-UNIT-006: expected exit code 0, got %d", exitCode)
	}

	trimmed := strings.TrimSpace(stdout)
	if strings.HasPrefix(trimmed, "{") && json.Valid([]byte(trimmed)) {
		t.Fatalf("[P1] 5.2-UNIT-006: default object output must be plain text, not JSON\nraw: %s", stdout)
	}
	// Structural: the aligned record names the object and its type.
	if !strings.Contains(stdout, "Object:") || !strings.Contains(stdout, "Type:") {
		t.Errorf("[P1] 5.2-UNIT-006: plain object output should carry aligned Object:/Type: keys\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-007 [P1]: Object dump with --json flag explicitly also works.
// AC#1: --json flag is accepted.
// ---------------------------------------------------------------------------

func TestObjectDump_WithJSONFlag_OutputsJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	ref, _ := discoverValidRef(t, bin, pdfPath)

	stdout, _, exitCode := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.2-UNIT-007: expected exit code 0, got %d", exitCode)
	}

	if !json.Valid([]byte(stdout)) {
		t.Fatalf("[P1] 5.2-UNIT-007: stdout with --json flag is not valid JSON\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-008 [P2]: "0 0 R" is syntactically valid but refers to the
// free-list head entry; it should return null and be caught as "object
// not found" with exit code 2.
// AC#2: Non-existent references produce an error.
// ---------------------------------------------------------------------------

func TestObjectDump_ZeroZeroRef_NotFound(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "0 0 R", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P2] 5.2-UNIT-008: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.2-UNIT-008: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.2-UNIT-008: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.2-UNIT-008: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-009 [P2]: Non-existent file path returns JSON error on stderr
// and exit code 2 (file error, not usage error).
// AC#2 (boundary): File errors use exit code 2.
// ---------------------------------------------------------------------------

func TestObjectDump_NonexistentFile_JSONErrorExitCode2(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "1 0 R", "/nonexistent/path/fake.pdf")

	if exitCode != 2 {
		t.Errorf("[P2] 5.2-UNIT-009: expected exit code 2 for file error, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.2-UNIT-009: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.2-UNIT-009: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.2-UNIT-009: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-010 [P2]: Negative object number in ref is rejected.
// AC#3: Negative object/generation numbers are invalid.
// ---------------------------------------------------------------------------

func TestObjectDump_NegativeRefNumbers_Error(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	cases := []struct {
		name string
		ref  string
	}{
		{"negative obj num", "-1 0 R"},
		{"negative gen num", "1 -1 R"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, exitCode := runCLI(t, bin, "dump", "object", "--ref", tc.ref, pdfPath)

			if exitCode != 1 {
				t.Errorf("[P2] 5.2-UNIT-010/%s: expected exit code 1 (usage error), got %d", tc.name, exitCode)
			}

			if strings.TrimSpace(stdout) != "" {
				t.Errorf("[P2] 5.2-UNIT-010/%s: stdout should be empty, got: %s", tc.name, stdout)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-011 [P1]: Stdout contains ONLY JSON (no log noise from pdfcpu).
// AC#1: Output is well-formed JSON suitable for piping to jq.
// ---------------------------------------------------------------------------

func TestObjectDump_StdoutContainsOnlyJSON(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	ref, _ := discoverValidRef(t, bin, pdfPath)

	stdout, _, exitCode := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)

	if exitCode != 0 {
		t.Fatalf("[P1] 5.2-UNIT-011: expected exit code 0, got %d", exitCode)
	}

	if !json.Valid([]byte(stdout)) {
		t.Fatalf("[P1] 5.2-UNIT-011: stdout is not valid JSON (log noise present?)\nraw: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-012 [P2]: Encrypted PDF with --ref produces JSON error on stderr
// and exit code 2. Verifies that the object dump command handles encrypted
// PDFs the same way as tree dump -- JSON error, not a crash.
// AC#2 (boundary): Encrypted PDF is a file-level error.
// ---------------------------------------------------------------------------

func TestObjectDump_EncryptedPDF_JSONErrorExitCode2(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "encrypted.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "1 0 R", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P2] 5.2-UNIT-012: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.2-UNIT-012: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.2-UNIT-012: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.2-UNIT-012: stderr JSON missing 'error' key")
	}
}

// ---------------------------------------------------------------------------
// 5.2-UNIT-013 [P2]: Malformed PDF with --ref produces JSON error on stderr
// and exit code 2. Verifies the object dump command handles corrupt PDFs
// gracefully with structured error output.
// AC#2 (boundary): Malformed PDF is a file-level error.
// ---------------------------------------------------------------------------

func TestObjectDump_MalformedPDF_JSONErrorExitCode2(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "malformed.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "1 0 R", pdfPath)

	if exitCode != 2 {
		t.Errorf("[P2] 5.2-UNIT-013: expected exit code 2, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("[P2] 5.2-UNIT-013: stdout should be empty for error cases, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("[P2] 5.2-UNIT-013: stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}

	if _, ok := errObj["error"]; !ok {
		t.Error("[P2] 5.2-UNIT-013: stderr JSON missing 'error' key")
	}
}
