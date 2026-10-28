package wails_v3_stable_upgrade_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var workflowFiles = []string{
	".github/workflows/ci.yml",
	".github/workflows/release.yml",
}

func TestWorkflowsInstallTheGTK4DevPackages(t *testing.T) {
	for _, wf := range workflowFiles {
		src := readRepoFile(t, wf)
		for _, pkg := range []string{"libgtk-4-dev", "libwebkitgtk-6.0-dev"} {
			if !strings.Contains(src, pkg) {
				t.Errorf("%s does not install %s", wf, pkg)
			}
		}
		for _, pkg := range []string{"libgtk-3-dev", "libwebkit2gtk-4.1-dev"} {
			if strings.Contains(src, pkg) {
				t.Errorf("%s still installs the GTK3 package %s", wf, pkg)
			}
		}
	}
}

// gtk3Token matches the gtk3 build tag in any spelling it was passed in
// (`-tags gtk3`, `-tags=gtk3`, `EXTRA_TAGS=gtk3`, `production,gtk3`) and the
// comments that explained it.
var gtk3Token = regexp.MustCompile(`(?i)\bgtk3\b`)

func TestNoGtk3BuildTagRemains(t *testing.T) {
	files := append([]string{
		"build/Taskfile.yml",
		"build/linux/Taskfile.yml",
		"tests/boot-smoke/boot_smoke_test.go",
		"tests/scaffold/scaffold_test.go",
	}, workflowFiles...)
	for _, f := range files {
		src := readRepoFile(t, f)
		for i, line := range strings.Split(src, "\n") {
			if gtk3Token.MatchString(line) {
				t.Errorf("%s:%d still mentions gtk3: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// build/linux/nfpm/nfpm.yaml is local Wails scaffolding that .gitignore keeps
// out of the repo, so a fresh checkout skips this check.
func TestNfpmPackageDependsOnGTK4(t *testing.T) {
	nfpm := filepath.Join(projectRoot(t), "build", "linux", "nfpm", "nfpm.yaml")
	if _, err := os.Stat(nfpm); errors.Is(err, fs.ErrNotExist) {
		t.Skip("build/linux/nfpm/nfpm.yaml is not present in this checkout")
	}
	src := readRepoFile(t, "build/linux/nfpm/nfpm.yaml")
	var active []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		active = append(active, line)
	}
	body := strings.Join(active, "\n")
	for _, old := range []string{"libgtk-3-0", "libwebkit2gtk-4.1-0", "webkit2gtk4.1", "webkit2gtk-4.1"} {
		if strings.Contains(body, old) {
			t.Errorf("nfpm.yaml still depends on %s", old)
		}
	}
	if gtk3Token.MatchString(body) {
		t.Error("nfpm.yaml still depends on gtk3")
	}
	for _, want := range []string{"libgtk-4-1", "libwebkitgtk-6.0-4"} {
		if !strings.Contains(body, want) {
			t.Errorf("nfpm.yaml default depends lacks %s", want)
		}
	}
}

func TestReadmeLinuxSectionNamesTheGTK4Stack(t *testing.T) {
	src := readRepoFile(t, "README.md")
	for _, want := range []string{
		"libgtk-4-1",
		"libwebkitgtk-6.0-4",
		"libgtk-4-dev",
		"libwebkitgtk-6.0-dev",
		"xdg-desktop-portal",
		"22.04",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("README.md does not mention %s", want)
		}
	}
	for _, old := range []string{"libwebkit2gtk-4.1", "libgtk-3-dev"} {
		if strings.Contains(src, old) {
			t.Errorf("README.md still names the GTK3 package %s", old)
		}
	}
}

// ciDoctorStep matches a Linux-only CI step that runs `wails3 doctor`, whose
// report lists the GTK4 and WebKitGTK 6.0 packages found on the runner.
var ciDoctorStep = regexp.MustCompile(`(?m)^\s+if: runner\.os == 'Linux'\n(?:\s+shell: bash\n)?\s+run: wails3 doctor\s*$`)

func TestCIReportsTheLinuxStackWithWailsDoctor(t *testing.T) {
	src := readRepoFile(t, ".github/workflows/ci.yml")
	if !ciDoctorStep.MatchString(src) {
		t.Error("ci.yml has no Linux-only step that runs `wails3 doctor`")
	}
}
