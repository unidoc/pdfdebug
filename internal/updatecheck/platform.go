package updatecheck

import (
	"errors"
	"runtime"
	"syscall"
)

// goos and goarch read the running platform. They are the single call site for
// runtime.GOOS/GOARCH so assetFor stays a pure, injectable function.
func goos() string   { return runtime.GOOS }
func goarch() string { return runtime.GOARCH }

// isCrossDevice reports whether err is the cross-filesystem rename failure
// (EXDEV) that triggers the copy+remove fallback in moveFile.
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
