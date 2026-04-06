package pdfservice

import (
	"errors"
	"path/filepath"
	"testing"

	"unipdf-debugger/internal/pdfcore"
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
	defer svc.CloseDocument(info.TabID)

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
	defer svc.CloseDocument(info.TabID)

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
	defer svc.CloseDocument(info.TabID)

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
	defer svc.CloseDocument(info.TabID)

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
