// Package image_extraction_test provides acceptance tests for Image Extraction
// in PDF Core.
//
// Test Levels: Unit (Go) and Integration (Go) -- pdfcore API validation.
// No browser interaction required; all criteria are Go package validation.
//
// Run: cd tests/image-extraction && go test -v -count=1 ./...
package image_extraction_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGoTest runs "go test" with the given -run pattern against the given
// package path from the project root. It fails the calling test if the
// delegated test exits non-zero, does not contain "PASS", or if no tests
// matched the -run pattern (which means the unit test does not exist yet).
func runGoTest(t *testing.T, runPattern, pkgPath string) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-v", "-run", runPattern, "-count=1", pkgPath)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil {
		t.Fatalf("delegated test failed:\n%s", out)
	}
	if strings.Contains(out, "no tests to run") {
		t.Fatalf("no unit tests matched pattern %q -- unit test does not exist yet:\n%s", runPattern, out)
	}
	if !strings.Contains(out, "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", out)
	}
}

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
// GetImageData extracts image from XObject Image stream and returns
// base64-encoded PNG/JPEG with correct MIME type.
// Given a tree node corresponding to an XObject image (Subtype=Image),
// When GetImageData(tabID, nodeID) is called,
// Then the image is extracted and encoded as base64,
// And mimeType is "image/png" or "image/jpeg",
// And the exit is non-error.
// Given a DCTDecode (JPEG) image, raw JPEG bytes are base64-encoded
// directly and mimeType is "image/jpeg".
// Given a FlateDecode image, the stream is decompressed and assembled
// into a PNG with mimeType "image/png".
// ---------------------------------------------------------------------------

func TestGetImageDataExtractsImage(t *testing.T) {
	// Verify fixture exists
	imagePDF := filepath.Join(testdataDir(t), "image-xobject.pdf")
	if _, err := os.Stat(imagePDF); os.IsNotExist(err) {
		t.Fatalf("testdata/image-xobject.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/image_test.go::TestGetImageData_ExtractsJPEGImage
	// which must test:
	// - open image-xobject.pdf, find an image node (iconHint == "image")
	// - call GetImageData, verify non-empty Base64
	// - verify MimeType is "image/jpeg" or "image/png"
	// - verify Error is empty
	runGoTest(t, "TestGetImageData_ExtractsJPEGImage", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// GetImageData returns image metadata: width, height, colorSpace,
// bitsPerComponent, filter.
// Given an XObject image node, When GetImageData is called,
// Then image metadata is returned with all numeric dimensions > 0.
// ---------------------------------------------------------------------------

func TestGetImageDataReturnsMetadata(t *testing.T) {
	// Delegates to internal/pdfcore/image_test.go::TestGetImageData_ReturnsMetadata
	// which must test:
	// - Width > 0, Height > 0
	// - ColorSpace non-empty (e.g., "DeviceRGB")
	// - BitsPerComponent > 0
	// - Filter non-empty (e.g., "DCTDecode" or "FlateDecode")
	runGoTest(t, "TestGetImageData_ReturnsMetadata", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// GetImageData on corrupted/invalid image stream returns error without
// panic. safeCall wraps all pdfcpu calls.
// Given an image that cannot be extracted (corrupted data),
// When extraction is attempted,
// Then ImageData.Error is populated with a descriptive message,
// And no panic occurs,
// And the function returns the ImageData struct (not a Go error),
// And metadata fields are still populated where possible.
// ---------------------------------------------------------------------------

func TestGetImageDataPanicRecovery(t *testing.T) {
	// Delegates to internal/pdfcore/image_test.go::TestGetImageData_PanicRecovery
	// which must test:
	// - a malformed stream dict does not cause a panic
	// - ImageData.Error is non-empty
	// - function returns *ImageData, not a Go error
	runGoTest(t, "TestGetImageData_PanicRecovery", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// GetImageData with empty nodeID returns Go error.
// Guard: Empty nodeID is a programmer error, returns Go error
// wrapping ErrDocumentNotFound (matches existing pattern).
// ---------------------------------------------------------------------------

func TestGetImageDataEmptyNodeID(t *testing.T) {
	// Delegates to internal/pdfcore/image_test.go::TestGetImageData_InvalidNodeID
	// which must test:
	// - calling GetImageData with empty nodeID returns a Go error (not nil)
	runGoTest(t, "TestGetImageData_InvalidNodeID", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// GetImageData on non-image node returns descriptive error.
// Given a node that is NOT an XObject image (e.g., a dict, a Form
// XObject, a content stream),
// When GetImageData(tabID, nodeID) is called,
// Then ImageData.Error is populated with "not an image XObject",
// And no panic occurs.
// ---------------------------------------------------------------------------

func TestGetImageDataNonImageNode(t *testing.T) {
	// Delegates to internal/pdfcore/image_test.go::TestGetImageData_NonImageNode
	// which must test:
	// - call GetImageData with a dict node (e.g., the catalog "root")
	// - verify ImageData.Error contains "not an image"
	// - verify no panic
	runGoTest(t, "TestGetImageData_NonImageNode", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// GetImageData with unknown tabID returns ErrDocumentNotFound.
// ---------------------------------------------------------------------------

func TestGetImageDataUnknownTab(t *testing.T) {
	// Delegates to internal/pdfcore/image_test.go::TestGetImageData_UnknownTab
	// which must test:
	// - calling GetImageData with a nonexistent tabID returns ErrDocumentNotFound
	runGoTest(t, "TestGetImageData_UnknownTab", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// GetImageData with error-prefixed nodeID returns ImageData.Error,
// no panic.
// Graceful degradation for error nodes.
// ---------------------------------------------------------------------------

func TestGetImageDataErrorNode(t *testing.T) {
	// Delegates to internal/pdfcore/image_test.go::TestGetImageData_ErrorNode
	// which must test:
	// - calling GetImageData with "error:something" nodeID
	// - verify ImageData.Error is non-empty
	// - verify no panic
	runGoTest(t, "TestGetImageData_ErrorNode", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// image.go exists in internal/pdfcore: Image extraction logic lives in
// pdfcore/image.go (mirrors stream.go).
// ---------------------------------------------------------------------------

func TestImageFileExists(t *testing.T) {
	root := projectRoot(t)

	imagePath := filepath.Join(root, "internal", "pdfcore", "image.go")
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		t.Fatal("internal/pdfcore/image.go does not exist")
	}
}

// ---------------------------------------------------------------------------
// Inspector.GetImageData method exists in image.go: Method signature
// present in pdfcore.
// ---------------------------------------------------------------------------

func TestGetImageDataMethodExists(t *testing.T) {
	root := projectRoot(t)

	imagePath := filepath.Join(root, "internal", "pdfcore", "image.go")
	content, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("cannot read image.go: %v", err)
	}
	if !strings.Contains(string(content), "func (ins *Inspector) GetImageData(") {
		t.Error("image.go missing GetImageData method on Inspector")
	}
}

// ---------------------------------------------------------------------------
// ImageData model type exists in model.go with correct JSON tags.
// ImageData struct with all required fields for frontend display.
// ---------------------------------------------------------------------------

func TestImageDataModelExists(t *testing.T) {
	root := projectRoot(t)

	modelPath := filepath.Join(root, "internal", "pdfcore", "model.go")
	content, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatalf("cannot read model.go: %v", err)
	}

	modelContent := string(content)

	if !strings.Contains(modelContent, "type ImageData struct") {
		t.Fatal("model.go missing ImageData struct")
	}

	// Verify required JSON tags
	requiredTags := []string{
		`json:"nodeId"`,
		`json:"objectRef"`,
		`json:"mimeType"`,
		`json:"base64"`,
		`json:"width"`,
		`json:"height"`,
		`json:"colorSpace"`,
		`json:"bitsPerComponent"`,
		`json:"filter"`,
		`json:"warning"`,
		`json:"error"`,
	}

	for _, tag := range requiredTags {
		if !strings.Contains(modelContent, tag) {
			t.Errorf("ImageData missing JSON tag: %s", tag)
		}
	}
}

// ---------------------------------------------------------------------------
// PDFService.GetImageData binding exists in service.go: Thin adapter
// pass-through from pdfservice to pdfcore.
// ---------------------------------------------------------------------------

func TestServiceGetImageDataMethodExists(t *testing.T) {
	root := projectRoot(t)

	servicePath := filepath.Join(root, "internal", "pdfservice", "service.go")
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("cannot read service.go: %v", err)
	}
	if !strings.Contains(string(content), "func (s *PDFService) GetImageData(") {
		t.Error("service.go missing GetImageData method on PDFService")
	}
}

// ---------------------------------------------------------------------------
// PDFService.GetImageData delegates to Inspector and returns data to
// frontend caller.
// Service layer integration -- call through pdfservice with real PDF.
// ---------------------------------------------------------------------------

func TestPDFServiceGetImageData(t *testing.T) {
	// Run pdfservice-level test that validates GetImageData binding
	runGoTest(t, "TestGetImageData", "./internal/pdfservice/...")
}

// ---------------------------------------------------------------------------
// testdata/image-xobject.pdf exists with embedded images. Prerequisite for
// all image extraction tests.
// ---------------------------------------------------------------------------

func TestImageTestDataExists(t *testing.T) {
	imagePDF := filepath.Join(testdataDir(t), "image-xobject.pdf")
	if _, err := os.Stat(imagePDF); os.IsNotExist(err) {
		t.Fatal("testdata/image-xobject.pdf does not exist -- must be created for image extraction tests")
	}
}

// ---------------------------------------------------------------------------
// All pdfcore tests still pass (no regression).
// ---------------------------------------------------------------------------

func TestPdfcoreNoRegression(t *testing.T) {
	root := projectRoot(t)

	// Run all pdfcore tests (no -run filter -- regression check, not targeted)
	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfcore regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// All pdfservice tests still pass (no regression).
// ---------------------------------------------------------------------------

func TestPdfserviceNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfservice regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// go vet passes on pdfcore package.
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

// ---------------------------------------------------------------------------
// pdfcore/image.go has zero Wails imports. Architecture compliance:
// pdfcore must not depend on desktop framework.
// ---------------------------------------------------------------------------

func TestImageFileZeroWailsImports(t *testing.T) {
	root := projectRoot(t)

	imagePath := filepath.Join(root, "internal", "pdfcore", "image.go")
	content, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("cannot read image.go: %v", err)
	}
	if strings.Contains(string(content), "wailsapp") {
		t.Error("image.go imports Wails (contains 'wailsapp') -- pdfcore must have zero Wails dependencies")
	}
}

// ---------------------------------------------------------------------------
// DCTDecode image returns "image/jpeg" MIME type and filter metadata
// contains "DCTDecode".
// Given a DCTDecode (JPEG) image, raw JPEG bytes are base64-encoded
// directly and mimeType is "image/jpeg".
// ---------------------------------------------------------------------------

func TestGetImageDataDCTDecodeJPEG(t *testing.T) {
	runGoTest(t, "TestGetImageData_DCTDecodeJPEG", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// GetImageData on a StreamDict with wrong Subtype (e.g., page content
// stream) returns "not an image" error.
// Form XObject / non-image stream distinction.
// ---------------------------------------------------------------------------

func TestGetImageDataStreamDictNonImage(t *testing.T) {
	runGoTest(t, "TestGetImageData_StreamDictNonImage", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// GetImageData returns consistent results across multiple calls -- no stale
// state between extractions.
// Idempotency / no caching side effects.
// ---------------------------------------------------------------------------

func TestGetImageDataIdempotency(t *testing.T) {
	runGoTest(t, "TestGetImageData_Idempotency", "./internal/pdfcore/...")
}
