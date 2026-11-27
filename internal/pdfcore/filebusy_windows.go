//go:build windows

package pdfcore

import (
	"errors"
	"syscall"
)

// Windows errnos for a file another process holds without read sharing.
const (
	errnoSharingViolation = syscall.Errno(32)
	errnoLockViolation    = syscall.Errno(33)
)

// isFileBusy reports whether err is a Windows sharing or lock violation.
func isFileBusy(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == errnoSharingViolation || errno == errnoLockViolation
}
