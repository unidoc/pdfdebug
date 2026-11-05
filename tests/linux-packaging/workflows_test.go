package linux_packaging_test

import (
	"strings"
	"testing"
)

func TestCIInstallsTheDebAndBootsThePackagedBinary(t *testing.T) {
	src := readRepoFile(t, ".github/workflows/ci.yml")
	for _, want := range []string{
		"wails3 task linux:generate:deb",
		"wails3 task linux:generate:rpm",
		"sudo apt-get install -y --no-install-recommends ./bin/unidoc-pdf-debugger.deb",
		"sudo aa-status",
		"sysctl -n kernel.apparmor_restrict_unprivileged_userns)\" = 1",
		"BOOT_SMOKE_BINARY=" + installedGUI,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("ci.yml lacks %q", want)
		}
	}
	if strings.Contains(src, "apparmor_restrict_unprivileged_userns=0") {
		t.Error("ci.yml lifts the user namespace restriction, so the boot smoke no longer shows the packaged profile works")
	}
}

func TestReleaseShipsTheDebAndRpm(t *testing.T) {
	src := readRepoFile(t, ".github/workflows/release.yml")
	for _, want := range []string{
		"wails3 task linux:generate:deb",
		"wails3 task linux:generate:rpm",
		`"dist/unidoc-pdf-debugger-${VERSION}-${PLATFORM}.deb"`,
		`"dist/unidoc-pdf-debugger-${VERSION}-${PLATFORM}.rpm"`,
		"EXPECTED_FILES=8",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("release.yml lacks %q", want)
		}
	}
}
