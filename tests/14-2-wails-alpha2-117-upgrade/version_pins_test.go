// AC1, AC2, AC3.1, AC5, AC8: the CLI==library version-pin invariant plus the
// runtime-honesty guard.
//
// scenario 14.2-INTG-001 [P0] (risk R-14-11): the wails3 CLI pin in BOTH
// .github/workflows/ci.yml AND .github/workflows/release.yml must EQUAL the
// go.mod library pin exactly, and go.mod must reach the committed target
// v3.0.0-alpha2.117. This mirrors tests/12-3 INTG-020/021 -- it reads the files
// statically and does NOT shell out to `wails3 --version` (the live
// go.mod==wails3 equality is confirmed only on the legs where the CLI is
// installed: the 3-runner build and the manual smoke, out of this module's
// scope). A mismatch means everything downstream tests a phantom -- stop.
//
// Authored RED (go.mod at alpha2.103, below the alpha2.117 floor); now GREEN
// after the committed bump and standing as the regression gate against drift.
package story_14_2_wails_alpha2_117_upgrade_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// AC1/AC2 -- go.mod library pin reaches the committed target
// ---------------------------------------------------------------------------

// Test_14_2_INTG_001_GoModPinAtTarget [P0] AC1/AC2: go.mod's wails/v3 pin
// reaches the committed target alpha2.117. Authored RED (pin at alpha2.103,
// below the target floor); GREEN once the bump lands. The assertion is a floor (>= target), not
// an exact ==, so a defensibly-newer most-baked pick per AC1 still passes while
// the current pin and both fallback rungs (alpha2.103, alpha.102) stay red.
func Test_14_2_INTG_001_GoModPinAtTarget(t *testing.T) {
	line := goWailsLine(t)
	if line == "" {
		t.Fatalf("[P0] 14.2-INTG-001: go.mod must declare a `github.com/wailsapp/wails/v3` require")
	}
	got := alphaOrdinal(goWailsRe, line)
	if got < 0 {
		t.Fatalf("[P0] 14.2-INTG-001: go.mod wails/v3 line %q must carry a v3.0.0-alpha.N or v3.0.0-alpha2.N tag", line)
	}
	if got < targetOrdinal {
		t.Errorf("[P0] 14.2-INTG-001: go.mod wails/v3 pin %s has not reached the committed target %s (AC1/AC2). Current known-good fallback rung 1 = alpha2.103, deeper rung 2 = alpha.102 both sit below this floor.",
			fmtOrdinal(got), fmtOrdinal(targetOrdinal))
	}
}

// Test_14_2_INTG_001_GoSumCarriesNewPin [P0] AC2/AC8: go.sum carries an entry
// for the bumped wails/v3 pin (proves `go mod tidy` ran). Skips cleanly while
// go.mod is still below the target so the RED signal stays on
// Test_14_2_INTG_001_GoModPinAtTarget.
func Test_14_2_INTG_001_GoSumCarriesNewPin(t *testing.T) {
	got := alphaOrdinal(goWailsRe, goWailsLine(t))
	if got < targetOrdinal {
		t.Skipf("[P0] 14.2-INTG-001: skipped -- go.mod not bumped to target yet (see Test_14_2_INTG_001_GoModPinAtTarget)")
	}
	needle := "github.com/wailsapp/wails/v3 v3.0.0-" + tagFromOrdinal(got)
	gosum := readSource(t, "go.sum")
	if !strings.Contains(gosum, needle) {
		t.Errorf("[P0] 14.2-INTG-001: go.sum must contain %q -- run `go mod tidy` after editing go.mod (AC2 / Task 2.1)", needle)
	}
}

// ---------------------------------------------------------------------------
// AC3.1 -- CLI==library: wails3 install pin parity in BOTH workflows
// ---------------------------------------------------------------------------

// assertWorkflowPinMatchesGoMod is the shared body for the ci.yml / release.yml
// CLI-pin parity assertions (mirrors tests/12-3 INTG-020/021): the workflow must
// install wails3 with a v3.0.0 alpha pin, every alpha pin in the file must move
// strictly past the pre-bump baseline (alpha2.103), and every one must EQUAL the
// go.mod pin exactly (CLI == library). Authored RED (install line at alpha2.103,
// not strictly past the alpha2.103 baseline); GREEN once the workflows are bumped.
func assertWorkflowPinMatchesGoMod(t *testing.T, relPath, testID string) {
	t.Helper()
	src := readSource(t, relPath)
	if !strings.Contains(src, "wails3@v3.0.0-alpha") {
		t.Fatalf("[P0] %s: %s must install wails3 with a v3.0.0-alpha pin", testID, relPath)
	}
	got := allAlphaOrdinals(goWailsRe, src)
	if len(got) == 0 {
		t.Fatalf("[P0] %s: %s must reference a v3.0.0-alpha pin", testID, relPath)
	}
	goOrd := alphaOrdinal(goWailsRe, goWailsLine(t))
	for _, n := range got {
		if n <= currentBaselineOrdinal {
			t.Errorf("[P0] %s: %s carries %s -- a bump must move it strictly past the pre-bump baseline %s (AC3.1)",
				testID, relPath, fmtOrdinal(n), fmtOrdinal(currentBaselineOrdinal))
		}
		if goOrd > 0 && n != goOrd {
			t.Errorf("[P0] %s: %s wails3 CLI pin %s must EQUAL the go.mod library pin %s exactly (CLI == library, AC3.1)",
				testID, relPath, fmtOrdinal(n), fmtOrdinal(goOrd))
		}
	}
}

// Test_14_2_INTG_001_CiWorkflowPinParity [P0] AC3.1: ci.yml's wails3 CLI install
// pin is bumped and equals the go.mod library pin.
func Test_14_2_INTG_001_CiWorkflowPinParity(t *testing.T) {
	assertWorkflowPinMatchesGoMod(t, ".github/workflows/ci.yml", "14.2-INTG-001")
}

// Test_14_2_INTG_001_ReleaseWorkflowPinParity [P0] AC3.1: release.yml's wails3
// CLI install pin is bumped and equals the go.mod library pin.
func Test_14_2_INTG_001_ReleaseWorkflowPinParity(t *testing.T) {
	assertWorkflowPinMatchesGoMod(t, ".github/workflows/release.yml", "14.2-INTG-001")
}

// Test_14_2_INTG_001_ReleaseExpectedFilesInvariant [P1] AC8 (regression net):
// release.yml retains the EXPECTED_FILES=6 publish invariant. A framework bump
// must not disturb the artifact-count contract. Passes today; stands as a net.
func Test_14_2_INTG_001_ReleaseExpectedFilesInvariant(t *testing.T) {
	src := readSource(t, ".github/workflows/release.yml")
	if !strings.Contains(src, "EXPECTED_FILES=6") {
		t.Errorf("[P1] 14.2-INTG-001: release.yml must retain EXPECTED_FILES=6 invariant across the bump")
	}
}

// ---------------------------------------------------------------------------
// AC5/AC8 -- runtime held to what upstream publishes (no phantom alpha2)
// ---------------------------------------------------------------------------

// Test_14_2_INTG_004_RuntimePinNoPhantomAlpha2 [P0] AC5/AC8: @wailsio/runtime is
// a well-formed 3.0.0-alpha.N tag with N >= the current pin (alpha.79). It must
// NOT be rewritten to a phantom alpha2.* tag -- npm publishes no alpha2 runtime,
// and Go-side currency (the alpha2.117 library bump) does NOT license runtime
// skew. This mirrors tests/12-3 INTG-030 and stays GREEN today (runtime is
// alpha.79); it HARD-FAILS if the bump hand-rolls an alpha2 runtime string.
// NOTE (AC8): if upstream DOES publish a matching alpha2.* runtime and it is
// adopted, this guard must be relaxed in lockstep or it goes red.
func Test_14_2_INTG_004_RuntimePinNoPhantomAlpha2(t *testing.T) {
	src := readSource(t, "frontend/package.json")
	var pkg map[string]any
	if err := json.Unmarshal([]byte(src), &pkg); err != nil {
		t.Fatalf("[P0] 14.2-INTG-004: package.json is not valid JSON: %v", err)
	}
	deps, ok := pkg["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("[P0] 14.2-INTG-004: package.json must declare a dependencies object")
	}
	raw, ok := deps["@wailsio/runtime"].(string)
	if !ok {
		t.Fatalf("[P0] 14.2-INTG-004: dependencies must declare @wailsio/runtime as a string pin")
	}
	m := jsRuntimeRe.FindStringSubmatch(raw)
	if len(m) < 3 {
		t.Fatalf("[P0] 14.2-INTG-004: @wailsio/runtime pin %q must carry a 3.0.0-alpha.N tag", raw)
	}
	// Phantom-alpha2 guard: no alpha2 runtime exists on npm (AC5 anti-pattern).
	if m[1] == "2" {
		t.Errorf("[P0] 14.2-INTG-004: @wailsio/runtime pin %q uses a phantom alpha2.* tag -- npm publishes no alpha2 runtime; Go-side currency does not license runtime skew (AC5). Relax this guard ONLY if upstream publishes a matching runtime and it is adopted (AC8).", raw)
	}
	n, _ := strconv.Atoi(m[2])
	if n < jsRuntimeCurrentAlpha {
		t.Errorf("[P0] 14.2-INTG-004: @wailsio/runtime alpha.%d must be >= the current pin alpha.%d (AC5: never regress the runtime)", n, jsRuntimeCurrentAlpha)
	}
}

// Test_14_2_INTG_004_PackageLockRuntimeNotRegressed [P1] AC5/AC8: the lockfile's
// @wailsio/runtime resolution is not regressed below alpha.79 and carries no
// phantom alpha2.* tag. Mirrors tests/12-3 INTG-031. Passes today.
func Test_14_2_INTG_004_PackageLockRuntimeNotRegressed(t *testing.T) {
	relPath := "frontend/package-lock.json"
	if !fileExists(t, relPath) {
		t.Fatalf("[P1] 14.2-INTG-004: %s must exist", relPath)
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
			t.Errorf("[P1] 14.2-INTG-004: package-lock.json resolves @wailsio/runtime to a phantom alpha2.* tag -- no such runtime is published (AC5)")
			continue
		}
		if n, err := strconv.Atoi(m[2]); err == nil && n < jsRuntimeCurrentAlpha {
			t.Errorf("[P1] 14.2-INTG-004: package-lock.json carries @wailsio/runtime alpha.%d, older than the current pin alpha.%d -- lockfile is corrupt or out of sync (AC5)", n, jsRuntimeCurrentAlpha)
		}
	}
	if !sawRuntime {
		t.Errorf("[P1] 14.2-INTG-004: package-lock.json must reference an @wailsio/runtime 3.0.0-alpha.N resolution")
	}
}
