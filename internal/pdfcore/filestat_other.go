//go:build !windows

package pdfcore

import "os"

// statWithIdentity is os.Stat: off Windows the stat already carries the
// device and inode that os.SameFile compares.
func statWithIdentity(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
