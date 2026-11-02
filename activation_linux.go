//go:build linux && !gtk3

package main

/*
#cgo pkg-config: gtk4
#include <stdlib.h>
#include <gtk/gtk.h>
#ifdef GDK_WINDOWING_WAYLAND
#include <gdk/wayland/gdkwayland.h>
#endif

// On Wayland, setting the startup id of a mapped window activates it with
// the token. Presenting it afterwards would make GTK request a new token
// from the window's last input, which GNOME rejects with an "is ready"
// notification. Returns 1 when the token raised the window.
static int activate_with_token(void *window, const char *token) {
	GtkWindow *w = GTK_WINDOW(window);
	gtk_window_set_startup_id(w, token);
#ifdef GDK_WINDOWING_WAYLAND
	if (GDK_IS_WAYLAND_DISPLAY(gtk_widget_get_display(GTK_WIDGET(w))) && gtk_widget_get_mapped(GTK_WIDGET(w))) {
		return 1;
	}
#endif
	return 0;
}
*/
import "C"

import (
	"bytes"
	"os"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// launchActivationToken returns the token the desktop launched this process
// with: XDG_ACTIVATION_TOKEN on Wayland, DESKTOP_STARTUP_ID on X11. GTK unsets
// both while the library loads, before main runs, so they are read from the
// process's initial environment in /proc/self/environ.
func launchActivationToken() string {
	environ, err := os.ReadFile("/proc/self/environ")
	if err != nil {
		return ""
	}
	return activationTokenFromEnviron(environ)
}

// activationTokenFromEnviron picks the launch token out of a NUL-separated
// environment block, preferring XDG_ACTIVATION_TOKEN.
func activationTokenFromEnviron(environ []byte) string {
	var startupID string
	for _, entry := range bytes.Split(environ, []byte{0}) {
		if value, ok := bytes.CutPrefix(entry, []byte("XDG_ACTIVATION_TOKEN=")); ok && len(value) > 0 {
			return string(value)
		}
		if value, ok := bytes.CutPrefix(entry, []byte("DESKTOP_STARTUP_ID=")); ok {
			startupID = string(value)
		}
	}
	return startupID
}

// raiseWindow brings window to the front. GNOME only lets an already running
// window take focus when it presents a launch token, so the token is handed
// to GTK; without one GNOME shows an "is ready" notification instead. On X11,
// or when the window is not mapped, the token only sets the user time and the
// window is presented as usual.
func raiseWindow(window *application.WebviewWindow, token string) {
	activated := false
	if token != "" {
		application.InvokeSync(func() {
			native := window.NativeWindow()
			if native == nil {
				return
			}
			cToken := C.CString(token)
			defer C.free(unsafe.Pointer(cToken))
			activated = C.activate_with_token(native, cToken) == 1
		})
	}
	if !activated {
		window.Focus()
	}
}
