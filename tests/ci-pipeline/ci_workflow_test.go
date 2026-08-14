// Package ci_pipeline_test provides acceptance tests for GitHub Actions CI
// Pipeline.
//
// These tests verify that .github/workflows/ci.yml is structured per the
// acceptance criteria: matrix build on ubuntu-latest/macos-latest/windows-latest,
// pinned Go 1.26.x and Node 20, Linux native deps install, Wails CLI pin
// matching go.mod, per-suite test loop for tests/*/go.mod modules, dependency
// caching, distinct check runs per platform, and 30-minute job timeout.
//
// Test Levels: Integration (Go) -- YAML parsing + filesystem checks.
// No browser or API surface exists for this story. Per the test design
// (infrastructure-as-code epic), tests are heavily skewed to static validation.
// The single end-to-end acceptance criterion (PR triggers green CI on all 3
// matrix cells) is verified manually by the smoke-test PR -- it cannot be
// exercised from a local Go test.
//
// Run: cd tests/ci-pipeline && go test -v -count=1 ./...
package ci_pipeline_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// projectRoot returns the absolute path to the project root directory.
// It walks upward from the test file location to find the project root,
// identified by the presence of a go.mod whose module name is "unidoc-pdf-debugger".
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
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

// readCIWorkflow reads and returns the raw ci.yml content as a string.
// Fails the test immediately if the file does not exist.
func readCIWorkflow(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	ciPath := filepath.Join(root, ".github", "workflows", "ci.yml")
	content, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf(".github/workflows/ci.yml not found: %v", err)
	}
	return string(content)
}

// parseCIWorkflow parses ci.yml into a generic map for structural inspection.
func parseCIWorkflow(t *testing.T) map[string]interface{} {
	t.Helper()
	raw := readCIWorkflow(t)
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("ci.yml is not valid YAML: %v", err)
	}
	return parsed
}

// buildAndTestJob returns the parsed `build-and-test` job map, failing the
// test if it is missing or malformed.
func buildAndTestJob(t *testing.T) map[string]interface{} {
	t.Helper()
	parsed := parseCIWorkflow(t)
	jobsRaw, ok := parsed["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: missing top-level `jobs` map")
	}
	jobRaw, ok := jobsRaw["build-and-test"]
	if !ok {
		t.Fatalf("ci.yml: missing `build-and-test` job (required so matrix check runs are named `build-and-test (<os>)`)")
	}
	jobMap, ok := jobRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: `build-and-test` is not a map")
	}
	return jobMap
}

// jobSteps returns the `steps` slice of the build-and-test job.
func jobSteps(t *testing.T) []interface{} {
	t.Helper()
	job := buildAndTestJob(t)
	stepsRaw, ok := job["steps"].([]interface{})
	if !ok {
		t.Fatalf("ci.yml: build-and-test.steps is not a list")
	}
	return stepsRaw
}

// stepRunBodies returns the concatenated `run:` bodies of all steps so tests
// can search for shell commands (go vet, wails3 build, apt-get, etc.).
func stepRunBodies(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	for _, s := range jobSteps(t) {
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

// stepUses returns the concatenated `uses:` values of all steps.
func stepUses(t *testing.T) []string {
	t.Helper()
	var uses []string
	for _, s := range jobSteps(t) {
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
// ci.yml exists and parses as valid YAML Covers workflow is
// defined in .github/workflows/ci.yml
// ---------------------------------------------------------------------------

func TestCIWorkflowFileExistsAndParses(t *testing.T) {

	_ = parseCIWorkflow(t)
}

// ---------------------------------------------------------------------------
// Workflow triggers include pull_request and push on main+dev
// Covers ("Given a pull request is opened or updated against `main` (or `dev`)")
// ---------------------------------------------------------------------------

func TestCIWorkflowTriggers(t *testing.T) {

	parsed := parseCIWorkflow(t)

	// gopkg.in/yaml.v3 parses YAML 1.2 so `on` stays a string key.
	onRaw, ok := parsed["on"]
	if !ok {
		t.Fatalf("ci.yml: missing top-level `on:` triggers block")
	}
	onMap, ok := onRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: `on:` is not a map")
	}

	pr, hasPR := onMap["pull_request"]
	if !hasPR {
		t.Errorf("ci.yml: `on.pull_request` trigger missing (requires PR trigger)")
	}
	pu, hasPush := onMap["push"]
	if !hasPush {
		t.Errorf("ci.yml: `on.push` trigger missing (requires push trigger)")
	}

	// Both pull_request and push must target master AND dev
	for label, block := range map[string]interface{}{"pull_request": pr, "push": pu} {
		if block == nil {
			continue
		}
		bm, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		branches, ok := bm["branches"].([]interface{})
		if !ok {
			t.Errorf("ci.yml: `on.%s.branches` list missing", label)
			continue
		}
		var has = map[string]bool{}
		for _, b := range branches {
			if s, ok := b.(string); ok {
				has[s] = true
			}
		}
		if !has["master"] {
			t.Errorf("ci.yml: `on.%s.branches` missing `master`", label)
		}
		if !has["dev"] {
			t.Errorf("ci.yml: `on.%s.branches` missing `dev`", label)
		}
	}
}

// ---------------------------------------------------------------------------
// matrix strategy includes all 3 OS runners
// Covers "matrix strategy with macos-latest, windows-latest, ubuntu-latest"
// ---------------------------------------------------------------------------

func TestCIWorkflowMatrixOS(t *testing.T) {

	job := buildAndTestJob(t)

	strat, ok := job["strategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: build-and-test.strategy is missing")
	}

	// fail-fast MUST be false so one flaky platform does not mask others (rationale)
	ff, ok := strat["fail-fast"].(bool)
	if !ok {
		t.Errorf("ci.yml: build-and-test.strategy.fail-fast not set")
	} else if ff {
		t.Errorf("ci.yml: build-and-test.strategy.fail-fast must be false, got true")
	}

	matrix, ok := strat["matrix"].(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: build-and-test.strategy.matrix is missing")
	}
	osList, ok := matrix["os"].([]interface{})
	if !ok {
		t.Fatalf("ci.yml: build-and-test.strategy.matrix.os is not a list")
	}

	found := map[string]bool{}
	for _, v := range osList {
		if s, ok := v.(string); ok {
			found[s] = true
		}
	}
	for _, required := range []string{"ubuntu-latest", "macos-latest", "windows-latest"} {
		if !found[required] {
			t.Errorf("ci.yml: matrix.os missing %q", required)
		}
	}
}

// ---------------------------------------------------------------------------
// runs-on references matrix.os Covers matrix-driven
// runner selection
// ---------------------------------------------------------------------------

func TestCIWorkflowRunsOnMatrix(t *testing.T) {

	job := buildAndTestJob(t)
	runsOn, ok := job["runs-on"].(string)
	if !ok {
		t.Fatalf("ci.yml: build-and-test.runs-on is not a string")
	}
	// Accept either exact ${{ matrix.os }} or whitespace variant.
	if !strings.Contains(runsOn, "matrix.os") {
		t.Errorf("ci.yml: build-and-test.runs-on must reference matrix.os, got %q", runsOn)
	}
}

// ---------------------------------------------------------------------------
// No job-level `name:` override (default = "build-and-test (<os>)") Covers (three
// distinct check runs named build-and-test (<os>))
// ---------------------------------------------------------------------------

func TestCIWorkflowNoJobNameOverride(t *testing.T) {

	job := buildAndTestJob(t)
	if _, exists := job["name"]; exists {
		t.Errorf("ci.yml: build-and-test MUST NOT have a `name:` field -- it would override the default `build-and-test (<os>)` check-run name and break branch protection per-platform")
	}
}

// ---------------------------------------------------------------------------
// job-level timeout-minutes is 30 Covers hard
// 30-minute cap per matrix cell
// ---------------------------------------------------------------------------

func TestCIWorkflowJobTimeout(t *testing.T) {

	job := buildAndTestJob(t)
	tRaw, ok := job["timeout-minutes"]
	if !ok {
		t.Fatalf("ci.yml: build-and-test.timeout-minutes is not set (requires 30-minute cap)")
	}
	tVal, ok := tRaw.(int)
	if !ok {
		t.Fatalf("ci.yml: build-and-test.timeout-minutes is not an integer, got %T", tRaw)
	}
	if tVal != 30 {
		t.Errorf("ci.yml: build-and-test.timeout-minutes must be 30, got %d", tVal)
	}
}

// ---------------------------------------------------------------------------
// workflow-level `permissions: contents: read` Covers security
// hardening in Dev Notes + Task 1.2
// ---------------------------------------------------------------------------

func TestCIWorkflowPermissionsReadOnly(t *testing.T) {

	parsed := parseCIWorkflow(t)
	permsRaw, ok := parsed["permissions"]
	if !ok {
		t.Fatalf("ci.yml: workflow-level `permissions` block missing (defense-in-depth requirement)")
	}
	perms, ok := permsRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: `permissions` is not a map")
	}
	contents, ok := perms["contents"].(string)
	if !ok {
		t.Errorf("ci.yml: permissions.contents missing")
	} else if contents != "read" {
		t.Errorf("ci.yml: permissions.contents must be \"read\", got %q", contents)
	}
}

// ---------------------------------------------------------------------------
// workflow-level concurrency.cancel-in-progress is true Covers Dev Notes
// "Concurrency" rationale
// ---------------------------------------------------------------------------

func TestCIWorkflowConcurrency(t *testing.T) {

	parsed := parseCIWorkflow(t)
	cRaw, ok := parsed["concurrency"]
	if !ok {
		t.Fatalf("ci.yml: workflow-level `concurrency` block missing")
	}
	c, ok := cRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: `concurrency` is not a map")
	}
	group, ok := c["group"].(string)
	if !ok || group == "" {
		t.Errorf("ci.yml: concurrency.group missing or empty")
	} else if !strings.Contains(group, "github.ref") {
		t.Errorf("ci.yml: concurrency.group should key on github.ref, got %q", group)
	}
	if cip, ok := c["cancel-in-progress"].(bool); !ok {
		t.Errorf("ci.yml: concurrency.cancel-in-progress missing")
	} else if !cip {
		t.Errorf("ci.yml: concurrency.cancel-in-progress must be true")
	}
}

// ---------------------------------------------------------------------------
// Go pinned to 1.26.x via setup-go@v6 with cache-dependency-path Covers Go pinned to
// 1.26.x via actions/setup-go@v6 and (caching)
// ---------------------------------------------------------------------------

func TestCIWorkflowSetupGoPinAndCache(t *testing.T) {

	var goStep map[string]interface{}
	for _, s := range jobSteps(t) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if u, ok := m["uses"].(string); ok && strings.HasPrefix(u, "actions/setup-go@") {
			goStep = m
			if !strings.HasPrefix(u, "actions/setup-go@v6") {
				t.Errorf("ci.yml: setup-go must be @v6, got %q", u)
			}
			break
		}
	}
	if goStep == nil {
		t.Fatalf("ci.yml: actions/setup-go step not found")
	}

	with, ok := goStep["with"].(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: setup-go.with block missing")
	}
	ver, ok := with["go-version"].(string)
	if !ok {
		t.Fatalf("ci.yml: setup-go.with.go-version missing")
	}
	// Accept "1.26.x" or equivalent. Must start with 1.26. and NOT be 1.260, 1.26-rc, etc.
	goVerRe := regexp.MustCompile(`^1\.26(\.|$)`)
	if !goVerRe.MatchString(ver) {
		t.Errorf("ci.yml: setup-go.go-version must pin 1.26.x, got %q", ver)
	}

	// cache-dependency-path must include BOTH root go.sum AND tests/** go.sum
	cdp, hasCDP := with["cache-dependency-path"]
	if !hasCDP {
		t.Fatalf("ci.yml: setup-go.with.cache-dependency-path missing (per-suite modules must be cached)")
	}
	cdpStr := ""
	switch v := cdp.(type) {
	case string:
		cdpStr = v
	case []interface{}:
		for _, p := range v {
			if s, ok := p.(string); ok {
				cdpStr += s + "\n"
			}
		}
	default:
		t.Fatalf("ci.yml: cache-dependency-path has unexpected type %T", cdp)
	}
	if !strings.Contains(cdpStr, "go.sum") {
		t.Errorf("ci.yml: setup-go cache-dependency-path must include root go.sum")
	}
	if !strings.Contains(cdpStr, "tests/") || !strings.Contains(cdpStr, "go.sum") {
		t.Errorf("ci.yml: setup-go cache-dependency-path must include tests/**/go.sum")
	}
}

// ---------------------------------------------------------------------------
// Node pinned to 20 via setup-node@v5 with npm cache Covers Node pinned to 20 via
// actions/setup-node@v5 and (npm caching)
// ---------------------------------------------------------------------------

func TestCIWorkflowSetupNodePinAndCache(t *testing.T) {

	var nodeStep map[string]interface{}
	for _, s := range jobSteps(t) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if u, ok := m["uses"].(string); ok && strings.HasPrefix(u, "actions/setup-node@") {
			nodeStep = m
			if !strings.HasPrefix(u, "actions/setup-node@v5") {
				t.Errorf("ci.yml: setup-node must be @v5, got %q", u)
			}
			break
		}
	}
	if nodeStep == nil {
		t.Fatalf("ci.yml: actions/setup-node step not found")
	}

	with, ok := nodeStep["with"].(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: setup-node.with block missing")
	}

	// node-version may be "20" string or 20 integer.
	nv, ok := with["node-version"]
	if !ok {
		t.Fatalf("ci.yml: setup-node.with.node-version missing")
	}
	switch v := nv.(type) {
	case string:
		if !strings.HasPrefix(v, "20") {
			t.Errorf("ci.yml: setup-node.node-version must be 20 LTS, got %q", v)
		}
	case int:
		if v != 20 {
			t.Errorf("ci.yml: setup-node.node-version must be 20 LTS, got %d", v)
		}
	default:
		t.Errorf("ci.yml: setup-node.node-version has unexpected type %T", v)
	}

	cache, ok := with["cache"].(string)
	if !ok {
		t.Errorf("ci.yml: setup-node.with.cache missing")
	} else if cache != "npm" {
		t.Errorf("ci.yml: setup-node.with.cache must be \"npm\", got %q", cache)
	}

	cdp, ok := with["cache-dependency-path"].(string)
	if !ok {
		t.Errorf("ci.yml: setup-node.with.cache-dependency-path missing")
	} else if cdp != "frontend/package-lock.json" {
		t.Errorf("ci.yml: setup-node.cache-dependency-path must be frontend/package-lock.json, got %q", cdp)
	}
}

// ---------------------------------------------------------------------------
// Linux-guarded step installs apt native deps Covers libgtk-3-dev,
// libwebkit2gtk-4.1-dev, build-essential on Linux
// ---------------------------------------------------------------------------

func TestCIWorkflowLinuxNativeDeps(t *testing.T) {

	var linuxStep map[string]interface{}
	for _, s := range jobSteps(t) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		ifClause, _ := m["if"].(string)
		run, _ := m["run"].(string)
		if strings.Contains(ifClause, "Linux") && strings.Contains(run, "apt-get") {
			linuxStep = m
			break
		}
	}
	if linuxStep == nil {
		t.Fatalf("ci.yml: Linux-guarded apt-get step not found")
	}

	ifClause := linuxStep["if"].(string)
	if !strings.Contains(ifClause, "runner.os") || !strings.Contains(ifClause, "Linux") {
		t.Errorf("ci.yml: Linux step `if` must guard on `runner.os == 'Linux'`, got %q", ifClause)
	}

	run := linuxStep["run"].(string)
	for _, pkg := range []string{"libgtk-3-dev", "libwebkit2gtk-4.1-dev", "build-essential"} {
		if !strings.Contains(run, pkg) {
			t.Errorf("ci.yml: Linux apt-get step missing package %q", pkg)
		}
	}
	if !strings.Contains(run, "apt-get update") {
		t.Errorf("ci.yml: Linux step should run `apt-get update` before install")
	}
}

// ---------------------------------------------------------------------------
// Wails CLI pin matches go.mod Covers go install wails3 at pinned alpha
// version matching go.mod
// ---------------------------------------------------------------------------

func TestCIWorkflowWailsCLIPinMatchesGoMod(t *testing.T) {

	root := projectRoot(t)
	goModBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("failed to read root go.mod: %v", err)
	}
	// Extract the exact Wails v3 pin from go.mod, e.g. v3.0.0-alpha.74
	re := regexp.MustCompile(`github\.com/wailsapp/wails/v3\s+(v[0-9A-Za-z.+\-]+)`)
	m := re.FindStringSubmatch(string(goModBytes))
	if len(m) < 2 {
		t.Fatalf("could not extract wails/v3 pin from go.mod")
	}
	pin := m[1]

	run := stepRunBodies(t)
	expected := "github.com/wailsapp/wails/v3/cmd/wails3@" + pin
	if !strings.Contains(run, expected) {
		t.Errorf("ci.yml: wails3 install must pin to %q (from go.mod); not found in any step `run:` body", expected)
	}
}

// ---------------------------------------------------------------------------
// golangci-lint v2 module path pinned (not @latest) Covers Task 1.5 step
// 6 rationale + linting
// ---------------------------------------------------------------------------

func TestCIWorkflowGolangciLintV2Pinned(t *testing.T) {

	run := stepRunBodies(t)
	// v2 module path; the @v2.x.y pin (exact version, not @latest)
	re := regexp.MustCompile(`github\.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2\.[0-9]+\.[0-9]+`)
	if !re.MatchString(run) {
		t.Errorf("ci.yml: golangci-lint install must use v2 module path with pinned @v2.x.y version; @latest is forbidden")
	}
	if strings.Contains(run, "golangci-lint@latest") {
		t.Errorf("ci.yml: golangci-lint@latest is forbidden (pin a concrete version)")
	}
	// Must be invoked as a lint step
	if !strings.Contains(run, "golangci-lint run") {
		t.Errorf("ci.yml: golangci-lint run step missing")
	}
}

// ---------------------------------------------------------------------------
// go vet ./... step present Covers go vet ./...
// passes
// ---------------------------------------------------------------------------

func TestCIWorkflowGoVetStep(t *testing.T) {

	run := stepRunBodies(t)
	if !strings.Contains(run, "go vet ./...") {
		t.Errorf("ci.yml: `go vet./...` step missing")
	}
}

// ---------------------------------------------------------------------------
// go test ./... at repo root
// Covers go test ./... at root and is paired with per-suite loop test
// ---------------------------------------------------------------------------

func TestCIWorkflowGoTestRootStep(t *testing.T) {

	run := stepRunBodies(t)
	if !strings.Contains(run, "go test ./...") {
		t.Errorf("ci.yml: `go test./...` step missing for repo root")
	}
}

// ---------------------------------------------------------------------------
// per-suite tests/*/go.mod loop step exists and is correct Covers iterate
// tests/*/go.mod modules; skip e2e + support
// ---------------------------------------------------------------------------

func TestCIWorkflowPerSuiteModuleLoop(t *testing.T) {

	var loopStep map[string]interface{}
	for _, s := range jobSteps(t) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		run, _ := m["run"].(string)
		if strings.Contains(run, "tests/*/go.mod") && strings.Contains(run, "for ") {
			loopStep = m
			break
		}
	}
	if loopStep == nil {
		t.Fatalf("ci.yml: per-suite module test loop not found (requires iterating tests/*/go.mod)")
	}

	// Must use bash (Windows default is PowerShell)
	shell, ok := loopStep["shell"].(string)
	if !ok || shell != "bash" {
		t.Errorf("ci.yml: per-suite loop step must set `shell: bash` for Windows compat, got %q", shell)
	}

	// Must have a step-level timeout
	if _, ok := loopStep["timeout-minutes"]; !ok {
		t.Errorf("ci.yml: per-suite loop step must set step-level `timeout-minutes` (Task 2.2 requires 20m)")
	}

	run := loopStep["run"].(string)

	// Must skip e2e and support Task 2.3
	if !strings.Contains(run, "e2e") || !strings.Contains(run, "support") {
		t.Errorf("ci.yml: per-suite loop must skip tests/e2e and tests/support (Task 2.3)")
	}

	// Must fail when no modules found (count guard)
	if !strings.Contains(run, "exit 1") {
		t.Errorf("ci.yml: per-suite loop must fail-fast when zero modules found (Task 2.2 count guard)")
	}

	// Must run go test inside each module dir
	if !strings.Contains(run, "go test") {
		t.Errorf("ci.yml: per-suite loop must invoke `go test` inside each module")
	}

	// Must use set -euo pipefail or equivalent bash strictness
	if !strings.Contains(run, "set -e") && !strings.Contains(run, "set -euo") {
		t.Errorf("ci.yml: per-suite loop must use `set -euo pipefail` for safety")
	}
}

// ---------------------------------------------------------------------------
// per-suite module directories actually exist at expected count Covers 20 per-suite
// go.mod files -- guards against accidental deletion
// ---------------------------------------------------------------------------

func TestPerSuiteGoModuleCount(t *testing.T) {

	root := projectRoot(t)
	testsDir := filepath.Join(root, "tests")
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		t.Fatalf("tests/ dir not readable: %v", err)
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// e2e and support are excluded by the loop, but the count here is the raw
		// total of tests/<name>/go.mod files so the story's "20" number can be asserted.
		if _, err := os.Stat(filepath.Join(testsDir, e.Name(), "go.mod")); err == nil {
			count++
		}
	}
	// Story says 20 today; adding this suite (ci-pipeline) makes 21. Accept >= 20.
	if count < 20 {
		t.Errorf("tests/*/go.mod count is %d; the CI per-suite loop expects at least 20 (if someone moved the per-suite modules, update the CI loop and this test together)", count)
	}
}

// ---------------------------------------------------------------------------
// Frontend lint + typecheck + test steps Covers tsc --noEmit,
// ESLint, Vitest on frontend
// ---------------------------------------------------------------------------

func TestCIWorkflowFrontendSteps(t *testing.T) {

	run := stepRunBodies(t)

	// npm ci --prefix frontend (frontend dep install)
	if !strings.Contains(run, "npm ci") || !strings.Contains(run, "frontend") {
		t.Errorf("ci.yml: frontend dep install (`npm ci --prefix frontend`) missing")
	}

	// TypeScript type-check (tsc --noEmit) OR `npm run typecheck`
	typecheckRe := regexp.MustCompile(`tsc --noEmit|npm run typecheck`)
	if !typecheckRe.MatchString(run) {
		t.Errorf("ci.yml: frontend typecheck step (tsc --noEmit or npm run typecheck) missing")
	}

	// ESLint via npm run lint
	lintRe := regexp.MustCompile(`npm run lint|eslint `)
	if !lintRe.MatchString(run) {
		t.Errorf("ci.yml: frontend lint step (npm run lint or eslint) missing")
	}

	// Vitest via npm run test
	testRe := regexp.MustCompile(`npm run test|vitest`)
	if !testRe.MatchString(run) {
		t.Errorf("ci.yml: frontend Vitest run (npm run test) missing")
	}
}

// ---------------------------------------------------------------------------
// wails3 build step present Covers wails3 build succeeds for
// each matrix platform
// ---------------------------------------------------------------------------

func TestCIWorkflowWailsBuildStep(t *testing.T) {

	run := stepRunBodies(t)
	if !strings.Contains(run, "wails3 build") {
		t.Errorf("ci.yml: `wails3 build` step missing")
	}
}

// ---------------------------------------------------------------------------
// CLI sanity build (go build ./cmd/cli/) Covers Task 1.5 step
// 16 (CLI compile check)
// ---------------------------------------------------------------------------

func TestCIWorkflowCLIBuildStep(t *testing.T) {

	run := stepRunBodies(t)
	if !strings.Contains(run, "go build") || !strings.Contains(run, "cmd/cli") {
		t.Errorf("ci.yml: `go build -o bin/pdfdebug ./cmd/cli/` sanity build missing (Task 1.5 step 16)")
	}
}

// ---------------------------------------------------------------------------
// checkout@v5 is the first action step Covers Task 1.5 step
// 1 (ordering requirement)
// ---------------------------------------------------------------------------

func TestCIWorkflowCheckoutIsFirst(t *testing.T) {

	uses := stepUses(t)
	if len(uses) == 0 {
		t.Fatalf("ci.yml: no `uses:` steps found")
	}
	if !strings.HasPrefix(uses[0], "actions/checkout@v5") {
		t.Errorf("ci.yml: first step must be actions/checkout@v5, got %q", uses[0])
	}
}
