//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// psTimeLayout is the format of ps -o lstart= under LC_ALL=C.
const psTimeLayout = "Mon Jan _2 15:04:05 2006"

func platformSupported() error { return nil }

// probeProcess reports whether pid is alive and its start time from ps.
func probeProcess(pid int) (string, bool) {
	if pid <= 0 || syscall.Kill(pid, 0) == syscall.ESRCH {
		return "", false
	}
	cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	start := strings.Join(strings.Fields(string(out)), " ")
	return start, start != ""
}

// terminateGroup sends SIGTERM to the process group led by pid. A group that
// has already exited is not an error.
func terminateGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

// setProcessGroup starts cmd in its own process group, so stop reaches every
// process it spawns and a terminal interrupt does not.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
