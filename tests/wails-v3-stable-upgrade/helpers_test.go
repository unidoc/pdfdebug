// Package wails_v3_stable_upgrade_test checks the move to a stable Wails v3
// release: the version pins and toolchain floor, the GTK4 Linux stack, the
// typed-event literal gate on frontend/src, the backend-only embedded file
// save and Open Recent.
//
// The Go unit tests for the new internal code live next to that code
// (internal/recentfiles, internal/pdfservice and the main package) and run in
// the root `go test ./...` sweep. This suite runs them again, the list and
// service tests under -race, so their result also shows up here.
//
// Run: cd tests/wails-v3-stable-upgrade && go test -count=1 ./...
package wails_v3_stable_upgrade_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// projectRoot walks up from cwd to the directory holding the main go.mod.
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

// readRepoFile returns the content of a file given relative to the repo root.
func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// goModWailsPin returns the github.com/wailsapp/wails/v3 version required by go.mod.
func goModWailsPin(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^\s*github\.com/wailsapp/wails/v3\s+(v\S+)`).FindStringSubmatch(readRepoFile(t, "go.mod"))
	if m == nil {
		t.Fatal("go.mod has no github.com/wailsapp/wails/v3 require")
	}
	return m[1]
}

// parseVersion splits "v1.2.3", "1.2.3" or "1.27" into numeric parts and the
// pre-release suffix ("" for a release).
func parseVersion(t *testing.T, v string) ([]int, string) {
	t.Helper()
	v = strings.TrimPrefix(v, "v")
	pre := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v, pre = v[:i], v[i+1:]
	}
	var nums []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("version %q has a non-numeric part %q", v, p)
		}
		nums = append(nums, n)
	}
	for len(nums) < 3 {
		nums = append(nums, 0)
	}
	return nums, pre
}

// versionAtLeast reports whether got >= min, comparing numeric parts only.
func versionAtLeast(t *testing.T, got, min string) bool {
	t.Helper()
	g, _ := parseVersion(t, got)
	m, _ := parseVersion(t, min)
	for i := 0; i < 3; i++ {
		if g[i] != m[i] {
			return g[i] > m[i]
		}
	}
	return true
}

// runGoTest runs `go test` with the given args from the repo root and returns
// the combined output.
func runGoTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
