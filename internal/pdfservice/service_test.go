package pdfservice

import (
	"errors"
	"path/filepath"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata")
}

func TestNewPDFService(t *testing.T) {
	svc := NewPDFService(nil)
	if svc.inspector == nil {
		t.Fatal("NewPDFService(nil) returned service with nil inspector")
	}
}

func TestOpenFileValidPDF(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile returned error: %v", err)
	}
	if info == nil {
		t.Fatal("OpenFile returned nil DocumentInfo")
	}
	if info.TabID == "" {
		t.Error("DocumentInfo.TabID is empty")
	}
	if info.FileName != "minimal.pdf" {
		t.Errorf("FileName = %q, want %q", info.FileName, "minimal.pdf")
	}
	if info.PageCount < 1 {
		t.Errorf("PageCount = %d, want >= 1", info.PageCount)
	}

	// Clean up
	_ = svc.CloseDocument(info.TabID)
}

func TestOpenFileNonExistent(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile("/nonexistent/path/fake.pdf")
	if err == nil {
		t.Fatal("OpenFile with nonexistent path should return error")
	}
	if info != nil {
		t.Error("OpenFile with nonexistent path should return nil DocumentInfo")
	}
}

func TestOpenFileMalformed(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "malformed.pdf"))
	if err == nil {
		t.Fatal("OpenFile with malformed PDF should return error")
	}
	if !errors.Is(err, pdfcore.ErrMalformedPDF) {
		t.Errorf("expected ErrMalformedPDF, got: %v", err)
	}
	if info != nil {
		t.Error("OpenFile with malformed PDF should return nil DocumentInfo")
	}
}

func TestOpenFileEncrypted(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "encrypted.pdf"))
	if err == nil {
		t.Fatal("OpenFile with encrypted PDF should return error")
	}
	if !errors.Is(err, pdfcore.ErrEncryptedPDF) {
		t.Errorf("expected ErrEncryptedPDF, got: %v", err)
	}
	if info != nil {
		t.Error("OpenFile with encrypted PDF should return nil DocumentInfo")
	}
}

func TestCloseDocumentValid(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	err = svc.CloseDocument(info.TabID)
	if err != nil {
		t.Errorf("CloseDocument returned error: %v", err)
	}
}

func TestCloseDocumentUnknown(t *testing.T) {
	svc := NewPDFService(nil)
	err := svc.CloseDocument("nonexistent-tab-id")
	if err == nil {
		t.Fatal("CloseDocument with unknown tabID should return error")
	}
	if !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
}

func TestGetTreeRootValid(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	root, err := svc.GetTreeRoot(info.TabID)
	if err != nil {
		t.Fatalf("GetTreeRoot returned error: %v", err)
	}
	if root == nil {
		t.Fatal("GetTreeRoot returned nil")
	}
	if root.ID != "root" {
		t.Errorf("root.ID = %q, want %q", root.ID, "root")
	}
}

func TestGetTreeRootUnknown(t *testing.T) {
	svc := NewPDFService(nil)
	root, err := svc.GetTreeRoot("nonexistent-tab-id")
	if err == nil {
		t.Fatal("GetTreeRoot with unknown tabID should return error")
	}
	if root != nil {
		t.Error("GetTreeRoot with unknown tabID should return nil")
	}
}

func TestGetChildrenValid(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	children, err := svc.GetChildren(info.TabID, "root")
	if err != nil {
		t.Fatalf("GetChildren returned error: %v", err)
	}
	if len(children) == 0 {
		t.Error("GetChildren returned empty slice for root node")
	}
}

func TestGetChildrenUnknown(t *testing.T) {
	svc := NewPDFService(nil)
	children, err := svc.GetChildren("nonexistent-tab-id", "root")
	if err == nil {
		t.Fatal("GetChildren with unknown tabID should return error")
	}
	if children != nil {
		t.Error("GetChildren with unknown tabID should return nil")
	}
}

func TestGetObjectDetailValid(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	detail, err := svc.GetObjectDetail(info.TabID, "root")
	if err != nil {
		t.Fatalf("GetObjectDetail returned error: %v", err)
	}
	if detail == nil {
		t.Fatal("GetObjectDetail returned nil")
	}
	if detail.Type != "dict" {
		t.Errorf("Type = %q, want %q", detail.Type, "dict")
	}
	if detail.NodeID != "root" {
		t.Errorf("NodeID = %q, want %q", detail.NodeID, "root")
	}
	if len(detail.Properties) == 0 {
		t.Error("Properties is empty for catalog dict")
	}
}

func TestGetObjectDetailUnknown(t *testing.T) {
	svc := NewPDFService(nil)
	detail, err := svc.GetObjectDetail("nonexistent-tab-id", "root")
	if err == nil {
		t.Fatal("GetObjectDetail with unknown tabID should return error")
	}
	if !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
	if detail != nil {
		t.Error("GetObjectDetail with unknown tabID should return nil")
	}
}

func TestGetAncestorPathValid(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	path, err := svc.GetAncestorPath(info.TabID, "root")
	if err != nil {
		t.Fatalf("GetAncestorPath returned error: %v", err)
	}
	if len(path) != 1 || path[0] != "root" {
		t.Errorf("path = %v, want [root]", path)
	}
}

func TestGetAncestorPathUnknown(t *testing.T) {
	svc := NewPDFService(nil)
	path, err := svc.GetAncestorPath("nonexistent-tab-id", "root")
	if err == nil {
		t.Fatal("GetAncestorPath with unknown tabID should return error")
	}
	if !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
	if path != nil {
		t.Error("GetAncestorPath with unknown tabID should return nil")
	}
}

// --- GetContentStream tests (Story 3-1) ---

func TestGetContentStreamValid(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "content-stream.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	// Walk the tree to find a stream node
	var findStream func(nodeID string, depth int) string
	findStream = func(nodeID string, depth int) string {
		if depth > 4 {
			return ""
		}
		children, err := svc.GetChildren(info.TabID, nodeID)
		if err != nil {
			return ""
		}
		for _, c := range children {
			if c.NodeType == "stream" {
				return c.ID
			}
		}
		for _, c := range children {
			if c.HasChildren {
				found := findStream(c.ID, depth+1)
				if found != "" {
					return found
				}
			}
		}
		return ""
	}

	streamNodeID := findStream("root", 0)
	if streamNodeID == "" {
		t.Fatal("no stream node found in content-stream.pdf")
	}

	result, err := svc.GetContentStream(info.TabID, streamNodeID)
	if err != nil {
		t.Fatalf("GetContentStream returned error: %v", err)
	}
	if result == nil {
		t.Fatal("GetContentStream returned nil")
	}
	if result.Error != "" {
		t.Fatalf("ContentStreamData.Error = %q, want empty", result.Error)
	}
	if result.Raw == "" {
		t.Fatal("Raw is empty, want decoded content stream text")
	}
}

func TestGetContentStreamUnknown(t *testing.T) {
	svc := NewPDFService(nil)
	_, err := svc.GetContentStream("nonexistent-tab-id", "root")
	if err == nil {
		t.Fatal("GetContentStream with unknown tabID should return error")
	}
	if !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
}

// --- GetImageData tests (Story 6-1) ---

func TestGetImageData(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "image-xobject.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	// Walk tree to find an image node
	var findImage func(nodeID string, depth int) string
	findImage = func(nodeID string, depth int) string {
		if depth > 5 {
			return ""
		}
		children, err := svc.GetChildren(info.TabID, nodeID)
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
				found := findImage(c.ID, depth+1)
				if found != "" {
					return found
				}
			}
		}
		return ""
	}

	imageNodeID := findImage("root", 0)
	if imageNodeID == "" {
		t.Fatal("no image node found in image-xobject.pdf")
	}

	result, err := svc.GetImageData(info.TabID, imageNodeID)
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
		t.Fatal("Base64 is empty")
	}
	if result.MimeType != "image/jpeg" && result.MimeType != "image/png" {
		t.Errorf("MimeType = %q, want image/jpeg or image/png", result.MimeType)
	}
}

func TestGetImageDataUnknownTab(t *testing.T) {
	svc := NewPDFService(nil)
	_, err := svc.GetImageData("nonexistent-tab-id", "root")
	if err == nil {
		t.Fatal("GetImageData with unknown tabID should return error")
	}
	if !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	svc := NewPDFService(nil)

	// Open
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	if info.TabID == "" {
		t.Fatal("TabID is empty")
	}

	// GetTreeRoot
	root, err := svc.GetTreeRoot(info.TabID)
	if err != nil {
		t.Fatalf("GetTreeRoot failed: %v", err)
	}
	if root.ID != "root" {
		t.Errorf("root.ID = %q, want %q", root.ID, "root")
	}

	// GetChildren
	children, err := svc.GetChildren(info.TabID, "root")
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}
	if len(children) == 0 {
		t.Error("GetChildren returned empty slice")
	}

	// Close
	err = svc.CloseDocument(info.TabID)
	if err != nil {
		t.Fatalf("CloseDocument failed: %v", err)
	}
}

// 9.4-UNIT-001: GoToPage exposes the pdfcore page-content-stream resolver to
// the Wails service layer. Valid page resolves to a non-empty node ID;
// out-of-range and unknown-tab paths surface as errors that map to user-facing
// error messages on the frontend.

func TestGoToPageValid(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "content-stream.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	nodeID, err := svc.GoToPage(info.TabID, 1)
	if err != nil {
		t.Fatalf("GoToPage(1) returned error: %v", err)
	}
	if nodeID == "" {
		t.Error("GoToPage(1) returned empty node ID")
	}
}

func TestGoToPageOutOfRange(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "content-stream.pdf"))
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	if _, err := svc.GoToPage(info.TabID, 9999); err == nil {
		t.Fatal("GoToPage with out-of-range page should error, got nil")
	}
}

func TestGoToPageUnknownTab(t *testing.T) {
	svc := NewPDFService(nil)
	_, err := svc.GoToPage("does-not-exist", 1)
	if err == nil {
		t.Fatal("GoToPage with unknown tab should error, got nil")
	}
	if !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("err = %v, want errors.Is(...,ErrDocumentNotFound)", err)
	}
}
