package dependency_updates_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// lockPackages returns every package in frontend/package-lock.json keyed by
// its lockfile path, with the package name and resolved version.
func lockPackages(t *testing.T) map[string][2]string {
	t.Helper()
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "frontend/package-lock.json")), &lock); err != nil {
		t.Fatalf("parse frontend/package-lock.json: %v", err)
	}
	out := map[string][2]string{}
	for path, p := range lock.Packages {
		i := strings.LastIndex(path, "node_modules/")
		if i < 0 || p.Version == "" {
			continue
		}
		out[path] = [2]string{path[i+len("node_modules/"):], p.Version}
	}
	return out
}

// Each resolved copy of these packages is outside the vulnerable ranges npm
// audit reports, or below the first patched release where npm gives none. A
// range is [from, to): a version v is flagged when from <= v < to.
func TestLockfileHasNoAdvisoryVersions(t *testing.T) {
	vulnerable := map[string][][2]string{
		"undici":          {{"7.0.0", "7.30.1"}},
		"js-yaml":         {{"4.0.0", "4.3.2"}},
		"brace-expansion": {{"0.0.0", "1.1.21"}, {"3.0.0", "5.0.12"}},
		"minimatch":       {{"7.0.0", "11.0.1"}},
		"vitest":          {{"4.0.0", "4.1.12"}},
		"@vitest/mocker":  {{"4.0.0", "4.1.12"}},
	}
	seen := map[string]bool{}
	for path, nv := range lockPackages(t) {
		ranges, ok := vulnerable[nv[0]]
		if !ok {
			continue
		}
		seen[nv[0]] = true
		for _, r := range ranges {
			if compareVersions(t, nv[1], r[0]) >= 0 && compareVersions(t, nv[1], r[1]) < 0 {
				t.Errorf("%s resolves %s %s, inside the advised range [%s, %s)", path, nv[0], nv[1], r[0], r[1])
			}
		}
	}
	for name := range vulnerable {
		if !seen[name] {
			t.Errorf("frontend/package-lock.json has no %s; the check would pass vacuously", name)
		}
	}
}

// typescript-eslint 8.73.0 is the first release whose typescript-estree accepts
// minimatch 11. An older one is allowed only with the scoped minimatch override.
func TestTypescriptESLintAcceptsPatchedMinimatch(t *testing.T) {
	var version string
	for path, nv := range lockPackages(t) {
		if path == "node_modules/typescript-eslint" {
			version = nv[1]
		}
	}
	if version == "" {
		t.Fatal("frontend/package-lock.json has no node_modules/typescript-eslint")
	}
	if compareVersions(t, version, "8.73.0") >= 0 {
		return
	}
	var pkg struct {
		Overrides map[string]any `json:"overrides"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "frontend/package.json")), &pkg); err != nil {
		t.Fatalf("parse frontend/package.json: %v", err)
	}
	estree, _ := pkg.Overrides["@typescript-eslint/typescript-estree"].(map[string]any)
	if estree["minimatch"] == nil {
		t.Errorf("typescript-eslint %s predates 8.73.0 and package.json has no @typescript-eslint/typescript-estree minimatch override", version)
	}
}
