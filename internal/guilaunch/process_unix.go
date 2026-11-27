//go:build !windows

package guilaunch

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// ProcessGroup signals real process groups with kill(2) on the negated PID.
var ProcessGroup = Group{
	Terminate: func(pid int) error { return signalGroup(pid, syscall.SIGTERM) },
	Kill:      func(pid int) error { return signalGroup(pid, syscall.SIGKILL) },
	Alive:     groupAlive,
}

// signalGroup sends sig to the group led by pid. A group that has already
// exited is not an error.
func signalGroup(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pid, sig); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

// groupAlive reports whether any process is left in the group led by pid. A
// group that exists but cannot be signalled (EPERM) counts as alive.
func groupAlive(pid int) bool {
	err := syscall.Kill(-pid, 0)
	return err == nil || err == syscall.EPERM
}

// SetProcessGroup starts cmd in its own process group, so ProcessGroup reaches
// every process it spawns and a terminal interrupt does not.
func SetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// LaunchSecond runs bin again with paths, so they reach the running instance
// through the single-instance handoff, and returns the handoff's output. The
// handoff exits 0, so any other exit is an error; one still running when ctx
// ends is ErrLaunchTimeout, with its process group killed. Paths must be
// absolute: the running instance ignores the launch's working directory.
func LaunchSecond(ctx context.Context, bin, dir string, env, paths []string) ([]byte, error) {
	cmd := exec.Command(bin, paths...)
	cmd.Dir = dir
	cmd.Env = env
	SetProcessGroup(cmd)
	var out lockedOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	// Bounds the wait for output pipes that a surviving grandchild holds open
	// after the handoff process exits.
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var waitErr error
	done := make(chan struct{})
	go func() { waitErr = cmd.Wait(); close(done) }()
	select {
	case <-done:
		if err := waitErr; err != nil {
			return out.bytes(), fmt.Errorf("second launch: %w", err)
		}
		return out.bytes(), nil
	case <-ctx.Done():
	}
	// A handoff that hangs has become a second primary instance; its WebKit
	// helpers share its group.
	_, _ = ProcessGroup.Stop(cmd.Process.Pid, 0, 5*time.Second)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	return out.bytes(), ErrLaunchTimeout
}

// lockedOutput collects a child's combined output; the copy goroutines and the
// caller may touch it at once.
type lockedOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *lockedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *lockedOutput) bytes() []byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Clone(o.buf.Bytes())
}
