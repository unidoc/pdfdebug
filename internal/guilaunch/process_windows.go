//go:build windows

package guilaunch

import (
	"context"
	"errors"
	"os/exec"
)

var errUnsupported = errors.New("not supported on Windows")

// ProcessGroup fails on Windows; guictl and the GUI suite run on macOS and
// Linux only.
var ProcessGroup = Group{
	Terminate: func(int) error { return errUnsupported },
	Kill:      func(int) error { return errUnsupported },
	Alive:     func(int) bool { return false },
}

// SetProcessGroup does nothing on Windows.
func SetProcessGroup(*exec.Cmd) {}

// LaunchSecond fails on Windows.
func LaunchSecond(context.Context, string, string, []string, []string) ([]byte, error) {
	return nil, errUnsupported
}
