// Story 10-1: Async Plain Text Load with Cancel -- service-layer tests.
//
// These cover PDFService.CancelPlainText and PDFService.GetPlainTextSize.

package pdfservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// TestServiceCancelPlainTextUnknownTab verifies the unknown-tab path on the
// new CancelPlainText service binding (routed through the thin pdfservice
// adapter).
func TestServiceCancelPlainTextUnknownTab(t *testing.T) {
	svc := NewPDFService(nil)
	err := svc.CancelPlainText("nonexistent-tab-id")
	if err == nil {
		t.Fatal("CancelPlainText on unknown tab should return error")
	}
	if !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// TestServiceCancelPlainTextValidNoOp verifies CancelPlainText on a known tab
// with no load in flight returns nil ("no-op if no load is in flight").
func TestServiceCancelPlainTextValidNoOp(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	if err := svc.CancelPlainText(info.TabID); err != nil {
		t.Errorf("CancelPlainText on idle tab: err = %v, want nil", err)
	}
}

// TestServiceCancelPlainTextCancelsInFlight verifies CancelPlainText cancels
// an in-flight GetPlainText (mirrors the inspector-level cancel test through
// the service binding). The story Task 2.2 explicitly requests this test.
//
// Uses a temporary copy of minimal.pdf padded out to 64 MiB so the chunked
// read loop has time to observe ctx.Done().
func TestServiceCancelPlainTextCancelsInFlight(t *testing.T) {
	// Build a 64 MiB temp PDF (real header + pad).
	srcPath := filepath.Join(testdataDir(t), "minimal.pdf")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read minimal.pdf: %v", err)
	}
	tmp, err := os.CreateTemp("", "pdfservice-10-1-oversize-*.pdf")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := tmp.Write(src); err != nil {
		t.Fatalf("write header: %v", err)
	}
	pad := make([]byte, 4096)
	for i := range pad {
		pad[i] = 'X'
	}
	const totalSize = int64(64) * 1024 * 1024
	remaining := totalSize - int64(len(src))
	for remaining > 0 {
		n := int64(len(pad))
		if n > remaining {
			n = remaining
		}
		if _, err := tmp.Write(pad[:n]); err != nil {
			t.Fatalf("pad: %v", err)
		}
		remaining -= n
	}
	_ = tmp.Close()
	path := tmp.Name()
	defer func() { _ = os.Remove(path) }()

	svc := NewPDFService(nil)
	info, err := svc.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	type result struct {
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		_, err := svc.GetPlainText(info.TabID)
		resultCh <- result{err: err}
	}()

	time.Sleep(10 * time.Millisecond)

	if err := svc.CancelPlainText(info.TabID); err != nil {
		t.Fatalf("CancelPlainText: %v", err)
	}

	select {
	case r := <-resultCh:
		if r.err == nil {
			t.Fatalf("GetPlainText returned no error -- cancel did not preempt the read")
		}
		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("err = %v, want errors.Is(..., context.Canceled)", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("GetPlainText did not return within 5s of cancel")
	}
}

// TestServiceGetPlainTextSizeUnknownTab verifies the unknown-tab path on the
// new GetPlainTextSize service binding.
func TestServiceGetPlainTextSizeUnknownTab(t *testing.T) {
	svc := NewPDFService(nil)
	_, err := svc.GetPlainTextSize("nonexistent-tab-id")
	if err == nil {
		t.Fatal("GetPlainTextSize on unknown tab should return error")
	}
	if !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// TestServiceGetPlainTextSizeValid verifies GetPlainTextSize returns the
// on-disk byte size for a known tab.
func TestServiceGetPlainTextSizeValid(t *testing.T) {
	svc := NewPDFService(nil)
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()

	size, err := svc.GetPlainTextSize(info.TabID)
	if err != nil {
		t.Fatalf("GetPlainTextSize: %v", err)
	}
	fi, err := os.Stat(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	if size != fi.Size() {
		t.Errorf("GetPlainTextSize = %d, want %d", size, fi.Size())
	}
}
