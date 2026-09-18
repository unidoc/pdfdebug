package updatecheck

import "runtime"

// goos and goarch read the running platform. They are the single call site for
// runtime.GOOS/GOARCH so assetFor stays a pure, injectable function.
func goos() string   { return runtime.GOOS }
func goarch() string { return runtime.GOARCH }
