//go:build windows

// Windows sharing and lock violations are "busy, try again", not a parse
// failure.
//
// Run: cd code && go test -run TestFileBusy ./internal/pdfcore/

package pdfcore

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"syscall"
	"testing"
)

const (
	errorFileNotFound     = syscall.Errno(2)
	errorSharingViolation = syscall.Errno(32)
	errorLockViolation    = syscall.Errno(33)
)

func TestFileBusyRecognisesWindowsErrnos(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"sharing violation in a PathError", &fs.PathError{Op: "open", Path: "x.pdf", Err: errorSharingViolation}, true},
		{"lock violation in a PathError", &fs.PathError{Op: "read", Path: "x.pdf", Err: errorLockViolation}, true},
		{"bare sharing violation", errorSharingViolation, true},
		{"file not found", &fs.PathError{Op: "open", Path: "x.pdf", Err: errorFileNotFound}, false},
		{"unrelated error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		if got := isFileBusy(tc.err); got != tc.want {
			t.Errorf("%s: isFileBusy = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Opening a file another process holds without read sharing returns
// ErrFileBusy, not ErrMalformedPDF.
func TestFileBusyOpenOfExclusivelyLockedFile(t *testing.T) {
	path := copyFixtureToTemp(t, "minimal.pdf")
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("lock file: %v", err)
	}
	defer func() { _ = syscall.CloseHandle(h) }()

	ins := NewInspector()
	_, err = ins.Open("tab-busy", path)
	if !errors.Is(err, ErrFileBusy) {
		t.Fatalf("Open of a locked file: err = %v, want ErrFileBusy", err)
	}
	if errors.Is(err, ErrMalformedPDF) {
		t.Errorf("a busy file must not read as malformed: %v", err)
	}
	if !strings.Contains(err.Error(), "file is busy") {
		t.Errorf("error text %q must carry the busy marker", err.Error())
	}
}

// A plain-text read of a file another process holds without read sharing
// returns ErrFileBusy, not ErrMalformedPDF.
func TestFileBusyPlainTextReadOfExclusivelyLockedFile(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-busy-read")
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("lock file: %v", err)
	}
	defer func() { _ = syscall.CloseHandle(h) }()

	_, err = ins.GetPlainText(context.Background(), "tab-busy-read")
	if !errors.Is(err, ErrFileBusy) {
		t.Fatalf("GetPlainText of a locked file: err = %v, want ErrFileBusy", err)
	}
	if errors.Is(err, ErrMalformedPDF) {
		t.Errorf("a busy file must not read as malformed: %v", err)
	}
}
