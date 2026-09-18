//go:build !windows

package updateservice

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// revealInFileManager selects the file in the platform file manager (macOS) or
// opens its containing folder (Linux). Best effort; failures are ignored.
func revealInFileManager(path string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", "-R", path)
	} else {
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	revealAndWait(cmd)
}
