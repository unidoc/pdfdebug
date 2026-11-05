// Package linux_packaging_test pins the Linux .deb and .rpm packaging: the
// nfpm config, the AppArmor profile the .deb installs for WebKit's sandbox,
// the install scripts, and the CI and release steps that build them.
//
// Run: cd tests/linux-packaging && go test -count=1 ./...
package linux_packaging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	nfpmConfig     = "build/linux/nfpm/nfpm.yaml"
	appArmorFile   = "build/linux/apparmor/unidoc-pdf-debugger"
	postinstall    = "build/linux/nfpm/scripts/postinstall.sh"
	postremove     = "build/linux/nfpm/scripts/postremove.sh"
	desktopEntry   = "build/linux/unidoc-pdf-debugger.desktop"
	installedGUI   = "/usr/bin/unidoc-pdf-debugger"
	installedCLI   = "/usr/bin/pdfdebug"
	installedRules = "/etc/apparmor.d/unidoc-pdf-debugger"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(data), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// nfpmEntry returns the contents entry whose dst is dst: the lines from its
// "- src:" line up to the next entry or the end of the list.
func nfpmEntry(t *testing.T, dst string) string {
	t.Helper()
	src := readRepoFile(t, nfpmConfig)
	entries := strings.Split(src, "\n  - src:")
	for _, e := range entries[1:] {
		if end := strings.Index(e, "\n\n"); end >= 0 {
			e = e[:end]
		}
		if strings.Contains(e, `dst: "`+dst+`"`) {
			return e
		}
	}
	t.Fatalf("%s has no contents entry with dst %s", nfpmConfig, dst)
	return ""
}
