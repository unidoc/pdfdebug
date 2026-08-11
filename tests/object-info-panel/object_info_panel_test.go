// Package object_info_panel_test provides acceptance tests for Story 2.6:
// Object Info Panel -- Property Display for Selected Nodes.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 2-6 is implemented.
//
// Test Levels: Unit (Go) -- pdfcore GetObjectDetail API validation.
// No browser interaction required; all criteria are Go package validation.
//
// Run: go test ./tests/object-info-panel/... -v -count=1
package object_info_panel_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot returns the absolute path to the project root directory.
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

// runPdfcoreTest runs a named test pattern in internal/pdfcore/... and fails if
// the test does not pass or does not exist.
func runPdfcoreTest(t *testing.T, runPattern string) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-v", "-run", runPattern, "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	outStr := string(output)
	if err != nil {
		t.Fatalf("pdfcore test failed:\n%s", outStr)
	}
	if strings.Contains(outStr, "no tests to run") {
		t.Fatalf("no matching test found for pattern %q -- unit test not implemented yet:\n%s", runPattern, outStr)
	}
	if !strings.Contains(outStr, "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", outStr)
	}
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-001 [P0]: GetObjectDetail() returns correct key-value pairs for
// dictionary node with type-preserved values
// AC#2: Given a dictionary node is selected in the tree, When the
//       ObjectInfoPanel updates, Then it displays a key-value table with
//       each PDF dictionary key and its value, And values are typed.
// ---------------------------------------------------------------------------

func TestGetObjectDetailDict(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailDict
	// which must:
	// - Open minimal.pdf via Inspector.Open()
	// - Call GetObjectDetail(tabID, "root") to get the catalog dict detail
	// - Assert ObjectDetail.Type == "dict"
	// - Assert ObjectDetail.NodeID == "root"
	// - Assert ObjectDetail.Properties is non-nil and non-empty
	// - Assert each PropertyEntry has a Key starting with "/"
	// - Assert each PropertyEntry.Value has a non-empty Type field
	//   (one of: "name", "string", "number", "boolean", "null", "reference",
	//    "dict", "array")
	// - Assert Properties are sorted alphabetically by Key
	// - Assert the /Type property has Value.Type == "name" and
	//   Value.Display == "/Catalog"
	runPdfcoreTest(t, "TestGetObjectDetailDict$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-002 [P0]: GetObjectDetail() returns correct elements for array node
// AC#3: Given an array node is selected, When the ObjectInfoPanel updates,
//       Then it displays an indexed list of array elements with type-colored
//       values.
// ---------------------------------------------------------------------------

func TestGetObjectDetailArray(t *testing.T) {
	multipagePDF := filepath.Join(testdataDir(t), "multipage.pdf")
	if _, err := os.Stat(multipagePDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/multipage.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailArray
	// which must:
	// - Open multipage.pdf via Inspector.Open()
	// - Navigate to an array node (e.g., /Kids array under /Pages)
	// - Call GetObjectDetail(tabID, arrayNodeID)
	// - Assert ObjectDetail.Type == "array"
	// - Assert ObjectDetail.Elements is non-nil and has correct count
	// - Assert each ValueEntry has Type, Display, and Raw fields set
	// - Assert IndirectRef elements have Type == "reference" and RefTarget
	//   in "obj:{gen}:{num}" format
	runPdfcoreTest(t, "TestGetObjectDetailArray$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-003 [P0]: GetObjectDetail() returns correct value for scalar node
// AC#4: Given a scalar node is selected, When the ObjectInfoPanel updates,
//       Then it displays the single value with its type label.
// ---------------------------------------------------------------------------

func TestGetObjectDetailScalar(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailScalar
	// which must:
	// - Open minimal.pdf via Inspector.Open()
	// - Navigate to a scalar node (e.g., dict:root:Type which is a Name)
	// - Call GetObjectDetail(tabID, scalarNodeID)
	// - Assert ObjectDetail.Type == "scalar"
	// - Assert ObjectDetail.ScalarValue is non-nil
	// - Assert ScalarValue.Type is set (e.g., "name")
	// - Assert ScalarValue.Display is set (e.g., "/Catalog")
	runPdfcoreTest(t, "TestGetObjectDetailScalar$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-004 [P0]: GetObjectDetail() returns stream properties and metadata
// AC#6: Given a stream node is selected, When the ObjectInfoPanel updates,
//       Then it displays the stream's dictionary properties as a key-value
//       table, And it displays stream metadata (length and filter names)
//       below the properties.
// ---------------------------------------------------------------------------

func TestGetObjectDetailStream(t *testing.T) {
	// content-stream.pdf or multipage.pdf should have stream objects
	multipagePDF := filepath.Join(testdataDir(t), "multipage.pdf")
	if _, err := os.Stat(multipagePDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/multipage.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailStream
	// which must:
	// - Open a PDF with stream objects
	// - Navigate to a stream node
	// - Call GetObjectDetail(tabID, streamNodeID)
	// - Assert ObjectDetail.Type == "stream"
	// - Assert ObjectDetail.Properties is non-nil (stream's dict entries)
	// - Assert ObjectDetail.StreamInfo is non-nil
	// - Assert StreamInfo.Length >= 0
	// - Assert StreamInfo.Filters is non-nil (possibly empty []string{})
	runPdfcoreTest(t, "TestGetObjectDetailStream$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-005 [P0]: GetObjectDetail() populates ObjectRef for indirect objects
// AC#2: The object reference (e.g., "4 0 R") is displayed in the panel header.
// ---------------------------------------------------------------------------

func TestGetObjectDetailObjectRef(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailObjectRef
	// which must:
	// - Open minimal.pdf, get children of root to find an obj: node
	// - Call GetObjectDetail(tabID, "obj:{gen}:{num}")
	// - Assert ObjectDetail.ObjectRef matches "{num} {gen} R" format
	// - Also test with "root" nodeID and assert ObjectRef is empty
	//   (root is not an indirect object)
	runPdfcoreTest(t, "TestGetObjectDetailObjectRef$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-006 [P0]: GetObjectDetail() returns empty properties for empty dict
// AC#5: Given an empty dictionary is selected, When the ObjectInfoPanel
//       updates, Then it shows "Empty dictionary" in muted text.
// (Backend: returns ObjectDetail.Type == "dict" with Properties == [])
// ---------------------------------------------------------------------------

func TestGetObjectDetailEmptyDict(t *testing.T) {
	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailEmptyDict
	// which must:
	// - Construct or navigate to an empty dict scenario
	// - Call GetObjectDetail
	// - Assert ObjectDetail.Type == "dict"
	// - Assert ObjectDetail.Properties is non-nil and len == 0
	runPdfcoreTest(t, "TestGetObjectDetailEmptyDict$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-007 [P0]: GetObjectDetail() returns empty elements for empty array
// AC#5: Given an empty array is selected, When the ObjectInfoPanel updates,
//       Then it shows "Empty array" in muted text.
// (Backend: returns ObjectDetail.Type == "array" with Elements == [])
// ---------------------------------------------------------------------------

func TestGetObjectDetailEmptyArray(t *testing.T) {
	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailEmptyArray
	// which must:
	// - Construct or navigate to an empty array scenario
	// - Call GetObjectDetail
	// - Assert ObjectDetail.Type == "array"
	// - Assert ObjectDetail.Elements is non-nil and len == 0
	runPdfcoreTest(t, "TestGetObjectDetailEmptyArray$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-008 [P1]: GetObjectDetail() with unknown tabID returns error
// AC#2 (negative): Given an unknown tabID, When GetObjectDetail is called,
//                  Then it returns ErrDocumentNotFound.
// ---------------------------------------------------------------------------

func TestGetObjectDetailUnknownTabID(t *testing.T) {
	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailUnknownTabID
	// which must:
	// - Call GetObjectDetail with a tabID that was never opened
	// - Assert error is returned
	// - Assert error wraps ErrDocumentNotFound
	runPdfcoreTest(t, "TestGetObjectDetailUnknownTabID$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-009 [P1]: GetObjectDetail() with invalid nodeID returns error
// AC#2 (negative): Given an invalid nodeID, When GetObjectDetail is called,
//                  Then it returns an error (not panic).
// ---------------------------------------------------------------------------

func TestGetObjectDetailInvalidNodeID(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailInvalidNodeID
	// which must:
	// - Open minimal.pdf
	// - Call GetObjectDetail with an invalid nodeID (e.g., "bogus")
	// - Assert error is returned
	// - Assert no panic occurs
	runPdfcoreTest(t, "TestGetObjectDetailInvalidNodeID$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-010 [P1]: GetObjectDetail() reference values have RefTarget set
// AC#2: References in purple (text-type-reference, underlined and clickable).
// Backend must set RefTarget on IndirectRef ValueEntry so frontend can render.
// ---------------------------------------------------------------------------

func TestGetObjectDetailRefTarget(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/inspector_test.go::TestGetObjectDetailRefTarget
	// which must:
	// - Open minimal.pdf, get detail for root (catalog dict)
	// - Find a property whose Value.Type == "reference"
	// - Assert Value.RefTarget is non-empty and matches "obj:{gen}:{num}" format
	// - Assert Value.Display matches "{num} {gen} R" format
	runPdfcoreTest(t, "TestGetObjectDetailRefTarget$")
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-011 [P1]: GetObjectDetail method exists on Inspector
// AC#2: inspector.go has GetObjectDetail method signature.
// ---------------------------------------------------------------------------

func TestGetObjectDetailMethodExists(t *testing.T) {
	root := projectRoot(t)

	inspectorPath := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(inspectorPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/inspector.go does not exist: %v", err)
	}

	src := string(content)

	if !strings.Contains(src, "func (ins *Inspector) GetObjectDetail(") {
		t.Error("inspector.go missing GetObjectDetail method on Inspector")
	}
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-012 [P1]: valueEntryFromObject helper exists
// AC#2: Centralized pdfcpu-type-to-ValueEntry mapping helper.
// ---------------------------------------------------------------------------

func TestValueEntryHelperExists(t *testing.T) {
	root := projectRoot(t)

	inspectorPath := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(inspectorPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/inspector.go does not exist: %v", err)
	}

	src := string(content)

	if !strings.Contains(src, "func valueEntryFromObject(") {
		t.Error("inspector.go missing valueEntryFromObject helper")
	}
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-013 [P1]: extractStreamInfo helper exists
// AC#6: Stream metadata extraction helper.
// ---------------------------------------------------------------------------

func TestExtractStreamInfoHelperExists(t *testing.T) {
	root := projectRoot(t)

	inspectorPath := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(inspectorPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/inspector.go does not exist: %v", err)
	}

	src := string(content)

	if !strings.Contains(src, "func extractStreamInfo(") {
		t.Error("inspector.go missing extractStreamInfo helper")
	}
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-014 [P1]: GetObjectDetail wrapped in safeCall for panic recovery
// AC#2: All pdfcpu calls must be wrapped in safeCall.
// ---------------------------------------------------------------------------

func TestGetObjectDetailPanicRecovery(t *testing.T) {
	root := projectRoot(t)

	inspectorPath := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(inspectorPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/inspector.go does not exist: %v", err)
	}

	src := string(content)

	// GetObjectDetail must use safeCall
	if !strings.Contains(src, "safeCall") {
		t.Error("inspector.go GetObjectDetail does not use safeCall for panic recovery")
	}
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-015 [P1]: inspector.go has no Wails imports
// AC#2: pdfcore must have zero Wails dependencies.
// ---------------------------------------------------------------------------

func TestInspectorNoWailsImports(t *testing.T) {
	root := projectRoot(t)

	inspectorPath := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(inspectorPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/inspector.go does not exist: %v", err)
	}

	src := string(content)

	if strings.Contains(src, "wailsapp") || strings.Contains(src, "wails/v3") {
		t.Error("inspector.go imports Wails -- pdfcore must have zero Wails dependencies")
	}
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-016 [P1]: All pdfcore tests pass after GetObjectDetail addition
// AC#2-#6: Unit tests cover all object detail scenarios.
// ---------------------------------------------------------------------------

func TestAllPdfcoreTestsPass(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfcore test suite failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-017 [P1]: go vet passes on pdfcore with GetObjectDetail
// AC#2: No vet warnings after adding GetObjectDetail.
// ---------------------------------------------------------------------------

func TestPdfcoreGoVetWithObjectDetail(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet failed on pdfcore:\n%s", string(output))
	}
	_ = output
}

// ---------------------------------------------------------------------------
// 2.6-UNIT-018 [P1]: pdfcore compiles with GetObjectDetail added
// AC#2: go build ./... succeeds.
// ---------------------------------------------------------------------------

func TestPdfcoreCompilesWithObjectDetail(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "build", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build./internal/pdfcore/... failed:\n%s", string(output))
	}
	_ = output
}
