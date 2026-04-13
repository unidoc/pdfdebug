// Package content_stream_decode_test provides acceptance tests for Story 3.1:
// Content Stream Decoding in PDF Core.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 3-1 is implemented.
//
// Test Levels: Unit (Go) and Integration (Go) -- pdfcore API validation.
// No browser interaction required; all criteria are Go package validation.
//
// Run: cd tests/content-stream-decode && go test -v -count=1 ./...
package content_stream_decode_test

import (
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

// ---------------------------------------------------------------------------
// 3.1-UNIT-001 [P0]: GetContentStream returns decoded plain text for a
// page's Contents node using testdata/content-stream.pdf.
// AC#1: Given a tree node ID corresponding to a page's Contents entry,
//       When GetContentStream(tabID, nodeID) is called,
//       Then it returns ContentStreamData with decoded plain text in Raw,
//       And NodeID is populated correctly.
// ---------------------------------------------------------------------------

func TestGetContentStreamValid(t *testing.T) {
	root := projectRoot(t)

	// Verify fixture exists
	contentStreamPDF := filepath.Join(testdataDir(t), "content-stream.pdf")
	if _, err := os.Stat(contentStreamPDF); os.IsNotExist(err) {
		t.Fatalf("[P0] testdata/content-stream.pdf does not exist -- create test fixture first")
	}

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamValid", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P0] 3.1-UNIT-001: GetContentStream valid stream test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P0] 3.1-UNIT-001: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-002 [P0]: GetContentStream on corrupted/non-decodable stream
// returns error in ContentStreamData.Error, does not panic.
// AC#2: Given a content stream that cannot be decoded,
//       When GetContentStream is called,
//       Then the Error field is populated with a clear message,
//       And the function does not crash or panic.
// Note: The non-stream node test covers the error-path branch since
//       content-stream fixtures with unsupported filters are not available.
// ---------------------------------------------------------------------------

func TestGetContentStreamNonStream(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamNonStream", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P0] 3.1-UNIT-002: GetContentStream non-stream node test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P0] 3.1-UNIT-002: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-004 [P0]: GetContentStream with invalid tabID returns
// ErrDocumentNotFound.
// ---------------------------------------------------------------------------

func TestGetContentStreamUnknownTab(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamUnknownTab", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P0] 3.1-UNIT-004: GetContentStream unknown tab test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P0] 3.1-UNIT-004: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-005 [P0]: GetContentStream with empty nodeID returns Go error.
// ---------------------------------------------------------------------------

func TestGetContentStreamEmptyNodeID(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamEmptyNodeID", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P0] 3.1-UNIT-005: GetContentStream empty nodeID test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P0] 3.1-UNIT-005: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-006 [P0]: Decoded content stream is cached per-document.
// AC#3: Second call returns same result from cache.
// ---------------------------------------------------------------------------

func TestGetContentStreamCached(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamCached", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P0] 3.1-UNIT-006: GetContentStream caching test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P0] 3.1-UNIT-006: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-006b [P0]: GetContentStream with error-prefixed nodeID returns
// ContentStreamData.Error, no panic.
// AC#2: Graceful degradation for error nodes.
// ---------------------------------------------------------------------------

func TestGetContentStreamErrorNode(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamErrorNode", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P0] 3.1-UNIT-006b: GetContentStream error node test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P0] 3.1-UNIT-006b: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-009 [P1]: ContentStreamData.NodeID field is populated correctly.
// ---------------------------------------------------------------------------

func TestGetContentStreamNodeIDPopulated(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamNodeIDPopulated", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 3.1-UNIT-009: GetContentStream NodeID population test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 3.1-UNIT-009: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-010 [P1]: PDFService.GetContentStream() delegates to
// Inspector.GetContentStream() and returns ContentStreamData.
// AC#1: Thin adapter pattern for service layer.
// ---------------------------------------------------------------------------

func TestPDFServiceGetContentStreamValid(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamValid", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 3.1-UNIT-010: PDFService.GetContentStream() valid test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 3.1-UNIT-010: expected PASS in output but got:\n%s", string(output))
	}
}

func TestPDFServiceGetContentStreamUnknown(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStreamUnknown", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 3.1-UNIT-010b: PDFService.GetContentStream() unknown tab test failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 3.1-UNIT-010b: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-INTG-001 [P1]: Full decode pipeline: Open() + GetContentStream() on
// testdata/content-stream.pdf returns readable text with PDF operators.
// ---------------------------------------------------------------------------

func TestGetContentStreamIntegration(t *testing.T) {
	root := projectRoot(t)

	// Run all content stream tests together as an integration check
	cmd := exec.Command("go", "test", "-v", "-run", "TestGetContentStream", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 3.1-INTG-001: Full content stream decode pipeline failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 3.1-INTG-001: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-INTG-002 [P1]: stream.go exists in internal/pdfcore/
// AC#1: Content stream decoding logic lives in pdfcore/stream.go.
// ---------------------------------------------------------------------------

func TestStreamFileExists(t *testing.T) {
	root := projectRoot(t)

	streamPath := filepath.Join(root, "internal", "pdfcore", "stream.go")
	if _, err := os.Stat(streamPath); os.IsNotExist(err) {
		t.Fatal("[P1] 3.1-INTG-002: internal/pdfcore/stream.go does not exist")
	}
}

// ---------------------------------------------------------------------------
// 3.1-INTG-003 [P1]: Inspector.GetContentStream method exists
// AC#1: Method signature present in pdfcore.
// ---------------------------------------------------------------------------

func TestGetContentStreamMethodExists(t *testing.T) {
	root := projectRoot(t)

	// Check stream.go or inspector.go for the method signature
	streamPath := filepath.Join(root, "internal", "pdfcore", "stream.go")
	content, err := os.ReadFile(streamPath)
	if err != nil {
		t.Fatalf("[P1] 3.1-INTG-003: cannot read stream.go: %v", err)
	}
	if !strings.Contains(string(content), "func (ins *Inspector) GetContentStream(") {
		t.Error("[P1] 3.1-INTG-003: stream.go missing GetContentStream method on Inspector")
	}
}

// ---------------------------------------------------------------------------
// 3.1-INTG-004 [P1]: PDFService.GetContentStream method exists in service.go
// AC#1: Thin adapter pass-through.
// ---------------------------------------------------------------------------

func TestServiceGetContentStreamMethodExists(t *testing.T) {
	root := projectRoot(t)

	servicePath := filepath.Join(root, "internal", "pdfservice", "service.go")
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("[P1] 3.1-INTG-004: cannot read service.go: %v", err)
	}
	if !strings.Contains(string(content), "func (s *PDFService) GetContentStream(") {
		t.Error("[P1] 3.1-INTG-004: service.go missing GetContentStream method on PDFService")
	}
}

// ---------------------------------------------------------------------------
// 3.1-INTG-005 [P1]: DocumentState has streamCache field
// AC#3: Caching infrastructure exists.
// ---------------------------------------------------------------------------

func TestDocumentStateHasStreamCache(t *testing.T) {
	root := projectRoot(t)

	inspectorPath := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(inspectorPath)
	if err != nil {
		t.Fatalf("[P1] 3.1-INTG-005: cannot read inspector.go: %v", err)
	}
	if !strings.Contains(string(content), "streamCache") {
		t.Error("[P1] 3.1-INTG-005: DocumentState does not have streamCache field")
	}
}

// ---------------------------------------------------------------------------
// 3.1-INTG-006 [P1]: All pdfcore tests still pass (no regression)
// ---------------------------------------------------------------------------

func TestPdfcoreNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 3.1-INTG-006: pdfcore regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 3.1-INTG-006: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-INTG-007 [P1]: All pdfservice tests still pass (no regression)
// ---------------------------------------------------------------------------

func TestPdfserviceNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 3.1-INTG-007: pdfservice regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 3.1-INTG-007: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 3.1-INTG-008 [P1]: go vet passes on pdfcore package
// ---------------------------------------------------------------------------

func TestPdfcoreGoVet(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 3.1-INTG-008: go vet failed on pdfcore:\n%s", string(output))
	}
}
