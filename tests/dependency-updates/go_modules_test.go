package dependency_updates_test

import (
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The go directive carries the net/http fix for GO-2026-6859. No toolchain
// line: the directive alone makes an older local toolchain fetch 1.27.4.
func TestGoDirectiveCarriesTheNetHTTPFix(t *testing.T) {
	gomod := readRepoFile(t, "go.mod")
	m := regexp.MustCompile(`(?m)^go\s+(\S+)\s*$`).FindStringSubmatch(gomod)
	if m == nil {
		t.Fatal("go.mod has no go directive")
	}
	if m[1] != "1.27.4" {
		t.Errorf("go directive = %s, want 1.27.4", m[1])
	}
	if regexp.MustCompile(`(?m)^toolchain\s`).MatchString(gomod) {
		t.Errorf("go.mod carries a toolchain line; the go directive is enough")
	}
}

func TestModulesAtTargetVersions(t *testing.T) {
	reqs := goModRequires(t)
	for _, tc := range []struct {
		module, version string
		indirect        bool
	}{
		{"github.com/pdfcpu/pdfcpu", "v0.16.4", false},
		{"golang.org/x/image", "v0.48.0", false},
		{"golang.org/x/sys", "v0.50.0", false},
		{"golang.org/x/mod", "v0.43.0", false},
		{"golang.org/x/crypto", "v0.59.0", true},
		{"golang.org/x/text", "v0.44.0", true},
		{"github.com/hhrutter/tiff", "v1.0.8", true},
		{"github.com/coder/websocket", "v1.8.15", true},
		{"github.com/godbus/dbus/v5", "v5.2.4", true},
		{"github.com/mattn/go-colorable", "v0.1.16", true},
		{"github.com/mattn/go-isatty", "v0.0.24", true},
		// pdfcpu's own requires, at pdfcpu's minimums.
		{"github.com/mattn/go-runewidth", "v0.0.31", true},
		{"github.com/xdg-go/stringprep", "v1.0.4", true},
		{"go.yaml.in/yaml/v3", "v3.0.5", true},
	} {
		got, ok := reqs[tc.module]
		if !ok {
			t.Errorf("go.mod does not require %s", tc.module)
			continue
		}
		if got.version != tc.version {
			t.Errorf("%s = %s, want %s", tc.module, got.version, tc.version)
		}
		if got.indirect != tc.indirect {
			t.Errorf("%s indirect = %v, want %v", tc.module, got.indirect, tc.indirect)
		}
	}
}

func TestDroppedModulesAreGone(t *testing.T) {
	reqs := goModRequires(t)
	for _, module := range []string{
		"github.com/hhrutter/" + "lzw",
		"github.com/hhrutter/pkcs7",
		"github.com/kr/pretty",
		"github.com/pkg/errors",
		"github.com/rogpeppe/go-internal",
		"gopkg.in/check.v1",
		"gopkg.in/yaml.v2",
	} {
		if r, ok := reqs[module]; ok {
			t.Errorf("go.mod still requires %s %s", module, r.version)
		}
	}
}

func TestDirectRequiresAreExactlyTheApprovedSet(t *testing.T) {
	want := []string{
		"github.com/adrg/xdg",
		"github.com/google/uuid",
		"github.com/pdfcpu/pdfcpu",
		"github.com/wailsapp/wails/v3",
		"golang.org/x/image",
		"golang.org/x/mod",
		"golang.org/x/sys",
	}
	var got []string
	for module, r := range goModRequires(t) {
		if !r.indirect {
			got = append(got, module)
		}
	}
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("direct requires:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestGoModIsTidy(t *testing.T) {
	cmd := exec.Command("go", "mod", "tidy", "-diff")
	cmd.Dir = projectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("go mod tidy would change go.mod or go.sum:\n%s", out)
	}
}

// govulncheck needs its vulnerability database, so it is skipped when the
// tool is not installed.
func TestGovulncheckReportsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("govulncheck skipped in -short mode")
	}
	path, err := exec.LookPath("govulncheck")
	if err != nil {
		t.Skip("govulncheck not on PATH")
	}
	cmd := exec.Command(path, "./...")
	cmd.Dir = projectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("govulncheck ./... reports findings:\n%s", out)
	}
}
