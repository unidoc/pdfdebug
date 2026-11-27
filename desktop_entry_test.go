package main

import (
	"os"
	"strings"
	"testing"
)

func TestDesktopEntryMatchesAppID(t *testing.T) {
	content, err := os.ReadFile("build/linux/unidoc-pdf-debugger.desktop")
	if err != nil {
		t.Fatal(err)
	}
	want := "StartupWMClass=" + appID
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == want {
			return
		}
	}
	t.Errorf(".desktop file is missing %q", want)
}
