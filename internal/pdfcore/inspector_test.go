package pdfcore

import (
	"errors"
	"path/filepath"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata")
}

func TestOpenValidPDF(t *testing.T) {
	ins := NewInspector()
	info, err := ins.Open("tab-1", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.TabID != "tab-1" {
		t.Errorf("TabID = %q, want %q", info.TabID, "tab-1")
	}
	if info.FileName != "minimal.pdf" {
		t.Errorf("FileName = %q, want %q", info.FileName, "minimal.pdf")
	}
	if info.PageCount < 1 {
		t.Errorf("PageCount = %d, want >= 1", info.PageCount)
	}
	if info.FileSize <= 0 {
		t.Errorf("FileSize = %d, want > 0", info.FileSize)
	}
	if info.FilePath == "" {
		t.Error("FilePath is empty")
	}
	if !filepath.IsAbs(info.FilePath) {
		t.Errorf("FilePath %q is not absolute", info.FilePath)
	}
}

func TestOpenMultipagePDF(t *testing.T) {
	ins := NewInspector()
	info, err := ins.Open("tab-2", filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.PageCount < 2 {
		t.Errorf("PageCount = %d, want >= 2", info.PageCount)
	}
}

func TestOpenMalformedPDF(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-3", filepath.Join(testdataDir(t), "malformed.pdf"))
	if err == nil {
		t.Fatal("expected error for malformed PDF, got nil")
	}
	if !errors.Is(err, ErrMalformedPDF) {
		t.Errorf("expected ErrMalformedPDF, got %v", err)
	}
}

func TestOpenEncryptedPDF(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-4", filepath.Join(testdataDir(t), "encrypted.pdf"))
	if err == nil {
		t.Fatal("expected error for encrypted PDF, got nil")
	}
	if !errors.Is(err, ErrEncryptedPDF) {
		t.Errorf("expected ErrEncryptedPDF, got %v", err)
	}
}

func TestOpenNonExistentFile(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-5", "/nonexistent/path/to/file.pdf")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestCloseRemovesDocument(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-6", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := ins.Close("tab-6"); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	_, err = ins.GetDocument("tab-6")
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
}

func TestGetDocumentReturnsOpened(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-7", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	doc, err := ins.GetDocument("tab-7")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc.PageCount < 1 {
		t.Errorf("PageCount = %d, want >= 1", doc.PageCount)
	}
}

func TestGetDocumentUnknownTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetDocument("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestOpenEmptyTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err == nil {
		t.Fatal("expected error for empty tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestOpenEmptyFilePath(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-x", "")
	if err == nil {
		t.Fatal("expected error for empty filePath, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestOpenDirectoryPath(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-x", testdataDir(t))
	if err == nil {
		t.Fatal("expected error for directory path, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestOpenOverwritesSameTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-dup", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	info, err := ins.Open("tab-dup", filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	if info.FileName != "multipage.pdf" {
		t.Errorf("FileName = %q, want %q", info.FileName, "multipage.pdf")
	}
	doc, err := ins.GetDocument("tab-dup")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc.PageCount < 2 {
		t.Errorf("PageCount = %d, want >= 2 (multipage)", doc.PageCount)
	}
}

func TestCloseUnknownTabID(t *testing.T) {
	ins := NewInspector()
	err := ins.Close("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}
