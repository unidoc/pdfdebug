// AC 1, 2, 3: version-pin assertions across go.mod, go.sum, the two CI
// workflows, and the frontend runtime. All compare on the unified alpha-ordinal
// so both the alpha2.103 target and the alpha.102 fallback (AC 10) pass.
package story_12_3_wails_alpha2_103_upgrade_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// AC 1 -- Go dependency bumped
// ---------------------------------------------------------------------------

// TestGoModWailsBumped asserts go.mod's wails/v3 pin is strictly newer than the
// pre-bump baseline alpha.95.
func TestGoModWailsBumped(t *testing.T) {
	line := goWailsLine(t)
	if line == "" {
		t.Fatalf("go.mod must declare a `github.com/wailsapp/wails/v3` require")
	}
	got := alphaOrdinal(goWailsRe, line)
	if got < 0 {
		t.Fatalf("go.mod wails/v3 line %q must carry a v3.0.0-alpha.N or v3.0.0-alpha2.N tag", line)
	}
	if got <= preBumpBaselineOrdinal {
		t.Errorf("go.mod wails/v3 pin %s is not strictly newer than the pre-bump baseline %s (target alpha2.103, fallback alpha.102 -- both clear this)",
			fmtOrdinal(got), fmtOrdinal(preBumpBaselineOrdinal))
	}
}

// TestGoSumCarriesNewPin asserts go.sum carries an entry for the bumped wails/v3
// pin, which proves `go mod tidy` ran. It skips cleanly while go.mod is still at
// the baseline so the failure signal stays on TestGoModWailsBumped.
func TestGoSumCarriesNewPin(t *testing.T) {
	line := goWailsLine(t)
	got := alphaOrdinal(goWailsRe, line)
	if got <= preBumpBaselineOrdinal {
		t.Skipf("skipped -- go.mod is not bumped past the baseline wails/v3 pin yet")
	}
	tag := tagFromOrdinal(got)
	gosum := readSource(t, "go.sum")
	needle := "github.com/wailsapp/wails/v3 v3.0.0-" + tag
	if !strings.Contains(gosum, needle) {
		t.Errorf("go.sum must contain %q -- run `go mod tidy` after editing go.mod (/ Task 1.1)", needle)
	}
}

// TestWebview2NotHandPinned asserts the indirect
// github.com/wailsapp/wails/webview2 dependency stays marked `// indirect` and is
// left for `go mod tidy` to resolve rather than hand-pinned, guarding against a
// dev manually freezing the transitive pin.
func TestWebview2NotHandPinned(t *testing.T) {
	src := readSource(t, "go.mod")
	var webview2Line string
	for _, l := range strings.Split(src, "\n") {
		if strings.Contains(l, "github.com/wailsapp/wails/webview2") {
			webview2Line = l
			break
		}
	}
	if webview2Line == "" {
		// Acceptable: alpha2.103 may not pull webview2 transitively at all.
		// Absence is not a failure; only a present-but-promoted-to-direct line is.
		return
	}
	if !strings.Contains(webview2Line, "// indirect") {
		t.Errorf("go.mod webview2 line %q must stay `// indirect` -- the story says let `go mod tidy` resolve it, do NOT hand-pin/promote it", strings.TrimSpace(webview2Line))
	}
}

// ---------------------------------------------------------------------------
// AC 2 -- CLI pins bumped in BOTH workflows, CLI == library exactly
// ---------------------------------------------------------------------------

// assertWorkflowPinMatchesGoMod is the shared body for the ci.yml / release.yml
// CLI-pin parity assertions: the workflow must install wails3 with a v3.0.0
// alpha pin, every alpha pin in the file must be strictly newer than the
// baseline, and every one must EQUAL the go.mod pin exactly (CLI == library).
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
		if n <= preBumpBaselineOrdinal {
			t.Errorf("%s carries %s -- must be strictly newer than baseline %s",
				relPath, fmtOrdinal(n), fmtOrdinal(preBumpBaselineOrdinal))
		}
		if goOrd > 0 && n != goOrd {
			t.Errorf("%s pin %s must EQUAL the go.mod pin %s exactly -- the CLI and the library must be the same version",
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
// EXPECTED_FILES=6 publish invariant, so a bump cannot disturb the artifact-count
// contract.
func TestReleaseExpectedFilesInvariant(t *testing.T) {
	src := readSource(t, ".github/workflows/release.yml")
	if !strings.Contains(src, "EXPECTED_FILES=6") {
		t.Errorf("release.yml must retain EXPECTED_FILES=6 invariant across the bump")
	}
}

// ---------------------------------------------------------------------------
// AC 3 -- Frontend runtime: verify-only, bump only if npm has a newer publish
// ---------------------------------------------------------------------------

// TestRuntimePinWellFormedNoPhantom asserts @wailsio/runtime is a well-formed
// 3.0.0-alpha.N tag with N at or above the pre-bump pin (alpha.79), and is not
// rewritten to a phantom alpha2.* tag: npm publishes no alpha2 runtime and one
// must not be invented.
func TestRuntimePinWellFormedNoPhantom(t *testing.T) {
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
	// Phantom-alpha2 guard: no alpha2 runtime exists on npm (AC3 anti-pattern).
	if m[1] == "2" {
		t.Errorf("@wailsio/runtime pin %q uses a phantom alpha2.* tag -- npm publishes no alpha2 runtime; the runtime stays on the alpha.N line", raw)
	}
	n, _ := strconv.Atoi(m[2])
	if n < jsRuntimePreBumpAlpha {
		t.Errorf("@wailsio/runtime alpha.%d must be >= the pre-bump pin alpha.%d (verify-only, never regress)", n, jsRuntimePreBumpAlpha)
	}
}

// TestPackageLockRuntimeNotRegressed asserts the lockfile's @wailsio/runtime
// resolution is not regressed below alpha.79. The expected outcome is no change at
// all -- the runtime stays at alpha.79 -- so this fails only on an older or
// phantom-alpha2 runtime, meaning a corrupt edit.
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
		if n, err := strconv.Atoi(m[2]); err == nil && n < jsRuntimePreBumpAlpha {
			t.Errorf("package-lock.json carries @wailsio/runtime alpha.%d, older than the pre-bump pin alpha.%d -- lockfile is corrupt or out of sync", n, jsRuntimePreBumpAlpha)
		}
	}
	if !sawRuntime {
		t.Errorf("package-lock.json must reference an @wailsio/runtime 3.0.0-alpha.N resolution")
	}
}

// tagFromOrdinal renders an ordinal back to the bare tag form used in go.sum
// (`alpha.N` / `alpha2.N`), without the leading `v3.0.0-`.
func tagFromOrdinal(ord int) string {
	const alpha2Base = 1000
	if ord >= alpha2Base {
		return "alpha2." + strconv.Itoa(ord-alpha2Base)
	}
	return "alpha." + strconv.Itoa(ord)
}
