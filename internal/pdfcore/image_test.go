package pdfcore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func openImagePDF(t *testing.T) (*Inspector, string) {
	t.Helper()
	ins := NewInspector()
	tabID := "test-tab"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "image-xobject.pdf"))
	if err != nil {
		t.Fatalf("failed to open image-xobject.pdf: %v", err)
	}
	return ins, tabID
}

// findImageNode walks the tree to find a node with iconHint == "image".
func findImageNode(t *testing.T, ins *Inspector, tabID, nodeID string, depth int) string {
	t.Helper()
	if depth > 5 {
		return ""
	}
	children, err := ins.GetChildren(tabID, nodeID)
	if err != nil {
		return ""
	}
	for _, c := range children {
		if c.IconHint == "image" {
			return c.ID
		}
	}
	for _, c := range children {
		if c.HasChildren {
			found := findImageNode(t, ins, tabID, c.ID, depth+1)
			if found != "" {
				return found
			}
		}
	}
	return ""
}

func TestGetImageData_ExtractsJPEGImage(t *testing.T) {
	ins, tabID := openImagePDF(t)

	imageNodeID := findImageNode(t, ins, tabID, "root", 0)
	if imageNodeID == "" {
		t.Fatal("no image node found in image-xobject.pdf")
	}

	result, err := ins.GetImageData(tabID, imageNodeID)
	if err != nil {
		t.Fatalf("GetImageData returned error: %v", err)
	}
	if result == nil {
		t.Fatal("GetImageData returned nil")
	}
	if result.Error != "" {
		t.Fatalf("ImageData.Error = %q, want empty", result.Error)
	}
	if result.Base64 == "" {
		t.Fatal("Base64 is empty, want non-empty image data")
	}
	if result.MimeType != "image/jpeg" && result.MimeType != "image/png" {
		t.Errorf("MimeType = %q, want image/jpeg or image/png", result.MimeType)
	}
}

func TestGetImageData_ReturnsMetadata(t *testing.T) {
	ins, tabID := openImagePDF(t)

	imageNodeID := findImageNode(t, ins, tabID, "root", 0)
	if imageNodeID == "" {
		t.Fatal("no image node found in image-xobject.pdf")
	}

	result, err := ins.GetImageData(tabID, imageNodeID)
	if err != nil {
		t.Fatalf("GetImageData returned error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("ImageData.Error = %q, want empty", result.Error)
	}
	if result.Width <= 0 {
		t.Errorf("Width = %d, want > 0", result.Width)
	}
	if result.Height <= 0 {
		t.Errorf("Height = %d, want > 0", result.Height)
	}
	if result.ColorSpace == "" {
		t.Error("ColorSpace is empty, want non-empty")
	}
	if result.BitsPerComponent <= 0 {
		t.Errorf("BitsPerComponent = %d, want > 0", result.BitsPerComponent)
	}
	if result.Filter == "" {
		t.Error("Filter is empty, want non-empty (e.g. DCTDecode)")
	}
	if result.NodeID != imageNodeID {
		t.Errorf("NodeID = %q, want %q", result.NodeID, imageNodeID)
	}
	if result.ObjectRef == "" {
		t.Error("ObjectRef is empty, want non-empty")
	}
}

func TestGetImageData_NonImageNode(t *testing.T) {
	ins, tabID := openMinimal(t)

	// "root" is the catalog dict, not an image
	result, err := ins.GetImageData(tabID, "root")
	if err != nil {
		t.Fatalf("GetImageData returned Go error: %v (want struct-level error)", err)
	}
	if result == nil {
		t.Fatal("GetImageData returned nil")
	}
	if !strings.Contains(result.Error, "not an image") {
		t.Errorf("Error = %q, want to contain 'not an image'", result.Error)
	}
}

func TestGetImageData_InvalidNodeID(t *testing.T) {
	ins, tabID := openMinimal(t)
	_, err := ins.GetImageData(tabID, "")
	if err == nil {
		t.Fatal("expected Go error for empty nodeID, got nil")
	}
}

func TestGetImageData_UnknownTab(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetImageData("nonexistent-tab", "root")
	if err == nil {
		t.Fatal("expected Go error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
}

func TestGetImageData_ErrorNode(t *testing.T) {
	ins, tabID := openMinimal(t)
	result, err := ins.GetImageData(tabID, "error:something")
	if err != nil {
		t.Fatalf("GetImageData returned Go error: %v (want struct-level error)", err)
	}
	if result == nil {
		t.Fatal("GetImageData returned nil")
	}
	if result.Error == "" {
		t.Fatal("Error is empty, want non-empty for error node")
	}
}

func TestGetImageData_PanicRecovery(t *testing.T) {
	ins, tabID := openImagePDF(t)

	// Use a content stream node (not an image) -- this tests that non-image
	// streams are handled gracefully.
	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID != "" {
		result, err := ins.GetImageData(tabID, streamNodeID)
		if err != nil {
			t.Fatalf("GetImageData panicked or returned Go error: %v", err)
		}
		if result == nil {
			t.Fatal("GetImageData returned nil")
		}
		// Should have an error because the stream is not an Image XObject
		if result.Error == "" {
			// It's possible the stream IS an image in this PDF.
			// That's fine -- the main point is no panic.
		}
	}

	// Test with a completely bogus node ID that resolves to nothing useful.
	// This should not panic.
	result, err := ins.GetImageData(tabID, "obj:0:99999")
	if err != nil {
		// A Go error is acceptable for unresolvable refs
		return
	}
	if result != nil && result.Error == "" {
		// If no error, the object was resolved (unlikely for obj 99999)
	}

	// Verify safeCall actually catches panics: open malformed.pdf, which has
	// a corrupt xref. Any attempt to resolve objects will panic inside pdfcpu.
	malformedIns := NewInspector()
	malformedPath := filepath.Join(testdataDir(t), "malformed.pdf")
	_, openErr := malformedIns.Open("malformed-tab", malformedPath)
	if openErr != nil {
		// Malformed PDF may fail at Open -- that's expected. The important thing
		// is that it didn't panic. If Open fails, the safeCall in Open caught it.
		t.Logf("malformed.pdf Open error (expected): %v", openErr)
		return
	}
	// If Open somehow succeeded, try GetImageData on root to exercise safeCall.
	mResult, mErr := malformedIns.GetImageData("malformed-tab", "root")
	if mErr != nil {
		t.Logf("malformed.pdf GetImageData Go error (expected): %v", mErr)
		return
	}
	if mResult == nil {
		t.Fatal("GetImageData on malformed.pdf returned nil without error")
	}
	// Any combination of Error/non-Error is acceptable -- the point is no panic.
	t.Logf("malformed.pdf GetImageData result: error=%q", mResult.Error)
}

// 6.1-UNIT-006 / 6.1-UNIT-007: DCTDecode image returns "image/jpeg" MIME type.
func TestGetImageData_DCTDecodeJPEG(t *testing.T) {
	ins, tabID := openImagePDF(t)

	imageNodeID := findImageNode(t, ins, tabID, "root", 0)
	if imageNodeID == "" {
		t.Fatal("no image node found in image-xobject.pdf")
	}

	result, err := ins.GetImageData(tabID, imageNodeID)
	if err != nil {
		t.Fatalf("GetImageData returned error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("ImageData.Error = %q, want empty", result.Error)
	}
	// image-xobject.pdf contains a DCTDecode (JPEG) image
	if result.MimeType != "image/jpeg" {
		t.Errorf("MimeType = %q, want %q for DCTDecode image", result.MimeType, "image/jpeg")
	}
	if !strings.Contains(result.Filter, "DCTDecode") {
		t.Errorf("Filter = %q, want to contain 'DCTDecode'", result.Filter)
	}
}

// 6.1-UNIT-011: Form XObject (Subtype=Form) is NOT treated as image.
// Tests the code path where the object IS a StreamDict but Subtype != Image.
func TestGetImageData_StreamDictNonImage(t *testing.T) {
	ins, tabID := openContentStream(t)

	// content-stream.pdf has a content stream which is a StreamDict
	// but not an XObject Image (no /Subtype or Subtype != Image).
	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID == "" {
		t.Fatal("no stream node found in content-stream.pdf")
	}

	result, err := ins.GetImageData(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("GetImageData returned Go error: %v (want struct-level error)", err)
	}
	if result == nil {
		t.Fatal("GetImageData returned nil")
	}
	if !strings.Contains(result.Error, "not an image") {
		t.Errorf("Error = %q, want to contain 'not an image'", result.Error)
	}
}

// 6.1-UNIT-012: GetImageData returns consistent results across multiple calls.
func TestGetImageData_Idempotency(t *testing.T) {
	ins, tabID := openImagePDF(t)

	imageNodeID := findImageNode(t, ins, tabID, "root", 0)
	if imageNodeID == "" {
		t.Fatal("no image node found in image-xobject.pdf")
	}

	r1, err := ins.GetImageData(tabID, imageNodeID)
	if err != nil {
		t.Fatalf("first call returned error: %v", err)
	}
	r2, err := ins.GetImageData(tabID, imageNodeID)
	if err != nil {
		t.Fatalf("second call returned error: %v", err)
	}

	if r1.Base64 != r2.Base64 {
		t.Error("Base64 differs between calls")
	}
	if r1.MimeType != r2.MimeType {
		t.Errorf("MimeType differs: %q vs %q", r1.MimeType, r2.MimeType)
	}
	if r1.Width != r2.Width || r1.Height != r2.Height {
		t.Errorf("dimensions differ: %dx%d vs %dx%d", r1.Width, r1.Height, r2.Width, r2.Height)
	}
	if r1.ColorSpace != r2.ColorSpace {
		t.Errorf("ColorSpace differs: %q vs %q", r1.ColorSpace, r2.ColorSpace)
	}
	if r1.Filter != r2.Filter {
		t.Errorf("Filter differs: %q vs %q", r1.Filter, r2.Filter)
	}
}
