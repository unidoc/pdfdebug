//go:build windows

package updateservice

import (
	"os/exec"
	"syscall"
)

// revealInFileManager selects the file in Explorer. It sets the raw command line
// (SysProcAttr.CmdLine) rather than the escaped arg vector: Go would quote the
// whole "/select,C:\path with space\x.zip" token and Explorer will not parse
// /select, out of a quoted argument, so it would silently open the default folder
// instead. Quoting only the path handles spaces. Best effort; failures ignored.
func revealInFileManager(path string) {
	cmd := exec.Command("explorer")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer /select,"` + path + `"`}
	revealAndWait(cmd)
}
