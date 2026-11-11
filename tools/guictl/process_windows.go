//go:build windows

package main

import (
	"errors"
	"os/exec"
)

// psTimeLayout is unused on Windows; guictl does not run there.
const psTimeLayout = "Mon Jan _2 15:04:05 2006"

// platformSupported fails every command: build:mcp and the process-group
// handling guictl relies on exist on macOS and Linux only.
func platformSupported() error { return errors.New("not supported on Windows") }

func probeProcess(int) (string, bool) { return "", false }

func terminateGroup(int) error { return errors.New("not supported on Windows") }

func killGroup(int) error { return errors.New("not supported on Windows") }

func setProcessGroup(*exec.Cmd) {}
