//go:build !linux || gtk3

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// launchActivationToken returns "": only the Linux GTK4 build uses launch tokens.
func launchActivationToken() string { return "" }

// raiseWindow brings window to the front.
func raiseWindow(window *application.WebviewWindow, _ string) {
	window.Focus()
}
