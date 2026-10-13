//go:build darwin || linux

package main

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// openPTY returns the terminal side of a new pseudo-terminal pair, skipping
// the test when the system has none to give.
func openPTY(t *testing.T) *os.File {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	name, err := unlockPTY(master)
	if err != nil {
		t.Skipf("cannot unlock the pseudo-terminal: %v", err)
	}
	tty, err := os.OpenFile(name, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("cannot open %s: %v", name, err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	return tty
}

func TestPseudoTerminalReportsItsColumnWidth(t *testing.T) {
	tty := openPTY(t)
	if err := unix.IoctlSetWinsize(int(tty.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 31, Col: 97}); err != nil {
		t.Fatalf("size the pseudo-terminal: %v", err)
	}
	if ok, width := terminalInfo(tty); !ok || width != 97 {
		t.Errorf("terminalInfo = %v, %d; want a terminal 97 columns wide", ok, width)
	}
}
