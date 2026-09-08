// One current-state version contract for the Wails toolchain, replacing the
// per-release floor suites that each froze a different lower bound.
//
// The tree pins the Wails Go module and wails3 CLI in three places (go.mod and
// the two CI workflow install lines) and the JS runtime in two (package.json
// and its lockfile resolution). This suite asserts those five touch-points agree
// with the single declared target below. It compares literal strings: no
// ordinal, no version-shape regex, no floor. A bump is editing the two target
// constants and the pins they describe; a bump that updates two of the three Go
// touch-points, or leaves a range specifier on the runtime, fails here.
package wails_version_contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goWailsTarget is the exact Wails Go module / wails3 CLI version the tree pins.
// go.mod and both workflow install lines must equal it verbatim.
const goWailsTarget = "v3.0.0-beta.18"

// runtimeTarget is the exact @wailsio/runtime version the frontend pins. The
// package.json spec (with no range specifier) and the lockfile resolution must
// both equal it verbatim.
const runtimeTarget = "3.0.0-beta.18"

// projectRoot walks up from the working directory to the project go.mod (module
// unidoc-pdf-debugger) and returns its absolute path.
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

// readSource reads a file relative to the project root.
func readSource(t *testing.T, relPath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectRoot(t), relPath))
	if err != nil {
		t.Fatalf("cannot read %s: %v", relPath, err)
	}
	return string(content)
}

// TestGoModPinEqualsTarget asserts go.mod's wails/v3 require line pins exactly
// the declared target.
func TestGoModPinEqualsTarget(t *testing.T) {
	var pin string
	for _, line := range strings.Split(readSource(t, "go.mod"), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "github.com/wailsapp/wails/v3" {
			pin = fields[1]
			break
		}
	}
	if pin == "" {
		t.Fatalf("go.mod must declare a `github.com/wailsapp/wails/v3` require")
	}
	if pin != goWailsTarget {
		t.Errorf("go.mod wails/v3 pin = %q, want %q (edit go.mod and this suite's goWailsTarget together)", pin, goWailsTarget)
	}
}

// TestGoSumCarriesTarget asserts go.sum carries an entry for the pinned wails/v3
// version, which proves `go mod tidy` ran after the go.mod edit. A bump that
// edits go.mod but skips tidy leaves go.sum stale; this fails loud here rather
// than letting it surface later as a build error.
func TestGoSumCarriesTarget(t *testing.T) {
	needle := "github.com/wailsapp/wails/v3 " + goWailsTarget
	if !strings.Contains(readSource(t, "go.sum"), needle) {
		t.Errorf("go.sum must carry an entry for %q -- run `go mod tidy` after editing go.mod", needle)
	}
}

// wails3InstallPins returns the version pinned on EVERY `wails3@<version>`
// install line in the given workflow. A marker with no version after it yields
// an empty-string pin rather than being skipped, so a malformed line surfaces as
// a mismatch instead of passing silently. Every occurrence is returned so a
// stale duplicate install line cannot hide behind a corrected first one.
func wails3InstallPins(src string) []string {
	const marker = "wails3@"
	var pins []string
	for _, line := range strings.Split(src, "\n") {
		i := strings.Index(line, marker)
		if i < 0 {
			continue
		}
		fields := strings.FieldsFunc(line[i+len(marker):], func(r rune) bool {
			return r == ' ' || r == '\t' || r == '"' || r == '\''
		})
		if len(fields) == 0 {
			pins = append(pins, "")
			continue
		}
		pins = append(pins, fields[0])
	}
	return pins
}

// TestWorkflowInstallPinsEqualTarget asserts both CI workflows install the wails3
// CLI at exactly the declared target on every install line, so the CLI and the
// library can never skew and a bump that touches only one workflow file, or
// leaves a stale duplicate install line, fails.
func TestWorkflowInstallPinsEqualTarget(t *testing.T) {
	for _, wf := range []string{".github/workflows/ci.yml", ".github/workflows/release.yml"} {
		pins := wails3InstallPins(readSource(t, wf))
		if len(pins) == 0 {
			t.Errorf("%s must install wails3 with a `wails3@<version>` pin", wf)
			continue
		}
		for _, pin := range pins {
			if pin != goWailsTarget {
				t.Errorf("%s installs wails3@%s, want wails3@%s (every wails3 install pin must equal the go.mod library pin)", wf, pin, goWailsTarget)
			}
		}
	}
}

// runtimeSpec returns the @wailsio/runtime dependency spec declared in
// package.json.
func runtimeSpec(t *testing.T) string {
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(readSource(t, "frontend/package.json")), &pkg); err != nil {
		t.Fatalf("package.json is not valid JSON: %v", err)
	}
	spec, ok := pkg.Dependencies["@wailsio/runtime"]
	if !ok {
		t.Fatalf("package.json dependencies must declare @wailsio/runtime")
	}
	return spec
}

// TestRuntimePinExactNoRange asserts @wailsio/runtime is pinned to exactly the
// declared runtime target with no range specifier, so the runtime cannot float
// underneath the app between installs.
func TestRuntimePinExactNoRange(t *testing.T) {
	spec := runtimeSpec(t)
	if len(spec) > 0 && (spec[0] == '^' || spec[0] == '~' || spec[0] == '>' || spec[0] == '<' || spec[0] == '=' || spec[0] == '*') {
		t.Errorf("@wailsio/runtime spec %q carries a range specifier -- pin it exactly (a range lets a prerelease runtime float between installs)", spec)
	}
	if spec != runtimeTarget {
		t.Errorf("@wailsio/runtime spec = %q, want exactly %q", spec, runtimeTarget)
	}
}

// TestLockfileRuntimeResolvesTarget asserts the lockfile resolves
// @wailsio/runtime to exactly what package.json declares, so declared and
// resolved cannot drift.
func TestLockfileRuntimeResolvesTarget(t *testing.T) {
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(readSource(t, "frontend/package-lock.json")), &lock); err != nil {
		t.Fatalf("package-lock.json is not valid JSON: %v", err)
	}
	node, ok := lock.Packages["node_modules/@wailsio/runtime"]
	if !ok {
		t.Fatalf("package-lock.json must resolve node_modules/@wailsio/runtime")
	}
	if node.Version != runtimeTarget {
		t.Errorf("package-lock.json resolves @wailsio/runtime to %q, want %q -- run `npm install` after editing package.json", node.Version, runtimeTarget)
	}
}
