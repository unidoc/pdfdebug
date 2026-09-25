//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// terminalInfo reports whether f is a console and, if so, its window width in
// columns (0 when unknown). Pipes, files and NUL are not consoles; neither are
// the pipes Git Bash and mintty hand a process.
func terminalInfo(f *os.File) (bool, int) {
	h := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false, 0
	}
	var info windows.ConsoleScreenBufferInfo
	if windows.GetConsoleScreenBufferInfo(h, &info) != nil {
		return true, 0
	}
	return true, int(info.Window.Right-info.Window.Left) + 1
}
