package wails_v3_stable_upgrade_test

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestWailsPinIsAStableV3Release(t *testing.T) {
	pin := goModWailsPin(t)
	nums, pre := parseVersion(t, pin)
	if nums[0] != 3 {
		t.Fatalf("go.mod Wails pin %s is not a v3 release", pin)
	}
	if pre != "" {
		t.Fatalf("go.mod Wails pin %s is a pre-release (%s); the target is a stable v3 release", pin, pre)
	}
}

func TestRuntimePackageMatchesTheGoModPin(t *testing.T) {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "frontend/package.json")), &pkg); err != nil {
		t.Fatalf("parse frontend/package.json: %v", err)
	}
	got, ok := pkg.Dependencies["@wailsio/runtime"]
	if !ok {
		got = pkg.DevDependencies["@wailsio/runtime"]
	}
	want := strings.TrimPrefix(goModWailsPin(t), "v")
	if got != want {
		t.Fatalf("@wailsio/runtime = %q, want exactly %q (the go.mod Wails pin)", got, want)
	}
}

func TestGoDirectiveRequiresTheX509Fix(t *testing.T) {
	m := regexp.MustCompile(`(?m)^go\s+(\S+)`).FindStringSubmatch(readRepoFile(t, "go.mod"))
	if m == nil {
		t.Fatal("go.mod has no go directive")
	}
	if !versionAtLeast(t, m[1], "1.27.3") {
		t.Fatalf("go.mod go directive is %s, want 1.27.3 or later (crypto/x509 fix GO-2026-6809)", m[1])
	}
}

func TestViteIsAtLeastThePatchedRelease(t *testing.T) {
	var pkg struct {
		DevDependencies map[string]string `json:"devDependencies"`
		Dependencies    map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "frontend/package.json")), &pkg); err != nil {
		t.Fatalf("parse frontend/package.json: %v", err)
	}
	spec, ok := pkg.DevDependencies["vite"]
	if !ok {
		spec = pkg.Dependencies["vite"]
	}
	floor := strings.TrimLeft(spec, "^~>= ")
	if !versionAtLeast(t, floor, "8.2.4") {
		t.Errorf("frontend/package.json vite range %q allows versions below 8.2.4", spec)
	}

	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "frontend/package-lock.json")), &lock); err != nil {
		t.Fatalf("parse frontend/package-lock.json: %v", err)
	}
	resolved := lock.Packages["node_modules/vite"].Version
	if resolved == "" {
		t.Fatal("package-lock.json does not resolve node_modules/vite")
	}
	if !versionAtLeast(t, resolved, "8.2.4") {
		t.Errorf("package-lock.json resolves vite %s, want 8.2.4 or later", resolved)
	}
}

func TestModuleGraphCarriesTheSecurityFixes(t *testing.T) {
	floors := map[string]string{
		"golang.org/x/image":            "v0.45.0",
		"github.com/klauspost/compress": "v1.18.7",
	}
	for mod, floor := range floors {
		cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", mod)
		cmd.Dir = projectRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(out), "not a known dependency") {
				continue
			}
			t.Fatalf("go list -m %s: %v\n%s", mod, err, out)
		}
		got := strings.TrimSpace(string(out))
		if !versionAtLeast(t, got, floor) {
			t.Errorf("%s resolves to %s, want %s or later", mod, got, floor)
		}
	}
}

func TestInstalledWailsCLIMatchesTheGoModPin(t *testing.T) {
	if _, err := exec.LookPath("wails3"); err != nil {
		t.Skip("wails3 not on PATH")
	}
	out, err := exec.Command("wails3", "version").CombinedOutput()
	if err != nil {
		t.Fatalf("wails3 version: %v\n%s", err, out)
	}
	pin := goModWailsPin(t)
	if strings.TrimSpace(string(out)) != pin {
		t.Fatalf("wails3 version prints %q, want %q (the go.mod pin)", strings.TrimSpace(string(out)), pin)
	}
}
