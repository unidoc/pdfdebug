//go:build linux && !gtk3

package main

import "testing"

func TestActivationTokenFromEnviron(t *testing.T) {
	cases := []struct {
		name    string
		environ string
		want    string
	}{
		{"wayland token wins", "HOME=/home/a\x00DESKTOP_STARTUP_ID=x11\x00XDG_ACTIVATION_TOKEN=wl\x00", "wl"},
		{"x11 startup id", "DESKTOP_STARTUP_ID=x11_TIME5\x00PATH=/bin\x00", "x11_TIME5"},
		{"empty wayland token falls back", "XDG_ACTIVATION_TOKEN=\x00DESKTOP_STARTUP_ID=x11\x00", "x11"},
		{"neither set", "HOME=/home/a\x00", ""},
		{"prefix only matches whole name", "MY_XDG_ACTIVATION_TOKEN=no\x00", ""},
	}
	for _, c := range cases {
		if got := activationTokenFromEnviron([]byte(c.environ)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
