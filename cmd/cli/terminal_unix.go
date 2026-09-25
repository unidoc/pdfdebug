//go:build unix

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// terminalInfo reports whether f is a terminal and, if so, its width in
// columns (0 when unknown). The window-size ioctl fails on pipes, files and
// /dev/null, so none of them count as a terminal.
func terminalInfo(f *os.File) (bool, int) {
	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return false, 0
	}
	return true, int(ws.Col)
}
