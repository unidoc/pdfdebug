//go:build windows

// Windows sharing and lock violations are "busy, try again", not a parse
// failure.
//
// Run: cd code && go test -run TestFileBusy ./internal/pdfcore/

package pdfcore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"syscall"
	"testing"
	"unsafe"
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

// Opening a file another handle holds an exclusive byte-range lock on returns
// ErrFileBusy, not ErrMalformedPDF.
func TestFileBusyOpenOfExclusivelyLockedFile(t *testing.T) {
	path := copyFixtureToTemp(t, "minimal.pdf")
	lockWholeFile(t, path)

	ins := NewInspector()
	_, err := ins.Open("tab-busy", path)
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

// A plain-text read of a file another handle holds an exclusive byte-range
// lock on returns ErrFileBusy, not ErrMalformedPDF.
func TestFileBusyPlainTextReadOfExclusivelyLockedFile(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-busy-read")
	lockWholeFile(t, path)

	_, err := ins.GetPlainText(context.Background(), "tab-busy-read")
	if !errors.Is(err, ErrFileBusy) {
		t.Fatalf("GetPlainText of a locked file: err = %v, want ErrFileBusy", err)
	}
	if errors.Is(err, ErrMalformedPDF) {
		t.Errorf("a busy file must not read as malformed: %v", err)
	}
}

// A signature hole read of a file another handle holds an exclusive
// byte-range lock on is noted as busy on the signature, not a hole mismatch.
func TestFileBusySignaturesHoleReadOfLockedFile(t *testing.T) {
	ins, path := openTempCopy(t, "signed.pdf", "tab-busy-sig")
	lockWholeFile(t, path)

	list, err := ins.GetSignatures("tab-busy-sig")
	if err != nil {
		t.Fatalf("GetSignatures of a locked file: %v", err)
	}
	if len(list.Signatures) == 0 || list.Signatures[0].HoleCheckError != ErrFileBusy.Error() {
		t.Fatalf("signatures %+v, want holeCheckError %q", list.Signatures, ErrFileBusy.Error())
	}
}

// Copying the raw bytes of a file another handle holds an exclusive byte-range
// lock on returns ErrFileBusy.
func TestFileBusyCopyOfLockedFile(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-busy-copy")
	lockWholeFile(t, path)

	if _, err := ins.CopyDocumentFile("tab-busy-copy", io.Discard); !errors.Is(err, ErrFileBusy) {
		t.Fatalf("CopyDocumentFile of a locked file: err = %v, want ErrFileBusy", err)
	}
}

var procLockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// lockWholeFile takes an exclusive byte-range lock on all of path through a
// second handle, held until the test ends. Reads through any other handle then
// fail with a lock violation. A share-mode lock is not used: os.Open sets
// FILE_FLAG_BACKUP_SEMANTICS, which gets past it on an elevated CI runner.
func lockWholeFile(t *testing.T, path string) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("open file to lock: %v", err)
	}
	t.Cleanup(func() { _ = syscall.CloseHandle(h) })
	const lockfileFailImmediately, lockfileExclusiveLock = 0x1, 0x2
	var ol syscall.Overlapped
	r, _, e := procLockFileEx.Call(uintptr(h), lockfileExclusiveLock|lockfileFailImmediately, 0, 0xFFFFFFFF, 0xFFFFFFFF, uintptr(unsafe.Pointer(&ol)))
	if r == 0 {
		t.Fatalf("lock file: %v", e)
	}
}
