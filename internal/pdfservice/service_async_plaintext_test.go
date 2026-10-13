// Async Plain Text Load with Cancel -- service-layer tests.
//
// These cover PDFService.GetPlainText cancellation via the passed context and
// PDFService.GetPlainTextSize.

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

// TestServiceGetPlainTextCancelsViaContext verifies cancelling the context
// passed to GetPlainText cancels an in-flight read (mirrors the inspector-level
// cancel test through the service binding). In production the frontend aborts
// the bound call, which cancels the Wails-injected request context; here the
// test drives that context directly.
//
// Uses a temporary copy of minimal.pdf padded out to 64 MiB so the chunked
// read loop has time to observe ctx.Done().
func TestServiceGetPlainTextCancelsViaContext(t *testing.T) {
	// Build a 64 MiB temp PDF (real header + pad).
	srcPath := filepath.Join(testdataDir(t), "minimal.pdf")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read minimal.pdf: %v", err)
	}
	tmp, err := os.CreateTemp("", "pdfservice-plaintext-oversize-*.pdf")
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
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_, err := svc.GetPlainText(ctx, info.TabID)
		resultCh <- result{err: err}
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

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
// GetPlainTextSize service binding.
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
