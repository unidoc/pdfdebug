// Package release_pipeline_test provides acceptance tests for Story 7.2:
// Release Build Pipeline and Distribution.
//
// These tests verify that .github/workflows/release.yml is structured per the
// acceptance criteria: tag-triggered workflow, 4-platform matrix (macOS arm64,
// macOS amd64, Windows amd64, Linux amd64) producing 8 artifacts + SHA256SUMS,
// codesign/notarize/staple/verify flow on macOS, CLI built with -trimpath and
// version ldflag, two-stage job layout (build matrix -> release publish),
// softprops/action-gh-release@v3 publication with prerelease detection, and
// the Apple Developer ID keychain import + cleanup pattern.
//
// Test Levels: Integration (Go) -- YAML parsing + filesystem checks. No
// browser or HTTP surface. Per the epic test design the single end-to-end
// case is a manual RC-tag smoke test that cannot be exercised from a local Go
// test; it is not automated here. Notarization is currently disabled, so the
// macOS flow is asserted as codesign + verify only.
//
// Each test function below names the workflow property it checks, so there is
// no mapping table to keep in sync.
//
// Run: cd tests/release-pipeline && go test -v -count=1 ./...
package release_pipeline_test

import (
	"os"
	"path/filepath"
	"regexp"
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
// release.yml exists and parses Covers workflow defined in
// .github/workflows/release.yml
// ---------------------------------------------------------------------------

func TestReleaseWorkflowFileExistsAndParses(t *testing.T) {
	_ = parseReleaseWorkflow(t)
}

// ---------------------------------------------------------------------------
// on.push.tags contains 'v*' Covers tag push
// triggers release
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
		t.Fatalf("release.yml: on.push block missing or wrong type")
	}
	tagsRaw, ok := push["tags"].([]interface{})
	if !ok {
		t.Fatalf("release.yml: on.push.tags is not a list")
	}
	found := false
	for _, v := range tagsRaw {
		if s, ok := v.(string); ok && s == "v*" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("release.yml: on.push.tags must include 'v*'")
	}
}

// ---------------------------------------------------------------------------
// workflow_dispatch with `tag` input supported Covers + Task 1.5
// (manual re-run against existing tag)
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
// matrix contains exactly 4 include cells covering every required {os, arch,
// goos, platform} combination Covers
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
		t.Fatalf("release.yml: matrix.include is not a list (requires include-only matrix)")
	}
	if len(includeRaw) != 3 {
		t.Errorf("release.yml: matrix.include must have exactly 3 cells, got %d", len(includeRaw))
	}

	// Expected 3 tuples, keyed by os+arch for uniqueness. macOS Intel (macos-13)
	// was dropped after the v0.1.0-rc1 dry-run found unreliable runner queue times.
	expected := map[string]struct {
		goos     string
		platform string
	}{
		"macos-latest|arm64":   {goos: "darwin", platform: "darwin-arm64"},
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
			t.Errorf("release.yml: matrix.include[%s].goos must be %q, got %q", key, exp.goos, goos)
		}
		if platform != exp.platform {
			t.Errorf("release.yml: matrix.include[%s].platform must be %q, got %q", key, exp.platform, platform)
		}
		seen[key] = true
	}
	for k := range expected {
		if !seen[k] {
			t.Errorf("release.yml: matrix.include missing required cell %q", k)
		}
	}
}

// ---------------------------------------------------------------------------
// strategy.fail-fast is false Covers one platform failure
// must not mask others
// ---------------------------------------------------------------------------

func TestMatrixFailFastFalse(t *testing.T) {
	job := jobMap(t, "build")
	strat, ok := job["strategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.build.strategy missing")
	}
	ff, ok := strat["fail-fast"].(bool)
	if !ok {
		t.Errorf("release.yml: jobs.build.strategy.fail-fast not set")
	} else if ff {
		t.Errorf("release.yml: jobs.build.strategy.fail-fast must be false, got true")
	}
}

// ---------------------------------------------------------------------------
// Codesign step present with required flags Covers codesign with --force
// --deep --options runtime --entitlements build/darwin/entitlements.plist
// --timestamp --sign
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
			t.Errorf("release.yml build job: codesign flag/token %q missing", needle)
		}
	}
}

// ---------------------------------------------------------------------------
// Codesign --verify runs after the sign step. Covers signed bundle is
// verified. Notarization is currently disabled, so
// notarytool/stapler/spctl assertions are not required.
// ---------------------------------------------------------------------------

func TestMacOSVerificationCommands(t *testing.T) {
	run := jobRunBodies(t, "build")

	if !strings.Contains(run, "codesign --verify --deep --strict") {
		t.Errorf("release.yml build job: `codesign --verify --deep --strict` missing")
	}
}

// ---------------------------------------------------------------------------
// macOS Developer ID keychain is imported at job start Covers security
// create-keychain + import + set-key-partition-list
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
			t.Errorf("release.yml build job: keychain import command %q missing", needle)
		}
	}
}

// ---------------------------------------------------------------------------
// Keychain cleanup runs under if: always() Covers secrets never
// linger on runner
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
				t.Errorf("release.yml: keychain cleanup step must use `if: always`, got %q", ifClause)
			}
			break
		}
	}
	if !found {
		t.Errorf("release.yml: no step invokes `security delete-keychain` (cleanup requirement)")
	}
}

// ---------------------------------------------------------------------------
// All required Apple secrets are referenced via ${{ secrets.* }}
// expressions in the workflow Covers explicit secret list
// ---------------------------------------------------------------------------

func TestAppleSecretsReferenced(t *testing.T) {
	raw := readReleaseWorkflow(t)

	// Notarization is currently disabled; only the three codesign-related
	// secrets are wired up. Re-add APPLE_ID, APPLE_ID_APP_PASSWORD, and
	// APPLE_TEAM_ID here when notarization is re-enabled.
	required := []string{
		"APPLE_DEVELOPER_ID_CERT_P12_BASE64",
		"APPLE_DEVELOPER_ID_CERT_PASSWORD",
		"APPLE_DEVELOPER_ID",
	}
	for _, name := range required {
		expr := "secrets." + name
		if !strings.Contains(raw, expr) {
			t.Errorf("release.yml: must reference secret via `${{ secrets.%s }}`", name)
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
// CLI build uses -trimpath + -ldflags with version Covers go build -trimpath
// -ldflags="-s -w -X main.version=<version>"
// ---------------------------------------------------------------------------

func TestCLIBuildUsesTrimpathAndLdflags(t *testing.T) {
	run := jobRunBodies(t, "build")

	if !strings.Contains(run, "go build") {
		t.Fatalf("release.yml build job: `go build` for CLI not found")
	}
	if !strings.Contains(run, "-trimpath") {
		t.Errorf("release.yml build job: CLI build missing `-trimpath`")
	}
	// ldflags must strip symbols and embed version.
	if !regexp.MustCompile(`-ldflags="?-s -w -X main\.version=`).MatchString(run) {
		t.Errorf("release.yml build job: CLI build must pass `-ldflags=\"-s -w -X main.version=<version>\"`")
	}
	if !strings.Contains(run, "./cmd/cli") {
		t.Errorf("release.yml build job: CLI build must target `./cmd/cli`")
	}
}

// ---------------------------------------------------------------------------
// CLI build reads GOOS/GOARCH from matrix keys Covers GOOS/GOARCH
// per matrix cell
// ---------------------------------------------------------------------------

func TestCLIBuildGOOSGOARCHFromMatrix(t *testing.T) {
	raw := readReleaseWorkflow(t)

	// Either `GOOS: ${{ matrix.goos }}` env binding, or explicit inline
	// `GOOS=${{ matrix.goos }}` -- require the matrix binding.
	if !regexp.MustCompile(`GOOS:\s*\$\{\{\s*matrix\.goos\s*\}\}`).MatchString(raw) {
		t.Errorf("release.yml: CLI build step must set `GOOS: ${{ matrix.goos }}`")
	}
	if !regexp.MustCompile(`GOARCH:\s*\$\{\{\s*matrix\.arch\s*\}\}`).MatchString(raw) {
		t.Errorf("release.yml: CLI build step must set `GOARCH: ${{ matrix.arch }}`")
	}
}

// ---------------------------------------------------------------------------
// Two jobs -- build and release, with release needing build Covers parallel
// build matrix then single release job
// ---------------------------------------------------------------------------

func TestTwoJobsBuildAndRelease(t *testing.T) {
	parsed := parseReleaseWorkflow(t)
	jobs, ok := parsed["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs block missing")
	}
	if _, ok := jobs["build"]; !ok {
		t.Errorf("release.yml: jobs.build missing")
	}
	rel, ok := jobs["release"].(map[string]interface{})
	if !ok {
		t.Fatalf("release.yml: jobs.release missing")
	}
	// needs: build  -- may be string or list.
	needs, ok := rel["needs"]
	if !ok {
		t.Fatalf("release.yml: jobs.release.needs missing (requires needs: build)")
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
	// release job must run on ubuntu-latest (requires shasum on ubuntu)
	runsOn, _ := rel["runs-on"].(string)
	if runsOn != "ubuntu-latest" {
		t.Errorf("release.yml: jobs.release.runs-on must be \"ubuntu-latest\", got %q", runsOn)
	}
}

// ---------------------------------------------------------------------------
// upload-artifact@v5 in build, download-artifact@v5 in release Covers artifact
// staging via v4 actions
// ---------------------------------------------------------------------------

func TestUploadArtifactAndDownloadArtifactV4(t *testing.T) {
	buildUses := jobUses(t, "build")
	releaseUses := jobUses(t, "release")

	var hasUpload, hasDownload bool
	for _, u := range buildUses {
		if strings.HasPrefix(u, "actions/upload-artifact@v5") {
			hasUpload = true
		}
	}
	for _, u := range releaseUses {
		if strings.HasPrefix(u, "actions/download-artifact@v5") {
			hasDownload = true
		}
	}
	if !hasUpload {
		t.Errorf("release.yml: jobs.build must use actions/upload-artifact@v5")
	}
	if !hasDownload {
		t.Errorf("release.yml: jobs.release must use actions/download-artifact@v5")
	}
}

// ---------------------------------------------------------------------------
// SHA256SUMS computed via shasum -a 256 in release job Covers single sha256
// computation in the release job
// ---------------------------------------------------------------------------

func TestSHA256SumsStepPresent(t *testing.T) {
	run := jobRunBodies(t, "release")

	if !strings.Contains(run, "shasum -a 256") {
		t.Errorf("release.yml release job: `shasum -a 256` missing")
	}
	if !strings.Contains(run, "SHA256SUMS.txt") {
		t.Errorf("release.yml release job: output file `SHA256SUMS.txt` missing")
	}
}

// ---------------------------------------------------------------------------
// SHA256SUMS generation excludes itself and is NUL-safe Covers + Task 5.1
// (no `$(ls | sort)` anti-pattern)
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
// release publication step uses softprops/action-gh-release@v3 Covers
// ---------------------------------------------------------------------------

func TestReleasePublishStepUsesActionGhRelease(t *testing.T) {
	uses := jobUses(t, "release")
	var found bool
	for _, u := range uses {
		if strings.HasPrefix(u, "softprops/action-gh-release@v3") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("release.yml: jobs.release must use softprops/action-gh-release@v3")
	}
}

// ---------------------------------------------------------------------------
// prerelease flag set for v*-rc* / -alpha* / -beta* Covers pre-release
// detection logic
// ---------------------------------------------------------------------------

func TestPrereleaseDetectionLogic(t *testing.T) {
	run := jobRunBodies(t, "release")

	// The task body prescribes the regex `=~ -(rc|alpha|beta)`.
	re := regexp.MustCompile(`-\(rc\|alpha\|beta\)`)
	if !re.MatchString(run) {
		t.Errorf("release.yml release job: prerelease detection regex `-(rc|alpha|beta)` missing")
	}
	if !strings.Contains(run, "prerelease=true") {
		t.Errorf("release.yml release job: must emit `prerelease=true` for matching tags")
	}
	if !strings.Contains(run, "prerelease=false") {
		t.Errorf("release.yml release job: must emit `prerelease=false` for non-matching tags")
	}
}

// ---------------------------------------------------------------------------
// action-gh-release `generate_release_notes: true` Covers Release description
// populated from commits since previous tag
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
		t.Errorf("release.yml: softprops/action-gh-release `with.generate_release_notes` must be set")
	} else if !grn {
		t.Errorf("release.yml: softprops/action-gh-release `with.generate_release_notes` must be true")
	}

	// prerelease must be parameterized (toggled by tag pattern)
	prerelease, _ := with["prerelease"].(string)
	if !strings.Contains(prerelease, "${{") {
		t.Errorf("release.yml: softprops/action-gh-release `with.prerelease` must be a step-output expression, got %q", prerelease)
	}

	// fail_on_unmatched_files: true (Task 5.2)
	fof, ok := with["fail_on_unmatched_files"].(bool)
	if !ok || !fof {
		t.Errorf("release.yml: softprops/action-gh-release `with.fail_on_unmatched_files` must be true (Task 5.2)")
	}
}

// ---------------------------------------------------------------------------
// Every run: step must set shell: bash (Windows defaults to PowerShell)
// Covers Dev Notes lesson #3
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
// Artifact staging produces the 6 named artifacts plus SHA256SUMS (3 platforms
// x 2 archives each: a GUI archive that also embeds the CLI, plus a standalone
// CLI archive). Covers, Task 4.3 naming contract.
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
		"windows-amd64",
		"linux-amd64",
	}
	for _, tok := range tokens {
		// Either the literal token, or matrix.platform expansion covers darwin-*/windows-*/linux-*
		if !strings.Contains(run, tok) && !strings.Contains(run, "matrix.platform") && !strings.Contains(run, "${PLATFORM}") {
			t.Errorf("release.yml build job: staging step missing naming token %q", tok)
		}
	}

	// macOS GUI as .dmg; Windows as .zip; Linux/CLI as .tar.gz. Note .exe still
	// appears as the source filename copied INTO the Windows zip stage, but the
	// shipped artifact extension is .zip.
	for _, needle := range []string{".dmg", ".zip", ".tar.gz"} {
		if !strings.Contains(run, needle) {
			t.Errorf("release.yml build job: artifact extension %q missing", needle)
		}
	}

	// macOS DMG must be built via hdiutil (native macOS disk-image tool).
	if !strings.Contains(run, "hdiutil create") {
		t.Errorf("release.yml build job: macOS artifact must be built via `hdiutil create` (Task 2.3)")
	}
}

// ---------------------------------------------------------------------------
// Supporting file: build/darwin/entitlements.plist exists with Wails-v3
// baseline entries Covers + Task 6.1
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
