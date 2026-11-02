//go:build linux && !gtk3

package main

/*
#cgo pkg-config: gtk4
#include <stdlib.h>
#include <gtk/gtk.h>

static void set_startup_id(void *window, const char *token) {
	gtk_window_set_startup_id(GTK_WINDOW(window), token);
}
*/
import "C"

import (
	"os"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// launchActivationToken returns the token the desktop launched this process
// with: XDG_ACTIVATION_TOKEN on Wayland, DESKTOP_STARTUP_ID on X11.
func launchActivationToken() string {
	if token := os.Getenv("XDG_ACTIVATION_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("DESKTOP_STARTUP_ID")
}

// raiseWindow brings window to the front. GNOME only lets an already running
// window take focus when it presents a launch token, so the token is handed
// to GTK first; without one GNOME shows an "is ready" notification instead.
func raiseWindow(window *application.WebviewWindow, token string) {
	if token != "" {
		application.InvokeSync(func() {
			native := window.NativeWindow()
			if native == nil {
				return
			}
			cToken := C.CString(token)
			defer C.free(unsafe.Pointer(cToken))
			C.set_startup_id(native, cToken)
		})
	}
	window.Focus()
}
