// Package release_pipeline_test: additional static-validation tests closing
// coverage gaps in the Story 7.2 release pipeline.
//
// These tests target concrete behaviors mandated by Story 7.2 tasks and Review
// findings that were not asserted by the original 26 acceptance tests:
//
//   - actions/download-artifact@v4 uses merge-multiple: true (AC #9 / Task 1.4)
//   - build-job artifact name pattern `build-${{ matrix.platform }}` (Task 4.4)
//   - build-job timeout-minutes: 45 budget for macOS notarize (Task 1.2)
//   - Wails CLI pin matches go.mod direct dependency (Task 1.3 #5)
//   - `wails3 generate bindings -clean=true` present BEFORE any frontend build
//     (Task 1.3 #7; story 7-1 Review #1 lesson carried forward)
//   - Go 1.25.x and Node 20 pins match ci.yml (cross-workflow consistency;
//     Dev Notes "Reuse Everything from Story 7-1")
//   - SemVer validation in Resolve version step (Task 1.3 #8; rejects crafted tags)
//   - Apple secret gate idiom (`steps.apple_secrets.outputs.available == 'true'`)
//     (Task 2.6)
//   - Platform-specific GUI build steps invoke `wails3 task <os>:build|package`
//     with ARCH from matrix (Task 3.1, 3.2)
//   - CLI `--help` smoke-test step per cell (Task 3.4)
//   - SHA256SUMS.txt integrity guard: EXPECTED_FILES=8, line-count invariant,
//     and `shasum -a 256 -c` self-verify (Review #3 Medium)
//   - fail_on_unmatched_files: true + files glob on action-gh-release
//     (AC #7 / Task 5.2)
//   - PlistBuddy version step strips BOTH `-pre` and `+build` SemVer suffixes
//     (Task 6.3; Review #1 Medium)
//   - workflow_dispatch checkout uses `ref: ${{ inputs.tag || github.ref }}`
//     (Review #1 Medium; tag-commit source-of-truth on manual dispatch)
//   - Signing-identity lookup uses `grep -qF` fixed-string match (Review #3 Low;
//     Apple Developer ID strings contain regex metachars `.` and `()`)
//   - KEYCHAIN_PASS is NOT exported to $GITHUB_ENV (Review #2 Medium; GHA does
//     not auto-mask non-secrets env values)
//   - Pre-release tag detection regex covers -rc, -alpha, -beta (cross-check
//     with TestPrereleaseDetectionLogic; this asserts BOTH branches match)
//   - Entitlements plist is tracked (not gitignored) per Review #1 Critical
//
// All tests stay at integration/Go level (lowest viable layer for
// infrastructure-as-code), per the caller's directive:
// "push new tests to the lowest viable layer; do NOT add E2E tests for scenarios
// already covered at lower layers; only add E2E to fill gaps in critical
// happy-path coverage."
//
// The single E2E scenario in the epic test design (7.2-E2E-001: RC tag produces
// downloadable assets) is documented as a manual post-merge verification step
// and cannot be exercised from a Go test; no new E2E is added here.
//
// Run: cd tests/release-pipeline && go test -v -count=1 ./...
package release_pipeline_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// findStepByPredicate returns the first step in the named job where predicate
// returns true, or nil if none match.
func findStepByPredicate(t *testing.T, jobName string, predicate func(map[string]interface{}) bool) map[string]interface{} {
	t.Helper()
	for _, s := range jobSteps(t, jobName) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if predicate(m) {
			return m
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-004 (P1): download-artifact uses merge-multiple: true
// Covers AC #9 + Task 1.4 (single `dist/` merges all four matrix cells)
// ---------------------------------------------------------------------------

func TestDownloadArtifactMergeMultiple(t *testing.T) {
	step := findStepByPredicate(t, "release", func(m map[string]interface{}) bool {
		u, _ := m["uses"].(string)
		return strings.HasPrefix(u, "actions/download-artifact@v4")
	})
	if step == nil {
		t.Fatalf("release.yml: actions/download-artifact@v4 step missing in release job (AC #9)")
	}
	with, ok := step["with"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: download-artifact step missing `with:` block")
	}
	mm, ok := with["merge-multiple"].(bool)
	if !ok || !mm {
		t.Errorf("release.yml: download-artifact `with.merge-multiple` must be true (Task 1.4; merges 4 matrix cells into single dist/)")
	}
	pattern, _ := with["pattern"].(string)
	if pattern != "build-*" {
		t.Errorf("release.yml: download-artifact `with.pattern` must be \"build-*\" (Task 1.4), got %q", pattern)
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-004 (P1): upload-artifact uses `build-${{ matrix.platform }}` name
// Covers Task 4.4 (stable per-cell artifact name consumed by download-artifact
// pattern: build-*)
// ---------------------------------------------------------------------------

func TestUploadArtifactNamePattern(t *testing.T) {
	step := findStepByPredicate(t, "build", func(m map[string]interface{}) bool {
		u, _ := m["uses"].(string)
		return strings.HasPrefix(u, "actions/upload-artifact@v4")
	})
	if step == nil {
		t.Fatalf("release.yml: actions/upload-artifact@v4 step missing in build job (AC #9)")
	}
	with, ok := step["with"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: upload-artifact step missing `with:` block")
	}
	name, _ := with["name"].(string)
	// GHA templating leaves the `${{ ... }}` in the raw string.
	if !strings.Contains(name, "build-") || !strings.Contains(name, "matrix.platform") {
		t.Errorf("release.yml: upload-artifact `with.name` must be \"build-${{ matrix.platform }}\" (Task 4.4), got %q", name)
	}
}

// ---------------------------------------------------------------------------
// Build job has an explicit timeout-minutes budget.
// Notarization is currently disabled, so the budget is sized for build + sign
// only. Re-raise the upper bound when notarization is re-enabled.
// ---------------------------------------------------------------------------

func TestBuildJobTimeoutMinutes(t *testing.T) {
	job := jobMap(t, "build")
	timeout, ok := job["timeout-minutes"].(int)
	if !ok {
		t.Fatalf("release.yml: jobs.build.timeout-minutes missing or not an int (Task 1.2)")
	}
	if timeout < 15 || timeout > 60 {
		t.Errorf("release.yml: jobs.build.timeout-minutes should be in [15, 60] (build+sign budget, no notarize), got %d", timeout)
	}
}

// ---------------------------------------------------------------------------
// Wails CLI install pin matches the go.mod direct dependency version.
// Covers Task 1.3 #5 + Dev Notes "Version Pins -- Source of Truth".
// Divergence is E7-R-002 (Wails v3 alpha cross-platform build) high risk.
// ---------------------------------------------------------------------------

func TestWailsCLIPinMatchesGoMod(t *testing.T) {
	raw := readReleaseWorkflow(t)

	// Extract the pin used in the workflow.
	wfRe := regexp.MustCompile(`github\.com/wailsapp/wails/v3/cmd/wails3@(v\S+)`)
	wfMatch := wfRe.FindStringSubmatch(raw)
	if wfMatch == nil {
		t.Fatalf("release.yml: `go install github.com/wailsapp/wails/v3/cmd/wails3@<pin>` not found (Task 1.3 #5)")
	}
	wfPin := wfMatch[1]

	root := projectRoot(t)
	goModRaw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("cannot read go.mod: %v", err)
	}
	// Match any wailsapp/wails/v3 direct-dep line in go.mod (not an indirect CLI).
	gmRe := regexp.MustCompile(`github\.com/wailsapp/wails/v3\s+(v\S+)`)
	gmMatch := gmRe.FindStringSubmatch(string(goModRaw))
	if gmMatch == nil {
		t.Fatalf("go.mod: no `github.com/wailsapp/wails/v3 v...` direct dependency found")
	}
	gmPin := gmMatch[1]

	if wfPin != gmPin {
		t.Errorf("release.yml Wails CLI pin %q must match go.mod wails/v3 pin %q (Task 1.3 #5; E7-R-002 mitigation)", wfPin, gmPin)
	}
}

// ---------------------------------------------------------------------------
// `wails3 generate bindings -clean=true` runs BEFORE any frontend/GUI build step.
// Covers Task 1.3 #7 + story 7-1 Review #1 carry-forward (frontend/bindings/
// is gitignored; all frontend steps fail without it).
// ---------------------------------------------------------------------------

func TestWailsBindingsGeneratedBeforeBuild(t *testing.T) {
	steps := jobSteps(t, "build")

	bindingsIdx := -1
	firstBuildIdx := -1
	for i, s := range steps {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		run, _ := m["run"].(string)
		if bindingsIdx == -1 && strings.Contains(run, "wails3 generate bindings") {
			bindingsIdx = i
		}
		// First step that invokes a `wails3 task ...:build|package` IS a GUI build.
		if firstBuildIdx == -1 && regexp.MustCompile(`wails3 task \w+:(build|package)`).MatchString(run) {
			firstBuildIdx = i
		}
	}

	if bindingsIdx == -1 {
		t.Errorf("release.yml build job: `wails3 generate bindings -clean=true` step missing (Task 1.3 #7; gitignored bindings would break the build)")
		return
	}
	if firstBuildIdx == -1 {
		t.Errorf("release.yml build job: no `wails3 task <os>:build|package` step found")
		return
	}
	if bindingsIdx >= firstBuildIdx {
		t.Errorf("release.yml build job: `wails3 generate bindings` step (idx %d) must run BEFORE the first `wails3 task ...:build` step (idx %d)", bindingsIdx, firstBuildIdx)
	}

	// Also assert the `-clean=true` flag (Task 1.3 #7 prescribes it exactly).
	run := jobRunBodies(t, "build")
	if !strings.Contains(run, "wails3 generate bindings -clean=true") {
		t.Errorf("release.yml build job: `wails3 generate bindings` must pass `-clean=true` (Task 1.3 #7)")
	}
}

// ---------------------------------------------------------------------------
// Go + Node pins in release.yml match ci.yml (Dev Notes "Reuse Everything").
// Divergence between the two workflows is E7-R-005 (CI matrix divergence).
// ---------------------------------------------------------------------------

func TestGoAndNodePinsMatchCIWorkflow(t *testing.T) {
	root := projectRoot(t)
	ciRaw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("cannot read .github/workflows/ci.yml: %v", err)
	}
	relRaw := readReleaseWorkflow(t)

	// setup-go go-version pin
	goRe := regexp.MustCompile(`go-version:\s*'([^']+)'`)
	ciGo := goRe.FindStringSubmatch(string(ciRaw))
	relGo := goRe.FindStringSubmatch(relRaw)
	if ciGo == nil {
		t.Fatalf("ci.yml: setup-go go-version not found")
	}
	if relGo == nil {
		t.Fatalf("release.yml: setup-go go-version not found")
	}
	if ciGo[1] != relGo[1] {
		t.Errorf("release.yml setup-go pin %q must match ci.yml pin %q (E7-R-005 mitigation)", relGo[1], ciGo[1])
	}

	// setup-node node-version pin
	nodeRe := regexp.MustCompile(`node-version:\s*'([^']+)'`)
	ciNode := nodeRe.FindStringSubmatch(string(ciRaw))
	relNode := nodeRe.FindStringSubmatch(relRaw)
	if ciNode == nil || relNode == nil {
		t.Fatalf("setup-node node-version not found in one of the workflows (ci=%v release=%v)", ciNode != nil, relNode != nil)
	}
	if ciNode[1] != relNode[1] {
		t.Errorf("release.yml setup-node pin %q must match ci.yml pin %q (E7-R-005 mitigation)", relNode[1], ciNode[1])
	}
}

// ---------------------------------------------------------------------------
// Resolve version step validates SemVer and refuses non-matching tags.
// Covers Task 1.3 #8 + Dev Notes injection-hardening (crafted tags otherwise
// flow into ldflags / PlistBuddy / artifact filenames).
// ---------------------------------------------------------------------------

func TestResolveVersionRejectsNonSemVer(t *testing.T) {
	run := jobRunBodies(t, "build")

	// Must contain SemVer regex anchored to full string.
	if !regexp.MustCompile(`\^v\[0-9\]\+\\?\.\[0-9\]\+\\?\.\[0-9\]\+`).MatchString(run) {
		t.Errorf("release.yml build job: Resolve version step must validate tag against SemVer regex `^v[0-9]+\\.[0-9]+\\.[0-9]+...` (Task 1.3 #8; injection hardening)")
	}
	// Must explicitly refuse + exit non-zero on mismatch.
	if !strings.Contains(run, "Refusing to build") {
		t.Errorf("release.yml build job: Resolve version step must emit `Refusing to build` + `exit 1` on non-SemVer tag (Task 1.3 #8)")
	}
	// Same validation must repeat in the release job's Resolve release tag step.
	relRun := jobRunBodies(t, "release")
	if !strings.Contains(relRun, "Refusing to publish") {
		t.Errorf("release.yml release job: Resolve release tag step must re-validate SemVer on workflow_dispatch inputs (defense-in-depth; a dispatch caller could otherwise publish a release under an arbitrary tag name)")
	}
}

// ---------------------------------------------------------------------------
// Apple signing steps are gated by `steps.apple_secrets.outputs.available ==
// 'true'`, enabling the workflow to merge + CI-test without real secrets
// (Task 2.6).
// ---------------------------------------------------------------------------

func TestAppleSignGatingOutput(t *testing.T) {
	steps := jobSteps(t, "build")

	// Detect the apple_secrets probe step.
	var probeStep map[string]interface{}
	for _, s := range steps {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if id, _ := m["id"].(string); id == "apple_secrets" {
			probeStep = m
			break
		}
	}
	if probeStep == nil {
		t.Fatalf("release.yml build job: step with `id: apple_secrets` missing (Task 2.6 gating idiom)")
	}
	// Must emit `available=true|false` via GITHUB_OUTPUT.
	run, _ := probeStep["run"].(string)
	if !strings.Contains(run, "available=true") || !strings.Contains(run, "available=false") {
		t.Errorf("release.yml: apple_secrets probe must emit both `available=true` and `available=false` via GITHUB_OUTPUT (Task 2.6)")
	}

	// Every codesign step must gate on the output. Notarization is currently
	// disabled; re-add `xcrun notarytool submit` here when re-enabled.
	gateExpr := "steps.apple_secrets.outputs.available == 'true'"
	gatedNeedles := []string{
		"security create-keychain",   // Import Apple Developer ID cert
		"codesign --force",           // Sign macOS app bundle
	}
	for _, needle := range gatedNeedles {
		for _, s := range steps {
			m, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			r, _ := m["run"].(string)
			if !strings.Contains(r, needle) {
				continue
			}
			ifClause, _ := m["if"].(string)
			if !strings.Contains(ifClause, gateExpr) {
				name, _ := m["name"].(string)
				t.Errorf("release.yml: step %q runs `%s` but does not gate on `%s` (Task 2.6; unsigned branch would still try to sign)", name, needle, gateExpr)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Platform-specific GUI build steps invoke `wails3 task <os>:<target>` with
// ARCH sourced from matrix.arch (Task 3.1, 3.2; host-only `wails3 build` is
// the anti-pattern).
// ---------------------------------------------------------------------------

func TestPlatformGUIBuildSteps(t *testing.T) {
	raw := readReleaseWorkflow(t)

	cases := []struct {
		desc    string
		pattern string
	}{
		{"macOS package", `wails3 task darwin:package ARCH=`},
		{"Windows build", `wails3 task windows:build ARCH=`},
		{"Linux build", `wails3 task linux:build ARCH=`},
	}
	for _, c := range cases {
		if !strings.Contains(raw, c.pattern) {
			t.Errorf("release.yml: %s step must invoke `%s...` (Task 3.1/3.2)", c.desc, c.pattern)
		}
	}

	// Anti-pattern: bare `wails3 build` (host-only smoke; Dev Notes #6 from 7-1).
	if regexp.MustCompile(`(?m)^\s*wails3 build(\s|$)`).MatchString(raw) {
		t.Errorf("release.yml: bare `wails3 build` is forbidden (host-only smoke, not arch-specific). Use `wails3 task <os>:<target>`.")
	}
}

// ---------------------------------------------------------------------------
// CLI smoke test step is present (per-cell binary launch check).
// Covers Task 3.4 (risk E7-R-002 mitigation: confirm the produced binary
// launches before uploading as a release asset).
// ---------------------------------------------------------------------------

func TestCLISmokeTestStep(t *testing.T) {
	// Find the smoke-test step by name (the run body uses a bash `bin` local
	// variable so `pdfdebug` and `--help` are not on the same line as the invocation).
	step := findStepByPredicate(t, "build", func(m map[string]interface{}) bool {
		name, _ := m["name"].(string)
		return strings.Contains(strings.ToLower(name), "smoke")
	})
	if step == nil {
		t.Fatalf("release.yml build job: CLI smoke test step missing (Task 3.4; must invoke the produced binary with `--help` or `--version` to confirm launch)")
	}
	r, _ := step["run"].(string)

	// The step body must reference the CLI binary name AND a harmless invocation flag.
	if !strings.Contains(r, "pdfdebug") {
		t.Errorf("release.yml build job: CLI smoke step does not reference `pdfdebug` binary (Task 3.4)")
	}
	if !strings.Contains(r, "--help") && !strings.Contains(r, "--version") {
		t.Errorf("release.yml build job: CLI smoke step must invoke `--help` or `--version` (Task 3.4)")
	}

	// Must run on every cell, not gated to a single matrix.os.
	ifClause, _ := step["if"].(string)
	if ifClause != "" {
		if strings.Contains(ifClause, "matrix.os") && strings.Contains(ifClause, "==") {
			t.Errorf("release.yml: CLI smoke test step restricts to a single platform via `%s` -- must run on all 4 cells (Task 3.4)", ifClause)
		}
	}
}

// ---------------------------------------------------------------------------
// SHA256SUMS integrity guard: asserts EXPECTED_FILES=8, line-count invariant,
// and `shasum -a 256 -c` self-verify.
// Covers Review #3 Medium (AC #7 "all 8 assets MUST be attached" invariant).
// ---------------------------------------------------------------------------

func TestSHA256SumsIntegrityGuard(t *testing.T) {
	run := jobRunBodies(t, "release")

	// Explicit artifact-count assertion (8 = 4 platforms x 2 artifacts).
	if !strings.Contains(run, "EXPECTED_FILES=8") {
		t.Errorf("release.yml release job: SHA256SUMS step must assert `EXPECTED_FILES=8` (Review #3 Medium; AC #7 invariant)")
	}
	// Line-count vs file-count consistency check.
	if !regexp.MustCompile(`\$FILES.*!=.*\$LINES|\$LINES.*!=.*\$FILES`).MatchString(run) {
		t.Errorf("release.yml release job: SHA256SUMS step must compare FILES vs LINES and fail on mismatch (Review #3 Medium)")
	}
	// Self-verify with `shasum -a 256 -c` so a corrupted manifest fails
	// before publication, not silently post-upload.
	if !strings.Contains(run, "shasum -a 256 -c") {
		t.Errorf("release.yml release job: SHA256SUMS step must self-verify via `shasum -a 256 -c SHA256SUMS.txt` before publication")
	}
}

// ---------------------------------------------------------------------------
// action-gh-release publish step uploads `dist/*` (picks up SHA256SUMS.txt)
// and fails on unmatched files.
// Covers AC #8 (SHA256SUMS.txt uploaded as 9th asset) + Task 5.2.
// ---------------------------------------------------------------------------

func TestReleasePublishFilesGlob(t *testing.T) {
	step := findStepByPredicate(t, "release", func(m map[string]interface{}) bool {
		u, _ := m["uses"].(string)
		return strings.HasPrefix(u, "softprops/action-gh-release@")
	})
	if step == nil {
		t.Fatalf("release.yml: softprops/action-gh-release step missing")
	}
	with, ok := step["with"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: action-gh-release `with:` missing")
	}

	files, _ := with["files"].(string)
	// `files:` is a multi-line string; look for `dist/*` on any line.
	if !regexp.MustCompile(`(?m)^\s*dist/\*\s*$`).MatchString(files) {
		t.Errorf("release.yml: action-gh-release `with.files` must include `dist/*` so SHA256SUMS.txt is uploaded (AC #8)")
	}
	// Re-assert fail_on_unmatched_files: TestGenerateReleaseNotesEnabled already
	// covers this, but we include it here for the gap-test's explicit traceability
	// to Task 5.2.
	fof, _ := with["fail_on_unmatched_files"].(bool)
	if !fof {
		t.Errorf("release.yml: action-gh-release `with.fail_on_unmatched_files` must be true (Task 5.2)")
	}

	// tag_name and name must be parameterized to the resolved tag output (not
	// hard-coded and not based on github.ref_name, which is a branch ref on
	// workflow_dispatch).
	tagName, _ := with["tag_name"].(string)
	if !strings.Contains(tagName, "steps.tag.outputs.tag") {
		t.Errorf("release.yml: action-gh-release `with.tag_name` must reference `steps.tag.outputs.tag` (Review #1 Medium; workflow_dispatch safety), got %q", tagName)
	}
}

// ---------------------------------------------------------------------------
// PlistBuddy version step strips BOTH `-pre` and `+build` SemVer suffixes.
// Covers Task 6.3 + Review #1 Medium (Apple validators reject non-integer
// CFBundleShortVersionString / CFBundleVersion).
// ---------------------------------------------------------------------------

func TestPlistBuddyStripsBothSuffixes(t *testing.T) {
	run := jobRunBodies(t, "build")

	if !strings.Contains(run, "PlistBuddy") {
		t.Errorf("release.yml build job: PlistBuddy step missing (Task 6.3)")
		return
	}

	// Pre-release suffix strip: `${VERSION%%-*}`
	if !strings.Contains(run, "${VERSION%%-*}") {
		t.Errorf("release.yml: PlistBuddy step must strip SemVer pre-release suffix via `${VERSION%%-*}` (Task 6.3)")
	}
	// Build-metadata suffix strip: `${PLIST_VERSION%%+*}` or similar.
	// Review #1 Medium explicitly required this to handle `1.2.3+build.1`.
	if !regexp.MustCompile(`\$\{[A-Z_]+%%\+\*\}`).MatchString(run) {
		t.Errorf("release.yml: PlistBuddy step must also strip SemVer build-metadata suffix via `${VAR%%+*}` (Review #1 Medium: Apple validators reject `+build.N`)")
	}
	// Both plist keys must be set.
	for _, key := range []string{"CFBundleShortVersionString", "CFBundleVersion"} {
		if !strings.Contains(run, key) {
			t.Errorf("release.yml: PlistBuddy step must set %q (Task 6.3)", key)
		}
	}
}

// ---------------------------------------------------------------------------
// workflow_dispatch checkout uses the requested tag ref (not branch HEAD).
// Covers Review #1 Medium (workflow_dispatch otherwise builds wrong commit).
// ---------------------------------------------------------------------------

func TestCheckoutUsesTagRefOnDispatch(t *testing.T) {
	step := findStepByPredicate(t, "build", func(m map[string]interface{}) bool {
		u, _ := m["uses"].(string)
		return strings.HasPrefix(u, "actions/checkout@")
	})
	if step == nil {
		t.Fatalf("release.yml build job: actions/checkout step missing")
	}
	with, ok := step["with"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml build job: checkout `with:` missing")
	}
	ref, _ := with["ref"].(string)
	// Expected: `${{ inputs.tag || github.ref }}`
	if !strings.Contains(ref, "inputs.tag") || !strings.Contains(ref, "github.ref") {
		t.Errorf("release.yml build job: checkout `with.ref` must be `${{ inputs.tag || github.ref }}` (Review #1 Medium; otherwise workflow_dispatch builds branch HEAD, not the requested tag commit), got %q", ref)
	}
}

// ---------------------------------------------------------------------------
// Signing-identity lookup uses `grep -qF` (fixed-string), not `grep -q`.
// Covers Review #3 Low (Apple Developer ID strings contain `.` and `()` which
// are BRE regex metacharacters; `grep -q` could accept a near-miss identity).
// ---------------------------------------------------------------------------

func TestSigningIdentityGrepFixedString(t *testing.T) {
	run := jobRunBodies(t, "build")

	// Anti-pattern: plain `grep -q "$IDENTITY"` in the signing identity check.
	// The check should use `grep -qF` or `grep -F -q`.
	if regexp.MustCompile(`find-identity[^|]*\|\s*grep\s+-q\s+"\$IDENTITY"`).MatchString(run) {
		t.Errorf("release.yml build job: signing-identity check must use `grep -qF` (fixed-string), not `grep -q` (Review #3 Low; Apple identity strings contain regex metachars `.` and `()`)")
	}
	// Positive: the `-F` flag must appear in the find-identity pipeline.
	// Accept any of: grep -qF / grep -Fq / grep -F -q / grep -q -F
	if !regexp.MustCompile(`find-identity[^|]*\|\s*grep\s+(-qF|-Fq|-q -F|-F -q)`).MatchString(run) {
		t.Errorf("release.yml build job: signing-identity check must pipe through `grep -qF` (Review #3 Low)")
	}
}

// ---------------------------------------------------------------------------
// KEYCHAIN_PASS is NOT exported to $GITHUB_ENV.
// Covers Review #2 Medium (values written to $GITHUB_ENV are visible to all
// downstream steps AND are NOT auto-masked by GHA).
// ---------------------------------------------------------------------------

func TestKeychainPassNotInGithubEnv(t *testing.T) {
	run := jobRunBodies(t, "build")

	// Anti-pattern: any line that writes KEYCHAIN_PASS to $GITHUB_ENV.
	// Allow KEYCHAIN=... to $GITHUB_ENV (the path is not sensitive), but never the password.
	if regexp.MustCompile(`KEYCHAIN_PASS\s*=.*>>\s*"?\$GITHUB_ENV"?`).MatchString(run) {
		t.Errorf("release.yml build job: KEYCHAIN_PASS must NOT be exported to $GITHUB_ENV (Review #2 Medium; GHA does not auto-mask env values not written by ${{ secrets.* }}, and every downstream step would inherit the ephemeral password)")
	}
	// Also defend against `echo "KEYCHAIN_PASS=..."` patterns targeting GITHUB_ENV.
	if regexp.MustCompile(`echo\s+"KEYCHAIN_PASS=[^"]*"\s*>>\s*"?\$GITHUB_ENV"?`).MatchString(run) {
		t.Errorf("release.yml build job: `echo \"KEYCHAIN_PASS=...\" >> $GITHUB_ENV` pattern detected (Review #2 Medium)")
	}
}

// ---------------------------------------------------------------------------
// Entitlements plist is tracked (not gitignored).
// Covers Review #1 Critical (initial version had the file un-committable
// because `.gitignore` matched `build/*/entitlements.plist`).
// ---------------------------------------------------------------------------

func TestEntitlementsPlistNotGitignored(t *testing.T) {
	root := projectRoot(t)

	// The file itself must be present (already checked by TestEntitlementsPlistExists).
	p := filepath.Join(root, "build", "darwin", "entitlements.plist")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("build/darwin/entitlements.plist not present: %v", err)
	}

	giPath := filepath.Join(root, ".gitignore")
	giRaw, err := os.ReadFile(giPath)
	if err != nil {
		// If there's no .gitignore, the file is tracked by default.
		return
	}
	gi := string(giRaw)

	// Look for a negation re-including the file. Acceptable forms:
	//   !build/darwin/entitlements.plist
	//   !**/entitlements.plist
	//   !entitlements.plist
	// Absence of ANY rule that would match the file is also fine -- but the
	// repo's .gitignore intentionally ignores `build/<os>/*.plist` (Review #1
	// Critical documents this), so we require the explicit negation line.
	hasIgnoreRule := regexp.MustCompile(`(?m)^\s*build/.*\.plist\s*$|^\s*\*\.plist\s*$|^\s*entitlements\.plist\s*$`).MatchString(gi)
	hasNegation := strings.Contains(gi, "!build/darwin/entitlements.plist") ||
		strings.Contains(gi, "!**/entitlements.plist") ||
		strings.Contains(gi, "!entitlements.plist")

	if hasIgnoreRule && !hasNegation {
		t.Errorf(".gitignore: ignores build/*.plist but missing `!build/darwin/entitlements.plist` negation (Review #1 Critical; workflow would fail with 'entitlements file not found')")
	}
}

// ---------------------------------------------------------------------------
// Prerelease detection regex matches both `-rcN`, `-alphaN`, and `-betaN`.
// Covers AC #7 and verifies the exact contract via simulated shell-regex match
// (the existing TestPrereleaseDetectionLogic only asserts the regex STRING is
// present; this asserts the matching behavior across all three prefixes).
// ---------------------------------------------------------------------------

func TestPrereleaseRegexMatchesAllPrefixes(t *testing.T) {
	run := jobRunBodies(t, "release")

	// Extract the RHS of the bash =~ test: we expect `=~ -(rc|alpha|beta)`.
	re := regexp.MustCompile(`=~\s*(-\(rc\|alpha\|beta\))`)
	m := re.FindStringSubmatch(run)
	if m == nil {
		t.Fatalf("release.yml: prerelease bash test missing `=~ -(rc|alpha|beta)` (AC #7)")
	}

	// Simulate the bash regex in Go (ERE): `-(rc|alpha|beta)`
	goRe := regexp.MustCompile(`-(rc|alpha|beta)`)

	// Positive cases -- must match (pre-release).
	for _, tag := range []string{
		"v0.0.0-rc1",
		"v1.2.3-rc.1",
		"v0.1.0-alpha1",
		"v0.1.0-alpha.2",
		"v0.1.0-beta3",
	} {
		if !goRe.MatchString(tag) {
			t.Errorf("release.yml prerelease regex should match %q as pre-release (AC #7)", tag)
		}
	}
	// Negative cases -- must NOT match (production tag).
	for _, tag := range []string{
		"v0.1.0",
		"v1.0.0",
		"v10.20.30",
	} {
		if goRe.MatchString(tag) {
			t.Errorf("release.yml prerelease regex should NOT match %q as pre-release (AC #7)", tag)
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow_dispatch `tag` input has `required: true` and type `string`.
// Covers Task 1.1 + Task 1.5 contract (missing input causes immediate failure
// rather than defaulting to branch HEAD).
// ---------------------------------------------------------------------------

func TestWorkflowDispatchTagInputTyped(t *testing.T) {
	parsed := parseReleaseWorkflow(t)
	on, _ := parsed["on"].(map[string]interface{})
	wd, _ := on["workflow_dispatch"].(map[string]interface{})
	inputs, _ := wd["inputs"].(map[string]interface{})
	tag, ok := inputs["tag"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: workflow_dispatch.inputs.tag is not a map (Task 1.5)")
	}
	if req, _ := tag["required"].(bool); !req {
		t.Errorf("release.yml: workflow_dispatch.inputs.tag.required must be true (Task 1.5; otherwise an empty dispatch silently falls back to branch HEAD)")
	}
	if ty, _ := tag["type"].(string); ty != "string" {
		t.Errorf("release.yml: workflow_dispatch.inputs.tag.type must be \"string\", got %q", ty)
	}
}

// ---------------------------------------------------------------------------
// Apple-secrets probe emits a `::warning::` on partial-secret state.
// Covers Story 8-5 AC #1 pre-flight (the dry-run pre-flight relies on a
// partial state being noisy so it isn't accidentally taken for fully-absent;
// release.yml line 117 emits this warning only when SOME but not ALL of the
// three codesign secrets are set).
// ---------------------------------------------------------------------------

func TestAppleSecretsPartialStateEmitsWarning(t *testing.T) {
	step := findStepByPredicate(t, "build", func(m map[string]interface{}) bool {
		id, _ := m["id"].(string)
		return id == "apple_secrets"
	})
	if step == nil {
		t.Fatalf("release.yml build job: step with `id: apple_secrets` missing (Story 8-5 AC #1 pre-flight)")
	}
	run, _ := step["run"].(string)

	// Must emit a `::warning::` annotation when partial-secret state is detected.
	// The annotation surfaces in the Actions UI so a partial state is not
	// silently treated as "fully absent" (see Story 8-5 AC #1 + Task 1.1).
	if !strings.Contains(run, "::warning::") {
		t.Errorf("release.yml: apple_secrets probe must emit `::warning::` on partial-secret state (Story 8-5 AC #1; otherwise partial config silently degrades to unsigned)")
	}
}

// ---------------------------------------------------------------------------
// Linux native runtime deps are installed in the build job.
// Covers Story 8-5 AC #5: the Linux artifact must be runnable, which depends
// on `libgtk-3` and `libwebkit2gtk-4.1` being present on the runner. The
// install step at release.yml lines 62-63 is also the README's source of
// truth for the runtime-deps note end users follow.
// ---------------------------------------------------------------------------

func TestLinuxRuntimeDepsInstalled(t *testing.T) {
	step := findStepByPredicate(t, "build", func(m map[string]interface{}) bool {
		name, _ := m["name"].(string)
		return strings.Contains(strings.ToLower(name), "linux") && strings.Contains(strings.ToLower(name), "deps")
	})
	if step == nil {
		t.Fatalf("release.yml build job: Linux native deps install step missing (Story 8-5 AC #5)")
	}
	run, _ := step["run"].(string)

	for _, pkg := range []string{"libgtk-3-dev", "libwebkit2gtk-4.1-dev"} {
		if !strings.Contains(run, pkg) {
			t.Errorf("release.yml Linux deps step: missing apt package %q (Story 8-5 AC #5; runtime deps documented in README)", pkg)
		}
	}

	// Must be gated to Linux runners so macOS/Windows cells don't spin on apt-get.
	ifClause, _ := step["if"].(string)
	if !strings.Contains(ifClause, "Linux") && !strings.Contains(ifClause, "ubuntu") {
		t.Errorf("release.yml Linux deps step: must gate on `runner.os == 'Linux'` or matrix.os == 'ubuntu-*', got %q", ifClause)
	}
}
