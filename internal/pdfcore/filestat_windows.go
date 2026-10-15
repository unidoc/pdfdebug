//go:build windows

package pdfcore

import (
	"io/fs"
	"os"
	"syscall"
)

// fileReadAttributes is FILE_READ_ATTRIBUTES: enough to query the handle, and
// never refused by another process's share mode.
const fileReadAttributes = 0x80

// statWithIdentity stats path through one attributes-only handle, so the
// FileInfo already carries the file ID and os.SameFile opens nothing more.
// os.Stat would load the ID lazily, with a second open per comparison.
func statWithIdentity(path string) (os.FileInfo, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: err}
	}
	h, err := syscall.CreateFile(p, fileReadAttributes,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)
	defer func() { _ = f.Close() }()
	return f.Stat()
}
