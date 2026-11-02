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
	lines := strings.Split(string(content), "\n")
	for _, want := range []string{"StartupWMClass=" + appID, "StartupNotify=true"} {
		found := false
		for _, line := range lines {
			if strings.TrimSpace(line) == want {
				found = true
			}
		}
		if !found {
			t.Errorf(".desktop file is missing %q", want)
		}
	}
}
