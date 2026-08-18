// The CLI==library version-pin invariant plus the runtime-honesty guard.
//
// Scenario: the wails3 CLI pin in BOTH .github/workflows/ci.yml AND
// .github/workflows/release.yml must EQUAL the go.mod library pin exactly,
// and go.mod must reach the committed target v3.0.0-alpha2.117. This
// mirrors the version-pin cases in tests/wails-alpha2-103-upgrade -- it
// reads the files statically and does NOT shell out to `wails3 --version`
// (the live go.mod==wails3 equality is confirmed only on the legs where the
// CLI is installed: the 3-runner build and the manual smoke, out of this
// module's scope). A mismatch means everything downstream tests a phantom
// -- stop.
package wails_alpha2_117_upgrade_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// go.mod library pin reaches the committed target
// ---------------------------------------------------------------------------

// TestGoModPinAtTarget asserts go.mod's wails/v3 pin reaches the committed target
// alpha2.117. The assertion is a floor (>= target) rather than an exact ==, so a
// defensibly-newer most-baked pick still passes while the fallback rungs
// (alpha2.103, alpha.102) do not.
func TestGoModPinAtTarget(t *testing.T) {
	line := goWailsLine(t)
	if line == "" {
		t.Fatalf("go.mod must declare a `github.com/wailsapp/wails/v3` require")
	}
	got := alphaOrdinal(goWailsRe, line)
	if got < 0 {
		t.Fatalf("go.mod wails/v3 line %q must carry a v3.0.0-alpha.N or v3.0.0-alpha2.N tag", line)
	}
	if got < targetOrdinal {
		t.Errorf("go.mod wails/v3 pin %s has not reached the committed target %s. Current known-good fallback rung 1 = alpha2.103, deeper rung 2 = alpha.102 both sit below this floor.",
			fmtOrdinal(got), fmtOrdinal(targetOrdinal))
	}
}

// TestGoSumCarriesNewPin asserts go.sum carries an entry for the bumped wails/v3
// pin, which proves `go mod tidy` ran. It skips cleanly while go.mod is still
// below the target so the failure signal stays on TestGoModPinAtTarget.
func TestGoSumCarriesNewPin(t *testing.T) {
	got := alphaOrdinal(goWailsRe, goWailsLine(t))
	if got < targetOrdinal {
		t.Skipf("skipped -- go.mod not bumped to target yet (see TestGoModPinAtTarget)")
	}
	needle := "github.com/wailsapp/wails/v3 v3.0.0-" + tagFromOrdinal(got)
	gosum := readSource(t, "go.sum")
	if !strings.Contains(gosum, needle) {
		t.Errorf("go.sum must contain %q -- run `go mod tidy` after editing go.mod (Task 2.1)", needle)
	}
}

// ---------------------------------------------------------------------------
// CLI==library: wails3 install pin parity in BOTH workflows
// ---------------------------------------------------------------------------

// assertWorkflowPinMatchesGoMod is the shared body for the ci.yml / release.yml
// CLI-pin parity assertions (mirroring tests/wails-alpha2-103-upgrade): the
// workflow must
// install wails3 with a v3.0.0 alpha pin, every alpha pin in the file must move
// strictly past the pre-bump baseline (alpha2.103), and every one must EQUAL the
// go.mod pin exactly (CLI == library).
func assertWorkflowPinMatchesGoMod(t *testing.T, relPath string) {
	t.Helper()
	src := readSource(t, relPath)
	if !strings.Contains(src, "wails3@v3.0.0-alpha") {
		t.Fatalf("%s must install wails3 with a v3.0.0-alpha pin", relPath)
	}
	got := allAlphaOrdinals(goWailsRe, src)
	if len(got) == 0 {
		t.Fatalf("%s must reference a v3.0.0-alpha pin", relPath)
	}
	goOrd := alphaOrdinal(goWailsRe, goWailsLine(t))
	for _, n := range got {
		if n <= currentBaselineOrdinal {
			t.Errorf("%s carries %s -- a bump must move it strictly past the pre-bump baseline %s",
				relPath, fmtOrdinal(n), fmtOrdinal(currentBaselineOrdinal))
		}
		if goOrd > 0 && n != goOrd {
			t.Errorf("%s wails3 CLI pin %s must EQUAL the go.mod library pin %s exactly -- the CLI and the library must be the same version",
				relPath, fmtOrdinal(n), fmtOrdinal(goOrd))
		}
	}
}

// TestCiWorkflowPinParity asserts ci.yml's wails3 CLI install pin is bumped and
// equals the go.mod library pin.
func TestCiWorkflowPinParity(t *testing.T) {
	assertWorkflowPinMatchesGoMod(t, ".github/workflows/ci.yml")
}

// TestReleaseWorkflowPinParity asserts release.yml's wails3 CLI install pin is
// bumped and equals the go.mod library pin.
func TestReleaseWorkflowPinParity(t *testing.T) {
	assertWorkflowPinMatchesGoMod(t, ".github/workflows/release.yml")
}

// TestReleaseExpectedFilesInvariant asserts release.yml retains the
// EXPECTED_FILES=6 publish invariant, so a framework bump cannot disturb the
// artifact-count contract.
func TestReleaseExpectedFilesInvariant(t *testing.T) {
	src := readSource(t, ".github/workflows/release.yml")
	if !strings.Contains(src, "EXPECTED_FILES=6") {
		t.Errorf("release.yml must retain EXPECTED_FILES=6 invariant across the bump")
	}
}

// ---------------------------------------------------------------------------
// Runtime held to what upstream publishes (no phantom alpha2)
// ---------------------------------------------------------------------------

// TestRuntimePinNoPhantomAlpha2 asserts @wailsio/runtime is a well-formed
// 3.0.0-alpha.N tag with N at or above the current pin (alpha.79), and is NOT
// rewritten to a phantom alpha2.* tag: npm publishes no alpha2 runtime, and
// Go-side currency from the alpha2.117 library bump does not license runtime skew.
// If upstream ever publishes a matching alpha2.* runtime and it is adopted, this
// guard must be relaxed in lockstep or it goes red.
func TestRuntimePinNoPhantomAlpha2(t *testing.T) {
	src := readSource(t, "frontend/package.json")
	var pkg map[string]any
	if err := json.Unmarshal([]byte(src), &pkg); err != nil {
		t.Fatalf("package.json is not valid JSON: %v", err)
	}
	deps, ok := pkg["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("package.json must declare a dependencies object")
	}
	raw, ok := deps["@wailsio/runtime"].(string)
	if !ok {
		t.Fatalf("dependencies must declare @wailsio/runtime as a string pin")
	}
	m := jsRuntimeRe.FindStringSubmatch(raw)
	if len(m) < 3 {
		t.Fatalf("@wailsio/runtime pin %q must carry a 3.0.0-alpha.N tag", raw)
	}
	// Phantom-alpha2 guard: no alpha2 runtime exists on npm (anti-pattern).
	if m[1] == "2" {
		t.Errorf("@wailsio/runtime pin %q uses a phantom alpha2.* tag -- npm publishes no alpha2 runtime; Go-side currency does not license runtime skew. Relax this guard ONLY if upstream publishes a matching runtime and it is adopted.", raw)
	}
	n, _ := strconv.Atoi(m[2])
	if n < jsRuntimeCurrentAlpha {
		t.Errorf("@wailsio/runtime alpha.%d must be >= the current pin alpha.%d (never regress the runtime)", n, jsRuntimeCurrentAlpha)
	}
}

// TestPackageLockRuntimeNotRegressed asserts the lockfile's @wailsio/runtime
// resolution is not regressed below alpha.79 and carries no phantom alpha2.* tag.
func TestPackageLockRuntimeNotRegressed(t *testing.T) {
	relPath := "frontend/package-lock.json"
	if !fileExists(t, relPath) {
		t.Fatalf("%s must exist", relPath)
	}
	src := readSource(t, relPath)
	sawRuntime := false
	for _, line := range strings.Split(src, "\n") {
		if !strings.Contains(line, "wailsio") {
			continue
		}
		m := jsRuntimeRe.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		sawRuntime = true
		if m[1] == "2" {
			t.Errorf("package-lock.json resolves @wailsio/runtime to a phantom alpha2.* tag -- no such runtime is published")
			continue
		}
		if n, err := strconv.Atoi(m[2]); err == nil && n < jsRuntimeCurrentAlpha {
			t.Errorf("package-lock.json carries @wailsio/runtime alpha.%d, older than the current pin alpha.%d -- lockfile is corrupt or out of sync", n, jsRuntimeCurrentAlpha)
		}
	}
	if !sawRuntime {
		t.Errorf("package-lock.json must reference an @wailsio/runtime 3.0.0-alpha.N resolution")
	}
}
