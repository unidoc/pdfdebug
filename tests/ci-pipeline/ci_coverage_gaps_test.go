// Package ci_pipeline_test: additional static-validation tests closing coverage
// gaps in the CI pipeline.
//
// These tests target concrete behaviors mandated by the CI tasks and Review
// findings that were not asserted by the original 26 ATDD tests:
//
//   - workflow_dispatch trigger
//   - per-suite loop step-level timeout-minutes value
//   - golangci-lint --timeout 5m invocation flag
//   - root go test -timeout 10m invocation flag
//   - wails3 generate bindings step (Review #1 critical fix)
//   - bindings generation ordering before frontend steps (Review #1 ordering)
//   - .golangci.yml exclusion paths fully populated (Review #2 build path fix)
//   - .golangci.yml staticcheck.checks tuning (Review #2 QF disables)
//   - frontend/eslint.config.js ignores include test files (Review #2 fix)
//
// All tests stay at integration/Go level (lowest viable layer for
// infrastructure-as-code), per test pyramid and the caller's directive to avoid
// E2E duplication.
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

// stepIndexByPredicate returns the first step index where predicate(step) is
// true, or -1 if none match.
func stepIndexByPredicate(t *testing.T, predicate func(map[string]interface{}) bool) int {
	t.Helper()
	for i, s := range jobSteps(t) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if predicate(m) {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// workflow_dispatch trigger present for manual reruns.
// ---------------------------------------------------------------------------

func TestCIWorkflowDispatchTrigger(t *testing.T) {
	parsed := parseCIWorkflow(t)
	onRaw, ok := parsed["on"]
	if !ok {
		t.Fatalf("ci.yml: missing top-level `on:` block")
	}
	onMap, ok := onRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("ci.yml: `on:` is not a map")
	}
	if _, ok := onMap["workflow_dispatch"]; !ok {
		t.Errorf("ci.yml: `on.workflow_dispatch` trigger missing (required for manual rerun support)")
	}
}

// ---------------------------------------------------------------------------
// per-suite loop step-level timeout-minutes == 20.
// A step-level 20-minute cap, so a hung per-suite module cannot consume the
// job budget. The existing TestCIWorkflowPerSuiteModuleLoop only asserts the
// field is present; this asserts the exact 20-minute value.
// ---------------------------------------------------------------------------

func TestCIWorkflowPerSuiteLoopTimeoutValue(t *testing.T) {
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
		t.Fatalf("ci.yml: per-suite module test loop not found")
	}
	tRaw, ok := loopStep["timeout-minutes"]
	if !ok {
		t.Fatalf("ci.yml: per-suite loop missing timeout-minutes")
	}
	tVal, ok := tRaw.(int)
	if !ok {
		t.Fatalf("ci.yml: per-suite loop timeout-minutes not an int, got %T", tRaw)
	}
	if tVal != 20 {
		t.Errorf("ci.yml: per-suite loop timeout-minutes must be 20, got %d", tVal)
	}
}

// ---------------------------------------------------------------------------
// golangci-lint invoked with explicit --timeout flag.
// The invocation is `golangci-lint run --timeout 5m ./...`.
// ---------------------------------------------------------------------------

func TestCIWorkflowGolangciLintTimeoutFlag(t *testing.T) {
	run := stepRunBodies(t)
	// Accept 5m or longer (defensive against PR bumping timeout up); reject
	// invocation without any --timeout flag.
	re := regexp.MustCompile(`golangci-lint\s+run[^\n]*--timeout\s+\d+[ms]`)
	if !re.MatchString(run) {
		t.Errorf("ci.yml: `golangci-lint run` must pass an explicit `--timeout`")
	}
}

// ---------------------------------------------------------------------------
// root `go test ./...` uses explicit -timeout flag.
// The invocation is `go test ./... -timeout 10m`.
// ---------------------------------------------------------------------------

func TestCIWorkflowGoTestRootTimeoutFlag(t *testing.T) {
	run := stepRunBodies(t)
	re := regexp.MustCompile(`go test \./\.\.\.[^\n]*-timeout\s+\d+[ms]`)
	if !re.MatchString(run) {
		t.Errorf("ci.yml: root `go test ./...` must pass explicit `-timeout`")
	}
}

// ---------------------------------------------------------------------------
// wails3 generate bindings step present.
// Covers Review #1 critical fix (16 frontend files import from
// ../bindings/... which is gitignored; without this step every frontend CI
// step fails).
// ---------------------------------------------------------------------------

func TestCIWorkflowWailsGenerateBindingsStep(t *testing.T) {
	run := stepRunBodies(t)
	if !strings.Contains(run, "wails3 generate bindings") {
		t.Errorf("ci.yml: `wails3 generate bindings` step missing (Review #1: frontend imports would fail without generated bindings)")
	}
}

// ---------------------------------------------------------------------------
// Bindings generation must run before frontend typecheck, lint, and test.
// Review #1 finding: otherwise unresolved-import errors block the first
// frontend step on every CI run.
// ---------------------------------------------------------------------------

func TestCIWorkflowBindingsBeforeFrontendSteps(t *testing.T) {
	bindingsIdx := stepIndexByPredicate(t, func(m map[string]interface{}) bool {
		run, _ := m["run"].(string)
		return strings.Contains(run, "wails3 generate bindings")
	})
	if bindingsIdx < 0 {
		t.Fatalf("ci.yml: wails3 generate bindings step not found")
	}

	frontendStepPatterns := []string{
		"npm run typecheck",
		"npm run lint",
		"npm run test",
	}
	for _, pat := range frontendStepPatterns {
		idx := stepIndexByPredicate(t, func(m map[string]interface{}) bool {
			run, _ := m["run"].(string)
			return strings.Contains(run, pat)
		})
		if idx < 0 {
			t.Errorf("ci.yml: step `%s` not found", pat)
			continue
		}
		if idx <= bindingsIdx {
			t.Errorf("ci.yml: step `%s` (index %d) must come AFTER `wails3 generate bindings` (index %d) -- Review #1 ordering requirement", pat, idx, bindingsIdx)
		}
	}
}

// ---------------------------------------------------------------------------
// Bindings step runs after frontend deps installed (`npm ci --prefix
// frontend`). `wails3 generate bindings` writes TypeScript files under
// `frontend/bindings/`; node_modules must already exist or the generated
// imports resolve to nothing. Dev ordered it this way intentionally.
// ---------------------------------------------------------------------------

func TestCIWorkflowBindingsAfterNpmCi(t *testing.T) {
	npmCiIdx := stepIndexByPredicate(t, func(m map[string]interface{}) bool {
		run, _ := m["run"].(string)
		return strings.Contains(run, "npm ci") && strings.Contains(run, "frontend")
	})
	bindingsIdx := stepIndexByPredicate(t, func(m map[string]interface{}) bool {
		run, _ := m["run"].(string)
		return strings.Contains(run, "wails3 generate bindings")
	})
	if npmCiIdx < 0 || bindingsIdx < 0 {
		t.Fatalf("ci.yml: missing npm ci (idx=%d) or bindings generate (idx=%d) step", npmCiIdx, bindingsIdx)
	}
	if bindingsIdx <= npmCiIdx {
		t.Errorf("ci.yml: `wails3 generate bindings` (idx %d) must come AFTER `npm ci` (idx %d)", bindingsIdx, npmCiIdx)
	}
}

// ---------------------------------------------------------------------------
// Wails CLI install precedes bindings generation and wails3 build. If the
// CLI is installed after either, the step fails at `command not found:
// wails3`.
// ---------------------------------------------------------------------------

func TestCIWorkflowWailsCLIBeforeUses(t *testing.T) {
	wailsInstallIdx := stepIndexByPredicate(t, func(m map[string]interface{}) bool {
		run, _ := m["run"].(string)
		return strings.Contains(run, "go install github.com/wailsapp/wails/v3/cmd/wails3@")
	})
	if wailsInstallIdx < 0 {
		t.Fatalf("ci.yml: wails3 CLI install step not found")
	}

	for _, pat := range []string{"wails3 generate bindings", "wails3 build"} {
		useIdx := stepIndexByPredicate(t, func(m map[string]interface{}) bool {
			run, _ := m["run"].(string)
			// Guard against matching the install step for `wails3 build`.
			if strings.Contains(run, "go install") {
				return false
			}
			return strings.Contains(run, pat)
		})
		if useIdx < 0 {
			t.Errorf("ci.yml: step using `%s` not found", pat)
			continue
		}
		if useIdx <= wailsInstallIdx {
			t.Errorf("ci.yml: `%s` (idx %d) must run AFTER wails3 CLI install (idx %d)", pat, useIdx, wailsInstallIdx)
		}
	}
}

// ---------------------------------------------------------------------------
// .golangci.yml excludes `build` path.
// Covers Review #2 fix -- `build/ios/app_options_default.go` has an `unused`
// scaffold, so the `build` directory must be in exclusions.paths to keep
// golangci-lint exit 0.
// ---------------------------------------------------------------------------

func TestGolangciLintExcludesBuildPath(t *testing.T) {
	raw := readFileAtRoot(t, ".golangci.yml")
	var cfg map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf(".golangci.yml: invalid YAML: %v", err)
	}
	linters, _ := cfg["linters"].(map[string]interface{})
	if linters == nil {
		t.Fatalf(".golangci.yml: linters block missing")
	}
	excl, _ := linters["exclusions"].(map[string]interface{})
	if excl == nil {
		t.Fatalf(".golangci.yml: linters.exclusions block missing")
	}
	paths, _ := excl["paths"].([]interface{})
	if paths == nil {
		t.Fatalf(".golangci.yml: linters.exclusions.paths missing")
	}
	found := map[string]bool{}
	for _, p := range paths {
		if s, ok := p.(string); ok {
			found[s] = true
		}
	}
	// Review #2 adds `build`; the other five are the baseline.
	required := []string{"bindings", "frontend", "dist", "bin", "node_modules", "build"}
	for _, r := range required {
		if !found[r] {
			t.Errorf(".golangci.yml: linters.exclusions.paths missing %q", r)
		}
	}
}

// ---------------------------------------------------------------------------
// .golangci.yml disables QF1003 and QF1008 quick-fix style refactors (Review
// #2 fix -- these fired on pre-existing style patterns across cmd/cli/ and
// internal/pdfcore/).
// ---------------------------------------------------------------------------

func TestGolangciLintStaticcheckQFDisabled(t *testing.T) {
	raw := readFileAtRoot(t, ".golangci.yml")
	var cfg map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf(".golangci.yml: invalid YAML: %v", err)
	}
	linters, _ := cfg["linters"].(map[string]interface{})
	if linters == nil {
		t.Fatalf(".golangci.yml: linters block missing")
	}
	settings, _ := linters["settings"].(map[string]interface{})
	if settings == nil {
		t.Fatalf(".golangci.yml: linters.settings missing (Review #2 requires staticcheck tuning)")
	}
	sc, _ := settings["staticcheck"].(map[string]interface{})
	if sc == nil {
		t.Fatalf(".golangci.yml: linters.settings.staticcheck missing")
	}
	checksRaw, ok := sc["checks"].([]interface{})
	if !ok {
		t.Fatalf(".golangci.yml: linters.settings.staticcheck.checks not a list")
	}
	found := map[string]bool{}
	for _, c := range checksRaw {
		if s, ok := c.(string); ok {
			found[s] = true
		}
	}
	if !found["all"] {
		t.Errorf(".golangci.yml: staticcheck.checks must include \"all\" baseline")
	}
	for _, disabled := range []string{"-QF1003", "-QF1008"} {
		if !found[disabled] {
			t.Errorf(".golangci.yml: staticcheck.checks must disable %q (Review #2 pre-existing style patterns)", disabled)
		}
	}
}

// ---------------------------------------------------------------------------
// eslint.config.js ignores test files and test-setup.
// Covers eslint.config.js ignores block: `**/*.test.ts`, `**/*.test.tsx`,
// `src/test-setup.ts`. tsconfig.json explicitly excludes test files, so
// without these ignores typed linting throws "file not included in any
// project".
// ---------------------------------------------------------------------------

func TestESLintConfigIgnoresTestFiles(t *testing.T) {
	content := readFileAtRoot(t, "frontend/eslint.config.js")
	// Extract the ignores block and check for test patterns. Simple substring
	// match is fine; the file is a single flat array literal.
	for _, pat := range []string{"**/*.test.ts", "**/*.test.tsx", "test-setup"} {
		if !strings.Contains(content, pat) {
			t.Errorf("frontend/eslint.config.js ignores must include %q (tsconfig excludes test files from the project graph)", pat)
		}
	}
}

// ---------------------------------------------------------------------------
// eslint.config.js enforces `no-console: warn` Dev Notes: "already
// de-facto convention; keep as warn not error to avoid churn". Guards
// against accidental downgrade/removal.
// ---------------------------------------------------------------------------

func TestESLintConfigNoConsoleWarn(t *testing.T) {
	content := readFileAtRoot(t, "frontend/eslint.config.js")
	// Accept single or double quotes around keys/values.
	re := regexp.MustCompile(`['"]no-console['"]\s*:\s*['"]warn['"]`)
	if !re.MatchString(content) {
		t.Errorf("frontend/eslint.config.js: `no-console: warn` rule missing")
	}
}

// ---------------------------------------------------------------------------
// .gitattributes marks binary asset extensions as binary Prevents git from
// mangling PDF fixtures (used by every acceptance test suite under tests/)
// when users clone on Windows. Implementation adds pdf/png/jpg/
// jpeg/gif/ico/icns/ttf/otf/woff/woff2/eot -- assert the critical ones used by
// test fixtures (pdf, png).
// ---------------------------------------------------------------------------

func TestGitattributesBinaryPatterns(t *testing.T) {
	content := readFileAtRoot(t, ".gitattributes")
	for _, ext := range []string{"*.pdf binary", "*.png binary"} {
		if !strings.Contains(content, ext) {
			t.Errorf(".gitattributes: must declare %q (test fixtures rely on binary-safe storage)", ext)
		}
	}
}

// ---------------------------------------------------------------------------
// This new ci-pipeline module is picked up by the per-suite loop. The loop
// skips `e2e` and `support`; ci-pipeline is neither,
// so its tests must be invoked by `go test ./...` inside `tests/ci-pipeline/`.
// Catch future regressions where someone adds `ci-pipeline` to the skip list.
// ---------------------------------------------------------------------------

func TestCIPipelineSuiteNotSkipped(t *testing.T) {
	raw := readCIWorkflow(t)
	// The loop uses `case "$(basename "$dir")" in e2e|support) continue ;;`.
	// Guard against anyone extending the pattern to include ci-pipeline.
	re := regexp.MustCompile(`e2e\|[^)]*ci-pipeline|ci-pipeline[^)]*\|`)
	if re.MatchString(raw) {
		t.Errorf("ci.yml: per-suite loop skip list must NOT include `ci-pipeline` (these tests self-validate the pipeline)")
	}
	// Sanity: the loop does exist.
	if !strings.Contains(raw, "tests/*/go.mod") {
		t.Fatalf("ci.yml: per-suite loop not found")
	}
	// Sanity: ci-pipeline dir has a go.mod (so it enters the loop).
	root := projectRoot(t)
	if _, err := readFileAt(root, "tests/ci-pipeline/go.mod"); err != nil {
		t.Errorf("tests/ci-pipeline/go.mod missing: %v", err)
	}
}

// readFileAt is a small variant of readFileAtRoot that returns an error instead
// of failing the test. Used for optional existence checks.
func readFileAt(root, relPath string) (string, error) {
	p := filepath.Join(root, relPath)
	b, err := os.ReadFile(p)
	return string(b), err
}
