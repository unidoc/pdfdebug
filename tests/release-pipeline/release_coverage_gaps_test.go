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
//   - UNSIGNED suffix applied when SIGNED != true (Task 4.2)
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
	"os/exec"
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
// Covers Task 1.2 (macOS notarize can legit take 10+ minutes; budget 45 min
// total so a single stuck cell cannot eat the job indefinitely).
// ---------------------------------------------------------------------------

func TestBuildJobTimeoutMinutes(t *testing.T) {
	job := jobMap(t, "build")
	timeout, ok := job["timeout-minutes"].(int)
	if !ok {
		t.Fatalf("release.yml: jobs.build.timeout-minutes missing or not an int (Task 1.2)")
	}
	// Allow a reasonable range; exact value is 45 per Task 1.2, but any sane
	// budget that accommodates macOS notarization is acceptable.
	if timeout < 30 || timeout > 90 {
		t.Errorf("release.yml: jobs.build.timeout-minutes should be ~45 (Task 1.2: macOS notarize budget), got %d", timeout)
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

	// Every codesign/notarize step must gate on the output.
	gateExpr := "steps.apple_secrets.outputs.available == 'true'"
	gatedNeedles := []string{
		"security create-keychain",   // Import Apple Developer ID cert
		"codesign --force",           // Sign macOS app bundle
		"xcrun notarytool submit",    // Notarize and staple
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
// Staging step marks the macOS `.app.zip` as `-UNSIGNED` when
// steps.apple_secrets.outputs.available != 'true'.
// Covers Task 4.2 (users downloading pre-release artifacts must know they
// must bypass Gatekeeper manually).
// ---------------------------------------------------------------------------

func TestStagingUnsignedSuffix(t *testing.T) {
	run := jobRunBodies(t, "build")

	// Look for SUFFIX assignment driven by SIGNED.
	if !strings.Contains(run, "SUFFIX=\"-UNSIGNED\"") {
		t.Errorf("release.yml build job: staging step must set `SUFFIX=\"-UNSIGNED\"` when signed=false (Task 4.2)")
	}
	// And the darwin-* branch must interpolate ${SUFFIX} into the .app.zip name.
	if !regexp.MustCompile(`darwin-[^"}]+\$\{SUFFIX\}\.app\.zip|\$\{PLATFORM\}\$\{SUFFIX\}\.app\.zip`).MatchString(run) {
		t.Errorf("release.yml build job: staging step must interpolate ${SUFFIX} into darwin .app.zip filename (Task 4.2)")
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

// ===========================================================================
// Story 7.4 gap-filler tests (appended 2026-04-17)
// ===========================================================================
//
// These tests close coverage gaps in the 7.4 homebrew job + formula template +
// render script that the 20 ATDD tests did not assert. All assertions are
// static/integration (YAML shape, raw-text grep, formula template content,
// shell-script subprocess with bad input). No new E2E; the single E2E in 7.4
// (brew install smoke test) is already covered by TestHomebrewJobSmokeTestsBrewInstall
// at workflow level and runs as part of the homebrew job itself.
//
// Gap analysis vs the 20 existing 7.4 tests:
//   - Render script: input validation (arg count, SHA hex shape) untested.
//   - Render script: sed delimiter choice (`|`) critical for URL correctness,
//     untested.
//   - Homebrew job: `timeout-minutes: 15` operational-safety budget untested.
//   - Homebrew job: checkout `ref: needs.release.outputs.tag` (tag pinning)
//     untested; a regression to default HEAD would render a stale formula.
//   - Homebrew job: `Reject UNSIGNED` inverse guard requires linux-amd64.tar.gz
//     (review #1 medium finding) untested.
//   - Homebrew job: `git ls-files --error-unmatch` defense-in-depth (review #1
//     high finding) untested.
//   - Homebrew job: `Resolve version string` rejects SemVer `+build` suffix,
//     untested at integration level.
//   - Homebrew job: `Fetch SHA256SUMS.txt` step uses awk `$2==f` exact match
//     (substring would collide on prefixed filenames), untested.
//   - Homebrew job: `Fetch SHA256SUMS.txt` step validates 64-hex SHA format.
//   - Formula template: Linux block wrapped in `on_intel` (Task 2.1 critical
//     note; without it Linuxbrew on arm64 fails opaquely).
//   - Formula template: single class-level `def install` branching on OS.mac?
//     (Task 2.1 critical Ruby trap).
//   - Formula template: `.app` rename to `unipdf-debugger.app` at install time
//     (Task 2.1 contract reconciling artifact internal name vs user brand).
//   - Formula template: `test do` block exercises `--version` (Task 2.1; aligns
//     formula self-test with AC #6 smoke test).

// ---------------------------------------------------------------------------
// Homebrew job timeout-minutes budget.
// Covers Task 3.3 (operational safety: a stuck `brew install` cannot eat the
// job indefinitely; 15 min is generous for a fresh tap + .zip extraction).
// ---------------------------------------------------------------------------

// TestHomebrewJobTimeoutMinutes asserts the homebrew job declares a
// timeout-minutes budget. Without it, a stuck gh call or brew install could
// pin the macOS runner for the GHA default 360-minute ceiling.
func TestHomebrewJobTimeoutMinutes(t *testing.T) {
	job := jobMap(t, "homebrew")
	timeout, ok := job["timeout-minutes"].(int)
	if !ok {
		t.Fatalf("release.yml: jobs.homebrew.timeout-minutes missing or not an int (Task 3.3 operational safety)")
	}
	if timeout < 5 || timeout > 30 {
		t.Errorf("release.yml: jobs.homebrew.timeout-minutes should be ~15 (Task 3.3), got %d", timeout)
	}
}

// ---------------------------------------------------------------------------
// Homebrew job checkout pins `ref: needs.release.outputs.tag`.
// Covers Task 3.3 (without the ref, checkout defaults to HEAD of the branch
// on which the workflow fired; subsequent merges to that branch between tag
// creation and job start would render a formula from the wrong tree).
// ---------------------------------------------------------------------------

// TestHomebrewJobCheckoutUsesReleaseTagRef asserts the actions/checkout step
// in the homebrew job pins `with.ref` to `${{ needs.release.outputs.tag }}`.
// A plain checkout (no `ref:`) would pull whatever HEAD the workflow fired
// on, which for `release: published` events is the release commit but for
// `workflow_dispatch` could be arbitrary.
func TestHomebrewJobCheckoutUsesReleaseTagRef(t *testing.T) {
	steps := jobSteps(t, "homebrew")
	var checkoutStep map[string]interface{}
	for _, s := range steps {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if u, _ := m["uses"].(string); strings.HasPrefix(u, "actions/checkout@") {
			checkoutStep = m
			break
		}
	}
	if checkoutStep == nil {
		t.Fatalf("release.yml: jobs.homebrew actions/checkout step missing (Task 3.3)")
	}
	with, ok := checkoutStep["with"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.homebrew checkout step missing `with:` block (must pin ref)")
	}
	ref, _ := with["ref"].(string)
	if !strings.Contains(ref, "needs.release.outputs.tag") {
		t.Errorf("release.yml: jobs.homebrew checkout `with.ref` must pin `${{ needs.release.outputs.tag }}` (Task 3.3; stale-HEAD guard), got %q", ref)
	}
}

// ---------------------------------------------------------------------------
// Homebrew job `Reject UNSIGNED` step's inverse guard requires
// linux-amd64.tar.gz (review #1 medium finding).
// ---------------------------------------------------------------------------

// TestHomebrewJobUnsignedGuardRequiresLinuxAsset asserts the
// `Reject UNSIGNED macOS assets` step's inverse (missing-asset) guard lists
// linux-amd64.tar.gz among the required assets. Without it, a silently failed
// Linux build with no tap token would exit 0 while publishing a formula
// (well, not publishing, since tap_token==false, but the UNSIGNED gate is
// NOT tap-token gated and runs unconditionally -- which is exactly why it
// must ALSO verify the Linux artifact is present).
func TestHomebrewJobUnsignedGuardRequiresLinuxAsset(t *testing.T) {
	block := homebrewJobBlock(t)

	// Locate the Reject UNSIGNED step body by its anchor line.
	anchor := "Reject UNSIGNED macOS assets"
	idx := strings.Index(block, anchor)
	if idx == -1 {
		t.Fatalf("release.yml jobs.homebrew: `%s` step missing (AC #11)", anchor)
	}
	body := block[idx:]
	// Bound the body to roughly one step (cut at the next `- name:` header or
	// the end of the block, whichever is first).
	if end := strings.Index(body[len(anchor):], "\n      - name:"); end != -1 {
		body = body[:len(anchor)+end]
	}

	for _, required := range []string{
		"darwin-arm64.app.zip",
		"darwin-amd64.app.zip",
		"linux-amd64.tar.gz",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("release.yml jobs.homebrew `Reject UNSIGNED` step must verify %q is present in release assets (review #1 medium; symmetry with macOS guards)", required)
		}
	}
}

// ---------------------------------------------------------------------------
// Homebrew job publish step asserts staged formula presence before
// commit-or-skip (review #1 high finding).
// ---------------------------------------------------------------------------

// TestHomebrewJobPublishGuardsStagedFile asserts the publish step invokes
// `git ls-files --error-unmatch Formula/unipdf-debugger.rb` BETWEEN `git add`
// and `git diff --cached --quiet`. Without this guard, a tap-side .gitignore
// excluding the Formula path would turn `git add` into a silent no-op, the
// staged-diff check would report no changes, and the job would exit 0 without
// publishing -- hidden failure mode.
func TestHomebrewJobPublishGuardsStagedFile(t *testing.T) {
	block := homebrewJobBlock(t)

	// Match shell-executable lines only (leading whitespace, NOT `#` comments).
	// The homebrew job body has explanatory `# git diff --cached --quiet` prose
	// that would false-match a bare substring search.
	addRe := regexp.MustCompile(`(?m)^\s+git add Formula/unipdf-debugger\.rb\s*$`)
	guardRe := regexp.MustCompile(`(?m)^\s+git ls-files --error-unmatch Formula/unipdf-debugger\.rb`)
	diffRe := regexp.MustCompile(`(?m)^\s+if git diff --cached --quiet`)

	addLoc := addRe.FindStringIndex(block)
	guardLoc := guardRe.FindStringIndex(block)
	diffLoc := diffRe.FindStringIndex(block)

	if addLoc == nil {
		t.Fatalf("release.yml jobs.homebrew publish: executable `git add Formula/unipdf-debugger.rb` line missing")
	}
	if guardLoc == nil {
		t.Errorf("release.yml jobs.homebrew publish: missing executable `git ls-files --error-unmatch Formula/unipdf-debugger.rb` defense-in-depth guard (review #1 high: catches tap-side .gitignore silent-skip)")
		return
	}
	if diffLoc == nil {
		t.Fatalf("release.yml jobs.homebrew publish: executable `if git diff --cached --quiet` guard missing (AC #12)")
	}
	if !(addLoc[0] < guardLoc[0] && guardLoc[0] < diffLoc[0]) {
		t.Errorf("release.yml jobs.homebrew publish: order must be `git add` -> `git ls-files --error-unmatch` -> `git diff --cached --quiet`, got offsets add=%d guard=%d diff=%d", addLoc[0], guardLoc[0], diffLoc[0])
	}
}

// ---------------------------------------------------------------------------
// Homebrew job `Resolve version string` rejects SemVer build-metadata
// (+build.N) suffixes. Covers Task 3.3 critical note: Homebrew formula
// `version "..."` chokes on `+` while release.yml overall permits it.
// ---------------------------------------------------------------------------

// TestHomebrewJobRejectsBuildMetadataTags asserts the `Resolve version string`
// step fails fast on tags containing `+` (SemVer build-metadata suffix).
// Without this guard, the render step would produce a formula with an
// invalid `version` field that only surfaces at `brew install` time.
func TestHomebrewJobRejectsBuildMetadataTags(t *testing.T) {
	block := homebrewJobBlock(t)

	resolveIdx := strings.Index(block, "Resolve version string")
	if resolveIdx == -1 {
		t.Fatalf("release.yml jobs.homebrew: `Resolve version string` step missing (Task 3.3)")
	}
	body := block[resolveIdx:]
	if end := strings.Index(body[len("Resolve version string"):], "\n      - name:"); end != -1 {
		body = body[:len("Resolve version string")+end]
	}

	// Expect an inline test for `+` in the version raw and an explicit `exit 1`.
	if !strings.Contains(body, `*+*`) && !strings.Contains(body, `*\+*`) {
		t.Errorf("release.yml jobs.homebrew `Resolve version string`: must reject SemVer `+build` suffix via `[[ \"$raw\" == *+* ]]` or equivalent (Task 3.3)")
	}
	if !strings.Contains(body, "exit 1") {
		t.Errorf("release.yml jobs.homebrew `Resolve version string`: must `exit 1` on build-metadata suffix (Task 3.3)")
	}
}

// ---------------------------------------------------------------------------
// Homebrew job `Fetch SHA256SUMS.txt` uses exact-match awk and validates the
// extracted hash is 64 hex chars. Covers Task 3.3 contract + review
// defense-in-depth: a truncated/malformed SHA would render a formula that
// fails `brew install` with an opaque mismatch for every user.
// ---------------------------------------------------------------------------

// TestHomebrewJobFetchExactAwkMatchAndHexValidation asserts the
// `Fetch SHA256SUMS.txt` step uses `$2==f` (exact field match, not substring)
// AND validates the extracted SHA matches `^[0-9a-fA-F]{64}$` before
// emitting it as a step output. Substring matching on asset filenames would
// collide when one filename is a suffix of another (e.g. `debugger.zip`
// matches `pdf-debugger.zip`).
func TestHomebrewJobFetchExactAwkMatchAndHexValidation(t *testing.T) {
	block := homebrewJobBlock(t)

	fetchIdx := strings.Index(block, "Fetch SHA256SUMS.txt")
	if fetchIdx == -1 {
		t.Fatalf("release.yml jobs.homebrew: `Fetch SHA256SUMS.txt` step missing (AC #3)")
	}
	body := block[fetchIdx:]
	if end := strings.Index(body[len("Fetch SHA256SUMS.txt"):], "\n      - name:"); end != -1 {
		body = body[:len("Fetch SHA256SUMS.txt")+end]
	}

	// Exact-match awk: `$2==f` (f is the awk variable bound to the asset name).
	if !regexp.MustCompile(`\$2\s*==\s*f`).MatchString(body) {
		t.Errorf("release.yml jobs.homebrew `Fetch SHA256SUMS.txt`: must use `awk '$2==f ...'` exact-match (Task 3.3; substring match would collide on overlapping filenames)")
	}
	// SHA hex-shape validation on the extracted value.
	if !regexp.MustCompile(`\[0-9a-fA-F\]\{64\}`).MatchString(body) {
		t.Errorf("release.yml jobs.homebrew `Fetch SHA256SUMS.txt`: must validate extracted SHA is 64 hex chars before emitting (defense-in-depth; rendered formula with bad SHA breaks every `brew install`)")
	}
}

// ---------------------------------------------------------------------------
// Formula template: Linux block wrapped in `on_intel`.
// Covers Task 2.1 critical note: no linux-arm64 artifact exists. Without the
// on_intel guard, Linuxbrew on arm64 would download the amd64 binary and
// fail opaquely at exec time.
// ---------------------------------------------------------------------------

// TestHomebrewFormulaTemplateLinuxOnIntelGuard asserts the template's
// `on_linux` block contains `on_intel` (no linux-arm64 fallback). Homebrew
// reports "formula not available for this arch" cleanly when a user on arm64
// Linuxbrew tries to install; without the guard the install succeeds and the
// user gets a cryptic exec error.
func TestHomebrewFormulaTemplateLinuxOnIntelGuard(t *testing.T) {
	root := projectRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "homebrew", "unipdf-debugger.rb.tmpl"))
	if err != nil {
		t.Fatalf("scripts/homebrew/unipdf-debugger.rb.tmpl not found: %v", err)
	}
	body := string(raw)

	onLinuxIdx := strings.Index(body, "on_linux do")
	if onLinuxIdx == -1 {
		t.Fatalf("template: `on_linux do` block missing")
	}
	// Bound the on_linux body at its matching `end` -- approximate with the
	// next `end` after the block's `on_intel`/`on_arm` nesting. A simple
	// substring check suffices: `on_intel` must appear AFTER `on_linux do`
	// and before the class's final `end`.
	linuxBody := body[onLinuxIdx:]
	endIdx := strings.Index(linuxBody, "\n  end\n")
	if endIdx != -1 {
		linuxBody = linuxBody[:endIdx]
	}
	if !strings.Contains(linuxBody, "on_intel") {
		t.Errorf("template: `on_linux` block must wrap url/sha in `on_intel` (Task 2.1; no linux-arm64 artifact; Linuxbrew on arm64 would otherwise download amd64 and fail at exec)")
	}
}

// ---------------------------------------------------------------------------
// Formula template: single class-level `def install` (not duplicated inside
// on_macos/on_linux blocks). Covers Task 2.1 Ruby trap.
// ---------------------------------------------------------------------------

// TestHomebrewFormulaTemplateSingleInstallDef asserts the template declares
// exactly one `def install` (at class scope), not two. Putting `def install`
// inside DSL blocks like `on_macos` / `on_linux` is a Ruby class-body trap:
// both `def` statements execute at parse time and the second overwrites the
// first regardless of platform at install time.
func TestHomebrewFormulaTemplateSingleInstallDef(t *testing.T) {
	root := projectRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "homebrew", "unipdf-debugger.rb.tmpl"))
	if err != nil {
		t.Fatalf("scripts/homebrew/unipdf-debugger.rb.tmpl not found: %v", err)
	}
	body := string(raw)

	// Count `def install` as an executable Ruby statement (line that starts with
	// optional whitespace followed by `def install`). Prose-comment mentions of
	// `def install` (e.g. the Task 2.1 warning comment) are excluded.
	defRe := regexp.MustCompile(`(?m)^\s*def install\b`)
	n := len(defRe.FindAllString(body, -1))
	if n != 1 {
		t.Errorf("template: expected exactly 1 executable `def install` (Task 2.1 Ruby trap: duplicate defs silently overwrite), got %d", n)
	}
	// The install method must branch on `OS.mac?` to handle platform differences
	// without resorting to the dual-def trap.
	if !strings.Contains(body, "OS.mac?") {
		t.Errorf("template: `def install` must branch on `OS.mac?` (Task 2.1; platform differences belong inside the single install method)")
	}
}

// ---------------------------------------------------------------------------
// Formula template: macOS install renames .app to `unipdf-debugger.app`.
// Covers Task 2.1 contract: reconciles artifact-internal name
// (`unidoc-pdf-debugger.app`) with user-facing brand (`unipdf-debugger`) so
// `open -a unipdf-debugger` resolves.
// ---------------------------------------------------------------------------

// TestHomebrewFormulaTemplateMacosAppRename asserts the install method
// renames `unidoc-pdf-debugger.app` to `unipdf-debugger.app` when installing
// on macOS. Without the rename, `open -a unipdf-debugger` would fail even
// though `unipdf-debugger` (the shim) is on PATH, creating a confusing
// discoverability gap between CLI shim and GUI-launcher name.
func TestHomebrewFormulaTemplateMacosAppRename(t *testing.T) {
	root := projectRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "homebrew", "unipdf-debugger.rb.tmpl"))
	if err != nil {
		t.Fatalf("scripts/homebrew/unipdf-debugger.rb.tmpl not found: %v", err)
	}
	body := string(raw)
	// Expect `prefix.install "unidoc-pdf-debugger.app" => "unipdf-debugger.app"`.
	if !strings.Contains(body, `"unidoc-pdf-debugger.app" => "unipdf-debugger.app"`) {
		t.Errorf("template: macOS install must rename `unidoc-pdf-debugger.app` to `unipdf-debugger.app` (Task 2.1; reconciles artifact-internal name with user brand so `open -a unipdf-debugger` resolves)")
	}
	// Linux install: rename the bare binary similarly.
	if !strings.Contains(body, `"unidoc-pdf-debugger" => "unipdf-debugger"`) {
		t.Errorf("template: Linux install must rename `unidoc-pdf-debugger` binary to `unipdf-debugger` (Task 2.1; consistent user-facing name across platforms)")
	}
}

// ---------------------------------------------------------------------------
// Formula template: `test do` block exercises `--version`.
// Covers Task 2.1: `brew test unipdf-debugger` must at minimum print the
// installed version, aligning the Homebrew self-test with AC #6 smoke test.
// ---------------------------------------------------------------------------

// TestHomebrewFormulaTemplateTestBlockRunsVersion asserts the template has
// a `test do` block that invokes `--version` against the installed binary.
// Homebrew uses this block for `brew test formula` and for auditing formulas
// before accepting them into homebrew-core (not a concern for third-party
// taps, but a good smoke-level contract).
func TestHomebrewFormulaTemplateTestBlockRunsVersion(t *testing.T) {
	root := projectRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "homebrew", "unipdf-debugger.rb.tmpl"))
	if err != nil {
		t.Fatalf("scripts/homebrew/unipdf-debugger.rb.tmpl not found: %v", err)
	}
	body := string(raw)

	testIdx := strings.Index(body, "test do")
	if testIdx == -1 {
		t.Fatalf("template: `test do` block missing (Task 2.1)")
	}
	testBody := body[testIdx:]
	if end := strings.Index(testBody, "\n  end\n"); end != -1 {
		testBody = testBody[:end]
	}
	if !strings.Contains(testBody, "--version") {
		t.Errorf("template: `test do` block must exercise `--version` (Task 2.1; aligns with AC #6 smoke test)")
	}
	if !strings.Contains(testBody, `bin/"unipdf-debugger"`) {
		t.Errorf("template: `test do` block must invoke the installed `bin/\"unipdf-debugger\"` shim (not the raw artifact name)")
	}
}

// ---------------------------------------------------------------------------
// render-formula.sh: sed uses `|` delimiter (URLs contain `/`).
// Covers Task 2.3 contract. A `/` delimiter would choke on the URL values,
// producing a corrupted formula and opaque sed errors at release time.
// ---------------------------------------------------------------------------

// TestRenderFormulaScriptUsesPipeSedDelimiter asserts the render script uses
// `|` as the sed substitution delimiter. Using `/` (the conventional default)
// would fail immediately because the URL values contain literal `/`.
func TestRenderFormulaScriptUsesPipeSedDelimiter(t *testing.T) {
	root := projectRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "homebrew", "render-formula.sh"))
	if err != nil {
		t.Fatalf("scripts/homebrew/render-formula.sh not found: %v", err)
	}
	body := string(raw)

	// Expect at least one `sed -e "s|@@...@@|...|g"` invocation.
	if !regexp.MustCompile(`s\|@@[A-Z0-9_]+@@\|`).MatchString(body) {
		t.Errorf("scripts/homebrew/render-formula.sh: sed must use `|` delimiter `s|@@TOKEN@@|VALUE|g` (URLs contain `/`; `/` delimiter would abort)")
	}
	// Negative: no `s/@@...@@/` forms (which would break on URL values).
	if regexp.MustCompile(`s/@@[A-Z0-9_]+@@/`).MatchString(body) {
		t.Errorf("scripts/homebrew/render-formula.sh: sed must NOT use `/` delimiter for @@...@@ substitutions (URL values contain `/` literals)")
	}
}

// ---------------------------------------------------------------------------
// render-formula.sh rejects wrong arg count.
// Covers Task 2.3 contract: script exits non-zero with a usage message when
// invoked with != 5 args. A silent-success on bad input would render a
// partially-filled formula with placeholder tokens that passes `ruby -c`
// (the tokens are inside quoted strings) but breaks `brew install`.
// ---------------------------------------------------------------------------

// TestRenderFormulaScriptRejectsWrongArgCount invokes the render script with
// 0, 1, and 4 args (all != 5) and asserts exit code is non-zero.
func TestRenderFormulaScriptRejectsWrongArgCount(t *testing.T) {
	root := projectRoot(t)
	script := filepath.Join(root, "scripts", "homebrew", "render-formula.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("render-formula.sh missing: %v", err)
	}

	cases := [][]string{
		{},
		{"0.1.0"},
		{"0.1.0", strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)},
	}
	for i, args := range cases {
		cmd := exec.Command("bash", append([]string{script}, args...)...)
		cmd.Dir = root
		err := cmd.Run()
		if err == nil {
			t.Errorf("render-formula.sh case %d (%d args): expected non-zero exit, got success", i, len(args))
		}
	}
}

// ---------------------------------------------------------------------------
// render-formula.sh rejects malformed SHA args.
// Covers Task 2.3 contract (script validates `^[0-9a-fA-F]{64}$` before
// proceeding). A bad SHA would render an invalid formula that passes
// `ruby -c` but fails every user's `brew install` with an opaque mismatch.
// ---------------------------------------------------------------------------

// TestRenderFormulaScriptRejectsMalformedSHA invokes the render script with
// malformed SHA arguments (short, non-hex, sed-metachar) and asserts non-zero
// exit. This is a critical guard: a 63-char SHA, a SHA with `&` (sed back-
// reference metachar), or a SHA with a `g` or other non-hex letter would
// render an invalid formula.
func TestRenderFormulaScriptRejectsMalformedSHA(t *testing.T) {
	root := projectRoot(t)
	script := filepath.Join(root, "scripts", "homebrew", "render-formula.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("render-formula.sh missing: %v", err)
	}

	validSHA := strings.Repeat("a", 64)
	badArgs := []string{
		strings.Repeat("a", 63),             // too short
		strings.Repeat("a", 65),             // too long
		"not-hex-" + strings.Repeat("a", 56), // non-hex chars
		strings.Repeat("z", 64),             // hex-length but z is not hex
	}

	for _, bad := range badArgs {
		// Put the malformed value in each of the three SHA slots in turn.
		for slot := 0; slot < 3; slot++ {
			args := []string{"0.1.0", validSHA, validSHA, validSHA, "v0.1.0"}
			args[1+slot] = bad
			cmd := exec.Command("bash", append([]string{script}, args...)...)
			cmd.Dir = root
			if err := cmd.Run(); err == nil {
				t.Errorf("render-formula.sh: accepted malformed SHA %q in slot %d (expected non-zero exit)", bad, slot)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// render-formula.sh emits a `# frozen_string_literal: true` pragma (Ruby best
// practice for Homebrew formulas; template ships with it, render must not
// strip it). Inexpensive static check on the rendered output.
// ---------------------------------------------------------------------------

// TestRenderFormulaScriptPreservesFrozenStringLiteralPragma asserts the
// render script output carries the template's `frozen_string_literal: true`
// magic comment through to the rendered formula. Some editors strip leading
// magic comments during save-transform pipelines; catching this at test time
// prevents a silent downgrade of the formula's Ruby idiom quality.
func TestRenderFormulaScriptPreservesFrozenStringLiteralPragma(t *testing.T) {
	root := projectRoot(t)
	script := filepath.Join(root, "scripts", "homebrew", "render-formula.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("render-formula.sh missing: %v", err)
	}

	sha := strings.Repeat("a", 64)
	cmd := exec.Command("bash", script, "0.1.0", sha, sha, sha, "v0.1.0")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("render-formula.sh failed: %v", err)
	}
	rendered := string(out)
	if !strings.Contains(rendered, "frozen_string_literal: true") {
		t.Errorf("render-formula.sh: rendered formula must preserve `# frozen_string_literal: true` magic comment from template")
	}
	if !strings.Contains(rendered, "typed: false") {
		t.Errorf("render-formula.sh: rendered formula must preserve `# typed: false` magic comment from template")
	}
}
