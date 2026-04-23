// Package release_pipeline_test provides acceptance tests for Story 7.2:
// Release Build Pipeline and Distribution.
//
// These tests verify that .github/workflows/release.yml is structured per the
// acceptance criteria: tag-triggered workflow, 4-platform matrix (macOS arm64,
// macOS amd64, Windows amd64, Linux amd64) producing 8 artifacts + SHA256SUMS,
// codesign/notarize/staple/verify flow on macOS, CLI built with -trimpath and
// version ldflag, two-stage job layout (build matrix -> release publish),
// softprops/action-gh-release@v2 publication with prerelease detection, and
// the Apple Developer ID keychain import + cleanup pattern.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 7.2 is
// implemented. No t.Skip() sentinels (per story 7-1 dev-story outcome: the
// repo ships TDD-red tests directly).
//
// Test Levels: Integration (Go) -- YAML parsing + filesystem checks. No
// browser or HTTP surface. Per Epic 7 test design, the single E2E AC (#10,
// 7.2-E2E-001) is a manual RC-tag smoke test that cannot be exercised from a
// local Go test; it is not automated here.
//
// Trace:
//   AC #1  -> TestReleaseWorkflowFileExistsAndParses, TestTriggerIsVersionTag,
//              TestWorkflowDispatchSupported (7.2-STATIC-001)
//   AC #2  -> TestMatrixContainsAllFourPlatforms, TestArtifactStagingNaming
//              (7.2-STATIC-002)
//   AC #3  -> TestMatrixContainsAllFourPlatforms, TestMatrixFailFastFalse
//              (7.2-STATIC-002)
//   AC #4  -> TestCodesignStepPresent, TestNotarizeAndStaplePresent,
//              TestMacOSVerificationCommands (7.2-STATIC-003)
//   AC #5  -> TestAppleKeychainImport, TestKeychainCleanupAlways,
//              TestAppleSecretsReferenced (7.2-STATIC-003)
//   AC #6  -> TestCLIBuildUsesTrimpathAndLdflags,
//              TestCLIBuildGOOSGOARCHFromMatrix (7.2-STATIC-002)
//   AC #7  -> TestReleasePublishStepUsesActionGhRelease,
//              TestPrereleaseDetectionLogic, TestGenerateReleaseNotesEnabled
//              (7.2-STATIC-006)
//   AC #8  -> TestSHA256SumsStepPresent, TestSHA256SumsExcludesSelf
//              (7.2-STATIC-005)
//   AC #9  -> TestTwoJobsBuildAndRelease, TestUploadArtifactAndDownloadArtifactV4
//              (7.2-STATIC-004)
//   Cross  -> TestSecretsNotLoggedOutsideSecretsExpr,
//              TestConcurrencyCancelInProgressFalse,
//              TestWorkflowPermissionsWrite, TestShellBashOnRunSteps
//
// Supporting files:
//   AC #4  -> TestEntitlementsPlistExists (Task 6.1)
//
// Run: cd tests/release-pipeline && go test -v -count=1 ./...
package release_pipeline_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// projectRoot walks upward from cwd to the unidoc-pdf-debugger module root.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if content, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// readReleaseWorkflow returns the raw release.yml text, failing if absent.
func readReleaseWorkflow(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	p := filepath.Join(root, ".github", "workflows", "release.yml")
	content, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf(".github/workflows/release.yml not found: %v", err)
	}
	return string(content)
}

// parseReleaseWorkflow parses release.yml into a generic map.
func parseReleaseWorkflow(t *testing.T) map[string]interface{} {
	t.Helper()
	raw := readReleaseWorkflow(t)
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("release.yml is not valid YAML: %v", err)
	}
	return parsed
}

// jobMap returns jobs.<jobName> as a map, failing if missing or malformed.
func jobMap(t *testing.T, jobName string) map[string]interface{} {
	t.Helper()
	parsed := parseReleaseWorkflow(t)
	jobsRaw, ok := parsed["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: missing top-level `jobs` map")
	}
	jobRaw, ok := jobsRaw[jobName]
	if !ok {
		t.Fatalf("release.yml: missing `%s` job", jobName)
	}
	j, ok := jobRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: job `%s` is not a map", jobName)
	}
	return j
}

// jobSteps returns the steps list for a given job.
func jobSteps(t *testing.T, jobName string) []interface{} {
	t.Helper()
	j := jobMap(t, jobName)
	steps, ok := j["steps"].([]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.%s.steps is not a list", jobName)
	}
	return steps
}

// jobRunBodies concatenates every `run:` body in a job into one string.
func jobRunBodies(t *testing.T, jobName string) string {
	t.Helper()
	var out strings.Builder
	for _, s := range jobSteps(t, jobName) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if run, ok := m["run"].(string); ok {
			out.WriteString(run)
			out.WriteString("\n")
		}
	}
	return out.String()
}

// jobUses returns every `uses:` value in a job.
func jobUses(t *testing.T, jobName string) []string {
	t.Helper()
	var uses []string
	for _, s := range jobSteps(t, jobName) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if u, ok := m["uses"].(string); ok {
			uses = append(uses, u)
		}
	}
	return uses
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-001 (P0): release.yml exists and parses
// Covers AC #1 (workflow defined in .github/workflows/release.yml)
// ---------------------------------------------------------------------------

func TestReleaseWorkflowFileExistsAndParses(t *testing.T) {
	_ = parseReleaseWorkflow(t)
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-001 (P0): on.push.tags contains 'v*'
// Covers AC #1 (tag push triggers release)
// ---------------------------------------------------------------------------

func TestTriggerIsVersionTag(t *testing.T) {
	parsed := parseReleaseWorkflow(t)
	onRaw, ok := parsed["on"]
	if !ok {
		t.Fatalf("release.yml: missing top-level `on:` triggers")
	}
	onMap, ok := onRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: `on:` is not a map")
	}
	push, ok := onMap["push"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: on.push block missing or wrong type (AC #1)")
	}
	tagsRaw, ok := push["tags"].([]interface{})
	if !ok {
		t.Fatalf("release.yml: on.push.tags is not a list (AC #1)")
	}
	found := false
	for _, v := range tagsRaw {
		if s, ok := v.(string); ok && s == "v*" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("release.yml: on.push.tags must include 'v*' (AC #1)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-001 (P0): workflow_dispatch with `tag` input supported
// Covers AC #1 + Task 1.5 (manual re-run against existing tag)
// ---------------------------------------------------------------------------

func TestWorkflowDispatchSupported(t *testing.T) {
	parsed := parseReleaseWorkflow(t)
	on, ok := parsed["on"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: `on` block missing")
	}
	wdRaw, ok := on["workflow_dispatch"]
	if !ok {
		t.Fatalf("release.yml: on.workflow_dispatch missing (Task 1.5)")
	}
	wd, ok := wdRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: workflow_dispatch is not a map")
	}
	inputs, ok := wd["inputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: workflow_dispatch.inputs missing (need `tag` input)")
	}
	if _, ok := inputs["tag"]; !ok {
		t.Errorf("release.yml: workflow_dispatch.inputs.tag missing (Task 1.5: maintainer re-run against existing tag)")
	}
}

// ---------------------------------------------------------------------------
// Workflow-level permissions: contents: write (required to create Release)
// Covers Task 1.1
// ---------------------------------------------------------------------------

func TestWorkflowPermissionsWrite(t *testing.T) {
	parsed := parseReleaseWorkflow(t)
	permsRaw, ok := parsed["permissions"]
	if !ok {
		t.Fatalf("release.yml: workflow-level `permissions` block missing (Task 1.1)")
	}
	perms, ok := permsRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: `permissions` is not a map")
	}
	contents, _ := perms["contents"].(string)
	if contents != "write" {
		t.Errorf("release.yml: permissions.contents must be \"write\" (Task 1.1 -- required to create Release), got %q", contents)
	}
}

// ---------------------------------------------------------------------------
// Concurrency: cancel-in-progress must be FALSE (opposite of ci.yml)
// Covers Task 1.1 reasoning (cancelling mid-release corrupts assets)
// ---------------------------------------------------------------------------

func TestConcurrencyCancelInProgressFalse(t *testing.T) {
	parsed := parseReleaseWorkflow(t)
	cRaw, ok := parsed["concurrency"]
	if !ok {
		t.Fatalf("release.yml: concurrency block missing (Task 1.1)")
	}
	c, ok := cRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: concurrency is not a map")
	}
	group, _ := c["group"].(string)
	if !strings.Contains(group, "github.ref") {
		t.Errorf("release.yml: concurrency.group should reference github.ref, got %q", group)
	}
	cip, ok := c["cancel-in-progress"].(bool)
	if !ok {
		t.Errorf("release.yml: concurrency.cancel-in-progress missing")
	} else if cip {
		t.Errorf("release.yml: concurrency.cancel-in-progress MUST be false (Task 1.1: cancelling mid-release leaves orphan assets)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-002 (P0): matrix contains exactly 4 include cells covering every
// required {os, arch, goos, platform} combination
// Covers AC #2, AC #3, AC #6
// ---------------------------------------------------------------------------

func TestMatrixContainsAllFourPlatforms(t *testing.T) {
	job := jobMap(t, "build")

	strat, ok := job["strategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.build.strategy missing")
	}
	matrix, ok := strat["matrix"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.build.strategy.matrix missing")
	}
	includeRaw, ok := matrix["include"].([]interface{})
	if !ok {
		t.Fatalf("release.yml: matrix.include is not a list (AC #3 requires include-only matrix)")
	}
	if len(includeRaw) != 4 {
		t.Errorf("release.yml: matrix.include must have exactly 4 cells (AC #3), got %d", len(includeRaw))
	}

	// Expected 4 tuples, keyed by os+arch for uniqueness.
	expected := map[string]struct {
		goos     string
		platform string
	}{
		"macos-latest|arm64":   {goos: "darwin", platform: "darwin-arm64"},
		"macos-13|amd64":       {goos: "darwin", platform: "darwin-amd64"},
		"windows-latest|amd64": {goos: "windows", platform: "windows-amd64"},
		"ubuntu-latest|amd64":  {goos: "linux", platform: "linux-amd64"},
	}
	seen := map[string]bool{}

	for i, c := range includeRaw {
		cell, ok := c.(map[string]interface{})
		if !ok {
			t.Errorf("release.yml: matrix.include[%d] is not a map", i)
			continue
		}
		osName, _ := cell["os"].(string)
		archName, _ := cell["arch"].(string)
		goos, _ := cell["goos"].(string)
		platform, _ := cell["platform"].(string)

		key := osName + "|" + archName
		exp, known := expected[key]
		if !known {
			t.Errorf("release.yml: matrix.include[%d] unexpected os+arch combo %q", i, key)
			continue
		}
		if goos != exp.goos {
			t.Errorf("release.yml: matrix.include[%s].goos must be %q, got %q (AC #2)", key, exp.goos, goos)
		}
		if platform != exp.platform {
			t.Errorf("release.yml: matrix.include[%s].platform must be %q, got %q (AC #2)", key, exp.platform, platform)
		}
		seen[key] = true
	}
	for k := range expected {
		if !seen[k] {
			t.Errorf("release.yml: matrix.include missing required cell %q (AC #3)", k)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-002 (P0): strategy.fail-fast is false
// Covers AC #3 (one platform failure must not mask others)
// ---------------------------------------------------------------------------

func TestMatrixFailFastFalse(t *testing.T) {
	job := jobMap(t, "build")
	strat, ok := job["strategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.build.strategy missing")
	}
	ff, ok := strat["fail-fast"].(bool)
	if !ok {
		t.Errorf("release.yml: jobs.build.strategy.fail-fast not set (AC #3)")
	} else if ff {
		t.Errorf("release.yml: jobs.build.strategy.fail-fast must be false (AC #3), got true")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-003 (P0): codesign step present with required flags
// Covers AC #4 (codesign with --force --deep --options runtime --entitlements
// build/darwin/entitlements.plist --timestamp --sign)
// ---------------------------------------------------------------------------

func TestCodesignStepPresent(t *testing.T) {
	run := jobRunBodies(t, "build")

	for _, needle := range []string{
		"codesign",
		"--force",
		"--deep",
		"--options runtime",
		"--entitlements build/darwin/entitlements.plist",
		"--timestamp",
		"--sign",
	} {
		if !strings.Contains(run, needle) {
			t.Errorf("release.yml build job: codesign flag/token %q missing (AC #4)", needle)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-003 (P0): notarytool submit + log + stapler staple present
// Covers AC #4 (submit to Apple Notary, fetch log, staple)
// ---------------------------------------------------------------------------

func TestNotarizeAndStaplePresent(t *testing.T) {
	run := jobRunBodies(t, "build")

	for _, needle := range []string{
		"xcrun notarytool submit",
		"--wait",
		"--output-format json",
		"xcrun notarytool log",
		"xcrun stapler staple",
	} {
		if !strings.Contains(run, needle) {
			t.Errorf("release.yml build job: notarize/staple token %q missing (AC #4)", needle)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-003 (P0): both codesign --verify and spctl --assess run post-staple
// Covers AC #4 (all five steps must exit 0: sign, notarize, staple, codesign
// --verify, spctl --assess)
// ---------------------------------------------------------------------------

func TestMacOSVerificationCommands(t *testing.T) {
	run := jobRunBodies(t, "build")

	if !strings.Contains(run, "codesign --verify --deep --strict") {
		t.Errorf("release.yml build job: `codesign --verify --deep --strict` missing (AC #4)")
	}
	if !strings.Contains(run, "spctl --assess --type execute") {
		t.Errorf("release.yml build job: `spctl --assess --type execute` missing (AC #4 -- both verify AND assess required)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-003 (P0): macOS Developer ID keychain is imported at job start
// Covers AC #5 (security create-keychain + import + set-key-partition-list)
// ---------------------------------------------------------------------------

func TestAppleKeychainImport(t *testing.T) {
	run := jobRunBodies(t, "build")

	for _, needle := range []string{
		"security create-keychain",
		"security import",
		"security set-key-partition-list",
		"security unlock-keychain",
	} {
		if !strings.Contains(run, needle) {
			t.Errorf("release.yml build job: keychain import command %q missing (AC #5)", needle)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-003 (P0): keychain cleanup runs under if: always()
// Covers AC #5 (secrets never linger on runner)
// ---------------------------------------------------------------------------

func TestKeychainCleanupAlways(t *testing.T) {
	steps := jobSteps(t, "build")

	var found bool
	for _, s := range steps {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		run, _ := m["run"].(string)
		ifClause, _ := m["if"].(string)
		if strings.Contains(run, "security delete-keychain") {
			found = true
			if !strings.Contains(ifClause, "always()") {
				t.Errorf("release.yml: keychain cleanup step must use `if: always()` (AC #5), got %q", ifClause)
			}
			break
		}
	}
	if !found {
		t.Errorf("release.yml: no step invokes `security delete-keychain` (AC #5 cleanup requirement)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-003 (P0): all required Apple secrets are referenced via
// ${{ secrets.* }} expressions in the workflow
// Covers AC #5 (explicit secret list)
// ---------------------------------------------------------------------------

func TestAppleSecretsReferenced(t *testing.T) {
	raw := readReleaseWorkflow(t)

	required := []string{
		"APPLE_DEVELOPER_ID_CERT_P12_BASE64",
		"APPLE_DEVELOPER_ID_CERT_PASSWORD",
		"APPLE_DEVELOPER_ID",
		"APPLE_ID",
		"APPLE_ID_APP_PASSWORD",
		"APPLE_TEAM_ID",
	}
	for _, name := range required {
		expr := "secrets." + name
		if !strings.Contains(raw, expr) {
			t.Errorf("release.yml: must reference secret via `${{ secrets.%s }}` (AC #5)", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Secrets must never appear as literal environment values outside the
// ${{ secrets.* }} expression form. Verifies no accidental plaintext logging.
// Covers Task 8.2 final bullet + security hardening
// ---------------------------------------------------------------------------

func TestSecretsNotLoggedOutsideSecretsExpr(t *testing.T) {
	raw := readReleaseWorkflow(t)

	// For each sensitive secret, any occurrence of NAME: must be
	// followed by a ${{ secrets.NAME }} expression (possibly with
	// whitespace). We disallow bare `NAME: somevalue` mappings where
	// the value is not a secrets expression.
	sensitive := []string{
		"APPLE_ID_APP_PASSWORD",
		"APPLE_DEVELOPER_ID_CERT_PASSWORD",
	}
	for _, name := range sensitive {
		// Match only YAML-level `NAME:` mappings at line start (optionally
		// indented). Excludes shell parameter expansions like
		// `${NAME:-default}` which happen to share the `NAME:` substring but
		// are not key-value mappings.
		re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `:\s*(.+)$`)
		for _, m := range re.FindAllStringSubmatch(raw, -1) {
			val := strings.TrimSpace(m[1])
			if !strings.HasPrefix(val, "${{") {
				t.Errorf("release.yml: secret %q mapped to non-secrets-expression value %q (plaintext leak risk)", name, val)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-002 (P0): CLI build uses -trimpath + -ldflags with version
// Covers AC #6 (go build -trimpath -ldflags="-s -w -X main.version=<version>")
// ---------------------------------------------------------------------------

func TestCLIBuildUsesTrimpathAndLdflags(t *testing.T) {
	run := jobRunBodies(t, "build")

	if !strings.Contains(run, "go build") {
		t.Fatalf("release.yml build job: `go build` for CLI not found (AC #6)")
	}
	if !strings.Contains(run, "-trimpath") {
		t.Errorf("release.yml build job: CLI build missing `-trimpath` (AC #6)")
	}
	// ldflags must strip symbols and embed version.
	if !regexp.MustCompile(`-ldflags="?-s -w -X main\.version=`).MatchString(run) {
		t.Errorf("release.yml build job: CLI build must pass `-ldflags=\"-s -w -X main.version=<version>\"` (AC #6)")
	}
	if !strings.Contains(run, "./cmd/cli") {
		t.Errorf("release.yml build job: CLI build must target `./cmd/cli` (AC #6)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-002 (P0): CLI build reads GOOS/GOARCH from matrix keys
// Covers AC #6 (GOOS/GOARCH per matrix cell)
// ---------------------------------------------------------------------------

func TestCLIBuildGOOSGOARCHFromMatrix(t *testing.T) {
	raw := readReleaseWorkflow(t)

	// Either `GOOS: ${{ matrix.goos }}` env binding, or explicit inline
	// `GOOS=${{ matrix.goos }}` -- require the matrix binding.
	if !regexp.MustCompile(`GOOS:\s*\$\{\{\s*matrix\.goos\s*\}\}`).MatchString(raw) {
		t.Errorf("release.yml: CLI build step must set `GOOS: ${{ matrix.goos }}` (AC #6)")
	}
	if !regexp.MustCompile(`GOARCH:\s*\$\{\{\s*matrix\.arch\s*\}\}`).MatchString(raw) {
		t.Errorf("release.yml: CLI build step must set `GOARCH: ${{ matrix.arch }}` (AC #6)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-004 (P1): two jobs -- build and release, with release needing build
// Covers AC #9 (parallel build matrix then single release job)
// ---------------------------------------------------------------------------

func TestTwoJobsBuildAndRelease(t *testing.T) {
	parsed := parseReleaseWorkflow(t)
	jobs, ok := parsed["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs block missing")
	}
	if _, ok := jobs["build"]; !ok {
		t.Errorf("release.yml: jobs.build missing (AC #9)")
	}
	rel, ok := jobs["release"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.release missing (AC #9)")
	}
	// needs: build  -- may be string or list.
	needs, ok := rel["needs"]
	if !ok {
		t.Fatalf("release.yml: jobs.release.needs missing (AC #9 requires needs: build)")
	}
	switch n := needs.(type) {
	case string:
		if n != "build" {
			t.Errorf("release.yml: jobs.release.needs must be \"build\", got %q", n)
		}
	case []interface{}:
		found := false
		for _, v := range n {
			if s, ok := v.(string); ok && s == "build" {
				found = true
			}
		}
		if !found {
			t.Errorf("release.yml: jobs.release.needs must include \"build\"")
		}
	default:
		t.Errorf("release.yml: jobs.release.needs has unexpected type %T", needs)
	}
	// release job must run on ubuntu-latest (AC #8 requires shasum on ubuntu)
	runsOn, _ := rel["runs-on"].(string)
	if runsOn != "ubuntu-latest" {
		t.Errorf("release.yml: jobs.release.runs-on must be \"ubuntu-latest\" (AC #8), got %q", runsOn)
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-004 (P1): upload-artifact@v4 in build, download-artifact@v4 in release
// Covers AC #9 (artifact staging via v4 actions)
// ---------------------------------------------------------------------------

func TestUploadArtifactAndDownloadArtifactV4(t *testing.T) {
	buildUses := jobUses(t, "build")
	releaseUses := jobUses(t, "release")

	var hasUpload, hasDownload bool
	for _, u := range buildUses {
		if strings.HasPrefix(u, "actions/upload-artifact@v4") {
			hasUpload = true
		}
	}
	for _, u := range releaseUses {
		if strings.HasPrefix(u, "actions/download-artifact@v4") {
			hasDownload = true
		}
	}
	if !hasUpload {
		t.Errorf("release.yml: jobs.build must use actions/upload-artifact@v4 (AC #9)")
	}
	if !hasDownload {
		t.Errorf("release.yml: jobs.release must use actions/download-artifact@v4 (AC #9)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-005 (P1): SHA256SUMS computed via shasum -a 256 in release job
// Covers AC #8 (single sha256 computation in the release job)
// ---------------------------------------------------------------------------

func TestSHA256SumsStepPresent(t *testing.T) {
	run := jobRunBodies(t, "release")

	if !strings.Contains(run, "shasum -a 256") {
		t.Errorf("release.yml release job: `shasum -a 256` missing (AC #8)")
	}
	if !strings.Contains(run, "SHA256SUMS.txt") {
		t.Errorf("release.yml release job: output file `SHA256SUMS.txt` missing (AC #8)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-005 (P1): SHA256SUMS generation excludes itself and is NUL-safe
// Covers AC #8 + Task 5.1 (no `$(ls | sort)` anti-pattern)
// ---------------------------------------------------------------------------

func TestSHA256SumsExcludesSelf(t *testing.T) {
	run := jobRunBodies(t, "release")

	if !strings.Contains(run, "! -name SHA256SUMS.txt") {
		t.Errorf("release.yml release job: SHA256SUMS step must exclude itself from input via `! -name SHA256SUMS.txt` (Task 5.1)")
	}
	// Anti-pattern: ls | sort word-splits and is not NUL-safe.
	if regexp.MustCompile(`\bls\b[^|]*\|\s*sort`).MatchString(run) {
		t.Errorf("release.yml release job: `ls | sort` anti-pattern present (Task 5.1: use find -print0 | sort -z)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-006 (P2): release publication step uses softprops/action-gh-release@v2
// Covers AC #7
// ---------------------------------------------------------------------------

func TestReleasePublishStepUsesActionGhRelease(t *testing.T) {
	uses := jobUses(t, "release")
	var found bool
	for _, u := range uses {
		if strings.HasPrefix(u, "softprops/action-gh-release@v2") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("release.yml: jobs.release must use softprops/action-gh-release@v2 (AC #7)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-006 (P2): prerelease flag set for v*-rc* / -alpha* / -beta*
// Covers AC #7 (pre-release detection logic)
// ---------------------------------------------------------------------------

func TestPrereleaseDetectionLogic(t *testing.T) {
	run := jobRunBodies(t, "release")

	// The task body prescribes the regex `=~ -(rc|alpha|beta)`.
	re := regexp.MustCompile(`-\(rc\|alpha\|beta\)`)
	if !re.MatchString(run) {
		t.Errorf("release.yml release job: prerelease detection regex `-(rc|alpha|beta)` missing (AC #7)")
	}
	if !strings.Contains(run, "prerelease=true") {
		t.Errorf("release.yml release job: must emit `prerelease=true` for matching tags (AC #7)")
	}
	if !strings.Contains(run, "prerelease=false") {
		t.Errorf("release.yml release job: must emit `prerelease=false` for non-matching tags (AC #7)")
	}
}

// ---------------------------------------------------------------------------
// 7.2-STATIC-006 (P2): action-gh-release `generate_release_notes: true`
// Covers AC #7 (Release description populated from commits since previous tag)
// ---------------------------------------------------------------------------

func TestGenerateReleaseNotesEnabled(t *testing.T) {
	steps := jobSteps(t, "release")

	var publishStep map[string]interface{}
	for _, s := range steps {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if u, _ := m["uses"].(string); strings.HasPrefix(u, "softprops/action-gh-release@") {
			publishStep = m
			break
		}
	}
	if publishStep == nil {
		t.Fatalf("release.yml: softprops/action-gh-release step not found")
	}
	with, ok := publishStep["with"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: softprops/action-gh-release step missing `with:` block")
	}

	grn, ok := with["generate_release_notes"].(bool)
	if !ok {
		t.Errorf("release.yml: softprops/action-gh-release `with.generate_release_notes` must be set (AC #7)")
	} else if !grn {
		t.Errorf("release.yml: softprops/action-gh-release `with.generate_release_notes` must be true (AC #7)")
	}

	// prerelease must be parameterized (AC #7: toggled by tag pattern)
	prerelease, _ := with["prerelease"].(string)
	if !strings.Contains(prerelease, "${{") {
		t.Errorf("release.yml: softprops/action-gh-release `with.prerelease` must be a step-output expression, got %q (AC #7)", prerelease)
	}

	// fail_on_unmatched_files: true (Task 5.2)
	fof, ok := with["fail_on_unmatched_files"].(bool)
	if !ok || !fof {
		t.Errorf("release.yml: softprops/action-gh-release `with.fail_on_unmatched_files` must be true (Task 5.2)")
	}
}

// ---------------------------------------------------------------------------
// Every run: step must set shell: bash (Windows defaults to PowerShell)
// Covers Dev Notes lesson #3 from story 7-1
// ---------------------------------------------------------------------------

func TestShellBashOnRunSteps(t *testing.T) {
	for _, jobName := range []string{"build", "release"} {
		for i, s := range jobSteps(t, jobName) {
			m, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			run, hasRun := m["run"].(string)
			if !hasRun || run == "" {
				continue
			}
			shell, _ := m["shell"].(string)
			if shell != "bash" {
				name, _ := m["name"].(string)
				t.Errorf("release.yml: jobs.%s.steps[%d] (name=%q) has `run:` but missing `shell: bash` (Windows compat)", jobName, i, name)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Artifact staging produces the 8 named artifacts plus SHA256SUMS
// Covers AC #2, Task 4.3 naming contract
// ---------------------------------------------------------------------------

func TestArtifactStagingNaming(t *testing.T) {
	run := jobRunBodies(t, "build")

	// Naming tokens that MUST appear literally in the staging step.
	// <ver> is shell-interpolated at runtime but the PLATFORM placeholder
	// and the GUI/CLI prefixes are fixed.
	tokens := []string{
		"unidoc-pdf-debugger-",       // GUI prefix
		"pdfdebug-",                  // CLI prefix
		"darwin-arm64",               // platform tokens appear via matrix.platform
		"darwin-amd64",
		"windows-amd64",
		"linux-amd64",
	}
	for _, tok := range tokens {
		// Either the literal token, or matrix.platform expansion covers darwin-*/windows-*/linux-*
		if !strings.Contains(run, tok) && !strings.Contains(run, "matrix.platform") && !strings.Contains(run, "${PLATFORM}") {
			t.Errorf("release.yml build job: staging step missing naming token %q (AC #2, Task 4.3)", tok)
		}
	}

	// macOS GUI must be packaged as .app.zip; Windows as .exe; Linux as .tar.gz
	for _, needle := range []string{".app.zip", ".exe", ".tar.gz"} {
		if !strings.Contains(run, needle) {
			t.Errorf("release.yml build job: artifact extension %q missing (AC #2, Task 4.3)", needle)
		}
	}

	// macOS must use ditto -c -k --keepParent (not plain `zip`) per Task 2.3 / Anti-Patterns
	if !strings.Contains(run, "ditto -c -k --keepParent") {
		t.Errorf("release.yml build job: macOS artifact must be zipped via `ditto -c -k --keepParent` (Task 2.3)")
	}
}

// ---------------------------------------------------------------------------
// Supporting file: build/darwin/entitlements.plist exists with Wails-v3
// baseline entries
// Covers AC #4 + Task 6.1
// ---------------------------------------------------------------------------

func TestEntitlementsPlistExists(t *testing.T) {
	root := projectRoot(t)
	p := filepath.Join(root, "build", "darwin", "entitlements.plist")
	content, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("build/darwin/entitlements.plist not found: %v (Task 6.1 -- file MUST exist before release.yml fires)", err)
	}
	raw := string(content)

	// XML plist header
	if !strings.Contains(raw, "<?xml") || !strings.Contains(raw, "<plist") {
		t.Errorf("build/darwin/entitlements.plist is not a valid plist document")
	}

	// Wails v3 baseline entitlements (Task 6.1)
	for _, key := range []string{
		"com.apple.security.cs.allow-unsigned-executable-memory",
		"com.apple.security.cs.disable-library-validation",
	} {
		if !strings.Contains(raw, key) {
			t.Errorf("build/darwin/entitlements.plist missing required key %q (Task 6.1)", key)
		}
	}
}

// ===========================================================================
// Story 7.4: Homebrew Tap for macOS/Linux Distribution -- appended tests
// ===========================================================================
//
// These 19 test functions extend this suite for Story 7.4 (appended 2026-04-17).
// They are TDD RED PHASE: they MUST fail until Story 7.4 is implemented. No
// t.Skip() sentinels except for the two Ruby-gated tests (per story 7-4 Task
// 4.1 directive).
//
// Trace (AC -> test):
//   AC #2  -> TestHomebrewFormulaTemplateExists,
//              TestHomebrewFormulaTemplateIsSyntacticallyValid
//   AC #3  -> TestHomebrewJobFetchesSHA256SUMS,
//              TestHomebrewJobRendersFormula,
//              TestHomebrewJobPushesToTap,
//              TestHomebrewJobCommitAuthor
//   AC #4  -> TestHomebrewJobNeedsBuildAndRelease,
//              TestHomebrewJobHasAssetReachabilityCheck
//   AC #5  -> TestHomebrewJobSkipsOnPrerelease,
//              TestReleaseJobExposesPrereleaseOutput
//   AC #6  -> TestHomebrewJobRunsOnMacos,
//              TestHomebrewJobSmokeTestsBrewInstall
//   AC #8  -> TestHomebrewJobTokenGateExists,
//              TestHomebrewJobSecretsNotLoggedOutsideSecretsExpr
//   AC #9  -> TestHomebrewFormulaTemplateExists,
//              TestHomebrewFormulaTemplateIsSyntacticallyValid,
//              TestRenderFormulaScriptExistsAndExecutable,
//              TestRenderFormulaScriptProducesValidRuby
//   AC #11 -> TestHomebrewJobRejectsUnsignedMacosAssets
//   AC #12 -> TestHomebrewJobIdempotentPush
//   AC #1 (job existence) -> TestHomebrewJobExists
//
// Test design scenarios (epic-7-test-design.md):
//   7.4-STATIC-001 -> TestHomebrewFormulaTemplateExists,
//                     TestHomebrewFormulaTemplateIsSyntacticallyValid
//   7.4-STATIC-002 -> TestHomebrewFormulaTemplateExists (class name assertion)
//   7.4-INTG-001   -> TestHomebrewJobExists, TestHomebrewJobNeedsBuildAndRelease,
//                     TestHomebrewJobFetchesSHA256SUMS,
//                     TestHomebrewJobRendersFormula,
//                     TestHomebrewJobPushesToTap,
//                     TestHomebrewJobCommitAuthor,
//                     TestHomebrewJobHasAssetReachabilityCheck
//   7.4-INTG-002   -> TestHomebrewJobIdempotentPush
//   7.4-E2E-001    -> TestHomebrewJobSmokeTestsBrewInstall (the job's own
//                     post-push brew-install step; no separate E2E suite)
//
// Helper: homebrewJobBlock returns the raw YAML substring of the `homebrew:`
// job block, bounded by the next top-level `<id>:` job header at the same
// indentation (two spaces). Failing the test if the job block is missing.

// homebrewJobBlock extracts the raw YAML slice that begins at `  homebrew:`
// and runs until the next job header at the same indentation (or EOF). This
// gives the 7-4 assertions a targeted substring to grep without regressing
// accidentally into adjacent jobs.
func homebrewJobBlock(t *testing.T) string {
	t.Helper()
	raw := readReleaseWorkflow(t)
	// Anchor at two-space indent to match the existing `build:` / `release:`
	// jobs in release.yml (top-level `jobs:` map, two-space child indent).
	start := strings.Index(raw, "\n  homebrew:\n")
	if start == -1 {
		t.Fatalf("release.yml: `homebrew:` job block not found at jobs.<> indentation (AC #1, #3, #4)")
	}
	body := raw[start+1:]
	// Find next top-level job header at same indent; the regex matches two-space
	// indent + identifier + colon + newline.
	re := regexp.MustCompile(`(?m)^  [A-Za-z][A-Za-z0-9_-]*:\n`)
	if loc := re.FindStringIndex(body[len("  homebrew:\n"):]); loc != nil {
		return body[:len("  homebrew:\n")+loc[0]]
	}
	return body
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): release.yml declares a `homebrew` job
// Covers AC #1 (job exists so Homebrew publication path is present)
// ---------------------------------------------------------------------------

// TestHomebrewJobExists asserts that release.yml defines a `homebrew` job
// under the top-level `jobs:` map. Red-phase: job does not yet exist.
func TestHomebrewJobExists(t *testing.T) {
	parsed := parseReleaseWorkflow(t)
	jobs, ok := parsed["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs block missing (AC #1)")
	}
	if _, ok := jobs["homebrew"]; !ok {
		t.Errorf("release.yml: jobs.homebrew missing (AC #1, #3 -- Homebrew publication job)")
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job needs: [build, release]
// Covers AC #4 (ordering: homebrew runs AFTER build matrix AND release publish)
// ---------------------------------------------------------------------------

// TestHomebrewJobNeedsBuildAndRelease asserts the `homebrew` job's `needs:`
// contains BOTH `build` AND `release`, enforcing AC #4's strict ordering
// (build matrix completes, then release publishes, THEN homebrew runs).
func TestHomebrewJobNeedsBuildAndRelease(t *testing.T) {
	job := jobMap(t, "homebrew")
	needs, ok := job["needs"]
	if !ok {
		t.Fatalf("release.yml: jobs.homebrew.needs missing (AC #4 requires needs: [build, release])")
	}
	required := map[string]bool{"build": false, "release": false}
	switch n := needs.(type) {
	case string:
		if _, ok := required[n]; ok {
			required[n] = true
		}
	case []interface{}:
		for _, v := range n {
			if s, ok := v.(string); ok {
				if _, expected := required[s]; expected {
					required[s] = true
				}
			}
		}
	default:
		t.Fatalf("release.yml: jobs.homebrew.needs has unexpected type %T (AC #4)", needs)
	}
	for name, found := range required {
		if !found {
			t.Errorf("release.yml: jobs.homebrew.needs must include %q (AC #4)", name)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job is skipped on pre-release tags
// Covers AC #5 (pre-release suffix -> formula update SKIPPED)
// ---------------------------------------------------------------------------

// TestHomebrewJobSkipsOnPrerelease asserts the `homebrew` job's `if:` gate is
// exactly `needs.release.outputs.prerelease == 'false'`. This short-circuits
// formula publication on rc/alpha/beta tags (AC #5).
func TestHomebrewJobSkipsOnPrerelease(t *testing.T) {
	job := jobMap(t, "homebrew")
	ifExpr, _ := job["if"].(string)
	expected := "needs.release.outputs.prerelease == 'false'"
	if !strings.Contains(ifExpr, expected) {
		t.Errorf("release.yml: jobs.homebrew.if must gate on %q (AC #5), got %q", expected, ifExpr)
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): release job exposes `prerelease` + `tag` as job outputs
// Covers AC #5 (homebrew `if:` cannot evaluate without release-level output
// mapping; Task 3.2 adds the `outputs:` section)
// ---------------------------------------------------------------------------

// TestReleaseJobExposesPrereleaseOutput asserts jobs.release.outputs declares
// keys `prerelease` and `tag`. Without these, the `homebrew` job's
// `needs.release.outputs.prerelease` reference evaluates to empty string and
// the `== 'false'` check silently evaluates false, skipping the job on every
// run (including real releases).
func TestReleaseJobExposesPrereleaseOutput(t *testing.T) {
	job := jobMap(t, "release")
	outputsRaw, ok := job["outputs"]
	if !ok {
		t.Fatalf("release.yml: jobs.release.outputs missing (Task 3.2; AC #5 gate)")
	}
	outputs, ok := outputsRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.release.outputs is not a map")
	}
	for _, key := range []string{"prerelease", "tag"} {
		if _, ok := outputs[key]; !ok {
			t.Errorf("release.yml: jobs.release.outputs.%s missing (Task 3.2; AC #5 gate)", key)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.4-E2E-001 (P0): homebrew job runs on macos-latest (required for the
// brew-install smoke test in AC #6)
// ---------------------------------------------------------------------------

// TestHomebrewJobRunsOnMacos asserts `jobs.homebrew.runs-on` is
// `macos-latest`. AC #6's smoke test (`brew install --build-from-source`)
// requires a macOS runner; running the job on ubuntu-latest would fail the
// final step.
func TestHomebrewJobRunsOnMacos(t *testing.T) {
	job := jobMap(t, "homebrew")
	runsOn, _ := job["runs-on"].(string)
	if runsOn != "macos-latest" {
		t.Errorf("release.yml: jobs.homebrew.runs-on must be \"macos-latest\" (AC #6 smoke test requires macOS), got %q", runsOn)
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job verifies release assets are reachable via
// curl -fsI before rendering the formula (AC #4 E7-R-003 mitigation)
// ---------------------------------------------------------------------------

// TestHomebrewJobHasAssetReachabilityCheck asserts the homebrew job block
// contains a curl-based reachability probe against the four expected asset
// filenames (three GUI assets + SHA256SUMS.txt) BEFORE formula computation.
// This is AC #4's explicit guard against E7-R-003 "Homebrew formula
// auto-update race" (CDN staging delay / missing asset).
func TestHomebrewJobHasAssetReachabilityCheck(t *testing.T) {
	block := homebrewJobBlock(t)

	if !strings.Contains(block, "curl -fsI") {
		t.Errorf("release.yml jobs.homebrew: must invoke `curl -fsI` for asset reachability (AC #4 / E7-R-003)")
	}
	for _, asset := range []string{
		"darwin-arm64.app.zip",
		"darwin-amd64.app.zip",
		"linux-amd64.tar.gz",
		"SHA256SUMS.txt",
	} {
		if !strings.Contains(block, asset) {
			t.Errorf("release.yml jobs.homebrew: asset-reachability step missing expected asset token %q (AC #4)", asset)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job fetches SHA256SUMS.txt from the release
// Covers AC #3 (job must download SHA256SUMS.txt via gh release download)
// ---------------------------------------------------------------------------

// TestHomebrewJobFetchesSHA256SUMS asserts the homebrew job block includes
// a `gh release download` invocation targeting SHA256SUMS.txt. The rendered
// formula's sha256 fields are extracted from this file.
func TestHomebrewJobFetchesSHA256SUMS(t *testing.T) {
	block := homebrewJobBlock(t)

	if !strings.Contains(block, "gh release download") {
		t.Errorf("release.yml jobs.homebrew: must call `gh release download` to fetch SHA256SUMS.txt (AC #3)")
	}
	if !strings.Contains(block, "SHA256SUMS.txt") {
		t.Errorf("release.yml jobs.homebrew: must reference `SHA256SUMS.txt` filename (AC #3)")
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job renders the formula via render-formula.sh
// and syntax-checks the output via `ruby -c`
// Covers AC #3 + AC #9 (template renders to valid-shape Ruby)
// ---------------------------------------------------------------------------

// TestHomebrewJobRendersFormula asserts the homebrew job invokes
// `scripts/homebrew/render-formula.sh` AND runs `ruby -c` on the rendered
// output before publishing. Without `ruby -c` the job can push syntactically
// broken Ruby to the tap repo and the failure only surfaces at `brew install`
// time, which is the wrong layer.
func TestHomebrewJobRendersFormula(t *testing.T) {
	block := homebrewJobBlock(t)

	if !strings.Contains(block, "scripts/homebrew/render-formula.sh") {
		t.Errorf("release.yml jobs.homebrew: must invoke `scripts/homebrew/render-formula.sh` (AC #3, #9)")
	}
	if !strings.Contains(block, "ruby -c") {
		t.Errorf("release.yml jobs.homebrew: must syntax-check rendered formula via `ruby -c` before publish (AC #3, #9)")
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job pushes the rendered formula to the tap
// Covers AC #3 + AC #8 (push target + token-based auth via gh)
// ---------------------------------------------------------------------------

// TestHomebrewJobPushesToTap asserts the homebrew job clones/pushes to
// `unidoc/homebrew-tap`, writes the rendered file to
// `Formula/unipdf-debugger.rb`, and authenticates via `gh auth login
// --with-token` (stdin-fed token, never env-exposed) per AC #8's secret
// handling contract.
func TestHomebrewJobPushesToTap(t *testing.T) {
	block := homebrewJobBlock(t)

	for _, token := range []string{
		"unidoc/homebrew-tap",
		"Formula/unipdf-debugger.rb",
		"gh auth login --with-token",
	} {
		if !strings.Contains(block, token) {
			t.Errorf("release.yml jobs.homebrew: missing required token %q (AC #3, #8)", token)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job commits as unidoc-release-bot
// Covers AC #3 (commit author identity on the external tap repo)
// ---------------------------------------------------------------------------

// TestHomebrewJobCommitAuthor asserts the homebrew job configures the git
// commit author as `unidoc-release-bot <release-bot@unidoc.io>`. A bot
// identity keeps human maintainers' emails off external-repo commit history
// and makes the tap commit log self-documenting.
func TestHomebrewJobCommitAuthor(t *testing.T) {
	block := homebrewJobBlock(t)

	for _, needle := range []string{
		"unidoc-release-bot",
		"release-bot@unidoc.io",
	} {
		if !strings.Contains(block, needle) {
			t.Errorf("release.yml jobs.homebrew: must configure git commit author %q (AC #3)", needle)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.4-E2E-001 (P0): homebrew job smoke-tests `brew install` after push
// Covers AC #6 (post-publish `brew install` smoke test of the tap)
// ---------------------------------------------------------------------------

// TestHomebrewJobSmokeTestsBrewInstall asserts the homebrew job runs, as its
// last step, `brew tap unidoc/tap` + `brew install --build-from-source
// unidoc/tap/unipdf-debugger` and exercises `--version` against the installed
// binary (AC #6).
func TestHomebrewJobSmokeTestsBrewInstall(t *testing.T) {
	block := homebrewJobBlock(t)

	for _, needle := range []string{
		"brew tap unidoc/tap",
		"brew install --build-from-source unidoc/tap/unipdf-debugger",
		"--version",
	} {
		if !strings.Contains(block, needle) {
			t.Errorf("release.yml jobs.homebrew: missing brew-install smoke-test token %q (AC #6)", needle)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job gates downstream steps on tap-token presence
// Covers AC #8 (fork without HOMEBREW_TAP_TOKEN: warn + skip, do not fail)
// ---------------------------------------------------------------------------

// TestHomebrewJobTokenGateExists asserts the homebrew job contains a step
// that checks HOMEBREW_TAP_TOKEN presence and emits an `available=...` step
// output, and that a downstream step is gated via `if: ... == 'true'`.
// Rationale: a fork running the release workflow without the secret should
// surface a warning and exit 0, not fail the release (AC #8).
func TestHomebrewJobTokenGateExists(t *testing.T) {
	block := homebrewJobBlock(t)

	if !strings.Contains(block, "HOMEBREW_TAP_TOKEN") {
		t.Errorf("release.yml jobs.homebrew: must reference HOMEBREW_TAP_TOKEN (AC #8)")
	}
	// Gate-step signature: writes an `available=...` step-output, keyed on
	// token presence.
	if !regexp.MustCompile(`available=(true|false)`).MatchString(block) {
		t.Errorf("release.yml jobs.homebrew: token gate must emit `available=true|false` step output (AC #8)")
	}
	// At least one downstream step must consume the gate via `if:` expr
	// referencing the gate's output.
	if !regexp.MustCompile(`if:\s*steps\.[a-zA-Z_][a-zA-Z_0-9]*\.outputs\.available\s*==\s*'true'`).MatchString(block) {
		t.Errorf("release.yml jobs.homebrew: downstream steps must be gated via `if: steps.<id>.outputs.available == 'true'` (AC #8)")
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): HOMEBREW_TAP_TOKEN never mapped to a plaintext literal
// Covers AC #8 (mirrors TestSecretsNotLoggedOutsideSecretsExpr pattern)
// ---------------------------------------------------------------------------

// TestHomebrewJobSecretsNotLoggedOutsideSecretsExpr asserts every YAML-level
// `HOMEBREW_TAP_TOKEN:` mapping has a value starting with `${{`. Shell-level
// references (`$HOMEBREW_TAP_TOKEN`, `${HOMEBREW_TAP_TOKEN:-}`) inside `run:`
// scripts are NOT flagged: they are runtime env expansions, not YAML keys.
func TestHomebrewJobSecretsNotLoggedOutsideSecretsExpr(t *testing.T) {
	raw := readReleaseWorkflow(t)

	re := regexp.MustCompile(`(?m)^\s*HOMEBREW_TAP_TOKEN:\s*(.+)$`)
	matches := re.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		t.Errorf("release.yml: no HOMEBREW_TAP_TOKEN: YAML mapping found; homebrew job must reference the secret (AC #8)")
		return
	}
	for _, m := range matches {
		val := strings.TrimSpace(m[1])
		if !strings.HasPrefix(val, "${{") {
			t.Errorf("release.yml: secret HOMEBREW_TAP_TOKEN mapped to non-secrets-expression value %q (plaintext leak risk; AC #8)", val)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-001 (P0): homebrew job rejects UNSIGNED macOS assets
// Covers AC #11 (fail fast if release contains `...-UNSIGNED.app.zip`)
// ---------------------------------------------------------------------------

// TestHomebrewJobRejectsUnsignedMacosAssets asserts the homebrew job contains
// a gate that detects `...-UNSIGNED.app.zip` assets on the release and fails
// fast with `exit 1` before formula render. Publishing a formula that points
// at unsigned .app assets ships Gatekeeper-blocked binaries to `brew install`
// users.
func TestHomebrewJobRejectsUnsignedMacosAssets(t *testing.T) {
	block := homebrewJobBlock(t)

	// The substring `-UNSIGNED.app.zip` (possibly within a regex or a
	// single-quoted grep pattern) must appear inside a step that calls
	// `gh release view` and includes an `exit 1`. Grep for all three.
	if !strings.Contains(block, "-UNSIGNED") || !strings.Contains(block, ".app.zip") {
		t.Errorf("release.yml jobs.homebrew: must detect `-UNSIGNED.app.zip` assets (AC #11)")
	}
	if !strings.Contains(block, "gh release view") {
		t.Errorf("release.yml jobs.homebrew: UNSIGNED-asset gate must use `gh release view` to enumerate assets (AC #11)")
	}
	if !strings.Contains(block, "exit 1") {
		t.Errorf("release.yml jobs.homebrew: UNSIGNED-asset gate must fail fast via `exit 1` (AC #11)")
	}
}

// ---------------------------------------------------------------------------
// 7.4-INTG-002 (P1): homebrew job is idempotent on re-run against same tag
// Covers AC #12 (workflow_dispatch re-run of same tag -> no duplicate commit)
// ---------------------------------------------------------------------------

// TestHomebrewJobIdempotentPush asserts the homebrew job's publish step
// stages the formula (`git add`) BEFORE checking for staged changes (`git
// diff --cached --quiet`). This pattern correctly handles BOTH first-ever
// publish (new untracked file) AND no-op re-runs (byte-identical content ->
// no commit). A bare `git diff --quiet` without the prior `git add` would
// silently skip the first-ever publish for untracked new files (AC #12).
func TestHomebrewJobIdempotentPush(t *testing.T) {
	block := homebrewJobBlock(t)

	addIdx := strings.Index(block, "git add Formula/unipdf-debugger.rb")
	if addIdx == -1 {
		t.Errorf("release.yml jobs.homebrew: publish step must `git add Formula/unipdf-debugger.rb` before diff check (AC #12 first-publish safety)")
		return
	}
	diffIdx := strings.Index(block, "git diff --cached --quiet")
	if diffIdx == -1 {
		t.Errorf("release.yml jobs.homebrew: publish step must guard commit via `git diff --cached --quiet` (AC #12 idempotent re-run)")
		return
	}
	if diffIdx < addIdx {
		t.Errorf("release.yml jobs.homebrew: `git add` must precede `git diff --cached --quiet` (AC #12; order matters for untracked files)")
	}
}

// ---------------------------------------------------------------------------
// 7.4-STATIC-001 + 7.4-STATIC-002 (P0): Homebrew formula template exists with
// required placeholder tokens, class name, and core attributes
// Covers AC #2 + AC #9
// ---------------------------------------------------------------------------

// TestHomebrewFormulaTemplateExists asserts the template ships at
// `scripts/homebrew/unipdf-debugger.rb.tmpl` and contains the canonical
// class header + all seven `@@...@@` placeholder tokens that the render
// step substitutes.
func TestHomebrewFormulaTemplateExists(t *testing.T) {
	root := projectRoot(t)
	p := filepath.Join(root, "scripts", "homebrew", "unipdf-debugger.rb.tmpl")
	content, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("scripts/homebrew/unipdf-debugger.rb.tmpl not found: %v (AC #2, #9)", err)
	}
	raw := string(content)

	// 7.4-STATIC-002: class name convention (filename-derived camelCase).
	if !strings.Contains(raw, "class UnipdfDebugger < Formula") {
		t.Errorf("template missing `class UnipdfDebugger < Formula` (AC #2; 7.4-STATIC-002 class-name convention)")
	}

	// AC #2 core attributes.
	for _, attr := range []string{
		`desc "`,
		`homepage "https://github.com/unidoc/unipdf-debugger"`,
		`license "Apache-2.0"`,
		`on_macos`,
		`on_linux`,
	} {
		if !strings.Contains(raw, attr) {
			t.Errorf("template missing required attribute %q (AC #2)", attr)
		}
	}

	// AC #9: seven placeholder tokens (3 SHAs + 3 URLs + 1 version).
	placeholders := []string{
		"@@VERSION@@",
		"@@DARWIN_ARM64_SHA256@@",
		"@@DARWIN_AMD64_SHA256@@",
		"@@LINUX_AMD64_SHA256@@",
		"@@DARWIN_ARM64_URL@@",
		"@@DARWIN_AMD64_URL@@",
		"@@LINUX_AMD64_URL@@",
	}
	for _, ph := range placeholders {
		if !strings.Contains(raw, ph) {
			t.Errorf("template missing placeholder token %q (AC #9)", ph)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.4-STATIC-001 (P0): template is syntactically valid Ruby after placeholder
// substitution (`ruby -c` passes). Skipped if Ruby is not on PATH (local dev).
// ---------------------------------------------------------------------------

// TestHomebrewFormulaTemplateIsSyntacticallyValid reads the template,
// substitutes every `@@...@@` with a safe stub (SemVer zeroes, example URL,
// 64-char hex), writes the stubbed content to t.TempDir, and runs `ruby -c`.
// Must exit 0.
func TestHomebrewFormulaTemplateIsSyntacticallyValid(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH; CI runners (macos-latest, ubuntu-latest) ship Ruby -- skipping locally")
	}
	root := projectRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "homebrew", "unipdf-debugger.rb.tmpl"))
	if err != nil {
		t.Fatalf("template not readable: %v (AC #2, #9)", err)
	}

	stub := string(raw)
	sha := strings.Repeat("0", 64)
	repls := map[string]string{
		"@@VERSION@@":             "0.0.0",
		"@@DARWIN_ARM64_URL@@":    "https://example.com/x",
		"@@DARWIN_AMD64_URL@@":    "https://example.com/x",
		"@@LINUX_AMD64_URL@@":     "https://example.com/x",
		"@@DARWIN_ARM64_SHA256@@": sha,
		"@@DARWIN_AMD64_SHA256@@": sha,
		"@@LINUX_AMD64_SHA256@@":  sha,
	}
	for k, v := range repls {
		stub = strings.ReplaceAll(stub, k, v)
	}

	tmp := filepath.Join(t.TempDir(), "formula-stub.rb")
	if err := os.WriteFile(tmp, []byte(stub), 0o644); err != nil {
		t.Fatalf("write stubbed formula: %v", err)
	}
	cmd := exec.Command("ruby", "-c", tmp)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("template is not syntactically valid Ruby after stub-substitution (AC #2, #9)\n%s", string(out))
	}
}

// ---------------------------------------------------------------------------
// 7.4-STATIC-001 (P0): render-formula.sh exists and is executable
// Covers AC #3 + AC #9 (render script present in repo)
// ---------------------------------------------------------------------------

// TestRenderFormulaScriptExistsAndExecutable asserts
// `scripts/homebrew/render-formula.sh` exists and has the executable bit set
// on Unix (Windows skipped per story 7-3 precedent).
func TestRenderFormulaScriptExistsAndExecutable(t *testing.T) {
	root := projectRoot(t)
	p := filepath.Join(root, "scripts", "homebrew", "render-formula.sh")
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("scripts/homebrew/render-formula.sh not found: %v (AC #3, #9)", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	// Executable bit set for owner (at minimum).
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("scripts/homebrew/render-formula.sh is not executable (perm=%v); `chmod +x` required (AC #3, #9)", info.Mode().Perm())
	}
}

// ---------------------------------------------------------------------------
// 7.4-STATIC-001 (P0): render-formula.sh produces valid Ruby when invoked
// with stub arguments. Skipped if Ruby is not on PATH.
// ---------------------------------------------------------------------------

// TestRenderFormulaScriptProducesValidRuby invokes the render script with
// documented stub arguments (version, three SHA256 stubs, tag), pipes stdout
// through `ruby -c`, and verifies (a) exit 0, (b) the output contains the
// expected version literal + stub SHAs + three github.com release URLs.
func TestRenderFormulaScriptProducesValidRuby(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH; CI runners ship Ruby -- skipping locally")
	}
	root := projectRoot(t)
	script := filepath.Join(root, "scripts", "homebrew", "render-formula.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("render-formula.sh missing: %v (AC #3, #9)", err)
	}

	sha1 := strings.Repeat("a", 64)
	sha2 := strings.Repeat("b", 64)
	sha3 := strings.Repeat("c", 64)
	version := "0.1.0"
	tag := "v0.1.0"

	cmd := exec.Command("bash", script, version, sha1, sha2, sha3, tag)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("render-formula.sh failed with args (%s, %s..., %s..., %s..., %s): %v", version, sha1[:4], sha2[:4], sha3[:4], tag, err)
	}
	rendered := string(out)

	// Syntax check via `ruby -c /dev/stdin`.
	rcheck := exec.Command("ruby", "-c", "/dev/stdin")
	rcheck.Stdin = strings.NewReader(rendered)
	if rout, rerr := rcheck.CombinedOutput(); rerr != nil {
		t.Errorf("rendered formula is not valid Ruby (AC #3, #9)\n%s", string(rout))
	}

	// Content assertions: version literal + three SHAs + three asset URLs.
	if !strings.Contains(rendered, `version "0.1.0"`) {
		t.Errorf("rendered formula missing `version \"0.1.0\"` (AC #3)")
	}
	for _, sha := range []string{sha1, sha2, sha3} {
		if !strings.Contains(rendered, sha) {
			t.Errorf("rendered formula missing injected SHA256 %s... (AC #3)", sha[:8])
		}
	}
	baseURL := "https://github.com/unidoc/unipdf-debugger/releases/download/v0.1.0/"
	for _, asset := range []string{
		baseURL + "unidoc-pdf-debugger-0.1.0-darwin-arm64.app.zip",
		baseURL + "unidoc-pdf-debugger-0.1.0-darwin-amd64.app.zip",
		baseURL + "unidoc-pdf-debugger-0.1.0-linux-amd64.tar.gz",
	} {
		if !strings.Contains(rendered, asset) {
			t.Errorf("rendered formula missing expected asset URL %q (AC #3)", asset)
		}
	}
}
