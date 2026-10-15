//go:build !windows

// Busy detection is Windows-only; elsewhere every error is not busy.
//
// Run: cd code && go test -run TestFileBusy ./internal/pdfcore/

package pdfcore

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"
)

func TestFileBusyIsFalseOffWindows(t *testing.T) {
	for _, err := range []error{
		syscall.Errno(32),
		syscall.Errno(33),
		&fs.PathError{Op: "open", Path: "x.pdf", Err: syscall.EACCES},
		errors.New("boom"),
		nil,
	} {
		if isFileBusy(err) {
			t.Errorf("isFileBusy(%v) = true, want false off Windows", err)
		}
	}
}
