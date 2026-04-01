package pdfcore

import (
	"errors"
	"strings"
	"testing"
)

func TestSafeCallSuccess(t *testing.T) {
	err := safeCall(func() error { return nil })
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestSafeCallReturnsError(t *testing.T) {
	want := errors.New("something went wrong")
	err := safeCall(func() error { return want })
	if err != want {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestSafeCallCatchesStringPanic(t *testing.T) {
	err := safeCall(func() error { panic("kaboom") })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "pdf parsing panic:") {
		t.Fatalf("expected 'pdf parsing panic:' in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("expected 'kaboom' in error, got %v", err)
	}
}

func TestSafeCallCatchesErrorPanic(t *testing.T) {
	err := safeCall(func() error { panic(errors.New("error panic")) })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "pdf parsing panic:") {
		t.Fatalf("expected 'pdf parsing panic:' in error, got %v", err)
	}
}

func TestWrapPDFErrorPasswordBecomesEncrypted(t *testing.T) {
	err := wrapPDFError(errors.New("pdfcpu: please provide the correct password"))
	if !errors.Is(err, ErrEncryptedPDF) {
		t.Fatalf("expected ErrEncryptedPDF, got %v", err)
	}
}

func TestWrapPDFErrorOwnerPasswordBecomesEncrypted(t *testing.T) {
	err := wrapPDFError(errors.New("please provide owner password"))
	if !errors.Is(err, ErrEncryptedPDF) {
		t.Fatalf("expected ErrEncryptedPDF, got %v", err)
	}
}

func TestWrapPDFErrorPanicPrefixBecomesMalformed(t *testing.T) {
	err := wrapPDFError(errors.New("pdf parsing panic: something with password in it"))
	if !errors.Is(err, ErrMalformedPDF) {
		t.Fatalf("expected ErrMalformedPDF, got %v", err)
	}
}

func TestWrapPDFErrorGenericBecomesMalformed(t *testing.T) {
	err := wrapPDFError(errors.New("unexpected EOF"))
	if !errors.Is(err, ErrMalformedPDF) {
		t.Fatalf("expected ErrMalformedPDF, got %v", err)
	}
}

func TestWrapPDFErrorPreservesOriginal(t *testing.T) {
	orig := errors.New("some pdfcpu error")
	wrapped := wrapPDFError(orig)
	if !strings.Contains(wrapped.Error(), "some pdfcpu error") {
		t.Fatalf("expected original message preserved, got %v", wrapped)
	}
}
