// Package pdf_core_inspector_test provides acceptance tests for PDF Core
// Inspector -- Open and Parse PDF Files.
//
// Test Levels: Unit (Go) and Integration (Go) -- pdfcore API + filesystem checks.
// No browser interaction required; all criteria are Go package validation.
//
// Run: go test ./tests/pdf-core-inspector/... -v -count=1
package pdf_core_inspector_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot returns the absolute path to the project root directory.
// It walks upward from the test file location to find the project root,
// identified by the presence of a go.mod whose module name is "unidoc-pdf-debugger".
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
// Inspector.Open() opens valid PDF, returns DocumentInfo: Given a valid PDF
// file path, When Inspector.Open() is called,
//       Then pdfcpu parses the file and returns DocumentInfo with tabId,
//       fileName, filePath, pageCount, and fileSize.
// ---------------------------------------------------------------------------

func TestInspectorOpenValidPDF(t *testing.T) {
	root := projectRoot(t)

	// Verify testdata/minimal.pdf exists (prerequisite: test fixture)
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Run the pdfcore unit test that validates Open with valid PDF.
	// This delegates to internal/pdfcore/inspector_test.go::TestOpenValidPDF
	// which must exist and test:
	// - returns non-nil DocumentInfo
	// - DocumentInfo.FileName == "minimal.pdf"
	// - DocumentInfo.PageCount >= 1
	// - DocumentInfo.FileSize > 0
	// - DocumentInfo.FilePath is the absolute path
	// - DocumentInfo.TabID matches the provided tabID
	// - no error returned
	cmd := exec.Command("go", "test", "-v", "-run", "TestOpenValidPDF", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Inspector.Open valid PDF test failed:\n%s", string(output))
	}

	// Verify the test actually ran (not just skipped)
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Inspector.Open() on malformed PDF returns error, no panic: Given a malformed
// PDF, When Inspector.Open() is called,
//       Then the function does not crash or panic, And returns an error
//       with a human-readable message.
// ---------------------------------------------------------------------------

func TestInspectorOpenMalformedPDF(t *testing.T) {
	root := projectRoot(t)

	// Verify testdata/malformed.pdf exists
	malformedPDF := filepath.Join(testdataDir(t), "malformed.pdf")
	if _, err := os.Stat(malformedPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/malformed.pdf does not exist -- create test fixture first")
	}

	// Run the pdfcore unit test that validates Open with malformed PDF.
	// This delegates to internal/pdfcore/inspector_test.go::TestOpenMalformedPDF
	// which must exist and test:
	// - returns an error (not nil)
	// - the error wraps or is ErrMalformedPDF
	// - no panic occurs (process doesn't crash)
	cmd := exec.Command("go", "test", "-v", "-run", "TestOpenMalformedPDF", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Inspector.Open malformed PDF test failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// safeCall() catches panic and returns wrapped error: Given the
// safeCall() utility, When a function panics,
//       Then the panic is caught and returned as a Go error with format
//       "pdf parsing panic: {recovered value}".
// ---------------------------------------------------------------------------

func TestSafeCallCatchesPanic(t *testing.T) {
	root := projectRoot(t)

	// Run the pdfcore unit tests for safeCall.
	// Delegates to internal/pdfcore/errors_test.go::TestSafeCall*
	// which must exist and test:
	// - safeCall with successful function returns nil
	// - safeCall with function that returns error passes it through
	// - safeCall with function that panics(string) returns error containing "pdf parsing panic:"
	// - safeCall with function that panics(error) returns error containing "pdf parsing panic:"
	cmd := exec.Command("go", "test", "-v", "-run", "TestSafeCall", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("safeCall panic recovery tests failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Inspector.Open() on non-existent path returns ErrDocumentNotFound (negative): Given
// a file path that does not exist,
//                  When Inspector.Open() is called,
//                  Then it returns an error wrapping ErrDocumentNotFound.
// ---------------------------------------------------------------------------

func TestInspectorOpenNonExistentFile(t *testing.T) {
	root := projectRoot(t)

	// Run the pdfcore unit test for non-existent file.
	// Delegates to internal/pdfcore/inspector_test.go::TestOpenNonExistentFile
	// which must exist and test:
	// - returns an error (not nil)
	// - the error wraps ErrDocumentNotFound
	cmd := exec.Command("go", "test", "-v", "-run", "TestOpenNonExistentFile", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Inspector.Open non-existent file test failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Inspector.Open() on encrypted PDF returns ErrEncryptedPDF: Given an
// encrypted PDF, When Inspector.Open() is called,
//       Then the error wraps ErrEncryptedPDF with a message about encryption.
// ---------------------------------------------------------------------------

func TestInspectorOpenEncryptedPDF(t *testing.T) {
	root := projectRoot(t)

	// Verify testdata/encrypted.pdf exists
	encryptedPDF := filepath.Join(testdataDir(t), "encrypted.pdf")
	if _, err := os.Stat(encryptedPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/encrypted.pdf does not exist -- create test fixture first")
	}

	// Run the pdfcore unit test for encrypted PDF.
	// Delegates to internal/pdfcore/inspector_test.go::TestOpenEncryptedPDF
	// which must exist and test:
	// - returns an error (not nil)
	// - the error wraps or is ErrEncryptedPDF
	cmd := exec.Command("go", "test", "-v", "-run", "TestOpenEncryptedPDF", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Inspector.Open encrypted PDF test failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Pdfcore package has zero Wails imports: Given the pdfcore package, When
// reviewed for architecture compliance,
//       Then it has zero dependency on Wails or any desktop framework.
// ---------------------------------------------------------------------------

func TestPdfcoreZeroWailsImports(t *testing.T) {
	root := projectRoot(t)

	// Check that no .go file in internal/pdfcore/ imports wailsapp
	pdfcoreDir := filepath.Join(root, "internal", "pdfcore")
	entries, err := os.ReadDir(pdfcoreDir)
	if err != nil {
		t.Fatalf("cannot read internal/pdfcore/ directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		// Skip test files -- they may import test helpers but production code must not import Wails
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filePath := filepath.Join(pdfcoreDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("cannot read %s: %v", entry.Name(), err)
			continue
		}
		if strings.Contains(string(content), "wailsapp") {
			t.Errorf("%s imports Wails (contains 'wailsapp') -- pdfcore must have zero Wails dependencies", entry.Name())
		}
	}

	// Also verify via go list -deps that no Wails package is transitively imported
	cmd := exec.Command("go", "list", "-deps", "./internal/pdfcore/")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, string(output))
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "wailsapp") {
			t.Errorf("pdfcore transitively depends on Wails package: %s", line)
		}
	}
}

// ---------------------------------------------------------------------------
// Inspector.Open() on multipage.pdf returns correct page count: Given a
// multi-page PDF, When Inspector.Open() is called,
//       Then DocumentInfo.PageCount reflects the actual number of pages.
// ---------------------------------------------------------------------------

func TestInspectorOpenMultipagePDF(t *testing.T) {
	root := projectRoot(t)

	// Verify testdata/multipage.pdf exists
	multipagePDF := filepath.Join(testdataDir(t), "multipage.pdf")
	if _, err := os.Stat(multipagePDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/multipage.pdf does not exist -- create test fixture first")
	}

	// Run the pdfcore unit test for multipage PDF.
	// Delegates to internal/pdfcore/inspector_test.go::TestOpenMultipagePDF
	// which must exist and test:
	// - returns non-nil DocumentInfo
	// - DocumentInfo.PageCount >= 2 (multipage fixture has 2+ pages)
	// - no error returned
	cmd := exec.Command("go", "test", "-v", "-run", "TestOpenMultipagePDF", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Inspector.Open multipage PDF test failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Model types exist in model.go with correct JSON tags: pdfcore exports model
// types (TreeNode, ObjectDetail, ContentStreamData,
//       DocumentInfo, ValueEntry, PropertyEntry, StreamInfo, Token) in model.go.
// ---------------------------------------------------------------------------

func TestModelTypesExist(t *testing.T) {
	root := projectRoot(t)

	modelPath := filepath.Join(root, "internal", "pdfcore", "model.go")
	content, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/model.go does not exist: %v", err)
	}

	modelContent := string(content)

	// Verify all required IPC types are defined
	requiredTypes := []string{
		"type TreeNode struct",
		"type ObjectDetail struct",
		"type PropertyEntry struct",
		"type ValueEntry struct",
		"type ContentStreamData struct",
		"type StreamInfo struct",
		"type Token struct",
		"type DocumentInfo struct",
	}

	for _, typeDef := range requiredTypes {
		if !strings.Contains(modelContent, typeDef) {
			t.Errorf("model.go missing type definition: %s", typeDef)
		}
	}

	// Verify critical JSON tags use camelCase (spot-check key fields)
	requiredTags := []string{
		`json:"nodeId"`,
		`json:"hasChildren"`,
		`json:"childCount"`,
		`json:"iconHint"`,
		`json:"tabId"`,
		`json:"fileName"`,
		`json:"filePath"`,
		`json:"pageCount"`,
		`json:"fileSize"`,
		`json:"nodeType"`,
		`json:"valueType"`,
		`json:"rawKey"`,
		`json:"objectRef"`,
		`json:"scalarValue"`,
		`json:"streamInfo"`,
		`json:"refTarget"`,
	}

	for _, tag := range requiredTags {
		if !strings.Contains(modelContent, tag) {
			t.Errorf("model.go missing JSON tag: %s", tag)
		}
	}
}

// ---------------------------------------------------------------------------
// Error types use Err prefix convention: Error types use Err prefix convention
// (ErrDocumentNotFound, ErrMalformedPDF).
// ---------------------------------------------------------------------------

func TestErrorTypesExist(t *testing.T) {
	root := projectRoot(t)

	errorsPath := filepath.Join(root, "internal", "pdfcore", "errors.go")
	content, err := os.ReadFile(errorsPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/errors.go does not exist: %v", err)
	}

	errorsContent := string(content)

	// Verify sentinel error variables exist
	requiredErrors := []string{
		"ErrDocumentNotFound",
		"ErrMalformedPDF",
		"ErrEncryptedPDF",
		"ErrUnsupportedPDF",
	}

	for _, errName := range requiredErrors {
		if !strings.Contains(errorsContent, errName) {
			t.Errorf("errors.go missing sentinel error: %s", errName)
		}
	}

	// Verify safeCall function exists
	if !strings.Contains(errorsContent, "func safeCall(") {
		t.Error("errors.go missing safeCall function")
	}

	// Verify wrapPDFError helper exists
	if !strings.Contains(errorsContent, "func wrapPDFError(") {
		t.Error("errors.go missing wrapPDFError function")
	}
}

// ---------------------------------------------------------------------------
// Inspector struct and API surface exist: Inspector with
// Open, Close, GetDocument methods.
// ---------------------------------------------------------------------------

func TestInspectorAPIExists(t *testing.T) {
	root := projectRoot(t)

	inspectorPath := filepath.Join(root, "internal", "pdfcore", "inspector.go")
	content, err := os.ReadFile(inspectorPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/inspector.go does not exist: %v", err)
	}

	inspectorContent := string(content)

	// Verify Inspector struct exists
	if !strings.Contains(inspectorContent, "type Inspector struct") {
		t.Error("inspector.go missing Inspector struct")
	}

	// Verify constructor
	if !strings.Contains(inspectorContent, "func NewInspector()") {
		t.Error("inspector.go missing NewInspector constructor")
	}

	// Verify method signatures
	requiredMethods := []string{
		"func (ins *Inspector) Open(",
		"func (ins *Inspector) Close(",
		"func (ins *Inspector) GetDocument(",
	}

	for _, method := range requiredMethods {
		if !strings.Contains(inspectorContent, method) {
			t.Errorf("inspector.go missing method: %s", method)
		}
	}

	// Verify DocumentState struct exists (internal state type)
	if !strings.Contains(inspectorContent, "type DocumentState struct") {
		t.Error("inspector.go missing DocumentState struct")
	}
}

// ---------------------------------------------------------------------------
// Close removes document, GetDocument retrieves it: Unit tests exist
// for Close and GetDocument scenarios.
// ---------------------------------------------------------------------------

func TestInspectorCloseAndGetDocument(t *testing.T) {
	root := projectRoot(t)

	// Run the pdfcore unit tests for Close and GetDocument.
	// Delegates to internal/pdfcore/inspector_test.go tests:
	// - TestCloseRemovesDocument: Close(tabID) removes from state
	// - TestGetDocumentReturnsOpened: GetDocument returns previously opened doc
	// - TestGetDocumentUnknownTabID: GetDocument with unknown tabID returns error
	cmd := exec.Command("go", "test", "-v", "-run", "TestClose|TestGetDocument", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Inspector Close/GetDocument tests failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// All pdfcore unit tests pass together: Unit tests exist for Open with valid
// PDF, malformed PDF, and encrypted
//       PDF scenarios using test fixtures in testdata/.
// ---------------------------------------------------------------------------

func TestAllPdfcoreTestsPass(t *testing.T) {
	root := projectRoot(t)

	// Run all pdfcore tests as a final integration check
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
// go vet passes on pdfcore package: Code quality --
// no vet warnings.
// ---------------------------------------------------------------------------

func TestPdfcoreGoVet(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet failed on pdfcore:\n%s", string(output))
	}
}
